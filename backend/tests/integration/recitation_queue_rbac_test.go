//go:build integration

package integration

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/KarimMFadel/halaqaty/backend/internal/auth"
	"github.com/KarimMFadel/halaqaty/backend/internal/middleware"
	phttp "github.com/KarimMFadel/halaqaty/backend/internal/platform/http"
	"github.com/KarimMFadel/halaqaty/backend/internal/platform/httpconst"
	"github.com/KarimMFadel/halaqaty/backend/internal/queue"
	"github.com/KarimMFadel/halaqaty/backend/internal/sessions"
)

// T025 — RBAC denial coverage for the F-003 US1 queue management surface.
// Every manager operation is denied for unauthenticated callers, non-members,
// removed ("inactive") members, active students, and a session creator whose
// current role is no longer teacher/supervisor. Every denial asserts the
// standard error envelope AND zero state mutation across the queue tables
// (recitation_queue, recitation_queue_entries, recitation_queue_preorder,
// queue_command_receipts, queue_event_outbox — so no outbox rows and no
// receipts leak from rejected commands).
//
// The suite runs the production queue.Handler (T029 deliverable) behind the
// real auth middleware on a real router with real route patterns; Firebase is
// stubbed only at the token-verifier boundary. It is the red spec for T029:
// it does not build until queue.NewHandler exists.

type queueRBACEnv struct {
	mux             *http.ServeMux
	pool            *pgxpool.Pool
	repo            *queue.Repository
	userIDs         map[string]string
	tokens          map[string]string
	backendSessions map[string]string
	sessionID       string // live session created by the teacher (main session)
	creatorSession  string // scheduled session created by the later-demoted creator
	foreignSession  string // active other-circle session the teacher cannot manage
}

// rbacManagerOps returns one request builder per US1 manager operation against
// the given session. Request bodies read current committed versions so the
// only possible non-2xx answer for an authorized actor is success; for denied
// actors the role check must fire before any state handling.
type rbacOp struct {
	name string
	do   func(e *queueRBACEnv, t *testing.T, sessionID string) (method, target, body string)
}

var rbacOps = []rbacOp{
	{"create round", func(e *queueRBACEnv, _ *testing.T, sessionID string) (string, string, string) {
		return http.MethodPost, "/api/v1/sessions/" + sessionID + "/queue/rounds",
			`{"round_type":"revision","surah_id":1,"from_ayah":1,"to_ayah":7,"grading_required":false}`
	}},
	{"reorder queue", func(e *queueRBACEnv, t *testing.T, sessionID string) (string, string, string) {
		candidates := e.preparedCandidates(t, sessionID)
		version := e.roundVersion(t, sessionID, "prepared")
		ids := ""
		for i, studentID := range candidates {
			if i > 0 {
				ids += ","
			}
			ids += fmt.Sprintf("%q", studentID)
		}
		return http.MethodPut, "/api/v1/sessions/" + sessionID + "/queue/order",
			fmt.Sprintf(`{"ordered_ids":[%s],"expected_version":%d}`, ids, version)
	}},
	{"advance queue", func(e *queueRBACEnv, t *testing.T, sessionID string) (string, string, string) {
		return http.MethodPost, "/api/v1/sessions/" + sessionID + "/queue/advance",
			fmt.Sprintf(`{"expected_version":%d}`, e.roundVersion(t, sessionID, "active"))
	}},
	{"move entry", func(e *queueRBACEnv, t *testing.T, sessionID string) (string, string, string) {
		entryID, _ := e.entryRefFor(t, sessionID)
		return http.MethodPost, "/api/v1/sessions/" + sessionID + "/queue/entries/" + entryID + "/move",
			fmt.Sprintf(`{"new_position":1,"expected_version":%d}`, e.roundVersion(t, sessionID, "active"))
	}},
	{"update entry status", func(e *queueRBACEnv, t *testing.T, sessionID string) (string, string, string) {
		// expected_entry_version is the entry's own optimistic-lock version.
		entryID, entryVersion := e.entryRefFor(t, sessionID)
		return http.MethodPut, "/api/v1/sessions/" + sessionID + "/queue/entries/" + entryID + "/status",
			fmt.Sprintf(`{"status":"skipped","expected_entry_version":%d}`, entryVersion)
	}},
	{"reset queue", func(e *queueRBACEnv, t *testing.T, sessionID string) (string, string, string) {
		return http.MethodPost, "/api/v1/sessions/" + sessionID + "/queue/reset",
			fmt.Sprintf(`{"round_type":"old_revision","surah_id":2,"from_ayah":4,"to_ayah":8,"grading_required":false,"expected_version":%d}`, e.roundVersion(t, sessionID, "active"))
	}},
	{"update policy", func(e *queueRBACEnv, t *testing.T, sessionID string) (string, string, string) {
		return http.MethodPatch, "/api/v1/sessions/" + sessionID + "/queue/policy",
			fmt.Sprintf(`{"opt_out":"auto_approve","expected_version":%d}`, e.policyVersion(t, sessionID))
	}},
}

