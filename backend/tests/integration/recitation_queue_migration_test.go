//go:build integration

package integration

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	migration017Up   = "000017_recitation_queue_system.up.sql"
	migration017Down = "000017_recitation_queue_system.down.sql"
)

// recitationQueueHeadMigrations is every migration up to and including F-003.
var recitationQueueHeadMigrations = append(liveSessionHeadMigrations, migration016Up, migration017Up)

// recitationQueueTables are all tables created by migration 000017.
var recitationQueueTables = []string{
	"quran_surahs",
	"recitation_queue",
	"recitation_queue_preorder",
	"recitation_queue_entries",
	"queue_opt_out_requests",
	"queue_command_receipts",
	"queue_event_outbox",
	"memorization_progress",
}

// sessionQueuePolicyColumns are the F-003 columns added to sessions (ADR-018).
var sessionQueuePolicyColumns = []string{
	"queue_population_policy",
	"queue_finalization_policy",
	"queue_opt_out_policy",
	"queue_grade_visibility",
	"queue_grade_correction",
	"queue_policy_version",
}

type sqlViolationCase struct {
	name string
	sql  string
	code string
	args []any
}

func TestRecitationQueueMigration_FreshSchemaAndSurahSeed(t *testing.T) {
	ctx := context.Background()
	pool := openPool(t, ctx)
	defer pool.Close()
	conn := acquireConn(t, pool, ctx)
	defer conn.Release()

	schema := uniqueSchemaName(t)
	createSchema(t, conn, ctx, schema)
	defer dropSchema(t, pool, ctx, schema)

	for _, migration := range recitationQueueHeadMigrations {
		runMigrationFile(t, conn, ctx, migration)
	}

	assertRecitationQueueTables(t, conn, ctx, true)

	var count, distinctIDs, minID, maxID, minAyahCount int
	if err := conn.QueryRow(ctx, `
		SELECT COUNT(*), COUNT(DISTINCT id), MIN(id), MAX(id), MIN(ayah_count)
		FROM quran_surahs
	`).Scan(&count, &distinctIDs, &minID, &maxID, &minAyahCount); err != nil {
		t.Fatalf("read surah seed summary: %v", err)
	}
	if count != 114 || distinctIDs != 114 || minID != 1 || maxID != 114 {
		t.Fatalf("surah seed: got count=%d distinct=%d min_id=%d max_id=%d, want 114 rows with ids 1..114",
			count, distinctIDs, minID, maxID)
	}
	if minAyahCount <= 0 {
		t.Fatalf("surah seed: minimum ayah_count=%d, want positive", minAyahCount)
	}
}

