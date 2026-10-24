//go:build integration

package notifications_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/KarimMFadel/halaqaty/backend/internal/auth"
	"github.com/KarimMFadel/halaqaty/backend/internal/notifications"
)

func newDispatchIntegrationPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		t.Skip("DATABASE_URL is not set")
	}
	pool, err := pgxpool.New(context.Background(), databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	return pool
}

func deliveryStateOf(t *testing.T, ctx context.Context, pool *pgxpool.Pool, id uuid.UUID) string {
	t.Helper()
	var state string
	if err := pool.QueryRow(ctx, "SELECT delivery_state FROM notifications WHERE id = $1", id).Scan(&state); err != nil {
		t.Fatal(err)
	}
	return state
}

func disableQuietHours(t *testing.T, ctx context.Context, repo *notifications.Repository, userID uuid.UUID) {
	t.Helper()
	if err := repo.UpdatePreferences(ctx, userID.String(), nil,
		&notifications.QuietHoursUpdate{Enabled: false, StartLocal: "22:00", EndLocal: "07:00"}); err != nil {
		t.Fatal(err)
	}
}

func createIntent(t *testing.T, ctx context.Context, repo *notifications.Repository, userID uuid.UUID, eventType notifications.EventType, sourceEventID string, override bool) uuid.UUID {
	t.Helper()
	id, inserted, err := repo.Create(ctx, notifications.CreateInput{
		RecipientID:        userID,
		Type:               eventType,
		SourceEventID:      sourceEventID,
		SourceRef:          notifications.SourceRef{Type: notifications.SourceMessage, ID: uuid.New()},
		Title:              "New event",
		Body:               "Open the app to view it",
		PushExpiresAt:      time.Now().Add(24 * time.Hour),
		PreferenceOverride: override,
	})
	if err != nil || !inserted {
		t.Fatalf("create intent %s: inserted=%t err=%v", sourceEventID, inserted, err)
	}
	return id
}

