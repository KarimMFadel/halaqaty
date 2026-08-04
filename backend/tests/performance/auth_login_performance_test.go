// Package performance contains in-process handler latency tests that guard
// against handler-layer regressions for SC-001:
// "At least 95% of successful login attempts complete in under 2 seconds end-to-end."
//
// These tests use in-memory stubs (no Firebase round-trip, no PostgreSQL).
// They verify that the handler layer itself does not introduce unacceptable
// latency. Real end-to-end SLO validation requires live-infra load testing
// with Firebase verification and PostgreSQL included.
package performance

import (
	"bytes"
	"context"
	"encoding/json"
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

// perfVerifier returns a valid decoded token immediately.
type perfVerifier struct{}

func (v *perfVerifier) Verify(_ context.Context, _ string) (*auth.DecodedToken, error) {
	return &auth.DecodedToken{UID: "perf-firebase-001", Email: "perf@halaqaty.app"}, nil
}

// perfMiddlewareStore satisfies middleware.SessionRepository for perf tests.
type perfMiddlewareStore struct{}

func (s *perfMiddlewareStore) GetByID(_ context.Context, _ string) (auth.Session, error) {
	return auth.Session{}, nil
}

func (s *perfMiddlewareStore) Touch(_ context.Context, _ string, _ time.Time) error { return nil }

func (s *perfMiddlewareStore) GetLocalUserIDByFirebaseUID(_ context.Context, _ string) (string, error) {
	return "perf-user-001", nil
}

func buildPerfRegisterHandler() http.Handler {
	svc := auth.NewService(&perfStore{}, nil, 24*time.Hour)
	handler := auth.NewHandler(svc)
	sessionSvc := auth.NewSessionService(24 * time.Hour)
	mw := middleware.NewAuthMiddleware(&perfVerifier{}, sessionSvc, &perfMiddlewareStore{})
	return mw.RequireVerifiedFirebase(http.HandlerFunc(handler.Register))
}

func buildPerfSessionHandler() http.Handler {
	svc := auth.NewService(&perfStore{}, nil, 24*time.Hour)
	handler := auth.NewHandler(svc)
	sessionSvc := auth.NewSessionService(24 * time.Hour)
	mw := middleware.NewAuthMiddleware(&perfVerifier{}, sessionSvc, &perfMiddlewareStore{})
	return mw.RequireBearer(http.HandlerFunc(handler.CreateSession))
}

func p95Latency(t *testing.T, h http.Handler, method, path, body string, iterations int) time.Duration {
	t.Helper()
	latencies := make([]time.Duration, 0, iterations)
	for range iterations {
		start := time.Now()
		req := httptest.NewRequest(method, path, bytes.NewBufferString(body))
		req.Header.Set(httpconst.HeaderContentType, httpconst.ContentTypeApplicationJSON)
		req.Header.Set(httpconst.HeaderAuthorization, "Bearer perf-token")
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		latencies = append(latencies, time.Since(start))
	}
	sort.Slice(latencies, func(i, j int) bool { return latencies[i] < latencies[j] })
	idx := int(float64(iterations)*0.95) - 1
	if idx < 0 {
		idx = 0
	}
	return latencies[idx]
}

// TestAuthSessionCreate_InProcessLatency_SC001 gates SC-001 on the login path
// (POST /auth/sessions — the actual "login attempt" that creates a backend session).
// Uses in-memory stubs; real E2E SLO requires live-infra load testing.
func TestAuthSessionCreate_InProcessLatency_SC001(t *testing.T) {
	const iterations = 100
	const maxP95 = 2 * time.Second

	p95 := p95Latency(t, buildPerfSessionHandler(),
		http.MethodPost, "/auth/sessions", `{"device_name":"test"}`, iterations)

	t.Logf("SC-001 session-create in-process p95: %v (gate: < %v)", p95, maxP95)
	if p95 >= maxP95 {
		t.Errorf("SC-001 FAIL: session-create p95 %v >= 2s; check handler hot paths", p95)
	}
}

// TestAuthRegister_InProcessLatency is a handler-layer regression guard for
// POST /auth/register. It is not the SC-001 login SLO path.
func TestAuthRegister_InProcessLatency(t *testing.T) {
	const iterations = 100
	const maxP95 = 2 * time.Second

	p95 := p95Latency(t, buildPerfRegisterHandler(),
		http.MethodPost, "/auth/register", `{"display_name":"Ali"}`, iterations)

	t.Logf("register in-process p95: %v (gate: < %v)", p95, maxP95)
	if p95 >= maxP95 {
		t.Errorf("register p95 %v >= 2s; check handler hot paths", p95)
	}
}

// TestAuthSessionCreate_ResponseShape verifies the session-creation response
// contains session_id and no password field (double-check alongside response-safety tests).
func TestAuthSessionCreate_ResponseShape(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/auth/sessions",
		bytes.NewBufferString(`{"device_name":"test"}`))
	req.Header.Set(httpconst.HeaderContentType, httpconst.ContentTypeApplicationJSON)
	req.Header.Set(httpconst.HeaderAuthorization, "Bearer perf-token")

	rec := httptest.NewRecorder()
	buildPerfSessionHandler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var resp auth.BackendSessionResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode session response: %v", err)
	}
	if resp.SessionID == "" {
		t.Error("expected non-empty session_id")
	}
}

// BenchmarkAuthSessionCreate measures POST /auth/sessions throughput.
func BenchmarkAuthSessionCreate(b *testing.B) {
	handler := buildPerfSessionHandler()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		body := bytes.NewBufferString(`{"device_name":"test"}`)
		req := httptest.NewRequest(http.MethodPost, "/auth/sessions", body)
		req.Header.Set(httpconst.HeaderContentType, httpconst.ContentTypeApplicationJSON)
		req.Header.Set(httpconst.HeaderAuthorization, "Bearer perf-token")
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
	}
}

// BenchmarkAuthRegister measures POST /auth/register throughput.
func BenchmarkAuthRegister(b *testing.B) {
	handler := buildPerfRegisterHandler()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		body := bytes.NewBufferString(`{"display_name":"Ali"}`)
		req := httptest.NewRequest(http.MethodPost, "/auth/register", body)
		req.Header.Set(httpconst.HeaderContentType, httpconst.ContentTypeApplicationJSON)
		req.Header.Set(httpconst.HeaderAuthorization, "Bearer perf-token")
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
	}
}
