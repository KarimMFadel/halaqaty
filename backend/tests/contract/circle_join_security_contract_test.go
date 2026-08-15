//go:build contract

package contract

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/KarimMFadel/halaqaty/backend/internal/auth"
	"github.com/KarimMFadel/halaqaty/backend/internal/middleware"
	"github.com/KarimMFadel/halaqaty/backend/internal/platform/httpconst"
	"github.com/KarimMFadel/halaqaty/backend/internal/rbac"
)

func buildSecuredInviteJoinRoute(store *circleStoreStub, verifier auth.TokenVerifier, rateLimit *middleware.RateLimitMiddleware) http.Handler {
	handler := rbac.NewHandler(rbac.NewService(store, nil))
	repo := &stubSessionRepo{sessionID: testSessionID, userID: testLocalUserID}
	authMW := middleware.NewAuthMiddleware(verifier, auth.NewSessionService(30*24*time.Hour), repo)
	var next http.Handler = http.HandlerFunc(handler.JoinCircle)
	if rateLimit != nil {
		next = rateLimit.Limit(next)
	}
	return authMW.Require(next)
}

func buildSecuredDiscoveryRoute(store *discoveryStoreStub, verifier auth.TokenVerifier) http.Handler {
	handler := rbac.NewHandler(rbac.NewService(store, nil))
	repo := &stubSessionRepo{sessionID: testSessionID, userID: testLocalUserID}
	authMW := middleware.NewAuthMiddleware(verifier, auth.NewSessionService(30*24*time.Hour), repo)
	return authMW.Require(http.HandlerFunc(handler.DiscoverPublicCircles))
}

func TestCircleJoinSecurity_RequiresCredentials(t *testing.T) {
	for _, tc := range []struct {
		name  string
		route http.Handler
		path  string
		body  string
	}{
		{"discovery", buildSecuredDiscoveryRoute(&discoveryStoreStub{circleStoreStub: newCircleStoreStub()}, &alwaysOKVerifier{}), "/circles/discover", ""},
		{"invite join", buildSecuredInviteJoinRoute(newCircleStoreStub(), &alwaysOKVerifier{}, nil), "/circles/join", `{"invite_code":"HLQ-7X2K"}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, tc.path, bytes.NewBufferString(tc.body))
			if tc.body != "" {
				req.Method = http.MethodPost
				req.Header.Set(httpconst.HeaderContentType, httpconst.ContentTypeApplicationJSON)
			}
			rec := httptest.NewRecorder()
			tc.route.ServeHTTP(rec, req)
			if rec.Code != http.StatusUnauthorized {
				t.Fatalf("status: got %d want %d body=%s", rec.Code, http.StatusUnauthorized, rec.Body.String())
			}
		})
	}
}

func TestCircleJoinSecurity_DiscoveryRejectsEmptyPrincipal(t *testing.T) {
	handler := rbac.NewHandler(rbac.NewService(&discoveryStoreStub{circleStoreStub: newCircleStoreStub()}, nil))
	req := httptest.NewRequest(http.MethodGet, "/circles/discover", nil)
	req = req.WithContext(auth.WithPrincipal(req.Context(), auth.AuthPrincipal{}))
	rec := httptest.NewRecorder()

	handler.DiscoverPublicCircles(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status: got %d want %d body=%s", rec.Code, http.StatusUnauthorized, rec.Body.String())
	}
}

func TestCircleJoinSecurity_RejectsInvalidFirebaseAndSession(t *testing.T) {
	for _, tc := range []struct {
		name       string
		verifier   auth.TokenVerifier
		authHeader string
		sessionID  string
	}{
		{"invalid Firebase token", &rejectingCircleVerifier{}, "Bearer invalid", testSessionID},
		{"invalid session", &alwaysOKVerifier{}, bearerValid, "missing-session"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/circles/join", bytes.NewBufferString(`{"invite_code":"HLQ-7X2K"}`))
			req.Header.Set(httpconst.HeaderAuthorization, tc.authHeader)
			req.Header.Set(httpconst.HeaderSessionID, tc.sessionID)
			req.Header.Set(httpconst.HeaderContentType, httpconst.ContentTypeApplicationJSON)
			rec := httptest.NewRecorder()
			buildSecuredInviteJoinRoute(newCircleStoreStub(), tc.verifier, nil).ServeHTTP(rec, req)
			if rec.Code != http.StatusUnauthorized {
				t.Fatalf("status: got %d want %d body=%s", rec.Code, http.StatusUnauthorized, rec.Body.String())
			}
		})
	}
}

func TestCircleJoinSecurity_RateLimitsRepeatedJoinAndRejectsInvalidInvite(t *testing.T) {
	store := newCircleStoreStub()
	store.circles[contractCircleID] = rbac.Circle{ID: contractCircleID, Name: "Public Circle", InviteCode: "HLQ-7X2K", CreatedAt: time.Now().UTC()}
	route := buildSecuredInviteJoinRoute(store, &alwaysOKVerifier{}, middleware.NewRateLimitMiddleware(100, 1))
	for requestNumber, wantStatus := range []int{http.StatusOK, http.StatusTooManyRequests} {
		req := httptest.NewRequest(http.MethodPost, "/circles/join", bytes.NewBufferString(`{"invite_code":"HLQ-7X2K"}`))
		req.Header.Set(httpconst.HeaderAuthorization, bearerValid)
		req.Header.Set(httpconst.HeaderSessionID, testSessionID)
		req.Header.Set(httpconst.HeaderContentType, httpconst.ContentTypeApplicationJSON)
		rec := httptest.NewRecorder()
		route.ServeHTTP(rec, req)
		if rec.Code != wantStatus {
			t.Fatalf("request %d: got %d want %d body=%s", requestNumber+1, rec.Code, wantStatus, rec.Body.String())
		}
	}

	req := httptest.NewRequest(http.MethodPost, "/circles/join", bytes.NewBufferString(`{"invite_code":"invalid"}`))
	req.Header.Set(httpconst.HeaderAuthorization, bearerValid)
	req.Header.Set(httpconst.HeaderSessionID, testSessionID)
	req.Header.Set(httpconst.HeaderContentType, httpconst.ContentTypeApplicationJSON)
	rec := httptest.NewRecorder()
	buildSecuredInviteJoinRoute(newCircleStoreStub(), &alwaysOKVerifier{}, nil).ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("invalid invite status: got %d want %d body=%s", rec.Code, http.StatusBadRequest, rec.Body.String())
	}
}

func TestCircleJoinSecurity_PublicResultsAreRedacted(t *testing.T) {
	store := &discoveryStoreStub{
		circleStoreStub: newCircleStoreStub(),
		publicCircles: []rbac.PublicCircleSummary{{
			ID: "11111111-1111-1111-1111-111111111111", Name: "Public Circle", MaxCapacity: 50,
			GenderRestriction: "unspecified", Language: "ar",
		}},
	}
	req := httptest.NewRequest(http.MethodGet, "/circles/discover", nil)
	req.Header.Set(httpconst.HeaderAuthorization, bearerValid)
	req.Header.Set(httpconst.HeaderSessionID, testSessionID)
	rec := httptest.NewRecorder()
	buildSecuredDiscoveryRoute(store, &alwaysOKVerifier{}).ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status: got %d want %d body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}
	for _, forbidden := range []string{"invite_code", "invite_link", "user_id", "role", "is_private", "is_archived", "member_count", "my_role"} {
		if strings.Contains(strings.ToLower(rec.Body.String()), forbidden) {
			t.Fatalf("public discovery leaked %q: %s", forbidden, rec.Body.String())
		}
	}
}
