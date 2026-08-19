//go:build contract

package contract

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"

	"github.com/KarimMFadel/halaqaty/backend/internal/auth"
	phttp "github.com/KarimMFadel/halaqaty/backend/internal/platform/http"
	"github.com/KarimMFadel/halaqaty/backend/internal/platform/httpconst"
	"github.com/KarimMFadel/halaqaty/backend/internal/realtime"
	"github.com/KarimMFadel/halaqaty/backend/internal/sessions"
)

// T028 — moderation/presence REST and WebSocket response-safety, RBAC-denial,
// and no-provider-leak contract tests against the production sessions handler
// and realtime hub. The canonical sources are the F-005 sections of
// docs/contracts/openapi.yaml and docs/contracts/ws_events.md.

const (
	scStudent2     = "12121212-1212-1212-1212-121212121212"
	scOtherCircle  = "34343434-3434-3434-3434-343434343434"
	scOtherTeacher = "56565656-5656-5656-5656-565656565656"
)

// ---- moderation store stub ---------------------------------------------------

// modStoreStub extends the start/join store stub with the moderation port so
// the service's moderation methods work in-memory.
type modStoreStub struct {
	*sessionStoreStub

	displayNames map[string]string
	rolesByUser  map[string]string
	hands        map[string]map[string]time.Time
	failLock     bool
}

func newModStoreStub() *modStoreStub {
	return &modStoreStub{
		sessionStoreStub: newSessionStoreStub(),
		displayNames:     map[string]string{},
		rolesByUser:      map[string]string{},
		hands:            map[string]map[string]time.Time{},
	}
}

// CreateAdHocSession rekeys the inherited stub session to a real UUID: the
// production handler validates the session path parameter.
func (s *modStoreStub) CreateAdHocSession(ctx context.Context, circleID, createdBy string) (sessions.Session, error) {
	sess, err := s.sessionStoreStub.CreateAdHocSession(ctx, circleID, createdBy)
	if err != nil {
		return sess, err
	}
	id := uuid.NewString()
	stored := s.sessions[sess.ID]
	delete(s.sessions, sess.ID)
	stored.ID = id
	s.sessions[id] = stored
	return *stored, nil
}

func (s *modStoreStub) SetLock(_ context.Context, sessionID string, locked bool) (sessions.Session, error) {
	sess, ok := s.sessions[sessionID]
	if !ok {
		return sessions.Session{}, sessions.ErrSessionNotFound
	}
	if s.failLock {
		return sessions.Session{}, fmt.Errorf("set lock: database exploded")
	}
	if sess.Status != sessions.SessionStatusActive {
		return sessions.Session{}, sessions.ErrSessionAlreadyEnded
	}
	sess.IsLocked = locked
	sess.UpdatedAt = time.Now()
	return *sess, nil
}

func (s *modStoreStub) EndSession(_ context.Context, sessionID string, reason sessions.EndReason) (sessions.Session, error) {
	sess, ok := s.sessions[sessionID]
	if !ok {
		return sessions.Session{}, sessions.ErrSessionNotFound
	}
	if sess.Status == sessions.SessionStatusEnded {
		return sessions.Session{}, sessions.ErrSessionAlreadyEnded
	}
	if sess.Status != sessions.SessionStatusActive {
		return sessions.Session{}, sessions.ErrSessionNotStartable
	}
	now := time.Now()
	sess.Status = sessions.SessionStatusEnded
	sess.ActualEnd = &now
	sess.EndReason = reason
	for userID := range s.present[sessionID] {
		s.present[sessionID][userID] = false
		delete(s.hands[sessionID], userID)
	}
	return *sess, nil
}

func (s *modStoreStub) ReconnectPresence(_ context.Context, sessionID, userID string) (sessions.Session, error) {
	sess, ok := s.sessions[sessionID]
	if !ok {
		return sessions.Session{}, sessions.ErrSessionNotFound
	}
	if s.removed[sessionID] != nil && s.removed[sessionID][userID] {
		return sessions.Session{}, sessions.ErrParticipantRemoved
	}
	if s.present[sessionID] == nil {
		s.present[sessionID] = map[string]bool{}
	}
	s.present[sessionID][userID] = true
	return *sess, nil
}