func TestRecitationQueueMigration_RoundConstraints(t *testing.T) {
	ctx := context.Background()
	pool := openPool(t, ctx)
	defer pool.Close()
	conn := acquireConn(t, pool, ctx)
	defer conn.Release()

	schema := uniqueSchemaName(t)
	createSchema(t, conn, ctx, schema)
	defer dropSchema(t, pool, ctx, schema)

	for _, migration := range recitationQueueHeadMigrations {
		runMigrationFile(t, conn, ctx, migration)
	}

	userID := seedLiveSessionUser(t, conn, ctx, "round-constraints")
	circleID := seedLiveSessionCircle(t, conn, ctx, userID)
	sessionID := insertLiveSession(t, conn, ctx, circleID, userID, "")
	otherSessionID := insertLiveSession(t, conn, ctx, circleID, userID, "")

	// Several prepared rounds may stack (clarification B1); at most one active
	// round per session; a different session may hold its own active round.
	insertRecitationRound(t, conn, ctx, sessionID, userID, "prepared", 1)
	insertRecitationRound(t, conn, ctx, sessionID, userID, "active", 2)
	insertRecitationRound(t, conn, ctx, sessionID, userID, "prepared", 3)
	insertRecitationRound(t, conn, ctx, otherSessionID, userID, "active", 1)

	expectViolations(t, conn, ctx, []sqlViolationCase{
		{"second active round in session", `
			INSERT INTO recitation_queue (session_id, round_number, round_type, surah_id, from_ayah, to_ayah, grading_required, lifecycle, created_by)
			VALUES ($1::uuid, 4, 'revision', 1, 1, 7, TRUE, 'active', $2::uuid)`, "23505", []any{sessionID, userID}},
		{"duplicate round number", `
			INSERT INTO recitation_queue (session_id, round_number, round_type, surah_id, from_ayah, to_ayah, grading_required, lifecycle, created_by)
			VALUES ($1::uuid, 1, 'revision', 1, 1, 7, TRUE, 'prepared', $2::uuid)`, "23505", []any{sessionID, userID}},
		{"bad round type", `
			INSERT INTO recitation_queue (session_id, round_number, round_type, surah_id, from_ayah, to_ayah, grading_required, lifecycle, created_by)
			VALUES ($1::uuid, 5, 'talfil', 1, 1, 7, TRUE, 'prepared', $2::uuid)`, "23514", []any{sessionID, userID}},
		{"bad lifecycle", `
			INSERT INTO recitation_queue (session_id, round_number, round_type, surah_id, from_ayah, to_ayah, grading_required, lifecycle, created_by)
			VALUES ($1::uuid, 6, 'revision', 1, 1, 7, TRUE, 'paused', $2::uuid)`, "23514", []any{sessionID, userID}},
		{"zero round number", `
			INSERT INTO recitation_queue (session_id, round_number, round_type, surah_id, from_ayah, to_ayah, grading_required, lifecycle, created_by)
			VALUES ($1::uuid, 0, 'revision', 1, 1, 7, TRUE, 'prepared', $2::uuid)`, "23514", []any{sessionID, userID}},
		{"zero from_ayah", `
			INSERT INTO recitation_queue (session_id, round_number, round_type, surah_id, from_ayah, to_ayah, grading_required, lifecycle, created_by)
			VALUES ($1::uuid, 7, 'revision', 1, 0, 7, TRUE, 'prepared', $2::uuid)`, "23514", []any{sessionID, userID}},
		{"from_ayah above to_ayah", `
			INSERT INTO recitation_queue (session_id, round_number, round_type, surah_id, from_ayah, to_ayah, grading_required, lifecycle, created_by)
			VALUES ($1::uuid, 8, 'revision', 1, 8, 7, TRUE, 'prepared', $2::uuid)`, "23514", []any{sessionID, userID}},
		{"missing session", `
			INSERT INTO recitation_queue (session_id, round_number, round_type, surah_id, from_ayah, to_ayah, grading_required, lifecycle, created_by)
			VALUES ('99999999-9999-9999-9999-999999999999'::uuid, 9, 'revision', 1, 1, 7, TRUE, 'prepared', $1::uuid)`, "23503", []any{userID}},
		{"missing surah", `
			INSERT INTO recitation_queue (session_id, round_number, round_type, surah_id, from_ayah, to_ayah, grading_required, lifecycle, created_by)
			VALUES ($1::uuid, 10, 'revision', 999, 1, 7, TRUE, 'prepared', $2::uuid)`, "23503", []any{sessionID, userID}},
	})
}

func TestRecitationQueueMigration_PreorderConstraints(t *testing.T) {
	ctx := context.Background()
	pool := openPool(t, ctx)
	defer pool.Close()
	conn := acquireConn(t, pool, ctx)
	defer conn.Release()

	schema := uniqueSchemaName(t)
	createSchema(t, conn, ctx, schema)
	defer dropSchema(t, pool, ctx, schema)

	for _, migration := range recitationQueueHeadMigrations {
		runMigrationFile(t, conn, ctx, migration)
	}

	teacherID := seedLiveSessionUser(t, conn, ctx, "preorder-teacher")
	circleID := seedLiveSessionCircle(t, conn, ctx, teacherID)
	sessionID := insertLiveSession(t, conn, ctx, circleID, teacherID, "")
	queueID := insertRecitationRound(t, conn, ctx, sessionID, teacherID, "prepared", 1)
	student1 := seedLiveSessionUser(t, conn, ctx, "preorder-s1")
	student2 := seedLiveSessionUser(t, conn, ctx, "preorder-s2")
	student3 := seedLiveSessionUser(t, conn, ctx, "preorder-s3")

	insertQueuePreorder(t, conn, ctx, queueID, student1, teacherID, 1)

	expectViolations(t, conn, ctx, []sqlViolationCase{
		{"duplicate candidate student", `
			INSERT INTO recitation_queue_preorder (queue_id, student_id, position, added_by)
			VALUES ($1::uuid, $2::uuid, 2, $3::uuid)`, "23505", []any{queueID, student1, teacherID}},
		{"duplicate position", `
			INSERT INTO recitation_queue_preorder (queue_id, student_id, position, added_by)
			VALUES ($1::uuid, $2::uuid, 1, $3::uuid)`, "23505", []any{queueID, student2, teacherID}},
		{"zero position", `
			INSERT INTO recitation_queue_preorder (queue_id, student_id, position, added_by)
			VALUES ($1::uuid, $2::uuid, 0, $3::uuid)`, "23514", []any{queueID, student3, teacherID}},
		{"missing round", `
			INSERT INTO recitation_queue_preorder (queue_id, student_id, position, added_by)
			VALUES ('99999999-9999-9999-9999-999999999999'::uuid, $1::uuid, 1, $2::uuid)`, "23503", []any{student3, teacherID}},
	})

	// A second candidate at the next position stays legal.
	insertQueuePreorder(t, conn, ctx, queueID, student2, teacherID, 2)
}