// T011: per-device transitions, no-device hold, wake-up, invalid-token
// deletion through the F-001 auth seam, and provider-acceptance settle.
func TestDispatcherPerDeviceTransitionsAndInvalidTokenDeletion(t *testing.T) {
	pool := newDispatchIntegrationPool(t)
	ctx := context.Background()
	userID := createNotificationTestUser(t, ctx, pool)
	t.Cleanup(func() {
		if _, err := pool.Exec(context.Background(), "DELETE FROM users WHERE id = $1", userID); err != nil {
			t.Errorf("delete dispatcher integration user: %v", err)
		}
	})
	if _, err := pool.Exec(ctx, "INSERT INTO profiles (user_id, timezone) VALUES ($1, 'UTC')", userID); err != nil {
		t.Fatal(err)
	}
	repo := notifications.NewRepository(pool)
	disableQuietHours(t, ctx, repo, userID)
	authRepo := auth.NewSessionRepository(pool)
	metrics := &notifications.NotificationMetrics{}
	sender := &fakePushSender{}
	dispatcher := notifications.NewDispatcher(repo, sender, authRepo, metrics,
		func() time.Time { return time.Now() }, func(d time.Duration) time.Duration { return d })

	acceptedIntent := createIntent(t, ctx, repo, userID, notifications.EventGroupMessage, "ev-accepted", false)

	// No-device hold: intents stay pending_no_device until registration.
	if err := dispatcher.DispatchDue(ctx, 10); err != nil {
		t.Fatal(err)
	}
	if state := deliveryStateOf(t, ctx, pool, acceptedIntent); state != "pending_no_device" {
		t.Fatalf("state before registration=%q, want pending_no_device", state)
	}

	// Registration wakes eligible intents (T009 seam under the auth boundary).
	staleToken := "stale-" + uuid.NewString()
	liveToken := "live-" + uuid.NewString()
	if err := authRepo.UpsertDeviceToken(ctx, userID.String(), staleToken, "android", nil); err != nil {
		t.Fatal(err)
	}
	if err := authRepo.UpsertDeviceToken(ctx, userID.String(), liveToken, "web", nil); err != nil {
		t.Fatal(err)
	}
	if state := deliveryStateOf(t, ctx, pool, acceptedIntent); state != "pending_attempt" {
		t.Fatalf("state after registration=%q, want pending_attempt", state)
	}

	sender.hook = func(message notifications.PushMessage) error {
		if message.Token == staleToken {
			return fmt.Errorf("send: %w", notifications.ErrInvalidDeviceToken)
		}
		return nil
	}
	if err := dispatcher.DispatchDue(ctx, 10); err != nil {
		t.Fatal(err)
	}

	// Invalid token deleted through the auth seam; its attempt row cascades.
	var tokenCount int
	if err := pool.QueryRow(ctx, "SELECT COUNT(*) FROM device_tokens WHERE user_id = $1", userID).Scan(&tokenCount); err != nil {
		t.Fatal(err)
	}
	if tokenCount != 1 {
		t.Fatalf("device tokens=%d, want 1 after invalid-token deletion", tokenCount)
	}
	if state := deliveryStateOf(t, ctx, pool, acceptedIntent); state != "accepted_by_provider" {
		t.Fatalf("accepted intent state=%q, want accepted_by_provider", state)
	}
	var attemptState string
	var attemptCount int
	if err := pool.QueryRow(ctx, `
SELECT a.state, a.attempt_count
FROM notification_device_attempts a
JOIN device_tokens dt ON dt.id = a.device_token_id
WHERE a.notification_id = $1`, acceptedIntent).Scan(&attemptState, &attemptCount); err != nil {
		t.Fatal(err)
	}
	if attemptState != "accepted_by_provider" || attemptCount != 1 {
		t.Fatalf("attempt=(%q, %d), want accepted_by_provider with one attempt", attemptState, attemptCount)
	}

	// Phase 2: the held intent is created after the accepted one settled, is
	// woken by token re-registration, and loses its only device as
	// unregistered — it returns to pending_no_device to wait for the next
	// registration instead of being retried or parked.
	heldIntent := createIntent(t, ctx, repo, userID, notifications.EventGroupMessage, "ev-held", false)
	if err := authRepo.UpsertDeviceToken(ctx, userID.String(), liveToken, "web", nil); err != nil {
		t.Fatal(err)
	}
	if state := deliveryStateOf(t, ctx, pool, heldIntent); state != "pending_attempt" {
		t.Fatalf("held intent after re-registration=%q, want pending_attempt", state)
	}
	sender.hook = func(notifications.PushMessage) error {
		return fmt.Errorf("send: %w", notifications.ErrInvalidDeviceToken)
	}
	if err := dispatcher.DispatchDue(ctx, 10); err != nil {
		t.Fatal(err)
	}
	if state := deliveryStateOf(t, ctx, pool, heldIntent); state != "pending_no_device" {
		t.Fatalf("held intent state=%q, want pending_no_device", state)
	}
	if err := pool.QueryRow(ctx, "SELECT COUNT(*) FROM device_tokens WHERE user_id = $1", userID).Scan(&tokenCount); err != nil {
		t.Fatal(err)
	}
	if tokenCount != 0 {
		t.Fatalf("device tokens=%d, want 0 after the last device was invalidated", tokenCount)
	}
	summary := metrics.Summary().Outcomes
	if summary[notifications.DispatchAccepted] != 1 || summary[notifications.DispatchInvalidToken] != 2 {
		t.Fatalf("accepted=%d invalid=%d, want 1 and 2",
			summary[notifications.DispatchAccepted], summary[notifications.DispatchInvalidToken])
	}
	for _, message := range sender.sent {
		if message.Title == "" || message.Body == "" || message.SourceRef.ID == uuid.Nil {
			t.Fatalf("push message=%+v, want generic projection", message)
		}
	}
}

