package middleware

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/KarimMFadel/halaqaty/backend/internal/auth"
	phttp "github.com/KarimMFadel/halaqaty/backend/internal/platform/http"
	"github.com/KarimMFadel/halaqaty/backend/internal/platform/httpconst"
)

func TestWSRateLimitMiddleware_LimitUpgrade_UnauthenticatedRequestRejected(t *testing.T) {
	m := NewWSRateLimitMiddleware(3, 100)
	handler := m.LimitUpgrade(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Error("upgrade handler must not run without a principal")
		w.WriteHeader(http.StatusSwitchingProtocols)
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/ws", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status: got %d, want %d (auth failure, not authorization)", rec.Code, http.StatusUnauthorized)
	}
	var envelope phttp.ErrorEnvelope
	if err := json.Unmarshal(rec.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("decode error envelope: %v", err)
	}
	if envelope.Error.Code != httpconst.ErrorCodeUnauthorized {
		t.Fatalf("error code: got %q, want %q", envelope.Error.Code, httpconst.ErrorCodeUnauthorized)
	}
	if envelope.Error.Message != httpconst.ErrorMessageUnauthorized {
		t.Fatalf("error message: got %q, want %q", envelope.Error.Message, httpconst.ErrorMessageUnauthorized)
	}
}

func TestWSRateLimitMiddleware_LimitUpgrade_ExhaustedBudgetRejected(t *testing.T) {
	const max = 2
	m := NewWSRateLimitMiddleware(max, 100)
	for range max {
		if !m.OpenConnection("ws-budget-user") {
			t.Fatal("seeding connections within budget must succeed")
		}
	}
	handler := m.LimitUpgrade(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Error("upgrade handler must not run once the budget is exhausted")
		w.WriteHeader(http.StatusSwitchingProtocols)
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/ws", nil).WithContext(
		auth.WithPrincipal(httptest.NewRequest(http.MethodGet, "/api/v1/ws", nil).Context(), auth.AuthPrincipal{UserID: "ws-budget-user"}),
	)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("status: got %d, want %d body=%s", rec.Code, http.StatusTooManyRequests, rec.Body.String())
	}
	var envelope phttp.ErrorEnvelope
	if err := json.Unmarshal(rec.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("decode error envelope: %v", err)
	}
	if envelope.Error.Code != httpconst.ErrorCodeRateLimitExceeded {
		t.Fatalf("error code: got %q, want %q", envelope.Error.Code, httpconst.ErrorCodeRateLimitExceeded)
	}
	if envelope.Error.Message != httpconst.ErrorMessageWebSocketConnLimitExceeded {
		t.Fatalf("error message: got %q, want %q", envelope.Error.Message, httpconst.ErrorMessageWebSocketConnLimitExceeded)
	}
}

func TestWSRateLimitMiddleware_LimitUpgrade_AllowsWithinBudget(t *testing.T) {
	m := NewWSRateLimitMiddleware(3, 100)
	reached := false
	handler := m.LimitUpgrade(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		reached = true
		w.WriteHeader(http.StatusSwitchingProtocols)
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/ws", nil).WithContext(
		auth.WithPrincipal(httptest.NewRequest(http.MethodGet, "/api/v1/ws", nil).Context(), auth.AuthPrincipal{UserID: "ws-fresh-user"}),
	)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusSwitchingProtocols {
		t.Fatalf("status: got %d, want %d body=%s", rec.Code, http.StatusSwitchingProtocols, rec.Body.String())
	}
	if !reached {
		t.Fatal("upgrade handler must run while the user has connection budget")
	}
}

func TestWSRateLimitMiddleware_LimitUpgrade_NilMiddlewarePassesThrough(t *testing.T) {
	var m *WSRateLimitMiddleware
	handler := m.LimitUpgrade(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusSwitchingProtocols)
	}))

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/ws", nil))

	if rec.Code != http.StatusSwitchingProtocols {
		t.Fatalf("status: got %d, want %d (nil limiter must not block upgrades)", rec.Code, http.StatusSwitchingProtocols)
	}
}
