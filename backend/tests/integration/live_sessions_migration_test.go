//go:build integration

package integration

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	migration016Up   = "000016_live_sessions.up.sql"
	migration016Down = "000016_live_sessions.down.sql"
)

// liveSessionHeadMigrations is every migration up to the pre-F-005 head.
var liveSessionHeadMigrations = []string{
	"000010_auth_roles_profile.up.sql",
	"000011_auth_roles_profile_alignment.up.sql",
	"000012_auth_profiles_display_name.up.sql",
	"000013_create_circles.up.sql",
	"000014_circle_members_circle_fk.up.sql",
	"000015_circle_management.up.sql",
}

func TestLiveSessionsMigration_FreshSchema(t *testing.T) {
	ctx := context.Background()
	pool := openPool(t, ctx)
	defer pool.Close()
	conn := acquireConn(t, pool, ctx)
	defer conn.Release()

	schema := uniqueSchemaName(t)
	createSchema(t, conn, ctx, schema)
	defer dropSchema(t, pool, ctx, schema)

	for _, migration := range append(liveSessionHeadMigrations, migration016Up) {
		runMigrationFile(t, conn, ctx, migration)
	}

	assertConstraintExists(t, conn, ctx, schema, "sessions", "ck_sessions_status")
	assertConstraintExists(t, conn, ctx, schema, "sessions", "ck_sessions_end_reason")
	assertConstraintExists(t, conn, ctx, schema, "sessions", "ck_sessions_media_mode")
	assertConstraintExists(t, conn, ctx, schema, "sessions", "ck_sessions_participant_count")
	assertConstraintExists(t, conn, ctx, schema, "sessions", "uq_sessions_media_room_ref")
	assertConstraintExists(t, conn, ctx, schema, "sessions", "fk_sessions_circle_id")
	assertConstraintExists(t, conn, ctx, schema, "sessions", "fk_sessions_created_by")
	assertConstraintExists(t, conn, ctx, schema, "session_participant_presence", "uq_session_participant_presence_session_user")
	assertConstraintExists(t, conn, ctx, schema, "session_participant_presence", "fk_session_participant_presence_session_id")
	assertConstraintExists(t, conn, ctx, schema, "session_participant_presence", "fk_session_participant_presence_user_id")
	assertIndexExists(t, conn, ctx, schema, "idx_sessions_circle_status")
	assertIndexExists(t, conn, ctx, schema, "idx_session_presence_current")
	assertColumnNotNull(t, conn, ctx, schema, "sessions", "status")
	assertColumnNotNull(t, conn, ctx, schema, "sessions", "media_mode")
	assertColumnNotNull(t, conn, ctx, schema, "sessions", "participant_count")

	// F-005 rows are ad-hoc: scheduled_at stays nullable and unset by default.
	userID := seedLiveSessionUser(t, conn, ctx, "fresh-schema")
	circleID := seedLiveSessionCircle(t, conn, ctx, userID)
	var status, mediaMode string
	var scheduledAt *string
	var isLocked bool
	var participantCount int
	if err := conn.QueryRow(ctx, `
		INSERT INTO sessions (circle_id, created_by)
		VALUES ($1::uuid, $2::uuid)
		RETURNING status, scheduled_at, media_mode, is_locked, participant_count
	`, circleID, userID).Scan(&status, &scheduledAt, &mediaMode, &isLocked, &participantCount); err != nil {
		t.Fatalf("insert default session: %v", err)
	}
	if status != "scheduled" || scheduledAt != nil || mediaMode != "audio_only" || isLocked || participantCount != 0 {
		t.Fatalf("unexpected session defaults: status=%q scheduled_at=%v media_mode=%q is_locked=%v count=%d",
			status, scheduledAt, mediaMode, isLocked, participantCount)
	}
}

