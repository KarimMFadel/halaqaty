//go:build contract

package contract

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/KarimMFadel/halaqaty/backend/internal/auth"
	"github.com/KarimMFadel/halaqaty/backend/internal/middleware"
	"github.com/KarimMFadel/halaqaty/backend/internal/platform/httpconst"
	"github.com/KarimMFadel/halaqaty/backend/internal/rbac"
)

func buildGetCircleRoute(store *circleStoreStub) http.Handler {
	handler := rbac.NewHandler(rbac.NewService(store, nil))
	repo := &stubSessionRepo{sessionID: testSessionID, userID: testLocalUserID}
	authMW := middleware.NewAuthMiddleware(&alwaysOKVerifier{}, auth.NewSessionService(30*24*time.Hour), repo)
	mux := http.NewServeMux()
	mux.Handle("GET /api/v1/circles/{circleId}", authMW.Require(http.HandlerFunc(handler.GetCircle)))
	mux.Handle("GET /api/v1/circles/{circleId}/members", authMW.Require(http.HandlerFunc(handler.ListMembers)))
	return mux
}

func TestCircleReadContract(t *testing.T) {
	store := newCircleStoreStub()
	store.circles[contractCircleID] = rbac.Circle{
		ID:                contractCircleID,
		Name:              "My Circle",
		InviteCode:        "HLQ-7X2K",
		MaxCapacity:       50,
		IsPrivate:         true,
		GenderRestriction: "unspecified",
		Language:          "ar",
		CreatedAt:         time.Now().UTC(),
	}
	store.members[contractCircleID] = map[string]string{testLocalUserID: rbac.RoleStudent}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/circles/"+contractCircleID, nil)
	req.Header.Set(httpconst.HeaderAuthorization, bearerValid)
	req.Header.Set(httpconst.HeaderSessionID, testSessionID)
	rec := httptest.NewRecorder()

	buildGetCircleRoute(store).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status: got %d want %d body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}
	var circle map[string]any
	if err := json.NewDecoder(rec.Body).Decode(&circle); err != nil {
		t.Fatalf("decode get circle: %v", err)
	}
	for _, key := range []string{"id", "name", "invite_code", "invite_link", "max_capacity", "is_private", "gender_restriction", "language", "is_archived", "created_at"} {
		if _, ok := circle[key]; !ok {
			t.Fatalf("missing key %q in circle response: %v", key, circle)
		}
	}
	inviteLink, _ := circle["invite_link"].(string)
	if inviteLink != "https://halaqaty.app/join/HLQ-7X2K" {
		t.Fatalf("invite_link: got %q, want https://halaqaty.app/join/HLQ-7X2K", inviteLink)
	}
}

func TestCircleReadContract_MemberListSchema(t *testing.T) {
	store := newCircleStoreStub()
	store.circles[contractCircleID] = rbac.Circle{ID: contractCircleID, Name: "My Circle", CreatedAt: time.Now().UTC()}
	store.members[contractCircleID] = map[string]string{
		testLocalUserID:    rbac.RoleStudent,
		contractTeacherAID: rbac.RoleTeacher,
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/circles/"+contractCircleID+"/members", nil)
	req.Header.Set(httpconst.HeaderAuthorization, bearerValid)
	req.Header.Set(httpconst.HeaderSessionID, testSessionID)
	rec := httptest.NewRecorder()

	buildGetCircleRoute(store).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status: got %d want %d body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}
	var response struct {
		Data []map[string]any `json:"data"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&response); err != nil {
		t.Fatalf("decode member list: %v", err)
	}
	if len(response.Data) == 0 {
		t.Fatalf("expected at least one member in response")
	}
	for _, member := range response.Data {
		for _, key := range []string{"user_id", "display_name", "role", "joined_at"} {
			if _, ok := member[key]; !ok {
				t.Fatalf("member missing key %q: %v", key, member)
			}
		}
	}
}

func TestCircleReadContract_AccessBoundariesAndArchivedRead(t *testing.T) {
	for _, tc := range []struct {
		name       string
		path       string
		seed       func(*circleStoreStub)
		withAuth   bool
		wantStatus int
	}{
		{
			name: "allows archived circle read for member",
			path: "/api/v1/circles/" + contractCircleID,
			seed: func(store *circleStoreStub) {
				store.circles[contractCircleID] = rbac.Circle{ID: contractCircleID, Name: "Archived Circle", IsArchived: true, CreatedAt: time.Now().UTC()}
				store.members[contractCircleID] = map[string]string{testLocalUserID: rbac.RoleStudent}
			},
			withAuth:   true,
			wantStatus: http.StatusOK,
		},
		{
			name: "allows archived member list for member",
			path: "/api/v1/circles/" + contractCircleID + "/members",
			seed: func(store *circleStoreStub) {
				store.circles[contractCircleID] = rbac.Circle{ID: contractCircleID, Name: "Archived Circle", IsArchived: true, CreatedAt: time.Now().UTC()}
				store.members[contractCircleID] = map[string]string{testLocalUserID: rbac.RoleStudent}
			},
			withAuth:   true,
			wantStatus: http.StatusOK,
		},
		{
			name: "returns 403 for non-member",
			path: "/api/v1/circles/" + contractCircleID,
			seed: func(store *circleStoreStub) {
				store.circles[contractCircleID] = rbac.Circle{ID: contractCircleID, Name: "Private Circle", CreatedAt: time.Now().UTC()}
			},
			withAuth:   true,
			wantStatus: http.StatusForbidden,
		},
		{
			name:       "returns 401 when unauthenticated",
			path:       "/api/v1/circles/" + contractCircleID,
			seed:       func(*circleStoreStub) {},
			withAuth:   false,
			wantStatus: http.StatusUnauthorized,
		},
		{
			name:       "returns 404 for missing circle",
			path:       "/api/v1/circles/99999999-9999-9999-9999-999999999999",
			seed:       func(*circleStoreStub) {},
			withAuth:   true,
			wantStatus: http.StatusNotFound,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			store := newCircleStoreStub()
			tc.seed(store)

			req := httptest.NewRequest(http.MethodGet, tc.path, nil)
			if tc.withAuth {
				req.Header.Set(httpconst.HeaderAuthorization, bearerValid)
				req.Header.Set(httpconst.HeaderSessionID, testSessionID)
			}
			rec := httptest.NewRecorder()

			buildGetCircleRoute(store).ServeHTTP(rec, req)

			if rec.Code != tc.wantStatus {
				t.Fatalf("status: got %d want %d body=%s", rec.Code, tc.wantStatus, rec.Body.String())
			}
		})
	}
}
