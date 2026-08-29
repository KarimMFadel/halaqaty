//go:build contract

package contract

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
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

// T024 — behavioral REST contract tests for the F-003 US1 queue management
// surface, pinned to specs/003-recitation-queue-system/contracts/
// recitation-queue.openapi.yaml (US1 endpoints only; opt-out, grade, and
// decision endpoints are T041/T053 and out of scope here).
//
// The suite runs the production queue.Handler (T029 deliverable) against real
// PostgreSQL, so it is the red spec for T029: it does not build until
// queue.NewHandler exists with the constructor and method surface referenced
// below. Queue services are DB-bound (*queue.Repository), hence real
// PostgreSQL with a fresh schema per test.
//
// Handler API surface specified for T029:
//
//	queue.NewHandler(repo *queue.Repository, rounds *queue.RoundService,
//		turns *queue.TurnService, policies *queue.PolicyService) *queue.Handler
//
// with one exported http.HandlerFunc-style method per US1 operation:
// GetQueue, CreateRound, ResetQueue, Advance, Reorder, MoveEntry,
// UpdateEntryStatus, UpdatePolicy.

// rqcIdempotencyKey: no httpconst constant exists for this header yet; the
// canonical name comes from the F-003 contract (Idempotency-Key parameter).
const rqcIdempotencyKey = "Idempotency-Key"

var rqcMigrations = []string{
	"000010_auth_roles_profile.up.sql",
	"000011_auth_roles_profile_alignment.up.sql",
	"000012_auth_profiles_display_name.up.sql",
	"000013_create_circles.up.sql",
	"000014_circle_members_circle_fk.up.sql",
	"000015_circle_management.up.sql",
	"000016_live_sessions.up.sql",
	"000017_recitation_queue_system.up.sql",
}

type rqcEnv struct {
	mux       *http.ServeMux
	pool      *pgxpool.Pool
	teacherID string
	superID   string
	students  []string
}

// ponytail: schema helpers duplicated from tests/integration (unexported
// there, different build-tagged package). Consolidate into a shared testutil
// package when a third suite needs them.
func rqcAcquireConn(t *testing.T, pool *pgxpool.Pool, ctx context.Context) *pgxpool.Conn {
	t.Helper()
	conn, err := pool.Acquire(ctx)
	if err != nil {
		t.Fatalf("acquire connection: %v", err)
	}
	return conn
}

func rqcUniqueSchemaName(t *testing.T) string {
	t.Helper()
	return fmt.Sprintf("test_rq_contract_%d", time.Now().UnixNano())
}

func rqcCreateSchema(t *testing.T, conn *pgxpool.Conn, ctx context.Context, schema string) {
	t.Helper()
	if _, err := conn.Exec(ctx, fmt.Sprintf("CREATE SCHEMA IF NOT EXISTS %s", schema)); err != nil {
		t.Fatalf("create schema: %v", err)
	}
	if _, err := conn.Exec(ctx, fmt.Sprintf("SET search_path TO %s", schema)); err != nil {
		t.Fatalf("set search_path: %v", err)
	}
}

func rqcDropSchema(t *testing.T, pool *pgxpool.Pool, ctx context.Context, schema string) {
	t.Helper()
	dropCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	_, _ = pool.Exec(dropCtx, fmt.Sprintf("DROP SCHEMA IF EXISTS %s CASCADE", schema))
}

func rqcRunMigrationFile(t *testing.T, conn *pgxpool.Conn, ctx context.Context, filename string) {
	t.Helper()
	_, currentFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("could not determine test file path")
	}
	path := filepath.Join(filepath.Dir(currentFile), "..", "..", "migrations", filename)
	sqlBytes, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read migration file %s: %v", filename, err)
	}
	if _, err := conn.Exec(ctx, string(sqlBytes)); err != nil {
		t.Fatalf("execute migration %s: %v", filename, err)
	}
}

func setupRqcEnv(t *testing.T) *rqcEnv {
	t.Helper()
	ctx := context.Background()

	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		t.Skip("DATABASE_URL is not set; T024 contract tests require PostgreSQL")
	}

	adminPool, err := pgxpool.New(ctx, dbURL)
	if err != nil {
		t.Fatalf("open admin pool: %v", err)
	}
	conn := rqcAcquireConn(t, adminPool, ctx)
	schema := rqcUniqueSchemaName(t)
	rqcCreateSchema(t, conn, ctx, schema)
	for _, file := range rqcMigrations {
		rqcRunMigrationFile(t, conn, ctx, file)
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
		rqcDropSchema(t, adminPool, ctx, schema)
		adminPool.Close()
	})

	repo := queue.NewQueueRepository(pool)
	handler := queue.NewHandler(repo,
		queue.NewRoundService(repo),
		queue.NewTurnService(repo),
		queue.NewPolicyService(repo),
	)

	env := &rqcEnv{mux: rqcMux(handler), pool: pool}
	env.teacherID = rqcSeedUser(t, pool, "rqc-teacher", "Ustadh Teacher")
	env.superID = rqcSeedUser(t, pool, "rqc-supervisor", "Ustadh Supervisor")
	for i, name := range []string{"Talib One", "Talib Two", "Talib Three"} {
		env.students = append(env.students, rqcSeedUser(t, pool, fmt.Sprintf("rqc-student-%d", i), name))
	}
	circleID := rqcSeedCircle(t, pool, env.teacherID)
	rqcSeedMember(t, pool, circleID, env.teacherID, "teacher")
	rqcSeedMember(t, pool, circleID, env.superID, "supervisor")
	for _, studentID := range env.students {
		rqcSeedMember(t, pool, circleID, studentID, "student")
	}
	return env
}

