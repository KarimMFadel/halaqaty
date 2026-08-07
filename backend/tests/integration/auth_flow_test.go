//go:build integration

package integration

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/KarimMFadel/halaqaty/backend/internal/auth"
	"github.com/KarimMFadel/halaqaty/backend/internal/middleware"
	phttp "github.com/KarimMFadel/halaqaty/backend/internal/platform/http"
	"github.com/KarimMFadel/halaqaty/backend/internal/platform/httpconst"
)

const (
	flowFirebaseUID = "firebase-auth-flow-001"
	flowEmail       = "student@halaqaty.app"
	flowUserID      = "44444444-4444-4444-4444-444444444444"
	flowBearer      = "Bearer valid-token"
)

func TestAuthFlow_RegisterSessionProtectedLogout(t *testing.T) {
	repo := newFlowStore()
	verifier := &flowVerifier{
		decoded: &auth.DecodedToken{
			UID:   flowFirebaseUID,
			Email: flowEmail,
		},
	}

	authService := auth.NewService(repo, nil, 24*time.Hour)
	authHandler := auth.NewHandler(authService)
	sessionService := auth.NewSessionService(24 * time.Hour)
	authMW := middleware.NewAuthMiddleware(verifier, sessionService, repo)

	mux := http.NewServeMux()
	mux.Handle("POST /auth/register", authMW.RequireVerifiedFirebase(http.HandlerFunc(authHandler.Register)))
	mux.Handle("POST /auth/sessions", authMW.RequireBearer(http.HandlerFunc(authHandler.CreateSession)))
	mux.Handle("POST /auth/logout", authMW.Require(http.HandlerFunc(authHandler.Logout)))
	mux.Handle("GET /protected", authMW.Require(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"ok":true}`))
	})))

	registerResp := doJSONRequest(t, mux, http.MethodPost, "/auth/register", `{"display_name":"Ali"}`, map[string]string{
		httpconst.HeaderAuthorization: flowBearer,
		httpconst.HeaderContentType:   httpconst.ContentTypeApplicationJSON,
	})
	if registerResp.Code != http.StatusCreated {
		t.Fatalf("register status: got %d want %d body=%s", registerResp.Code, http.StatusCreated, registerResp.Body.String())
	}

	createSessionResp := doJSONRequest(t, mux, http.MethodPost, "/auth/sessions", `{"device_name":"web"}`, map[string]string{
		httpconst.HeaderAuthorization: flowBearer,
		httpconst.HeaderContentType:   httpconst.ContentTypeApplicationJSON,
	})
	if createSessionResp.Code != http.StatusOK {
		t.Fatalf("create session status: got %d want %d body=%s", createSessionResp.Code, http.StatusOK, createSessionResp.Body.String())
	}

	var session auth.BackendSessionResponse
	if err := json.Unmarshal(createSessionResp.Body.Bytes(), &session); err != nil {
		t.Fatalf("decode create session response: %v", err)
	}
	if session.SessionID == "" {
		t.Fatal("expected non-empty session_id from session creation")
	}

	protectedOk := doJSONRequest(t, mux, http.MethodGet, "/protected", "", map[string]string{
		httpconst.HeaderAuthorization: flowBearer,
		httpconst.HeaderSessionID:     session.SessionID,
	})
	if protectedOk.Code != http.StatusOK {
		t.Fatalf("protected before logout status: got %d want %d body=%s", protectedOk.Code, http.StatusOK, protectedOk.Body.String())
	}

	logoutResp := doJSONRequest(t, mux, http.MethodPost, "/auth/logout", "", map[string]string{
		httpconst.HeaderAuthorization: flowBearer,
		httpconst.HeaderSessionID:     session.SessionID,
	})
	if logoutResp.Code != http.StatusNoContent {
		t.Fatalf("logout status: got %d want %d body=%s", logoutResp.Code, http.StatusNoContent, logoutResp.Body.String())
	}

	protectedDenied := doJSONRequest(t, mux, http.MethodGet, "/protected", "", map[string]string{
		httpconst.HeaderAuthorization: flowBearer,
		httpconst.HeaderSessionID:     session.SessionID,
	})
	if protectedDenied.Code != http.StatusUnauthorized {
		t.Fatalf("protected after logout status: got %d want %d body=%s", protectedDenied.Code, http.StatusUnauthorized, protectedDenied.Body.String())
	}

	var env phttp.ErrorEnvelope
	if err := json.Unmarshal(protectedDenied.Body.Bytes(), &env); err != nil {
		t.Fatalf("decode unauthorized error envelope: %v", err)
	}
	if env.Error.Code != httpconst.ErrorCodeSessionRevoked {
		t.Fatalf("error code after logout: got %q want %q", env.Error.Code, httpconst.ErrorCodeSessionRevoked)
	}
}

func doJSONRequest(t *testing.T, handler http.Handler, method, path, body string, headers map[string]string) *httptest.ResponseRecorder {
	t.Helper()

	req := httptest.NewRequest(method, path, bytes.NewBufferString(body))
	for key, value := range headers {
		req.Header.Set(key, value)
	}
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	return rec
}

type flowVerifier struct {
	decoded *auth.DecodedToken
}

func (v *flowVerifier) Verify(_ context.Context, bearerToken string) (*auth.DecodedToken, error) {
	if bearerToken != "valid-token" {
		return nil, errors.New("invalid token")
	}
	return v.decoded, nil
}

type flowStore struct {
	mu          sync.Mutex
	userByUID   map[string]auth.User
	userIDByUID map[string]string
	profiles    map[string]auth.UserProfile
	sessions    map[string]auth.Session
}

func newFlowStore() *flowStore {
	return &flowStore{
		userByUID:   make(map[string]auth.User),
		userIDByUID: make(map[string]string),
		profiles:    make(map[string]auth.UserProfile),
		sessions:    make(map[string]auth.Session),
	}
}

func (s *flowStore) UpsertUserByFirebaseUID(_ context.Context, firebaseUID, email string) (auth.User, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if existing, ok := s.userByUID[firebaseUID]; ok {
		existing.Email = email
		s.userByUID[firebaseUID] = existing
		return existing, false, nil
	}

	user := auth.User{
		ID:          flowUserID,
		FirebaseUID: firebaseUID,
		Email:       email,
		CreatedAt:   time.Now().UTC(),
		UpdatedAt:   time.Now().UTC(),
	}
	s.userByUID[firebaseUID] = user
	s.userIDByUID[firebaseUID] = user.ID
	return user, true, nil
}

func (s *flowStore) UpsertProfileOnRegister(_ context.Context, userID, displayName, preferredLanguage string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	profile, ok := s.profiles[userID]
	if ok {
		return nil
	}
	profile = auth.UserProfile{
		ID:                userID,
		FirebaseUID:       flowFirebaseUID,
		DisplayName:       &displayName,
		PreferredLanguage: preferredLanguage,
		CreatedAt:         time.Now().UTC(),
	}
	s.profiles[userID] = profile
	return nil
}

func (s *flowStore) GetUserProfileByUserID(_ context.Context, userID string) (auth.UserProfile, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	profile, ok := s.profiles[userID]
	if !ok {
		return auth.UserProfile{}, auth.ErrUserNotFound
	}
	return profile, nil
}

func (s *flowStore) CreateSession(_ context.Context, session auth.Session) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sessions[session.ID] = session
	return nil
}

func (s *flowStore) Revoke(_ context.Context, sessionID string, revokedAt time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	session, ok := s.sessions[sessionID]
	if !ok {
		return auth.ErrSessionNotFound
	}
	session.RevokedAt = &revokedAt
	s.sessions[sessionID] = session
	return nil
}

func (s *flowStore) GetByID(_ context.Context, sessionID string) (auth.Session, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	session, ok := s.sessions[sessionID]
	if !ok {
		return auth.Session{}, auth.ErrSessionNotFound
	}
	return session, nil
}

func (s *flowStore) Touch(_ context.Context, sessionID string, lastActivityAt time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	session, ok := s.sessions[sessionID]
	if !ok {
		return auth.ErrSessionNotFound
	}
	session.LastActivityAt = lastActivityAt.UTC()
	s.sessions[sessionID] = session
	return nil
}

func (s *flowStore) GetLocalUserIDByFirebaseUID(_ context.Context, firebaseUID string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	userID, ok := s.userIDByUID[firebaseUID]
	if !ok {
		return "", auth.ErrUserNotFound
	}
	return userID, nil
}
