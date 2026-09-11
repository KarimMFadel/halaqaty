//go:build integration

package integration

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/KarimMFadel/halaqaty/backend/internal/auth"
	"github.com/KarimMFadel/halaqaty/backend/internal/chat"
	"github.com/KarimMFadel/halaqaty/backend/internal/middleware"
	phttp "github.com/KarimMFadel/halaqaty/backend/internal/platform/http"
	"github.com/KarimMFadel/halaqaty/backend/internal/platform/httpconst"
	"github.com/KarimMFadel/halaqaty/backend/internal/platform/metrics"
	"github.com/KarimMFadel/halaqaty/backend/internal/rbac"
)

func TestChatLifecycleSecurity_CurrentDeviceSessionRequired(t *testing.T) {
	env := setupChatLifecycleEnv(t)
	circle := env.createCircle(t, "creator", `{"name":"Chat Session Security","is_private":true}`)
	path := "/api/v1/circles/" + circle.ID + "/messages"

	tests := []struct {
		name    string
		headers map[string]string
		code    string
	}{
		{name: "missing firebase identity", headers: map[string]string{httpconst.HeaderSessionID: env.sessions["creator"]}, code: httpconst.ErrorCodeUnauthorized},
		{name: "invalid firebase identity", headers: map[string]string{httpconst.HeaderAuthorization: "Bearer invalid", httpconst.HeaderSessionID: env.sessions["creator"]}, code: httpconst.ErrorCodeUnauthorized},
		{name: "missing backend session", headers: map[string]string{httpconst.HeaderAuthorization: env.tokens["creator"]}, code: httpconst.ErrorCodeSessionMissing},
		{name: "invalid backend session", headers: map[string]string{httpconst.HeaderAuthorization: env.tokens["creator"], httpconst.HeaderSessionID: uuid.NewString()}, code: httpconst.ErrorCodeSessionNotFound},
		{name: "mismatched backend session", headers: map[string]string{httpconst.HeaderAuthorization: env.tokens["creator"], httpconst.HeaderSessionID: env.sessions["student"]}, code: httpconst.ErrorCodeSessionUserMismatch},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			response := doJSONRequest(t, env.mux, http.MethodGet, path, "", tt.headers)
			assertChatError(t, response.Code, response.Body.Bytes(), http.StatusUnauthorized, tt.code)
		})
	}

	if _, err := env.pool.Exec(context.Background(), `UPDATE user_sessions SET revoked_at = NOW() WHERE session_id = $1`, env.sessions["teacher_a"]); err != nil {
		t.Fatalf("revoke backend session: %v", err)
	}
	response := doJSONRequest(t, env.mux, http.MethodGet, path, "", map[string]string{
		httpconst.HeaderAuthorization: env.tokens["teacher_a"],
		httpconst.HeaderSessionID:     env.sessions["teacher_a"],
	})
	assertChatError(t, response.Code, response.Body.Bytes(), http.StatusUnauthorized, httpconst.ErrorCodeSessionRevoked)
}

func TestChatLifecycleSecurity_RemovalRejoinAndArchive(t *testing.T) {
	env := setupChatLifecycleEnv(t)
	ctx := context.Background()
	circle := env.createCircle(t, "creator", `{"name":"Chat Membership Lifecycle","is_private":true}`)
	circleID := uuid.MustParse(circle.ID)
	creatorID := uuid.MustParse(env.userIDs["creator"])
	studentID := uuid.MustParse(env.userIDs["student"])
	if err := env.circleService.AddStudentMember(ctx, circle.ID, env.userIDs["student"]); err != nil {
		t.Fatalf("add initial student membership: %v", err)
	}

	joinedAt := time.Now().UTC().Add(-time.Hour).Truncate(time.Microsecond)
	if _, err := env.pool.Exec(ctx, `UPDATE circle_members SET joined_at = $1 WHERE circle_id = $2 AND user_id = $3`, joinedAt, circleID, studentID); err != nil {
		t.Fatalf("set initial membership period: %v", err)
	}
	beforeRemoval := seedLifecycleMessage(t, env, creatorID, circleID, joinedAt.Add(time.Minute), "first membership")

	historyPath := "/api/v1/circles/" + circle.ID + "/messages"
	studentHeaders := map[string]string{
		httpconst.HeaderAuthorization: env.tokens["student"],
		httpconst.HeaderSessionID:     env.sessions["student"],
	}
	assertLifecycleHistory(t, doJSONRequest(t, env.mux, http.MethodGet, historyPath, "", studentHeaders), beforeRemoval)

	if err := env.circleRepo.RemoveMember(ctx, circle.ID, env.userIDs["student"]); err != nil {
		t.Fatalf("remove student: %v", err)
	}
	removed := doJSONRequest(t, env.mux, http.MethodGet, historyPath, "", studentHeaders)
	unknown := doJSONRequest(t, env.mux, http.MethodGet, "/api/v1/circles/"+uuid.NewString()+"/messages", "", studentHeaders)
	if removed.Code != http.StatusForbidden || unknown.Code != http.StatusForbidden || removed.Body.String() != unknown.Body.String() {
		t.Fatalf("removed and unknown circles must be non-enumerating: removed=%d %s unknown=%d %s", removed.Code, removed.Body.String(), unknown.Code, unknown.Body.String())
	}

	rejoinedAt := time.Now().UTC().Add(time.Minute).Truncate(time.Microsecond)
	if _, err := env.pool.Exec(ctx, `INSERT INTO circle_members (circle_id, user_id, role, joined_at) VALUES ($1, $2, 'student', $3)`, circleID, studentID, rejoinedAt); err != nil {
		t.Fatalf("rejoin student: %v", err)
	}
	afterRejoin := seedLifecycleMessage(t, env, creatorID, circleID, rejoinedAt.Add(time.Minute), "second membership")
	assertLifecycleHistory(t, doJSONRequest(t, env.mux, http.MethodGet, historyPath, "", studentHeaders), afterRejoin)

	if err := env.circleRepo.ArchiveCircle(ctx, circle.ID); err != nil {
		t.Fatalf("archive circle: %v", err)
	}
	assertLifecycleHistory(t, doJSONRequest(t, env.mux, http.MethodGet, historyPath, "", studentHeaders), afterRejoin)
	send := doJSONRequest(t, env.mux, http.MethodPost, historyPath, `{"message_type":"text","content":"archived write"}`, map[string]string{
		httpconst.HeaderAuthorization:  env.tokens["student"],
		httpconst.HeaderSessionID:      env.sessions["student"],
		httpconst.HeaderContentType:    httpconst.ContentTypeApplicationJSON,
		httpconst.HeaderIdempotencyKey: "archived-write",
	})
	assertChatError(t, send.Code, send.Body.Bytes(), http.StatusConflict, httpconst.ErrorCodeConflict)
}

