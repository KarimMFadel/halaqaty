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
	limit         int
}

func (s *discoveryStoreStub) ListPublicCircles(_ context.Context, query string, limit int) ([]rbac.PublicCircleSummary, error) {
	s.query, s.limit = query, limit
	return s.publicCircles, nil
}

func buildDiscoverPublicCirclesRoute(store *discoveryStoreStub) http.Handler {
	handler := rbac.NewHandler(rbac.NewService(store, nil))
	repo := &stubSessionRepo{sessionID: testSessionID, userID: testLocalUserID}
	authMW := middleware.NewAuthMiddleware(&alwaysOKVerifier{}, auth.NewSessionService(30*24*time.Hour), repo)
	return authMW.Require(http.HandlerFunc(handler.DiscoverPublicCircles))
}

func TestCircleDiscoveryContract_PublicSummariesAreFilteredAndPaginated(t *testing.T) {
	store := &discoveryStoreStub{
		circleStoreStub: newCircleStoreStub(),
		publicCircles: []rbac.PublicCircleSummary{{
			ID: "11111111-1111-1111-1111-111111111111", Name: "Noor Circle", MaxCapacity: 50,
			GenderRestriction: "unspecified", Language: "ar",
		}},
	}
	req := httptest.NewRequest(http.MethodGet, "/circles/discover?query=Noor&cursor=next-page", nil)
	req.Header.Set(httpconst.HeaderAuthorization, bearerValid)
	req.Header.Set(httpconst.HeaderSessionID, testSessionID)
	rec := httptest.NewRecorder()

	buildDiscoverPublicCirclesRoute(store).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status: got %d want %d body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}
	if store.query != "Noor" || store.limit == 0 {
		t.Fatalf("discovery filter/pagination: query=%q limit=%d", store.query, store.limit)
	}
	var response struct {
		Data       []json.RawMessage `json:"data"`
		NextCursor *string           `json:"next_cursor"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&response); err != nil {
		t.Fatalf("decode public circle response: %v", err)
	}
	if len(response.Data) != 1 || response.NextCursor == nil {
		t.Fatalf("pagination response: data=%d next_cursor=%v", len(response.Data), response.NextCursor)
	}
	for _, forbidden := range []string{"invite_code", "invite_link", "user_id", "role", "is_private"} {
		if strings.Contains(strings.ToLower(string(response.Data[0])), forbidden) {
			t.Fatalf("public summary leaked %q: %s", forbidden, response.Data[0])
		}
	}
}