func setupQueueRBACEnv(t *testing.T) *queueRBACEnv {
	t.Helper()
	ctx := context.Background()

	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		t.Skip("DATABASE_URL is not set; T025 requires PostgreSQL via DATABASE_URL")
	}

	adminPool, err := pgxpool.New(ctx, dbURL)
	if err != nil {
		t.Fatalf("open admin pool: %v", err)
	}
	conn := acquireConn(t, adminPool, ctx)
	schema := uniqueSchemaName(t) + "_queue_rbac"
	createSchema(t, conn, ctx, schema)
	for _, file := range []string{
		"000010_auth_roles_profile.up.sql",
		"000011_auth_roles_profile_alignment.up.sql",
		"000012_auth_profiles_display_name.up.sql",
		"000013_create_circles.up.sql",
		"000014_circle_members_circle_fk.up.sql",
		"000015_circle_management.up.sql",
		"000016_live_sessions.up.sql",
		"000017_recitation_queue_system.up.sql",
	} {
		runMigrationFile(t, conn, ctx, file)
	}
	conn.Release()

	poolCfg, err := pgxpool.ParseConfig(dbURL)
	if err != nil {
		t.Fatalf("parse pool config: %v", err)
	}
	poolCfg.ConnConfig.RuntimeParams["search_path"] = schema
	pool, err := pgxpool.NewWithConfig(ctx, poolCfg)
	if err != nil {
		t.Fatalf("open schema pool: %v", err)
	}
	t.Cleanup(func() {
		pool.Close()
		dropSchema(t, adminPool, ctx, schema)
		adminPool.Close()
	})

	verifier := &circleTokenVerifier{tokens: make(map[string]*auth.DecodedToken)}
	for _, user := range []string{"teacher", "creator", "student", "outsider", "removed"} {
		verifier.tokens[user+"-token"] = &auth.DecodedToken{
			UID:   "firebase-queue-" + user,
			Email: user + "@halaqaty.app",
		}
	}

	sessionRepo := auth.NewSessionRepository(pool)
	authMW := middleware.NewAuthMiddleware(verifier, auth.NewSessionService(24*time.Hour), sessionRepo)
	authHandler := auth.NewHandler(auth.NewService(sessionRepo, nil, 24*time.Hour))

	repo := queue.NewQueueRepository(pool)
	handler := queue.NewHandler(repo,
		queue.NewRoundService(repo),
		queue.NewTurnService(repo),
		queue.NewPolicyService(repo),
	)

	mux := http.NewServeMux()
	mux.Handle("POST /auth/register", authMW.RequireVerifiedFirebase(http.HandlerFunc(authHandler.Register)))
	mux.Handle("GET /api/v1/sessions/{sessionId}/queue", authMW.Require(http.HandlerFunc(handler.GetQueue)))
	mux.Handle("POST /api/v1/sessions/{sessionId}/queue/rounds", authMW.Require(http.HandlerFunc(handler.CreateRound)))
	mux.Handle("POST /api/v1/sessions/{sessionId}/queue/reset", authMW.Require(http.HandlerFunc(handler.ResetQueue)))
	mux.Handle("POST /api/v1/sessions/{sessionId}/queue/advance", authMW.Require(http.HandlerFunc(handler.Advance)))
	mux.Handle("PUT /api/v1/sessions/{sessionId}/queue/order", authMW.Require(http.HandlerFunc(handler.Reorder)))
	mux.Handle("POST /api/v1/sessions/{sessionId}/queue/entries/{entryId}/move", authMW.Require(http.HandlerFunc(handler.MoveEntry)))
	mux.Handle("PUT /api/v1/sessions/{sessionId}/queue/entries/{entryId}/status", authMW.Require(http.HandlerFunc(handler.UpdateEntryStatus)))
	mux.Handle("PATCH /api/v1/sessions/{sessionId}/queue/policy", authMW.Require(http.HandlerFunc(handler.UpdatePolicy)))

	env := &queueRBACEnv{
		mux:             mux,
		pool:            pool,
		repo:            repo,
		userIDs:         make(map[string]string),
		tokens:          make(map[string]string),
		backendSessions: make(map[string]string),
	}
	for _, user := range []string{"teacher", "creator", "student", "outsider", "removed"} {
		env.registerQueueRBACUser(t, user)
	}

	// Circle, memberships, and a second circle where the outsider is a
	// teacher (being a manager elsewhere must not grant access here).
	var circleID, otherCircleID string
	if err := pool.QueryRow(ctx, `
		INSERT INTO circles (name, teacher_id, invite_code)
		VALUES ('T025 RBAC Circle', $1::uuid, 'HLQ-T025A')
		RETURNING id::text
	`, env.userIDs["teacher"]).Scan(&circleID); err != nil {
		t.Fatalf("seed circle: %v", err)
	}
	if err := pool.QueryRow(ctx, `
		INSERT INTO circles (name, teacher_id, invite_code)
		VALUES ('T025 Other Circle', $1::uuid, 'HLQ-T025B')
		RETURNING id::text
	`, env.userIDs["outsider"]).Scan(&otherCircleID); err != nil {
		t.Fatalf("seed other circle: %v", err)
	}
	for _, pair := range []struct{ user, role string }{
		{"teacher", "teacher"}, {"creator", "teacher"}, {"student", "student"}, {"removed", "teacher"},
	} {
		if _, err := pool.Exec(ctx, `
			INSERT INTO circle_members (circle_id, user_id, role)
			VALUES ($1::uuid, $2::uuid, $3)
		`, circleID, env.userIDs[pair.user], pair.role); err != nil {
			t.Fatalf("seed member %s: %v", pair.user, err)
		}
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO circle_members (circle_id, user_id, role)
		VALUES ($1::uuid, $2::uuid, 'teacher')
	`, otherCircleID, env.userIDs["outsider"]); err != nil {
		t.Fatalf("seed outsider membership: %v", err)
	}

	// Filler students give the main session's active round real entries.
	fillers := []string{env.seedQueueRBACUser(t, "filler-a"), env.seedQueueRBACUser(t, "filler-b")}
	for _, fillerID := range fillers {
		if _, err := pool.Exec(ctx, `
			INSERT INTO circle_members (circle_id, user_id, role)
			VALUES ($1::uuid, $2::uuid, 'student')
		`, circleID, fillerID); err != nil {
			t.Fatalf("seed filler student membership: %v", err)
		}
	}

	// Main session: created by the teacher, live, with an active round
	// (entries materialized for present students) and a stacked prepared round.
	if err := pool.QueryRow(ctx, `
		INSERT INTO sessions (circle_id, created_by)
		VALUES ($1::uuid, $2::uuid)
		RETURNING id::text
	`, circleID, env.userIDs["teacher"]).Scan(&env.sessionID); err != nil {
		t.Fatalf("insert main session: %v", err)
	}
	for _, present := range append([]string{env.userIDs["student"]}, fillers...) {
		if _, err := pool.Exec(ctx, `
			INSERT INTO session_participant_presence (session_id, user_id, first_joined_at, last_joined_at, is_currently_present)
			VALUES ($1::uuid, $2::uuid, NOW(), NOW(), TRUE)
		`, env.sessionID, present); err != nil {
			t.Fatalf("seed presence: %v", err)
		}
	}
	if _, err := sessions.NewSessionRepository(pool).StartSession(ctx, env.sessionID, sessions.MediaRoomRef("t025-rbac-room")); err != nil {
		t.Fatalf("start main session: %v", err)
	}
	rounds := queue.NewRoundService(repo)
	activeRound, err := rounds.Prepare(ctx, queue.PrepareRoundInput{
		SessionID: env.sessionID, Type: queue.RoundTypeRevision, SurahID: 1,
		FromAyah: 1, ToAyah: 7, SurahAyahCount: 7, GradingRequired: true,
		CreatedBy: env.userIDs["teacher"],
		Preorder:  []string{env.userIDs["student"], fillers[0], fillers[1]},
	})
	if err != nil {
		t.Fatalf("prepare active round: %v", err)
	}
	if _, err := rounds.Prepare(ctx, queue.PrepareRoundInput{
		SessionID: env.sessionID, Type: queue.RoundTypeTest, SurahID: 2,
		FromAyah: 1, ToAyah: 3, SurahAyahCount: 286, GradingRequired: false,
		CreatedBy: env.userIDs["teacher"],
		Preorder:  []string{fillers[1], fillers[0]},
	}); err != nil {
		t.Fatalf("prepare stacked round: %v", err)
	}
	// Setup guard: the active round must have materialized entries so the
	// entry-scoped denial paths target real resources.
	state, err := repo.LoadQueueState(ctx, activeRound.ID, queue.Viewer{UserID: env.userIDs["teacher"], IsManager: true})
	if err != nil {
		t.Fatalf("load active round state: %v", err)
	}
	if len(state.Entries) == 0 {
		t.Fatal("active round must have materialized entries for the denial paths")
	}

	// Creator session: created and prepared by the creator while still a
	// teacher; the creator is demoted afterwards.
	if err := pool.QueryRow(ctx, `
		INSERT INTO sessions (circle_id, created_by)
		VALUES ($1::uuid, $2::uuid)
		RETURNING id::text
	`, circleID, env.userIDs["creator"]).Scan(&env.creatorSession); err != nil {
		t.Fatalf("insert creator session: %v", err)
	}
	if _, err := rounds.Prepare(ctx, queue.PrepareRoundInput{
		SessionID: env.creatorSession, Type: queue.RoundTypeRevision, SurahID: 1,
		FromAyah: 1, ToAyah: 7, SurahAyahCount: 7, GradingRequired: false,
		CreatedBy: env.userIDs["creator"],
		Preorder:  []string{env.userIDs["student"], fillers[0]},
	}); err != nil {
		t.Fatalf("prepare creator round: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		UPDATE circle_members SET role = 'student'
		WHERE circle_id = $1::uuid AND user_id = $2::uuid
	`, circleID, env.userIDs["creator"]); err != nil {
		t.Fatalf("demote creator: %v", err)
	}

	// The foreign active round belongs to another circle. Its entries exercise
	// the session-path ownership boundary for entry mutations.
	for _, fillerID := range fillers {
		if _, err := pool.Exec(ctx, `
			INSERT INTO circle_members (circle_id, user_id, role)
			VALUES ($1::uuid, $2::uuid, 'student')
		`, otherCircleID, fillerID); err != nil {
			t.Fatalf("seed foreign filler membership: %v", err)
		}
	}
	if err := pool.QueryRow(ctx, `
		INSERT INTO sessions (circle_id, created_by)
		VALUES ($1::uuid, $2::uuid)
		RETURNING id::text
	`, otherCircleID, env.userIDs["outsider"]).Scan(&env.foreignSession); err != nil {
		t.Fatalf("insert foreign session: %v", err)
	}
	for _, fillerID := range fillers {
		if _, err := pool.Exec(ctx, `
			INSERT INTO session_participant_presence (session_id, user_id, first_joined_at, last_joined_at, is_currently_present)
			VALUES ($1::uuid, $2::uuid, NOW(), NOW(), TRUE)
		`, env.foreignSession, fillerID); err != nil {
			t.Fatalf("seed foreign presence: %v", err)
		}
	}
	if _, err := sessions.NewSessionRepository(pool).StartSession(ctx, env.foreignSession, sessions.MediaRoomRef("t025-foreign-room")); err != nil {
		t.Fatalf("start foreign session: %v", err)
	}
	if _, err := rounds.Prepare(ctx, queue.PrepareRoundInput{
		SessionID: env.foreignSession, Type: queue.RoundTypeRevision, SurahID: 1,
		FromAyah: 1, ToAyah: 7, SurahAyahCount: 7, GradingRequired: false,
		CreatedBy: env.userIDs["outsider"], Preorder: fillers,
	}); err != nil {
		t.Fatalf("prepare foreign round: %v", err)
	}

	return env
}

