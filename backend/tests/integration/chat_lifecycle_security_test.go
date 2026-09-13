//go:build integration

package integration

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"

	"github.com/KarimMFadel/halaqaty/backend/internal/auth"
	"github.com/KarimMFadel/halaqaty/backend/internal/chat"
	"github.com/KarimMFadel/halaqaty/backend/internal/middleware"
	phttp "github.com/KarimMFadel/halaqaty/backend/internal/platform/http"
	"github.com/KarimMFadel/halaqaty/backend/internal/platform/httpconst"
	"github.com/KarimMFadel/halaqaty/backend/internal/platform/metrics"
	"github.com/KarimMFadel/halaqaty/backend/internal/rbac"
	"github.com/KarimMFadel/halaqaty/backend/internal/realtime"
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
	search := chat.NewGroupService(chat.NewRepository(env.pool), env.circleRepo, &metrics.ChatMetrics{}, nil)
	results, err := search.Search(ctx, studentID, circleID, "second membership", nil, 50)
	if err != nil || len(results) != 1 || results[0].ID != afterRejoin {
		t.Fatalf("retained membership search: results=%v err=%v want %s", results, err, afterRejoin)
	}

	if err := env.circleRepo.ArchiveCircle(ctx, circle.ID); err != nil {
		t.Fatalf("archive circle: %v", err)
	}
	assertLifecycleHistory(t, doJSONRequest(t, env.mux, http.MethodGet, historyPath, "", studentHeaders), afterRejoin)
	presence := chat.NewPresenceService(chat.NewRepository(env.pool), env.circleRepo)
	readPath := historyPath + "/" + afterRejoin.String() + "/read"
	readResponse := doJSONRequest(t, env.mux, http.MethodPost, readPath, "", map[string]string{
		httpconst.HeaderAuthorization:  env.tokens["student"],
		httpconst.HeaderSessionID:      env.sessions["student"],
		httpconst.HeaderIdempotencyKey: "archived-read",
	})
	assertChatError(t, readResponse.Code, readResponse.Body.Bytes(), http.StatusConflict, httpconst.ErrorCodeConflict)
	if err := presence.MarkGroupMessageRead(ctx, studentID, circleID, afterRejoin); !errors.Is(err, chat.ErrCircleArchived) {
		t.Fatalf("archived mark-read error=%v, want ErrCircleArchived", err)
	}
	var readFacts int
	if err := env.pool.QueryRow(ctx, `SELECT COUNT(*) FROM message_reads WHERE message_id = $1 AND user_id = $2`, afterRejoin, studentID).Scan(&readFacts); err != nil {
		t.Fatalf("count archived mark-read facts: %v", err)
	}
	if readFacts != 0 {
		t.Fatalf("archived mark-read persisted %d read facts, want 0", readFacts)
	}
	var readEvents int
	if err := env.pool.QueryRow(ctx, `SELECT COUNT(*) FROM chat_event_outbox WHERE message_id = $1 AND event_type = $2`, afterRejoin, realtime.EventChatMessageRead).Scan(&readEvents); err != nil {
		t.Fatalf("count archived mark-read outbox events: %v", err)
	}
	if readEvents != 0 {
		t.Fatalf("archived mark-read persisted %d outbox events, want 0", readEvents)
	}
	send := doJSONRequest(t, env.mux, http.MethodPost, historyPath, `{"message_type":"text","content":"archived write"}`, map[string]string{
		httpconst.HeaderAuthorization:  env.tokens["student"],
		httpconst.HeaderSessionID:      env.sessions["student"],
		httpconst.HeaderContentType:    httpconst.ContentTypeApplicationJSON,
		httpconst.HeaderIdempotencyKey: "archived-write",
	})
	assertChatError(t, send.Code, send.Body.Bytes(), http.StatusConflict, httpconst.ErrorCodeConflict)
}

