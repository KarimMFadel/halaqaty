//go:build integration

// Package integration contains feature-001 integration tests.
// RateLimitPolicy tests exercise the in-memory rate limiters and do not
// require a database connection; they are tagged integration to run in the
// same suite as other integration tests.
package integration

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/KarimMFadel/halaqaty/backend/internal/auth"
	"github.com/KarimMFadel/halaqaty/backend/internal/middleware"
	"github.com/KarimMFadel/halaqaty/backend/internal/platform/httpconst"
)

// ratePrincipal returns a principal-injecting handler that sets the given userID.
func ratePrincipal(userID string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		principal := auth.AuthPrincipal{UserID: userID, FirebaseUID: "uid-" + userID}
		next.ServeHTTP(w, r.WithContext(auth.WithPrincipal(r.Context(), principal)))
	})
}

// rateOKHandler is a trivial 200 endpoint used to count successful vs. limited requests.
var rateOKHandler = http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusOK)
})

// hitCount counts how many responses have the given status out of n requests.
func hitCount(t *testing.T, h http.Handler, n int, ip, userID string) (ok, limited int) {
	t.Helper()
	for range n {
		req := httptest.NewRequest(http.MethodGet, "/ping", nil)
		if ip != "" {
			req.RemoteAddr = ip + ":9000"
		}
		if userID != "" {
			req = req.WithContext(auth.WithPrincipal(req.Context(), auth.AuthPrincipal{UserID: userID}))
		}
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if rec.Code == http.StatusTooManyRequests {
			limited++
		} else {
			ok++
		}
	}
	return ok, limited
}

// TestRateLimitPolicy_IPLimit asserts that requests beyond perIPPerMin from the
// same IP are rejected with 429 by LimitByIP.
func TestRateLimitPolicy_IPLimit(t *testing.T) {
	const limit = 5
	rl := middleware.NewRateLimitMiddleware(limit, 1000)
	handler := rl.LimitByIP(rateOKHandler)

	ok, limited := hitCount(t, handler, limit+3, "10.0.0.1", "")
	if ok != limit {
		t.Errorf("expected %d successful requests before IP cap, got %d", limit, ok)
	}
	if limited != 3 {
		t.Errorf("expected 3 rate-limited responses after IP cap, got %d", limited)
	}
}

// TestRateLimitPolicy_UserLimit asserts per-user request cap is enforced with 429 by Limit.
func TestRateLimitPolicy_UserLimit(t *testing.T) {
	const limit = 5
	rl := middleware.NewRateLimitMiddleware(1000, limit)
	handler := rl.Limit(rateOKHandler)

	ok, limited := hitCount(t, handler, limit+2, "", "user-abc")
	if ok != limit {
		t.Errorf("expected %d successful requests before user cap, got %d", limit, ok)
	}
	if limited != 2 {
		t.Errorf("expected 2 rate-limited responses after user cap, got %d", limited)
	}
}

// TestRateLimitPolicy_DifferentIPsNotBlocked asserts distinct IPs have independent
// budgets under LimitByIP.
func TestRateLimitPolicy_DifferentIPsNotBlocked(t *testing.T) {
	const limit = 3
	rl := middleware.NewRateLimitMiddleware(limit, 1000)
	handler := rl.LimitByIP(rateOKHandler)

	// Both IPs should get their own full budget.
	ok1, _ := hitCount(t, handler, limit, "10.0.0.1", "")
	ok2, _ := hitCount(t, handler, limit, "10.0.0.2", "")
	if ok1 != limit || ok2 != limit {
		t.Errorf("distinct IPs should each get %d requests; got ip1=%d ip2=%d", limit, ok1, ok2)
	}
}

// wsRateMock satisfies the auth principal requirement for WS tests.
func newWSReq(userID string) *http.Request {
	req := httptest.NewRequest(http.MethodGet, "/ws", nil)
	return req.WithContext(auth.WithPrincipal(req.Context(), auth.AuthPrincipal{UserID: userID}))
}

// TestRateLimitPolicy_WSConnectionLimit asserts that the 4th connection attempt
// is rejected when maxConnectionsPerUser is 3.
func TestRateLimitPolicy_WSConnectionLimit(t *testing.T) {
	const max = 3
	wsrl := middleware.NewWSRateLimitMiddleware(max, 1000)

	for i := range max {
		if !wsrl.OpenConnection("ws-user-1") {
			t.Fatalf("connection %d should be accepted (max=%d)", i+1, max)
		}
	}
	if wsrl.OpenConnection("ws-user-1") {
		t.Error("4th connection should be rejected when max=3")
	}
}

// TestRateLimitPolicy_WSConnectionLimitUpgradeHandler asserts LimitUpgrade returns
// 429 when the connection budget is exhausted.
func TestRateLimitPolicy_WSConnectionLimitUpgradeHandler(t *testing.T) {
	const max = 2
	wsrl := middleware.NewWSRateLimitMiddleware(max, 1000)

	// Pre-fill the budget.
	for range max {
		wsrl.OpenConnection("ws-user-2")
	}

	handler := wsrl.LimitUpgrade(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusSwitchingProtocols)
	}))

	req := newWSReq("ws-user-2")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusTooManyRequests {
		t.Errorf("expected 429 when WS connection budget exhausted, got %d", rec.Code)
	}
}