func TestRecitationQueueMigration_EntryConstraints(t *testing.T) {
	ctx := context.Background()
	pool := openPool(t, ctx)
	defer pool.Close()
	conn := acquireConn(t, pool, ctx)
	defer conn.Release()

	schema := uniqueSchemaName(t)
	createSchema(t, conn, ctx, schema)
	defer dropSchema(t, pool, ctx, schema)

	for _, migration := range recitationQueueHeadMigrations {
		runMigrationFile(t, conn, ctx, migration)
	}

	teacherID := seedLiveSessionUser(t, conn, ctx, "entry-teacher")
	circleID := seedLiveSessionCircle(t, conn, ctx, teacherID)
	sessionID := insertLiveSession(t, conn, ctx, circleID, teacherID, "")
	queueID := insertRecitationRound(t, conn, ctx, sessionID, teacherID, "active", 1)
	student1 := seedLiveSessionUser(t, conn, ctx, "entry-s1")
	student2 := seedLiveSessionUser(t, conn, ctx, "entry-s2")
	student3 := seedLiveSessionUser(t, conn, ctx, "entry-s3")
	student4 := seedLiveSessionUser(t, conn, ctx, "entry-s4")

	insertQueueEntry(t, conn, ctx, queueID, student1, "waiting", 1)
	insertQueueEntry(t, conn, ctx, queueID, student2, "reciting", 2)

	expectViolations(t, conn, ctx, []sqlViolationCase{
		{"second reciter in round", `
			INSERT INTO recitation_queue_entries (queue_id, student_id, position, status)
			VALUES ($1::uuid, $2::uuid, 3, 'reciting')`, "23505", []any{queueID, student3}},
		{"duplicate student in round", `
			INSERT INTO recitation_queue_entries (queue_id, student_id, position, status)
			VALUES ($1::uuid, $2::uuid, 4, 'waiting')`, "23505", []any{queueID, student1}},
		{"duplicate position", `
			INSERT INTO recitation_queue_entries (queue_id, student_id, position, status)
			VALUES ($1::uuid, $2::uuid, 2, 'waiting')`, "23505", []any{queueID, student3}},
		{"bad entry status", `
			INSERT INTO recitation_queue_entries (queue_id, student_id, position, status)
			VALUES ($1::uuid, $2::uuid, 5, 'paused')`, "23514", []any{queueID, student3}},
		{"bad grade", `
			INSERT INTO recitation_queue_entries (queue_id, student_id, position, status, grade)
			VALUES ($1::uuid, $2::uuid, 5, 'waiting', 'mashallah')`, "23514", []any{queueID, student3}},
	})

	// A canonical grade on a completed entry stays legal.
	if _, err := conn.Exec(ctx, `
		INSERT INTO recitation_queue_entries (queue_id, student_id, position, status, grade)
		VALUES ($1::uuid, $2::uuid, 6, 'completed', 'excellent')
	`, queueID, student4); err != nil {
		t.Fatalf("insert completed entry with canonical grade: %v", err)
	}
}

