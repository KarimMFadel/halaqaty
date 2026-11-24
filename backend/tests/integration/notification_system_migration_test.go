//go:build integration

package integration

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	migration020Up   = "000020_schedule_calendar_attendance.up.sql"
	migration021Up   = "000021_notification_system.up.sql"
	migration021Down = "000021_notification_system.down.sql"
)

// notificationHeadMigrations is every migration up to the pre-F-008 head.
var notificationHeadMigrations = append(recitationQueueHeadMigrations,
	migration018Up, migration019Up, migration020Up)

// notificationSystemTables are all tables created by migration 000021.
var notificationSystemTables = []string{
	"device_tokens",
	"notifications",
	"notification_device_attempts",
	"notification_preferences",
}

// T015: migration 000021 up creates the four F-008 tables with the
// load-bearing constraints, indexes, and cascade rules the dispatcher and
// retention cleanup depend on (ADR-027).
func TestNotificationSystemMigration_FreshSchemaAndConstraints(t *testing.T) {
	ctx := context.Background()
	pool := openPool(t, ctx)
	defer pool.Close()
	conn := acquireConn(t, pool, ctx)
	defer conn.Release()

	schema := uniqueSchemaName(t)
	createSchema(t, conn, ctx, schema)
	defer dropSchema(t, pool, ctx, schema)

	for _, migration := range append(notificationHeadMigrations, migration021Up) {
		runMigrationFile(t, conn, ctx, migration)
	}

	assertNotificationSystemTables(t, conn, ctx, true)
	for _, index := range []string{
		"idx_notifications_user_history",
		"idx_notifications_user_unread",
		"idx_notifications_dispatch_due",
		"idx_notifications_retention",
		"idx_notification_device_attempts_due",
		"uq_notification_preferences_user_type",
		"uq_notification_preferences_user_quiet_hours",
	} {
		assertIndexExists(t, conn, ctx, schema, index)
	}
	assertConstraintExists(t, conn, ctx, schema, "notifications", "uq_notifications_user_type_source")
	assertConstraintExists(t, conn, ctx, schema, "notifications", "ck_notifications_delivery_state")
	assertConstraintExists(t, conn, ctx, schema, "notifications", "ck_notifications_attempt_count")
	assertConstraintExists(t, conn, ctx, schema, "device_tokens", "uq_device_tokens_user_token")

	userID := seedLiveSessionUser(t, conn, ctx, "notification-schema")
	var tokenID string
	if err := conn.QueryRow(ctx, `
		INSERT INTO device_tokens (user_id, token, platform)
		VALUES ($1::uuid, 'token-1', 'android')
		RETURNING id::text
	`, userID).Scan(&tokenID); err != nil {
		t.Fatalf("insert device token: %v", err)
	}
	var notificationID string
	if err := conn.QueryRow(ctx, `
		INSERT INTO notifications (
			user_id, type, source_event_id, source_ref_type, source_ref_id,
			title, body, push_expires_at
		)
		VALUES ($1::uuid, 'group_message', 'event-1', 'message', gen_random_uuid(),
		        'New message', 'Open the app to view it', NOW() + INTERVAL '24 hours')
		RETURNING id::text, delivery_state
	`, userID).Scan(&notificationID, new(string)); err != nil {
		t.Fatalf("insert notification: %v", err)
	}
	if _, err := conn.Exec(ctx, `
		INSERT INTO notification_device_attempts (notification_id, device_token_id)
		VALUES ($1::uuid, $2::uuid)
	`, notificationID, tokenID); err != nil {
		t.Fatalf("insert device attempt: %v", err)
	}
	if _, err := conn.Exec(ctx, `
		INSERT INTO notification_preferences (user_id, notification_type, enabled)
		VALUES ($1::uuid, 'group_message', FALSE)
	`, userID); err != nil {
		t.Fatalf("insert type preference: %v", err)
	}

	expectViolations(t, conn, ctx, []sqlViolationCase{
		{"duplicate recipient/type/source idempotency key", `
			INSERT INTO notifications (user_id, type, source_event_id, source_ref_type, source_ref_id, title, body, push_expires_at)
			VALUES ($1::uuid, 'group_message', 'event-1', 'message', gen_random_uuid(), 'New message', 'Open the app to view it', NOW())`, "23505", []any{userID}},
		{"unknown delivery state", `
			INSERT INTO notifications (user_id, type, source_event_id, source_ref_type, source_ref_id, title, body, push_expires_at, delivery_state)
			VALUES ($1::uuid, 'group_message', 'event-2', 'message', gen_random_uuid(), 'New message', 'Open the app to view it', NOW(), 'delivered')`, "23514", []any{userID}},
		{"attempt count above retry ceiling", `
			INSERT INTO notifications (user_id, type, source_event_id, source_ref_type, source_ref_id, title, body, push_expires_at, attempt_count)
			VALUES ($1::uuid, 'group_message', 'event-3', 'message', gen_random_uuid(), 'New message', 'Open the app to view it', NOW(), 6)`, "23514", []any{userID}},
		{"read without read_at", `
			INSERT INTO notifications (user_id, type, source_event_id, source_ref_type, source_ref_id, title, body, push_expires_at, is_read)
			VALUES ($1::uuid, 'group_message', 'event-4', 'message', gen_random_uuid(), 'New message', 'Open the app to view it', NOW(), TRUE)`, "23514", []any{userID}},
		{"unknown event type", `
			INSERT INTO notifications (user_id, type, source_event_id, source_ref_type, source_ref_id, title, body, push_expires_at)
			VALUES ($1::uuid, 'marketing_blast', 'event-5', 'message', gen_random_uuid(), 'New message', 'Open the app to view it', NOW())`, "23514", []any{userID}},
		{"unknown source reference type", `
			INSERT INTO notifications (user_id, type, source_event_id, source_ref_type, source_ref_id, title, body, push_expires_at)
			VALUES ($1::uuid, 'group_message', 'event-6', 'notification', gen_random_uuid(), 'New message', 'Open the app to view it', NOW())`, "23514", []any{userID}},
		{"duplicate attempt for notification and device", `
			INSERT INTO notification_device_attempts (notification_id, device_token_id)
			VALUES ($1::uuid, $2::uuid)`, "23505", []any{notificationID, tokenID}},
		{"unknown attempt state", `
			INSERT INTO notification_device_attempts (notification_id, device_token_id, state)
			VALUES ($1::uuid, $2::uuid, 'delivered')`, "23514", []any{notificationID, "99999999-9999-9999-9999-999999999999"}},
		{"duplicate token for one user", `
			INSERT INTO device_tokens (user_id, token, platform)
			VALUES ($1::uuid, 'token-1', 'ios')`, "23505", []any{userID}},
		{"unknown token platform", `
			INSERT INTO device_tokens (user_id, token, platform)
			VALUES ($1::uuid, 'token-2', 'desktop')`, "23514", []any{userID}},
		{"type preference row carrying quiet hours", `
			INSERT INTO notification_preferences (user_id, notification_type, enabled, quiet_hours_start, quiet_hours_end)
			VALUES ($1::uuid, 'session_reminder', TRUE, '22:00', '07:00')`, "23514", []any{userID}},
		{"quiet hours with equal endpoints", `
			INSERT INTO notification_preferences (user_id, notification_type, enabled, quiet_hours_start, quiet_hours_end)
			VALUES ($1::uuid, NULL, TRUE, '22:00', '22:00')`, "23514", []any{"99999999-9999-9999-9999-999999999999"}},
	})

	// The retention cleanup relies on the attempt cascade from notifications.
	if _, err := conn.Exec(ctx, `DELETE FROM notifications WHERE id = $1::uuid`, notificationID); err != nil {
		t.Fatalf("delete notification: %v", err)
	}
	var attemptCount int
	if err := conn.QueryRow(ctx,
		`SELECT COUNT(*) FROM notification_device_attempts WHERE notification_id = $1::uuid`,
		notificationID).Scan(&attemptCount); err != nil {
		t.Fatalf("count attempts after cascade: %v", err)
	}
	if attemptCount != 0 {
		t.Fatalf("attempts after notification delete=%d, want cascaded removal", attemptCount)
	}
	// Account removal cascades tokens, history, and preferences.
	if _, err := conn.Exec(ctx, `DELETE FROM users WHERE id = $1::uuid`, userID); err != nil {
		t.Fatalf("delete user: %v", err)
	}
	var orphanCount int
	if err := conn.QueryRow(ctx, `
		SELECT (SELECT COUNT(*) FROM device_tokens WHERE user_id = $1::uuid)
		     + (SELECT COUNT(*) FROM notifications WHERE user_id = $1::uuid)
		     + (SELECT COUNT(*) FROM notification_preferences WHERE user_id = $1::uuid)
	`, userID).Scan(&orphanCount); err != nil {
		t.Fatalf("count rows after user cascade: %v", err)
	}
	if orphanCount != 0 {
		t.Fatalf("rows after user delete=%d, want cascaded removal", orphanCount)
	}
}

