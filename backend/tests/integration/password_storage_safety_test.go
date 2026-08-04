//go:build integration

// Package integration contains feature-001 integration tests.
// PasswordStorageSafety tests assert that the Go backend never receives,
// logs, stores, or returns plaintext passwords in any path.
package integration

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/KarimMFadel/halaqaty/backend/internal/auth"
	"github.com/KarimMFadel/halaqaty/backend/internal/middleware"
	phttp "github.com/KarimMFadel/halaqaty/backend/internal/platform/http"
	"github.com/KarimMFadel/halaqaty/backend/internal/platform/httpconst"
)

// passwordSafetyStore records every value written to the store so tests can
// assert that no plaintext password was persisted.
type passwordSafetyStore struct {
	upsertCalls []upsertArgs
	profileArgs []profileArgs
	sessions    []auth.Session
}

type upsertArgs struct{ uid, email string }
type profileArgs struct{ userID, displayName, lang string }

func (s *passwordSafetyStore) UpsertUserByFirebaseUID(_ context.Context, uid, email string) (auth.User, bool, error) {
	s.upsertCalls = append(s.upsertCalls, upsertArgs{uid, email})
	return auth.User{ID: "safety-user", FirebaseUID: uid, Email: email}, true, nil
}

func (s *passwordSafetyStore) UpsertProfileOnRegister(_ context.Context, userID, displayName, lang string) error {
	s.profileArgs = append(s.profileArgs, profileArgs{userID, displayName, lang})
	return nil
}

func (s *passwordSafetyStore) GetUserProfileByUserID(_ context.Context, _ string) (auth.UserProfile, error) {
	return auth.UserProfile{}, nil
}

func (s *passwordSafetyStore) CreateSession(_ context.Context, session auth.Session) error {
	s.sessions = append(s.sessions, session)
	return nil
}

func (s *passwordSafetyStore) Revoke(_ context.Context, _ string, _ time.Time) error { return nil }

// containsPassword returns true if the string contains a "password" key in any
// case variant, indicating a potential plaintext credential leak.
func containsPassword(s string) bool {
	return strings.Contains(strings.ToLower(s), `"password"`)
}

// safetyVerifier resolves tokens without invoking Firebase, returning a
// deterministic identity so tests are hermetic.
type safetyVerifier struct{}

func (v *safetyVerifier) Verify(_ context.Context, _ string) (*auth.DecodedToken, error) {
	return &auth.DecodedToken{UID: "safety-firebase-uid", Email: "safety@halaqaty.app"}, nil
}

// safetyMiddlewareStore satisfies middleware.SessionRepository.
type safetyMiddlewareStore struct{ userID string }

func (s *safetyMiddlewareStore) GetByID(_ context.Context, _ string) (auth.Session, error) {
	return auth.Session{}, nil
}

func (s *safetyMiddlewareStore) Touch(_ context.Context, _ string, _ time.Time) error { return nil }

func (s *safetyMiddlewareStore) GetLocalUserIDByFirebaseUID(_ context.Context, _ string) (string, error) {
	return s.userID, nil
}

func buildPasswordSafetyHandler(store *passwordSafetyStore) http.Handler {
	svc := auth.NewService(store, nil, 24*time.Hour)
	handler := auth.NewHandler(svc)
	sessionSvc := auth.NewSessionService(24 * time.Hour)
	mwStore := &safetyMiddlewareStore{userID: "safety-user"}
	mw := middleware.NewAuthMiddleware(&safetyVerifier{}, sessionSvc, mwStore)
	return mw.RequireVerifiedFirebase(http.HandlerFunc(handler.Register))
}

// TestPasswordStorageSafety_RegisterRequestBodyIgnoresPasswordField asserts
// that sending a "password" field in the register request body does not cause
// it to appear in the service store calls or the response.
func TestPasswordStorageSafety_RegisterRequestBodyIgnoresPasswordField(t *testing.T) {
	store := &passwordSafetyStore{}
	handler := buildPasswordSafetyHandler(store)

	// Deliberately include a "password" field – it should be rejected as an
	// unknown field (DisallowUnknownFields is active in DecodeJSONBody) or
	// silently ignored; it must never reach the service layer.
	body := `{"display_name":"Ali","password":"super-secret"}`
	req := httptest.NewRequest(http.MethodPost, "/auth/register", bytes.NewBufferString(body))
	req.Header.Set(httpconst.HeaderContentType, httpconst.ContentTypeApplicationJSON)
	req.Header.Set(httpconst.HeaderAuthorization, "Bearer test-token")

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	// The handler uses DisallowUnknownFields, so it should reject the request.
	// Either 400 (unknown field rejected) or 201/409 (password field stripped by
	// a permissive decoder) are acceptable — but the response body and stored
	// state must never contain the plaintext password value.
	respBody := rec.Body.String()
	if containsPassword(respBody) {
		t.Errorf("register response must not contain 'password' field, got: %s", respBody)
	}
	if strings.Contains(respBody, "super-secret") {
		t.Errorf("register response must not echo the password value, got: %s", respBody)
	}

	// No store call should have received the raw password.
	for _, call := range store.upsertCalls {
		if strings.Contains(call.email, "super-secret") || strings.Contains(call.uid, "super-secret") {
			t.Errorf("upsert call received password value: %+v", call)
		}
	}
	for _, p := range store.profileArgs {
		if strings.Contains(p.displayName, "super-secret") {
			t.Errorf("profile upsert received password value: %+v", p)
		}
	}
}