func (s *modStoreStub) RemoveParticipant(_ context.Context, sessionID, userID string) (sessions.Session, error) {
	sess, ok := s.sessions[sessionID]
	if !ok {
		return sessions.Session{}, sessions.ErrSessionNotFound
	}
	if s.present[sessionID] == nil || !s.present[sessionID][userID] {
		return sessions.Session{}, sessions.ErrParticipantRemoved
	}
	s.present[sessionID][userID] = false
	if s.removed[sessionID] == nil {
		s.removed[sessionID] = map[string]bool{}
	}
	s.removed[sessionID][userID] = true
	delete(s.hands[sessionID], userID)
	if sess.ParticipantCount > 0 {
		sess.ParticipantCount--
	}
	return *sess, nil
}

func (s *modStoreStub) SetHandRaised(_ context.Context, sessionID, userID string) error {
	return s.setHand(sessionID, userID, time.Now())
}

func (s *modStoreStub) SetHandLowered(_ context.Context, sessionID, userID string) error {
	return s.setHand(sessionID, userID, time.Time{})
}

func (s *modStoreStub) setHand(sessionID, userID string, at time.Time) error {
	if s.present[sessionID] == nil || !s.present[sessionID][userID] {
		return sessions.ErrParticipantRemoved
	}
	if s.removed[sessionID] != nil && s.removed[sessionID][userID] {
		return sessions.ErrParticipantRemoved
	}
	if s.hands[sessionID] == nil {
		s.hands[sessionID] = map[string]time.Time{}
	}
	s.hands[sessionID][userID] = at
	return nil
}

func (s *modStoreStub) ListSessionParticipants(_ context.Context, sessionID string) ([]sessions.ParticipantPresence, error) {
	if _, ok := s.sessions[sessionID]; !ok {
		return nil, sessions.ErrSessionNotFound
	}
	rows := []sessions.ParticipantPresence{}
	for userID, present := range s.present[sessionID] {
		removed := s.removed[sessionID] != nil && s.removed[sessionID][userID]
		row := sessions.ParticipantPresence{
			SessionID: sessionID, UserID: userID,
			DisplayName: s.displayName(userID),
			Role:        s.role(userID),
		}
		if hand, ok := s.hands[sessionID][userID]; ok && !hand.IsZero() {
			raised := hand
			row.HandRaisedAt = &raised
		}
		if present && !removed {
			row.IsCurrentlyPresent = true
			rows = append(rows, row)
		}
	}
	return rows, nil
}

func (s *modStoreStub) displayName(userID string) string {
	if name, ok := s.displayNames[userID]; ok {
		return name
	}
	return "Member"
}

func (s *modStoreStub) role(userID string) string {
	if role, ok := s.rolesByUser[userID]; ok {
		return role
	}
	return "student"
}

// modGatewayStub records every moderation media operation.
type modGatewayStub struct {
	endpoint string
	muted    []string
	unmuted  []string
	muteAll  int
	removed  []string
	closed   []string
}

func (g *modGatewayStub) EnsureRoom(_ context.Context, _ sessions.MediaRoomRef, _ sessions.MediaMode) error {
	return nil
}
func (g *modGatewayStub) CloseRoom(_ context.Context, room sessions.MediaRoomRef) error {
	g.closed = append(g.closed, string(room))
	return nil
}
func (g *modGatewayStub) MuteParticipant(_ context.Context, _ sessions.MediaRoomRef, userID string) error {
	g.muted = append(g.muted, userID)
	return nil
}
func (g *modGatewayStub) UnmuteParticipant(_ context.Context, _ sessions.MediaRoomRef, userID string) error {
	g.unmuted = append(g.unmuted, userID)
	return nil
}
func (g *modGatewayStub) MuteAll(_ context.Context, _ sessions.MediaRoomRef) error {
	g.muteAll++
	return nil
}
func (g *modGatewayStub) RemoveParticipant(_ context.Context, _ sessions.MediaRoomRef, userID string) error {
	g.removed = append(g.removed, userID)
	return nil
}
func (g *modGatewayStub) IssueConnection(_ context.Context, _ sessions.MediaRoomRef, userID string, _ sessions.MediaGrants) (sessions.MediaConnection, error) {
	return sessions.MediaConnection{
		Endpoint:   g.endpoint,
		Credential: sessions.MediaCredential("mod-cred-" + userID),
		ExpiresAt:  time.Now().Add(time.Hour),
	}, nil
}

