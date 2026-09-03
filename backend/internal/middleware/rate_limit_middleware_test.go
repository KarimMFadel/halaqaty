package middleware

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/KarimMFadel/halaqaty/backend/internal/auth"
	phttp "github.com/KarimMFadel/halaqaty/backend/internal/platform/http"
	"github.com/KarimMFadel/halaqaty/backend/internal/platform/httpconst"
)

func TestClientIP_ResolvesFirstForwardedForEntry(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name       string
		forwarded  string
		remoteAddr string
		want       string
	}{
		{
			name:       "first entry of a multi-value forwarded-for header wins",
			forwarded:  "203.0.113.7, 198.51.100.9 , 192.0.2.4",
			remoteAddr: "10.0.0.1:4433",
			want:       "203.0.113.7",
		},
		{
			name:       "single forwarded-for entry is trimmed",
			forwarded:  " 198.51.100.23 ",
			remoteAddr: "10.0.0.2:4433",
			want:       "198.51.100.23",
		},
		{
			name:       "remote address with port yields the host",
			remoteAddr: "192.0.2.10:51234",
			want:       "192.0.2.10",
		},
		{
			name:       "remote address without port is returned as-is",
			remoteAddr: "192.0.2.11",
			want:       "192.0.2.11",
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			req := httptest.NewRequest(http.MethodGet, "/ping", nil)
			req.RemoteAddr = tc.remoteAddr
			if tc.forwarded != "" {
				req.Header.Set(httpconst.HeaderForwardedFor, tc.forwarded)
			}

			if got := clientIP(req); got != tc.want {
				t.Fatalf("clientIP: got %q, want %q", got, tc.want)
			}
		})
	}
}

func TestRateLimitMiddleware_EvictDropsStaleWindowsOnly(t *testing.T) {
	m := NewRateLimitMiddleware(100, 100)
	// Override the clock before any traffic; the janitor goroutine's first
	// tick is a minute out, so the write below never races it in practice.
	now := time.Date(2026, 9, 3, 12, 0, 30, 0, time.UTC)
	m.nowFn = func() time.Time { return now }

	ipLimited := m.LimitByIP(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	userLimited := m.Limit(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	ipReq := httptest.NewRequest(http.MethodGet, "/ping", nil)
	ipReq.RemoteAddr = "203.0.113.20:7777"
	ipLimited.ServeHTTP(httptest.NewRecorder(), ipReq)

	userReq := httptest.NewRequest(http.MethodGet, "/ping", nil).WithContext(
		auth.WithPrincipal(httptest.NewRequest(http.MethodGet, "/ping", nil).Context(), auth.AuthPrincipal{UserID: "evict-user"}),
	)
	userLimited.ServeHTTP(httptest.NewRecorder(), userReq)

	now = now.Add(90 * time.Second)
	m.evict()

	m.mu.Lock()
	staleIP := len(m.ipCounters)
	staleUser := len(m.userCounters)
	m.mu.Unlock()
	if staleIP != 0 || staleUser != 0 {
		t.Fatalf("after eviction: ip counters=%d user counters=%d, want 0/0", staleIP, staleUser)
	}

	// A fresh request after eviction must not be limited by the stale window.
	rec := httptest.NewRecorder()
	ipLimited.ServeHTTP(rec, ipReq)
	if rec.Code != http.StatusOK {
		t.Fatalf("request after eviction: got %d, want %d", rec.Code, http.StatusOK)
	}

	// Counters in the current window survive eviction.
	m.evict()
	m.mu.Lock()
	currentIP := len(m.ipCounters)
	m.mu.Unlock()
	if currentIP != 1 {
		t.Fatalf("current-window ip counters: got %d, want 1", currentIP)
	}
}

func TestRateLimitMiddleware_LimitByIP_WritesErrorEnvelope(t *testing.T) {
	m := NewRateLimitMiddleware(1, 0)
	handler := m.LimitByIP(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	doRequest := func() *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodGet, "/ping", nil)
		req.RemoteAddr = "198.51.100.77:9999"
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		return rec
	}

	if rec := doRequest(); rec.Code != http.StatusOK {
		t.Fatalf("first request: got %d, want %d", rec.Code, http.StatusOK)
	}
	limited := doRequest()
	if limited.Code != http.StatusTooManyRequests {
		t.Fatalf("second request: got %d, want %d body=%s", limited.Code, http.StatusTooManyRequests, limited.Body.String())
	}

	var envelope phttp.ErrorEnvelope
	if err := json.Unmarshal(limited.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("decode error envelope: %v", err)
	}
	if envelope.Error.Code != httpconst.ErrorCodeRateLimitExceeded {
		t.Fatalf("error code: got %q, want %q", envelope.Error.Code, httpconst.ErrorCodeRateLimitExceeded)
	}
	if envelope.Error.Message != httpconst.ErrorMessageRateLimitExceeded {
		t.Fatalf("error message: got %q, want %q", envelope.Error.Message, httpconst.ErrorMessageRateLimitExceeded)
	}
}
