//go:build integration

package notifications_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/KarimMFadel/halaqaty/backend/internal/notifications"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestRepositoryListIsRecipientScopedBoundedAndCursorStable(t *testing.T) {
	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		t.Skip("DATABASE_URL is not set")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	userIDs := make([]uuid.UUID, 0, 2)
	t.Cleanup(func() {
		for _, id := range userIDs {
			if _, err := pool.Exec(context.Background(), "DELETE FROM users WHERE id = $1", id); err != nil {
				t.Errorf("delete notification integration user: %v", err)
			}
		}
		pool.Close()
	})
	userID := createNotificationTestUser(t, ctx, pool)
	userIDs = append(userIDs, userID)
	otherUserID := createNotificationTestUser(t, ctx, pool)
	userIDs = append(userIDs, otherUserID)

	for i, createdAt := range []time.Time{time.Now().Add(-time.Hour), time.Now().Add(-2 * time.Hour)} {
		_, err := pool.Exec(ctx, `
INSERT INTO notifications (
    user_id, type, source_event_id, source_ref_type, source_ref_id,
    title, body, created_at, push_expires_at
) VALUES ($1, 'group_message', $2, 'message', $3, 'New message', 'Open the app to view it', $4::timestamptz, $4::timestamptz + INTERVAL '24 hours')`,
			userID, fmt.Sprintf("recent-%d", i), uuid.New(), createdAt)
		if err != nil {
			t.Fatal(err)
		}
	}
	_, err = pool.Exec(ctx, `
INSERT INTO notifications (
    user_id, type, source_event_id, source_ref_type, source_ref_id,
    title, body, created_at, push_expires_at
) VALUES ($1, 'group_message', 'old', 'message', $2, 'Old message', 'Open the app to view it', NOW() - INTERVAL '31 days', NOW())`,
		userID, uuid.New())
	if err != nil {
		t.Fatal(err)
	}
	_, err = pool.Exec(ctx, `
INSERT INTO notifications (
    user_id, type, source_event_id, source_ref_type, source_ref_id, title, body, push_expires_at
) VALUES ($1, 'group_message', 'other-user', 'message', $2, 'Other', 'Other user', NOW() + INTERVAL '24 hours')`,
		otherUserID, uuid.New())
	if err != nil {
		t.Fatal(err)
	}

	page, err := notifications.NewRepository(pool).List(ctx, userID.String(), nil, 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Items) != 1 || page.UnreadCount != 2 || page.NextCursor == nil {
		t.Fatalf("first page = %#v; want one item, unread=2, and next cursor", page)
	}
	second, err := notifications.NewRepository(pool).List(ctx, userID.String(), page.NextCursor, 50)
	if err != nil {
		t.Fatal(err)
	}
	if len(second.Items) != 1 || second.UnreadCount != 2 || second.NextCursor != nil {
		t.Fatalf("second page = %#v; want one item, unread=2, and no next cursor", second)
	}

	if _, err := pool.Exec(ctx, "UPDATE notifications SET is_read = TRUE, read_at = NOW() WHERE id = $1", second.Items[0].ID); err != nil {
		t.Fatal(err)
	}
	updated, err := notifications.NewRepository(pool).List(ctx, userID.String(), nil, 50)
	if err != nil {
		t.Fatal(err)
	}
	if len(updated.Items) != 2 || updated.UnreadCount != 1 {
		t.Fatalf("updated page = %#v; want two recent items and unread=1", updated)
	}
}