// T011: bounded five attempts with exponential backoff scheduling, safe error
// class, and terminal parking with metric.
func TestDispatcherBackoffSchedulingAndExhaustion(t *testing.T) {
	pool := newDispatchIntegrationPool(t)
	ctx := context.Background()
	userID := createNotificationTestUser(t, ctx, pool)
	t.Cleanup(func() {
		if _, err := pool.Exec(context.Background(), "DELETE FROM users WHERE id = $1", userID); err != nil {
			t.Errorf("delete backoff integration user: %v", err)
		}
	})
	if _, err := pool.Exec(ctx, "INSERT INTO profiles (user_id, timezone) VALUES ($1, 'UTC')", userID); err != nil {
		t.Fatal(err)
	}
	repo := notifications.NewRepository(pool)
	disableQuietHours(t, ctx, repo, userID)
	authRepo := auth.NewSessionRepository(pool)

	intentID := createIntent(t, ctx, repo, userID, notifications.EventGroupMessage, "ev-retry", false)
	if err := authRepo.UpsertDeviceToken(ctx, userID.String(), "retry-"+uuid.NewString(), "android", nil); err != nil {
		t.Fatal(err)
	}

	// Truncate to milliseconds so the value survives timestamptz microsecond
	// storage exactly for backoff comparisons.
	clock := time.Now().UTC().Truncate(time.Millisecond)
	metrics := &notifications.NotificationMetrics{}
	sender := &fakePushSender{err: errors.New("fcm unavailable")}
	dispatcher := notifications.NewDispatcher(repo, sender, authRepo, metrics,
		func() time.Time { return clock }, func(d time.Duration) time.Duration { return d })

	for attempt := 1; attempt <= notifications.MaxPushAttempts; attempt++ {
		if err := dispatcher.DispatchDue(ctx, 10); err != nil {
			t.Fatal(err)
		}
		var state, errorClass string
		var count int
		var nextAttemptAt time.Time
		err := pool.QueryRow(ctx, `
SELECT state, attempt_count, COALESCE(last_error_class, ''), next_attempt_at
FROM notification_device_attempts WHERE notification_id = $1`,
			intentID).Scan(&state, &count, &errorClass, &nextAttemptAt)
		if err != nil {
			t.Fatal(err)
		}
		if count != attempt {
			t.Fatalf("attempt %d: count=%d", attempt, count)
		}
		if attempt < notifications.MaxPushAttempts {
			if state != "pending" || errorClass != notifications.ErrorClassTransientProvider {
				t.Fatalf("attempt %d: (%q, %q), want pending transient", attempt, state, errorClass)
			}
			wantDelay := time.Duration(1) << (attempt - 1) * time.Second
			if !nextAttemptAt.Equal(clock.Add(wantDelay)) {
				t.Fatalf("attempt %d: next_attempt_at=%s, want %s", attempt, nextAttemptAt, clock.Add(wantDelay))
			}
			if state := deliveryStateOf(t, ctx, pool, intentID); state != "pending_attempt" {
				t.Fatalf("attempt %d: notification state=%q, want pending_attempt", attempt, state)
			}
		} else {
			if state != "exhausted" {
				t.Fatalf("final attempt state=%q, want exhausted (parked)", state)
			}
			if state := deliveryStateOf(t, ctx, pool, intentID); state != "exhausted" {
				t.Fatalf("notification state=%q, want exhausted", state)
			}
		}
		clock = clock.Add(30 * time.Second)
	}
	summary := metrics.Summary().Outcomes
	if summary[notifications.DispatchParked] != 1 || summary[notifications.DispatchRetried] != 4 {
		t.Fatalf("parked=%d retried=%d, want 1 and 4",
			summary[notifications.DispatchParked], summary[notifications.DispatchRetried])
	}
	if len(sender.sent) != notifications.MaxPushAttempts {
		t.Fatalf("sends=%d, want exactly %d", len(sender.sent), notifications.MaxPushAttempts)
	}
	// Parked intents are never reclaimed.
	if err := dispatcher.DispatchDue(ctx, 10); err != nil {
		t.Fatal(err)
	}
	if len(sender.sent) != notifications.MaxPushAttempts {
		t.Fatalf("sends after parking=%d, want no further attempts", len(sender.sent))
	}
}

