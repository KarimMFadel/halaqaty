//go:build integration

package notifications_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/KarimMFadel/halaqaty/backend/internal/auth"
	"github.com/KarimMFadel/halaqaty/backend/internal/notifications"
)

// insertCleanupNotification inserts one history row with explicit age, delivery
// state, and override marker so retention boundaries are exercised directly.
func insertCleanupNotification(t *testing.T, ctx context.Context, pool *pgxpool.Pool,
	userID uuid.UUID, sourceEventID, createdInterval, deliveryState string, override bool) uuid.UUID {
	t.Helper()
	var id uuid.UUID
	err := pool.QueryRow(ctx, `
INSERT INTO notifications (
    user_id, type, source_event_id, source_ref_type, source_ref_id,
    title, body, created_at, push_expires_at, delivery_state, preference_override
) VALUES ($1, 'circle_archived', $2, 'circle', $3, 'Circle update', 'Open the app to view it',
          NOW() - $4::interval, NOW() + INTERVAL '1 hour', $5, $6)
RETURNING id`, userID, sourceEventID, uuid.New(), createdInterval, deliveryState, override).Scan(&id)
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func notificationExists(t *testing.T, ctx context.Context, pool *pgxpool.Pool, id uuid.UUID) bool {
	t.Helper()
	var exists bool
	if err := pool.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM notifications WHERE id = $1)", id).Scan(&exists); err != nil {
		t.Fatal(err)
	}
	return exists
}

func attemptCountFor(t *testing.T, ctx context.Context, pool *pgxpool.Pool, notificationID uuid.UUID) int {
	t.Helper()
	var count int
	if err := pool.QueryRow(ctx,
		"SELECT COUNT(*) FROM notification_device_attempts WHERE notification_id = $1", notificationID).Scan(&count); err != nil {
		t.Fatal(err)
	}
	return count
}

// T013 retention boundary: rows inside the rolling 30-day window survive
// cleanup; rows outside it are deleted with their attempt state via cascade.
func TestCleanupDeletesExpiredHistoryAndKeepsRecentWindow(t *testing.T) {
	pool := newDispatchIntegrationPool(t)
	ctx := context.Background()
	userID := createNotificationTestUser(t, ctx, pool)
	t.Cleanup(func() {
		if _, err := pool.Exec(context.Background(), "DELETE FROM users WHERE id = $1", userID); err != nil {
			t.Errorf("delete cleanup integration user: %v", err)
		}
	})
	authRepo := auth.NewSessionRepository(pool)
	if err := authRepo.UpsertDeviceToken(ctx, userID.String(), "cleanup-"+uuid.NewString(), "android", nil); err != nil {
		t.Fatal(err)
	}

	recent := insertCleanupNotification(t, ctx, pool, userID, "cleanup-recent", "29 days", "accepted_by_provider", false)
	expired := insertCleanupNotification(t, ctx, pool, userID, "cleanup-expired", "31 days", "accepted_by_provider", false)
	// Give both rows an attempt record so terminal-state removal (recent) and
	// cascade removal (expired) are each verified.
	for _, id := range []uuid.UUID{recent, expired} {
		if _, err := pool.Exec(ctx, `
INSERT INTO notification_device_attempts (notification_id, device_token_id, state)
SELECT $1, id, 'accepted_by_provider' FROM device_tokens WHERE user_id = $2`, id, userID); err != nil {
			t.Fatal(err)
		}
	}

	metrics := &notifications.NotificationMetrics{}
	cleaner := notifications.NewCleaner(notifications.NewRepository(pool), metrics)
	if err := cleaner.CleanOnce(ctx, 100); err != nil {
		t.Fatal(err)
	}

	if notificationExists(t, ctx, pool, expired) {
		t.Fatal("expired row past the 30-day window must be deleted")
	}
	if !notificationExists(t, ctx, pool, recent) {
		t.Fatal("recent row inside the 30-day window must survive cleanup")
	}
	if got := attemptCountFor(t, ctx, pool, expired); got != 0 {
		t.Fatalf("expired attempt rows=%d, want cascaded removal", got)
	}
	// The recent row is terminal: its attempt state is terminal cleanup scope
	// while the history row itself remains visible.
	if got := attemptCountFor(t, ctx, pool, recent); got != 0 {
		t.Fatalf("terminal recent attempt rows=%d, want removed by cleanup", got)
	}
	summary := metrics.Summary()
	if summary.CleanupHistoryDeleted != 1 || summary.CleanupAttemptsDeleted != 2 {
		t.Fatalf("cleanup metrics=(attempts %d, history %d), want (2, 1)",
			summary.CleanupAttemptsDeleted, summary.CleanupHistoryDeleted)
	}
	// The 30-day history window still hides the expired row even before
	// cleanup; the recent row stays visible.
	page, err := notifications.NewRepository(pool).List(ctx, userID.String(), nil, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Items) != 1 || page.Items[0].ID != recent {
		t.Fatalf("history page=%+v, want only the recent row inside the window", page.Items)
	}
}