func TestRepositoryPreferencesResolveDefaultsAndUpdateAtomically(t *testing.T) {
	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		t.Skip("DATABASE_URL is not set")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	userID := createNotificationTestUser(t, ctx, pool)
	t.Cleanup(func() {
		if _, err := pool.Exec(context.Background(), "DELETE FROM users WHERE id = $1", userID); err != nil {
			t.Errorf("delete notification preference integration user: %v", err)
		}
		pool.Close()
	})
	if _, err := pool.Exec(ctx, "INSERT INTO profiles (user_id, timezone) VALUES ($1, 'Asia/Riyadh')", userID); err != nil {
		t.Fatal(err)
	}

	repo := notifications.NewRepository(pool)
	preferences, err := repo.GetPreferences(ctx, userID.String())
	if err != nil {
		t.Fatal(err)
	}
	if len(preferences.Types) != len(notifications.EventPreferences()) ||
		!preferences.QuietHours.Enabled || preferences.QuietHours.StartLocal != "22:00" ||
		preferences.QuietHours.EndLocal != "07:00" || preferences.QuietHours.Timezone != "Asia/Riyadh" {
		t.Fatalf("default preferences = %#v", preferences)
	}

	if err := repo.UpdatePreferences(ctx, userID.String(), []notifications.TypeSettingUpdate{
		{Type: notifications.EventGroupMessage, Enabled: false},
	}, &notifications.QuietHoursUpdate{Enabled: false, StartLocal: "23:00", EndLocal: "06:00"}); err != nil {
		t.Fatal(err)
	}
	preferences, err = repo.GetPreferences(ctx, userID.String())
	if err != nil {
		t.Fatal(err)
	}
	if preferences.Types[1].Enabled || preferences.QuietHours.Enabled ||
		preferences.QuietHours.StartLocal != "23:00" || preferences.QuietHours.EndLocal != "06:00" {
		t.Fatalf("updated preferences = %#v", preferences)
	}
	if err := repo.UpdatePreferences(ctx, userID.String(), []notifications.TypeSettingUpdate{
		{Type: notifications.EventPrivateMessage, Enabled: false},
	}, nil); !errors.Is(err, notifications.ErrPreferenceNotConfigurable) {
		t.Fatalf("always-on update error = %v, want ErrPreferenceNotConfigurable", err)
	}
	if err := repo.UpdatePreferences(ctx, userID.String(), nil,
		&notifications.QuietHoursUpdate{Enabled: true, StartLocal: "22:00", EndLocal: "22:00"}); !errors.Is(err, notifications.ErrInvalidQuietHours) {
		t.Fatalf("equal quiet-hours update error = %v, want ErrInvalidQuietHours", err)
	}
}

func TestCreateIsIdempotentAndJoinsSourceTransaction(t *testing.T) {
	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		t.Skip("DATABASE_URL is not set")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	userID := createNotificationTestUser(t, ctx, pool)
	t.Cleanup(func() {
		if _, err := pool.Exec(context.Background(), "DELETE FROM users WHERE id = $1", userID); err != nil {
			t.Errorf("delete notification creation integration user: %v", err)
		}
		pool.Close()
	})

	input := notifications.CreateInput{
		RecipientID:   userID,
		Type:          notifications.EventCircleInvitation,
		SourceEventID: "invitation-1",
		SourceRef: notifications.SourceRef{
			Type: notifications.SourceInvitation,
			ID:   uuid.New(),
		},
		Title:         "Circle invitation",
		Body:          "You have a new invitation",
		PushExpiresAt: time.Now().Add(24 * time.Hour),
	}
	repo := notifications.NewRepository(pool)
	id, inserted, err := repo.Create(ctx, input)
	if err != nil || !inserted || id == uuid.Nil {
		t.Fatalf("first create = (%s, %t, %v), want inserted", id, inserted, err)
	}
	replayedID, inserted, err := repo.Create(ctx, input)
	if err != nil || inserted || replayedID != id {
		t.Fatalf("replay = (%s, %t, %v), want same id and inserted=false", replayedID, inserted, err)
	}
	var count int
	if err := pool.QueryRow(ctx, "SELECT COUNT(*) FROM notifications WHERE user_id=$1", userID).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("notification count after replay=%d, want 1", count)
	}

	input.SourceEventID = "rolled-back-event"
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, inserted, err := notifications.CreateInTx(ctx, tx, input); err != nil || !inserted {
		t.Fatalf("transactional create = inserted %t, err %v", inserted, err)
	}
	if err := tx.Rollback(ctx); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, "SELECT COUNT(*) FROM notifications WHERE user_id=$1", userID).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("notification count after source rollback=%d, want 1", count)
	}
}

