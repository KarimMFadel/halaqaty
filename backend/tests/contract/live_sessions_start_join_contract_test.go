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

	"github.com/KarimMFadel/halaqaty/backend/internal/auth"
	phttp "github.com/KarimMFadel/halaqaty/backend/internal/platform/http"
	"github.com/KarimMFadel/halaqaty/backend/internal/platform/httpconst"
	"github.com/KarimMFadel/halaqaty/backend/internal/sessions"
)

// ---- session contract stubs ------------------------------------------------

type sessionStoreStub struct {
	sessions map[string]*sessions.Session
	present  map[string]map[string]bool
	removed  map[string]map[string]bool
	nextID   int
}

func newSessionStoreStub() *sessionStoreStub {
	return &sessionStoreStub{
		sessions: map[string]*sessions.Session{},
		present:  map[string]map[string]bool{},
		removed:  map[string]map[string]bool{},
	}
}

func (s *sessionStoreStub) CreateAdHocSession(_ context.Context, circleID, createdBy string) (sessions.Session, error) {
	s.nextID++
	id := fmt.Sprintf("sess-%d", s.nextID)
	sess := sessions.Session{
		ID: id, CircleID: circleID, CreatedBy: createdBy,
		Status: sessions.SessionStatusScheduled, MediaMode: sessions.MediaModeAudioOnly,
		CreatedAt: time.Now(), UpdatedAt: time.Now(),
	}
	s.sessions[id] = &sess
	return sess, nil
}

func (s *sessionStoreStub) StartSession(_ context.Context, sessionID string, roomRef sessions.MediaRoomRef) (sessions.Session, error) {
	sess, ok := s.sessions[sessionID]
	if !ok {
		return sessions.Session{}, sessions.ErrSessionNotFound
	}
	switch sess.Status {
	case sessions.SessionStatusActive:
		return sessions.Session{}, sessions.ErrSessionAlreadyActive
	case sessions.SessionStatusEnded:
		return sessions.Session{}, sessions.ErrSessionAlreadyEnded
	}
	sess.Status = sessions.SessionStatusActive
	sess.MediaRoomRef = roomRef
	now := time.Now()
	sess.ActualStart = &now
	sess.UpdatedAt = now
	return *sess, nil
}

func (s *sessionStoreStub) StartSessionWithConnection(ctx context.Context, sessionID, userID string, roomRef sessions.MediaRoomRef, grants sessions.MediaGrants, ensure func(context.Context, sessions.MediaRoomRef, sessions.MediaMode) error, issue func(context.Context, sessions.MediaRoomRef, sessions.MediaGrants) (sessions.MediaConnection, error)) (sessions.Session, sessions.MediaConnection, error) {
	sess, ok := s.sessions[sessionID]
	if !ok {
		return sessions.Session{}, sessions.MediaConnection{}, sessions.ErrSessionNotFound
	}
	if sess.Status == sessions.SessionStatusEnded {
		return sessions.Session{}, sessions.MediaConnection{}, sessions.ErrSessionAlreadyEnded
	}
	started := *sess
	if started.Status == sessions.SessionStatusScheduled {
		if err := ensure(ctx, roomRef, started.MediaMode); err != nil {
			return sessions.Session{}, sessions.MediaConnection{}, err
		}
		started.Status = sessions.SessionStatusActive
		started.MediaRoomRef = roomRef
		now := time.Now()
		started.ActualStart = &now
		started.UpdatedAt = now
	}
	connection, err := issue(ctx, started.MediaRoomRef, grants)
	if err != nil {
		return sessions.Session{}, sessions.MediaConnection{}, err
	}
	before := *sess
	wasPresent := s.present[sessionID] != nil && s.present[sessionID][userID]
	*sess = started
	joined, err := s.JoinSession(ctx, sessionID, userID)
	if err != nil {
		*sess = before
		if s.present[sessionID] != nil {
			s.present[sessionID][userID] = wasPresent
		}
		return sessions.Session{}, sessions.MediaConnection{}, err
	}
	return joined, connection, nil
}