func (e *queueRBACEnv) registerQueueRBACUser(t *testing.T, user string) {
	t.Helper()
	token := "Bearer " + user + "-token"
	resp := doJSONRequest(t, e.mux, http.MethodPost, "/auth/register", `{"display_name":"`+user+`"}`, map[string]string{
		httpconst.HeaderAuthorization: token,
		httpconst.HeaderContentType:   httpconst.ContentTypeApplicationJSON,
	})
	if resp.Code != http.StatusCreated {
		t.Fatalf("register %s: got %d body=%s", user, resp.Code, resp.Body.String())
	}
	var session auth.BackendSessionResponse
	if err := json.Unmarshal(resp.Body.Bytes(), &session); err != nil {
		t.Fatalf("decode register response for %s: %v", user, err)
	}
	e.tokens[user] = token
	e.userIDs[user] = session.User.ID
	e.backendSessions[user] = session.SessionID
}

func (e *queueRBACEnv) seedQueueRBACUser(t *testing.T, label string) string {
	t.Helper()
	var id string
	if err := e.pool.QueryRow(context.Background(), `
		INSERT INTO users (firebase_uid, email)
		VALUES ($1, $2)
		RETURNING id::text
	`, "firebase-queue-"+label, label+"@example.com").Scan(&id); err != nil {
		t.Fatalf("seed user %s: %v", label, err)
	}
	return id
}