// ---- fixtures ------------------------------------------------------------------

func newModerationFixture() (*modStoreStub, *modGatewayStub, *sessions.Service, *sessions.Handler) {
	store := newModStoreStub()
	store.displayNames[scTeacher] = "Ustaza Sara"
	store.displayNames[scStudent] = "Ahmed"
	store.rolesByUser = map[string]string{scTeacher: "teacher", scSuper: "supervisor", scStudent: "student", scStudent2: "student"}
	gw := &modGatewayStub{endpoint: scEndpoint}
	roles := &sessionRoleStub{roles: map[string]map[string]string{
		scCircleID:    {scTeacher: "teacher", scSuper: "supervisor", scStudent: "student", scStudent2: "student"},
		scOtherCircle: {scOtherTeacher: "teacher"},
	}}
	svc := sessions.NewService(store, gw, roles)
	return store, gw, svc, sessions.NewHandler(svc)
}

// startModeratedSession creates, starts, and joins one teacher and two students.
func startModeratedSession(t *testing.T, svc *sessions.Service) sessions.Session {
	t.Helper()
	ctx := context.Background()
	created, err := svc.CreateAdHocSession(ctx, scTeacher, scCircleID)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if _, _, err := svc.StartSession(ctx, scTeacher, created.ID); err != nil {
		t.Fatalf("start: %v", err)
	}
	if _, _, err := svc.JoinSession(ctx, scStudent, created.ID); err != nil {
		t.Fatalf("student join: %v", err)
	}
	if _, _, err := svc.JoinSession(ctx, scStudent2, created.ID); err != nil {
		t.Fatalf("student2 join: %v", err)
	}
	return created
}

func doModReqNoAuth(h http.Handler, method, target, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, target, strings.NewReader(body))
	req.Header.Set(httpconst.HeaderContentType, httpconst.ContentTypeApplicationJSON)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

// assertNoProviderLeak fails when a body carries provider names or private
// media material. Moderation responses must never contain them.
func assertNoProviderLeak(t *testing.T, label, body string) {
	t.Helper()
	lower := strings.ToLower(body)
	for _, banned := range []string{"livekit", "credential", "media_connection", "media_room_ref", "token"} {
		if strings.Contains(lower, banned) {
			t.Errorf("%s leaks %q: %s", label, banned, body)
		}
	}
}

// ---- lock / end / mute / remove contracts --------------------------------------

func TestModerationLockUnlockContract(t *testing.T) {
	_, _, svc, handler := newModerationFixture()
	created := startModeratedSession(t, svc)

	for _, tc := range []struct {
		locked bool
	}{
		{true}, {true}, {false},
	} {
		rec := doSessionReq(handler, scTeacher, http.MethodPost,
			"/api/v1/sessions/"+created.ID+"/lock", fmt.Sprintf(`{"locked":%t}`, tc.locked))
		if rec.Code != http.StatusOK {
			t.Fatalf("lock(%t) = %d: %s", tc.locked, rec.Code, rec.Body.String())
		}
		var sess map[string]any
		if err := json.Unmarshal(rec.Body.Bytes(), &sess); err != nil {
			t.Fatalf("decode: %v", err)
		}
		for _, f := range []string{"id", "circle_id", "status", "media_mode", "created_by", "participant_count", "is_locked", "actual_start", "actual_end", "end_reason"} {
			if _, ok := sess[f]; !ok {
				t.Errorf("session missing contract field %q", f)
			}
		}
		if sess["is_locked"] != tc.locked {
			t.Errorf("is_locked = %v, want %v", sess["is_locked"], tc.locked)
		}
		if sess["status"] != "active" {
			t.Errorf("status = %v, want active", sess["status"])
		}
		assertNoProviderLeak(t, "lock response", rec.Body.String())
	}
}