func TestRecitationQueueMigration_OptOutSinglePendingRequest(t *testing.T) {
	ctx := context.Background()
	pool := openPool(t, ctx)
	defer pool.Close()
	conn := acquireConn(t, pool, ctx)
	defer conn.Release()

	schema := uniqueSchemaName(t)
	createSchema(t, conn, ctx, schema)
	defer dropSchema(t, pool, ctx, schema)

	for _, migration := range recitationQueueHeadMigrations {
		runMigrationFile(t, conn, ctx, migration)
	}

	teacherID := seedLiveSessionUser(t, conn, ctx, "optout-teacher")
	circleID := seedLiveSessionCircle(t, conn, ctx, teacherID)
	sessionID := insertLiveSession(t, conn, ctx, circleID, teacherID, "")
	queueID := insertRecitationRound(t, conn, ctx, sessionID, teacherID, "active", 1)
	studentID := seedLiveSessionUser(t, conn, ctx, "optout-student")
	entryID := insertQueueEntry(t, conn, ctx, queueID, studentID, "waiting", 1)

	if _, err := conn.Exec(ctx, `
		INSERT INTO queue_opt_out_requests (queue_entry_id, requested_by, status)
		VALUES ($1::uuid, $2::uuid, 'pending')
	`, entryID, studentID); err != nil {
		t.Fatalf("insert pending opt-out request: %v", err)
	}
	// An already-decided request may coexist with the pending one (audit trail).
	if _, err := conn.Exec(ctx, `
		INSERT INTO queue_opt_out_requests (queue_entry_id, requested_by, status, decided_by, decided_at)
		VALUES ($1::uuid, $2::uuid, 'approved', $3::uuid, NOW())
	`, entryID, studentID, teacherID); err != nil {
		t.Fatalf("insert decided opt-out request: %v", err)
	}

	expectViolations(t, conn, ctx, []sqlViolationCase{
		{"second pending request for entry", `
			INSERT INTO queue_opt_out_requests (queue_entry_id, requested_by, status)
			VALUES ($1::uuid, $2::uuid, 'pending')`, "23505", []any{entryID, studentID}},
		{"bad request status", `
			INSERT INTO queue_opt_out_requests (queue_entry_id, requested_by, status)
			VALUES ($1::uuid, $2::uuid, 'cancelled')`, "23514", []any{entryID, studentID}},
	})
}

func TestRecitationQueueMigration_CommandReceiptsCompositeKey(t *testing.T) {
	ctx := context.Background()
	pool := openPool(t, ctx)
	defer pool.Close()
	conn := acquireConn(t, pool, ctx)
	defer conn.Release()

	schema := uniqueSchemaName(t)
	createSchema(t, conn, ctx, schema)
	defer dropSchema(t, pool, ctx, schema)

	for _, migration := range recitationQueueHeadMigrations {
		runMigrationFile(t, conn, ctx, migration)
	}

	actorID := seedLiveSessionUser(t, conn, ctx, "receipt-actor")
	otherActorID := seedLiveSessionUser(t, conn, ctx, "receipt-other")
	circleID := seedLiveSessionCircle(t, conn, ctx, actorID)
	sessionID := insertLiveSession(t, conn, ctx, circleID, actorID, "")

	if _, err := conn.Exec(ctx, `
		INSERT INTO queue_command_receipts (session_id, actor_id, idempotency_key, command)
		VALUES ($1::uuid, $2::uuid, 'create-round-1', 'queue.create_round')
	`, sessionID, actorID); err != nil {
		t.Fatalf("insert command receipt: %v", err)
	}

	// A reused key with another command is a conflict on the composite key.
	expectViolations(t, conn, ctx, []sqlViolationCase{
		{"reused idempotency key", `
			INSERT INTO queue_command_receipts (session_id, actor_id, idempotency_key, command)
			VALUES ($1::uuid, $2::uuid, 'create-round-1', 'queue.advance')`, "23505", []any{sessionID, actorID}},
	})

	// Same key under another actor, and another key under the same actor, stay legal.
	if _, err := conn.Exec(ctx, `
		INSERT INTO queue_command_receipts (session_id, actor_id, idempotency_key, command)
		VALUES ($1::uuid, $2::uuid, 'create-round-1', 'queue.create_round')
	`, sessionID, otherActorID); err != nil {
		t.Fatalf("insert receipt for other actor: %v", err)
	}
	if _, err := conn.Exec(ctx, `
		INSERT INTO queue_command_receipts (session_id, actor_id, idempotency_key, command)
		VALUES ($1::uuid, $2::uuid, 'advance-1', 'queue.advance')
	`, sessionID, actorID); err != nil {
		t.Fatalf("insert receipt with another key: %v", err)
	}
}

