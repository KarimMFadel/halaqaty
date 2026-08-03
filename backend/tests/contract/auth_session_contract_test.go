//go:build contract

package contract

import (
	"context"
	"encoding/json"
	"errors"
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

const (
	testFirebaseUID = "firebase-123"
	testLocalUserID = "11111111-1111-1111-1111-111111111111"
	testOtherUserID = "22222222-2222-2222-2222-222222222222"
	testSessionID   = "33333333-3333-3333-3333-333333333333"
)

var testDecodedToken = &auth.DecodedToken{
	UID:   testFirebaseUID,
	Email: "user@halaqaty.app",
}

func TestAuthSessionContract(t *testing.T) {
	verifier := &stubVerifier{token: "valid-token", decoded: testDecodedToken}
	repo := &stubSessionRepo{sessionID: testSessionID, userID: testLocalUserID}
	svc := auth.NewSessionService(30 * 24 * time.Hour)
	authMW := middleware.NewAuthMiddleware(verifier, svc, repo)

	okHandler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"ok":true}`))
	})
	protected := authMW.Require(okHandler)

	cases := []struct {
		name       string
		authHeader string
		sessionID  string
		repoSetup  func(*stubSessionRepo)
		wantStatus int
		wantCode   string
	}{
		{
			name:       "missing bearer token is rejected",
			authHeader: "",
			sessionID:  testSessionID,
			wantStatus: http.StatusUnauthorized,
			wantCode:   httpconst.ErrorCodeUnauthorized,
		},
		{
			name:       "invalid bearer token is rejected",
			authHeader: "Bearer invalid-token",
			sessionID:  testSessionID,
			wantStatus: http.StatusUnauthorized,
			wantCode:   httpconst.ErrorCodeUnauthorized,
		},
		{
			name:       "missing session id is rejected",
			authHeader: "Bearer valid-token",
			sessionID:  "",
			wantStatus: http.StatusUnauthorized,
			wantCode:   httpconst.ErrorCodeSessionMissing,
		},
		{
			name:       "unknown session id is rejected",
			authHeader: "Bearer valid-token",
			sessionID:  "unknown-session",
			wantStatus: http.StatusUnauthorized,
			wantCode:   httpconst.ErrorCodeSessionNotFound,
		},
		{
			name:       "revoked session is rejected",
			authHeader: "Bearer valid-token",
			sessionID:  testSessionID,
			repoSetup: func(r *stubSessionRepo) {
				revoked := time.Now().UTC().Add(-time.Hour)
				r.revokedAt = &revoked
			},
			wantStatus: http.StatusUnauthorized,
			wantCode:   httpconst.ErrorCodeSessionRevoked,
		},
		{
			name:       "expired session is rejected",
			authHeader: "Bearer valid-token",
			sessionID:  testSessionID,
			repoSetup: func(r *stubSessionRepo) {
				r.expiresAt = time.Now().UTC().Add(-time.Hour)
			},
			wantStatus: http.StatusUnauthorized,
			wantCode:   httpconst.ErrorCodeSessionExpired,
		},
		{
			name:       "session belonging to another user is rejected",
			authHeader: "Bearer valid-token",
			sessionID:  testSessionID,
			repoSetup: func(r *stubSessionRepo) {
				r.userID = testOtherUserID
			},
			wantStatus: http.StatusUnauthorized,
			wantCode:   httpconst.ErrorCodeSessionUserMismatch,
		},
		{
			name:       "valid bearer and session is accepted",
			authHeader: "Bearer valid-token",
			sessionID:  testSessionID,
			wantStatus: http.StatusOK,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// Reset repo state and apply per-case mutation.
			repo.sessionID = testSessionID
			repo.userID = testLocalUserID
			repo.revokedAt = nil
			repo.expiresAt = time.Now().UTC().Add(24 * time.Hour)
			repo.touched = false
			if tc.repoSetup != nil {
				tc.repoSetup(repo)
			}

			req := httptest.NewRequest(http.MethodGet, "/protected", nil)
			if tc.authHeader != "" {
				req.Header.Set(httpconst.HeaderAuthorization, tc.authHeader)
			}
			if tc.sessionID != "" {
				req.Header.Set(httpconst.HeaderSessionID, tc.sessionID)
			}

			rec := httptest.NewRecorder()
			protected.ServeHTTP(rec, req)

			if rec.Code != tc.wantStatus {
				t.Fatalf("status: got %d, want %d", rec.Code, tc.wantStatus)
			}

			if tc.wantStatus == http.StatusOK {
				return
			}

			var envelope phttp.ErrorEnvelope
			if err := json.Unmarshal(rec.Body.Bytes(), &envelope); err != nil {
				t.Fatalf("invalid error envelope: %v", err)
			}
			if envelope.Error.Code != tc.wantCode {
				t.Fatalf("error code: got %q, want %q", envelope.Error.Code, tc.wantCode)
			}
		})
	}

	// Valid request should also touch last activity.
	t.Run("valid request touches session activity", func(t *testing.T) {
		repo.sessionID = testSessionID
		repo.userID = testLocalUserID
		repo.revokedAt = nil
		repo.expiresAt = time.Now().UTC().Add(24 * time.Hour)
		repo.touched = false

		req := httptest.NewRequest(http.MethodGet, "/protected", nil)
		req.Header.Set(httpconst.HeaderAuthorization, "Bearer valid-token")
		req.Header.Set(httpconst.HeaderSessionID, testSessionID)
		rec := httptest.NewRecorder()
		protected.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d", rec.Code)
		}
		if !repo.touched {
			t.Fatal("expected session activity to be touched")
		}
	})
}

type stubVerifier struct {
	token   string
	decoded *auth.DecodedToken
	err     error
}

func (v *stubVerifier) Verify(_ context.Context, bearerToken string) (*auth.DecodedToken, error) {
	if bearerToken != v.token {
		return nil, errors.New("invalid token")
	}
	if v.err != nil {
		return nil, v.err
	}
	return v.decoded, nil
}

type stubSessionRepo struct {
	sessionID string
	userID    string
	revokedAt *time.Time
	expiresAt time.Time
	touched   bool
}

func (r *stubSessionRepo) GetByID(_ context.Context, sessionID string) (auth.Session, error) {
	if strings.TrimSpace(sessionID) == "" || sessionID != r.sessionID {
		return auth.Session{}, auth.ErrSessionNotFound
	}
	return auth.Session{
		ID:             r.sessionID,
		UserID:         r.userID,
		LastActivityAt: time.Now().UTC().Add(-time.Minute),
		ExpiresAt:      r.expiresAt,
		RevokedAt:      r.revokedAt,
	}, nil
}

func (r *stubSessionRepo) Touch(_ context.Context, _ string, _ time.Time) error {
	r.touched = true
	return nil
}

func (r *stubSessionRepo) GetLocalUserIDByFirebaseUID(_ context.Context, firebaseUID string) (string, error) {
	if firebaseUID == testFirebaseUID {
		return testLocalUserID, nil
	}
	return "", auth.ErrUserNotFound
}