// queueTablesSnapshot counts every queue table row scoped to the session.
// recitation_queue_entries and recitation_queue_preorder have no session_id
// column, so they are scoped through their round.
func (e *queueRBACEnv) queueTablesSnapshot(t *testing.T, sessionID string) [5]int {
	t.Helper()
	var snapshot [5]int
	err := e.pool.QueryRow(context.Background(), `
		SELECT
			(SELECT COUNT(*) FROM recitation_queue WHERE session_id = $1::uuid),
			(SELECT COUNT(*) FROM recitation_queue_entries
			 WHERE queue_id IN (SELECT id FROM recitation_queue WHERE session_id = $1::uuid)),
			(SELECT COUNT(*) FROM recitation_queue_preorder
			 WHERE queue_id IN (SELECT id FROM recitation_queue WHERE session_id = $1::uuid)),
			(SELECT COUNT(*) FROM queue_command_receipts WHERE session_id = $1::uuid),
			(SELECT COUNT(*) FROM queue_event_outbox WHERE session_id = $1::uuid)
	`, sessionID).Scan(&snapshot[0], &snapshot[1], &snapshot[2], &snapshot[3], &snapshot[4])
	if err != nil {
		t.Fatalf("snapshot queue tables for %s: %v", sessionID, err)
	}
	return snapshot
}