func TestRecitationQueueMigration_EventOutboxShape(t *testing.T) {
	ctx := context.Background()
	pool := openPool(t, ctx)
	defer pool.Close()
	conn := acquireConn(t, pool, ctx)
	defer conn.Release()

	schema := uniqueSchemaName(t)
	createSchema(t, conn, ctx, schema)
	defer dropSchema(t, pool, ctx, schema)

	for _, migration := range recitationQueueHeadMigrations {
		runMigrationFile(t, conn, ctx, migration)
	}

	userID := seedLiveSessionUser(t, conn, ctx, "outbox-user")
	circleID := seedLiveSessionCircle(t, conn, ctx, userID)
	sessionID := insertLiveSession(t, conn, ctx, circleID, userID, "")
	queueID := insertRecitationRound(t, conn, ctx, sessionID, userID, "active", 1)

	if _, err := conn.Exec(ctx, `
		INSERT INTO queue_event_outbox (event_id, session_id, round_id, event_type, round_version, event_metadata, available_at, attempt_count)
		VALUES ('11111111-1111-1111-1111-111111111111'::uuid, $1::uuid, $2::uuid, 'queue.round_started', 1, '{}', NOW(), 0)
	`, sessionID, queueID); err != nil {
		t.Fatalf("insert outbox event: %v", err)
	}

	expectViolations(t, conn, ctx, []sqlViolationCase{
		{"duplicate event id", `
			INSERT INTO queue_event_outbox (event_id, session_id, round_id, event_type, round_version, event_metadata, available_at, attempt_count)
			VALUES ('11111111-1111-1111-1111-111111111111'::uuid, $1::uuid, $2::uuid, 'queue.entry_completed', 2, '{}', NOW(), 0)`, "23505", []any{sessionID, queueID}},
		{"negative attempt count", `
			INSERT INTO queue_event_outbox (event_id, session_id, round_id, event_type, round_version, event_metadata, available_at, attempt_count)
			VALUES ('22222222-2222-2222-2222-222222222222'::uuid, $1::uuid, $2::uuid, 'queue.entry_completed', 2, '{}', NOW(), -1)`, "23514", []any{sessionID, queueID}},
		{"missing round", `
			INSERT INTO queue_event_outbox (event_id, session_id, round_id, event_type, round_version, event_metadata, available_at, attempt_count)
			VALUES ('33333333-3333-3333-3333-333333333333'::uuid, $1::uuid, '99999999-9999-9999-9999-999999999999'::uuid, 'queue.round_started', 1, '{}', NOW(), 0)`, "23503", []any{sessionID}},
	})
}

