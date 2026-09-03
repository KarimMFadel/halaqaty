package middleware

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
	phttp "github.com/KarimMFadel/halaqaty/backend/internal/platform/http"
	"github.com/KarimMFadel/halaqaty/backend/internal/platform/httpconst"
	"github.com/KarimMFadel/halaqaty/backend/internal/platform/metrics"
)

const (
	testFirebaseUID = "firebase-123"
	testLocalUserID = "11111111-1111-1111-1111-111111111111"
	testEmail       = "user@halaqaty.app"
	testValidToken  = "valid-token"
)

var testDecodedToken = &auth.DecodedToken{
	UID:   testFirebaseUID,
	Email: testEmail,
}

func TestAuthMiddleware_RequireVerifiedFirebase(t *testing.T) {
	verifier := &stubVerifier{token: testValidToken, decoded: testDecodedToken}
	repo := &stubSessionRepo{userID: testLocalUserID, failLocalUserLookup: true}
	svc := auth.NewSessionService(30 * 24 * time.Hour)
	mw := NewAuthMiddleware(verifier, svc, repo)

	okHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		principal, ok := CurrentPrincipal(r.Context())
		if !ok {
			t.Error("expected principal in context")
		}
		if principal.UserID != "" {
			t.Errorf("expected empty local user id for registration, got %q", principal.UserID)
		}
		if principal.FirebaseUID != testFirebaseUID {
			t.Errorf("firebase uid: got %q, want %q", principal.FirebaseUID, testFirebaseUID)
		}
		if principal.Email != testEmail {
			t.Errorf("email: got %q, want %q", principal.Email, testEmail)
		}
		w.WriteHeader(http.StatusOK)
	})

	protected := mw.RequireVerifiedFirebase(okHandler)

	cases := []struct {
		name       string
		authHeader string
		wantStatus int
		wantCode   string
	}{
		{
			name:       "verified new firebase uid is accepted without local user",
			authHeader: "Bearer " + testValidToken,
			wantStatus: http.StatusOK,
		},
		{
			name:       "missing bearer token is rejected",
			authHeader: "",
			wantStatus: http.StatusUnauthorized,
			wantCode:   httpconst.ErrorCodeUnauthorized,
		},
		{
			name:       "invalid bearer token is rejected",
			authHeader: "Bearer invalid-token",
			wantStatus: http.StatusUnauthorized,
			wantCode:   httpconst.ErrorCodeUnauthorized,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			repo.localUserLookups = 0
			req := httptest.NewRequest(http.MethodPost, "/auth/register", nil)
			if tc.authHeader != "" {
				req.Header.Set(httpconst.HeaderAuthorization, tc.authHeader)
			}

			rec := httptest.NewRecorder()
			protected.ServeHTTP(rec, req)

			if rec.Code != tc.wantStatus {
				t.Fatalf("status: got %d, want %d", rec.Code, tc.wantStatus)
			}

			if tc.wantStatus == http.StatusOK {
				if repo.localUserLookups != 0 {
					t.Fatalf("local user lookup called %d time(s), expected 0", repo.localUserLookups)
				}
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
}

func TestAuthMiddleware_RequireBearer_ResolvesExistingUser(t *testing.T) {
	verifier := &stubVerifier{token: testValidToken, decoded: testDecodedToken}
	repo := &stubSessionRepo{userID: testLocalUserID}
	svc := auth.NewSessionService(30 * 24 * time.Hour)
	mw := NewAuthMiddleware(verifier, svc, repo)

	okHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		principal, ok := CurrentPrincipal(r.Context())
		if !ok {
			t.Error("expected principal in context")
		}
		if principal.UserID != testLocalUserID {
			t.Errorf("local user id: got %q, want %q", principal.UserID, testLocalUserID)
		}
		if principal.FirebaseUID != testFirebaseUID {
			t.Errorf("firebase uid: got %q, want %q", principal.FirebaseUID, testFirebaseUID)
		}
		w.WriteHeader(http.StatusOK)
	})

	protected := mw.RequireBearer(okHandler)

	t.Run("existing local user is resolved", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/auth/sessions", nil)
		req.Header.Set(httpconst.HeaderAuthorization, "Bearer "+testValidToken)

		rec := httptest.NewRecorder()
		protected.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("status: got %d, want %d", rec.Code, http.StatusOK)
		}
	})

	t.Run("unknown firebase uid is rejected", func(t *testing.T) {
		repo.userID = testLocalUserID
		repo.rejectFirebaseUID = true
		defer func() { repo.rejectFirebaseUID = false }()

		req := httptest.NewRequest(http.MethodPost, "/auth/sessions", nil)
		req.Header.Set(httpconst.HeaderAuthorization, "Bearer "+testValidToken)

		rec := httptest.NewRecorder()
		protected.ServeHTTP(rec, req)

		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("status: got %d, want %d", rec.Code, http.StatusUnauthorized)
		}
	})
}

func TestAuthMiddleware_SetMetrics_RecordsRejectionsAndRequests(t *testing.T) {
	verifier := &stubVerifier{token: testValidToken, decoded: testDecodedToken}
	repo := &stubSessionRepo{userID: testLocalUserID}
	mw := NewAuthMiddleware(verifier, auth.NewSessionService(30*24*time.Hour), repo)

	authMetrics := new(metrics.AuthMetrics)
	mw.SetMetrics(authMetrics)

	protected := mw.RequireVerifiedFirebase(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	rejected := httptest.NewRequest(http.MethodPost, "/auth/register", nil) // no bearer token
	rec := httptest.NewRecorder()
	protected.ServeHTTP(rec, rejected)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated status: got %d, want %d", rec.Code, http.StatusUnauthorized)
	}
	if summary := authMetrics.Summary(); summary.RejectionsTotal != 1 || summary.RequestsTotal != 0 {
		t.Fatalf("after rejection: summary=%+v, want rejections=1 requests=0", summary)
	}

	accepted := httptest.NewRequest(http.MethodPost, "/auth/register", nil)
	accepted.Header.Set(httpconst.HeaderAuthorization, "Bearer "+testValidToken)
	rec = httptest.NewRecorder()
	protected.ServeHTTP(rec, accepted)
	if rec.Code != http.StatusOK {
		t.Fatalf("authenticated status: got %d, want %d", rec.Code, http.StatusOK)
	}
	if summary := authMetrics.Summary(); summary.RejectionsTotal != 1 || summary.RequestsTotal != 1 {
		t.Fatalf("after acceptance: summary=%+v, want rejections=1 requests=1", summary)
	}
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
	sessionID           string
	userID              string
	revokedAt           *time.Time
	expiresAt           time.Time
	touched             bool
	rejectFirebaseUID   bool
	failLocalUserLookup bool
	localUserLookups    int
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
	r.localUserLookups++
	if r.failLocalUserLookup {
		return "", errors.New("local user lookup should not be called")
	}
	if r.rejectFirebaseUID {
		return "", auth.ErrUserNotFound
	}
	if firebaseUID == testFirebaseUID {
		return r.userID, nil
	}
	return "", auth.ErrUserNotFound
}