// roundVersion returns the committed version of the session's latest round
// with the given lifecycle. Returns 1 when no round matches so denied
// requests stay well-formed.
func (e *queueRBACEnv) roundVersion(t *testing.T, sessionID, lifecycle string) int64 {
	t.Helper()
	var version int64
	if err := e.pool.QueryRow(context.Background(), `
		SELECT version FROM recitation_queue
		WHERE session_id = $1::uuid AND lifecycle = $2
		ORDER BY round_number DESC LIMIT 1
	`, sessionID, lifecycle).Scan(&version); err != nil {
		return 1
	}
	return version
}

// preparedCandidates lists the pre-set candidate student IDs of the first
// prepared round (deterministic while exactly one prepared round exists).
func (e *queueRBACEnv) preparedCandidates(t *testing.T, sessionID string) []string {
	t.Helper()
	rows, err := e.pool.Query(context.Background(), `
		SELECT p.student_id::text
		FROM recitation_queue_preorder p
		JOIN recitation_queue q ON q.id = p.queue_id
		WHERE q.session_id = $1::uuid AND q.lifecycle = 'prepared'
		ORDER BY q.round_number, p.position
	`, sessionID)
	if err != nil {
		t.Fatalf("load prepared candidates: %v", err)
	}
	defer rows.Close()
	var candidates []string
	for rows.Next() {
		var studentID string
		if err := rows.Scan(&studentID); err != nil {
			t.Fatalf("scan prepared candidate: %v", err)
		}
		candidates = append(candidates, studentID)
	}
	if len(candidates) == 0 {
		candidates = []string{"00000000-0000-0000-0000-000000000000"}
	}
	return candidates
}