// T012: preference gate suppression is terminal, history/unread is untouched,
// and re-enabling the preference never replays the occurrence.
func TestDispatcherPreferenceSuppressionIsTerminalAndNotReplayed(t *testing.T) {
	pool := newDispatchIntegrationPool(t)
	ctx := context.Background()
	userID := createNotificationTestUser(t, ctx, pool)
	t.Cleanup(func() {
		if _, err := pool.Exec(context.Background(), "DELETE FROM users WHERE id = $1", userID); err != nil {
			t.Errorf("delete suppression integration user: %v", err)
		}
	})
	if _, err := pool.Exec(ctx, "INSERT INTO profiles (user_id, timezone) VALUES ($1, 'Asia/Riyadh')", userID); err != nil {
		t.Fatal(err)
	}
	repo := notifications.NewRepository(pool)
	disableQuietHours(t, ctx, repo, userID)
	authRepo := auth.NewSessionRepository(pool)
	if err := repo.UpdatePreferences(ctx, userID.String(), []notifications.TypeSettingUpdate{
		{Type: notifications.EventGroupMessage, Enabled: false},
	}, nil); err != nil {
		t.Fatal(err)
	}

	intentID := createIntent(t, ctx, repo, userID, notifications.EventGroupMessage, "ev-muted", false)
	if err := authRepo.UpsertDeviceToken(ctx, userID.String(), "mute-"+uuid.NewString(), "ios", nil); err != nil {
		t.Fatal(err)
	}

	metrics := &notifications.NotificationMetrics{}
	sender := &fakePushSender{}
	dispatcher := notifications.NewDispatcher(repo, sender, authRepo, metrics,
		func() time.Time { return time.Now() }, func(d time.Duration) time.Duration { return d })
	if err := dispatcher.DispatchDue(ctx, 10); err != nil {
		t.Fatal(err)
	}
	if state := deliveryStateOf(t, ctx, pool, intentID); state != "suppressed_preference" {
		t.Fatalf("state=%q, want suppressed_preference", state)
	}
	if len(sender.sent) != 0 {
		t.Fatalf("sender called %d times, want 0", len(sender.sent))
	}
	var attemptState string
	if err := pool.QueryRow(ctx,
		"SELECT state FROM notification_device_attempts WHERE notification_id = $1", intentID).Scan(&attemptState); err != nil {
		t.Fatal(err)
	}
	if attemptState != "suppressed_preference" {
		t.Fatalf("attempt state=%q, want suppressed_preference", attemptState)
	}
	// History and unread are untouched by suppression.
	page, err := repo.List(ctx, userID.String(), nil, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Items) != 1 || page.UnreadCount != 1 {
		t.Fatalf("page=%+v, want the suppressed item retained as unread", page)
	}

	// Re-enabling the preference must not replay the suppressed occurrence.
	if err := repo.UpdatePreferences(ctx, userID.String(), []notifications.TypeSettingUpdate{
		{Type: notifications.EventGroupMessage, Enabled: true},
	}, nil); err != nil {
		t.Fatal(err)
	}
	if err := dispatcher.DispatchDue(ctx, 10); err != nil {
		t.Fatal(err)
	}
	if state := deliveryStateOf(t, ctx, pool, intentID); state != "suppressed_preference" {
		t.Fatalf("state after re-enable=%q, want terminal suppressed_preference", state)
	}
	if len(sender.sent) != 0 {
		t.Fatalf("sender called after re-enable, want no replay")
	}
}

// T012: quiet hours suppress push at first dispatch in the profile timezone,
// including the preference-override manager-deletion notice.
func TestDispatcherQuietHoursGateSuppressesIncludingOverride(t *testing.T) {
	pool := newDispatchIntegrationPool(t)
	ctx := context.Background()
	userID := createNotificationTestUser(t, ctx, pool)
	t.Cleanup(func() {
		if _, err := pool.Exec(context.Background(), "DELETE FROM users WHERE id = $1", userID); err != nil {
			t.Errorf("delete quiet-hours integration user: %v", err)
		}
	})
	if _, err := pool.Exec(ctx, "INSERT INTO profiles (user_id, timezone) VALUES ($1, 'UTC')", userID); err != nil {
		t.Fatal(err)
	}
	repo := notifications.NewRepository(pool)
	authRepo := auth.NewSessionRepository(pool)

	// Quiet hours covering the injected evaluation instant in profile time.
	now := time.Now().UTC()
	if err := repo.UpdatePreferences(ctx, userID.String(), nil, &notifications.QuietHoursUpdate{
		Enabled:    true,
		StartLocal: now.Add(-time.Hour).Format("15:04"),
		EndLocal:   now.Add(time.Hour).Format("15:04"),
	}); err != nil {
		t.Fatal(err)
	}

	ordinary := createIntent(t, ctx, repo, userID, notifications.EventGroupMessage, "ev-quiet", false)
	override := createIntent(t, ctx, repo, userID, notifications.EventCircleArchived, "ev-quiet-override", true)
	if err := authRepo.UpsertDeviceToken(ctx, userID.String(), "quiet-"+uuid.NewString(), "android", nil); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx,
		"UPDATE notifications SET next_attempt_at = NOW() - INTERVAL '5 minutes' WHERE user_id = $1", userID); err != nil {
		t.Fatal(err)
	}

	sender := &fakePushSender{}
	metrics := &notifications.NotificationMetrics{}
	dispatcher := notifications.NewDispatcher(repo, sender, authRepo, metrics,
		func() time.Time { return now }, func(d time.Duration) time.Duration { return d })
	if err := dispatcher.DispatchDue(ctx, 10); err != nil {
		t.Fatal(err)
	}
	for _, id := range []uuid.UUID{ordinary, override} {
		if state := deliveryStateOf(t, ctx, pool, id); state != "suppressed_quiet_hours" {
			t.Fatalf("intent %s state=%q, want suppressed_quiet_hours", id, state)
		}
	}
	if len(sender.sent) != 0 {
		t.Fatalf("sender called during quiet hours")
	}
	if got := metrics.Summary().Outcomes[notifications.DispatchSuppressedQuietHours]; got != 2 {
		t.Fatalf("quiet-hours metric=%d, want 2", got)
	}
}

