package sessions

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/KarimMFadel/halaqaty/backend/internal/auth"
)

const (
	hTestCircleUUID  = "11111111-1111-1111-1111-111111111111"
	hTestSessionUUID = "22222222-2222-2222-2222-222222222222"
	hOtherUUID       = "99999999-9999-9999-9999-999999999999"
	hBadUUID         = "not-a-uuid"
)

// newTestHandler wires a handler over the package test fakes: one scheduled
// session in the test circle where teacher/supervisor/student are seeded.
func newTestHandler(t *testing.T) (*Handler, *fakeStore) {
	t.Helper()
	store := newFakeStore()
	gw := &fakeGateway{}
	roles := &fakeRoles{roles: map[string]map[string]string{
		hTestCircleUUID: {us1Teacher: "teacher", us1Super: "supervisor", us1Student: "student"},
	}}
	store.sessions[hTestSessionUUID] = &Session{
		ID: hTestSessionUUID, CircleID: hTestCircleUUID, CreatedBy: us1Teacher,
		Status: SessionStatusScheduled, MediaMode: MediaModeAudioOnly,
	}
	service, err := NewServiceWithRoomKey(store, gw, roles, []byte("test-room-key"))
	if err != nil {
		t.Fatal(err)
	}
	return NewHandler(service), store
}

// doAs runs one principal's request through the handler.
func doAs(handler *Handler, actor, method, target string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, target, nil)
	req = req.WithContext(auth.WithPrincipal(req.Context(), auth.AuthPrincipal{UserID: actor}))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	return rec
}

func decodeMap(t *testing.T, rec *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	body := map[string]any{}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response %q: %v", rec.Body.String(), err)
	}
	return body
}

func TestHandlerCreateSession(t *testing.T) {
	handler, store := newTestHandler(t)
	rec := doAs(handler, us1Teacher, http.MethodPost, "/circles/"+hTestCircleUUID+"/sessions?circleId="+hTestCircleUUID)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create status = %d, want 201 (body %s)", rec.Code, rec.Body.String())
	}
	body := decodeMap(t, rec)
	if body["status"] != "scheduled" || body["media_mode"] != "audio_only" {
		t.Fatalf("created session shape wrong: %v", body)
	}
	if _, leaked := body["media_room_ref"]; leaked {
		t.Fatal("session response must never expose the media room reference")
	}
	if len(store.sessions) != 2 {
		t.Fatalf("create must persist exactly one new session, have %d", len(store.sessions))
	}
}

func TestHandlerCreateSessionStudentDenied(t *testing.T) {
	handler, _ := newTestHandler(t)
	rec := doAs(handler, us1Student, http.MethodPost, "/circles/"+hTestCircleUUID+"/sessions?circleId="+hTestCircleUUID)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("student create status = %d, want 403 (body %s)", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"error"`) {
		t.Fatal("denials must use the standard error envelope")
	}
}

func TestHandlerStartSessionIssuesNoStoreConnection(t *testing.T) {
	handler, _ := newTestHandler(t)
	rec := doAs(handler, us1Teacher, http.MethodPost, "/sessions/"+hTestSessionUUID+"/start?sessionId="+hTestSessionUUID)
	if rec.Code != http.StatusOK {
		t.Fatalf("start status = %d (body %s)", rec.Code, rec.Body.String())
	}
	if got := rec.Header().Get("Cache-Control"); got != "no-store" {
		t.Fatalf("Cache-Control = %q, want no-store", got)
	}
	body := decodeMap(t, rec)
	sess, _ := body["session"].(map[string]any)
	if sess == nil || sess["status"] != "active" {
		t.Fatalf("start session object wrong: %v", body)
	}
	conn, _ := body["media_connection"].(map[string]any)
	if conn == nil || conn["endpoint"] == "" || conn["credential"] == "" || conn["expires_at"] == "" {
		t.Fatalf("media connection incomplete: %v", body)
	}
	if strings.Contains(rec.Body.String(), "media_room_ref") {
		t.Fatal("response leaked the media room reference")
	}
}

func TestHandlerJoinSessionIssuesNoStoreConnection(t *testing.T) {
	handler, store := newTestHandler(t)
	seeded := store.sessions[hTestSessionUUID]
	seeded.Status = SessionStatusActive
	seeded.MediaRoomRef = "room-abc"
	rec := doAs(handler, us1Student, http.MethodPost, "/sessions/"+hTestSessionUUID+"/join?sessionId="+hTestSessionUUID)
	if rec.Code != http.StatusOK {
		t.Fatalf("join status = %d (body %s)", rec.Code, rec.Body.String())
	}
	if got := rec.Header().Get("Cache-Control"); got != "no-store" {
		t.Fatalf("Cache-Control = %q, want no-store", got)
	}
	body := decodeMap(t, rec)
	conn, _ := body["media_connection"].(map[string]any)
	if conn == nil || conn["credential"] == "" {
		t.Fatalf("join must return the caller's own credential: %v", body)
	}
}

func TestHandlerSessionErrorMapping(t *testing.T) {
	handler, store := newTestHandler(t)
	ended := *store.sessions[hTestSessionUUID]
	ended.Status = SessionStatusEnded
	store.sessions[hTestSessionUUID] = &ended

	cases := []struct {
		target string
		want   int
	}{
		{"/sessions/" + hBadUUID + "/start?sessionId=" + hBadUUID, http.StatusBadRequest},
		{"/sessions/" + hTestSessionUUID + "/start?sessionId=" + hTestSessionUUID, http.StatusConflict},
		{"/sessions/" + hOtherUUID + "/join?sessionId=" + hOtherUUID, http.StatusNotFound},
	}
	for _, tc := range cases {
		rec := doAs(handler, us1Teacher, http.MethodPost, tc.target)
		if rec.Code != tc.want {
			t.Fatalf("%s status = %d, want %d (body %s)", tc.target, rec.Code, tc.want, rec.Body.String())
		}
		if !strings.Contains(rec.Body.String(), `"error"`) {
			t.Fatalf("%s must use the standard error envelope: %s", tc.target, rec.Body.String())
		}
	}
}

func TestHandlerMediaWebhook(t *testing.T) {
	handler := NewHandler(nil)
	handler.webhook = &stubWebhookVerifier{event: MediaWebhookEvent{ID: "evt-1", Type: EventParticipantJoined, RoomRef: "r"}}
	rec := httptest.NewRecorder()
	handler.HandleMediaWebhook(rec, httptest.NewRequest(http.MethodPost, "/webhooks/livekit", strings.NewReader("{}")))
	if rec.Code != http.StatusNoContent {
		t.Fatalf("webhook status = %d, want 204", rec.Code)
	}

	handler.webhook = &stubWebhookVerifier{err: ErrWebhookSignature}
	rec = httptest.NewRecorder()
	handler.HandleMediaWebhook(rec, httptest.NewRequest(http.MethodPost, "/webhooks/livekit", strings.NewReader("{}")))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("bad signature status = %d, want 401", rec.Code)
	}
}

// stubWebhookVerifier fakes the neutral verifier port.
type stubWebhookVerifier struct {
	event MediaWebhookEvent
	err   error
}

func (s *stubWebhookVerifier) Verify(_ *http.Request) (MediaWebhookEvent, error) {
	return s.event, s.err
}
