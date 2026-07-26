package middleware

import (
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"halaqaty/backend/internal/platform/httpconst"
)

type windowCounter struct {
	windowStart time.Time
	count       int
}

// RateLimitMiddleware enforces per-IP and per-user request limits.
type RateLimitMiddleware struct {
	perIPPerMin   int
	perUserPerMin int
	nowFn         func() time.Time

	mu           sync.Mutex
	lastCleanup  time.Time
	ipCounters   map[string]windowCounter
	userCounters map[string]windowCounter
}

// NewRateLimitMiddleware creates a one-minute fixed-window limiter.
func NewRateLimitMiddleware(perIPPerMin int, perUserPerMin int) *RateLimitMiddleware {
	return &RateLimitMiddleware{
		perIPPerMin:   perIPPerMin,
		perUserPerMin: perUserPerMin,
		nowFn:         time.Now,
		ipCounters:    map[string]windowCounter{},
		userCounters:  map[string]windowCounter{},
	}
}

// Limit rejects requests exceeding either IP or user budget.
func (m *RateLimitMiddleware) Limit(next http.Handler) http.Handler {
	if m == nil {
		return next
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ip := clientIP(r)
		if ip != "" && m.hitLimit(m.ipCounters, ip, m.perIPPerMin) {
			http.Error(w, httpconst.ErrorMessageRateLimitExceeded, http.StatusTooManyRequests)
			return
		}

		if principal, ok := CurrentPrincipal(r.Context()); ok {
			if m.hitLimit(m.userCounters, principal.UserID, m.perUserPerMin) {
				http.Error(w, httpconst.ErrorMessageRateLimitExceeded, http.StatusTooManyRequests)
				return
			}
		}

		next.ServeHTTP(w, r)
	})
}

func (m *RateLimitMiddleware) hitLimit(counters map[string]windowCounter, key string, limit int) bool {
	if limit <= 0 || key == "" {
		return false
	}

	now := m.nowFn().UTC()
	windowStart := now.Truncate(time.Minute)

	m.mu.Lock()
	defer m.mu.Unlock()

	m.cleanupStaleCountersLocked(now, windowStart)

	counter := counters[key]
	if counter.windowStart != windowStart {
		counter = windowCounter{windowStart: windowStart}
	}

	counter.count++
	counters[key] = counter

	return counter.count > limit
}

func (m *RateLimitMiddleware) cleanupStaleCountersLocked(now time.Time, currentWindow time.Time) {
	if !m.lastCleanup.IsZero() && now.Sub(m.lastCleanup) < time.Minute {
		return
	}

	for key, counter := range m.ipCounters {
		if counter.windowStart != currentWindow {
			delete(m.ipCounters, key)
		}
	}
	for key, counter := range m.userCounters {
		if counter.windowStart != currentWindow {
			delete(m.userCounters, key)
		}
	}

	m.lastCleanup = now
}

func clientIP(r *http.Request) string {
	if forwardedFor := strings.TrimSpace(r.Header.Get(httpconst.HeaderForwardedFor)); forwardedFor != "" {
		parts := strings.Split(forwardedFor, ",")
		return strings.TrimSpace(parts[0])
	}

	host, _, err := net.SplitHostPort(strings.TrimSpace(r.RemoteAddr))
	if err != nil {
		return strings.TrimSpace(r.RemoteAddr)
	}
	return host
}
