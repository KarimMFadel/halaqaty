// Package performance contains performance gate tests for SC-001:
// "At least 95% of successful login attempts complete in under 2 seconds end-to-end."
package performance

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"sort"
	"testing"
	"time"

	"github.com/KarimMFadel/halaqaty/backend/internal/auth"
	"github.com/KarimMFadel/halaqaty/backend/internal/middleware"
	"github.com/KarimMFadel/halaqaty/backend/internal/platform/httpconst"
)

// perfStore is an in-memory stub that satisfies auth.Store for performance tests.
type perfStore struct{}

func (s *perfStore) UpsertUserByFirebaseUID(_ context.Context, uid, email string) (auth.User, bool, error) {
	return auth.User{ID: "perf-user-001", FirebaseUID: uid, Email: email}, true, nil
}

func (s *perfStore) UpsertProfileOnRegister(_ context.Context, _, _, _ string) error { return nil }

func (s *perfStore) GetUserProfileByUserID(_ context.Context, _ string) (auth.UserProfile, error) {
	return auth.UserProfile{}, nil
}

func (s *perfStore) CreateSession(_ context.Context, _ auth.Session) error { return nil }

func (s *perfStore) Revoke(_ context.Context, _ string, _ time.Time) error { return nil }

// perfVerifier returns a valid decoded token immediately, simulating fast Firebase verification.
type perfVerifier struct{}

func (v *perfVerifier) Verify(_ context.Context, _ string) (*auth.DecodedToken, error) {
	return &auth.DecodedToken{UID: "perf-firebase-001", Email: "perf@halaqaty.app"}, nil
}

// buildPerfRegisterHandler wires a register handler with in-memory stubs.
func buildPerfRegisterHandler() http.Handler {
	store := &perfStore{}
	svc := auth.NewService(store, nil, 24*time.Hour)
	handler := auth.NewHandler(svc)
	sessionSvc := auth.NewSessionService(24 * time.Hour)
	mw := middleware.NewAuthMiddleware(&perfVerifier{}, sessionSvc, &perfMiddlewareStore{})
	return mw.RequireVerifiedFirebase(http.HandlerFunc(handler.Register))
}

// perfMiddlewareStore satisfies middleware.SessionRepository for the auth middleware.
type perfMiddlewareStore struct{}

func (s *perfMiddlewareStore) GetByID(_ context.Context, _ string) (auth.Session, error) {
	return auth.Session{}, nil
}

func (s *perfMiddlewareStore) Touch(_ context.Context, _ string, _ time.Time) error { return nil }

func (s *perfMiddlewareStore) GetLocalUserIDByFirebaseUID(_ context.Context, _ string) (string, error) {
	return "perf-user-001", nil
}

// TestAuthLogin_P95Latency_SC001 validates SC-001: p95 of successful registration
// attempts is under 2 seconds when using in-memory dependencies (no network I/O).
// This is an in-process gate; real end-to-end latency includes Firebase verification
// and PostgreSQL round-trips that require separate load testing with live infra.
func TestAuthLogin_P95Latency_SC001(t *testing.T) {
	handler := buildPerfRegisterHandler()

	const iterations = 100
	latencies := make([]time.Duration, 0, iterations)

	for range iterations {
		start := time.Now()
		body := bytes.NewBufferString(`{"display_name":"Ali"}`)
		req := httptest.NewRequest(http.MethodPost, "/auth/register", body)
		req.Header.Set(httpconst.HeaderContentType, httpconst.ContentTypeApplicationJSON)
		req.Header.Set(httpconst.HeaderAuthorization, "Bearer perf-token")

		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)

		latencies = append(latencies, time.Since(start))

		if rec.Code != http.StatusCreated && rec.Code != http.StatusConflict {
			t.Fatalf("unexpected status %d on iteration %d: %s", rec.Code, len(latencies), rec.Body.String())
		}
	}

	sort.Slice(latencies, func(i, j int) bool { return latencies[i] < latencies[j] })
	p95idx := int(float64(iterations)*0.95) - 1
	if p95idx < 0 {
		p95idx = 0
	}
	p95 := latencies[p95idx]

	const maxP95 = 2 * time.Second
	t.Logf("SC-001 in-process p95 latency: %v (gate: < %v)", p95, maxP95)
	if p95 >= maxP95 {
		t.Errorf("SC-001 FAIL: p95 latency %v >= 2s gate; check handler hot paths", p95)
	}
}

// BenchmarkAuthRegister measures throughput of POST /auth/register with in-memory deps.
func BenchmarkAuthRegister(b *testing.B) {
	handler := buildPerfRegisterHandler()

	b.ResetTimer()
	for b.Loop() {
		body := bytes.NewBufferString(`{"display_name":"Ali"}`)
		req := httptest.NewRequest(http.MethodPost, "/auth/register", body)
		req.Header.Set(httpconst.HeaderContentType, httpconst.ContentTypeApplicationJSON)
		req.Header.Set(httpconst.HeaderAuthorization, "Bearer perf-token")

		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
	}
}
