//go:build integration

package integration

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/KarimMFadel/halaqaty/backend/internal/auth"
	"github.com/KarimMFadel/halaqaty/backend/internal/middleware"
	"github.com/KarimMFadel/halaqaty/backend/internal/platform/httpconst"
)

func newQueueRateLimitRequest(authorization, remoteIP string) *http.Request {
	req := httptest.NewRequest(http.MethodGet, "/queue", nil)
	req.Header.Set(httpconst.HeaderAuthorization, authorization)
	req.RemoteAddr = remoteIP + ":1234"
	return req
}

func doQueueRateLimitRequest(handler http.Handler, req *http.Request) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	return rec
}

func TestRecitationQueueRateLimit_PerIPAndUserReturn429WithoutMutation(t *testing.T) {
	limiter := middleware.NewRateLimitMiddleware(2, 2)
	called := 0
	next := http.HandlerFunc(func(http.ResponseWriter, *http.Request) { called++ })
	handler := limiter.LimitByIP(limiter.Limit(next))
	for attempt := 0; attempt < 2; attempt++ {
		req := newQueueRateLimitRequest("Bearer user-1", "203.0.113.10")
		rec := doQueueRateLimitRequest(handler, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("allowed request %d status=%d", attempt, rec.Code)
		}
	}
	rec := doQueueRateLimitRequest(handler, newQueueRateLimitRequest("Bearer user-1", "203.0.113.10"))
	if rec.Code != http.StatusTooManyRequests || called != 2 {
		t.Fatalf("limited request status=%d called=%d, want 429/2", rec.Code, called)
	}
	for attempt := 0; attempt < 2; attempt++ {
		req := newQueueRateLimitRequest("Bearer user-1", "203.0.113.11")
		req = req.WithContext(auth.WithPrincipal(req.Context(), auth.AuthPrincipal{UserID: "user-1"}))
		if rec := doQueueRateLimitRequest(handler, req); rec.Code != http.StatusOK {
			t.Fatalf("user-allowed request %d status=%d", attempt, rec.Code)
		}
	}
	req := newQueueRateLimitRequest("Bearer user-1", "203.0.113.12")
	req = req.WithContext(auth.WithPrincipal(req.Context(), auth.AuthPrincipal{UserID: "user-1"}))
	if rec := doQueueRateLimitRequest(handler, req); rec.Code != http.StatusTooManyRequests {
		t.Fatal("per-user budget did not return 429")
	}
	if rec.Header().Get(httpconst.HeaderContentType) == "" {
		t.Fatal("rate-limit response omitted content type")
	}
}
