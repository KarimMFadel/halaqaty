package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/KarimMFadel/halaqaty/backend/internal/auth"
	"github.com/KarimMFadel/halaqaty/backend/internal/middleware"
	"github.com/KarimMFadel/halaqaty/backend/internal/platform/metrics"
)

func TestRouter_RegistersVersionedAuthRoutes(t *testing.T) {
	authMiddleware := middlewareForRouteTest()
	router := NewRouter(MiddlewareSet{Auth: authMiddleware})

	rec := httptest.NewRecorder()
	router.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/v1/auth/sessions", nil))

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status: got %d, want %d", rec.Code, http.StatusUnauthorized)
	}
}

func TestRouter_CircleReadRoutesRequireAuth(t *testing.T) {
	circleID := "00000000-0000-0000-0000-000000000001"
	router := NewRouter(MiddlewareSet{Auth: middlewareForRouteTest()})

	for _, path := range []string{
		"/api/v1/circles/" + circleID,
		"/api/v1/circles/" + circleID + "/members",
	} {
		rec := httptest.NewRecorder()
		router.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("GET %s: got %d, want %d (unauthenticated should be rejected)", path, rec.Code, http.StatusUnauthorized)
		}
	}
}

func TestRouter_MetricsRequiresToken(t *testing.T) {
	metricStore := new(metrics.AuthMetrics)
	metricStore.RecordRequest(time.Millisecond)
	router := NewRouter(MiddlewareSet{
		Metrics:      metricStore,
		MetricsToken: "metrics-secret",
	})

	unauthorized := httptest.NewRecorder()
	router.Handler().ServeHTTP(unauthorized, httptest.NewRequest(http.MethodGet, "/metrics", nil))
	if unauthorized.Code != http.StatusUnauthorized {
		t.Fatalf("unauthorized status: got %d, want %d", unauthorized.Code, http.StatusUnauthorized)
	}

	authorizedRequest := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	authorizedRequest.Header.Set("Authorization", "Bearer metrics-secret")
	authorized := httptest.NewRecorder()
	router.Handler().ServeHTTP(authorized, authorizedRequest)
	if authorized.Code != http.StatusOK {
		t.Fatalf("authorized status: got %d, want %d", authorized.Code, http.StatusOK)
	}
	var summary metrics.MetricsSummary
	if err := json.NewDecoder(authorized.Body).Decode(&summary); err != nil {
		t.Fatalf("decode metrics response: %v", err)
	}
	if summary.RequestsTotal != 1 {
		t.Fatalf("request count: got %d, want 1", summary.RequestsTotal)
	}
}

func middlewareForRouteTest() *middleware.AuthMiddleware {
	return middleware.NewAuthMiddleware(routeTestVerifier{}, auth.NewSessionService(time.Hour), routeTestSessionRepo{})
}

type routeTestVerifier struct{}

func (routeTestVerifier) Verify(context.Context, string) (*auth.DecodedToken, error) {
	return nil, nil
}

type routeTestSessionRepo struct{}

func (routeTestSessionRepo) GetByID(context.Context, string) (auth.Session, error) {
	return auth.Session{}, auth.ErrSessionNotFound
}

func (routeTestSessionRepo) Touch(context.Context, string, time.Time) error {
	return nil
}

func (routeTestSessionRepo) GetLocalUserIDByFirebaseUID(context.Context, string) (string, error) {
	return "", auth.ErrUserNotFound
}