// TestRateLimitPolicy_WSCloseConnection asserts that CloseConnection frees a slot.
func TestRateLimitPolicy_WSCloseConnection(t *testing.T) {
	const max = 1
	wsrl := middleware.NewWSRateLimitMiddleware(max, 1000)

	if !wsrl.OpenConnection("ws-user-3") {
		t.Fatal("first connection should be accepted")
	}
	if wsrl.OpenConnection("ws-user-3") {
		t.Fatal("second connection should be rejected when max=1")
	}
	wsrl.CloseConnection("ws-user-3")
	if !wsrl.OpenConnection("ws-user-3") {
		t.Error("connection after close should be accepted")
	}
}

// TestRateLimitPolicy_WSMessageLimit asserts per-user/circle message cap.
func TestRateLimitPolicy_WSMessageLimit(t *testing.T) {
	const max = 5
	wsrl := middleware.NewWSRateLimitMiddleware(100, max)

	allowed := 0
	for i := range max + 3 {
		if wsrl.AllowMessage("msg-user-1", "circle-1") {
			allowed++
		}
		_ = i
	}
	if allowed != max {
		t.Errorf("expected %d allowed messages before cap, got %d", max, allowed)
	}
}

// TestRateLimitPolicy_WSMessageDifferentCircles asserts circle budgets are independent.
func TestRateLimitPolicy_WSMessageDifferentCircles(t *testing.T) {
	const max = 3
	wsrl := middleware.NewWSRateLimitMiddleware(100, max)

	for range max {
		wsrl.AllowMessage("msg-user-2", "circle-A")
	}
	// circle-B should still have full budget.
	if !wsrl.AllowMessage("msg-user-2", "circle-B") {
		t.Error("different circle should have independent message budget")
	}
}

// TestRateLimitPolicy_LimitByIPOnlyChecksIP asserts that LimitByIP does not
// enforce per-user limits even when a principal is present in context.
func TestRateLimitPolicy_LimitByIPOnlyChecksIP(t *testing.T) {
	const ipLimit = 2
	const userLimit = 1 // would block after 1 if enforced
	rl := middleware.NewRateLimitMiddleware(ipLimit, userLimit)
	handler := rl.LimitByIP(rateOKHandler)

	// With a principal injected, LimitByIP should still allow > userLimit requests
	// because it only checks IP, not the user budget.
	for i := range userLimit + 2 {
		req := httptest.NewRequest(http.MethodGet, "/ping", nil)
		req.RemoteAddr = "10.0.1.1:0"
		req = req.WithContext(auth.WithPrincipal(req.Context(), auth.AuthPrincipal{UserID: "byip-user"}))
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		// All within ipLimit should succeed; user budget is irrelevant for LimitByIP.
		if i < ipLimit && rec.Code != http.StatusOK {
			t.Errorf("LimitByIP should not enforce user limit; got %d on req %d", rec.Code, i+1)
		}
	}
}

// TestRateLimitPolicy_LimitEnforcesUserAfterPrincipalSet asserts that Limit
// (used after auth sets the principal) does enforce per-user budgets.
func TestRateLimitPolicy_LimitEnforcesUserAfterPrincipalSet(t *testing.T) {
	const userLimit = 3
	rl := middleware.NewRateLimitMiddleware(1000, userLimit)
	handler := rl.Limit(rateOKHandler)

	// Simulate requests after auth has set the principal (correct wiring order).
	ok, limited := hitCount(t, handler, userLimit+2, "", "after-auth-user")
	if ok != userLimit {
		t.Errorf("expected %d ok before user cap, got %d", userLimit, ok)
	}
	if limited != 2 {
		t.Errorf("expected 2 limited after user cap, got %d", limited)
	}
}

// TestRateLimitPolicy_ResponseShape asserts 429 response contains JSON error envelope.
// Uses Limit (user-only post-auth limiter) with a principal injected.
func TestRateLimitPolicy_ResponseShape(t *testing.T) {
	const limit = 1
	rl := middleware.NewRateLimitMiddleware(1000, limit)
	handler := rl.Limit(rateOKHandler)

	// First request consumed; second is rate-limited.
	for i := range 2 {
		req := httptest.NewRequest(http.MethodGet, "/ping", nil)
		req.RemoteAddr = "1.2.3.4:0"
		req = req.WithContext(auth.WithPrincipal(req.Context(), auth.AuthPrincipal{UserID: "shape-user"}))
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if i == 1 {
			if rec.Code != http.StatusTooManyRequests {
				t.Fatalf("expected 429, got %d", rec.Code)
			}
			ct := rec.Header().Get(httpconst.HeaderContentType)
			if ct != httpconst.ContentTypeApplicationJSON {
				t.Errorf("expected JSON content-type on 429, got %q", ct)
			}
		}
	}
}
