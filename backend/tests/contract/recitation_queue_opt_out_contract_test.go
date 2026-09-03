//go:build contract

package contract

// T041 — behavioral REST/WS contract tests for the F-003 US2 opt-out
// surface, pinned to specs/003-recitation-queue-system/contracts/
// recitation-queue.openapi.yaml and recitation-queue.ws_events.md (runtime
// behavior only — document parity is T070).
//
// The suite runs the production queue.Handler against real PostgreSQL with a
// fresh schema per test. It is the red spec for T043 (and T044 for targeted
// delivery): it does not build until the opt-out handler surface exists.
//
// Handler API surface specified for T043:
//
//	queue.NewHandler(repo *queue.Repository, rounds *queue.RoundService,
//		turns *queue.TurnService, policies *queue.PolicyService,
//		optOuts *queue.OptOutService) *queue.Handler
//
// with the two US2 methods:
//
//	func (h *Handler) RequestOptOut(w http.ResponseWriter, r *http.Request)      // POST /sessions/{sessionId}/queue/opt-out
//	func (h *Handler) DecideOptOutRequest(w http.ResponseWriter, r *http.Request) // POST /sessions/{sessionId}/queue/opt-out-requests/{requestId}/decision
//
// T043 must also update the T024/T025 NewHandler call sites to the extended
// five-argument constructor.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/KarimMFadel/halaqaty/backend/internal/auth"
	"github.com/KarimMFadel/halaqaty/backend/internal/platform/httpconst"
	"github.com/KarimMFadel/halaqaty/backend/internal/queue"
	"github.com/KarimMFadel/halaqaty/backend/internal/realtime"
	"github.com/KarimMFadel/halaqaty/backend/internal/sessions"
)

type rqoEnv struct {
	mux        *http.ServeMux
	pool       *pgxpool.Pool
	repo       *queue.Repository
	circleID   string
	teacherID  string
	superID    string
	students   []string
	noEntryStu string
}

func rqoUniqueSchemaName(t *testing.T) string {
	t.Helper()
	return fmt.Sprintf("test_rqo_contract_%d", time.Now().UnixNano())
}

func setupRqoEnv(t *testing.T) *rqoEnv {
	t.Helper()
	ctx := context.Background()

	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		t.Skip("DATABASE_URL is not set; T041 contract tests require PostgreSQL")
	}

	adminPool, err := pgxpool.New(ctx, dbURL)
	if err != nil {
		t.Fatalf("open admin pool: %v", err)
	}
	conn := rqcAcquireConn(t, adminPool, ctx)
	schema := rqoUniqueSchemaName(t)
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
		queue.NewOptOutService(repo),
	)

	env := &rqoEnv{mux: rqoMux(handler), pool: pool, repo: repo}
	env.teacherID = rqcSeedUser(t, pool, "rqo-teacher", "Ustadh Teacher")
	env.superID = rqcSeedUser(t, pool, "rqo-supervisor", "Ustadh Supervisor")
	for i, name := range []string{"Talib One", "Talib Two"} {
		env.students = append(env.students, rqcSeedUser(t, pool, fmt.Sprintf("rqo-student-%d", i), name))
	}
	// A member with no round entry: present_at_activation gives them no
	// entry, so their own opt-out request cannot resolve (404).
	env.noEntryStu = rqcSeedUser(t, pool, "rqo-student-absent", "Talib Absent")
	env.circleID = rqcSeedCircle(t, pool, env.teacherID)
	rqcSeedMember(t, pool, env.circleID, env.teacherID, "teacher")
	rqcSeedMember(t, pool, env.circleID, env.superID, "supervisor")
	for _, studentID := range append([]string{env.noEntryStu}, env.students...) {
		rqcSeedMember(t, pool, env.circleID, studentID, "student")
	}
	return env
}

func rqoMux(h *queue.Handler) *http.ServeMux {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/sessions/{sessionId}/queue", h.GetQueue)
	mux.HandleFunc("POST /api/v1/sessions/{sessionId}/queue/rounds", h.CreateRound)
	mux.HandleFunc("POST /api/v1/sessions/{sessionId}/queue/reset", h.ResetQueue)
	mux.HandleFunc("PATCH /api/v1/sessions/{sessionId}/queue/policy", h.UpdatePolicy)
	mux.HandleFunc("POST /api/v1/sessions/{sessionId}/queue/opt-out", h.RequestOptOut)
	mux.HandleFunc("POST /api/v1/sessions/{sessionId}/queue/opt-out-requests/{requestId}/decision", h.DecideOptOutRequest)
	return mux
}

