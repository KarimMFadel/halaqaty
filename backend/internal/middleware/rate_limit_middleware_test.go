package middleware

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/KarimMFadel/halaqaty/backend/internal/auth"
	phttp "github.com/KarimMFadel/halaqaty/backend/internal/platform/http"
	"github.com/KarimMFadel/halaqaty/backend/internal/platform/httpconst"
)

// newIPLimiter builds a limiter with a fixed clock so windows are deterministic.
func newIPLimiter(t *testing.T, perIP int, trustedProxies int) *RateLimitMiddleware {
	t.Helper()
	m := NewRateLimitMiddleware(perIP, 0)
	m.SetTrustedProxyCount(trustedProxies)
	now := time.Date(2026, 9, 3, 12, 0, 30, 0, time.UTC)
	m.nowFn = func() time.Time { return now }
	return m
}

func TestClientIP_ResolvesOnlyTrustedHops(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name       string
		trusted    int
		forwarded  string
		remoteAddr string
		want       string
	}{
		{
			name:       "no trusted proxies ignores forwarded-for entirely",
			trusted:    0,
			forwarded:  "203.0.113.7, 198.51.100.9 , 192.0.2.4",
			remoteAddr: "10.0.0.1:4433",
			want:       "10.0.0.1",
		},
		{
			name:       "no trusted proxies without header yields the remote host",
			remoteAddr: "192.0.2.10:51234",
			want:       "192.0.2.10",
		},
		{
			name:       "no trusted proxies keeps a portless remote address",
			remoteAddr: "192.0.2.11",
			want:       "192.0.2.11",
		},
		{
			name:       "single trusted proxy resolves the rightmost hop",
			trusted:    1,
			forwarded:  " 198.51.100.23 ",
			remoteAddr: "10.0.0.2:4433",
			want:       "198.51.100.23",
		},
		{
			name:       "single trusted proxy ignores forged leading hops",
			trusted:    1,
			forwarded:  "203.0.113.7, 198.51.100.9",
			remoteAddr: "10.0.0.2:4433",
			want:       "198.51.100.9",
		},
		{
			name:       "two trusted proxies resolve two hops from the right",
			trusted:    2,
			forwarded:  "203.0.113.7, 198.51.100.9, 192.0.2.4",
			remoteAddr: "10.0.0.3:4433",
			want:       "198.51.100.9",
		},
		{
			name:       "chain shorter than the trusted count falls back to remote",
			trusted:    3,
			forwarded:  "198.51.100.23",
			remoteAddr: "10.0.0.4:4433",
			want:       "10.0.0.4",
		},
		{
			name:       "empty hop at the trusted boundary falls back to remote",
			trusted:    1,
			forwarded:  "198.51.100.23, ",
			remoteAddr: "10.0.0.5:4433",
			want:       "10.0.0.5",
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			m := newIPLimiter(t, 1, tc.trusted)
			req := httptest.NewRequest(http.MethodGet, "/ping", nil)
			req.RemoteAddr = tc.remoteAddr
			if tc.forwarded != "" {
				req.Header.Set(httpconst.HeaderForwardedFor, tc.forwarded)
			}

			if got := m.clientIP(req); got != tc.want {
				t.Fatalf("clientIP: got %q, want %q", got, tc.want)
			}
		})
	}
}

func TestRateLimitMiddleware_XFFSpoofingWithoutTrustedProxySharesBucket(t *testing.T) {
	t.Parallel()
	m := newIPLimiter(t, 1, 0)
	handler := m.LimitByIP(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	do := func(xff string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodGet, "/ping", nil)
		req.RemoteAddr = "203.0.113.50:1000"
		if xff != "" {
			req.Header.Set(httpconst.HeaderForwardedFor, xff)
		}
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		return rec
	}

	if rec := do("198.51.100.1"); rec.Code != http.StatusOK {
		t.Fatalf("first request: got %d, want %d", rec.Code, http.StatusOK)
	}
	// Rotating a forged X-Forwarded-For must not mint a new limiter bucket
	// when no proxy is trusted.
	if rec := do("198.51.100.2"); rec.Code != http.StatusTooManyRequests {
		t.Fatalf("rotated XFF request: got %d, want %d", rec.Code, http.StatusTooManyRequests)
	}
}

func TestRateLimitMiddleware_TrustedProxyForgeryBeyondBoundaryIgnored(t *testing.T) {
	t.Parallel()
	m := newIPLimiter(t, 1, 1)
	handler := m.LimitByIP(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	do := func(xff string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodGet, "/ping", nil)
		req.RemoteAddr = "10.0.0.9:1111"
		req.Header.Set(httpconst.HeaderForwardedFor, xff)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		return rec
	}

	if rec := do("198.51.100.5"); rec.Code != http.StatusOK {
		t.Fatalf("first request: got %d, want %d", rec.Code, http.StatusOK)
	}
	// Prepended forged hops are left of the trusted boundary; the rightmost
	// hop (the trusted proxy's view) still keys the limiter.
	if rec := do("203.0.113.1, 203.0.113.2, 198.51.100.5"); rec.Code != http.StatusTooManyRequests {
		t.Fatalf("forged-hop request: got %d, want %d", rec.Code, http.StatusTooManyRequests)
	}
}

func TestRateLimitMiddleware_MaxEntriesCapsCounterMap(t *testing.T) {
	t.Parallel()
	m := newIPLimiter(t, 1000, 0)
	m.maxEntries = 2
	handler := m.LimitByIP(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	for i := 0; i < 5; i++ {
		req := httptest.NewRequest(http.MethodGet, "/ping", nil)
		req.RemoteAddr = fmt.Sprintf("203.0.113.%d:1000", i)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("request %d: got %d, want %d", i, rec.Code, http.StatusOK)
		}
	}

	m.mu.Lock()
	got := len(m.ipCounters)
	m.mu.Unlock()
	if got != m.maxEntries {
		t.Fatalf("ip counters: got %d, want capped at %d", got, m.maxEntries)
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