// entryRefFor returns a real entry of the session's active round and its
// current optimistic-lock version, falling back to a well-formed unknown UUID
// with version 1 for sessions without entries — the role check must reject
// before any resource lookup either way.
func (e *queueRBACEnv) entryRefFor(t *testing.T, sessionID string) (string, int64) {
	t.Helper()
	var entryID string
	var version int64
	err := e.pool.QueryRow(context.Background(), `
		SELECT e.id::text, e.version
		FROM recitation_queue_entries e
		JOIN recitation_queue q ON q.id = e.queue_id
		WHERE q.session_id = $1::uuid
		ORDER BY e.position
		LIMIT 1
	`, sessionID).Scan(&entryID, &version)
	if err != nil {
		return "00000000-0000-0000-0000-000000000000", 1
	}
	return entryID, version
}

func (e *queueRBACEnv) entrySnapshot(t *testing.T, entryID string) (position int, status string, version int64) {
	t.Helper()
	if err := e.pool.QueryRow(context.Background(), `
		SELECT position, status, version
		FROM recitation_queue_entries
		WHERE id = $1::uuid
	`, entryID).Scan(&position, &status, &version); err != nil {
		t.Fatalf("snapshot entry %s: %v", entryID, err)
	}
	return position, status, version
}

func (e *queueRBACEnv) policyVersion(t *testing.T, sessionID string) int64 {
	t.Helper()
	var version int64
	if err := e.pool.QueryRow(context.Background(),
		`SELECT queue_policy_version FROM sessions WHERE id = $1::uuid`, sessionID).Scan(&version); err != nil {
		t.Fatalf("load policy version: %v", err)
	}
	return version
}