func TestCreateHoldsOnlyWhenNoDeviceTokenExists(t *testing.T) {
	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		t.Skip("DATABASE_URL is not set")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	withTokenUser := createNotificationTestUser(t, ctx, pool)
	withoutTokenUser := createNotificationTestUser(t, ctx, pool)
	t.Cleanup(func() {
		if _, err := pool.Exec(context.Background(), "DELETE FROM users WHERE id = $1", withTokenUser); err != nil {
			t.Errorf("delete creation-gate integration user: %v", err)
		}
		if _, err := pool.Exec(context.Background(), "DELETE FROM users WHERE id = $1", withoutTokenUser); err != nil {
			t.Errorf("delete creation-gate integration user: %v", err)
		}
		pool.Close()
	})
	if _, err := pool.Exec(ctx, `
INSERT INTO device_tokens (user_id, token, platform) VALUES ($1, 'f008-existing-registration', 'android')`,
		withTokenUser); err != nil {
		t.Fatal(err)
	}

	repo := notifications.NewRepository(pool)
	newIntent := func(recipient uuid.UUID, sourceEventID string) notifications.CreateInput {
		return notifications.CreateInput{
			RecipientID:   recipient,
			Type:          notifications.EventGroupMessage,
			SourceEventID: sourceEventID,
			SourceRef: notifications.SourceRef{
				Type: notifications.SourceMessage,
				ID:   uuid.New(),
			},
			Title:         "New message",
			Body:          "Open the app to view it",
			PushExpiresAt: time.Now().Add(24 * time.Hour),
		}
	}
	if _, inserted, err := repo.Create(ctx, newIntent(withTokenUser, "existing-token-intent")); err != nil || !inserted {
		t.Fatalf("create with registered token = %t, err %v", inserted, err)
	}
	if _, inserted, err := repo.Create(ctx, newIntent(withoutTokenUser, "no-token-intent")); err != nil || !inserted {
		t.Fatalf("create without registered token = %t, err %v", inserted, err)
	}

	var withTokenState string
	if err := pool.QueryRow(ctx, `
SELECT delivery_state FROM notifications WHERE user_id = $1 AND source_event_id = 'existing-token-intent'`,
		withTokenUser).Scan(&withTokenState); err != nil {
		t.Fatal(err)
	}
	if withTokenState != "pending_attempt" {
		t.Fatalf("state with registered token = %q, want pending_attempt (only the no-token case holds for registration)", withTokenState)
	}
	var withoutTokenState string
	if err := pool.QueryRow(ctx, `
SELECT delivery_state FROM notifications WHERE user_id = $1 AND source_event_id = 'no-token-intent'`,
		withoutTokenUser).Scan(&withoutTokenState); err != nil {
		t.Fatal(err)
	}
	if withoutTokenState != "pending_no_device" {
		t.Fatalf("state without registered token = %q, want pending_no_device", withoutTokenState)
	}
}

func createNotificationTestUser(t *testing.T, ctx context.Context, pool *pgxpool.Pool) uuid.UUID {
	t.Helper()
	userID := uuid.New()
	if _, err := pool.Exec(ctx, "INSERT INTO users (id, firebase_uid, email) VALUES ($1, $2, $3)",
		userID, "f008-integration-"+userID.String(), userID.String()+"@example.test"); err != nil {
		t.Fatal(err)
	}
	return userID
}

// insertTestNotification persists one unread history row and returns its id.
func insertTestNotification(t *testing.T, ctx context.Context, pool *pgxpool.Pool, userID uuid.UUID, sourceEventID string, createdAt time.Time) uuid.UUID {
	t.Helper()
	var id uuid.UUID
	if err := pool.QueryRow(ctx, `
INSERT INTO notifications (
    user_id, type, source_event_id, source_ref_type, source_ref_id,
    title, body, created_at, push_expires_at
) VALUES ($1, 'group_message', $2, 'message', $3, 'New message', 'Open the app to view it', $4::timestamptz, NOW() + INTERVAL '24 hours')
RETURNING id`,
		userID, sourceEventID, uuid.New(), createdAt).Scan(&id); err != nil {
		t.Fatal(err)
	}
	return id
}