func TestLiveSessionsMigration_UpgradeFromHead(t *testing.T) {
	ctx := context.Background()
	pool := openPool(t, ctx)
	defer pool.Close()
	conn := acquireConn(t, pool, ctx)
	defer conn.Release()

	schema := uniqueSchemaName(t)
	createSchema(t, conn, ctx, schema)
	defer dropSchema(t, pool, ctx, schema)

	for _, migration := range liveSessionHeadMigrations {
		runMigrationFile(t, conn, ctx, migration)
	}
	userID := seedLiveSessionUser(t, conn, ctx, "upgrade-head")
	circleID := seedLiveSessionCircle(t, conn, ctx, userID)

	runMigrationFile(t, conn, ctx, migration016Up)

	var name string
	if err := conn.QueryRow(ctx, `SELECT name FROM circles WHERE id = $1::uuid`, circleID).Scan(&name); err != nil {
		t.Fatalf("read legacy circle after upgrade: %v", err)
	}
	if name != "Legacy Circle" {
		t.Fatalf("upgrade changed legacy circle: %q", name)
	}
	var sessionID string
	if err := conn.QueryRow(ctx, `
		INSERT INTO sessions (circle_id, created_by, status, actual_start)
		VALUES ($1::uuid, $2::uuid, 'active', NOW())
		RETURNING id::text
	`, circleID, userID).Scan(&sessionID); err != nil {
		t.Fatalf("insert upgraded session: %v", err)
	}
	if _, err := conn.Exec(ctx, `
		INSERT INTO session_participant_presence (session_id, user_id, first_joined_at, last_joined_at, is_currently_present)
		VALUES ($1::uuid, $2::uuid, NOW(), NOW(), TRUE)
	`, sessionID, userID); err != nil {
		t.Fatalf("insert upgraded presence: %v", err)
	}
}

func TestLiveSessionsMigration_RollbackRestoresPriorState(t *testing.T) {
	ctx := context.Background()
	pool := openPool(t, ctx)
	defer pool.Close()
	conn := acquireConn(t, pool, ctx)
	defer conn.Release()

	schema := uniqueSchemaName(t)
	createSchema(t, conn, ctx, schema)
	defer dropSchema(t, pool, ctx, schema)

	for _, migration := range append(liveSessionHeadMigrations, migration016Up) {
		runMigrationFile(t, conn, ctx, migration)
	}
	userID := seedLiveSessionUser(t, conn, ctx, "rollback")
	circleID := seedLiveSessionCircle(t, conn, ctx, userID)

	runMigrationFile(t, conn, ctx, migration016Down)

	assertLiveSessionTablesDropped(t, conn, ctx)
	var name string
	if err := conn.QueryRow(ctx, `SELECT name FROM circles WHERE id = $1::uuid`, circleID).Scan(&name); err != nil {
		t.Fatalf("rollback removed legacy circle: %v", err)
	}
	if name != "Legacy Circle" {
		t.Fatalf("rollback changed legacy circle: %q", name)
	}

	// The down migration only drops F-005 objects and is replay-safe.
	runMigrationFile(t, conn, ctx, migration016Down)
}

func TestLiveSessionsMigration_RerunSafety(t *testing.T) {
	ctx := context.Background()
	pool := openPool(t, ctx)
	defer pool.Close()
	conn := acquireConn(t, pool, ctx)
	defer conn.Release()

	schema := uniqueSchemaName(t)
	createSchema(t, conn, ctx, schema)
	defer dropSchema(t, pool, ctx, schema)

	for _, migration := range append(liveSessionHeadMigrations, migration016Up) {
		runMigrationFile(t, conn, ctx, migration)
	}
	userID := seedLiveSessionUser(t, conn, ctx, "rerun")
	circleID := seedLiveSessionCircle(t, conn, ctx, userID)
	sessionID := insertLiveSession(t, conn, ctx, circleID, userID, "")
	if _, err := conn.Exec(ctx, `
		INSERT INTO session_participant_presence (session_id, user_id, first_joined_at, last_joined_at, is_currently_present)
		VALUES ($1::uuid, $2::uuid, NOW(), NOW(), TRUE)
	`, sessionID, userID); err != nil {
		t.Fatalf("seed presence: %v", err)
	}

	runMigrationFile(t, conn, ctx, migration016Up)

	var count int
	if err := conn.QueryRow(ctx, `SELECT COUNT(*) FROM session_participant_presence WHERE session_id = $1::uuid`, sessionID).Scan(&count); err != nil {
		t.Fatalf("count presence after rerun: %v", err)
	}
	if count != 1 {
		t.Fatalf("rerun changed presence rows: got %d, want 1", count)
	}
}