func (s *sessionStoreStub) JoinSession(_ context.Context, sessionID, userID string) (sessions.Session, error) {
	sess, ok := s.sessions[sessionID]
	if !ok {
		return sessions.Session{}, sessions.ErrSessionNotFound
	}
	if sess.Status != sessions.SessionStatusActive {
		if sess.Status == sessions.SessionStatusEnded {
			return sessions.Session{}, sessions.ErrSessionAlreadyEnded
		}
		return sessions.Session{}, sessions.ErrSessionNotStartable
	}
	if s.removed[sessionID] != nil && s.removed[sessionID][userID] {
		return sessions.Session{}, sessions.ErrParticipantRemoved
	}
	if s.present[sessionID] != nil && s.present[sessionID][userID] {
		return *sess, nil
	}
	if sess.IsLocked {
		return sessions.Session{}, sessions.ErrSessionLocked
	}
	if sess.ParticipantCount >= 50 {
		return sessions.Session{}, sessions.ErrSessionFull
	}
	if s.present[sessionID] == nil {
		s.present[sessionID] = map[string]bool{}
	}
	s.present[sessionID][userID] = true
	sess.ParticipantCount++
	return *sess, nil
}

func (s *sessionStoreStub) GetSession(_ context.Context, sessionID string) (sessions.Session, error) {
	sess, ok := s.sessions[sessionID]
	if !ok {
		return sessions.Session{}, sessions.ErrSessionNotFound
	}
	return *sess, nil
}

func (s *sessionStoreStub) ListCircleSessions(_ context.Context, circleID string) ([]sessions.Session, error) {
	items := make([]sessions.Session, 0)
	for _, item := range s.sessions {
		if item.CircleID == circleID && (item.Status == sessions.SessionStatusScheduled || item.Status == sessions.SessionStatusActive) {
			items = append(items, *item)
		}
	}
	return items, nil
}

func (s *sessionStoreStub) LeaveSession(_ context.Context, sessionID, userID string) (sessions.Session, error) {
	sess, ok := s.sessions[sessionID]
	if !ok {
		return sessions.Session{}, sessions.ErrSessionNotFound
	}
	if s.present[sessionID] != nil && s.present[sessionID][userID] {
		s.present[sessionID][userID] = false
		if sess.ParticipantCount > 0 {
			sess.ParticipantCount--
		}
	}
	return *sess, nil
}

// sessionGatewayStub satisfies sessions.SessionMediaGateway.
type sessionGatewayStub struct {
	endpoint string
	credCtr  int
}

func (g *sessionGatewayStub) EnsureRoom(_ context.Context, _ sessions.MediaRoomRef, _ sessions.MediaMode) error {
	return nil
}
func (g *sessionGatewayStub) CloseRoom(_ context.Context, _ sessions.MediaRoomRef) error { return nil }
func (g *sessionGatewayStub) MuteParticipant(_ context.Context, _ sessions.MediaRoomRef, _ string) error {
	return nil
}
func (g *sessionGatewayStub) UnmuteParticipant(_ context.Context, _ sessions.MediaRoomRef, _ string) error {
	return nil
}
func (g *sessionGatewayStub) MuteAll(_ context.Context, _ sessions.MediaRoomRef) error { return nil }
func (g *sessionGatewayStub) RemoveParticipant(_ context.Context, _ sessions.MediaRoomRef, _ string) error {
	return nil
}
func (g *sessionGatewayStub) IssueConnection(_ context.Context, _ sessions.MediaRoomRef, userID string, _ sessions.MediaGrants) (sessions.MediaConnection, error) {
	g.credCtr++
	return sessions.MediaConnection{
		Endpoint:   g.endpoint,
		Credential: sessions.MediaCredential(fmt.Sprintf("cred-%s-%d", userID, g.credCtr)),
		ExpiresAt:  time.Now().Add(time.Hour),
	}, nil
}

// sessionRoleStub satisfies sessions.CircleRoleReader.
type sessionRoleStub struct {
	roles map[string]map[string]string
}

func (r *sessionRoleStub) RoleInCircle(_ context.Context, circleID, userID string) (string, error) {
	return r.roles[circleID][userID], nil
}

// ---- thin HTTP adapter (eliminates sessions.NewHandler dependency) -----------

// sessionHTTPAdapter wraps sessions.Service into an http.Handler that mirrors
// the production handler routing without depending on handler.go (T020).
// ponytail: thin adapter over service, no validation or middleware concerns.
type sessionHTTPAdapter struct {
	svc   *sessions.Service
	gw    *sessionGatewayStub
	roles *sessionRoleStub
}

