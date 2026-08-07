package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/KarimMFadel/halaqaty/backend/internal/auth"
	"github.com/KarimMFadel/halaqaty/backend/internal/middleware"
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