// liveRound starts a live session and prepares a round pre-ordering the two
// present students; automatic activation materializes two waiting entries.
func (e *rqoEnv) liveRound(t *testing.T) (string, map[string]any) {
	t.Helper()
	ctx := context.Background()
	var sessionID string
	if err := e.pool.QueryRow(ctx, `
		INSERT INTO sessions (circle_id, created_by)
		VALUES ($1::uuid, $2::uuid)
		RETURNING id::text
	`, e.circleID, e.teacherID).Scan(&sessionID); err != nil {
		t.Fatalf("insert session: %v", err)
	}
	if _, err := sessions.NewSessionRepository(e.pool).StartSession(ctx, sessionID, sessions.MediaRoomRef("rqo-room-"+sessionID)); err != nil {
		t.Fatalf("start session: %v", err)
	}
	for _, studentID := range e.students {
		rqcSeedPresence(t, e.pool, sessionID, studentID)
	}
	body := fmt.Sprintf(`{"round_type":"revision","surah_id":1,"from_ayah":1,"to_ayah":7,"grading_required":true,"student_order":[%q,%q]}`,
		e.students[0], e.students[1])
	rec := e.req(t, e.teacherID, http.MethodPost, "/api/v1/sessions/"+sessionID+"/queue/rounds", body, "")
	if rec.Code != http.StatusCreated {
		t.Fatalf("prepare live round: got %d body=%s", rec.Code, rec.Body.String())
	}
	return sessionID, rqcDecode(t, rec)
}

// req performs one HTTP request as actorID; an empty actorID sends no
// authenticated principal so handlers must answer 401.
func (e *rqoEnv) req(t *testing.T, actorID, method, target, body, idemKey string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, target, strings.NewReader(body))
	req.Header.Set(httpconst.HeaderContentType, httpconst.ContentTypeApplicationJSON)
	if idemKey != "" {
		req.Header.Set(rqcIdempotencyKey, idemKey)
	}
	if actorID != "" {
		req = req.WithContext(auth.WithPrincipal(req.Context(), auth.AuthPrincipal{UserID: actorID}))
	}
	rec := httptest.NewRecorder()
	e.mux.ServeHTTP(rec, req)
	return rec
}

// rqoAssertOptOutResult pins the contract shape of the OptOutResult body.
func rqoAssertOptOutResult(t *testing.T, m map[string]any) (map[string]any, map[string]any) {
	t.Helper()
	request, ok := m["request"].(map[string]any)
	if !ok {
		t.Fatalf("OptOutResult.request is not an object: %v", m)
	}
	entry, ok := m["entry"].(map[string]any)
	if !ok {
		t.Fatalf("OptOutResult.entry is not an object: %v", m)
	}
	for _, field := range []string{"id", "queue_entry_id", "status", "requested_at"} {
		if _, ok := request[field]; !ok {
			t.Fatalf("OptOutResult.request missing required field %q: %v", field, request)
		}
	}
	for _, field := range []string{"id", "student_id", "student_name", "position", "status", "version"} {
		if _, ok := entry[field]; !ok {
			t.Fatalf("OptOutResult.entry missing required field %q: %v", field, entry)
		}
	}
	// Waiting/opted-out entries never carry grade material (SC-005): the
	// nullable projection fields must be absent or null.
	if value, present := entry["grade"]; present && value != nil {
		t.Fatalf("opt-out entry leaked a grade value: %v", entry)
	}
	if value, present := entry["grade_notes"]; present && value != nil {
		t.Fatalf("opt-out entry leaked a note value: %v", entry)
	}
	return request, entry
}

func (e *rqoEnv) countOutbox(t *testing.T, sessionID, eventType string) int {
	t.Helper()
	var count int
	if err := e.pool.QueryRow(context.Background(), `
		SELECT COUNT(*) FROM queue_event_outbox
		WHERE session_id = $1::uuid AND event_type = $2
	`, sessionID, eventType).Scan(&count); err != nil {
		t.Fatalf("count %s outbox rows: %v", eventType, err)
	}
	return count
}

func (e *rqoEnv) pendingRequestCount(t *testing.T, entryID string) int {
	t.Helper()
	var count int
	if err := e.pool.QueryRow(context.Background(), `
		SELECT COUNT(*) FROM queue_opt_out_requests
		WHERE queue_entry_id = $1::uuid AND status = 'pending'
	`, entryID).Scan(&count); err != nil {
		t.Fatalf("count pending requests: %v", err)
	}
	return count
}

