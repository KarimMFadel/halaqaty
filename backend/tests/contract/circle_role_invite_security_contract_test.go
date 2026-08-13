//go:build contract

package contract

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/KarimMFadel/halaqaty/backend/internal/auth"
	"github.com/KarimMFadel/halaqaty/backend/internal/middleware"
	"github.com/KarimMFadel/halaqaty/backend/internal/platform/httpconst"
	"github.com/KarimMFadel/halaqaty/backend/internal/rbac"
)

func buildSecuredCircleMutationRoutes(store *circleStoreStub, verifier auth.TokenVerifier, rateLimit *middleware.RateLimitMiddleware) http.Handler {
	handler := rbac.NewHandler(rbac.NewService(store, nil))
	sessionRepo := &stubSessionRepo{sessionID: testSessionID, userID: testLocalUserID}
	authMW := middleware.NewAuthMiddleware(verifier, auth.NewSessionService(30*24*time.Hour), sessionRepo)
	roleMW := middleware.NewRoleMiddleware(store)

	wrap := func(next http.Handler) http.Handler {
		if rateLimit != nil {
			next = rateLimit.Limit(next)
		}
		return authMW.Require(next)
	}

	mux := http.NewServeMux()
	mux.Handle("PUT /circles/{circleId}/members/{userId}/role", wrap(roleMW.RequireAny(rbac.RoleSupervisor, rbac.RoleTeacher)(http.HandlerFunc(handler.AssignRole))))
	mux.Handle("DELETE /circles/{circleId}/members/{userId}", wrap(roleMW.RequireAny(rbac.RoleTeacher)(http.HandlerFunc(handler.RemoveMember))))
	mux.Handle("POST /circles/{circleId}/invite-code/refresh", wrap(roleMW.RequireAny(rbac.RoleTeacher)(http.HandlerFunc(handler.RefreshInviteCode))))
	return mux
}

func TestCircleRoleInviteSecurity_MutationsRequireCredentials(t *testing.T) {
	for _, tc := range []struct {
		name   string
		method string
		path   string
		body   string
	}{
		{"role assignment", http.MethodPut, "/circles/" + contractCircleID + "/members/" + contractStudentID + "/role", `{"role":"supervisor"}`},
		{"member removal", http.MethodDelete, "/circles/" + contractCircleID + "/members/" + contractStudentID, ""},
		{"invite refresh", http.MethodPost, "/circles/" + contractCircleID + "/invite-code/refresh", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(tc.method, tc.path, bytes.NewBufferString(tc.body))
			if tc.body != "" {
				req.Header.Set(httpconst.HeaderContentType, httpconst.ContentTypeApplicationJSON)
			}
			rec := httptest.NewRecorder()
			buildSecuredCircleMutationRoutes(newCircleStoreStub(), &alwaysOKVerifier{}, nil).ServeHTTP(rec, req)

			if rec.Code != http.StatusUnauthorized {
				t.Fatalf("status: got %d want %d body=%s", rec.Code, http.StatusUnauthorized, rec.Body.String())
			}
		})
	}
}

func TestCircleRoleInviteSecurity_RejectsUnauthorizedRoles(t *testing.T) {
	for _, tc := range []struct {
		name      string
		actorRole string
		method    string
		path      string
		body      string
	}{
		{"student cannot assign roles", rbac.RoleStudent, http.MethodPut, "/circles/" + contractCircleID + "/members/" + contractStudentID + "/role", `{"role":"supervisor"}`},
		{"supervisor cannot remove members", rbac.RoleSupervisor, http.MethodDelete, "/circles/" + contractCircleID + "/members/" + contractStudentID, ""},
		{"supervisor cannot refresh invites", rbac.RoleSupervisor, http.MethodPost, "/circles/" + contractCircleID + "/invite-code/refresh", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			store := newCircleStoreStub()
			store.actorRole = tc.actorRole
			req := httptest.NewRequest(tc.method, tc.path, bytes.NewBufferString(tc.body))
			req.Header.Set(httpconst.HeaderAuthorization, bearerValid)
			req.Header.Set(httpconst.HeaderSessionID, testSessionID)
			if tc.body != "" {
				req.Header.Set(httpconst.HeaderContentType, httpconst.ContentTypeApplicationJSON)
			}
			rec := httptest.NewRecorder()
			buildSecuredCircleMutationRoutes(store, &alwaysOKVerifier{}, nil).ServeHTTP(rec, req)

			if rec.Code != http.StatusForbidden {
				t.Fatalf("status: got %d want %d body=%s", rec.Code, http.StatusForbidden, rec.Body.String())
			}
		})
	}
}