func TestRecitationQueueMigration_MemorizationProgressIntegrity(t *testing.T) {
	ctx := context.Background()
	pool := openPool(t, ctx)
	defer pool.Close()
	conn := acquireConn(t, pool, ctx)
	defer conn.Release()

	schema := uniqueSchemaName(t)
	createSchema(t, conn, ctx, schema)
	defer dropSchema(t, pool, ctx, schema)

	for _, migration := range recitationQueueHeadMigrations {
		runMigrationFile(t, conn, ctx, migration)
	}

	teacherID := seedLiveSessionUser(t, conn, ctx, "progress-teacher")
	circleID := seedLiveSessionCircle(t, conn, ctx, teacherID)
	sessionID := insertLiveSession(t, conn, ctx, circleID, teacherID, "")
	queueID := insertRecitationRound(t, conn, ctx, sessionID, teacherID, "active", 1)
	studentID := seedLiveSessionUser(t, conn, ctx, "progress-student")
	entryID := insertQueueEntry(t, conn, ctx, queueID, studentID, "completed", 1)

	assertColumnNotNull(t, conn, ctx, schema, "memorization_progress", "queue_entry_id")

	if _, err := conn.Exec(ctx, `
		INSERT INTO memorization_progress (student_id, circle_id, session_id, queue_entry_id, surah_id, surah_name, from_ayah, to_ayah, type, grade, date)
		VALUES ($1::uuid, $2::uuid, $3::uuid, $4::uuid, 1, 'Al-Fatihah', 1, 7, 'revision', 'excellent', CURRENT_DATE)
	`, studentID, circleID, sessionID, entryID); err != nil {
		t.Fatalf("insert progress row: %v", err)
	}

	expectViolations(t, conn, ctx, []sqlViolationCase{
		{"duplicate queue entry progress", `
			INSERT INTO memorization_progress (student_id, circle_id, session_id, queue_entry_id, surah_id, surah_name, from_ayah, to_ayah, type, grade, date)
			VALUES ($1::uuid, $2::uuid, $3::uuid, $4::uuid, 1, 'Al-Fatihah', 1, 7, 'revision', 'good', CURRENT_DATE)`, "23505", []any{studentID, circleID, sessionID, entryID}},
		{"bad round type", `
			INSERT INTO memorization_progress (student_id, circle_id, session_id, queue_entry_id, surah_id, surah_name, from_ayah, to_ayah, type, grade, date)
			VALUES ($1::uuid, $2::uuid, $3::uuid, $4::uuid, 1, 'Al-Fatihah', 1, 7, 'khatma', 'good', CURRENT_DATE)`, "23514", []any{studentID, circleID, sessionID, entryID}},
		{"bad grade", `
			INSERT INTO memorization_progress (student_id, circle_id, session_id, queue_entry_id, surah_id, surah_name, from_ayah, to_ayah, type, grade, date)
			VALUES ($1::uuid, $2::uuid, $3::uuid, $4::uuid, 1, 'Al-Fatihah', 1, 7, 'revision', 'mashallah', CURRENT_DATE)`, "23514", []any{studentID, circleID, sessionID, entryID}},
		// ADR-019: no silent history destruction via user deletion.
		{"hard delete user with progress", `DELETE FROM users WHERE id = $1::uuid`, "23503", []any{studentID}},
	})

	// ADR-019: every FK on memorization_progress uses default NO ACTION
	// (delete and update). pg_constraint is authoritative; the regclass cast
	// resolves via the test schema's search_path, and confdeltype/confupdtype
	// 'a' means NO ACTION. Filtered in SQL to avoid scanning the "char" type.
	var fkCount, noActionCount int
	if err := conn.QueryRow(ctx, `
		SELECT COUNT(*),
		       COUNT(*) FILTER (WHERE confdeltype = 'a' AND confupdtype = 'a')
		FROM pg_constraint
		WHERE conrelid = 'memorization_progress'::regclass
		  AND contype = 'f'
	`).Scan(&fkCount, &noActionCount); err != nil {
		t.Fatalf("query progress FK rules: %v", err)
	}
	if fkCount != 5 {
		t.Fatalf("memorization_progress FK count: got %d, want 5 (student, circle, session, queue_entry, surah)", fkCount)
	}
	if noActionCount != fkCount {
		t.Fatalf("memorization_progress NO ACTION FKs: got %d of %d, want all FKs NO ACTION on delete and update (ADR-019)", noActionCount, fkCount)
	}
}

