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
	"github.com/KarimMFadel/halaqaty/backend/internal/queue"
	"github.com/KarimMFadel/halaqaty/backend/internal/sessions"
)

func TestRouter_F005ProtectedRoutesRequireAuth(t *testing.T) {
	router := NewRouter(MiddlewareSet{Auth: middlewareForRouteTest(), SessionHandler: sessions.NewHandler(nil)})
	rec := httptest.NewRecorder()
	router.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/v1/sessions/00000000-0000-0000-0000-000000000001/start", nil))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("F-005 route status = %d, want 401 from auth middleware", rec.Code)
	}
}

func TestRouter_QueueRoutesRequireAuth(t *testing.T) {
	router := NewRouter(MiddlewareSet{
		Auth:         middlewareForRouteTest(),
		QueueHandler: queue.NewHandler(nil, nil, nil, nil),
	})
	rec := httptest.NewRecorder()
	router.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/sessions/00000000-0000-0000-0000-000000000001/queue", nil))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("queue route status = %d, want 401 from production auth middleware", rec.Code)
	}
}

func TestRouter_QueueRoutesUseProductionRateLimits(t *testing.T) {
	path := "/api/v1/sessions/00000000-0000-0000-0000-000000000001/queue"

	t.Run("per IP applies before authentication", func(t *testing.T) {
		router := NewRouter(MiddlewareSet{
			Auth:         middlewareForRouteTest(),
			QueueHandler: queue.NewHandler(nil, nil, nil, nil),
			RateLimit:    middleware.NewRateLimitMiddleware(1, 0),
		})
		for attempt, want := range []int{http.StatusUnauthorized, http.StatusTooManyRequests} {
			req := httptest.NewRequest(http.MethodGet, path, nil)
			req.RemoteAddr = "198.51.100.17:4321"
			rec := httptest.NewRecorder()
			router.Handler().ServeHTTP(rec, req)
			if rec.Code != want {
				t.Fatalf("attempt %d: got %d, want %d", attempt+1, rec.Code, want)
			}
		}
	})

	t.Run("per user applies after authentication", func(t *testing.T) {
		router := NewRouter(MiddlewareSet{
			Auth: middleware.NewAuthMiddleware(
				authenticatedRouteVerifier{}, auth.NewSessionService(time.Hour), authenticatedRouteSessionRepo{},
			),
			QueueHandler: queue.NewHandler(nil, nil, nil, nil),
			RateLimit:    middleware.NewRateLimitMiddleware(0, 1),
		})
		for attempt, want := range []int{http.StatusInternalServerError, http.StatusTooManyRequests} {
			req := httptest.NewRequest(http.MethodGet, path, nil)
			req.Header.Set("Authorization", "Bearer valid-token")
			req.Header.Set("X-Halaqaty-Session-ID", "session-1")
			rec := httptest.NewRecorder()
			router.Handler().ServeHTTP(rec, req)
			if rec.Code != want {
				t.Fatalf("attempt %d: got %d, want %d", attempt+1, rec.Code, want)
			}
		}
	})
}

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

func TestRouter_InviteRefreshRejectsSupervisor(t *testing.T) {
	circleID := "00000000-0000-0000-0000-000000000001"
	authMiddleware := middleware.NewAuthMiddleware(
		authenticatedRouteVerifier{},
		auth.NewSessionService(time.Hour),
		authenticatedRouteSessionRepo{},
	)
	router := NewRouter(MiddlewareSet{
		Auth: authMiddleware,
		Role: middleware.NewRoleMiddleware(supervisorMembershipRepo{}),
	})
	request := httptest.NewRequest(
		http.MethodPost,
		"/api/v1/circles/"+circleID+"/invite-code/refresh",
		nil,
	)
	request.Header.Set("Authorization", "Bearer valid-token")
	request.Header.Set("X-Halaqaty-Session-ID", "session-1")
	recorder := httptest.NewRecorder()

	router.Handler().ServeHTTP(recorder, request)

	if recorder.Code != http.StatusForbidden {
		t.Fatalf("status: got %d, want %d", recorder.Code, http.StatusForbidden)
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

type supervisorMembershipRepo struct{}

type authenticatedRouteVerifier struct{}

type authenticatedRouteSessionRepo struct{}

func (authenticatedRouteVerifier) Verify(context.Context, string) (*auth.DecodedToken, error) {
	return &auth.DecodedToken{UID: "firebase-supervisor"}, nil
}

func (authenticatedRouteSessionRepo) GetByID(context.Context, string) (auth.Session, error) {
	return auth.Session{
		ID:             "session-1",
		UserID:         "00000000-0000-0000-0000-000000000002",
		LastActivityAt: time.Now(),
		ExpiresAt:      time.Now().Add(time.Hour),
	}, nil
}

func (authenticatedRouteSessionRepo) Touch(context.Context, string, time.Time) error {
	return nil
}

func (authenticatedRouteSessionRepo) GetLocalUserIDByFirebaseUID(context.Context, string) (string, error) {
	return "00000000-0000-0000-0000-000000000002", nil
}

func (supervisorMembershipRepo) RoleForUserInCircle(context.Context, string, string) (string, error) {
	return "supervisor", nil
}

func (routeTestSessionRepo) GetByID(context.Context, string) (auth.Session, error) {
	return auth.Session{}, auth.ErrSessionNotFound
}

func (routeTestSessionRepo) Touch(context.Context, string, time.Time) error {
	return nil
}

func (routeTestSessionRepo) GetLocalUserIDByFirebaseUID(context.Context, string) (string, error) {
	return "", auth.ErrUserNotFound
}