// T015: down migration rolls the schema back cleanly (tables, trigger, and
// function gone; legacy rows intact) and re-applying up restores the objects.
func TestNotificationSystemMigration_DownUpCycleRestoresSchema(t *testing.T) {
	ctx := context.Background()
	pool := openPool(t, ctx)
	defer pool.Close()
	conn := acquireConn(t, pool, ctx)
	defer conn.Release()

	schema := uniqueSchemaName(t)
	createSchema(t, conn, ctx, schema)
	defer dropSchema(t, pool, ctx, schema)

	for _, migration := range notificationHeadMigrations {
		runMigrationFile(t, conn, ctx, migration)
	}
	userID := seedLiveSessionUser(t, conn, ctx, "down-up")
	circleID := seedLiveSessionCircle(t, conn, ctx, userID)
	sessionID := insertLiveSession(t, conn, ctx, circleID, userID, "")

	runMigrationFile(t, conn, ctx, migration021Up)
	assertNotificationSystemTables(t, conn, ctx, true)
	assertNotificationClosureTrigger(t, conn, ctx, true)

	runMigrationFile(t, conn, ctx, migration021Down)
	assertNotificationSystemTables(t, conn, ctx, false)
	assertNotificationClosureTrigger(t, conn, ctx, false)
	assertLegacyRowsIntact(t, conn, ctx, userID, circleID, sessionID)

	runMigrationFile(t, conn, ctx, migration021Up)
	assertNotificationSystemTables(t, conn, ctx, true)
	assertNotificationClosureTrigger(t, conn, ctx, true)
	assertLegacyRowsIntact(t, conn, ctx, userID, circleID, sessionID)
}

