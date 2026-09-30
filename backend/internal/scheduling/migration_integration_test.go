//go:build integration

// Package scheduling holds F-006 schedule, calendar and attendance code. It
// currently carries only the paired-migration integration coverage for
// 000020_schedule_calendar_attendance (ADR-026); feature handlers arrive in
// later Spec-Kit tasks.
package scheduling

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// migrationHarness mirrors the disposable-schema harness used by
// backend/tests/integration (helpers there are package-private, so this file
// keeps local copies with identical semantics).
var preFeatureMigrations = []string{
	"000010_auth_roles_profile.up.sql",
	"000011_auth_roles_profile_alignment.up.sql",
	"000012_auth_profiles_display_name.up.sql",
	"000013_create_circles.up.sql",
	"000014_circle_members_circle_fk.up.sql",
	"000015_circle_management.up.sql",
	"000016_live_sessions.up.sql",
	"000017_recitation_queue_system.up.sql",
	"000018_real_time_chat.up.sql",
	"000019_account_deletion_tombstone.up.sql",
}

const (
	migration020Up   = "000020_schedule_calendar_attendance.up.sql"
	migration020Down = "000020_schedule_calendar_attendance.down.sql"
)

var scheduleTables = []string{
	"schedules",
	"schedule_revisions",
	"schedule_selected_dates",
	"schedule_occurrence_exceptions",
	"planned_session_details",
	"session_attendance",
	"attendance_corrections",
	"schedule_request_replays",
}

func TestScheduleCalendarMigration_FreshUpDownAndReUp(t *testing.T) {
	ctx := context.Background()
	pool := openPool(t, ctx)
	defer pool.Close()
	conn := acquireConn(t, pool, ctx)
	defer conn.Release()

	schema := uniqueSchemaName(t)
	createSchema(t, conn, ctx, schema)
	defer dropSchema(t, pool, ctx, schema)

	for _, migration := range append(preFeatureMigrations, migration020Up) {
		runMigrationFile(t, conn, ctx, migration)
	}
	assertScheduleTablesExist(t, conn, ctx, true)
	assertColumnExists(t, conn, ctx, "profiles", "timezone", true)

	runMigrationFile(t, conn, ctx, migration020Down)
	assertScheduleTablesExist(t, conn, ctx, false)
	assertColumnExists(t, conn, ctx, "profiles", "timezone", false)

	// Re-up after down must restore the full additive surface.
	runMigrationFile(t, conn, ctx, migration020Up)
	assertScheduleTablesExist(t, conn, ctx, true)
	assertColumnExists(t, conn, ctx, "profiles", "timezone", true)
}

func TestScheduleCalendarMigration_UpgradeBackfillsProfileAndKeepsHistory(t *testing.T) {
	ctx := context.Background()
	pool := openPool(t, ctx)
	defer pool.Close()
	conn := acquireConn(t, pool, ctx)
	defer conn.Release()

	schema := uniqueSchemaName(t)
	createSchema(t, conn, ctx, schema)
	defer dropSchema(t, pool, ctx, schema)

	for _, migration := range preFeatureMigrations {
		runMigrationFile(t, conn, ctx, migration)
	}

	userID := "11111111-1111-1111-1111-111111111111"
	circleID := "22222222-2222-2222-2222-222222222222"
	sessionID := "33333333-3333-3333-3333-333333333333"
	if _, err := conn.Exec(ctx, `INSERT INTO users (id, firebase_uid, email) VALUES ($1::uuid, 'firebase-f006', 'f006@example.com')`, userID); err != nil {
		t.Fatalf("seed user: %v", err)
	}
	if _, err := conn.Exec(ctx, `INSERT INTO profiles (user_id, display_name, full_name) VALUES ($1::uuid, 'Teacher', 'Legacy Teacher')`, userID); err != nil {
		t.Fatalf("seed pre-006 profile: %v", err)
	}
	if _, err := conn.Exec(ctx, `INSERT INTO circles (id, name, teacher_id, invite_code) VALUES ($1::uuid, 'Legacy', $2::uuid, 'HLQ-F006AA')`, circleID, userID); err != nil {
		t.Fatalf("seed circle: %v", err)
	}
	if _, err := conn.Exec(ctx, `INSERT INTO sessions (id, circle_id, created_by) VALUES ($1::uuid, $2::uuid, $3::uuid)`, sessionID, circleID, userID); err != nil {
		t.Fatalf("seed pre-006 session: %v", err)
	}

	runMigrationFile(t, conn, ctx, migration020Up)

	var timezone string
	if err := conn.QueryRow(ctx, `SELECT timezone FROM profiles WHERE user_id = $1::uuid`, userID).Scan(&timezone); err != nil {
		t.Fatalf("read migrated profile timezone: %v", err)
	}
	if timezone != "UTC" {
		t.Fatalf("pre-existing profile must backfill to UTC, got %q", timezone)
	}

	// Historical-data safety: pre-006 rows keep their content untouched.
	var displayName string
	if err := conn.QueryRow(ctx, `SELECT display_name FROM profiles WHERE user_id = $1::uuid`, userID).Scan(&displayName); err != nil {
		t.Fatalf("read preserved profile: %v", err)
	}
	if displayName != "Teacher" {
		t.Fatalf("migration altered existing profile content: %q", displayName)
	}
	var status string
	var scheduledAt *time.Time
	if err := conn.QueryRow(ctx, `SELECT status, scheduled_at FROM sessions WHERE id = $1::uuid`, sessionID).Scan(&status, &scheduledAt); err != nil {
		t.Fatalf("read preserved session: %v", err)
	}
	if status != "scheduled" || scheduledAt != nil {
		t.Fatalf("migration altered pre-006 session: status=%q scheduled_at=%v", status, scheduledAt)
	}
}