// T011: pending intents past push_expires_at are swept to terminal expired.
func TestDispatcherSweepsExpiredIntents(t *testing.T) {
	pool := newDispatchIntegrationPool(t)
	ctx := context.Background()
	userID := createNotificationTestUser(t, ctx, pool)
	t.Cleanup(func() {
		if _, err := pool.Exec(context.Background(), "DELETE FROM users WHERE id = $1", userID); err != nil {
			t.Errorf("delete expiry integration user: %v", err)
		}
	})
	var intentID uuid.UUID
	if err := pool.QueryRow(ctx, `
INSERT INTO notifications (
    user_id, type, source_event_id, source_ref_type, source_ref_id,
    title, body, push_expires_at, delivery_state
) VALUES ($1, 'group_message', 'ev-expired', 'message', $2, 'Old event', 'Open the app to view it',
          NOW() - INTERVAL '1 hour', 'pending_no_device')
RETURNING id`, userID, uuid.New()).Scan(&intentID); err != nil {
		t.Fatal(err)
	}

	repo := notifications.NewRepository(pool)
	metrics := &notifications.NotificationMetrics{}
	dispatcher := notifications.NewDispatcher(repo, &fakePushSender{}, auth.NewSessionRepository(pool), metrics,
		func() time.Time { return time.Now() }, func(d time.Duration) time.Duration { return d })
	if err := dispatcher.DispatchDue(ctx, 10); err != nil {
		t.Fatal(err)
	}
	if state := deliveryStateOf(t, ctx, pool, intentID); state != "expired" {
		t.Fatalf("state=%q, want expired", state)
	}
	if got := metrics.Summary().Outcomes[notifications.DispatchExpired]; got != 1 {
		t.Fatalf("expired metric=%d, want 1", got)
	}
}