// T015: the down migration refuses to destroy live notification data — the
// safeguard raises before any table is dropped.
func TestNotificationSystemMigration_DownWithLiveDataIsRefused(t *testing.T) {
	ctx := context.Background()
	pool := openPool(t, ctx)
	defer pool.Close()
	conn := acquireConn(t, pool, ctx)
	defer conn.Release()

	schema := uniqueSchemaName(t)
	createSchema(t, conn, ctx, schema)
	defer dropSchema(t, pool, ctx, schema)

	for _, migration := range append(notificationHeadMigrations, migration021Up) {
		runMigrationFile(t, conn, ctx, migration)
	}
	userID := seedLiveSessionUser(t, conn, ctx, "notification-down-guard")
	if _, err := conn.Exec(ctx, `
		INSERT INTO notifications (user_id, type, source_event_id, source_ref_type, source_ref_id, title, body, push_expires_at)
		VALUES ($1::uuid, 'circle_archived', 'guard-1', 'circle', gen_random_uuid(), 'Circle update', 'Open the app to view it', NOW())
	`, userID); err != nil {
		t.Fatalf("insert live notification: %v", err)
	}

	err := execMigrationFile(conn, ctx, migration021Down)
	// PL/pgSQL RAISE EXCEPTION surfaces as P0001.
	if !isPgErrorCode(t, err, "P0001") {
		t.Fatalf("down with live data: got %v, want pg error P0001 (safeguard)", err)
	}
	assertNotificationSystemTables(t, conn, ctx, true)
}

func assertNotificationSystemTables(t *testing.T, conn *pgxpool.Conn, ctx context.Context, wantExist bool) {
	t.Helper()
	for _, table := range notificationSystemTables {
		var reg *string
		if err := conn.QueryRow(ctx, `SELECT to_regclass($1)`, table).Scan(&reg); err != nil {
			t.Fatalf("lookup table %s: %v", table, err)
		}
		if (reg != nil) != wantExist {
			t.Fatalf("table %s existence: got %v, want %v", table, reg != nil, wantExist)
		}
	}
}

// assertNotificationClosureTrigger checks the account-closure token-cleanup
// trigger and its function are present (or fully removed after rollback).
func assertNotificationClosureTrigger(t *testing.T, conn *pgxpool.Conn, ctx context.Context, wantExist bool) {
	t.Helper()
	var triggerCount, functionCount int
	if err := conn.QueryRow(ctx, `
		SELECT COUNT(*)
		FROM pg_trigger t
		JOIN pg_class r ON r.oid = t.tgrelid
		JOIN pg_namespace n ON n.oid = r.relnamespace
		WHERE n.nspname = current_schema()
		  AND r.relname = 'users'
		  AND t.tgname = 'trg_remove_closed_account_device_tokens'
		  AND NOT t.tgisinternal
	`).Scan(&triggerCount); err != nil {
		t.Fatalf("lookup closure trigger: %v", err)
	}
	if err := conn.QueryRow(ctx, `
		SELECT COUNT(*)
		FROM pg_proc p
		JOIN pg_namespace n ON n.oid = p.pronamespace
		WHERE n.nspname = current_schema()
		  AND p.proname = 'remove_closed_account_device_tokens'
	`).Scan(&functionCount); err != nil {
		t.Fatalf("lookup closure trigger function: %v", err)
	}
	if (triggerCount == 1) != wantExist || (functionCount == 1) != wantExist {
		t.Fatalf("closure trigger=%d function=%d, want existence %v", triggerCount, functionCount, wantExist)
	}
}

// execMigrationFile runs one migration file and reports the error instead of
// failing, for tests that expect a migration to be refused.
func execMigrationFile(conn *pgxpool.Conn, ctx context.Context, filename string) error {
	_, currentFile, _, ok := runtime.Caller(0)
	if !ok {
		return os.ErrNotExist
	}
	path := filepath.Join(filepath.Dir(currentFile), "..", "..", "migrations", filename)
	sql, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	_, err = conn.Exec(ctx, string(sql))
	return err
}