// TestPasswordStorageSafety_RegisterResponseNoPassword asserts a valid register
// response never contains any "password" key.
func TestPasswordStorageSafety_RegisterResponseNoPassword(t *testing.T) {
	store := &passwordSafetyStore{}
	handler := buildPasswordSafetyHandler(store)

	req := httptest.NewRequest(http.MethodPost, "/auth/register",
		bytes.NewBufferString(`{"display_name":"Safe User"}`))
	req.Header.Set(httpconst.HeaderContentType, httpconst.ContentTypeApplicationJSON)
	req.Header.Set(httpconst.HeaderAuthorization, "Bearer test-token")

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated && rec.Code != http.StatusConflict {
		t.Fatalf("unexpected status %d: %s", rec.Code, rec.Body.String())
	}
	if containsPassword(rec.Body.String()) {
		t.Errorf("register response must not contain 'password' key: %s", rec.Body.String())
	}
}

// TestPasswordStorageSafety_CreateSessionResponseNoPassword asserts the
// create-session response never contains a "password" key.
func TestPasswordStorageSafety_CreateSessionResponseNoPassword(t *testing.T) {
	store := &passwordSafetyStore{}
	sessionSvc := auth.NewSessionService(24 * time.Hour)
	svc := auth.NewService(store, nil, 24*time.Hour)
	handler := auth.NewHandler(svc)
	mwStore := &safetyMiddlewareStore{userID: "safety-user"}
	mw := middleware.NewAuthMiddleware(&safetyVerifier{}, sessionSvc, mwStore)

	h := mw.RequireBearer(http.HandlerFunc(handler.CreateSession))

	req := httptest.NewRequest(http.MethodPost, "/auth/sessions",
		bytes.NewBufferString(`{"device_name":"iPhone"}`))
	req.Header.Set(httpconst.HeaderContentType, httpconst.ContentTypeApplicationJSON)
	req.Header.Set(httpconst.HeaderAuthorization, "Bearer test-token")

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if containsPassword(rec.Body.String()) {
		t.Errorf("create session response must not contain 'password' key: %s", rec.Body.String())
	}
}

// TestPasswordStorageSafety_ErrorResponseNoPassword asserts that error
// responses (e.g. 401) do not accidentally include a password field.
func TestPasswordStorageSafety_ErrorResponseNoPassword(t *testing.T) {
	// A request without a valid bearer token should return a 401 error envelope.
	handler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		phttp.WriteError(w, httpconst.ErrorCodeUnauthorized, httpconst.ErrorMessageUnauthorized, http.StatusUnauthorized)
	})

	req := httptest.NewRequest(http.MethodPost, "/auth/register", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	var env phttp.ErrorEnvelope
	if err := json.Unmarshal(rec.Body.Bytes(), &env); err != nil {
		t.Fatalf("failed to decode error envelope: %v", err)
	}
	if containsPassword(rec.Body.String()) {
		t.Errorf("error response must not contain 'password' key: %s", rec.Body.String())
	}
}

// TestPasswordStorageSafety_StoredSessionNoPassword asserts that sessions
// persisted to the store do not contain any password-like field.
func TestPasswordStorageSafety_StoredSessionNoPassword(t *testing.T) {
	store := &passwordSafetyStore{}
	sessionSvc := auth.NewSessionService(24 * time.Hour)
	svc := auth.NewService(store, nil, 24*time.Hour)
	handler := auth.NewHandler(svc)
	mwStore := &safetyMiddlewareStore{userID: "safety-user"}
	mw := middleware.NewAuthMiddleware(&safetyVerifier{}, sessionSvc, mwStore)

	h := mw.RequireBearer(http.HandlerFunc(handler.CreateSession))

	req := httptest.NewRequest(http.MethodPost, "/auth/sessions",
		bytes.NewBufferString(`{"device_name":"test"}`))
	req.Header.Set(httpconst.HeaderContentType, httpconst.ContentTypeApplicationJSON)
	req.Header.Set(httpconst.HeaderAuthorization, "Bearer test-token")

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	for _, sess := range store.sessions {
		// Marshal the session struct to catch any stray password-like fields.
		marshaled, _ := json.Marshal(sess)
		if containsPassword(string(marshaled)) {
			t.Errorf("persisted session must not contain 'password' key: %s", string(marshaled))
		}
	}
}