func TestLiveSessionsMigration_ConstraintViolations(t *testing.T) {
	ctx := context.Background()
	pool := openPool(t, ctx)
	defer pool.Close()
	conn := acquireConn(t, pool, ctx)
	defer conn.Release()

	schema := uniqueSchemaName(t)
	createSchema(t, conn, ctx, schema)
	defer dropSchema(t, pool, ctx, schema)

	for _, migration := range append(liveSessionHeadMigrations, migration016Up) {
		runMigrationFile(t, conn, ctx, migration)
	}
	userID := seedLiveSessionUser(t, conn, ctx, "constraints")
	circleID := seedLiveSessionCircle(t, conn, ctx, userID)

	cases := []struct {
		name string
		sql  string
		code string
	}{
		{"bad status", `
			INSERT INTO sessions (circle_id, created_by, status)
			VALUES ($1::uuid, $2::uuid, 'paused')`, "23514"},
		{"bad end reason", `
			INSERT INTO sessions (circle_id, created_by, status, end_reason)
			VALUES ($1::uuid, $2::uuid, 'ended', 'crash')`, "23514"},
		{"bad media mode", `
			INSERT INTO sessions (circle_id, created_by, media_mode)
			VALUES ($1::uuid, $2::uuid, 'video_only')`, "23514"},
		{"participant count above 50", `
			INSERT INTO sessions (circle_id, created_by, participant_count)
			VALUES ($1::uuid, $2::uuid, 51)`, "23514"},
	}
	for _, tc := range cases {
		if _, err := conn.Exec(ctx, tc.sql, circleID, userID); !isPgErrorCode(t, err, tc.code) {
			t.Fatalf("%s: got %v, want pg error %s", tc.name, err, tc.code)
		}
	}

	firstID := insertLiveSession(t, conn, ctx, circleID, userID, "room-ref-a")
	if _, err := conn.Exec(ctx, `
		INSERT INTO sessions (circle_id, created_by, media_room_ref)
		VALUES ($1::uuid, $2::uuid, 'room-ref-a')
	`, circleID, userID); !isPgErrorCode(t, err, "23505") {
		t.Fatalf("duplicate media_room_ref: got %v, want unique violation", err)
	}

	secondUser := seedLiveSessionUser(t, conn, ctx, "constraints-second")
	if _, err := conn.Exec(ctx, `
		INSERT INTO session_participant_presence (session_id, user_id)
		VALUES ($1::uuid, $2::uuid)
	`, firstID, userID); err != nil {
		t.Fatalf("seed first presence row: %v", err)
	}
	if _, err := conn.Exec(ctx, `
		INSERT INTO session_participant_presence (session_id, user_id)
		VALUES ($1::uuid, $2::uuid)
	`, firstID, userID); !isPgErrorCode(t, err, "23505") {
		t.Fatalf("duplicate presence pair: got %v, want unique violation", err)
	}
	// A different user in the same session, and the same user elsewhere, stay legal.
	if _, err := conn.Exec(ctx, `
		INSERT INTO session_participant_presence (session_id, user_id)
		VALUES ($1::uuid, $2::uuid)
	`, firstID, secondUser); err != nil {
		t.Fatalf("seed second presence row: %v", err)
	}
}

// TestLiveSessionsMigration_ParticipantCapRace simulates the admission decision
// the repository performs under the session row lock: 51 concurrent admissions
// must admit exactly 50, backed by the participant_count CHECK constraint.
func TestLiveSessionsMigration_ParticipantCapRace(t *testing.T) {
	ctx := context.Background()
	pool := openPool(t, ctx)
	defer pool.Close()
	conn := acquireConn(t, pool, ctx)
	defer conn.Release()

	schema := uniqueSchemaName(t)
	createSchema(t, conn, ctx, schema)
	defer dropSchema(t, pool, ctx, schema)

	for _, migration := range append(liveSessionHeadMigrations, migration016Up) {
		runMigrationFile(t, conn, ctx, migration)
	}
	userID := seedLiveSessionUser(t, conn, ctx, "cap-race")
	circleID := seedLiveSessionCircle(t, conn, ctx, userID)
	sessionID := insertLiveSession(t, conn, ctx, circleID, userID, "")
	conn.Release()

	schemaPool := openSchemaPool(t, ctx, schema)
	defer schemaPool.Close()

	const attempts = 51
	results := make(chan bool, attempts)
	var wg sync.WaitGroup
	for range attempts {
		wg.Add(1)
		go func() {
			defer wg.Done()
			tag, err := schemaPool.Exec(ctx, `
				UPDATE sessions
				SET participant_count = participant_count + 1, updated_at = NOW()
				WHERE id = $1::uuid AND participant_count < 50
			`, sessionID)
			if err != nil {
				t.Errorf("concurrent admission: %v", err)
				results <- false
				return
			}
			results <- tag.RowsAffected() == 1
		}()
	}
	wg.Wait()
	close(results)

	admitted := 0
	for ok := range results {
		if ok {
			admitted++
		}
	}
	if admitted != 50 {
		t.Fatalf("concurrent admissions: got %d admitted, want exactly 50", admitted)
	}

	checkConn, err := schemaPool.Acquire(ctx)
	if err != nil {
		t.Fatalf("acquire check connection: %v", err)
	}
	defer checkConn.Release()
	var count int
	if err := checkConn.QueryRow(ctx, `SELECT participant_count FROM sessions WHERE id = $1::uuid`, sessionID).Scan(&count); err != nil {
		t.Fatalf("read final count: %v", err)
	}
	if count != 50 {
		t.Fatalf("final participant_count: got %d, want 50", count)
	}
	if _, err := checkConn.Exec(ctx, `
		UPDATE sessions SET participant_count = participant_count + 1 WHERE id = $1::uuid
	`, sessionID); !isPgErrorCode(t, err, "23514") {
		t.Fatalf("over-capacity update: got %v, want check violation", err)
	}
}

