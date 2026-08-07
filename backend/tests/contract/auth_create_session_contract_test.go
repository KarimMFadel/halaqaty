//go:build contract

package contract

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/KarimMFadel/halaqaty/backend/internal/auth"
	"github.com/KarimMFadel/halaqaty/backend/internal/middleware"
	phttp "github.com/KarimMFadel/halaqaty/backend/internal/platform/http"
	"github.com/KarimMFadel/halaqaty/backend/internal/platform/httpconst"
)

// createSessionStubStore satisfies auth.Store for session-creation contract tests.
type createSessionStubStore struct{}

func (s *createSessionStubStore) UpsertUserByFirebaseUID(_ context.Context, _, _ string) (auth.User, bool, error) {
	return auth.User{}, false, nil
}
func (s *createSessionStubStore) UpsertProfileOnRegister(_ context.Context, _, _, _ string) error {
	return nil
}
func (s *createSessionStubStore) GetUserProfileByUserID(_ context.Context, _ string) (auth.UserProfile, error) {
	dn := "Ali"
	return auth.UserProfile{
		ID:                testLocalUserID,
		FirebaseUID:       testFirebaseUID,
		DisplayName:       &dn,
		PreferredLanguage: "ar",
		CreatedAt:         time.Now().UTC(),
	}, nil
}
func (s *createSessionStubStore) CreateSession(_ context.Context, _ auth.Session) error { return nil }
func (s *createSessionStubStore) Revoke(_ context.Context, _ string, _ time.Time) error { return nil }

func buildCreateSessionRoute() http.Handler {
	store := &createSessionStubStore{}
	svc := auth.NewService(store, nil, 30*24*time.Hour)
	h := auth.NewHandler(svc)
	verifier := &stubVerifier{token: "valid-token", decoded: testDecodedToken}
	repo := &stubSessionRepo{sessionID: testSessionID, userID: testLocalUserID}
	svcMW := auth.NewSessionService(30 * 24 * time.Hour)
	authMW := middleware.NewAuthMiddleware(verifier, svcMW, repo)
	return authMW.RequireBearer(http.HandlerFunc(h.CreateSession))
}

func TestAuthCreateSessionContract(t *testing.T) {
	h := buildCreateSessionRoute()

	cases := []struct {
		name     string
		bearer   string
		body     string
		wantCode int
		wantErr  string
	}{
		{
			name:     "missing bearer returns 401",
			bearer:   "",
			body:     `{}`,
			wantCode: http.StatusUnauthorized,
			wantErr:  httpconst.ErrorCodeUnauthorized,
		},
		{
			name:     "invalid bearer token returns 401",
			bearer:   "Bearer bad-token",
			body:     `{}`,
			wantCode: http.StatusUnauthorized,
			wantErr:  httpconst.ErrorCodeUnauthorized,
		},
		{
			name:     "valid bearer happy path returns 200 with session",
			bearer:   "Bearer valid-token",
			body:     `{}`,
			wantCode: http.StatusOK,
		},
		{
			name:     "valid bearer with device_name returns 200 with session",
			bearer:   "Bearer valid-token",
			body:     `{"device_name":"iPhone 15"}`,
			wantCode: http.StatusOK,
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/auth/sessions", bytes.NewBufferString(tc.body))
			req.Header.Set(httpconst.HeaderContentType, httpconst.ContentTypeApplicationJSON)
			if tc.bearer != "" {
				req.Header.Set(httpconst.HeaderAuthorization, tc.bearer)
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
				return
			}

			var resp auth.BackendSessionResponse
			if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
				t.Fatalf("decode session response: %v", err)
			}
			if resp.SessionID == "" {
				t.Fatal("expected non-empty session_id")
			}
			if resp.ExpiresAt.IsZero() {
				t.Fatal("expected non-zero expires_at")
			}
		})
	}
}
