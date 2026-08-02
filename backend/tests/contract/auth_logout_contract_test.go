//go:build contract

package contract

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"halaqaty/backend/internal/auth"
	"halaqaty/backend/internal/middleware"
	phttp "halaqaty/backend/internal/platform/http"
	"halaqaty/backend/internal/platform/httpconst"
)

// logoutStubStore satisfies auth.Store for logout contract tests.
type logoutStubStore struct {
	revokeCalled bool
}

func (s *logoutStubStore) UpsertUserByFirebaseUID(_ context.Context, _, _ string) (auth.User, bool, error) {
	return auth.User{}, false, nil
}
func (s *logoutStubStore) UpsertProfileOnRegister(_ context.Context, _, _, _ string) error {
	return nil
}
func (s *logoutStubStore) GetUserProfileByUserID(_ context.Context, _ string) (auth.UserProfile, error) {
	return auth.UserProfile{}, nil
}
func (s *logoutStubStore) CreateSession(_ context.Context, _ auth.Session) error { return nil }
func (s *logoutStubStore) Revoke(_ context.Context, _ string, _ time.Time) error {
	s.revokeCalled = true
	return nil
}

func buildLogoutRoute(store *logoutStubStore, repo *stubSessionRepo) http.Handler {
	svc := auth.NewService(store, nil, 30*24*time.Hour)
	h := auth.NewHandler(svc)
	verifier := &stubVerifier{token: "valid-token", decoded: testDecodedToken}
	svcMW := auth.NewSessionService(30 * 24 * time.Hour)
	authMW := middleware.NewAuthMiddleware(verifier, svcMW, repo)
	return authMW.Require(http.HandlerFunc(h.Logout))
}

func TestAuthLogoutContract(t *testing.T) {
	cases := []struct {
		name     string
		bearer   string
		sessID   string
		wantCode int
		wantErr  string
	}{
		{
			name:     "missing bearer returns 401",
			bearer:   "",
			sessID:   testSessionID,
			wantCode: http.StatusUnauthorized,
			wantErr:  httpconst.ErrorCodeUnauthorized,
		},
		{
			name:     "missing session-id header returns 401",
			bearer:   "Bearer valid-token",
			sessID:   "",
			wantCode: http.StatusUnauthorized,
			wantErr:  httpconst.ErrorCodeSessionMissing,
		},
		{
			name:     "valid credentials returns 204",
			bearer:   "Bearer valid-token",
			sessID:   testSessionID,
			wantCode: http.StatusNoContent,
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			store := &logoutStubStore{}
			repo := &stubSessionRepo{
				sessionID: testSessionID,
				userID:    testLocalUserID,
				expiresAt: time.Now().UTC().Add(24 * time.Hour),
			}
			h := buildLogoutRoute(store, repo)

			req := httptest.NewRequest(http.MethodPost, "/auth/logout", nil)
			if tc.bearer != "" {
				req.Header.Set(httpconst.HeaderAuthorization, tc.bearer)
			}
			if tc.sessID != "" {
				req.Header.Set(httpconst.HeaderSessionID, tc.sessID)
			}

			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)

			if rec.Code != tc.wantCode {
				t.Fatalf("status: got %d, want %d — body: %s", rec.Code, tc.wantCode, rec.Body.String())
			}

			if tc.wantErr != "" {
				var env phttp.ErrorEnvelope
				if err := json.NewDecoder(rec.Body).Decode(&env); err != nil {
					t.Fatalf("decode error envelope: %v", err)
				}
				if env.Error.Code != tc.wantErr {
					t.Fatalf("error code: got %q, want %q", env.Error.Code, tc.wantErr)
				}
			}
		})
	}
}