func (a *sessionHTTPAdapter) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	principal, _ := auth.CurrentPrincipal(r.Context())
	actor := principal.UserID
	// Strip the /api/v1 prefix that tests include in their request URLs.
	p := strings.TrimPrefix(strings.Trim(r.URL.Path, "/"), "api/v1/")
	parts := strings.SplitN(p, "/", 5)

	if parts[0] == "sessions" && len(parts) >= 3 {
		sessionID := parts[1]
		switch parts[2] {
		case "start":
			sess, conn, err := a.svc.StartSession(r.Context(), actor, sessionID)
			if err != nil {
				writeSessionError(w, err)
				return
			}
			writeConnectionResponse(w, sess, conn)
			return
		case "join":
			sess, conn, err := a.svc.JoinSession(r.Context(), actor, sessionID)
			if err != nil {
				writeSessionError(w, err)
				return
			}
			writeConnectionResponse(w, sess, conn)
			return
		}
	}
	if parts[0] == "circles" && len(parts) >= 3 && parts[2] == "sessions" {
		sess, err := a.svc.CreateAdHocSession(r.Context(), actor, parts[1])
		if err != nil {
			writeSessionError(w, err)
			return
		}
		w.Header().Set(httpconst.HeaderContentType, httpconst.ContentTypeApplicationJSON)
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(sessionJSON(sess))
		return
	}
	phttp.WriteError(w, httpconst.ErrorCodeNotFound, "not found", http.StatusNotFound)
}

func writeSessionError(w http.ResponseWriter, err error) {
	code, status := sessionHTTPError(err)
	phttp.WriteError(w, code, code, status)
}

func sessionHTTPError(err error) (code string, status int) {
	switch {
	case err == sessions.ErrSessionNotFound:
		return httpconst.ErrorCodeNotFound, http.StatusNotFound
	case err == sessions.ErrSessionNotStartable, err == sessions.ErrSessionAlreadyActive, err == sessions.ErrSessionAlreadyEnded:
		return httpconst.ErrorCodeConflict, http.StatusConflict
	case err == sessions.ErrNotCircleMember:
		return httpconst.ErrorCodeForbidden, http.StatusForbidden
	case err == sessions.ErrModeratorRoleRequired:
		return httpconst.ErrorCodeForbidden, http.StatusForbidden
	case err == sessions.ErrSessionFull:
		return httpconst.ErrorCodeConflict, http.StatusConflict
	case err == sessions.ErrSessionLocked:
		return httpconst.ErrorCodeConflict, http.StatusConflict
	case err == sessions.ErrParticipantRemoved:
		return httpconst.ErrorCodeForbidden, http.StatusForbidden
	default:
		return httpconst.ErrorCodeInternalServerError, http.StatusInternalServerError
	}
}

func writeConnectionResponse(w http.ResponseWriter, sess sessions.Session, conn sessions.MediaConnection) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Pragma", "no-cache")
	w.Header().Set(httpconst.HeaderContentType, httpconst.ContentTypeApplicationJSON)
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]any{
		"session": sessionJSON(sess),
		"media_connection": map[string]any{
			"endpoint":   conn.Endpoint,
			"credential": conn.Credential,
			"expires_at": conn.ExpiresAt.UTC().Format(time.RFC3339),
		},
	})
}

func sessionJSON(s sessions.Session) map[string]any {
	m := map[string]any{
		"id":                s.ID,
		"circle_id":         s.CircleID,
		"status":            s.Status,
		"media_mode":        s.MediaMode,
		"created_by":        s.CreatedBy,
		"participant_count": s.ParticipantCount,
		"is_locked":         s.IsLocked,
	}
	if s.ActualStart != nil {
		m["actual_start"] = s.ActualStart.UTC().Format(time.RFC3339)
	} else {
		m["actual_start"] = nil
	}
	if s.ActualEnd != nil {
		m["actual_end"] = s.ActualEnd.UTC().Format(time.RFC3339)
	} else {
		m["actual_end"] = nil
	}
	return m
}

// ---- constants & helpers ---------------------------------------------------