// T013: pending required manager-deletion intents (preference_override) are
// retained past the 30-day window until terminal handling (ADR-027 §6); once
// terminal, they are cleaned like any other expired row.
func TestCleanupPreservesPendingOverrideIntentButCleansTerminalOverride(t *testing.T) {
	pool := newDispatchIntegrationPool(t)
	ctx := context.Background()
	userID := createNotificationTestUser(t, ctx, pool)
	t.Cleanup(func() {
		if _, err := pool.Exec(context.Background(), "DELETE FROM users WHERE id = $1", userID); err != nil {
			t.Errorf("delete override-cleanup integration user: %v", err)
		}
	})

	pendingNoDevice := insertCleanupNotification(t, ctx, pool, userID, "override-pending-no-device", "31 days", "pending_no_device", true)
	pendingAttempt := insertCleanupNotification(t, ctx, pool, userID, "override-pending-attempt", "31 days", "pending_attempt", true)
	terminalOverride := insertCleanupNotification(t, ctx, pool, userID, "override-terminal", "31 days", "expired", true)
	terminalOrdinary := insertCleanupNotification(t, ctx, pool, userID, "ordinary-terminal", "31 days", "suppressed_preference", false)

	cleaner := notifications.NewCleaner(notifications.NewRepository(pool), &notifications.NotificationMetrics{})
	if err := cleaner.CleanOnce(ctx, 100); err != nil {
		t.Fatal(err)
	}

	for _, id := range []uuid.UUID{pendingNoDevice, pendingAttempt} {
		if !notificationExists(t, ctx, pool, id) {
			t.Fatalf("pending override intent %s must survive past the 30-day window", id)
		}
	}
	for _, id := range []uuid.UUID{terminalOverride, terminalOrdinary} {
		if notificationExists(t, ctx, pool, id) {
			t.Fatalf("terminal expired row %s must be deleted past the 30-day window", id)
		}
	}

	// Once a pending override intent reaches a terminal state, the next cleanup
	// removes it like any other expired row.
	if _, err := pool.Exec(ctx,
		"UPDATE notifications SET delivery_state = 'accepted_by_provider' WHERE id = $1", pendingNoDevice); err != nil {
		t.Fatal(err)
	}
	if err := cleaner.CleanOnce(ctx, 100); err != nil {
		t.Fatal(err)
	}
	if notificationExists(t, ctx, pool, pendingNoDevice) {
		t.Fatal("override intent that settled terminal must be cleaned past the window")
	}
	if !notificationExists(t, ctx, pool, pendingAttempt) {
		t.Fatal("still-pending override intent must remain retained")
	}
}