func TestScheduleCalendarMigration_EnforcesIdentitiesAndChecks(t *testing.T) {
	ctx := context.Background()
	pool := openPool(t, ctx)
	defer pool.Close()
	conn := acquireConn(t, pool, ctx)
	defer conn.Release()

	schema := uniqueSchemaName(t)
	createSchema(t, conn, ctx, schema)
	defer dropSchema(t, pool, ctx, schema)

	for _, migration := range append(preFeatureMigrations, migration020Up) {
		runMigrationFile(t, conn, ctx, migration)
	}

	userID := "11111111-1111-1111-1111-111111111111"
	circleID := "22222222-2222-2222-2222-222222222222"
	scheduleID := "44444444-4444-4444-4444-444444444444"
	sessionID := "55555555-5555-5555-5555-555555555555"
	if _, err := conn.Exec(ctx, `INSERT INTO users (id, firebase_uid, email) VALUES ($1::uuid, 'firebase-f006', 'f006@example.com')`, userID); err != nil {
		t.Fatalf("seed user: %v", err)
	}
	if _, err := conn.Exec(ctx, `INSERT INTO circles (id, name, teacher_id, invite_code) VALUES ($1::uuid, 'Checks', $2::uuid, 'HLQ-F006BB')`, circleID, userID); err != nil {
		t.Fatalf("seed circle: %v", err)
	}
	if _, err := conn.Exec(ctx, `INSERT INTO schedules (id, circle_id, created_by, current_version) VALUES ($1::uuid, $2::uuid, $3::uuid, 1)`, scheduleID, circleID, userID); err != nil {
		t.Fatalf("seed schedule: %v", err)
	}
	if _, err := conn.Exec(ctx, `INSERT INTO sessions (id, circle_id, created_by) VALUES ($1::uuid, $2::uuid, $3::uuid)`, sessionID, circleID, userID); err != nil {
		t.Fatalf("seed session: %v", err)
	}

	validRevision := `
		INSERT INTO schedule_revisions (
			schedule_id, version, effective_local_date, mode, anchor_local_date,
			start_local_time, end_local_time, duration_minutes, timezone,
			interval_count, interval_unit
		) VALUES ($1::uuid, 1, DATE '2026-09-27', 'interval', DATE '2026-09-27',
			TIME '10:00', TIME '11:00', 60, 'UTC', 1, 'day')`
	if _, err := conn.Exec(ctx, validRevision, scheduleID); err != nil {
		t.Fatalf("seed valid interval revision: %v", err)
	}

	// Stable occurrence identity: duplicate (schedule_id, original_local_date).
	insertException := `
		INSERT INTO schedule_occurrence_exceptions (schedule_id, original_local_date, series_version, version, cancelled_at, updated_by)
		VALUES ($1::uuid, DATE '2026-10-01', 1, 1, NOW(), $2::uuid)`
	if _, err := conn.Exec(ctx, insertException, scheduleID, userID); err != nil {
		t.Fatalf("seed exception: %v", err)
	}
	assertPgErrorCode(t, execErr(ctx, conn, insertException, scheduleID, userID), "23505", "duplicate occurrence identity")

	// Attendance identity: duplicate (session_id, user_id).
	insertAttendance := `
		INSERT INTO session_attendance (session_id, user_id, roster_source)
		VALUES ($1::uuid, $2::uuid, 'start_snapshot')`
	if _, err := conn.Exec(ctx, insertAttendance, sessionID, userID); err != nil {
		t.Fatalf("seed attendance: %v", err)
	}
	assertPgErrorCode(t, execErr(ctx, conn, insertAttendance, sessionID, userID), "23505", "duplicate attendance identity")

	// Invalid mode is rejected.
	badMode := `
		INSERT INTO schedule_revisions (
			schedule_id, version, effective_local_date, mode, anchor_local_date,
			start_local_time, end_local_time, duration_minutes, timezone
		) VALUES ($1::uuid, 2, DATE '2026-09-27', 'bogus', DATE '2026-09-27',
			TIME '10:00', TIME '11:00', 60, 'UTC')`
	assertPgErrorCode(t, execErr(ctx, conn, badMode, scheduleID), "23514", "invalid mode")

	// Duration outside 1..44640 is rejected.
	badDuration := `
		INSERT INTO schedule_revisions (
			schedule_id, version, effective_local_date, mode, anchor_local_date,
			start_local_time, end_local_time, duration_minutes, timezone,
			interval_count, interval_unit
		) VALUES ($1::uuid, 3, DATE '2026-09-27', 'interval', DATE '2026-09-27',
			TIME '10:00', TIME '11:00', 44641, 'UTC', 1, 'day')`
	assertPgErrorCode(t, execErr(ctx, conn, badDuration, scheduleID), "23514", "duration above 44640")

	// Weekday mode without its selectors is rejected.
	badWeekday := `
		INSERT INTO schedule_revisions (
			schedule_id, version, effective_local_date, mode, anchor_local_date,
			start_local_time, end_local_time, duration_minutes, timezone
		) VALUES ($1::uuid, 4, DATE '2026-09-27', 'weekday_pattern', DATE '2026-09-27',
			TIME '10:00', TIME '11:00', 60, 'UTC')`
	assertPgErrorCode(t, execErr(ctx, conn, badWeekday, scheduleID), "23514", "weekday mode without selectors")

	badWeekCadence := `
		INSERT INTO schedule_revisions (
			schedule_id, version, effective_local_date, mode, anchor_local_date,
			start_local_time, end_local_time, duration_minutes, timezone, weekdays
		) VALUES ($1::uuid, 5, DATE '2026-09-27', 'weekday_pattern', DATE '2026-09-27',
			TIME '10:00', TIME '11:00', 60, 'UTC', ARRAY[0]::SMALLINT[])`
	assertPgErrorCode(t, execErr(ctx, conn, badWeekCadence, scheduleID), "23514", "weekday mode without cadence")

	badIntervalUnit := `
		INSERT INTO schedule_revisions (
			schedule_id, version, effective_local_date, mode, anchor_local_date,
			start_local_time, end_local_time, duration_minutes, timezone, interval_count
		) VALUES ($1::uuid, 6, DATE '2026-09-27', 'interval', DATE '2026-09-27',
			TIME '10:00', TIME '11:00', 60, 'UTC', 1)`
	assertPgErrorCode(t, execErr(ctx, conn, badIntervalUnit, scheduleID), "23514", "interval mode without unit")

	duplicateWeekday := `
		INSERT INTO schedule_revisions (
			schedule_id, version, effective_local_date, mode, anchor_local_date,
			start_local_time, end_local_time, duration_minutes, timezone, week_cadence, weekdays
		) VALUES ($1::uuid, 7, DATE '2026-09-27', 'weekday_pattern', DATE '2026-09-27',
			TIME '10:00', TIME '11:00', 60, 'UTC', 1, ARRAY[0,0]::SMALLINT[])`
	assertPgErrorCode(t, execErr(ctx, conn, duplicateWeekday, scheduleID), "23514", "duplicate weekday selector")
}