func TestModerationEndSessionContract(t *testing.T) {
	_, gw, svc, handler := newModerationFixture()
	created := startModeratedSession(t, svc)

	rec := doSessionReq(handler, scTeacher, http.MethodPost,
		"/api/v1/sessions/"+created.ID+"/end", `{"end_reason":"manual"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("end = %d: %s", rec.Code, rec.Body.String())
	}
	var sess map[string]any
	json.Unmarshal(rec.Body.Bytes(), &sess)
	if sess["status"] != "ended" {
		t.Errorf("status = %v, want ended", sess["status"])
	}
	if sess["end_reason"] != "manual" {
		t.Errorf("end_reason = %v, want manual", sess["end_reason"])
	}
	if sess["actual_end"] == nil {
		t.Error("actual_end must be set on the ended session")
	}
	if len(gw.closed) != 1 {
		t.Errorf("provider room closed %d times, want 1", len(gw.closed))
	}
	assertNoProviderLeak(t, "end response", rec.Body.String())
}

func TestModerationMuteAndRemoveContract(t *testing.T) {
	_, gw, svc, handler := newModerationFixture()
	created := startModeratedSession(t, svc)

	noContent := func(label, target string) {
		t.Helper()
		rec := doSessionReq(handler, scTeacher, http.MethodPost, target, "")
		if rec.Code != http.StatusNoContent {
			t.Fatalf("%s = %d: %s", label, rec.Code, rec.Body.String())
		}
		if rec.Body.Len() != 0 {
			t.Errorf("%s must return an empty body: %s", label, rec.Body.String())
		}
	}

	noContent("mute-all", "/api/v1/sessions/"+created.ID+"/participants/mute-all")
	noContent("mute", "/api/v1/sessions/"+created.ID+"/participants/"+scStudent+"/mute")
	noContent("unmute", "/api/v1/sessions/"+created.ID+"/participants/"+scStudent+"/unmute")
	noContent("remove", "/api/v1/sessions/"+created.ID+"/participants/"+scStudent2+"/remove")

	if gw.muteAll != 1 {
		t.Errorf("mute-all gateway calls = %d, want 1", gw.muteAll)
	}
	if len(gw.muted) != 1 || gw.muted[0] != scStudent {
		t.Errorf("muted = %v, want [%s]", gw.muted, scStudent)
	}
	if len(gw.unmuted) != 1 || gw.unmuted[0] != scStudent {
		t.Errorf("unmuted = %v, want [%s]", gw.unmuted, scStudent)
	}
	if len(gw.removed) != 1 || gw.removed[0] != scStudent2 {
		t.Errorf("removed = %v, want [%s]", gw.removed, scStudent2)
	}
}

func TestModerationParticipantsSnapshotContract(t *testing.T) {
	_, _, svc, handler := newModerationFixture()
	created := startModeratedSession(t, svc)
	if err := svc.SetHand(context.Background(), scStudent, created.ID, true); err != nil {
		t.Fatalf("raise hand: %v", err)
	}

	// A student (member, non-moderator) may read the snapshot.
	rec := doSessionReq(handler, scStudent, http.MethodGet, "/api/v1/sessions/"+created.ID+"/participants", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("participants = %d: %s", rec.Code, rec.Body.String())
	}
	var resp struct {
		Data []map[string]any `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(resp.Data) != 3 {
		t.Fatalf("data len = %d, want 3 (teacher + 2 students)", len(resp.Data))
	}
	byUser := map[string]map[string]any{}
	for _, p := range resp.Data {
		byUser[p["user_id"].(string)] = p
	}
	for userID, wantRole := range map[string]string{scTeacher: "teacher", scStudent: "student", scStudent2: "student"} {
		p, ok := byUser[userID]
		if !ok {
			t.Fatalf("participant %s missing from snapshot", userID)
		}
		// The canonical SessionParticipant schema has exactly five fields.
		if len(p) != 5 {
			t.Errorf("participant %s has %d fields, want 5: %v", userID, len(p), p)
		}
		for _, f := range []string{"user_id", "display_name", "role", "is_currently_present", "hand_raised_at"} {
			if _, ok := p[f]; !ok {
				t.Errorf("participant %s missing contract field %q", userID, f)
			}
		}
		if p["role"] != wantRole {
			t.Errorf("participant %s role = %v, want %v", userID, p["role"], wantRole)
		}
		if p["is_currently_present"] != true {
			t.Errorf("participant %s is_currently_present = %v, want true", userID, p["is_currently_present"])
		}
	}
	if byUser[scTeacher]["display_name"] != "Ustaza Sara" {
		t.Errorf("display_name = %v, want Ustaza Sara", byUser[scTeacher]["display_name"])
	}
	if byUser[scStudent]["hand_raised_at"] == nil {
		t.Error("raised hand must carry a timestamp")
	}
	if byUser[scStudent2]["hand_raised_at"] != nil {
		t.Error("lowered hand must be null")
	}
	assertNoProviderLeak(t, "participants response", rec.Body.String())
}

// ---- RBAC denial ----------------------------------------------------------------

func TestModerationRBACDenialContract(t *testing.T) {
	_, _, svc, handler := newModerationFixture()
	created := startModeratedSession(t, svc)

	studentTargets := []string{
		"/api/v1/sessions/" + created.ID + "/lock",
		"/api/v1/sessions/" + created.ID + "/end",
		"/api/v1/sessions/" + created.ID + "/participants/mute-all",
		"/api/v1/sessions/" + created.ID + "/participants/" + scStudent2 + "/mute",
		"/api/v1/sessions/" + created.ID + "/participants/" + scStudent2 + "/unmute",
		"/api/v1/sessions/" + created.ID + "/participants/" + scStudent2 + "/remove",
	}
	for _, target := range studentTargets {
		rec := doSessionReq(handler, scStudent, http.MethodPost, target, `{}`)
		if rec.Code != http.StatusForbidden {
			t.Errorf("student POST %s = %d, want 403: %s", target, rec.Code, rec.Body.String())
		}
		var env phttp.ErrorEnvelope
		if err := json.Unmarshal(rec.Body.Bytes(), &env); err != nil || env.Error.Code != httpconst.ErrorCodeForbidden {
			t.Errorf("student POST %s envelope = %s", target, rec.Body.String())
		}
		assertNoProviderLeak(t, "rbac denial", rec.Body.String())
	}

	// Cross-circle actor (teacher of another circle) is not a member here.
	outsiderTargets := append(studentTargets, "/api/v1/sessions/"+created.ID+"/participants")
	for _, target := range outsiderTargets {
		method := http.MethodPost
		if strings.HasSuffix(target, "/participants") {
			method = http.MethodGet
		}
		rec := doSessionReq(handler, scOtherTeacher, method, target, `{}`)
		if rec.Code != http.StatusForbidden {
			t.Errorf("cross-circle %s %s = %d, want 403: %s", method, target, rec.Code, rec.Body.String())
		}
	}
}

// ---- error envelope contract ------------------------------------------------------

func TestModerationErrorEnvelopesContract(t *testing.T) {
	store, _, svc, handler := newModerationFixture()
	created := startModeratedSession(t, svc)
	store.failLock = true

	cases := []struct {
		name       string
		actor      string
		method     string
		target     string
		body       string
		wantStatus int
		wantCode   string
	}{
		{"unknown session → 404", scTeacher, http.MethodPost,
			"/api/v1/sessions/99999999-9999-9999-9999-999999999999/lock", `{"locked":true}`,
			http.StatusNotFound, httpconst.ErrorCodeNotFound},
		{"invalid session uuid → 400", scTeacher, http.MethodPost,
			"/api/v1/sessions/not-a-uuid/lock", `{"locked":true}`,
			http.StatusBadRequest, httpconst.ErrorCodeValidationFailed},
		{"malformed body → 400", scTeacher, http.MethodPost,
			"/api/v1/sessions/" + created.ID + "/lock", `{not json`,
			http.StatusBadRequest, httpconst.ErrorCodeValidationFailed},
		{"unknown field → 400", scTeacher, http.MethodPost,
			"/api/v1/sessions/" + created.ID + "/lock", `{"locked":true,"extra":1}`,
			http.StatusBadRequest, httpconst.ErrorCodeValidationFailed},
		{"store failure → 500", scTeacher, http.MethodPost,
			"/api/v1/sessions/" + created.ID + "/lock", `{"locked":true}`,
			http.StatusInternalServerError, httpconst.ErrorCodeInternalServerError},
		{"unauthenticated → 401", "", http.MethodPost,
			"/api/v1/sessions/" + created.ID + "/lock", `{"locked":true}`,
			http.StatusUnauthorized, httpconst.ErrorCodeUnauthorized},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var rec *httptest.ResponseRecorder
			if tc.actor == "" {
				rec = doModReqNoAuth(handler, tc.method, tc.target, tc.body)
			} else {
				rec = doSessionReq(handler, tc.actor, tc.method, tc.target, tc.body)
			}
			if rec.Code != tc.wantStatus {
				t.Fatalf("status = %d, want %d: %s", rec.Code, tc.wantStatus, rec.Body.String())
			}
			var env phttp.ErrorEnvelope
			if err := json.Unmarshal(rec.Body.Bytes(), &env); err != nil {
				t.Fatalf("decode envelope: %v", err)
			}
			if env.Error.Code != tc.wantCode {
				t.Errorf("code = %q, want %q", env.Error.Code, tc.wantCode)
			}
			assertNoProviderLeak(t, "error envelope", rec.Body.String())
		})
	}
}