// T011 lease: a claimed intent is not re-claimable until the lease expires,
// which prevents concurrent re-claim across dispatcher instances while the
// lease holds and recovers rows after a dispatcher crash.
func TestClaimLeasePreventsReclaimUntilExpiry(t *testing.T) {
	pool := newDispatchIntegrationPool(t)
	ctx := context.Background()
	userID := createNotificationTestUser(t, ctx, pool)
	t.Cleanup(func() {
		if _, err := pool.Exec(context.Background(), "DELETE FROM users WHERE id = $1", userID); err != nil {
			t.Errorf("delete lease integration user: %v", err)
		}
	})
	repo := notifications.NewRepository(pool)
	authRepo := auth.NewSessionRepository(pool)
	intentID := createIntent(t, ctx, repo, userID, notifications.EventGroupMessage, "ev-lease", false)
	if err := authRepo.UpsertDeviceToken(ctx, userID.String(), "lease-"+uuid.NewString(), "web", nil); err != nil {
		t.Fatal(err)
	}

	now := time.Now().UTC().Add(time.Minute)
	intents, err := repo.ClaimDueIntents(ctx, now, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(intents) != 1 || intents[0].ID != intentID {
		t.Fatalf("first claim=%v, want the due intent", intents)
	}
	// The claim leased the row by pushing next_attempt_at to the lease deadline.
	var leasedUntil time.Time
	if err := pool.QueryRow(ctx, "SELECT next_attempt_at FROM notifications WHERE id = $1", intentID).Scan(&leasedUntil); err != nil {
		t.Fatal(err)
	}
	if delta := leasedUntil.Sub(now.Add(60 * time.Second)); delta < -time.Second || delta > time.Second {
		t.Fatalf("leased next_attempt_at=%s, want ~%s", leasedUntil, now.Add(60*time.Second))
	}
	// A concurrent dispatcher cycle cannot re-claim the leased row.
	intents, err = repo.ClaimDueIntents(ctx, now, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(intents) != 0 {
		t.Fatalf("re-claim before lease expiry=%v, want none", intents)
	}
	// After the lease expires the row is claimable again (crash recovery).
	intents, err = repo.ClaimDueIntents(ctx, now.Add(61*time.Second), 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(intents) != 1 || intents[0].ID != intentID {
		t.Fatalf("claim after lease expiry=%v, want the intent reclaimed", intents)
	}
}

// T011 expiry: sweeping a pending_attempt intent whose device attempt rows are
// still pending marks the intent terminally expired. The attempt rows remain
// in their pending state but are orphaned from dispatch: the terminal intent
// is never claimed again, so no sends or state changes occur for them. The
// F-008 retention cleanup (T013) removes terminal attempt state.
func TestExpiredSweepWithLivePendingAttemptRows(t *testing.T) {
	pool := newDispatchIntegrationPool(t)
	ctx := context.Background()
	userID := createNotificationTestUser(t, ctx, pool)
	t.Cleanup(func() {
		if _, err := pool.Exec(context.Background(), "DELETE FROM users WHERE id = $1", userID); err != nil {
			t.Errorf("delete expiry-attempts integration user: %v", err)
		}
	})
	if _, err := pool.Exec(ctx, "INSERT INTO profiles (user_id, timezone) VALUES ($1, 'UTC')", userID); err != nil {
		t.Fatal(err)
	}
	repo := notifications.NewRepository(pool)
	disableQuietHours(t, ctx, repo, userID)
	authRepo := auth.NewSessionRepository(pool)
	intentID := createIntent(t, ctx, repo, userID, notifications.EventGroupMessage, "ev-expiry-attempts", false)
	if err := authRepo.UpsertDeviceToken(ctx, userID.String(), "expiry-"+uuid.NewString(), "android", nil); err != nil {
		t.Fatal(err)
	}

	// One failed dispatch leaves a live pending attempt row.
	sender := &fakePushSender{err: errors.New("fcm unavailable")}
	dispatcher := notifications.NewDispatcher(repo, sender, authRepo, &notifications.NotificationMetrics{},
		func() time.Time { return time.Now() }, func(d time.Duration) time.Duration { return d })
	if err := dispatcher.DispatchDue(ctx, 10); err != nil {
		t.Fatal(err)
	}
	if state := deliveryStateOf(t, ctx, pool, intentID); state != "pending_attempt" {
		t.Fatalf("state after failed send=%q, want pending_attempt", state)
	}

	// The push deadline passes before the retry comes due.
	if _, err := pool.Exec(ctx, `
UPDATE notifications
SET push_expires_at = NOW() - INTERVAL '1 minute',
    next_attempt_at = NOW() - INTERVAL '1 minute'
WHERE id = $1`, intentID); err != nil {
		t.Fatal(err)
	}
	if err := dispatcher.DispatchDue(ctx, 10); err != nil {
		t.Fatal(err)
	}
	if state := deliveryStateOf(t, ctx, pool, intentID); state != "expired" {
		t.Fatalf("state after expiry=%q, want expired", state)
	}
	// The pending attempt row is retained but orphaned: no further sends occur
	// because the terminal intent is never claimed again.
	var attemptState string
	if err := pool.QueryRow(ctx,
		"SELECT state FROM notification_device_attempts WHERE notification_id = $1", intentID).Scan(&attemptState); err != nil {
		t.Fatal(err)
	}
	if attemptState != "pending" {
		t.Fatalf("attempt row state=%q, want retained pending row (T013 cleanup scope)", attemptState)
	}
	if err := dispatcher.DispatchDue(ctx, 10); err != nil {
		t.Fatal(err)
	}
	if len(sender.sent) != 1 {
		t.Fatalf("sends=%d, want no sends after expiry", len(sender.sent))
	}
	if state := deliveryStateOf(t, ctx, pool, intentID); state != "expired" {
		t.Fatalf("state after final cycle=%q, want terminal expired", state)
	}
}

// T015: the dispatch-time quiet-hours gate resolves the CURRENT profile IANA
// timezone — a non-UTC zone (America/New_York across the fall-back boundary,
// where the UTC clock would NOT be inside the window) suppresses, and after
// the user changes timezone the next dispatch check uses the new zone.
func TestDispatcherQuietHoursFollowsCurrentProfileTimezone(t *testing.T) {
	pool := newDispatchIntegrationPool(t)
	ctx := context.Background()
	userID := createNotificationTestUser(t, ctx, pool)
	t.Cleanup(func() {
		if _, err := pool.Exec(context.Background(), "DELETE FROM users WHERE id = $1", userID); err != nil {
			t.Errorf("delete timezone integration user: %v", err)
		}
	})
	if _, err := pool.Exec(ctx, "INSERT INTO profiles (user_id, timezone) VALUES ($1, 'America/New_York')", userID); err != nil {
		t.Fatal(err)
	}
	repo := notifications.NewRepository(pool)
	authRepo := auth.NewSessionRepository(pool)
	if err := repo.UpdatePreferences(ctx, userID.String(), nil,
		&notifications.QuietHoursUpdate{Enabled: true, StartLocal: "22:00", EndLocal: "07:00"}); err != nil {
		t.Fatal(err)
	}

	// 2026-11-01T11:30:00Z is 06:30 EST (inside 22:00–07:00) but 11:30 UTC
	// (outside): suppression here proves the gate is not evaluating UTC.
	now := time.Date(2026, time.November, 1, 11, 30, 0, 0, time.UTC)
	newYorkIntent := createIntent(t, ctx, repo, userID, notifications.EventGroupMessage, "ev-tz-ny", false)
	// Registration wakes the held intent to pending_attempt (T009 seam).
	token := "tz-" + uuid.NewString()
	if err := authRepo.UpsertDeviceToken(ctx, userID.String(), token, "android", nil); err != nil {
		t.Fatal(err)
	}
	// Align scheduling columns with the injected dispatcher clock (which is
	// ahead of wall time), or the expiry sweep would terminal-sweep the rows.
	if _, err := pool.Exec(ctx, `
UPDATE notifications
SET next_attempt_at = $2::timestamptz - INTERVAL '5 minutes',
    push_expires_at = $2::timestamptz + INTERVAL '24 hours'
WHERE user_id = $1`, userID, now); err != nil {
		t.Fatal(err)
	}

	sender := &fakePushSender{}
	metrics := &notifications.NotificationMetrics{}
	dispatcher := notifications.NewDispatcher(repo, sender, authRepo, metrics,
		func() time.Time { return now }, func(d time.Duration) time.Duration { return d })
	if err := dispatcher.DispatchDue(ctx, 10); err != nil {
		t.Fatal(err)
	}
	if state := deliveryStateOf(t, ctx, pool, newYorkIntent); state != "suppressed_quiet_hours" {
		t.Fatalf("New York intent state=%q, want suppressed_quiet_hours", state)
	}
	if len(sender.sent) != 0 {
		t.Fatalf("sender called during New York quiet hours")
	}

	// The user moves: the very next dispatch check evaluates the new zone.
	// The same instant is 14:30 in Riyadh — outside the window — so the new
	// occurrence is sent instead of suppressed.
	if _, err := pool.Exec(ctx, "UPDATE profiles SET timezone = 'Asia/Riyadh' WHERE user_id = $1", userID); err != nil {
		t.Fatal(err)
	}
	riyadhIntent := createIntent(t, ctx, repo, userID, notifications.EventGroupMessage, "ev-tz-riyadh", false)
	// Refreshing the same token wakes the new held intent before the next cycle.
	if err := authRepo.UpsertDeviceToken(ctx, userID.String(), token, "android", nil); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx,
		"UPDATE notifications SET push_expires_at = $2::timestamptz + INTERVAL '24 hours' WHERE id = $1",
		riyadhIntent, now); err != nil {
		t.Fatal(err)
	}
	if err := dispatcher.DispatchDue(ctx, 10); err != nil {
		t.Fatal(err)
	}
	if state := deliveryStateOf(t, ctx, pool, riyadhIntent); state != "accepted_by_provider" {
		t.Fatalf("Riyadh intent state=%q, want accepted_by_provider after timezone change", state)
	}
	if len(sender.sent) != 1 {
		t.Fatalf("sends=%d, want exactly one send after the timezone change", len(sender.sent))
	}
}