// assertAllManagerOpsDenied fires every manager operation as the given actor
// and asserts the expected status, the standard error envelope, and zero
// queue-table mutation after each attempt. An empty token omits the
// Authorization header entirely; an actor without a backend session sends no
// session header.
func (e *queueRBACEnv) assertAllManagerOpsDenied(t *testing.T, sessionID, actorLabel, actorToken string, wantStatus int, wantCode string) {
	t.Helper()
	for _, op := range rbacOps {
		t.Run(op.name, func(t *testing.T) {
			before := e.queueTablesSnapshot(t, sessionID)
			method, target, body := op.do(e, t, sessionID)
			headers := map[string]string{
				httpconst.HeaderContentType: httpconst.ContentTypeApplicationJSON,
			}
			if actorToken != "" {
				headers[httpconst.HeaderAuthorization] = actorToken
			}
			if sessionIDValue, ok := e.backendSessions[actorLabel]; ok {
				headers[httpconst.HeaderSessionID] = sessionIDValue
			}
			resp := doJSONRequest(t, e.mux, method, target, body, headers)
			if resp.Code != wantStatus {
				t.Fatalf("%s: got %d want %d body=%s", op.name, resp.Code, wantStatus, resp.Body.String())
			}
			var envl phttp.ErrorEnvelope
			if err := json.Unmarshal(resp.Body.Bytes(), &envl); err != nil {
				t.Fatalf("%s: decode error envelope: %v body=%s", op.name, err, resp.Body.String())
			}
			if envl.Error.Code != wantCode {
				t.Fatalf("%s: error code = %q, want %q", op.name, envl.Error.Code, wantCode)
			}
			if envl.Error.Message == "" {
				t.Fatalf("%s: error message must not be empty", op.name)
			}
			if after := e.queueTablesSnapshot(t, sessionID); after != before {
				t.Fatalf("%s: denied request mutated queue tables: before=%v after=%v", op.name, before, after)
			}
		})
	}
}