func TestModerationEndedSessionConflictContract(t *testing.T) {
	_, _, svc, handler := newModerationFixture()
	created := startModeratedSession(t, svc)
	if _, err := svc.EndSession(context.Background(), scTeacher, created.ID, sessions.EndReasonManual); err != nil {
		t.Fatalf("end: %v", err)
	}

	for _, tc := range []struct {
		name   string
		method string
		target string
		body   string
	}{
		{"lock ended → 409", http.MethodPost, "/api/v1/sessions/" + created.ID + "/lock", `{"locked":true}`},
		{"end again → 409", http.MethodPost, "/api/v1/sessions/" + created.ID + "/end", `{"end_reason":"manual"}`},
	} {
		rec := doSessionReq(handler, scTeacher, tc.method, tc.target, tc.body)
		if rec.Code != http.StatusConflict {
			t.Errorf("%s = %d, want 409: %s", tc.name, rec.Code, rec.Body.String())
		}
		var env phttp.ErrorEnvelope
		if err := json.Unmarshal(rec.Body.Bytes(), &env); err != nil || env.Error.Code != httpconst.ErrorCodeConflict {
			t.Errorf("%s envelope = %s", tc.name, rec.Body.String())
		}
	}
}

// ---- realtime ticket contract ------------------------------------------------------

