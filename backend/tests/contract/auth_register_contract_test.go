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

const bearerValid = "Bearer valid-register-token"

// registerStubStore satisfies auth.Store for registration contract tests.
type registerStubStore struct {
	existingUID    string
	duplicateEmail bool
}

func (s *registerStubStore) UpsertUserByFirebaseUID(_ context.Context, firebaseUID, _ string) (auth.User, bool, error) {
	if s.duplicateEmail {
		return auth.User{}, false, auth.ErrDuplicateEmail
	}
	inserted := s.existingUID != firebaseUID
	return auth.User{ID: testLocalUserID, FirebaseUID: firebaseUID, Email: "user@halaqaty.app"}, inserted, nil
}

func (s *registerStubStore) UpsertProfileOnRegister(_ context.Context, _, _, _ string) error {
	return nil
}

func (s *registerStubStore) GetUserProfileByUserID(_ context.Context, _ string) (auth.UserProfile, error) {
	dn := "Ali"
	return auth.UserProfile{
		ID:                testLocalUserID,
		FirebaseUID:       testFirebaseUID,
		DisplayName:       &dn,
		PreferredLanguage: "ar",
		CreatedAt:         time.Now().UTC(),
	}, nil
}

func (s *registerStubStore) CreateSession(_ context.Context, _ auth.Session) error { return nil }
func (s *registerStubStore) Revoke(_ context.Context, _ string, _ time.Time) error { return nil }

// alwaysOKVerifier accepts any non-empty bearer value and returns testDecodedToken.
type alwaysOKVerifier struct{}

func (v *alwaysOKVerifier) Verify(_ context.Context, _ string) (*auth.DecodedToken, error) {
	return testDecodedToken, nil
}

func buildRegisterRoute(store auth.Store) http.Handler {
	svc := auth.NewService(store, nil, 30*24*time.Hour)
	h := auth.NewHandler(svc)
	verifier := &alwaysOKVerifier{}
	repo := &stubSessionRepo{sessionID: testSessionID, userID: testLocalUserID}
	svcMW := auth.NewSessionService(30 * 24 * time.Hour)
	authMW := middleware.NewAuthMiddleware(verifier, svcMW, repo)
	return authMW.RequireVerifiedFirebase(http.HandlerFunc(h.Register))
}

func TestAuthRegisterContract(t *testing.T) {
	store := &registerStubStore{}
	h := buildRegisterRoute(store)

	cases := []struct {
		name      string
		bearer    string
		body      string
		setup     func()
		teardown  func()
		wantCode  int
		wantErr   string
		wantField string
	}{
		{
			name:     "missing bearer returns 401",
			bearer:   "",
			body:     `{"display_name":"Ali"}`,
			wantCode: http.StatusUnauthorized,
			wantErr:  httpconst.ErrorCodeUnauthorized,
		},
		{
			name:      "missing display_name returns 400",
			bearer:    bearerValid,
			body:      `{"preferred_language":"ar"}`,
			wantCode:  http.StatusBadRequest,
			wantErr:   httpconst.ErrorCodeValidationFailed,
			wantField: httpconst.FieldDisplayName,
		},
		{
			name:      "display_name single rune returns 400",
			bearer:    bearerValid,
			body:      `{"display_name":"A"}`,
			wantCode:  http.StatusBadRequest,
			wantErr:   httpconst.ErrorCodeValidationFailed,
			wantField: httpconst.FieldDisplayName,
		},
		{
			name:     "unknown JSON field returns 400",
			bearer:   bearerValid,
			body:     `{"display_name":"Ali","password":"secret"}`,
			wantCode: http.StatusBadRequest,
			wantErr:  httpconst.ErrorCodeValidationFailed,
		},
		{
			name:     "trailing JSON payload returns 400",
			bearer:   bearerValid,
			body:     `{"display_name":"Ali"}{"extra":"field"}`,
			wantCode: http.StatusBadRequest,
			wantErr:  httpconst.ErrorCodeValidationFailed,
		},
		{
			name:     "first registration returns 201 with session body",
			bearer:   bearerValid,
			body:     `{"display_name":"Ali"}`,
			wantCode: http.StatusCreated,
		},
		{
			name:     "idempotent replay returns 409 with session body",
			bearer:   bearerValid,
			body:     `{"display_name":"Ali"}`,
			setup:    func() { store.existingUID = testFirebaseUID },
			teardown: func() { store.existingUID = "" },
			wantCode: http.StatusConflict,
		},
		{
			name:     "duplicate email returns 409 conflict envelope",
			bearer:   bearerValid,
			body:     `{"display_name":"Ali"}`,
			setup:    func() { store.duplicateEmail = true },
			teardown: func() { store.duplicateEmail = false },
			wantCode: http.StatusConflict,
			wantErr:  httpconst.ErrorCodeConflict,
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			if tc.setup != nil {
				tc.setup()
			}
			t.Cleanup(func() {
				if tc.teardown != nil {
					tc.teardown()
				}
			})

			req := httptest.NewRequest(http.MethodPost, "/auth/register", bytes.NewBufferString(tc.body))
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
				if tc.wantField != "" {
					if _, ok := env.Error.Fields[tc.wantField]; !ok {
						t.Fatalf("expected field %q in Fields=%v", tc.wantField, env.Error.Fields)
					}
				}
				return
			}

			// Happy-path: body must be a valid BackendSessionResponse.
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