func TestRepositoryMarkReadIsIdempotentAndRecipientScoped(t *testing.T) {
	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		t.Skip("DATABASE_URL is not set")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	userIDs := make([]uuid.UUID, 0, 2)
	t.Cleanup(func() {
		for _, id := range userIDs {
			if _, err := pool.Exec(context.Background(), "DELETE FROM users WHERE id = $1", id); err != nil {
				t.Errorf("delete mark-read integration user: %v", err)
			}
		}
		pool.Close()
	})
	userID := createNotificationTestUser(t, ctx, pool)
	userIDs = append(userIDs, userID)
	otherUserID := createNotificationTestUser(t, ctx, pool)
	userIDs = append(userIDs, otherUserID)

	firstID := insertTestNotification(t, ctx, pool, userID, "mark-read-1", time.Now().Add(-time.Hour))
	insertTestNotification(t, ctx, pool, userID, "mark-read-2", time.Now().Add(-2*time.Hour))
	otherID := insertTestNotification(t, ctx, pool, otherUserID, "mark-read-other", time.Now().Add(-time.Hour))

	repo := notifications.NewRepository(pool)
	unread, err := repo.MarkRead(ctx, userID.String(), firstID)
	if err != nil || unread != 1 {
		t.Fatalf("first MarkRead = (%d, %v), want unread=1", unread, err)
	}
	// Re-marking an already-read notification is an idempotent success.
	unread, err = repo.MarkRead(ctx, userID.String(), firstID)
	if err != nil || unread != 1 {
		t.Fatalf("idempotent MarkRead = (%d, %v), want unread=1", unread, err)
	}
	// Foreign and unknown identifiers are indistinguishable (no disclosure).
	if _, err := repo.MarkRead(ctx, userID.String(), otherID); !errors.Is(err, notifications.ErrNotificationNotFound) {
		t.Fatalf("foreign MarkRead error = %v, want ErrNotificationNotFound", err)
	}
	if _, err := repo.MarkRead(ctx, userID.String(), uuid.New()); !errors.Is(err, notifications.ErrNotificationNotFound) {
		t.Fatalf("unknown MarkRead error = %v, want ErrNotificationNotFound", err)
	}

	var otherUnread int64
	if err := pool.QueryRow(ctx, countUnreadSQL, otherUserID).Scan(&otherUnread); err != nil {
		t.Fatal(err)
	}
	if otherUnread != 1 {
		t.Fatalf("other user unread=%d, want 1 (recipient scoping must not leak)", otherUnread)
	}
	var readAt *time.Time
	if err := pool.QueryRow(ctx, "SELECT read_at FROM notifications WHERE id = $1", firstID).Scan(&readAt); err != nil {
		t.Fatal(err)
	}
	if readAt == nil {
		t.Fatal("read_at is NULL after MarkRead, want a timestamp")
	}
}

