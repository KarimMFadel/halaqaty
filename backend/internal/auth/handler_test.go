package auth

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	phttp "github.com/KarimMFadel/halaqaty/backend/internal/platform/http"
	"github.com/KarimMFadel/halaqaty/backend/internal/platform/httpconst"
)

// stubAuthStore satisfies auth.Store for handler unit tests.
type stubAuthStore struct {
	profile    UserProfile
	upsertErr  error
	createErr  error
	revokeErr  error
	revokedIDs []string
}

func (s *stubAuthStore) UpsertUserByFirebaseUID(context.Context, string, string) (User, bool, error) {
	return User{}, false, s.upsertErr
}

func (s *stubAuthStore) UpsertProfileOnRegister(context.Context, string, string, string) error {
	return nil
}

func (s *stubAuthStore) GetUserProfileByUserID(_ context.Context, userID string) (UserProfile, error) {
	if s.profile.ID == "" {
		return UserProfile{ID: userID, PreferredLanguage: "ar"}, nil
	}
	return s.profile, nil
}

func (s *stubAuthStore) CreateSession(context.Context, Session) error {
	return s.createErr
}

func (s *stubAuthStore) Revoke(_ context.Context, sessionID string, _ time.Time) error {
	if s.revokeErr != nil {
		return s.revokeErr
	}
	s.revokedIDs = append(s.revokedIDs, sessionID)
	return nil
}

func newAuthHandlerRequest(method, target, body string) *http.Request {
	if body == "" {
		return httptest.NewRequest(method, target, nil)
	}
	req := httptest.NewRequest(method, target, strings.NewReader(body))
	req.Header.Set(httpconst.HeaderContentType, httpconst.ContentTypeApplicationJSON)
	return req
}

func withLocalUser(req *http.Request, userID string) *http.Request {
	return req.WithContext(WithPrincipal(req.Context(), AuthPrincipal{UserID: userID}))
}

func decodeAuthErrorEnvelope(t *testing.T, rec *httptest.ResponseRecorder) phttp.ErrorEnvelope {
	t.Helper()
	var env phttp.ErrorEnvelope
	if err := json.NewDecoder(rec.Body).Decode(&env); err != nil {
		t.Fatalf("decode error envelope: %v", err)
	}
	return env
}

func TestCreateSessionHandler_Rejections_MapToStatus(t *testing.T) {
	t.Parallel()

	configured := NewHandler(NewService(&stubAuthStore{}, nil, time.Hour))
	var unconfigured *Handler

	cases := []struct {
		name        string
		handler     *Handler
		principal   bool
		body        string
		wantCode    int
		wantErrCode string
		wantField   string
	}{
		{
			name:        "missing principal returns 401",
			handler:     configured,
			body:        `{}`,
			wantCode:    http.StatusUnauthorized,
			wantErrCode: httpconst.ErrorCodeUnauthorized,
		},
		{
			name:        "unconfigured handler returns 500",
			handler:     unconfigured,
			principal:   true,
			body:        `{}`,
			wantCode:    http.StatusInternalServerError,
			wantErrCode: httpconst.ErrorCodeInternalServerError,
		},
		{
			name:        "malformed body returns 400",
			handler:     configured,
			principal:   true,
			body:        `{bad json`,
			wantCode:    http.StatusBadRequest,
			wantErrCode: httpconst.ErrorCodeValidationFailed,
		},
		{
			name:        "over-length device name returns 400",
			handler:     configured,
			principal:   true,
			body:        `{"device_name":"` + strings.Repeat("d", 101) + `"}`,
			wantCode:    http.StatusBadRequest,
			wantErrCode: httpconst.ErrorCodeValidationFailed,
			wantField:   httpconst.FieldDeviceName,
		},
		{
			name:        "session store failure returns 500",
			handler:     NewHandler(NewService(&stubAuthStore{createErr: errors.New("db down")}, nil, time.Hour)),
			principal:   true,
			body:        `{}`,
			wantCode:    http.StatusInternalServerError,
			wantErrCode: httpconst.ErrorCodeInternalServerError,
		},
		{
			name:        "deleted account returns 401",
			handler:     NewHandler(NewService(&stubAuthStore{createErr: ErrAccountDeleted}, nil, time.Hour)),
			principal:   true,
			body:        `{}`,
			wantCode:    http.StatusUnauthorized,
			wantErrCode: httpconst.ErrorCodeUnauthorized,
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			req := newAuthHandlerRequest(http.MethodPost, "/auth/sessions", tc.body)
			if tc.principal {
				req = withLocalUser(req, "11111111-1111-1111-1111-111111111111")
			}
			rec := httptest.NewRecorder()
			tc.handler.CreateSession(rec, req)

			if rec.Code != tc.wantCode {
				t.Fatalf("status: got %d want %d — body: %s", rec.Code, tc.wantCode, rec.Body.String())
			}
			env := decodeAuthErrorEnvelope(t, rec)
			if env.Error.Code != tc.wantErrCode {
				t.Fatalf("error code: got %q want %q", env.Error.Code, tc.wantErrCode)
			}
			if tc.wantField != "" {
				if _, ok := env.Error.Fields[tc.wantField]; !ok {
					t.Fatalf("expected %q field error, got %v", tc.wantField, env.Error.Fields)
				}
			}
		})
	}
}