func TestRecitationQueueOptOut_RequestAndDecisionFlow(t *testing.T) {
	env := setupRqoEnv(t)
	sessionID, state := env.liveRound(t)
	entries := rqcObjects(t, state, "entries")
	requester := rqcStr(t, entries[0], "student_id")
	requesterEntryID := rqcStr(t, entries[0], "id")
	entryVersionBefore := rqcEntryVersion(t, state, requesterEntryID)

	var requestID string
	var entryVersionAtRequest float64

	t.Run("student self request returns 201 with a pending OptOutResult", func(t *testing.T) {
		rec := env.req(t, requester, http.MethodPost, "/api/v1/sessions/"+sessionID+"/queue/opt-out", "", "")
		if rec.Code != http.StatusCreated {
			t.Fatalf("opt-out request: got %d want 201 body=%s", rec.Code, rec.Body.String())
		}
		request, entry := rqoAssertOptOutResult(t, rqcDecode(t, rec))
		if got := rqcStr(t, request, "status"); got != "pending" {
			t.Fatalf("request status = %q, want pending", got)
		}
		if got := rqcStr(t, request, "queue_entry_id"); got != requesterEntryID {
			t.Fatalf("request entry = %q, want the caller's own entry %q", got, requesterEntryID)
		}
		if got := rqcStr(t, entry, "id"); got != requesterEntryID {
			t.Fatalf("result entry = %q, want %q", got, requesterEntryID)
		}
		if got := rqcStr(t, entry, "status"); got != "waiting" {
			t.Fatalf("entry stays %q while the request is pending, want waiting", got)
		}
		requestID = rqcStr(t, request, "id")
		entryVersionAtRequest = rqcNum(t, entry, "version")
		if entryVersionAtRequest != entryVersionBefore {
			t.Fatalf("pending request changed entry version to %v, want %v", entryVersionAtRequest, entryVersionBefore)
		}
	})

	t.Run("keyed replay returns 200 with the committed request", func(t *testing.T) {
		rec := env.req(t, requester, http.MethodPost, "/api/v1/sessions/"+sessionID+"/queue/opt-out", "", "rqo-key-1")
		if rec.Code != http.StatusOK && rec.Code != http.StatusCreated {
			t.Fatalf("keyed first request: got %d want 200 or 201 body=%s", rec.Code, rec.Body.String())
		}
		replay := env.req(t, requester, http.MethodPost, "/api/v1/sessions/"+sessionID+"/queue/opt-out", "", "rqo-key-1")
		if replay.Code != http.StatusOK {
			t.Fatalf("keyed replay: got %d want 200 body=%s", replay.Code, replay.Body.String())
		}
		request, _ := rqoAssertOptOutResult(t, rqcDecode(t, replay))
		if got := rqcStr(t, request, "id"); got != requestID {
			t.Fatalf("replay request id = %q, want the committed %q", got, requestID)
		}
	})

	t.Run("unkeyed duplicate returns 200 with the same pending request", func(t *testing.T) {
		rec := env.req(t, requester, http.MethodPost, "/api/v1/sessions/"+sessionID+"/queue/opt-out", "", "")
		if rec.Code != http.StatusOK {
			t.Fatalf("duplicate request: got %d want 200 body=%s", rec.Code, rec.Body.String())
		}
		request, _ := rqoAssertOptOutResult(t, rqcDecode(t, rec))
		if got := rqcStr(t, request, "id"); got != requestID {
			t.Fatalf("duplicate request id = %q, want the single pending %q", got, requestID)
		}
		if got := env.pendingRequestCount(t, requesterEntryID); got != 1 {
			t.Fatalf("pending requests after duplicate = %d, want exactly 1", got)
		}
	})

	t.Run("manager decision approves to opted_out", func(t *testing.T) {
		body := fmt.Sprintf(`{"decision":"approved","expected_entry_version":%d}`, int(entryVersionAtRequest))
		rec := env.req(t, env.superID, http.MethodPost,
			"/api/v1/sessions/"+sessionID+"/queue/opt-out-requests/"+requestID+"/decision", body, "")
		if rec.Code != http.StatusOK {
			t.Fatalf("decision: got %d want 200 body=%s", rec.Code, rec.Body.String())
		}
		request, entry := rqoAssertOptOutResult(t, rqcDecode(t, rec))
		if got := rqcStr(t, request, "status"); got != "approved" {
			t.Fatalf("decided request status = %q, want approved", got)
		}
		if got := rqcStr(t, entry, "status"); got != "opted_out" {
			t.Fatalf("decided entry status = %q, want opted_out", got)
		}
		if got := rqcNum(t, entry, "version"); got <= entryVersionAtRequest {
			t.Fatalf("decided entry version = %v, want > %v", got, entryVersionAtRequest)
		}
		if got := env.pendingRequestCount(t, requesterEntryID); got != 0 {
			t.Fatalf("pending requests after approval = %d, want 0", got)
		}
	})

	t.Run("deciding an already-decided request returns 409", func(t *testing.T) {
		body := fmt.Sprintf(`{"decision":"approved","expected_entry_version":%d}`, int(entryVersionAtRequest))
		rec := env.req(t, env.teacherID, http.MethodPost,
			"/api/v1/sessions/"+sessionID+"/queue/opt-out-requests/"+requestID+"/decision", body, "")
		if rec.Code != http.StatusConflict {
			t.Fatalf("re-decision: got %d want 409 body=%s", rec.Code, rec.Body.String())
		}
		envl := rqcDecodeError(t, rec)
		if envl.Error.Code != httpconst.ErrorCodeQueueInvalidTransition {
			t.Fatalf("re-decision code = %q, want %q", envl.Error.Code, httpconst.ErrorCodeQueueInvalidTransition)
		}
	})
}

