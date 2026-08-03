package middleware

import (
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/KarimMFadel/halaqaty/backend/internal/platform/httpconst"
)

type wsRateWindow struct {
	windowStart time.Time
	msgCount    int
}

type wsRateKey struct {
	userID   string
	circleID string
}

// WSRateLimitMiddleware enforces websocket connection and message budgets.
type WSRateLimitMiddleware struct {
	maxConnectionsPerUser int
	maxMessagesPerMinute  int
	nowFn                 func() time.Time

	mu            sync.Mutex
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

// LimitUpgrade checks the per-user connection budget before upgrading.
// The handler is responsible for calling OpenConnection/CloseConnection
// around the actual WebSocket connection loop to track live connections.
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

		// Check budget without incrementing — the handler calls OpenConnection
		// once the WebSocket upgrade succeeds and the connection loop begins.
		if !m.hasCapacity(principal.UserID) {
			http.Error(w, httpconst.ErrorMessageWebSocketConnLimitExceeded, http.StatusTooManyRequests)
			return
		}

		next.ServeHTTP(w, r)
	})
}

// OpenConnection increments the active connection count for userID.
// Must be called by the WebSocket handler after a successful upgrade,
// paired with a deferred CloseConnection call inside the connection loop.
func (m *WSRateLimitMiddleware) OpenConnection(userID string) bool {
	return m.openConnection(userID)
}

// CloseConnection decrements the active connection count for userID.
// Must be deferred by the WebSocket handler when the connection loop exits.
func (m *WSRateLimitMiddleware) CloseConnection(userID string) {
	m.closeConnection(userID)
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

	state := m.perUserCircle[key]
	if state.windowStart != windowStart {
		state = wsRateWindow{windowStart: windowStart}
	}
	state.msgCount++
	m.perUserCircle[key] = state

	return state.msgCount <= m.maxMessagesPerMinute
}

// hasCapacity returns true if the user has room for another connection without incrementing.
func (m *WSRateLimitMiddleware) hasCapacity(userID string) bool {
	if m.maxConnectionsPerUser <= 0 {
		return true
	}
	trimmedUserID := strings.TrimSpace(userID)
	if trimmedUserID == "" {
		return false
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	return m.perUserConns[trimmedUserID] < m.maxConnectionsPerUser
}

func (m *WSRateLimitMiddleware) openConnection(userID string) bool {
	if m.maxConnectionsPerUser <= 0 {
		return true
	}

	trimmedUserID := strings.TrimSpace(userID)
	if trimmedUserID == "" {
		return false
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	next := m.perUserConns[trimmedUserID] + 1
	if next > m.maxConnectionsPerUser {
		return false
	}
	m.perUserConns[trimmedUserID] = next
	return true
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