func TestRepositoryMarkAllReadIsAtomicAndSynchronized(t *testing.T) {
	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		t.Skip("DATABASE_URL is not set")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	userID := createNotificationTestUser(t, ctx, pool)
	t.Cleanup(func() {
		if _, err := pool.Exec(context.Background(), "DELETE FROM users WHERE id = $1", userID); err != nil {
			t.Errorf("delete mark-all-read integration user: %v", err)
		}
		pool.Close()
	})

	insertTestNotification(t, ctx, pool, userID, "mark-all-1", time.Now().Add(-time.Hour))
	insertTestNotification(t, ctx, pool, userID, "mark-all-2", time.Now().Add(-2*time.Hour))
	expiredID := insertTestNotification(t, ctx, pool, userID, "mark-all-expired", time.Now().Add(-31*24*time.Hour))

	repo := notifications.NewRepository(pool)
	unread, err := repo.MarkAllRead(ctx, userID.String())
	if err != nil || unread != 0 {
		t.Fatalf("MarkAllRead = (%d, %v), want unread=0", unread, err)
	}

	// The count is window-bounded, so the bulk update must share the 30-day
	// window: rows outside it stay unread without poisoning the count.
	var expiredUnread bool
	if err := pool.QueryRow(ctx, "SELECT NOT is_read FROM notifications WHERE id = $1", expiredID).Scan(&expiredUnread); err != nil {
		t.Fatal(err)
	}
	if !expiredUnread {
		t.Fatal("expired notification was marked read; want it untouched outside the 30-day window")
	}

	page, err := repo.List(ctx, userID.String(), nil, 50)
	if err != nil {
		t.Fatal(err)
	}
	if page.UnreadCount != 0 {
		t.Fatalf("List unread=%d after MarkAllRead, want 0", page.UnreadCount)
	}
	for _, item := range page.Items {
		if !item.IsRead || item.ReadAt == nil {
			t.Fatalf("item %s is_read=%t read_at=%v, want read with timestamp", item.ID, item.IsRead, item.ReadAt)
		}
	}
	// A second read-all is a no-op that still reports the authoritative zero.
	unread, err = repo.MarkAllRead(ctx, userID.String())
	if err != nil || unread != 0 {
		t.Fatalf("repeat MarkAllRead = (%d, %v), want unread=0", unread, err)
	}
}

const countUnreadSQL = `
SELECT COUNT(*)
FROM notifications
WHERE user_id = $1
  AND NOT is_read
  AND created_at >= NOW() - INTERVAL '30 days'
`

// T015 follow-up: when exactly `limit` rows remain in the window, no extra
// row is fetched, so the page must not emit a cursor that would page into
// emptiness.
func TestRepositoryListExactLimitReturnsNoNextCursor(t *testing.T) {
	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		t.Skip("DATABASE_URL is not set")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	userID := createNotificationTestUser(t, ctx, pool)
	t.Cleanup(func() {
		if _, err := pool.Exec(context.Background(), "DELETE FROM users WHERE id = $1", userID); err != nil {
			t.Errorf("delete exact-limit integration user: %v", err)
		}
		pool.Close()
	})

	const rowCount = 3
	for i := 0; i < rowCount; i++ {
		if _, err := pool.Exec(ctx, `
	INSERT INTO notifications (
	    user_id, type, source_event_id, source_ref_type, source_ref_id, title, body, push_expires_at
	) VALUES ($1, 'group_message', $2, 'message', $3, 'New message', 'Open the app to view it', NOW() + INTERVAL '24 hours')`,
			userID, fmt.Sprintf("exact-%d", i), uuid.New()); err != nil {
			t.Fatal(err)
		}
	}

	page, err := notifications.NewRepository(pool).List(ctx, userID.String(), nil, rowCount)
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Items) != rowCount {
		t.Fatalf("items=%d, want exactly %d", len(page.Items), rowCount)
	}
	if page.NextCursor != nil {
		t.Fatalf("NextCursor=%+v, want nil when exactly limit rows remain", page.NextCursor)
	}
}