func TestRecitationQueueMigration_SessionPolicyDefaultsAndChecks(t *testing.T) {
	ctx := context.Background()
	pool := openPool(t, ctx)
	defer pool.Close()
	conn := acquireConn(t, pool, ctx)
	defer conn.Release()

	schema := uniqueSchemaName(t)
	createSchema(t, conn, ctx, schema)
	defer dropSchema(t, pool, ctx, schema)

	for _, migration := range recitationQueueHeadMigrations {
		runMigrationFile(t, conn, ctx, migration)
	}

	userID := seedLiveSessionUser(t, conn, ctx, "policy-user")
	circleID := seedLiveSessionCircle(t, conn, ctx, userID)

	var population, finalization, optOut, visibility, correction string
	var version int64
	if err := conn.QueryRow(ctx, `
		INSERT INTO sessions (circle_id, created_by)
		VALUES ($1::uuid, $2::uuid)
		RETURNING queue_population_policy, queue_finalization_policy, queue_opt_out_policy,
			queue_grade_visibility, queue_grade_correction, queue_policy_version
	`, circleID, userID).Scan(&population, &finalization, &optOut, &visibility, &correction, &version); err != nil {
		t.Fatalf("insert session with policy defaults: %v", err)
	}
	if population != "present_at_activation" || finalization != "mark_unfinished_skipped" ||
		optOut != "approval_required" || visibility != "managers_and_student" ||
		correction != "audited_any_time" || version != 1 {
		t.Fatalf("unexpected session policy defaults: population=%q finalization=%q opt_out=%q visibility=%q correction=%q version=%d",
			population, finalization, optOut, visibility, correction, version)
	}

	expectViolations(t, conn, ctx, []sqlViolationCase{
		{"bad population policy", `
			INSERT INTO sessions (circle_id, created_by, queue_population_policy)
			VALUES ($1::uuid, $2::uuid, 'everyone')`, "23514", []any{circleID, userID}},
		{"bad finalization policy", `
			INSERT INTO sessions (circle_id, created_by, queue_finalization_policy)
			VALUES ($1::uuid, $2::uuid, 'delete_everything')`, "23514", []any{circleID, userID}},
		{"bad opt-out policy", `
			INSERT INTO sessions (circle_id, created_by, queue_opt_out_policy)
			VALUES ($1::uuid, $2::uuid, 'never')`, "23514", []any{circleID, userID}},
		{"bad grade visibility", `
			INSERT INTO sessions (circle_id, created_by, queue_grade_visibility)
			VALUES ($1::uuid, $2::uuid, 'nobody')`, "23514", []any{circleID, userID}},
		{"bad grade correction", `
			INSERT INTO sessions (circle_id, created_by, queue_grade_correction)
			VALUES ($1::uuid, $2::uuid, 'always_rewrite')`, "23514", []any{circleID, userID}},
		{"zero policy version", `
			INSERT INTO sessions (circle_id, created_by, queue_policy_version)
			VALUES ($1::uuid, $2::uuid, 0)`, "23514", []any{circleID, userID}},
	})
}

func TestRecitationQueueMigration_DownUpCyclePreservesLegacyData(t *testing.T) {
	ctx := context.Background()
	pool := openPool(t, ctx)
	defer pool.Close()
	conn := acquireConn(t, pool, ctx)
	defer conn.Release()

	schema := uniqueSchemaName(t)
	createSchema(t, conn, ctx, schema)
	defer dropSchema(t, pool, ctx, schema)

	for _, migration := range recitationQueueHeadMigrations {
		runMigrationFile(t, conn, ctx, migration)
	}

	userID := seedLiveSessionUser(t, conn, ctx, "down-up")
	circleID := seedLiveSessionCircle(t, conn, ctx, userID)
	sessionID := insertLiveSession(t, conn, ctx, circleID, userID, "")
	assertRecitationQueueTables(t, conn, ctx, true)

	runMigrationFile(t, conn, ctx, migration017Down)

	// Down removes only F-003 objects: tables and session policy columns.
	assertRecitationQueueTables(t, conn, ctx, false)
	for _, column := range sessionQueuePolicyColumns {
		assertColumnNotExists(t, conn, ctx, schema, "sessions", column)
	}

	// F-001/F-002/F-005 data survives the rollback.
	assertLegacyRowsIntact(t, conn, ctx, userID, circleID, sessionID)

	runMigrationFile(t, conn, ctx, migration017Up)

	assertRecitationQueueTables(t, conn, ctx, true)
	assertSurahSeedIntact(t, conn, ctx)
	assertLegacyRowsIntact(t, conn, ctx, userID, circleID, sessionID)

	// Re-up restores ADR-018 defaults on the pre-existing session row.
	var population string
	var version int64
	if err := conn.QueryRow(ctx, `
		SELECT queue_population_policy, queue_policy_version
		FROM sessions
		WHERE id = $1::uuid
	`, sessionID).Scan(&population, &version); err != nil {
		t.Fatalf("read restored session policy: %v", err)
	}
	if population != "present_at_activation" || version != 1 {
		t.Fatalf("restored session policy: got population=%q version=%d, want present_at_activation/1", population, version)
	}
}

// assertRecitationQueueTables verifies that every F-003 table exists (or none do).
func assertRecitationQueueTables(t *testing.T, conn *pgxpool.Conn, ctx context.Context, wantExist bool) {
	t.Helper()
	for _, table := range recitationQueueTables {
		var reg *string
		if err := conn.QueryRow(ctx, `SELECT to_regclass($1)`, table).Scan(&reg); err != nil {
			t.Fatalf("lookup table %s: %v", table, err)
		}
		if (reg != nil) != wantExist {
			t.Fatalf("table %s existence: got %v, want %v", table, reg != nil, wantExist)
		}
	}
}

