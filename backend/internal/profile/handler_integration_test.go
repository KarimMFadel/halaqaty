//go:build integration

package profile

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/KarimMFadel/halaqaty/backend/internal/auth"
)

func TestHandler_TimezonePersistsAcrossRequests(t *testing.T) {
	repo := newProfileRepository(t)
	userID := seedProfileUser(t, repo, "handler-timezone")
	fullName, country, completedAt := "Profile User", "EG", time.Now().UTC()
	if err := repo.UpdateByUserID(context.Background(), UpdateInput{UserID: userID, FullName: &fullName, Country: &country, CompletedAt: &completedAt}); err != nil {
		t.Fatalf("seed complete profile: %v", err)
	}
	handler := NewHandler(NewService(repo))

	put := func(body string) *httptest.ResponseRecorder {
		t.Helper()
		rec := httptest.NewRecorder()
		req := requestWithPrincipal(http.MethodPut, body)
		principal, _ := auth.CurrentPrincipal(req.Context())
		principal.UserID = userID
		req = req.WithContext(auth.WithPrincipal(req.Context(), principal))
		handler.UpdateMe(rec, req)
		return rec
	}

	if rec := put(`{"timezone":"Africa/Cairo"}`); rec.Code != http.StatusOK {
		t.Fatalf("timezone update: status=%d body=%s", rec.Code, rec.Body.String())
	}
	if rec := put(`{"bio":"old client edit"}`); rec.Code != http.StatusOK {
		t.Fatalf("old-client update: status=%d body=%s", rec.Code, rec.Body.String())
	}
	get := httptest.NewRecorder()
	req := requestWithPrincipal(http.MethodGet, "")
	principal, _ := auth.CurrentPrincipal(req.Context())
	principal.UserID = userID
	handler.GetMe(get, req.WithContext(auth.WithPrincipal(req.Context(), principal)))
	if get.Code != http.StatusOK {
		t.Fatalf("profile read: status=%d body=%s", get.Code, get.Body.String())
	}
	var profile auth.UserProfile
	if err := json.Unmarshal(get.Body.Bytes(), &profile); err != nil {
		t.Fatalf("decode profile: %v", err)
	}
	if profile.Timezone != "Africa/Cairo" {
		t.Fatalf("stored timezone=%q, want Africa/Cairo", profile.Timezone)
	}
}