func TestRepositoryListCursorOrdersEqualTimestampsByIDDescending(t *testing.T) {
	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		t.Skip("DATABASE_URL is not set")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	userID := createNotificationTestUser(t, ctx, pool)
	t.Cleanup(func() {
		if _, err := pool.Exec(context.Background(), "DELETE FROM users WHERE id = $1", userID); err != nil {
			t.Errorf("delete equal-timestamp history user: %v", err)
		}
		pool.Close()
	})

	createdAt := time.Now().UTC().Truncate(time.Microsecond)
	olderID := uuid.MustParse("00000000-0000-4000-8000-000000000001")
	newerID := uuid.MustParse("00000000-0000-4000-8000-000000000002")
	for id, eventID := range map[uuid.UUID]string{
		olderID: "equal-time-older-id",
		newerID: "equal-time-newer-id",
	} {
		if _, err := pool.Exec(ctx, `
INSERT INTO notifications (
    id, user_id, type, source_event_id, source_ref_type, source_ref_id,
    title, body, created_at, push_expires_at
) VALUES ($1, $2, 'group_message', $3, 'message', $4,
			'New message', 'Open the app to view it', $5, $6)`,
			id, userID, eventID, uuid.New(), createdAt, createdAt.Add(24*time.Hour)); err != nil {
			t.Fatal(err)
		}
	}

	repo := notifications.NewRepository(pool)
	first, err := repo.List(ctx, userID.String(), nil, 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(first.Items) != 1 || first.Items[0].ID != newerID || first.NextCursor == nil {
		t.Fatalf("first equal-time page=%+v, want newer UUID %s and cursor", first, newerID)
	}
	second, err := repo.List(ctx, userID.String(), first.NextCursor, 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(second.Items) != 1 || second.Items[0].ID != olderID || second.NextCursor != nil {
		t.Fatalf("second equal-time page=%+v, want older UUID %s and no cursor", second, olderID)
	}
}

func TestRepositoryListHonorsNearThirtyDayBoundary(t *testing.T) {
	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		t.Skip("DATABASE_URL is not set")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	userID := createNotificationTestUser(t, ctx, pool)
	t.Cleanup(func() {
		if _, err := pool.Exec(context.Background(), "DELETE FROM users WHERE id = $1", userID); err != nil {
			t.Errorf("delete history-boundary integration user: %v", err)
		}
		pool.Close()
	})

	insideWindowID := insertTestNotification(t, ctx, pool, userID, "inside-window", time.Now().Add(-30*24*time.Hour+time.Minute))
	insertTestNotification(t, ctx, pool, userID, "outside-window", time.Now().Add(-30*24*time.Hour-time.Minute))

	page, err := notifications.NewRepository(pool).List(ctx, userID.String(), nil, 50)
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Items) != 1 || page.UnreadCount != 1 || page.Items[0].ID != insideWindowID {
		t.Fatalf("near-boundary page = %#v, want only the row newer than 30 days", page)
	}
}

func TestRepositoryListKeepsPageAndUnreadCountOnOneConcurrentSnapshot(t *testing.T) {
	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		t.Skip("DATABASE_URL is not set")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	userID := createNotificationTestUser(t, ctx, pool)
	t.Cleanup(func() {
		if _, err := pool.Exec(context.Background(), "DELETE FROM users WHERE id = $1", userID); err != nil {
			t.Errorf("delete history-concurrency integration user: %v", err)
		}
		pool.Close()
	})

	start := make(chan struct{})
	errors := make(chan error, 2)
	var workers sync.WaitGroup
	workers.Add(2)
	go func() {
		defer workers.Done()
		<-start
		for i := 0; i < 20; i++ {
			_, err := pool.Exec(ctx, `
INSERT INTO notifications (
    user_id, type, source_event_id, source_ref_type, source_ref_id,
    title, body, push_expires_at
) VALUES ($1, 'group_message', $2, 'message', $3, 'New message', 'Open the app to view it', NOW() + INTERVAL '24 hours')`,
				userID, fmt.Sprintf("snapshot-%d", i), uuid.New())
			if err != nil {
				errors <- fmt.Errorf("insert concurrent history row: %w", err)
				return
			}
		}
	}()
	go func() {
		defer workers.Done()
		<-start
		repo := notifications.NewRepository(pool)
		for i := 0; i < 40; i++ {
			page, err := repo.List(ctx, userID.String(), nil, 50)
			if err != nil {
				errors <- err
				return
			}
			if int64(len(page.Items)) != page.UnreadCount {
				errors <- fmt.Errorf("page size %d disagrees with unread snapshot %d", len(page.Items), page.UnreadCount)
				return
			}
		}
	}()
	close(start)
	workers.Wait()
	close(errors)
	for err := range errors {
		if err != nil {
			t.Fatal(err)
		}
	}
}