func TestRecitationQueueOptOut_DeclineKeepsEntryWaiting(t *testing.T) {
	env := setupRqoEnv(t)
	sessionID, state := env.liveRound(t)
	entries := rqcObjects(t, state, "entries")
	requester := rqcStr(t, entries[0], "student_id")
	entryID := rqcStr(t, entries[0], "id")
	entryVersion := rqcEntryVersion(t, state, entryID)

	rec := env.req(t, requester, http.MethodPost, "/api/v1/sessions/"+sessionID+"/queue/opt-out", "", "")
	if rec.Code != http.StatusCreated {
		t.Fatalf("request: got %d body=%s", rec.Code, rec.Body.String())
	}
	request, _ := rqoAssertOptOutResult(t, rqcDecode(t, rec))
	body := fmt.Sprintf(`{"decision":"declined","expected_entry_version":%d}`, int(entryVersion))
	rec = env.req(t, env.teacherID, http.MethodPost,
		"/api/v1/sessions/"+sessionID+"/queue/opt-out-requests/"+rqcStr(t, request, "id")+"/decision", body, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("decline: got %d want 200 body=%s", rec.Code, rec.Body.String())
	}
	decidedRequest, entry := rqoAssertOptOutResult(t, rqcDecode(t, rec))
	if got := rqcStr(t, decidedRequest, "status"); got != "declined" {
		t.Fatalf("declined request status = %q, want declined", got)
	}
	if got := rqcStr(t, entry, "status"); got != "waiting" {
		t.Fatalf("entry after decline = %q, want waiting (CHK005)", got)
	}
}

func TestRecitationQueueOptOut_AutoApprove(t *testing.T) {
	env := setupRqoEnv(t)
	sessionID, state := env.liveRound(t)
	entries := rqcObjects(t, state, "entries")
	requester := rqcStr(t, entries[0], "student_id")
	entryID := rqcStr(t, entries[0], "id")

	patch := env.req(t, env.teacherID, http.MethodPatch, "/api/v1/sessions/"+sessionID+"/queue/policy",
		`{"opt_out":"auto_approve","expected_version":1}`, "")
	if patch.Code != http.StatusOK {
		t.Fatalf("patch opt-out policy: got %d body=%s", patch.Code, patch.Body.String())
	}

	rec := env.req(t, requester, http.MethodPost, "/api/v1/sessions/"+sessionID+"/queue/opt-out", "", "")
	if rec.Code != http.StatusCreated {
		t.Fatalf("auto-approve request: got %d want 201 body=%s", rec.Code, rec.Body.String())
	}
	request, entry := rqoAssertOptOutResult(t, rqcDecode(t, rec))
	if got := rqcStr(t, entry, "status"); got != "opted_out" {
		t.Fatalf("auto-approved entry status = %q, want opted_out", got)
	}
	if got := rqcStr(t, request, "status"); got != "approved" {
		t.Fatalf("auto-approved request status = %q, want approved", got)
	}
	if got := env.pendingRequestCount(t, entryID); got != 0 {
		t.Fatalf("pending rows under auto_approve = %d, want 0", got)
	}
	// Auto-approved opt-outs emit nothing targeted (ws_events.md).
	if got := env.countOutbox(t, sessionID, "queue.opt_out_requested"); got != 0 {
		t.Fatalf("queue.opt_out_requested rows under auto_approve = %d, want 0", got)
	}

	replay := env.req(t, requester, http.MethodPost, "/api/v1/sessions/"+sessionID+"/queue/opt-out", "", "")
	if replay.Code != http.StatusOK {
		t.Fatalf("auto-approve replay: got %d want 200 body=%s", replay.Code, replay.Body.String())
	}
	replayRequest, replayEntry := rqoAssertOptOutResult(t, rqcDecode(t, replay))
	if rqcStr(t, replayRequest, "id") != rqcStr(t, request, "id") {
		t.Fatalf("auto-approve replay returned a different request: %v vs %v", replayRequest, request)
	}
	if got := rqcStr(t, replayEntry, "status"); got != "opted_out" {
		t.Fatalf("auto-approve replay entry status = %q, want opted_out", got)
	}
}