type ticketCirclesStub struct{ circles []string }

func (s *ticketCirclesStub) ListCircleIDs(context.Context, string) ([]string, error) {
	return s.circles, nil
}

func TestModerationRealtimeTicketContract(t *testing.T) {
	tickets := realtime.NewTicketService(&ticketCirclesStub{circles: []string{scCircleID}})
	handler := http.HandlerFunc(realtime.NewHandler(tickets).CreateTicket)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/realtime/tickets", nil)
	req = req.WithContext(auth.WithPrincipal(req.Context(), auth.AuthPrincipal{UserID: scTeacher}))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("ticket = %d: %s", rec.Code, rec.Body.String())
	}
	if cc := rec.Header().Get("Cache-Control"); cc != "no-store" {
		t.Errorf("Cache-Control = %q, want no-store", cc)
	}
	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	token, _ := body["token"].(string)
	if token == "" {
		t.Error("token must be present and opaque")
	}
	expiresAt, _ := body["expires_at"].(string)
	if _, err := time.Parse(time.RFC3339, expiresAt); err != nil {
		t.Errorf("expires_at = %q, want RFC3339", expiresAt)
	}
	// The ticket body legitimately carries its own token; it must still carry
	// no provider names or media material.
	lower := strings.ToLower(rec.Body.String())
	for _, banned := range []string{"livekit", "credential", "media_connection", "media_room_ref"} {
		if strings.Contains(lower, banned) {
			t.Errorf("ticket response leaks %q: %s", banned, rec.Body.String())
		}
	}

	// Unauthenticated callers never receive a ticket.
	rec = doModReqNoAuth(handler, http.MethodPost, "/api/v1/realtime/tickets", "")
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("unauthenticated ticket = %d, want 401", rec.Code)
	}
}

