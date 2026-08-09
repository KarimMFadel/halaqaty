//go:build contract

package contract

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/KarimMFadel/halaqaty/backend/internal/auth"
	"github.com/KarimMFadel/halaqaty/backend/internal/middleware"
	"github.com/KarimMFadel/halaqaty/backend/internal/platform/httpconst"
	"github.com/KarimMFadel/halaqaty/backend/internal/rbac"
)

type rejectingCircleVerifier struct{}

func (*rejectingCircleVerifier) Verify(context.Context, string) (*auth.DecodedToken, error) {
	return nil, errors.New("invalid token")
}

func buildSecuredCreateCircleRoute(store *circleStoreStub, verifier auth.TokenVerifier, rateLimit *middleware.RateLimitMiddleware) http.Handler {
	handler := rbac.NewHandler(rbac.NewService(store, nil))
	repo := &stubSessionRepo{sessionID: testSessionID, userID: testLocalUserID}
	authMW := middleware.NewAuthMiddleware(verifier, auth.NewSessionService(30*24*time.Hour), repo)
	var next http.Handler = http.HandlerFunc(handler.CreateCircle)
	if rateLimit != nil {
		next = rateLimit.Limit(next)
	}
	return authMW.Require(next)
}

func TestCircleCreateSecurity_InvalidFirebaseAndSessionCredentials(t *testing.T) {
	cases := []struct {
		name       string
		verifier   auth.TokenVerifier
		authHeader string
		sessionID  string
	}{
		{"invalid firebase token", &rejectingCircleVerifier{}, "Bearer invalid", testSessionID},
		{"invalid session", &alwaysOKVerifier{}, bearerValid, "missing-session"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/circles", bytes.NewBufferString(`{"name":"Circle"}`))
			req.Header.Set(httpconst.HeaderAuthorization, tc.authHeader)
			req.Header.Set(httpconst.HeaderSessionID, tc.sessionID)
			req.Header.Set(httpconst.HeaderContentType, httpconst.ContentTypeApplicationJSON)
			rec := httptest.NewRecorder()
			buildSecuredCreateCircleRoute(newCircleStoreStub(), tc.verifier, nil).ServeHTTP(rec, req)
			if rec.Code != http.StatusUnauthorized {
				t.Fatalf("status: got %d body=%s", rec.Code, rec.Body.String())
			}
		})
	}
}

func TestCircleCreateSecurity_RateLimitsRepeatedCreation(t *testing.T) {
	store := newCircleStoreStub()
	route := buildSecuredCreateCircleRoute(store, &alwaysOKVerifier{}, middleware.NewRateLimitMiddleware(100, 1))
	for i, want := range []int{http.StatusCreated, http.StatusTooManyRequests} {
		req := httptest.NewRequest(http.MethodPost, "/circles", bytes.NewBufferString(`{"name":"Circle"}`))
		req.Header.Set(httpconst.HeaderAuthorization, bearerValid)
		req.Header.Set(httpconst.HeaderSessionID, testSessionID)
		req.Header.Set(httpconst.HeaderContentType, httpconst.ContentTypeApplicationJSON)
		rec := httptest.NewRecorder()
		route.ServeHTTP(rec, req)
		if rec.Code != want {
			t.Fatalf("request %d: got %d want %d", i+1, rec.Code, want)
		}
	}
}