func TestRecitationQueueOptOut_Authorization(t *testing.T) {
	env := setupRqoEnv(t)
	sessionID, _ := env.liveRound(t)

	t.Run("manager calling opt-out receives 403", func(t *testing.T) {
		rec := env.req(t, env.teacherID, http.MethodPost, "/api/v1/sessions/"+sessionID+"/queue/opt-out", "", "")
		if rec.Code != http.StatusForbidden {
			t.Fatalf("manager opt-out: got %d want 403 body=%s", rec.Code, rec.Body.String())
		}
		envl := rqcDecodeError(t, rec)
		if envl.Error.Code != httpconst.ErrorCodeForbidden {
			t.Fatalf("manager opt-out code = %q, want %q", envl.Error.Code, httpconst.ErrorCodeForbidden)
		}
	})

	t.Run("student without an entry receives 404", func(t *testing.T) {
		// present_at_activation gave the absent member no entry; the
		// student-self request cannot resolve an own entry.
		rec := env.req(t, env.noEntryStu, http.MethodPost, "/api/v1/sessions/"+sessionID+"/queue/opt-out", "", "")
		if rec.Code != http.StatusNotFound {
			t.Fatalf("entry-less opt-out: got %d want 404 body=%s", rec.Code, rec.Body.String())
		}
		envl := rqcDecodeError(t, rec)
		if envl.Error.Code != httpconst.ErrorCodeNotFound {
			t.Fatalf("entry-less opt-out code = %q, want %q", envl.Error.Code, httpconst.ErrorCodeNotFound)
		}
	})

	t.Run("student deciding a request receives 403", func(t *testing.T) {
		rec := env.req(t, env.students[1], http.MethodPost,
			"/api/v1/sessions/"+sessionID+"/queue/opt-out-requests/00000000-0000-0000-0000-000000000000/decision",
			`{"decision":"approved","expected_entry_version":1}`, "")
		if rec.Code != http.StatusForbidden {
			t.Fatalf("student decision: got %d want 403 body=%s", rec.Code, rec.Body.String())
		}
	})

	t.Run("unauthenticated opt-out receives 401", func(t *testing.T) {
		rec := env.req(t, "", http.MethodPost, "/api/v1/sessions/"+sessionID+"/queue/opt-out", "", "")
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("unauthenticated opt-out: got %d want 401 body=%s", rec.Code, rec.Body.String())
		}
		envl := rqcDecodeError(t, rec)
		if envl.Error.Code != httpconst.ErrorCodeUnauthorized {
			t.Fatalf("unauthenticated opt-out code = %q, want %q", envl.Error.Code, httpconst.ErrorCodeUnauthorized)
		}
	})

	t.Run("unauthenticated decision receives 401", func(t *testing.T) {
		rec := env.req(t, "", http.MethodPost,
			"/api/v1/sessions/"+sessionID+"/queue/opt-out-requests/00000000-0000-0000-0000-000000000000/decision",
			`{"decision":"approved","expected_entry_version":1}`, "")
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("unauthenticated decision: got %d want 401 body=%s", rec.Code, rec.Body.String())
		}
	})
}

