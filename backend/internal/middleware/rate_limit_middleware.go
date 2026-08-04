package middleware

import (
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	phttp "github.com/KarimMFadel/halaqaty/backend/internal/platform/http"
	"github.com/KarimMFadel/halaqaty/backend/internal/platform/httpconst"
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
	ipCounters   map[string]windowCounter
	userCounters map[string]windowCounter
}

// NewRateLimitMiddleware creates a one-minute fixed-window limiter.
func NewRateLimitMiddleware(perIPPerMin int, perUserPerMin int) *RateLimitMiddleware {
	m := &RateLimitMiddleware{
		perIPPerMin:   perIPPerMin,
		perUserPerMin: perUserPerMin,
		nowFn:         time.Now,
		ipCounters:    map[string]windowCounter{},
		userCounters:  map[string]windowCounter{},
	}
	go m.runEviction()
	return m
}

// runEviction periodically removes stale counters to prevent unbounded memory growth.
func (m *RateLimitMiddleware) runEviction() {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	for range ticker.C {
		m.evict()
	}
}

// evict removes counters that belong to a window older than the current one.
func (m *RateLimitMiddleware) evict() {
	now := m.nowFn().UTC()
	currentWindow := now.Truncate(time.Minute)

	m.mu.Lock()
	defer m.mu.Unlock()

	for key, c := range m.ipCounters {
		if c.windowStart.Before(currentWindow) {
			delete(m.ipCounters, key)
		}
	}
	for key, c := range m.userCounters {
		if c.windowStart.Before(currentWindow) {
			delete(m.userCounters, key)
		}
	}
}

// Limit rejects requests exceeding the per-user budget.
// It must be applied after auth middleware has set the principal so that
// per-user limits are enforced. IP limiting is handled separately by LimitByIP
// at the global layer; calling Limit post-auth avoids counting each protected
// request twice against the IP budget.
func (m *RateLimitMiddleware) Limit(next http.Handler) http.Handler {
	if m == nil {
		return next
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if principal, ok := CurrentPrincipal(r.Context()); ok {
			if m.hitLimit(m.userCounters, principal.UserID, m.perUserPerMin) {
				phttp.WriteError(
					w,
					httpconst.ErrorCodeRateLimitExceeded,
					httpconst.ErrorMessageRateLimitExceeded,
					http.StatusTooManyRequests,
				)
				return
			}
		}

		next.ServeHTTP(w, r)
	})
}

// LimitByIP rejects requests exceeding the per-IP budget.
// Use this at the global middleware layer (before auth) so that the IP cap
// is enforced even on unauthenticated requests. Per-user limiting requires
// the principal to be in context; use Limit after auth middleware instead.
func (m *RateLimitMiddleware) LimitByIP(next http.Handler) http.Handler {
	if m == nil {
		return next
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ip := clientIP(r)
		if ip != "" && m.hitLimit(m.ipCounters, ip, m.perIPPerMin) {
			phttp.WriteError(
				w,
				httpconst.ErrorCodeRateLimitExceeded,
				httpconst.ErrorMessageRateLimitExceeded,
				http.StatusTooManyRequests,
			)
			return
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

	counter := counters[key]
	if counter.windowStart != windowStart {
		counter = windowCounter{windowStart: windowStart}
	}

	counter.count++
	counters[key] = counter

	return counter.count > limit
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