func TestRegisterDeletedAccountReturnsUnauthorized(t *testing.T) {
	h := NewHandler(NewService(&stubAuthStore{upsertErr: ErrAccountDeleted}, nil, time.Hour))
	r := newAuthHandlerRequest(http.MethodPost, "/auth/register", `{"display_name":"Student"}`)
	r = r.WithContext(WithPrincipal(r.Context(), AuthPrincipal{FirebaseUID: "deleted-firebase"}))
	w := httptest.NewRecorder()
	h.Register(w, r)
	if w.Code != http.StatusUnauthorized || decodeAuthErrorEnvelope(t, w).Error.Code != httpconst.ErrorCodeUnauthorized {
		t.Fatalf("deleted registration: status=%d body=%s", w.Code, w.Body.String())
	}
}

func TestCreateSessionHandler_ValidRequest_ReturnsSessionWithProfile(t *testing.T) {
	t.Parallel()

	displayName := "Test User"
	store := &stubAuthStore{profile: UserProfile{
		ID: "11111111-1111-1111-1111-111111111111", DisplayName: &displayName, PreferredLanguage: "ar",
	}}
	handler := NewHandler(NewService(store, nil, time.Hour))

	req := withLocalUser(newAuthHandlerRequest(http.MethodPost, "/auth/sessions", `{"device_name":"Pixel 9"}`), store.profile.ID)
	rec := httptest.NewRecorder()
	handler.CreateSession(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status: got %d want 200 — body: %s", rec.Code, rec.Body.String())
	}
	var response BackendSessionResponse
	if err := json.NewDecoder(rec.Body).Decode(&response); err != nil {
		t.Fatalf("decode session response: %v", err)
	}
	if response.SessionID == "" || response.ExpiresAt.IsZero() {
		t.Fatalf("incomplete session payload: %+v", response)
	}
	if response.User.ID != store.profile.ID || response.User.DisplayName == nil || *response.User.DisplayName != displayName {
		t.Fatalf("profile projection: %+v", response.User)
	}
}

func TestDeleteMeRequiresConfirmationAndRecentAuthentication(t *testing.T) {
	store := &accountDeletionStoreStub{uid: "firebase-user"}
	admin := &accountAdminStub{}
	deletion := NewAccountDeletionService(store, admin)
	now := time.Now().UTC()
	deletion.nowFn = func() time.Time { return now }
	h := NewHandler(nil)
	h.SetAccountDeletionService(deletion)

	request := func(body string, authTime time.Time) *httptest.ResponseRecorder {
		r := httptest.NewRequest(http.MethodDelete, "/auth/me", strings.NewReader(body))
		r.Header.Set("Content-Type", "application/json")
		r = r.WithContext(WithPrincipal(r.Context(), AuthPrincipal{UserID: "local-user", AuthTime: authTime}))
		w := httptest.NewRecorder()
		h.DeleteMe(w, r)
		return w
	}
	if w := request(`{"confirm":false}`, now); w.Code != http.StatusBadRequest {
		t.Fatalf("missing confirmation: got %d want 400", w.Code)
	}
	if w := request(`{"confirm":true}`, now.Add(-6*time.Minute)); w.Code != http.StatusUnauthorized {
		t.Fatalf("stale authentication: got %d want 401", w.Code)
	}
	if len(store.closed) != 0 {
		t.Fatalf("rejected requests closed account: %v", store.closed)
	}
	if w := request(`{"confirm":true}`, now); w.Code != http.StatusNoContent {
		t.Fatalf("confirmed deletion: got %d want 204", w.Code)
	}
}

func TestDeleteMeReturnsServiceUnavailableBeforeClosure(t *testing.T) {
	store := &accountDeletionStoreStub{closeErr: errors.New("database unavailable")}
	deletion := NewAccountDeletionService(store, &accountAdminStub{})
	h := NewHandler(nil)
	h.SetAccountDeletionService(deletion)
	r := httptest.NewRequest(http.MethodDelete, "/auth/me", strings.NewReader(`{"confirm":true}`))
	r.Header.Set("Content-Type", "application/json")
	r = r.WithContext(WithPrincipal(r.Context(), AuthPrincipal{UserID: "local-user", AuthTime: time.Now().UTC()}))
	w := httptest.NewRecorder()
	h.DeleteMe(w, r)
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("pre-closure store failure: got %d want 503", w.Code)
	}
	if decodeAuthErrorEnvelope(t, w).Error.Code != httpconst.ErrorCodeServiceUnavailable {
		t.Fatalf("pre-closure failure used wrong error code: %s", w.Body.String())
	}
}