// ---- WebSocket response safety and RBAC ---------------------------------------------

// sessionTopicAuthStub denies one user (simulating a removed participant) and
// admits the other.
type sessionTopicAuthStub struct{ deniedUser string }

func (s *sessionTopicAuthStub) AuthorizeSessionTopic(_ context.Context, userID, _ string) error {
	if userID == s.deniedUser {
		return sessions.ErrParticipantRemoved
	}
	return nil
}

func TestModerationWebSocketResponseSafetyContract(t *testing.T) {
	tickets := realtime.NewTicketService(&ticketCirclesStub{circles: []string{scCircleID}})
	hub := realtime.NewHub(tickets, &sessionTopicAuthStub{deniedUser: scStudent})
	srv := httptest.NewServer(hub)
	defer srv.Close()

	// No ticket: the hub rejects before the upgrade.
	resp, err := http.Get(srv.URL)
	if err != nil {
		t.Fatalf("connect without ticket: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("missing ticket = %d, want 401", resp.StatusCode)
	}

	teacherTicket, err := tickets.Issue(context.Background(), scTeacher)
	if err != nil {
		t.Fatalf("issue teacher ticket: %v", err)
	}
	studentTicket, err := tickets.Issue(context.Background(), scStudent)
	if err != nil {
		t.Fatalf("issue student ticket: %v", err)
	}

	sessionTopic := "session.99999999-9999-9999-9999-999999999999"

	// Removed participant: session topic subscription is denied with a
	// neutral error and no provider material.
	removedConn := dialWS(t, srv, studentTicket.Token)
	defer removedConn.Close()
	writeWS(t, removedConn, map[string]any{"action": "subscribe", "topic": sessionTopic})
	msg := readWS(t, removedConn)
	if errorMsg, _ := msg["error"].(string); errorMsg == "" {
		t.Errorf("removed participant must be denied, got %v", msg)
	}
	assertNoProviderLeak(t, "ws denial", fmt.Sprint(msg))

	// Eligible participant: subscription is confirmed for exactly the
	// requested topic.
	conn := dialWS(t, srv, teacherTicket.Token)
	defer conn.Close()
	writeWS(t, conn, map[string]any{"action": "subscribe", "topic": sessionTopic})
	msg = readWS(t, conn)
	if msg["type"] != "subscribed" || msg["topic"] != sessionTopic {
		t.Errorf("subscribed ack = %v, want type=subscribed topic=%s", msg, sessionTopic)
	}
	assertNoProviderLeak(t, "ws ack", fmt.Sprint(msg))

	writeWS(t, conn, map[string]any{"action": "ping"})
	msg = readWS(t, conn)
	if msg["type"] != "pong" {
		t.Errorf("ping ack = %v, want pong", msg)
	}
}

func dialWS(t *testing.T, srv *httptest.Server, token string) *websocket.Conn {
	t.Helper()
	dialer := websocket.Dialer{HandshakeTimeout: 5 * time.Second}
	url := "ws" + strings.TrimPrefix(srv.URL, "http") + "?token=" + token
	conn, _, err := dialer.Dial(url, nil)
	if err != nil {
		t.Fatalf("dial %s: %v", url, err)
	}
	return conn
}

func writeWS(t *testing.T, conn *websocket.Conn, payload any) {
	t.Helper()
	if err := conn.WriteJSON(payload); err != nil {
		t.Fatalf("write ws message: %v", err)
	}
}

func readWS(t *testing.T, conn *websocket.Conn) map[string]any {
	t.Helper()
	var msg map[string]any
	if err := conn.ReadJSON(&msg); err != nil {
		t.Fatalf("read ws message: %v", err)
	}
	return msg
}