func TestScheduleFoundation_TransactionAuthorizationAndReplay(t *testing.T) {
	ctx := context.Background()
	pool := openPool(t, ctx)
	defer pool.Close()
	conn := acquireConn(t, pool, ctx)
	defer conn.Release()
	schema := uniqueSchemaName(t)
	createSchema(t, conn, ctx, schema)
	defer dropSchema(t, pool, ctx, schema)
	for _, migration := range append(preFeatureMigrations, migration020Up) {
		runMigrationFile(t, conn, ctx, migration)
	}
	userID := "11111111-1111-1111-1111-111111111111"
	circleID := "22222222-2222-2222-2222-222222222222"
	if _, err := conn.Exec(ctx, `INSERT INTO users (id, firebase_uid, email) VALUES ($1::uuid, 'firebase-foundation', 'foundation@example.com')`, userID); err != nil {
		t.Fatalf("seed user: %v", err)
	}
	if _, err := conn.Exec(ctx, `INSERT INTO circles (id, name, teacher_id, invite_code) VALUES ($1::uuid, 'Foundation', $2::uuid, 'HLQ-F006CC')`, circleID, userID); err != nil {
		t.Fatalf("seed circle: %v", err)
	}
	if _, err := conn.Exec(ctx, `INSERT INTO circle_members (circle_id, user_id, role) VALUES ($1::uuid, $2::uuid, 'teacher')`, circleID, userID); err != nil {
		t.Fatalf("seed membership: %v", err)
	}
	for attempt := 0; attempt < 2; attempt++ {
		tx, err := conn.Begin(ctx)
		if err != nil {
			t.Fatalf("begin: %v", err)
		}
		if _, err := RequireCircleManage(ctx, tx, circleID, userID); err != nil {
			_ = tx.Rollback(ctx)
			t.Fatalf("manage access: %v", err)
		}
		_, replayed, err := ReplayOrExecute(ctx, tx, userID, "create-1", "create:circle", func(context.Context) (int, *string, error) {
			return 201, &circleID, nil
		})
		if err != nil {
			_ = tx.Rollback(ctx)
			t.Fatalf("replay attempt %d: %v", attempt, err)
		}
		if replayed != (attempt == 1) {
			_ = tx.Rollback(ctx)
			t.Fatalf("attempt %d replayed=%v", attempt, replayed)
		}
		if err := tx.Commit(ctx); err != nil {
			t.Fatalf("commit: %v", err)
		}
	}
}