func TestCircleRoleInviteSecurity_RateLimitsRoleAndInviteMutations(t *testing.T) {
	for _, tc := range []struct {
		name   string
		method string
		path   string
		body   string
	}{
		{"role assignment", http.MethodPut, "/circles/" + contractCircleID + "/members/" + contractStudentID + "/role", `{"role":"supervisor"}`},
		{"invite refresh", http.MethodPost, "/circles/" + contractCircleID + "/invite-code/refresh", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			store := newCircleStoreStub()
			store.actorRole = rbac.RoleTeacher
			store.circles[contractCircleID] = rbac.Circle{ID: contractCircleID, Name: "Circle", InviteCode: "HLQ-7X2K", CreatedAt: time.Now().UTC()}
			store.members[contractCircleID] = map[string]string{testLocalUserID: rbac.RoleTeacher, contractStudentID: rbac.RoleStudent}
			route := buildSecuredCircleMutationRoutes(store, &alwaysOKVerifier{}, middleware.NewRateLimitMiddleware(100, 1))

			for requestNumber, wantStatus := range []int{http.StatusOK, http.StatusTooManyRequests} {
				req := httptest.NewRequest(tc.method, tc.path, bytes.NewBufferString(tc.body))
				req.Header.Set(httpconst.HeaderAuthorization, bearerValid)
				req.Header.Set(httpconst.HeaderSessionID, testSessionID)
				if tc.body != "" {
					req.Header.Set(httpconst.HeaderContentType, httpconst.ContentTypeApplicationJSON)
				}
				rec := httptest.NewRecorder()
				route.ServeHTTP(rec, req)
				if rec.Code != wantStatus {
					t.Fatalf("request %d: got %d want %d body=%s", requestNumber+1, rec.Code, wantStatus, rec.Body.String())
				}
			}
		})
	}
}

func TestCircleRoleInviteSecurity_InviteRefreshResponseContainsOnlyShareableInviteFields(t *testing.T) {
	store := newCircleStoreStub()
	store.actorRole = rbac.RoleTeacher
	store.circles[contractCircleID] = rbac.Circle{ID: contractCircleID, Name: "Private Circle", InviteCode: "HLQ-7X2K", CreatedAt: time.Now().UTC()}
	store.members[contractCircleID] = map[string]string{testLocalUserID: rbac.RoleTeacher}

	req := httptest.NewRequest(http.MethodPost, "/circles/"+contractCircleID+"/invite-code/refresh", nil)
	req.Header.Set(httpconst.HeaderAuthorization, bearerValid)
	req.Header.Set(httpconst.HeaderSessionID, testSessionID)
	rec := httptest.NewRecorder()
	buildSecuredCircleMutationRoutes(store, &alwaysOKVerifier{}, nil).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status: got %d want %d body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}
	for _, forbidden := range []string{"circle_id", "user_id", "role", "member", "is_private", "is_archived"} {
		if bytes.Contains(bytes.ToLower(rec.Body.Bytes()), []byte(forbidden)) {
			t.Fatalf("invite refresh leaked %q: %s", forbidden, rec.Body.String())
		}
	}
}