func seedLiveSessionUser(t *testing.T, conn *pgxpool.Conn, ctx context.Context, label string) string {
	t.Helper()
	var id string
	if err := conn.QueryRow(ctx, `
		INSERT INTO users (firebase_uid, email)
		VALUES ($1, $2)
		RETURNING id::text
	`, "firebase-"+label, label+"@example.com").Scan(&id); err != nil {
		t.Fatalf("seed user: %v", err)
	}
	return id
}

func seedLiveSessionCircle(t *testing.T, conn *pgxpool.Conn, ctx context.Context, teacherID string) string {
	t.Helper()
	var id string
	if err := conn.QueryRow(ctx, `
		INSERT INTO circles (name, teacher_id, invite_code)
		VALUES ('Legacy Circle', $1::uuid, 'HLQ-LIVE01')
		RETURNING id::text
	`, teacherID).Scan(&id); err != nil {
		t.Fatalf("seed circle: %v", err)
	}
	return id
}

func insertLiveSession(t *testing.T, conn *pgxpool.Conn, ctx context.Context, circleID, createdBy, roomRef string) string {
	t.Helper()
	var id string
	if err := conn.QueryRow(ctx, `
		INSERT INTO sessions (circle_id, created_by, media_room_ref)
		VALUES ($1::uuid, $2::uuid, NULLIF($3, ''))
		RETURNING id::text
	`, circleID, createdBy, roomRef).Scan(&id); err != nil {
		t.Fatalf("seed session: %v", err)
	}
	return id
}

func assertLiveSessionTablesDropped(t *testing.T, conn *pgxpool.Conn, ctx context.Context) {
	t.Helper()
	for _, table := range []string{"sessions", "session_participant_presence"} {
		var reg *string
		if err := conn.QueryRow(ctx, `SELECT to_regclass($1)`, table).Scan(&reg); err != nil {
			t.Fatalf("lookup table %s after rollback: %v", table, err)
		}
		if reg != nil {
			t.Fatalf("table %s still exists after rollback", table)
		}
	}
}

func isPgErrorCode(t *testing.T, err error, code string) bool {
	t.Helper()
	if err == nil {
		return false
	}
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == code
}

// openSchemaPool opens a pool whose connections resolve unqualified names in
// the isolated test schema, so concurrent goroutines can run against it.
func openSchemaPool(t *testing.T, ctx context.Context, schema string) *pgxpool.Pool {
	t.Helper()
	dbURL := databaseURLForSchema(t, schema)
	pool, err := pgxpool.New(ctx, dbURL)
	if err != nil {
		t.Fatalf("open schema pool: %v", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		t.Fatalf("ping schema pool: %v", err)
	}
	return pool
}

func databaseURLForSchema(t *testing.T, schema string) string {
	t.Helper()
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		t.Fatal("DATABASE_URL is not set")
	}
	sep := "?"
	if strings.Contains(dbURL, sep) {
		sep = "&"
	}
	return fmt.Sprintf("%s%ssearch_path=%s", dbURL, sep, schema)
}