// T013: attempt rows orphaned by a terminal intent (the expiry-sweep case
// documented in TestExpiredSweepWithLivePendingAttemptRows) are removed even
// while the notification history row remains inside the 30-day window.
func TestCleanupRemovesOrphanedAttemptStateOfTerminalNotifications(t *testing.T) {
	pool := newDispatchIntegrationPool(t)
	ctx := context.Background()
	userID := createNotificationTestUser(t, ctx, pool)
	t.Cleanup(func() {
		if _, err := pool.Exec(context.Background(), "DELETE FROM users WHERE id = $1", userID); err != nil {
			t.Errorf("delete orphan-cleanup integration user: %v", err)
		}
	})
	authRepo := auth.NewSessionRepository(pool)
	if err := authRepo.UpsertDeviceToken(ctx, userID.String(), "orphan-"+uuid.NewString(), "web", nil); err != nil {
		t.Fatal(err)
	}

	// A terminal expired intent whose attempt row is still pending (orphan).
	expiredIntent := insertCleanupNotification(t, ctx, pool, userID, "orphan-expired", "1 hour", "expired", false)
	// A live pending intent must keep its attempt rows untouched.
	liveIntent := insertCleanupNotification(t, ctx, pool, userID, "orphan-live", "1 hour", "pending_attempt", false)
	for _, id := range []uuid.UUID{expiredIntent, liveIntent} {
		if _, err := pool.Exec(ctx, `
INSERT INTO notification_device_attempts (notification_id, device_token_id, state)
SELECT $1, id, 'pending' FROM device_tokens WHERE user_id = $2`, id, userID); err != nil {
			t.Fatal(err)
		}
	}

	cleaner := notifications.NewCleaner(notifications.NewRepository(pool), &notifications.NotificationMetrics{})
	if err := cleaner.CleanOnce(ctx, 100); err != nil {
		t.Fatal(err)
	}

	if got := attemptCountFor(t, ctx, pool, expiredIntent); got != 0 {
		t.Fatalf("orphaned attempt rows=%d, want removed", got)
	}
	if got := attemptCountFor(t, ctx, pool, liveIntent); got != 1 {
		t.Fatalf("live intent attempt rows=%d, want preserved", got)
	}
	if !notificationExists(t, ctx, pool, expiredIntent) || !notificationExists(t, ctx, pool, liveIntent) {
		t.Fatal("history rows inside the 30-day window must be preserved")
	}
}

// T013: cleanup is bounded — one cycle deletes at most the configured batch
// per statement; repeated cycles drain the backlog.
func TestCleanupBatchBoundIsRespected(t *testing.T) {
	pool := newDispatchIntegrationPool(t)
	ctx := context.Background()
	userID := createNotificationTestUser(t, ctx, pool)
	t.Cleanup(func() {
		if _, err := pool.Exec(context.Background(), "DELETE FROM users WHERE id = $1", userID); err != nil {
			t.Errorf("delete batch-cleanup integration user: %v", err)
		}
	})

	const seeded = 5
	const bound = 3
	for i := 0; i < seeded; i++ {
		insertCleanupNotification(t, ctx, pool, userID, "batch-"+uuid.NewString(), "31 days", "expired", false)
	}

	cleaner := notifications.NewCleaner(notifications.NewRepository(pool), &notifications.NotificationMetrics{})
	if err := cleaner.CleanOnce(ctx, bound); err != nil {
		t.Fatal(err)
	}
	var remaining int
	if err := pool.QueryRow(ctx,
		"SELECT COUNT(*) FROM notifications WHERE user_id = $1", userID).Scan(&remaining); err != nil {
		t.Fatal(err)
	}
	if remaining != seeded-bound {
		t.Fatalf("remaining=%d after one cycle, want exactly %d (bound respected)", remaining, seeded-bound)
	}
	if err := cleaner.CleanOnce(ctx, bound); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx,
		"SELECT COUNT(*) FROM notifications WHERE user_id = $1", userID).Scan(&remaining); err != nil {
		t.Fatal(err)
	}
	if remaining != 0 {
		t.Fatalf("remaining=%d after second cycle, want drained backlog", remaining)
	}
}