func rqcMux(h *queue.Handler) *http.ServeMux {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/sessions/{sessionId}/queue", h.GetQueue)
	mux.HandleFunc("POST /api/v1/sessions/{sessionId}/queue/rounds", h.CreateRound)
	mux.HandleFunc("POST /api/v1/sessions/{sessionId}/queue/reset", h.ResetQueue)
	mux.HandleFunc("POST /api/v1/sessions/{sessionId}/queue/advance", h.Advance)
	mux.HandleFunc("PUT /api/v1/sessions/{sessionId}/queue/order", h.Reorder)
	mux.HandleFunc("POST /api/v1/sessions/{sessionId}/queue/entries/{entryId}/move", h.MoveEntry)
	mux.HandleFunc("PUT /api/v1/sessions/{sessionId}/queue/entries/{entryId}/status", h.UpdateEntryStatus)
	mux.HandleFunc("PATCH /api/v1/sessions/{sessionId}/queue/policy", h.UpdatePolicy)
	return mux
}

func rqcSeedUser(t *testing.T, pool *pgxpool.Pool, label, displayName string) string {
	t.Helper()
	ctx := context.Background()
	var id string
	if err := pool.QueryRow(ctx, `
		INSERT INTO users (firebase_uid, email)
		VALUES ($1, $2)
		RETURNING id::text
	`, "firebase-"+label, label+"@example.com").Scan(&id); err != nil {
		t.Fatalf("seed user %s: %v", label, err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO profiles (user_id, display_name)
		VALUES ($1::uuid, $2)
	`, id, displayName); err != nil {
		t.Fatalf("seed profile %s: %v", label, err)
	}
	return id
}

func rqcSeedCircle(t *testing.T, pool *pgxpool.Pool, teacherID string) string {
	t.Helper()
	var id string
	if err := pool.QueryRow(context.Background(), `
		INSERT INTO circles (name, teacher_id, invite_code)
		VALUES ('T024 Contract Circle', $1::uuid, 'HLQ-T024')
		RETURNING id::text
	`, teacherID).Scan(&id); err != nil {
		t.Fatalf("seed circle: %v", err)
	}
	return id
}

func rqcSeedMember(t *testing.T, pool *pgxpool.Pool, circleID, userID, role string) {
	t.Helper()
	if _, err := pool.Exec(context.Background(), `
		INSERT INTO circle_members (circle_id, user_id, role)
		VALUES ($1::uuid, $2::uuid, $3)
	`, circleID, userID, role); err != nil {
		t.Fatalf("seed member %s (%s): %v", userID, role, err)
	}
}

func rqcSeedPresence(t *testing.T, pool *pgxpool.Pool, sessionID, userID string) {
	t.Helper()
	if _, err := pool.Exec(context.Background(), `
		INSERT INTO session_participant_presence (session_id, user_id, first_joined_at, last_joined_at, is_currently_present)
		VALUES ($1::uuid, $2::uuid, NOW(), NOW(), TRUE)
	`, sessionID, userID); err != nil {
		t.Fatalf("seed presence %s: %v", userID, err)
	}
}

// newSession inserts a session; when live it is started through the
// production sessions repository so the queue activation invariant applies.
func (e *rqcEnv) newSession(t *testing.T, live bool) string {
	t.Helper()
	ctx := context.Background()
	var sessionID string
	if err := e.pool.QueryRow(ctx, `
		INSERT INTO sessions (circle_id, created_by)
		VALUES (
			(SELECT id FROM circles WHERE invite_code = 'HLQ-T024'),
			$1::uuid
		)
		RETURNING id::text
	`, e.teacherID).Scan(&sessionID); err != nil {
		t.Fatalf("insert session: %v", err)
	}
	if live {
		started, err := sessions.NewSessionRepository(e.pool).StartSession(ctx, sessionID, sessions.MediaRoomRef("rqc-contract-room"))
		if err != nil {
			t.Fatalf("start session: %v", err)
		}
		if started.Status != sessions.SessionStatusActive {
			t.Fatalf("session status = %q, want active", started.Status)
		}
	}
	return sessionID
}

// liveRound drives POST rounds over HTTP on a live session with the three
// students present and pre-ordered; the activation invariant materializes
// three waiting entries in preorder order.
func (e *rqcEnv) liveRound(t *testing.T) (string, map[string]any) {
	t.Helper()
	sessionID := e.newSession(t, true)
	for _, studentID := range e.students {
		rqcSeedPresence(t, e.pool, sessionID, studentID)
	}
	body := fmt.Sprintf(`{"round_type":"revision","surah_id":1,"from_ayah":1,"to_ayah":7,"grading_required":true,"student_order":[%q,%q,%q]}`,
		e.students[0], e.students[1], e.students[2])
	rec := e.req(t, e.teacherID, http.MethodPost, "/api/v1/sessions/"+sessionID+"/queue/rounds", body, "")
	if rec.Code != http.StatusCreated {
		t.Fatalf("prepare live round: got %d body=%s", rec.Code, rec.Body.String())
	}
	return sessionID, rqcDecode(t, rec)
}

func (e *rqcEnv) req(t *testing.T, actorID, method, target, body, idemKey string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, target, strings.NewReader(body))
	req.Header.Set(httpconst.HeaderContentType, httpconst.ContentTypeApplicationJSON)
	if idemKey != "" {
		req.Header.Set(rqcIdempotencyKey, idemKey)
	}
	req = req.WithContext(auth.WithPrincipal(req.Context(), auth.AuthPrincipal{UserID: actorID}))
	rec := httptest.NewRecorder()
	e.mux.ServeHTTP(rec, req)
	return rec
}

func rqcDecode(t *testing.T, rec *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &m); err != nil {
		t.Fatalf("decode JSON response: %v body=%s", err, rec.Body.String())
	}
	return m
}

func rqcDecodeError(t *testing.T, rec *httptest.ResponseRecorder) phttp.ErrorEnvelope {
	t.Helper()
	var env phttp.ErrorEnvelope
	if err := json.Unmarshal(rec.Body.Bytes(), &env); err != nil {
		t.Fatalf("decode error envelope: %v body=%s", err, rec.Body.String())
	}
	return env
}

func rqcStr(t *testing.T, m map[string]any, key string) string {
	t.Helper()
	value, ok := m[key].(string)
	if !ok {
		t.Fatalf("field %q is not a string: %v", key, m[key])
	}
	return value
}

func rqcNum(t *testing.T, m map[string]any, key string) float64 {
	t.Helper()
	value, ok := m[key].(float64)
	if !ok {
		t.Fatalf("field %q is not a number: %v", key, m[key])
	}
	return value
}

func rqcObjects(t *testing.T, m map[string]any, key string) []map[string]any {
	t.Helper()
	raw, ok := m[key].([]any)
	if !ok {
		t.Fatalf("field %q is not an array: %v", key, m[key])
	}
	out := make([]map[string]any, 0, len(raw))
	for _, item := range raw {
		obj, ok := item.(map[string]any)
		if !ok {
			t.Fatalf("%q item is not an object: %v", key, item)
		}
		out = append(out, obj)
	}
	return out
}

// rqcAssertQueueState pins every required QueueState field of the F-003
// contract, including the embedded policy projection.
func rqcAssertQueueState(t *testing.T, state map[string]any) {
	t.Helper()
	for _, field := range []string{
		"session_id", "round_id", "round_number", "round_type", "lifecycle",
		"surah_id", "from_ayah", "to_ayah", "grading_required", "version",
		"policy", "preorder", "entries",
	} {
		if _, ok := state[field]; !ok {
			t.Errorf("QueueState missing required field %q", field)
		}
	}
	if t.Failed() {
		t.Fatalf("incomplete QueueState: %v", state)
	}
	policy, ok := state["policy"].(map[string]any)
	if !ok {
		t.Fatalf("policy is not an object: %v", state["policy"])
	}
	for _, field := range []string{
		"population", "unfinished_finalization", "opt_out",
		"grade_visibility", "grade_correction", "version",
	} {
		if _, ok := policy[field]; !ok {
			t.Errorf("QueuePolicy missing required field %q", field)
		}
	}
	if t.Failed() {
		t.Fatalf("incomplete QueuePolicy: %v", policy)
	}
}

func rqcAssertEntries(t *testing.T, state map[string]any, wantStatus string, wantCount int) []map[string]any {
	t.Helper()
	entries := rqcObjects(t, state, "entries")
	if len(entries) != wantCount {
		t.Fatalf("entries count = %d, want %d", len(entries), wantCount)
	}
	for i, entry := range entries {
		for _, field := range []string{"id", "student_id", "student_name", "position", "status", "version"} {
			if _, ok := entry[field]; !ok {
				t.Fatalf("entry %d missing required field %q: %v", i, field, entry)
			}
		}
		if name := rqcStr(t, entry, "student_name"); name == "" {
			t.Fatalf("entry %d has an empty student_name", i)
		}
		if got := rqcNum(t, entry, "position"); got != float64(i+1) {
			t.Fatalf("entry %d position = %v, want %d", i, got, i+1)
		}
		if got := rqcStr(t, entry, "status"); got != wantStatus {
			t.Fatalf("entry %d status = %q, want %q", i, got, wantStatus)
		}
	}
	return entries
}

// rqcEntryVersion returns the entry's own optimistic-lock version from a
// QueueState — the token `expected_entry_version` must carry.
func rqcEntryVersion(t *testing.T, state map[string]any, entryID string) float64 {
	t.Helper()
	for _, entry := range rqcObjects(t, state, "entries") {
		if rqcStr(t, entry, "id") == entryID {
			return rqcNum(t, entry, "version")
		}
	}
	t.Fatalf("entry %s not present in state", entryID)
	return 0
}

func TestRecitationQueueManagement_QueueSnapshotAndCreateRound(t *testing.T) {
	env := setupRqcEnv(t)
	sessionID := env.newSession(t, false)

	t.Run("create round while scheduled returns 201 prepared state", func(t *testing.T) {
		body := fmt.Sprintf(`{"round_type":"new_memorization","surah_id":1,"from_ayah":1,"to_ayah":7,"grading_required":true,"student_order":[%q,%q,%q]}`,
			env.students[1], env.students[2], env.students[0])
		rec := env.req(t, env.teacherID, http.MethodPost, "/api/v1/sessions/"+sessionID+"/queue/rounds", body, "")
		if rec.Code != http.StatusCreated {
			t.Fatalf("status: got %d want 201 body=%s", rec.Code, rec.Body.String())
		}
		state := rqcDecode(t, rec)
		rqcAssertQueueState(t, state)
		if got := rqcStr(t, state, "session_id"); got != sessionID {
			t.Fatalf("session_id = %q, want %q", got, sessionID)
		}
		if got := rqcStr(t, state, "lifecycle"); got != "prepared" {
			t.Fatalf("lifecycle = %q, want prepared", got)
		}
		if got := rqcNum(t, state, "round_number"); got != 1 {
			t.Fatalf("round_number = %v, want 1", got)
		}
		if got := rqcNum(t, state, "surah_id"); got != 1 {
			t.Fatalf("surah_id = %v, want 1", got)
		}
		if got := rqcNum(t, state, "from_ayah"); got != 1 {
			t.Fatalf("from_ayah = %v, want 1", got)
		}
		if got := rqcNum(t, state, "to_ayah"); got != 7 {
			t.Fatalf("to_ayah = %v, want 7", got)
		}
		if got := state["grading_required"]; got != true {
			t.Fatalf("grading_required = %v, want true", got)
		}
		if got := rqcNum(t, state, "version"); got < 1 {
			t.Fatalf("version = %v, want >= 1", got)
		}
		// Entries materialize only on activation; preorder carries the order.
		rqcAssertEntries(t, state, "waiting", 0)
		preorder := rqcObjects(t, state, "preorder")
		if len(preorder) != 3 {
			t.Fatalf("preorder count = %d, want 3", len(preorder))
		}
		wantOrder := []string{env.students[1], env.students[2], env.students[0]}
		for i, item := range preorder {
			if got := rqcStr(t, item, "student_id"); got != wantOrder[i] {
				t.Fatalf("preorder[%d].student_id = %q, want %q", i, got, wantOrder[i])
			}
			for _, field := range []string{"student_id", "student_name", "position"} {
				if _, ok := item[field]; !ok {
					t.Fatalf("preorder item %d missing %q", i, field)
				}
			}
		}
	})

	t.Run("manager GET returns the full QueueState projection", func(t *testing.T) {
		rec := env.req(t, env.teacherID, http.MethodGet, "/api/v1/sessions/"+sessionID+"/queue", "", "")
		if rec.Code != http.StatusOK {
			t.Fatalf("status: got %d want 200 body=%s", rec.Code, rec.Body.String())
		}
		state := rqcDecode(t, rec)
		rqcAssertQueueState(t, state)
		if got := rqcStr(t, state, "lifecycle"); got != "prepared" {
			t.Fatalf("lifecycle = %q, want prepared", got)
		}
		if len(rqcObjects(t, state, "preorder")) != 3 {
			t.Fatalf("manager preorder must carry the three pre-set candidates")
		}
	})

	t.Run("non-manager GET receives an empty preorder (CHK008)", func(t *testing.T) {
		rec := env.req(t, env.students[0], http.MethodGet, "/api/v1/sessions/"+sessionID+"/queue", "", "")
		if rec.Code != http.StatusOK {
			t.Fatalf("status: got %d want 200 body=%s", rec.Code, rec.Body.String())
		}
		state := rqcDecode(t, rec)
		rqcAssertQueueState(t, state)
		if len(rqcObjects(t, state, "preorder")) != 0 {
			t.Fatalf("non-manager preorder must be an empty array: %v", state["preorder"])
		}
	})

	t.Run("unknown session GET returns a 404 error envelope", func(t *testing.T) {
		rec := env.req(t, env.teacherID, http.MethodGet, "/api/v1/sessions/00000000-0000-0000-0000-000000000000/queue", "", "")
		if rec.Code != http.StatusNotFound {
			t.Fatalf("status: got %d want 404 body=%s", rec.Code, rec.Body.String())
		}
		envl := rqcDecodeError(t, rec)
		if envl.Error.Code != httpconst.ErrorCodeNotFound {
			t.Fatalf("error code = %q, want %q", envl.Error.Code, httpconst.ErrorCodeNotFound)
		}
		if envl.Error.Message == "" {
			t.Fatal("error message must not be empty")
		}
	})
}

func TestRecitationQueueManagement_Reorder(t *testing.T) {
	env := setupRqcEnv(t)

	t.Run("full reorder of a prepared round returns 200", func(t *testing.T) {
		sessionID := env.newSession(t, false)
		body := fmt.Sprintf(`{"round_type":"revision","surah_id":1,"from_ayah":1,"to_ayah":7,"grading_required":false,"student_order":[%q,%q,%q]}`,
			env.students[0], env.students[1], env.students[2])
		rec := env.req(t, env.teacherID, http.MethodPost, "/api/v1/sessions/"+sessionID+"/queue/rounds", body, "")
		if rec.Code != http.StatusCreated {
			t.Fatalf("prepare: got %d body=%s", rec.Code, rec.Body.String())
		}
		created := rqcDecode(t, rec)
		version := rqcNum(t, created, "version")

		reorder := fmt.Sprintf(`{"ordered_ids":[%q,%q,%q],"expected_version":%d}`,
			env.students[2], env.students[0], env.students[1], int(version))
		// A current supervisor is a queue manager just like the teacher.
		rec = env.req(t, env.superID, http.MethodPut, "/api/v1/sessions/"+sessionID+"/queue/order", reorder, "")
		if rec.Code != http.StatusOK {
			t.Fatalf("reorder: got %d want 200 body=%s", rec.Code, rec.Body.String())
		}
		state := rqcDecode(t, rec)
		rqcAssertQueueState(t, state)
		wantOrder := []string{env.students[2], env.students[0], env.students[1]}
		preorder := rqcObjects(t, state, "preorder")
		for i, item := range preorder {
			if got := rqcStr(t, item, "student_id"); got != wantOrder[i] {
				t.Fatalf("reordered preorder[%d] = %q, want %q", i, got, wantOrder[i])
			}
		}
		if got := rqcNum(t, state, "version"); got != version+1 {
			t.Fatalf("version after reorder = %v, want %v", got, version+1)
		}
	})

	t.Run("reorder after activation returns 409", func(t *testing.T) {
		sessionID, state := env.liveRound(t)
		entries := rqcAssertEntries(t, state, "waiting", 3)
		var ordered []string
		for _, entry := range entries {
			ordered = append(ordered, rqcStr(t, entry, "student_id"))
		}
		body := fmt.Sprintf(`{"ordered_ids":[%q,%q,%q],"expected_version":%d}`,
			ordered[1], ordered[2], ordered[0], int(rqcNum(t, state, "version")))
		rec := env.req(t, env.teacherID, http.MethodPut, "/api/v1/sessions/"+sessionID+"/queue/order", body, "")
		if rec.Code != http.StatusConflict {
			t.Fatalf("reorder active round: got %d want 409 body=%s", rec.Code, rec.Body.String())
		}
		envl := rqcDecodeError(t, rec)
		if envl.Error.Code == "" || envl.Error.Message == "" {
			t.Fatalf("409 must carry the standard error envelope: %v", envl)
		}
	})
}

func TestRecitationQueueManagement_TurnFlow(t *testing.T) {
	env := setupRqcEnv(t)
	sessionID, state := env.liveRound(t)
	entries := rqcAssertEntries(t, state, "waiting", 3)
	first := entries[0]
	second := entries[1]
	version := rqcNum(t, state, "version")

	t.Run("advance selects the next waiting entry", func(t *testing.T) {
		rec := env.req(t, env.teacherID, http.MethodPost, "/api/v1/sessions/"+sessionID+"/queue/advance",
			fmt.Sprintf(`{"expected_version":%d}`, int(version)), "")
		if rec.Code != http.StatusOK {
			t.Fatalf("advance: got %d want 200 body=%s", rec.Code, rec.Body.String())
		}
		state = rqcDecode(t, rec)
		rqcAssertQueueState(t, state)
		if got := rqcStr(t, state, "selected_entry_id"); got != rqcStr(t, first, "id") {
			t.Fatalf("selected_entry_id = %q, want %q", got, rqcStr(t, first, "id"))
		}
		if got := rqcNum(t, state, "version"); got != version+1 {
			t.Fatalf("version after advance = %v, want %v", got, version+1)
		}
		version = rqcNum(t, state, "version")
	})

	t.Run("start applies reciting to the selected entry only", func(t *testing.T) {
		// expected_entry_version is the entry's own optimistic-lock version
		// (QueueEntry.version), not the round version.
		rec := env.req(t, env.teacherID, http.MethodPut,
			"/api/v1/sessions/"+sessionID+"/queue/entries/"+rqcStr(t, first, "id")+"/status",
			fmt.Sprintf(`{"status":"reciting","expected_entry_version":%d}`, int(rqcEntryVersion(t, state, rqcStr(t, first, "id")))), "")
		if rec.Code != http.StatusOK {
			t.Fatalf("start: got %d want 200 body=%s", rec.Code, rec.Body.String())
		}
		state = rqcDecode(t, rec)
		rqcAssertQueueState(t, state)
		reciting := 0
		for _, entry := range rqcObjects(t, state, "entries") {
			if rqcStr(t, entry, "status") == "reciting" {
				reciting++
			}
		}
		if reciting != 1 {
			t.Fatalf("reciting entries = %d, want exactly 1", reciting)
		}
		version = rqcNum(t, state, "version")
	})

	t.Run("advance while reciting returns 409", func(t *testing.T) {
		rec := env.req(t, env.teacherID, http.MethodPost, "/api/v1/sessions/"+sessionID+"/queue/advance",
			fmt.Sprintf(`{"expected_version":%d}`, int(version)), "")
		if rec.Code != http.StatusConflict {
			t.Fatalf("advance while reciting: got %d want 409 body=%s", rec.Code, rec.Body.String())
		}
		envl := rqcDecodeError(t, rec)
		if envl.Error.Code != httpconst.ErrorCodeQueueEntryReciting {
			t.Fatalf("error code = %q, want %q", envl.Error.Code, httpconst.ErrorCodeQueueEntryReciting)
		}
	})

	t.Run("move one waiting entry while another recites", func(t *testing.T) {
		rec := env.req(t, env.teacherID, http.MethodPost,
			"/api/v1/sessions/"+sessionID+"/queue/entries/"+rqcStr(t, second, "id")+"/move",
			fmt.Sprintf(`{"new_position":3,"expected_version":%d}`, int(version)), "")
		if rec.Code != http.StatusOK {
			t.Fatalf("move: got %d want 200 body=%s", rec.Code, rec.Body.String())
		}
		state = rqcDecode(t, rec)
		rqcAssertQueueState(t, state)
		for _, entry := range rqcObjects(t, state, "entries") {
			if rqcStr(t, entry, "id") == rqcStr(t, second, "id") && rqcNum(t, entry, "position") != 3 {
				t.Fatalf("moved entry position = %v, want 3", rqcNum(t, entry, "position"))
			}
		}
		version = rqcNum(t, state, "version")
	})

	t.Run("skip the reciting entry", func(t *testing.T) {
		// The start transition bumped the entry's own version; skip must
		// present the fresh entry token, not the round version.
		rec := env.req(t, env.teacherID, http.MethodPut,
			"/api/v1/sessions/"+sessionID+"/queue/entries/"+rqcStr(t, first, "id")+"/status",
			fmt.Sprintf(`{"status":"skipped","expected_entry_version":%d}`, int(rqcEntryVersion(t, state, rqcStr(t, first, "id")))), "")
		if rec.Code != http.StatusOK {
			t.Fatalf("skip: got %d want 200 body=%s", rec.Code, rec.Body.String())
		}
		state = rqcDecode(t, rec)
		for _, entry := range rqcObjects(t, state, "entries") {
			if rqcStr(t, entry, "id") == rqcStr(t, first, "id") && rqcStr(t, entry, "status") != "skipped" {
				t.Fatalf("skipped entry status = %q, want skipped", rqcStr(t, entry, "status"))
			}
		}
		version = rqcNum(t, state, "version")
	})

	t.Run("move a terminal entry returns 409", func(t *testing.T) {
		rec := env.req(t, env.teacherID, http.MethodPost,
			"/api/v1/sessions/"+sessionID+"/queue/entries/"+rqcStr(t, first, "id")+"/move",
			fmt.Sprintf(`{"new_position":1,"expected_version":%d}`, int(version)), "")
		if rec.Code != http.StatusConflict {
			t.Fatalf("move terminal entry: got %d want 409 body=%s", rec.Code, rec.Body.String())
		}
	})

	t.Run("advance after the turn ends", func(t *testing.T) {
		rec := env.req(t, env.teacherID, http.MethodPost, "/api/v1/sessions/"+sessionID+"/queue/advance",
			fmt.Sprintf(`{"expected_version":%d}`, int(version)), "")
		if rec.Code != http.StatusOK {
			t.Fatalf("advance after skip: got %d want 200 body=%s", rec.Code, rec.Body.String())
		}
		state = rqcDecode(t, rec)
		if selected, ok := state["selected_entry_id"].(string); !ok || selected == "" {
			t.Fatalf("selected_entry_id = %v, want a non-empty selection", state["selected_entry_id"])
		}
		version = rqcNum(t, state, "version")
	})

	t.Run("reset finalizes and creates the sequential successor", func(t *testing.T) {
		rec := env.req(t, env.teacherID, http.MethodPost, "/api/v1/sessions/"+sessionID+"/queue/reset",
			fmt.Sprintf(`{"round_type":"old_revision","surah_id":2,"from_ayah":4,"to_ayah":8,"grading_required":false,"expected_version":%d}`, int(version)), "")
		if rec.Code != http.StatusCreated {
			t.Fatalf("reset: got %d want 201 body=%s", rec.Code, rec.Body.String())
		}
		state = rqcDecode(t, rec)
		rqcAssertQueueState(t, state)
		if got := rqcNum(t, state, "round_number"); got != 2 {
			t.Fatalf("successor round_number = %v, want 2", got)
		}
	})
}

func TestRecitationQueueManagement_UpdatePolicy(t *testing.T) {
	env := setupRqcEnv(t)
	sessionID := env.newSession(t, false)

	t.Run("patch one dimension returns QueuePolicy not QueueState", func(t *testing.T) {
		rec := env.req(t, env.teacherID, http.MethodPatch, "/api/v1/sessions/"+sessionID+"/queue/policy",
			`{"opt_out":"auto_approve","expected_version":1}`, "")
		if rec.Code != http.StatusOK {
			t.Fatalf("patch policy: got %d want 200 body=%s", rec.Code, rec.Body.String())
		}
		policy := rqcDecode(t, rec)
		for _, field := range []string{
			"population", "unfinished_finalization", "opt_out",
			"grade_visibility", "grade_correction", "version",
		} {
			if _, ok := policy[field]; !ok {
				t.Fatalf("QueuePolicy missing required field %q: %v", field, policy)
			}
		}
		for _, stateOnly := range []string{"session_id", "round_id", "entries", "preorder"} {
			if _, ok := policy[stateOnly]; ok {
				t.Fatalf("policy response must not carry QueueState field %q", stateOnly)
			}
		}
		if got := rqcStr(t, policy, "opt_out"); got != "auto_approve" {
			t.Fatalf("opt_out = %q, want auto_approve", got)
		}
		if got := rqcNum(t, policy, "version"); got != 2 {
			t.Fatalf("policy version = %v, want 2 after one effective change", got)
		}
	})

	t.Run("stale expected_version returns 409", func(t *testing.T) {
		rec := env.req(t, env.teacherID, http.MethodPatch, "/api/v1/sessions/"+sessionID+"/queue/policy",
			`{"opt_out":"approval_required","expected_version":999}`, "")
		if rec.Code != http.StatusConflict {
			t.Fatalf("stale policy patch: got %d want 409 body=%s", rec.Code, rec.Body.String())
		}
		envl := rqcDecodeError(t, rec)
		if envl.Error.Code != httpconst.ErrorCodeQueueVersionConflict {
			t.Fatalf("error code = %q, want %q", envl.Error.Code, httpconst.ErrorCodeQueueVersionConflict)
		}
	})
}

func TestRecitationQueueManagement_ValidationAndConflicts(t *testing.T) {
	env := setupRqcEnv(t)
	sessionID, state := env.liveRound(t)
	entries := rqcAssertEntries(t, state, "waiting", 3)
	entry := entries[0]
	roundVersion := int(rqcNum(t, state, "version"))

	t.Run("round request validation returns 422", func(t *testing.T) {
		cases := []struct {
			name     string
			body     string
			wantCode string
			field    string
		}{
			{"surah below range", `{"round_type":"revision","surah_id":0,"from_ayah":1,"to_ayah":7,"grading_required":false}`, httpconst.ErrorCodeQueueInvalidRange, httpconst.FieldSurahID},
			{"surah above range", `{"round_type":"revision","surah_id":115,"from_ayah":1,"to_ayah":7,"grading_required":false}`, httpconst.ErrorCodeQueueInvalidRange, httpconst.FieldSurahID},
			{"from after to", `{"round_type":"revision","surah_id":1,"from_ayah":8,"to_ayah":3,"grading_required":false}`, httpconst.ErrorCodeQueueInvalidRange, ""},
			{"invalid round type", `{"round_type":"weekly","surah_id":1,"from_ayah":1,"to_ayah":7,"grading_required":false}`, httpconst.ErrorCodeQueueInvalidEnum, httpconst.FieldRoundType},
		}
		for _, tc := range cases {
			rec := env.req(t, env.teacherID, http.MethodPost, "/api/v1/sessions/"+sessionID+"/queue/rounds", tc.body, "")
			if rec.Code != http.StatusUnprocessableEntity {
				t.Fatalf("%s: got %d want 422 body=%s", tc.name, rec.Code, rec.Body.String())
			}
			envl := rqcDecodeError(t, rec)
			if envl.Error.Code != tc.wantCode {
				t.Fatalf("%s: error code = %q, want %q", tc.name, envl.Error.Code, tc.wantCode)
			}
			if tc.field != "" {
				if _, ok := envl.Error.Fields[tc.field]; !ok {
					t.Fatalf("%s: fields %v must mention %q", tc.name, envl.Error.Fields, tc.field)
				}
			}
			if envl.Error.Message == "" {
				t.Fatalf("%s: error message must not be empty", tc.name)
			}
		}
	})

	t.Run("grade sent with skipped returns 422", func(t *testing.T) {
		body := fmt.Sprintf(`{"status":"skipped","expected_entry_version":%d,"grade":"good"}`, roundVersion)
		rec := env.req(t, env.teacherID, http.MethodPut,
			"/api/v1/sessions/"+sessionID+"/queue/entries/"+rqcStr(t, entry, "id")+"/status", body, "")
		if rec.Code != http.StatusUnprocessableEntity {
			t.Fatalf("grade with skipped: got %d want 422 body=%s", rec.Code, rec.Body.String())
		}
		envl := rqcDecodeError(t, rec)
		if envl.Error.Code != httpconst.ErrorCodeQueueInvalidGrade {
			t.Fatalf("error code = %q, want %q", envl.Error.Code, httpconst.ErrorCodeQueueInvalidGrade)
		}
		if _, ok := envl.Error.Fields[httpconst.FieldGrade]; !ok {
			t.Fatalf("fields %v must mention %q", envl.Error.Fields, httpconst.FieldGrade)
		}
	})

	t.Run("stale advance version returns 409", func(t *testing.T) {
		rec := env.req(t, env.teacherID, http.MethodPost, "/api/v1/sessions/"+sessionID+"/queue/advance",
			fmt.Sprintf(`{"expected_version":%d}`, roundVersion+100), "")
		if rec.Code != http.StatusConflict {
			t.Fatalf("stale advance: got %d want 409 body=%s", rec.Code, rec.Body.String())
		}
		envl := rqcDecodeError(t, rec)
		if envl.Error.Code != httpconst.ErrorCodeQueueVersionConflict {
			t.Fatalf("error code = %q, want %q", envl.Error.Code, httpconst.ErrorCodeQueueVersionConflict)
		}
	})

	t.Run("stale entry version returns 409", func(t *testing.T) {
		body := fmt.Sprintf(`{"status":"skipped","expected_entry_version":%d}`, roundVersion+100)
		rec := env.req(t, env.teacherID, http.MethodPut,
			"/api/v1/sessions/"+sessionID+"/queue/entries/"+rqcStr(t, entry, "id")+"/status", body, "")
		if rec.Code != http.StatusConflict {
			t.Fatalf("stale entry version: got %d want 409 body=%s", rec.Code, rec.Body.String())
		}
		envl := rqcDecodeError(t, rec)
		if envl.Error.Code != httpconst.ErrorCodeQueueVersionConflict {
			t.Fatalf("error code = %q, want %q", envl.Error.Code, httpconst.ErrorCodeQueueVersionConflict)
		}
	})
}

func TestRecitationQueueManagement_IdempotencyReplay(t *testing.T) {
	env := setupRqcEnv(t)
	sessionID := env.newSession(t, true)
	for _, studentID := range env.students {
		rqcSeedPresence(t, env.pool, sessionID, studentID)
	}
	const key = "rqc-replay-1"
	bodyA := `{"round_type":"revision","surah_id":1,"from_ayah":1,"to_ayah":7,"grading_required":false}`
	bodyB := `{"round_type":"test","surah_id":2,"from_ayah":1,"to_ayah":3,"grading_required":false}`

	rec := env.req(t, env.teacherID, http.MethodPost, "/api/v1/sessions/"+sessionID+"/queue/rounds", bodyA, key)
	if rec.Code != http.StatusCreated {
		t.Fatalf("first keyed request: got %d want 201 body=%s", rec.Code, rec.Body.String())
	}
	first := rqcDecode(t, rec)
	roundID := rqcStr(t, first, "round_id")

	rec = env.req(t, env.teacherID, http.MethodPost, "/api/v1/sessions/"+sessionID+"/queue/rounds", bodyA, key)
	if rec.Code != http.StatusOK && rec.Code != http.StatusCreated {
		t.Fatalf("key replay: got %d want 200 or 201 body=%s", rec.Code, rec.Body.String())
	}
	replay := rqcDecode(t, rec)
	if got := rqcStr(t, replay, "round_id"); got != roundID {
		t.Fatalf("replay round_id = %q, want the committed resource %q", got, roundID)
	}

	var rounds int
	if err := env.pool.QueryRow(context.Background(),
		`SELECT COUNT(*) FROM recitation_queue WHERE session_id = $1::uuid`, sessionID).Scan(&rounds); err != nil {
		t.Fatalf("count rounds: %v", err)
	}
	if rounds != 1 {
		t.Fatalf("committed rounds after replay = %d, want 1", rounds)
	}

	rec = env.req(t, env.teacherID, http.MethodPost, "/api/v1/sessions/"+sessionID+"/queue/rounds", bodyB, key)
	if rec.Code != http.StatusConflict {
		t.Fatalf("key reuse with a different command: got %d want 409 body=%s", rec.Code, rec.Body.String())
	}
	envl := rqcDecodeError(t, rec)
	if envl.Error.Code != httpconst.ErrorCodeQueueDuplicateCommand {
		t.Fatalf("error code = %q, want %q", envl.Error.Code, httpconst.ErrorCodeQueueDuplicateCommand)
	}
}

func TestRecitationQueueManagement_RateLimited(t *testing.T) {
	env := setupRqcEnv(t)
	sessionID := env.newSession(t, false)
	rec := env.req(t, env.teacherID, http.MethodPost, "/api/v1/sessions/"+sessionID+"/queue/rounds",
		`{"round_type":"revision","surah_id":1,"from_ayah":1,"to_ayah":7,"grading_required":false}`, "")
	if rec.Code != http.StatusCreated {
		t.Fatalf("prepare: got %d body=%s", rec.Code, rec.Body.String())
	}

	// Tiny per-user budget: the first GET passes, the second is rejected.
	limiter := middleware.NewRateLimitMiddleware(100, 1)
	limited := limiter.Limit(env.mux)

	doGet := func() *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/sessions/"+sessionID+"/queue", nil)
		req = req.WithContext(auth.WithPrincipal(req.Context(), auth.AuthPrincipal{UserID: env.teacherID}))
		rec := httptest.NewRecorder()
		limited.ServeHTTP(rec, req)
		return rec
	}

	if rec := doGet(); rec.Code != http.StatusOK {
		t.Fatalf("first request: got %d want 200 body=%s", rec.Code, rec.Body.String())
	}
	rec = doGet()
	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("second request: got %d want 429 body=%s", rec.Code, rec.Body.String())
	}
	envl := rqcDecodeError(t, rec)
	if envl.Error.Code != httpconst.ErrorCodeRateLimitExceeded {
		t.Fatalf("error code = %q, want %q", envl.Error.Code, httpconst.ErrorCodeRateLimitExceeded)
	}
}