// assertSurahSeedIntact verifies the 114-row Quran seed.
func assertSurahSeedIntact(t *testing.T, conn *pgxpool.Conn, ctx context.Context) {
	t.Helper()
	var count int
	if err := conn.QueryRow(ctx, `SELECT COUNT(*) FROM quran_surahs`).Scan(&count); err != nil {
		t.Fatalf("count surah seed: %v", err)
	}
	if count != 114 {
		t.Fatalf("surah seed count: got %d, want 114", count)
	}
}

func insertRecitationRound(t *testing.T, conn *pgxpool.Conn, ctx context.Context, sessionID, createdBy, lifecycle string, roundNumber int) string {
	t.Helper()
	var id string
	var version int64
	if err := conn.QueryRow(ctx, `
		INSERT INTO recitation_queue (session_id, round_number, round_type, surah_id, from_ayah, to_ayah, grading_required, lifecycle, created_by)
		VALUES ($1::uuid, $2, 'revision', 1, 1, 7, TRUE, $3, $4::uuid)
		RETURNING id::text, version
	`, sessionID, roundNumber, lifecycle, createdBy).Scan(&id, &version); err != nil {
		t.Fatalf("insert round %d (%s): %v", roundNumber, lifecycle, err)
	}
	if version != 1 {
		t.Fatalf("round %d default version: got %d, want 1", roundNumber, version)
	}
	return id
}

func insertQueueEntry(t *testing.T, conn *pgxpool.Conn, ctx context.Context, queueID, studentID, status string, position int) string {
	t.Helper()
	var id string
	if err := conn.QueryRow(ctx, `
		INSERT INTO recitation_queue_entries (queue_id, student_id, position, status)
		VALUES ($1::uuid, $2::uuid, $3, $4)
		RETURNING id::text
	`, queueID, studentID, position, status).Scan(&id); err != nil {
		t.Fatalf("insert queue entry position %d (%s): %v", position, status, err)
	}
	return id
}

func insertQueuePreorder(t *testing.T, conn *pgxpool.Conn, ctx context.Context, queueID, studentID, addedBy string, position int) {
	t.Helper()
	if _, err := conn.Exec(ctx, `
		INSERT INTO recitation_queue_preorder (queue_id, student_id, position, added_by)
		VALUES ($1::uuid, $2::uuid, $3, $4::uuid)
	`, queueID, studentID, position, addedBy); err != nil {
		t.Fatalf("insert preorder position %d: %v", position, err)
	}
}

// assertLegacyRowsIntact verifies F-001/F-002/F-005 rows survive an F-003 rollback.
func assertLegacyRowsIntact(t *testing.T, conn *pgxpool.Conn, ctx context.Context, userID, circleID, sessionID string) {
	t.Helper()
	var email string
	if err := conn.QueryRow(ctx, `SELECT email FROM users WHERE id = $1::uuid`, userID).Scan(&email); err != nil {
		t.Fatalf("read legacy user: %v", err)
	}
	if email != "down-up@example.com" {
		t.Fatalf("legacy user email changed: %q", email)
	}
	var name string
	if err := conn.QueryRow(ctx, `SELECT name FROM circles WHERE id = $1::uuid`, circleID).Scan(&name); err != nil {
		t.Fatalf("read legacy circle: %v", err)
	}
	if name != "Legacy Circle" {
		t.Fatalf("legacy circle name changed: %q", name)
	}
	var status string
	if err := conn.QueryRow(ctx, `SELECT status FROM sessions WHERE id = $1::uuid`, sessionID).Scan(&status); err != nil {
		t.Fatalf("read legacy session: %v", err)
	}
	if status != "scheduled" {
		t.Fatalf("legacy session status changed: %q", status)
	}
}

// expectViolations asserts each case fails with the expected PostgreSQL error code.
func expectViolations(t *testing.T, conn *pgxpool.Conn, ctx context.Context, cases []sqlViolationCase) {
	t.Helper()
	for _, tc := range cases {
		if _, err := conn.Exec(ctx, tc.sql, tc.args...); !isPgErrorCode(t, err, tc.code) {
			t.Fatalf("%s: got %v, want pg error %s", tc.name, err, tc.code)
		}
	}
}