// TestChatLifecycleSecurity_ServiceMutationMatrix keeps the lifecycle rules
// covered below the HTTP layer as well.  The service is the common seam used
// by REST and background callers, so archived and non-member writes must be
// denied identically regardless of transport.
func TestChatLifecycleSecurity_ServiceMutationMatrix(t *testing.T) {
	env := setupChatLifecycleEnv(t)
	ctx := context.Background()
	circle := env.createCircle(t, "creator", `{"name":"Chat Service Lifecycle","is_private":true}`)
	circleID := uuid.MustParse(circle.ID)
	studentID := uuid.MustParse(env.userIDs["student"])
	if err := env.circleService.AddStudentMember(ctx, circle.ID, env.userIDs["student"]); err != nil {
		t.Fatalf("add student membership: %v", err)
	}

	service := chat.NewGroupService(chat.NewRepository(env.pool), env.circleRepo, &metrics.ChatMetrics{}, nil)
	if _, err := service.SendText(ctx, studentID, circleID, "before removal", "lifecycle-service-1"); err != nil {
		t.Fatalf("active member send: %v", err)
	}
	if err := env.circleRepo.RemoveMember(ctx, circle.ID, env.userIDs["student"]); err != nil {
		t.Fatalf("remove student membership: %v", err)
	}
	if _, err := service.SendText(ctx, studentID, circleID, "removed", "lifecycle-service-2"); !errors.Is(err, chat.ErrCircleNotVisible) {
		t.Fatalf("removed member send error=%v, want ErrCircleNotVisible", err)
	}
	if _, err := service.SendText(ctx, studentID, uuid.New(), "unknown", "lifecycle-service-3"); !errors.Is(err, chat.ErrCircleNotVisible) {
		t.Fatalf("unknown circle send error=%v, want ErrCircleNotVisible", err)
	}

	if _, err := env.pool.Exec(ctx, `INSERT INTO circle_members (circle_id, user_id, role, joined_at) VALUES ($1, $2, 'student', NOW())`, circleID, studentID); err != nil {
		t.Fatalf("rejoin student membership: %v", err)
	}
	if err := env.circleRepo.ArchiveCircle(ctx, circle.ID); err != nil {
		t.Fatalf("archive circle: %v", err)
	}
	if _, err := service.SendText(ctx, studentID, circleID, "archived", "lifecycle-service-4"); !errors.Is(err, chat.ErrCircleArchived) {
		t.Fatalf("archived member send error=%v, want ErrCircleArchived", err)
	}
	if _, err := service.History(ctx, studentID, circleID, nil, 50); err != nil {
		t.Fatalf("archived retained history: %v", err)
	}
}

// TestChatLifecycleSecurity_RealtimeAuthorizationRecheckedBeforeEveryWrite
// catches any projector or hub change that reuses the subscription-time
// audience without consulting current PostgreSQL membership and session state.
func TestChatLifecycleSecurity_RealtimeAuthorizationRecheckedBeforeEveryWrite(t *testing.T) {
	tests := []struct {
		name   string
		cutoff func(context.Context, *chatLifecycleEnv, string) error
	}{
		{
			name: "membership removal",
			cutoff: func(ctx context.Context, env *chatLifecycleEnv, circleID string) error {
				return env.circleRepo.RemoveMember(ctx, circleID, env.userIDs["student"])
			},
		},
		{
			name: "backend session revocation",
			cutoff: func(ctx context.Context, env *chatLifecycleEnv, _ string) error {
				return auth.NewSessionRepository(env.pool).Revoke(ctx, env.sessions["student"], time.Now())
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			env := setupChatLifecycleEnv(t)
			ctx := context.Background()
			circle := env.createCircle(t, "creator", `{"name":"Chat Realtime Cutoff","is_private":true}`)
			if err := env.circleService.AddStudentMember(ctx, circle.ID, env.userIDs["student"]); err != nil {
				t.Fatalf("add student membership: %v", err)
			}

			tickets := realtime.NewTicketService(env.circleRepo)
			ticket, err := tickets.IssueForSession(ctx, env.userIDs["student"], env.sessions["student"])
			if err != nil {
				t.Fatalf("issue realtime ticket: %v", err)
			}
			hub := realtime.NewHub(tickets, nil)
			server := httptest.NewServer(hub)
			t.Cleanup(server.Close)
			conn := dialLifecycleRealtime(t, server, ticket.Token)
			t.Cleanup(func() { _ = conn.Close() })
			subscribeLifecycleCircle(t, conn, circle.ID)

			sessionRepo := auth.NewSessionRepository(env.pool)
			projector := chat.NewRealtimeProjector(env.circleRepo, hub, tickets, func(ctx context.Context, sessionID, userID string) (bool, error) {
				session, err := sessionRepo.GetByIDAndUserID(ctx, sessionID, userID)
				if err != nil {
					return false, err
				}
				return session.RevokedAt == nil && time.Now().Before(session.ExpiresAt), nil
			})
			circleID := uuid.MustParse(circle.ID)
			creatorID := uuid.MustParse(env.userIDs["creator"])
			projectLifecycleMessage(t, projector, creatorID, circleID, "before cutoff")
			if got := readLifecycleRealtime(t, conn); got["type"] != realtime.EventChatMessage {
				t.Fatalf("authorized delivery=%v, want %s", got, realtime.EventChatMessage)
			}

			if err := tt.cutoff(ctx, env, circle.ID); err != nil {
				t.Fatalf("apply %s cutoff: %v", tt.name, err)
			}
			projectLifecycleMessage(t, projector, creatorID, circleID, "after cutoff")
			assertNoLifecycleRealtimeEvent(t, conn)
		})
	}
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
	presence := chat.NewPresenceService(chat.NewRepository(base.pool), circleRepo)
	presenceHandler := chat.NewPresenceHandler(presence)
	base.mux.Handle("POST /api/v1/circles/{circleId}/messages/{messageId}/read", authMW.Require(http.HandlerFunc(presenceHandler.MarkCircleMessageRead)))
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

func dialLifecycleRealtime(t *testing.T, server *httptest.Server, token string) *websocket.Conn {
	t.Helper()
	conn, _, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(server.URL, "http")+"?token="+token, nil)
	if err != nil {
		t.Fatalf("dial realtime hub: %v", err)
	}
	return conn
}