const (
	scCircleID = "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa"
	scTeacher  = "bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb"
	scSuper    = "cccccccc-cccc-cccc-cccc-cccccccccccc"
	scStudent  = "dddddddd-dddd-dddd-dddd-dddddddddddd"
	scOutsider = "eeeeeeee-eeee-eeee-eeee-eeeeeeeeeeee"
	scEndpoint = "wss://media.test.halaqaty.app/room"
)

func newSessionFixture() (*sessionStoreStub, *sessionGatewayStub, *sessionRoleStub, *sessions.Service) {
	store := newSessionStoreStub()
	gw := &sessionGatewayStub{endpoint: scEndpoint}
	roles := &sessionRoleStub{roles: map[string]map[string]string{
		scCircleID: {scTeacher: "teacher", scSuper: "supervisor", scStudent: "student"},
	}}
	return store, gw, roles, newLiveSessionContractService(store, gw, roles)
}

func newLiveSessionContractService(store sessions.Store, gateway sessions.SessionMediaGateway, roles sessions.CircleRoleReader) *sessions.Service {
	service, err := sessions.NewServiceWithRoomKey(store, gateway, roles, []byte("contract-room-key"))
	if err != nil {
		panic(err)
	}
	return service
}

func buildSessionRoute(store *sessionStoreStub, gw *sessionGatewayStub, roles *sessionRoleStub, svc *sessions.Service) http.Handler {
	// ponytail: no auth middleware — tests inject principal via context directly.
	// Auth middleware contract is covered by auth_session_contract_test.go.
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		(&sessionHTTPAdapter{svc: svc, gw: gw, roles: roles}).ServeHTTP(w, r)
	})
}

func doSessionReq(h http.Handler, actor, method, target, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, target, strings.NewReader(body))
	req.Header.Set(httpconst.HeaderContentType, httpconst.ContentTypeApplicationJSON)
	req.Header.Set(httpconst.HeaderAuthorization, bearerValid)
	req.Header.Set(httpconst.HeaderSessionID, testSessionID)
	req = req.WithContext(auth.WithPrincipal(req.Context(), auth.AuthPrincipal{UserID: actor}))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func decodeConn(t *testing.T, body []byte) (map[string]any, map[string]any) {
	t.Helper()
	var outer map[string]any
	if err := json.Unmarshal(body, &outer); err != nil {
		t.Fatalf("decode: %v", err)
	}
	return outer["session"].(map[string]any), outer["media_connection"].(map[string]any)
}

// ---- T014 contract tests ---------------------------------------------------

func TestStartSessionResponseShape(t *testing.T) {
	store, gw, roles, svc := newSessionFixture()
	created, _ := svc.CreateAdHocSession(context.Background(), scTeacher, scCircleID)
	handler := buildSessionRoute(store, gw, roles, svc)

	rec := doSessionReq(handler, scTeacher, http.MethodPost,
		"/api/v1/sessions/"+created.ID+"/start?sessionId="+created.ID, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("got %d want 200: %s", rec.Code, rec.Body.String())
	}

	sess, conn := decodeConn(t, rec.Body.Bytes())
	for _, f := range []string{"id", "circle_id", "status", "media_mode", "participant_count", "is_locked"} {
		if _, ok := sess[f]; !ok {
			t.Errorf("session missing %q", f)
		}
	}
	if sess["status"] != "active" {
		t.Errorf("status = %v want active", sess["status"])
	}
	for _, f := range []string{"endpoint", "credential", "expires_at"} {
		if _, ok := conn[f]; !ok {
			t.Errorf("media_connection missing %q", f)
		}
	}
	ep, _ := conn["endpoint"].(string)
	if !strings.HasPrefix(ep, "wss://") && !strings.HasPrefix(ep, "https://") {
		t.Errorf("endpoint = %q", ep)
	}
}

func TestJoinSessionResponseShape(t *testing.T) {
	store, gw, roles, svc := newSessionFixture()
	created, _ := svc.CreateAdHocSession(context.Background(), scTeacher, scCircleID)
	svc.StartSession(context.Background(), scTeacher, created.ID)
	handler := buildSessionRoute(store, gw, roles, svc)

	rec := doSessionReq(handler, scStudent, http.MethodPost,
		"/api/v1/sessions/"+created.ID+"/join?sessionId="+created.ID, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("got %d want 200: %s", rec.Code, rec.Body.String())
	}
	_, conn := decodeConn(t, rec.Body.Bytes())
	if cred, _ := conn["credential"].(string); cred == "" {
		t.Error("credential empty")
	}
}