func TestRecitationQueueOptOut_DecisionValidationAndConflicts(t *testing.T) {
	env := setupRqoEnv(t)

	newPending := func(t *testing.T) (sessionID, requestID, entryID string, entryVersion float64) {
		t.Helper()
		sessionID, state := env.liveRound(t)
		entries := rqcObjects(t, state, "entries")
		entryID = rqcStr(t, entries[0], "id")
		entryVersion = rqcEntryVersion(t, state, entryID)
		rec := env.req(t, rqcStr(t, entries[0], "student_id"), http.MethodPost, "/api/v1/sessions/"+sessionID+"/queue/opt-out", "", "")
		if rec.Code != http.StatusCreated {
			t.Fatalf("seed pending request: got %d body=%s", rec.Code, rec.Body.String())
		}
		request, _ := rqoAssertOptOutResult(t, rqcDecode(t, rec))
		return sessionID, rqcStr(t, request, "id"), entryID, entryVersion
	}

	t.Run("invalid decision enum returns 422", func(t *testing.T) {
		sessionID, requestID, _, version := newPending(t)
		body := fmt.Sprintf(`{"decision":"maybe","expected_entry_version":%d}`, int(version))
		rec := env.req(t, env.teacherID, http.MethodPost,
			"/api/v1/sessions/"+sessionID+"/queue/opt-out-requests/"+requestID+"/decision", body, "")
		if rec.Code != http.StatusUnprocessableEntity {
			t.Fatalf("invalid decision: got %d want 422 body=%s", rec.Code, rec.Body.String())
		}
		envl := rqcDecodeError(t, rec)
		if envl.Error.Code != httpconst.ErrorCodeQueueInvalidEnum {
			t.Fatalf("invalid decision code = %q, want %q", envl.Error.Code, httpconst.ErrorCodeQueueInvalidEnum)
		}
		if _, ok := envl.Error.Fields[httpconst.FieldDecision]; !ok {
			t.Fatalf("422 fields %v must mention %q", envl.Error.Fields, httpconst.FieldDecision)
		}
	})

	t.Run("stale expected_entry_version returns 409", func(t *testing.T) {
		sessionID, requestID, entryID, _ := newPending(t)
		body := fmt.Sprintf(`{"decision":"approved","expected_entry_version":%d}`, 999)
		rec := env.req(t, env.teacherID, http.MethodPost,
			"/api/v1/sessions/"+sessionID+"/queue/opt-out-requests/"+requestID+"/decision", body, "")
		if rec.Code != http.StatusConflict {
			t.Fatalf("stale decision: got %d want 409 body=%s", rec.Code, rec.Body.String())
		}
		envl := rqcDecodeError(t, rec)
		if envl.Error.Code != httpconst.ErrorCodeQueueVersionConflict {
			t.Fatalf("stale decision code = %q, want %q", envl.Error.Code, httpconst.ErrorCodeQueueVersionConflict)
		}
		if got := env.pendingRequestCount(t, entryID); got != 1 {
			t.Fatalf("pending requests after stale decision = %d, want still 1", got)
		}
	})

	t.Run("unknown request id returns 404", func(t *testing.T) {
		sessionID, _, _, _ := newPending(t)
		rec := env.req(t, env.teacherID, http.MethodPost,
			"/api/v1/sessions/"+sessionID+"/queue/opt-out-requests/00000000-0000-0000-0000-000000000000/decision",
			`{"decision":"approved","expected_entry_version":1}`, "")
		if rec.Code != http.StatusNotFound {
			t.Fatalf("unknown request: got %d want 404 body=%s", rec.Code, rec.Body.String())
		}
		envl := rqcDecodeError(t, rec)
		if envl.Error.Code != httpconst.ErrorCodeNotFound {
			t.Fatalf("unknown request code = %q, want %q", envl.Error.Code, httpconst.ErrorCodeNotFound)
		}
	})

	t.Run("request pending at round finalization is non-actionable", func(t *testing.T) {
		sessionID, requestID, _, version := newPending(t)
		current := env.req(t, env.teacherID, http.MethodGet, "/api/v1/sessions/"+sessionID+"/queue", "", "")
		if current.Code != http.StatusOK {
			t.Fatalf("load queue: got %d body=%s", current.Code, current.Body.String())
		}
		reset := fmt.Sprintf(`{"round_type":"old_revision","surah_id":2,"from_ayah":1,"to_ayah":3,"grading_required":false,"expected_version":%d}`,
			int(rqcNum(t, rqcDecode(t, current), "version")))
		rec := env.req(t, env.teacherID, http.MethodPost, "/api/v1/sessions/"+sessionID+"/queue/reset", reset, "")
		if rec.Code != http.StatusCreated {
			t.Fatalf("finalize round via reset: got %d body=%s", rec.Code, rec.Body.String())
		}

		body := fmt.Sprintf(`{"decision":"approved","expected_entry_version":%d}`, int(version))
		rec = env.req(t, env.teacherID, http.MethodPost,
			"/api/v1/sessions/"+sessionID+"/queue/opt-out-requests/"+requestID+"/decision", body, "")
		if rec.Code != http.StatusConflict {
			t.Fatalf("post-finalization decision: got %d want 409 body=%s", rec.Code, rec.Body.String())
		}
		envl := rqcDecodeError(t, rec)
		if envl.Error.Code != httpconst.ErrorCodeQueueRoundFinalized {
			t.Fatalf("post-finalization code = %q, want %q", envl.Error.Code, httpconst.ErrorCodeQueueRoundFinalized)
		}
	})
}

// --- Targeted delivery (T044 red spec) -------------------------------------

type rqoTicketReader struct{ circleID string }

func (r rqoTicketReader) ListCircleIDs(context.Context, string) ([]string, error) {
	return []string{r.circleID}, nil
}

type rqoAuthorizer struct{ allowed map[string]bool }

func (a rqoAuthorizer) AuthorizeSessionTopic(_ context.Context, userID, _ string) error {
	if !a.allowed[userID] {
		return errors.New("session topic unauthorized")
	}
	return nil
}

// rqoConn wraps a WebSocket connection with a single reader goroutine so that
// quiet-window assertions can timeout without corrupting the gorilla/websocket
// connection (the library docs state that a timed-out read leaves the
// connection in a failed state).
type rqoConn struct {
	conn *websocket.Conn
	ch   chan map[string]any
}

