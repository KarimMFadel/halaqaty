//go:build integration

package integration

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/KarimMFadel/halaqaty/backend/internal/auth"
	"github.com/KarimMFadel/halaqaty/backend/internal/middleware"
	"github.com/KarimMFadel/halaqaty/backend/internal/platform/httpconst"
)

func TestRecitationQueueRateLimit_PerIPAndUserReturn429WithoutMutation(t *testing.T) {
	env := setupQueueRBACEnv(t)
	newHandler := func(limit int) http.Handler {
		limiter := middleware.NewRateLimitMiddleware(limit, limit)
		return limiter.LimitByIP(limiter.Limit(env.mux))
	}
	baseHeaders := func(actor string) map[string]string {
		return map[string]string{
			httpconst.HeaderAuthorization: env.tokens[actor],
			httpconst.HeaderSessionID:     env.backendSessions[actor],
			httpconst.HeaderContentType:   httpconst.ContentTypeApplicationJSON,
		}
	}

	t.Run("per-IP queue reads reject before queue handler", func(t *testing.T) {
		handler := newHandler(2)
		for _, actor := range []string{"teacher", "student"} {
			response := doQueueRateLimitedRequest(t, handler, http.MethodGet, "/api/v1/sessions/"+env.sessionID+"/queue", "", baseHeaders(actor), "203.0.113.10", env.userIDs[actor])
			if response.Code != http.StatusOK {
				t.Fatalf("allowed queue read status=%d: %s", response.Code, response.Body.String())
			}
		}
		before := env.queueTablesSnapshot(t, env.sessionID)
		response := doQueueRateLimitedRequest(t, handler, http.MethodGet, "/api/v1/sessions/"+env.sessionID+"/queue", "", baseHeaders("teacher"), "203.0.113.10", env.userIDs["teacher"])
		assertQueueRateLimitedWithoutMutation(t, response, before, env)
	})

	t.Run("per-user queue mutation rejects before persistence", func(t *testing.T) {
		handler := newHandler(1)
		path := "/api/v1/sessions/" + env.sessionID + "/queue/rounds"
		body := `{"round_type":"test","surah_id":2,"from_ayah":1,"to_ayah":3,"grading_required":false}`
		first := doQueueRateLimitedRequest(t, handler, http.MethodPost, path, body, baseHeaders("teacher"), "203.0.113.11", env.userIDs["teacher"])
		if first.Code != http.StatusCreated {
			t.Fatalf("allowed queue mutation status=%d: %s", first.Code, first.Body.String())
		}
		before := env.queueTablesSnapshot(t, env.sessionID)
		response := doQueueRateLimitedRequest(t, handler, http.MethodPost, path, body, baseHeaders("teacher"), "203.0.113.12", env.userIDs["teacher"])
		assertQueueRateLimitedWithoutMutation(t, response, before, env)
	})
}

func doQueueRateLimitedRequest(t *testing.T, handler http.Handler, method, target, body string, headers map[string]string, remoteIP, userID string) *httptest.ResponseRecorder {
	t.Helper()
	request := httptest.NewRequest(method, target, strings.NewReader(body))
	request.RemoteAddr = remoteIP + ":1234"
	for name, value := range headers {
		request.Header.Set(name, value)
	}
	request = request.WithContext(auth.WithPrincipal(request.Context(), auth.AuthPrincipal{UserID: userID}))
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	return response
}

func assertQueueRateLimitedWithoutMutation(t *testing.T, response *httptest.ResponseRecorder, before [5]int, env *queueRBACEnv) {
	t.Helper()
	if response.Code != http.StatusTooManyRequests {
		t.Fatalf("limited request status=%d, want 429: %s", response.Code, response.Body.String())
	}
	if response.Header().Get(httpconst.HeaderContentType) == "" {
		t.Fatal("rate-limit response omitted content type")
	}
	if after := env.queueTablesSnapshot(t, env.sessionID); after != before {
		t.Fatalf("rate-limited request mutated queue tables: before=%v after=%v", before, after)
	}
}