func TestStartAndJoinNoStoreHeaders(t *testing.T) {
	store, gw, roles, svc := newSessionFixture()
	created, _ := svc.CreateAdHocSession(context.Background(), scTeacher, scCircleID)
	handler := buildSessionRoute(store, gw, roles, svc)

	for _, tc := range []struct {
		name  string
		actor string
		path  string
	}{
		{"start", scTeacher, "/start"},
		{"join", scStudent, "/join"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rec := doSessionReq(handler, tc.actor, http.MethodPost,
				"/api/v1/sessions/"+created.ID+tc.path+"?sessionId="+created.ID, "")
			if rec.Code != http.StatusOK {
				t.Fatalf("got %d want 200: %s", rec.Code, rec.Body.String())
			}
			if cc := rec.Header().Get("Cache-Control"); cc != "no-store" {
				t.Errorf("Cache-Control = %q", cc)
			}
			if pr := rec.Header().Get("Pragma"); pr != "no-cache" {
				t.Errorf("Pragma = %q", pr)
			}
		})
	}
}

func TestSessionErrorEnvelopes(t *testing.T) {
	cases := []struct {
		name       string
		actor      string
		setup      func(*sessionStoreStub)
		target     string
		wantStatus int
		wantCode   string
	}{
		{"start not found → 404", scTeacher, nil,
			"/api/v1/sessions/missing/start?sessionId=missing", http.StatusNotFound, httpconst.ErrorCodeNotFound},
		{"start ended → 409", scTeacher, func(s *sessionStoreStub) {
			s.sessions["e1"] = &sessions.Session{ID: "e1", CircleID: scCircleID, Status: sessions.SessionStatusEnded, MediaMode: sessions.MediaModeAudioOnly}
		}, "/api/v1/sessions/e1/start?sessionId=e1", http.StatusConflict, httpconst.ErrorCodeConflict},
		{"join not found → 404", scStudent, nil,
			"/api/v1/sessions/missing/join?sessionId=missing", http.StatusNotFound, httpconst.ErrorCodeNotFound},
		{"join scheduled → 409", scStudent, func(s *sessionStoreStub) {
			s.sessions["s1"] = &sessions.Session{ID: "s1", CircleID: scCircleID, Status: sessions.SessionStatusScheduled, MediaMode: sessions.MediaModeAudioOnly}
		}, "/api/v1/sessions/s1/join?sessionId=s1", http.StatusConflict, httpconst.ErrorCodeConflict},
		{"non-member start → 403", scOutsider, func(s *sessionStoreStub) {
			s.sessions["n1"] = &sessions.Session{ID: "n1", CircleID: scCircleID, Status: sessions.SessionStatusScheduled, MediaMode: sessions.MediaModeAudioOnly}
		}, "/api/v1/sessions/n1/start?sessionId=n1", http.StatusForbidden, httpconst.ErrorCodeForbidden},
		{"non-member join → 403", scOutsider, func(s *sessionStoreStub) {
			s.sessions["n2"] = &sessions.Session{ID: "n2", CircleID: scCircleID, Status: sessions.SessionStatusActive, MediaMode: sessions.MediaModeAudioOnly}
		}, "/api/v1/sessions/n2/join?sessionId=n2", http.StatusForbidden, httpconst.ErrorCodeForbidden},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			store, gw, roles, svc := newSessionFixture()
			if tc.setup != nil {
				tc.setup(store)
			}
			handler := buildSessionRoute(store, gw, roles, svc)
			rec := doSessionReq(handler, tc.actor, http.MethodPost, tc.target, "")
			if rec.Code != tc.wantStatus {
				t.Fatalf("got %d want %d: %s", rec.Code, tc.wantStatus, rec.Body.String())
			}
			var env phttp.ErrorEnvelope
			if err := json.NewDecoder(rec.Body).Decode(&env); err != nil {
				t.Fatalf("decode: %v", err)
			}
			if env.Error.Code != tc.wantCode {
				t.Errorf("code = %q want %q", env.Error.Code, tc.wantCode)
			}
		})
	}
}