func assertPgErrorCode(t *testing.T, err error, wantCode, scenario string) {
	t.Helper()
	if err == nil {
		t.Fatalf("%s: expected PostgreSQL error %s, got success", scenario, wantCode)
	}
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) || pgErr.Code != wantCode {
		t.Fatalf("%s: got %v, want PostgreSQL error %s", scenario, err, wantCode)
	}
}

func execErr(ctx context.Context, conn *pgxpool.Conn, sql string, args ...any) error {
	_, err := conn.Exec(ctx, sql, args...)
	return err
}

func assertScheduleTablesExist(t *testing.T, conn *pgxpool.Conn, ctx context.Context, want bool) {
	t.Helper()
	for _, table := range scheduleTables {
		var exists bool
		if err := conn.QueryRow(ctx, `SELECT to_regclass(current_schema() || '.' || $1) IS NOT NULL`, table).Scan(&exists); err != nil {
			t.Fatalf("check table %s: %v", table, err)
		}
		if exists != want {
			t.Fatalf("table %s exists=%v, want %v", table, exists, want)
		}
	}
}

func assertColumnExists(t *testing.T, conn *pgxpool.Conn, ctx context.Context, table, column string, want bool) {
	t.Helper()
	var exists bool
	if err := conn.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM information_schema.columns
			WHERE table_schema = current_schema() AND table_name = $1 AND column_name = $2
		)`, table, column).Scan(&exists); err != nil {
		t.Fatalf("check column %s.%s: %v", table, column, err)
	}
	if exists != want {
		t.Fatalf("column %s.%s exists=%v, want %v", table, column, exists, want)
	}
}

func openPool(t *testing.T, ctx context.Context) *pgxpool.Pool {
	t.Helper()
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		t.Skip("DATABASE_URL is not set")
	}
	pool, err := pgxpool.New(ctx, dbURL)
	if err != nil {
		t.Fatalf("open pool: %v", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		t.Fatalf("ping database: %v", err)
	}
	return pool
}

func acquireConn(t *testing.T, pool *pgxpool.Pool, ctx context.Context) *pgxpool.Conn {
	t.Helper()
	conn, err := pool.Acquire(ctx)
	if err != nil {
		t.Fatalf("acquire connection: %v", err)
	}
	return conn
}

func createSchema(t *testing.T, conn *pgxpool.Conn, ctx context.Context, schema string) {
	t.Helper()
	if _, err := conn.Exec(ctx, fmt.Sprintf("CREATE SCHEMA IF NOT EXISTS %s", schema)); err != nil {
		t.Fatalf("create schema: %v", err)
	}
	if _, err := conn.Exec(ctx, fmt.Sprintf("SET search_path TO %s", schema)); err != nil {
		t.Fatalf("set search_path: %v", err)
	}
}

func dropSchema(t *testing.T, pool *pgxpool.Pool, ctx context.Context, schema string) {
	t.Helper()
	dropCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	_, _ = pool.Exec(dropCtx, fmt.Sprintf("DROP SCHEMA IF EXISTS %s CASCADE", schema))
}

func runMigrationFile(t *testing.T, conn *pgxpool.Conn, ctx context.Context, filename string) {
	t.Helper()
	_, currentFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("could not determine test file path")
	}
	path := filepath.Join(filepath.Dir(currentFile), "..", "..", "migrations", filename)
	sql, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read migration file %s: %v", filename, err)
	}
	if _, err := conn.Exec(ctx, string(sql)); err != nil {
		t.Fatalf("execute migration %s: %v", filename, err)
	}
}

func uniqueSchemaName(t *testing.T) string {
	t.Helper()
	return fmt.Sprintf("test_scheduling_%d", time.Now().UnixNano())
}
