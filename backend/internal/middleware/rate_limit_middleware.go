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

// maxRateCounterEntries bounds each counter map so attacker-minted keys
// (e.g. forged X-Forwarded-For hops) cannot grow memory without bound
// between janitor sweeps.
const maxRateCounterEntries = 65536

// RateLimitMiddleware enforces per-IP and per-user request limits.
type RateLimitMiddleware struct {
	perIPPerMin       int
	perUserPerMin     int
	trustedProxyCount int
	maxEntries        int
	nowFn             func() time.Time

	mu           sync.Mutex
	ipCounters   map[string]windowCounter
	userCounters map[string]windowCounter
}

// NewRateLimitMiddleware creates a one-minute fixed-window limiter.
// By default no proxy is trusted: the limiter keys on the connection's
// RemoteAddr and ignores X-Forwarded-For entirely. Deployments behind a
// trusted ingress must opt in via SetTrustedProxyCount.
func NewRateLimitMiddleware(perIPPerMin int, perUserPerMin int) *RateLimitMiddleware {
	m := &RateLimitMiddleware{
		perIPPerMin:   perIPPerMin,
		perUserPerMin: perUserPerMin,
		maxEntries:    maxRateCounterEntries,
		nowFn:         time.Now,
		ipCounters:    map[string]windowCounter{},
		userCounters:  map[string]windowCounter{},
	}
	go m.runEviction()
	return m
}

// SetTrustedProxyCount declares how many proxy hops in front of the service
// are trusted to append truthful X-Forwarded-For entries. The client key is
// then the hop exactly trustedProxyCount positions from the right of the
// chain; everything left of that boundary is client-controlled and ignored.
// Zero (the default) trusts nothing but the connection's RemoteAddr.
// Must be called before the middleware starts serving traffic.
func (m *RateLimitMiddleware) SetTrustedProxyCount(count int) {
	m.trustedProxyCount = count
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
	m.mu.Lock()
	defer m.mu.Unlock()
	m.evictLocked(m.nowFn().UTC())
}

// evictLocked drops stale-window counters; callers must hold m.mu.
func (m *RateLimitMiddleware) evictLocked(now time.Time) {
	currentWindow := now.Truncate(time.Minute)

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
		ip := m.clientIP(r)
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

	counter, exists := counters[key]
	if !exists && len(counters) >= m.maxEntries {
		// ponytail: sweep-then-evict-one at cap; a smallest-windowStart scan
		// would be fairer but is unnecessary while windows are uniform.
		m.evictLocked(now)
		if len(counters) >= m.maxEntries {
			for victim := range counters {
				delete(counters, victim)
				break
			}
		}
	}
	if counter.windowStart != windowStart {
		counter = windowCounter{windowStart: windowStart}
	}

	counter.count++
	counters[key] = counter

	return counter.count > limit
}

// clientIP resolves the rate-limit key for a request. Without trusted
// proxies it is always the connection's RemoteAddr host; X-Forwarded-For is
// never consulted, so clients cannot forge or rotate their limiter key. With
// N trusted proxies it is the hop N positions from the right of the
// X-Forwarded-For chain — the last hop appended by an untrusted party — and
// malformed or shorter chains fall back to RemoteAddr (fail closed).
func (m *RateLimitMiddleware) clientIP(r *http.Request) string {
	remote := remoteHost(r)
	if m.trustedProxyCount <= 0 {
		return remote
	}

	forwardedFor := strings.TrimSpace(r.Header.Get(httpconst.HeaderForwardedFor))
	if forwardedFor == "" {
		return remote
	}

	hops := strings.Split(forwardedFor, ",")
	if len(hops) < m.trustedProxyCount {
		return remote
	}

	hop := strings.TrimSpace(hops[len(hops)-m.trustedProxyCount])
	if hop == "" {
		return remote
	}
	return hop
}

// remoteHost extracts the host portion of the connection address.
func remoteHost(r *http.Request) string {
	host, _, err := net.SplitHostPort(strings.TrimSpace(r.RemoteAddr))
	if err != nil {
		return strings.TrimSpace(r.RemoteAddr)
	}
	return host
}