func rqoConnect(t *testing.T, server *httptest.Server, tickets *realtime.TicketService, userID, sessionID string) *rqoConn {
	t.Helper()
	ticket, err := tickets.Issue(context.Background(), userID)
	if err != nil {
		t.Fatalf("issue ticket: %v", err)
	}
	conn, _, err := websocket.DefaultDialer.Dial("ws"+server.URL[len("http"):]+"?token="+ticket.Token, nil)
	if err != nil {
		t.Fatalf("dial hub: %v", err)
	}
	c := &rqoConn{conn: conn, ch: make(chan map[string]any, 16)}
	go func() {
		defer close(c.ch)
		for {
			_, raw, err := conn.ReadMessage()
			if err != nil {
				return
			}
			var event map[string]any
			if err := json.Unmarshal(raw, &event); err != nil {
				return
			}
			select {
			case c.ch <- event:
			case <-time.After(5 * time.Second):
				return
			}
		}
	}()
	if err := conn.WriteJSON(map[string]any{"action": "subscribe", "topic": "session." + sessionID}); err != nil {
		t.Fatalf("subscribe: %v", err)
	}
	if event := rqoRead(t, c); event["type"] != "subscribed" {
		t.Fatalf("subscribe response = %v", event)
	}
	if event := rqoRead(t, c); event["type"] != "queue.state" {
		t.Fatalf("queue state = %v", event)
	}
	return c
}

func rqoRead(t *testing.T, c *rqoConn) map[string]any {
	t.Helper()
	select {
	case event := <-c.ch:
		return event
	case <-time.After(2 * time.Second):
		t.Fatalf("read realtime event: timeout")
	}
	return nil
}

// rqoAssertNoEvent fails when another frame arrives inside the quiet window.
func rqoAssertNoEvent(t *testing.T, c *rqoConn, context string) {
	t.Helper()
	select {
	case event := <-c.ch:
		t.Fatalf("%s received an unexpected frame: %v", context, event)
	case <-time.After(300 * time.Millisecond):
	}
}

// rqoAssertNoSensitiveKeys walks an event for prohibited payload keys.
func rqoAssertNoSensitiveKeys(t *testing.T, event map[string]any) {
	t.Helper()
	var walk func(map[string]any)
	walk = func(values map[string]any) {
		for key, value := range values {
			switch key {
			case "grade", "notes", "grade_notes", "media", "credential", "room", "endpoint", "provider", "url":
				t.Fatalf("event leaked prohibited key %q: %v", key, event)
			}
			if child, ok := value.(map[string]any); ok {
				walk(child)
			}
		}
	}
	walk(event)
}