type chatLifecycleEnv struct {
	*circleRoleEnv
	circleRepo    *rbac.Repository
	circleService *rbac.Service
}

func setupChatLifecycleEnv(t *testing.T) *chatLifecycleEnv {
	t.Helper()
	base := setupCircleRoleEnv(t)
	ctx := context.Background()
	conn := acquireConn(t, base.pool, ctx)
	for _, migration := range []string{"000016_live_sessions.up.sql", "000017_recitation_queue_system.up.sql", "000018_real_time_chat.up.sql"} {
		runMigrationFile(t, conn, ctx, migration)
	}
	conn.Release()

	tokens := make(map[string]*auth.DecodedToken, len(circleRoleUsers))
	for _, user := range circleRoleUsers {
		tokens[user+"-token"] = &auth.DecodedToken{UID: "firebase-" + user, Email: user + "@halaqaty.app"}
	}
	sessionRepo := auth.NewSessionRepository(base.pool)
	authMW := middleware.NewAuthMiddleware(&circleTokenVerifier{tokens: tokens}, auth.NewSessionService(24*time.Hour), sessionRepo)
	circleRepo := rbac.NewRepository(base.pool)
	service := chat.NewGroupService(chat.NewRepository(base.pool), circleRepo, &metrics.ChatMetrics{}, nil)
	handler := chat.NewGroupHandler(service)
	base.mux.Handle("GET /api/v1/circles/{circleId}/messages", authMW.Require(http.HandlerFunc(handler.ListCircleMessages)))
	base.mux.Handle("POST /api/v1/circles/{circleId}/messages", authMW.Require(http.HandlerFunc(handler.SendCircleMessage)))
	return &chatLifecycleEnv{circleRoleEnv: base, circleRepo: circleRepo, circleService: base.svc}
}

func seedLifecycleMessage(t *testing.T, env *chatLifecycleEnv, senderID, circleID uuid.UUID, sentAt time.Time, content string) uuid.UUID {
	t.Helper()
	var id uuid.UUID
	if err := env.pool.QueryRow(context.Background(), `
		INSERT INTO messages (circle_id, sender_id, idempotency_key, message_type, content, sent_at)
		VALUES ($1, $2, $3, 'text', $4, $5)
		RETURNING id
	`, circleID, senderID, uuid.NewString(), content, sentAt).Scan(&id); err != nil {
		t.Fatalf("seed lifecycle message: %v", err)
	}
	return id
}

func assertLifecycleHistory(t *testing.T, response *httptest.ResponseRecorder, wantID uuid.UUID) {
	t.Helper()
	result := response.Result()
	defer result.Body.Close()
	if result.StatusCode != http.StatusOK {
		t.Fatalf("history status=%d, want 200", result.StatusCode)
	}
	var payload struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.NewDecoder(result.Body).Decode(&payload); err != nil {
		t.Fatalf("decode history: %v", err)
	}
	if len(payload.Data) != 1 || payload.Data[0].ID != wantID.String() {
		t.Fatalf("history IDs=%v, want only %s", payload.Data, wantID)
	}
}

func assertChatError(t *testing.T, gotStatus int, body []byte, wantStatus int, wantCode string) {
	t.Helper()
	if gotStatus != wantStatus {
		t.Fatalf("status=%d, want %d body=%s", gotStatus, wantStatus, body)
	}
	var envelope phttp.ErrorEnvelope
	if err := json.Unmarshal(body, &envelope); err != nil {
		t.Fatalf("decode error envelope: %v", err)
	}
	if envelope.Error.Code != wantCode {
		t.Fatalf("error code=%q, want %q body=%s", envelope.Error.Code, wantCode, body)
	}
}