func subscribeLifecycleCircle(t *testing.T, conn *websocket.Conn, circleID string) {
	t.Helper()
	if err := conn.WriteJSON(map[string]any{"action": "subscribe", "topic": "circle." + circleID}); err != nil {
		t.Fatalf("subscribe to circle topic: %v", err)
	}
	if got := readLifecycleRealtime(t, conn); got["type"] != "subscribed" {
		t.Fatalf("subscription response=%v, want subscribed", got)
	}
}

func projectLifecycleMessage(t *testing.T, projector *chat.RealtimeProjector, senderID, circleID uuid.UUID, content string) {
	t.Helper()
	messageID := uuid.New()
	message := chat.Message{
		ID:       messageID,
		CircleID: &circleID,
		SenderID: senderID,
		Type:     chat.MessageTypeText,
		Content:  content,
		State:    chat.MessageStateActive,
		SentAt:   time.Now().UTC(),
	}
	if err := projector.ProjectMessage(context.Background(), chat.OutboxEvent{
		EventID:   uuid.New(),
		MessageID: messageID,
		EventType: realtime.EventChatMessage,
	}, message); err != nil {
		t.Fatalf("project realtime message: %v", err)
	}
}

func readLifecycleRealtime(t *testing.T, conn *websocket.Conn) map[string]any {
	t.Helper()
	if err := conn.SetReadDeadline(time.Now().Add(2 * time.Second)); err != nil {
		t.Fatalf("set realtime read deadline: %v", err)
	}
	var message map[string]any
	if err := conn.ReadJSON(&message); err != nil {
		t.Fatalf("read realtime message: %v", err)
	}
	return message
}

func assertNoLifecycleRealtimeEvent(t *testing.T, conn *websocket.Conn) {
	t.Helper()
	if err := conn.SetReadDeadline(time.Now().Add(150 * time.Millisecond)); err != nil {
		t.Fatalf("set realtime cutoff deadline: %v", err)
	}
	if _, _, err := conn.ReadMessage(); err == nil {
		t.Fatal("connection received an event after authorization cutoff")
	} else if !websocket.IsCloseError(err, websocket.CloseNormalClosure, websocket.CloseGoingAway) {
		if networkErr, ok := err.(interface{ Timeout() bool }); !ok || !networkErr.Timeout() {
			t.Fatalf("read realtime cutoff: %v", err)
		}
	} else {
		t.Fatalf("realtime connection closed before cutoff assertion: %v", err)
	}
}