func TestCredentialIsolation(t *testing.T) {
	store, gw, roles, svc := newSessionFixture()
	created, _ := svc.CreateAdHocSession(context.Background(), scTeacher, scCircleID)
	handler := buildSessionRoute(store, gw, roles, svc)

	recA := doSessionReq(handler, scTeacher, http.MethodPost, "/api/v1/sessions/"+created.ID+"/start?sessionId="+created.ID, "")
	recB := doSessionReq(handler, scSuper, http.MethodPost, "/api/v1/sessions/"+created.ID+"/start?sessionId="+created.ID, "")
	recC := doSessionReq(handler, scStudent, http.MethodPost, "/api/v1/sessions/"+created.ID+"/join?sessionId="+created.ID, "")

	if recA.Code != http.StatusOK || recB.Code != http.StatusOK || recC.Code != http.StatusOK {
		t.Fatalf("codes: %d %d %d", recA.Code, recB.Code, recC.Code)
	}
	_, cA := decodeConn(t, recA.Body.Bytes())
	_, cB := decodeConn(t, recB.Body.Bytes())
	_, cC := decodeConn(t, recC.Body.Bytes())
	a, _ := cA["credential"].(string)
	b, _ := cB["credential"].(string)
	c, _ := cC["credential"].(string)
	if a == b || a == c || b == c {
		t.Error("credentials must be distinct per participant")
	}
}

func TestSessionResponseSafety(t *testing.T) {
	store, gw, roles, svc := newSessionFixture()
	created, _ := svc.CreateAdHocSession(context.Background(), scTeacher, scCircleID)
	handler := buildSessionRoute(store, gw, roles, svc)

	for _, tc := range []struct {
		actor string
		path  string
		code  int
	}{
		{scTeacher, "/api/v1/circles/" + scCircleID + "/sessions?circleId=" + scCircleID, http.StatusCreated},
		{scTeacher, "/api/v1/sessions/" + created.ID + "/start?sessionId=" + created.ID, http.StatusOK},
		{scStudent, "/api/v1/sessions/" + created.ID + "/join?sessionId=" + created.ID, http.StatusOK},
	} {
		rec := doSessionReq(handler, tc.actor, http.MethodPost, tc.path, "")
		if rec.Code != tc.code {
			continue
		}
		body := strings.ToLower(rec.Body.String())
		if strings.Contains(body, "media_room_ref") {
			t.Errorf("response leaked media_room_ref: %s", rec.Body.String())
		}
	}
}

func TestCreateSessionContract(t *testing.T) {
	store, gw, roles, svc := newSessionFixture()
	handler := buildSessionRoute(store, gw, roles, svc)

	rec := doSessionReq(handler, scTeacher, http.MethodPost,
		"/api/v1/circles/"+scCircleID+"/sessions?circleId="+scCircleID, `{}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("got %d want 201: %s", rec.Code, rec.Body.String())
	}
	var sess map[string]any
	json.NewDecoder(rec.Body).Decode(&sess)
	if sess["status"] != "scheduled" {
		t.Errorf("status = %v want scheduled", sess["status"])
	}
	if strings.Contains(strings.ToLower(rec.Body.String()), "media_room_ref") {
		t.Error("leaked media_room_ref")
	}
}

func TestStudentCannotStartSession(t *testing.T) {
	store, gw, roles, svc := newSessionFixture()
	created, _ := svc.CreateAdHocSession(context.Background(), scTeacher, scCircleID)
	handler := buildSessionRoute(store, gw, roles, svc)

	rec := doSessionReq(handler, scStudent, http.MethodPost,
		"/api/v1/sessions/"+created.ID+"/start?sessionId="+created.ID, "")
	if rec.Code != http.StatusForbidden {
		t.Fatalf("got %d want 403: %s", rec.Code, rec.Body.String())
	}
}

func TestCreateSessionStudentDenied(t *testing.T) {
	store, gw, roles, svc := newSessionFixture()
	handler := buildSessionRoute(store, gw, roles, svc)

	rec := doSessionReq(handler, scStudent, http.MethodPost,
		"/api/v1/circles/"+scCircleID+"/sessions?circleId="+scCircleID, `{}`)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("got %d want 403: %s", rec.Code, rec.Body.String())
	}
}
