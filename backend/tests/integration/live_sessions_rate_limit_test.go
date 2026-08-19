//go:build integration

package integration

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/KarimMFadel/halaqaty/backend/internal/auth"
	"github.com/KarimMFadel/halaqaty/backend/internal/middleware"
	phttp "github.com/KarimMFadel/halaqaty/backend/internal/platform/http"
	"github.com/KarimMFadel/halaqaty/backend/internal/platform/httpconst"
)

func TestLiveSessionsRateLimit_StartJoinShareUserBudget(t *testing.T) {
	limiter := middleware.NewRateLimitMiddleware(100, 2)
	handler := limiter.Limit(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))

	for i, path := range []string{"/api/v1/sessions/session-1/start", "/api/v1/sessions/session-1/join"} {
		recorder := liveSessionRateLimitedRequest(t, handler, "user-1", path)
		if recorder.Code != http.StatusNoContent {
			t.Fatalf("request %d (%s) = %d, want 204", i+1, path, recorder.Code)
		}
	}
	recorder := liveSessionRateLimitedRequest(t, handler, "user-1", "/api/v1/sessions/session-1/join")
	if recorder.Code != http.StatusTooManyRequests {
		t.Fatalf("third request = %d, want 429", recorder.Code)
	}
	var envelope phttp.ErrorEnvelope
	if err := json.Unmarshal(recorder.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("rate-limit response is not JSON: %v", err)
	}
	if envelope.Error.Code != httpconst.ErrorCodeRateLimitExceeded {
		t.Fatalf("rate-limit code = %q, want %q", envelope.Error.Code, httpconst.ErrorCodeRateLimitExceeded)
	}
}

func TestLiveSessionsRateLimit_DifferentUsersRemainIsolated(t *testing.T) {
	limiter := middleware.NewRateLimitMiddleware(100, 1)
	handler := limiter.Limit(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))

	for _, userID := range []string{"teacher-1", "student-1"} {
		recorder := liveSessionRateLimitedRequest(t, handler, userID, "/api/v1/sessions/session-1/join")
		if recorder.Code != http.StatusNoContent {
			t.Fatalf("first request for %s = %d, want 204", userID, recorder.Code)
		}
	}
}

func liveSessionRateLimitedRequest(t *testing.T, handler http.Handler, userID, path string) *httptest.ResponseRecorder {
	t.Helper()
	request := httptest.NewRequest(http.MethodPost, path, nil)
	request = request.WithContext(auth.WithPrincipal(request.Context(), auth.AuthPrincipal{UserID: userID}))
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	return recorder
}