// TestRecitationQueueOptOut_TargetedManagerEventDelivery pins the WS catalog:
// queue.opt_out_requested reaches ONLY current teachers/supervisors (never
// students, never the requester), auto-approved opt-outs emit nothing, and
// the post-decision queue.entry_updated is versioned and redacted through
// the existing outbox→hub projection path.
func TestRecitationQueueOptOut_TargetedManagerEventDelivery(t *testing.T) {
	env := setupRqoEnv(t)
	ctx := context.Background()
	sessionID, state := env.liveRound(t)
	entries := rqcObjects(t, state, "entries")
	requesterEntryID := rqcStr(t, entries[0], "id")

	repo := env.repo
	tickets := realtime.NewTicketService(rqoTicketReader{circleID: env.circleID})
	hub := realtime.NewHub(tickets, rqoAuthorizer{allowed: map[string]bool{
		env.teacherID: true, env.superID: true, env.students[0]: true, env.students[1]: true,
	}})
	projector := queue.NewRealtimeOutboxProjector(repo, hub)
	hub.RegisterSessionEventProvider(projector.QueueState)
	server := httptest.NewServer(hub)
	defer server.Close()

	// Drain the activation events before any client connects so the frame
	// accounting below only covers the opt-out flow.
	dispatcher := queue.NewOutboxDispatcher(repo, projector, nil, nil, nil, nil)
	drain := func() {
		events, err := repo.ClaimDueOutboxEvents(ctx, 100)
		if err != nil {
			t.Fatalf("claim outbox events: %v", err)
		}
		for _, event := range events {
			if err := dispatcher.Dispatch(ctx, event); err != nil {
				t.Fatalf("dispatch %s: %v", event.EventType, err)
			}
		}
	}
	drain()

	teacher := rqoConnect(t, server, tickets, env.teacherID, sessionID)
	defer func() { _ = teacher.conn.Close() }()
	supervisor := rqoConnect(t, server, tickets, env.superID, sessionID)
	defer func() { _ = supervisor.conn.Close() }()
	requester := rqoConnect(t, server, tickets, env.students[0], sessionID)
	defer func() { _ = requester.conn.Close() }()
	other := rqoConnect(t, server, tickets, env.students[1], sessionID)
	defer func() { _ = other.conn.Close() }()

	// Approval-required request: one targeted manager event, nothing for
	// students (the entry does not transition while pending).
	rec := env.req(t, env.students[0], http.MethodPost, "/api/v1/sessions/"+sessionID+"/queue/opt-out", "", "")
	if rec.Code != http.StatusCreated {
		t.Fatalf("opt-out request: got %d body=%s", rec.Code, rec.Body.String())
	}
	request, entry := rqoAssertOptOutResult(t, rqcDecode(t, rec))
	requestID := rqcStr(t, request, "id")
	entryVersion := rqcNum(t, entry, "version")

	events := func() []queue.OutboxEvent {
		t.Helper()
		claimed, err := repo.ClaimDueOutboxEvents(ctx, 100)
		if err != nil {
			t.Fatalf("claim outbox events: %v", err)
		}
		return claimed
	}
	var optOutEvents []queue.OutboxEvent
	claimed := events()
	for _, event := range claimed {
		if event.EventType == "queue.opt_out_requested" {
			optOutEvents = append(optOutEvents, event)
		}
	}
	if len(optOutEvents) != 1 {
		t.Fatalf("queue.opt_out_requested outbox rows = %d, want exactly 1", len(optOutEvents))
	}
	for _, event := range claimed {
		if err := dispatcher.Dispatch(ctx, event); err != nil {
			t.Fatalf("dispatch %s: %v", event.EventType, err)
		}
	}

	for _, manager := range []struct {
		name string
		c    *rqoConn
	}{{"teacher", teacher}, {"supervisor", supervisor}} {
		event := rqoRead(t, manager.c)
		if got := event["type"]; got != "queue.opt_out_requested" {
			t.Fatalf("%s first event = %v, want queue.opt_out_requested", manager.name, got)
		}
		if eventID, ok := event["event_id"].(string); !ok || eventID == "" {
			t.Fatalf("%s opt_out_requested must carry a stable event_id: %v", manager.name, event)
		}
		payload, ok := event["payload"].(map[string]any)
		if !ok {
			t.Fatalf("%s opt_out_requested payload malformed: %v", manager.name, event)
		}
		for _, field := range []string{"session_id", "round_id", "request_id", "queue_entry_id", "student_id", "version"} {
			if _, ok := payload[field]; !ok {
				t.Fatalf("%s opt_out_requested payload missing %q: %v", manager.name, field, payload)
			}
		}
		if got := payload["request_id"]; got != requestID {
			t.Fatalf("%s opt_out_requested request_id = %v, want %q", manager.name, got, requestID)
		}
		if got := payload["queue_entry_id"]; got != requesterEntryID {
			t.Fatalf("%s opt_out_requested queue_entry_id = %v, want %q", manager.name, got, requesterEntryID)
		}
		rqoAssertNoSensitiveKeys(t, event)
	}
	rqoAssertNoEvent(t, requester, "requesting student must not receive queue.opt_out_requested")
	rqoAssertNoEvent(t, other, "other student must not receive queue.opt_out_requested")

	// Decision: the broadcast entry_updated carries the new entry version
	// and no grade/note/media material.
	body := fmt.Sprintf(`{"decision":"approved","expected_entry_version":%d}`, int(entryVersion))
	rec = env.req(t, env.teacherID, http.MethodPost,
		"/api/v1/sessions/"+sessionID+"/queue/opt-out-requests/"+requestID+"/decision", body, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("decision: got %d body=%s", rec.Code, rec.Body.String())
	}
	drain()

	entryUpdated := func(reader string, c *rqoConn) map[string]any {
		t.Helper()
		for {
			event := rqoRead(t, c)
			if event["type"] == "queue.entry_updated" {
				return event
			}
		}
	}
	for _, client := range []struct {
		name string
		c    *rqoConn
	}{{"teacher", teacher}, {"supervisor", supervisor}, {"requester", requester}, {"other", other}} {
		event := entryUpdated(client.name, client.c)
		payload, ok := event["payload"].(map[string]any)
		if !ok {
			t.Fatalf("%s entry_updated payload malformed: %v", client.name, event)
		}
		if got := payload["new_status"]; got != "opted_out" {
			t.Fatalf("%s entry_updated new_status = %v, want opted_out", client.name, got)
		}
		if got := payload["old_status"]; got != "waiting" {
			t.Fatalf("%s entry_updated old_status = %v, want waiting", client.name, got)
		}
		if got, ok := payload["entry_version"].(float64); !ok || got <= entryVersion {
			t.Fatalf("%s entry_updated entry_version = %v, want > %v", client.name, payload["entry_version"], entryVersion)
		}
		if got, ok := payload["version"].(float64); !ok || got < entryVersion {
			t.Fatalf("%s entry_updated version = %v, want a newer committed round version", client.name, payload["version"])
		}
		if got := payload["queue_entry_id"]; got != requesterEntryID {
			t.Fatalf("%s entry_updated queue_entry_id = %v, want %q", client.name, got, requesterEntryID)
		}
		rqoAssertNoSensitiveKeys(t, event)
	}
	rqoAssertNoEvent(t, requester, "no further frames after the decision broadcast")
	rqoAssertNoEvent(t, other, "no further frames after the decision broadcast")
	rqoAssertNoEvent(t, teacher, "no further frames after the decision broadcast")
	rqoAssertNoEvent(t, supervisor, "no further frames after the decision broadcast")
}
