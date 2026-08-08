//go:build contract

package contract

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/KarimMFadel/halaqaty/backend/internal/middleware"
	"github.com/KarimMFadel/halaqaty/backend/internal/platform/httpconst"
	"github.com/KarimMFadel/halaqaty/backend/internal/rbac"
)

func TestUserSearchContract(t *testing.T) {
	store := newCircleStoreStub()
	store.searchResults = []rbac.UserSearchResult{{ID: contractTeacherAID, DisplayName: "Aisha"}}
	handler := buildUserSearchRoute(store, nil)

	for _, tc := range []struct {
		name       string
		query      string
		headers    bool
		wantStatus int
		wantBody   string
	}{
		{"requires credentials", "?q=Ai", false, http.StatusUnauthorized, httpconst.ErrorCodeUnauthorized},
		{"validates query", "?q=A", true, http.StatusBadRequest, httpconst.ErrorCodeValidationFailed},
		{"returns minimal user fields", "?q=Ai", true, http.StatusOK, `"display_name":"Aisha"`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/users/search"+tc.query, nil)
			if tc.headers {
				req.Header.Set(httpconst.HeaderAuthorization, "Bearer valid-token")
				req.Header.Set(httpconst.HeaderSessionID, testSessionID)
			}
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)
			if rec.Code != tc.wantStatus || !strings.Contains(rec.Body.String(), tc.wantBody) {
				t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
			}
		})
	}
}

func TestUserSearchContract_RateLimitsRepeatedSearch(t *testing.T) {
	handler := buildUserSearchRoute(
		newCircleStoreStub(),
		middleware.NewRateLimitMiddleware(100, 1),
	)
	for requestNumber, wantStatus := range []int{http.StatusOK, http.StatusTooManyRequests} {
		req := httptest.NewRequest(http.MethodGet, "/users/search?q=Ai", nil)
		req.Header.Set(httpconst.HeaderAuthorization, "Bearer valid-token")
		req.Header.Set(httpconst.HeaderSessionID, testSessionID)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != wantStatus {
			t.Fatalf("request %d: got %d want %d", requestNumber+1, rec.Code, wantStatus)
		}
	}
}
