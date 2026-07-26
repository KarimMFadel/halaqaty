package middleware

import (
	"context"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"halaqaty/backend/internal/platform/httpconst"
)

type wsRateWindow struct {
	windowStart time.Time
	msgCount    int
}

type wsRateKey struct {
	userID   string
	circleID string
}

type wsContextKey string

const wsConnectionTrackerContextKey wsContextKey = "ws-connection-tracker"

type wsConnectionTracker struct {
	release  func()
	upgraded atomic.Bool
}

// WSRateLimitMiddleware enforces websocket connection and message budgets.
type WSRateLimitMiddleware struct {
	maxConnectionsPerUser int
	maxMessagesPerMinute  int
	nowFn                 func() time.Time

	mu            sync.Mutex
	lastCleanup   time.Time
	perUserConns  map[string]int
	perUserCircle map[wsRateKey]wsRateWindow
}

// NewWSRateLimitMiddleware creates websocket rate/connection limiter.
func NewWSRateLimitMiddleware(maxConnectionsPerUser int, maxMessagesPerMinute int) *WSRateLimitMiddleware {
	return &WSRateLimitMiddleware{
		maxConnectionsPerUser: maxConnectionsPerUser,
		maxMessagesPerMinute:  maxMessagesPerMinute,
		nowFn:                 time.Now,
		perUserConns:          map[string]int{},
		perUserCircle:         map[wsRateKey]wsRateWindow{},
	}
}

// LimitUpgrade enforces max concurrent websocket connections per user.
func (m *WSRateLimitMiddleware) LimitUpgrade(next http.Handler) http.Handler {
	if m == nil {
		return next
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		principal, ok := CurrentPrincipal(r.Context())
		if !ok {
			http.Error(w, httpconst.ErrorMessageUnauthorized, http.StatusUnauthorized)
			return
		}

		release, ok := m.acquireConnection(principal.UserID)
		if !ok {
			http.Error(w, httpconst.ErrorMessageWebSocketConnLimitExceeded, http.StatusTooManyRequests)
			return
		}

		tracker := &wsConnectionTracker{release: release}
		ctx := context.WithValue(r.Context(), wsConnectionTrackerContextKey, tracker)
		next.ServeHTTP(w, r.WithContext(ctx))

		if !tracker.upgraded.Load() {
			tracker.release()
		}
	})
}

// MarkWSConnectionUpgraded marks that the request became a websocket connection.
func MarkWSConnectionUpgraded(ctx context.Context) bool {
	tracker, ok := currentWSConnectionTracker(ctx)
	if !ok {
		return false
	}

	tracker.upgraded.Store(true)
	return true
}

// ReleaseWSConnection should be called once when the websocket disconnects.
func ReleaseWSConnection(ctx context.Context) bool {
	tracker, ok := currentWSConnectionTracker(ctx)
	if !ok {
		return false
	}

	tracker.release()
	return true
}

// AllowMessage returns true when message budget is available for the user/circle tuple.
func (m *WSRateLimitMiddleware) AllowMessage(userID string, circleID string) bool {
	if m == nil {
		return true
	}

	userID = strings.TrimSpace(userID)
	circleID = strings.TrimSpace(circleID)
	if userID == "" || circleID == "" {
		return false
	}

	now := m.nowFn().UTC()
	windowStart := now.Truncate(time.Minute)
	key := wsRateKey{userID: userID, circleID: circleID}

	m.mu.Lock()
	defer m.mu.Unlock()

	m.cleanupStaleMessageWindowsLocked(now, windowStart)

	state := m.perUserCircle[key]
	if state.windowStart != windowStart {
		state = wsRateWindow{windowStart: windowStart}
	}
	state.msgCount++
	m.perUserCircle[key] = state

	return state.msgCount <= m.maxMessagesPerMinute
}

func (m *WSRateLimitMiddleware) acquireConnection(userID string) (func(), bool) {
	if m.maxConnectionsPerUser <= 0 {
		return func() {}, true
	}

	trimmedUserID := strings.TrimSpace(userID)
	if trimmedUserID == "" {
		return nil, false
	}

	m.mu.Lock()
	next := m.perUserConns[trimmedUserID] + 1
	if next > m.maxConnectionsPerUser {
		m.mu.Unlock()
		return nil, false
	}
	m.perUserConns[trimmedUserID] = next
	m.mu.Unlock()

	var once sync.Once
	return func() {
		once.Do(func() {
			m.closeConnection(trimmedUserID)
		})
	}, true
}

func (m *WSRateLimitMiddleware) closeConnection(userID string) {
	trimmedUserID := strings.TrimSpace(userID)
	if trimmedUserID == "" {
		return
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	current := m.perUserConns[trimmedUserID]
	if current <= 1 {
		delete(m.perUserConns, trimmedUserID)
		return
	}
	m.perUserConns[trimmedUserID] = current - 1
}

func currentWSConnectionTracker(ctx context.Context) (*wsConnectionTracker, bool) {
	tracker, ok := ctx.Value(wsConnectionTrackerContextKey).(*wsConnectionTracker)
	return tracker, ok
}

func (m *WSRateLimitMiddleware) cleanupStaleMessageWindowsLocked(now time.Time, currentWindow time.Time) {
	if !m.lastCleanup.IsZero() && now.Sub(m.lastCleanup) < time.Minute {
		return
	}

	for key, window := range m.perUserCircle {
		if window.windowStart != currentWindow {
			delete(m.perUserCircle, key)
		}
	}

	m.lastCleanup = now
}