func TestDeleteMeTombstoneRaceReturnsUnauthorized(t *testing.T) {
	h := NewHandler(nil)
	h.SetAccountDeletionService(NewAccountDeletionService(
		&accountDeletionStoreStub{closeErr: ErrAccountDeleted}, &accountAdminStub{}))
	r := newAuthHandlerRequest(http.MethodDelete, "/auth/me", `{"confirm":true}`)
	r = r.WithContext(WithPrincipal(r.Context(), AuthPrincipal{UserID: "local-user", AuthTime: time.Now().UTC()}))
	w := httptest.NewRecorder()
	h.DeleteMe(w, r)
	if w.Code != http.StatusUnauthorized || decodeAuthErrorEnvelope(t, w).Error.Code != httpconst.ErrorCodeUnauthorized {
		t.Fatalf("tombstone race: status=%d body=%s", w.Code, w.Body.String())
	}
}

func TestDeleteMePendingLogsRequestAndStage(t *testing.T) {
	var logs bytes.Buffer
	h := NewHandler(nil)
	h.SetLogger(slog.New(slog.NewJSONHandler(&logs, nil)))
	h.SetAccountDeletionService(NewAccountDeletionService(
		&accountDeletionStoreStub{uid: "firebase-user"},
		&accountAdminStub{err: errors.New("Firebase unavailable")},
	))
	r := newAuthHandlerRequest(http.MethodDelete, "/auth/me", `{"confirm":true}`)
	ctx := context.WithValue(r.Context(), phttp.RequestIDContextKey{}, "request-123")
	r = r.WithContext(WithPrincipal(ctx, AuthPrincipal{UserID: "local-user", AuthTime: time.Now().UTC()}))
	w := httptest.NewRecorder()
	h.DeleteMe(w, r)
	if w.Code != http.StatusAccepted || !strings.Contains(logs.String(), `"request_id":"request-123"`) || !strings.Contains(logs.String(), `"stage":"firebase_identity_removal"`) {
		t.Fatalf("pending outcome missing correlated stage log: status=%d logs=%s", w.Code, logs.String())
	}
}

func TestLogoutHandler_Rejections_MapToStatus(t *testing.T) {
	t.Parallel()

	userID := "11111111-1111-1111-1111-111111111111"
	configured := NewHandler(NewService(&stubAuthStore{}, nil, time.Hour))
	var unconfigured *Handler

	cases := []struct {
		name        string
		handler     *Handler
		principal   bool
		header      string
		wantCode    int
		wantErrCode string
	}{
		{
			name:        "missing principal returns 401",
			handler:     configured,
			header:      "22222222-2222-2222-2222-222222222222",
			wantCode:    http.StatusUnauthorized,
			wantErrCode: httpconst.ErrorCodeUnauthorized,
		},
		{
			name:        "unconfigured handler returns 500",
			handler:     unconfigured,
			principal:   true,
			header:      "22222222-2222-2222-2222-222222222222",
			wantCode:    http.StatusInternalServerError,
			wantErrCode: httpconst.ErrorCodeInternalServerError,
		},
		{
			name:        "missing session header returns 401 session missing",
			handler:     configured,
			principal:   true,
			wantCode:    http.StatusUnauthorized,
			wantErrCode: httpconst.ErrorCodeSessionMissing,
		},
		{
			name:        "whitespace session header returns 401 session missing",
			handler:     configured,
			principal:   true,
			header:      "   ",
			wantCode:    http.StatusUnauthorized,
			wantErrCode: httpconst.ErrorCodeSessionMissing,
		},
		{
			name: "revoke failure returns 500",
			handler: NewHandler(NewService(
				&stubAuthStore{revokeErr: errors.New("db down")}, nil, time.Hour,
			)),
			principal:   true,
			header:      "22222222-2222-2222-2222-222222222222",
			wantCode:    http.StatusInternalServerError,
			wantErrCode: httpconst.ErrorCodeInternalServerError,
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			req := newAuthHandlerRequest(http.MethodPost, "/auth/logout", "")
			if tc.principal {
				req = withLocalUser(req, userID)
			}
			req.Header.Set(httpconst.HeaderSessionID, tc.header)
			rec := httptest.NewRecorder()
			tc.handler.Logout(rec, req)

			if rec.Code != tc.wantCode {
				t.Fatalf("status: got %d want %d — body: %s", rec.Code, tc.wantCode, rec.Body.String())
			}
			if env := decodeAuthErrorEnvelope(t, rec); env.Error.Code != tc.wantErrCode {
				t.Fatalf("error code: got %q want %q", env.Error.Code, tc.wantErrCode)
			}
		})
	}
}

func TestLogoutHandler_ValidSession_RevokesExactlyThatSession(t *testing.T) {
	t.Parallel()

	store := &stubAuthStore{}
	handler := NewHandler(NewService(store, nil, time.Hour))
	sessionID := "33333333-3333-3333-3333-333333333333"

	req := withLocalUser(newAuthHandlerRequest(http.MethodPost, "/auth/logout", ""), "11111111-1111-1111-1111-111111111111")
	req.Header.Set(httpconst.HeaderSessionID, sessionID)
	rec := httptest.NewRecorder()
	handler.Logout(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("status: got %d want 204 — body: %s", rec.Code, rec.Body.String())
	}
	if len(store.revokedIDs) != 1 || store.revokedIDs[0] != sessionID {
		t.Fatalf("revoked sessions: got %v want exactly [%s]", store.revokedIDs, sessionID)
	}
}
