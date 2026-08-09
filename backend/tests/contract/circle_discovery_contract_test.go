//go:build contract

package contract

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/KarimMFadel/halaqaty/backend/internal/auth"
	"github.com/KarimMFadel/halaqaty/backend/internal/middleware"
	"github.com/KarimMFadel/halaqaty/backend/internal/platform/httpconst"
	"github.com/KarimMFadel/halaqaty/backend/internal/rbac"
)

type discoveryStoreStub struct {
	*circleStoreStub
	publicCircles []rbac.PublicCircleSummary
	query         string
	cursor        string
	limit         int
}

func (s *discoveryStoreStub) ListPublicCircles(_ context.Context, query, cursor string, limit int) ([]rbac.PublicCircleSummary, error) {
	s.query, s.cursor, s.limit = query, cursor, limit
	return s.publicCircles, nil
}

func buildDiscoverPublicCirclesRoute(store *discoveryStoreStub) http.Handler {
	handler := rbac.NewHandler(rbac.NewService(store, nil))
	repo := &stubSessionRepo{sessionID: testSessionID, userID: testLocalUserID}
	authMW := middleware.NewAuthMiddleware(&alwaysOKVerifier{}, auth.NewSessionService(30*24*time.Hour), repo)
	return authMW.Require(http.HandlerFunc(handler.DiscoverPublicCircles))
}

func TestCircleDiscoveryContract_PublicSummariesAreFilteredAndPaginated(t *testing.T) {
	publicCircles := make([]rbac.PublicCircleSummary, 21)
	for index := range publicCircles {
		publicCircles[index] = rbac.PublicCircleSummary{
			ID: "11111111-1111-1111-1111-111111111111", Name: "Noor Circle", MaxCapacity: 50,
			GenderRestriction: "unspecified", Language: "ar",
		}
	}
	store := &discoveryStoreStub{
		circleStoreStub: newCircleStoreStub(),
		publicCircles:   publicCircles,
	}
	req := httptest.NewRequest(http.MethodGet, "/circles/discover?query=%20Noor%20", nil)
	req.Header.Set(httpconst.HeaderAuthorization, bearerValid)
	req.Header.Set(httpconst.HeaderSessionID, testSessionID)
	rec := httptest.NewRecorder()

	buildDiscoverPublicCirclesRoute(store).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status: got %d want %d body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}
	if store.query != "Noor" || store.cursor != "" || store.limit != 21 {
		t.Fatalf("discovery filter/pagination: query=%q cursor=%q limit=%d", store.query, store.cursor, store.limit)
	}
	var response struct {
		Data       []json.RawMessage `json:"data"`
		NextCursor *string           `json:"next_cursor"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&response); err != nil {
		t.Fatalf("decode public circle response: %v", err)
	}
	if len(response.Data) != 20 || response.NextCursor == nil || *response.NextCursor == "" {
		t.Fatalf("pagination response: data=%d next_cursor=%v", len(response.Data), response.NextCursor)
	}
	for _, forbidden := range []string{"invite_code", "invite_link", "user_id", "role", "is_private"} {
		if strings.Contains(strings.ToLower(string(response.Data[0])), forbidden) {
			t.Fatalf("public summary leaked %q: %s", forbidden, response.Data[0])
		}
	}
}