func TestRecitationQueueRBAC(t *testing.T) {
	env := setupQueueRBACEnv(t)

	t.Run("unauthenticated callers receive 401 for every manager operation", func(t *testing.T) {
		env.assertAllManagerOpsDenied(t, env.sessionID, "", "", http.StatusUnauthorized, httpconst.ErrorCodeUnauthorized)
	})

	t.Run("invalid bearer tokens receive 401 for every manager operation", func(t *testing.T) {
		env.assertAllManagerOpsDenied(t, env.sessionID, "", "Bearer rbac-invalid-token", http.StatusUnauthorized, httpconst.ErrorCodeUnauthorized)
	})

	t.Run("non-members receive 403 for every manager operation", func(t *testing.T) {
		// The outsider is a teacher in another circle; a management role
		// elsewhere must not authorize this session's circle.
		env.assertAllManagerOpsDenied(t, env.sessionID, "outsider", env.tokens["outsider"], http.StatusForbidden, httpconst.ErrorCodeForbidden)
	})

	t.Run("removed members receive 403 for every manager operation", func(t *testing.T) {
		// circle_members has no inactive/retired status column (migrations
		// 000010-000017); removal from circle_members is the existing
		// inactive-membership concept, so exercise it directly.
		ctx := context.Background()
		if _, err := env.pool.Exec(ctx, `
			DELETE FROM circle_members
			WHERE user_id = $1::uuid
		`, env.userIDs["removed"]); err != nil {
			t.Fatalf("remove member: %v", err)
		}
		env.assertAllManagerOpsDenied(t, env.sessionID, "removed", env.tokens["removed"], http.StatusForbidden, httpconst.ErrorCodeForbidden)
	})

	t.Run("students receive 403 for every manager operation", func(t *testing.T) {
		env.assertAllManagerOpsDenied(t, env.sessionID, "student", env.tokens["student"], http.StatusForbidden, httpconst.ErrorCodeForbidden)
	})

	t.Run("demoted session creator receives 403 for every manager operation", func(t *testing.T) {
		// The creator seeded this session and its round while still a
		// teacher; the current student role must outrank creator identity.
		env.assertAllManagerOpsDenied(t, env.creatorSession, "creator", env.tokens["creator"], http.StatusForbidden, httpconst.ErrorCodeForbidden)
	})

	t.Run("manager cannot mutate a foreign-session entry through own session path", func(t *testing.T) {
		foreignEntryID, foreignEntryVersion := env.entryRefFor(t, env.foreignSession)
		foreignRoundVersion := env.roundVersion(t, env.foreignSession, "active")
		headers := map[string]string{
			httpconst.HeaderAuthorization: env.tokens["teacher"],
			httpconst.HeaderSessionID:     env.backendSessions["teacher"],
			httpconst.HeaderContentType:   httpconst.ContentTypeApplicationJSON,
		}
		for _, attempt := range []struct {
			name, method, suffix, body string
		}{
			{"move", http.MethodPost, "/move", fmt.Sprintf(`{"new_position":1,"expected_version":%d}`, foreignRoundVersion)},
			{"status", http.MethodPut, "/status", fmt.Sprintf(`{"status":"skipped","expected_entry_version":%d}`, foreignEntryVersion)},
		} {
			t.Run(attempt.name, func(t *testing.T) {
				beforePosition, beforeStatus, beforeVersion := env.entrySnapshot(t, foreignEntryID)
				beforeOwn := env.queueTablesSnapshot(t, env.sessionID)
				beforeForeign := env.queueTablesSnapshot(t, env.foreignSession)
				resp := doJSONRequest(t, env.mux, attempt.method,
					"/api/v1/sessions/"+env.sessionID+"/queue/entries/"+foreignEntryID+attempt.suffix,
					attempt.body, headers)
				if resp.Code != http.StatusNotFound {
					t.Fatalf("foreign %s: got %d want 404 body=%s", attempt.name, resp.Code, resp.Body.String())
				}
				afterPosition, afterStatus, afterVersion := env.entrySnapshot(t, foreignEntryID)
				if got, want := [3]any{afterPosition, afterStatus, afterVersion}, [3]any{beforePosition, beforeStatus, beforeVersion}; got != want {
					t.Fatalf("foreign %s mutated entry: before=%v after=%v", attempt.name, want, got)
				}
				if after := env.queueTablesSnapshot(t, env.sessionID); after != beforeOwn {
					t.Fatalf("foreign %s changed own-session queue rows: before=%v after=%v", attempt.name, beforeOwn, after)
				}
				if after := env.queueTablesSnapshot(t, env.foreignSession); after != beforeForeign {
					t.Fatalf("foreign %s changed foreign-session queue rows: before=%v after=%v", attempt.name, beforeForeign, after)
				}
			})
		}
	})

	t.Run("teacher happy-path control proves the 403s are RBAC", func(t *testing.T) {
		// Order matters: reorder runs while exactly one prepared round
		// exists; the move control must target a waiting entry, so it runs
		// before the skip control makes the first entry terminal; reset runs
		// last because it finalizes the active round.
		controlOrder := []string{
			"reorder queue", "create round", "advance queue",
			"move entry", "update entry status", "update policy", "reset queue",
		}
		for _, name := range controlOrder {
			var op rbacOp
			for _, candidate := range rbacOps {
				if candidate.name == name {
					op = candidate
					break
				}
			}
			t.Run(name, func(t *testing.T) {
				method, target, body := op.do(env, t, env.sessionID)
				resp := doJSONRequest(t, env.mux, method, target, body, map[string]string{
					httpconst.HeaderAuthorization: env.tokens["teacher"],
					httpconst.HeaderSessionID:     env.backendSessions["teacher"],
					httpconst.HeaderContentType:   httpconst.ContentTypeApplicationJSON,
				})
				if resp.Code < 200 || resp.Code > 299 {
					t.Fatalf("%s as teacher: got %d want 2xx body=%s", name, resp.Code, resp.Body.String())
				}
			})
		}
	})
}
