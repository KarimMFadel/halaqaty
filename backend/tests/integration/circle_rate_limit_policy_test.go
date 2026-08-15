//go:build integration

package integration

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/KarimMFadel/halaqaty/backend/internal/auth"
	"github.com/KarimMFadel/halaqaty/backend/internal/middleware"
	phttp "github.com/KarimMFadel/halaqaty/backend/internal/platform/http"
)

func TestCircleRateLimitPolicy_ReadsDiscoveryAndMutationsShareUserBudget(t *testing.T) {
	limiter := middleware.NewRateLimitMiddleware(100, 3)
	handler := limiter.Limit(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	paths := []string{
		"/circles/discover",
		"/circles/circle-1",
		"/circles/circle-1/members",
		"/circles/circle-1/invite-code/refresh",
	}
	for index, path := range paths {
		request := httptest.NewRequest(http.MethodGet, path, nil)
		request = request.WithContext(auth.WithPrincipal(request.Context(), auth.AuthPrincipal{UserID: "user-1"}))
		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, request)
		want := http.StatusOK
		if index == len(paths)-1 {
			want = http.StatusTooManyRequests
		}
		if recorder.Code != want {
			t.Fatalf("%s: got %d want %d", path, recorder.Code, want)
		}
	}
}

func TestCircleRateLimitPolicy_ReadAndMutationTimeoutsUseStandardStatus(t *testing.T) {
	for _, path := range []string{
		"/circles/discover",
		"/circles/circle-1/invite-code/refresh",
	} {
		t.Run(path, func(t *testing.T) {
			handler := phttp.TimeoutMiddleware(time.Millisecond, http.HandlerFunc(
				func(http.ResponseWriter, *http.Request) { time.Sleep(10 * time.Millisecond) },
			))
			recorder := httptest.NewRecorder()
			handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, path, nil))
			if recorder.Code != http.StatusServiceUnavailable {
				t.Fatalf("status: got %d want %d", recorder.Code, http.StatusServiceUnavailable)
			}
		})
	}
}
