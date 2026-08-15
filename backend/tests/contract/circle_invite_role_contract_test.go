//go:build contract

package contract

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/KarimMFadel/halaqaty/backend/internal/auth"
	"github.com/KarimMFadel/halaqaty/backend/internal/middleware"
	phttp "github.com/KarimMFadel/halaqaty/backend/internal/platform/http"
	"github.com/KarimMFadel/halaqaty/backend/internal/platform/httpconst"
	"github.com/KarimMFadel/halaqaty/backend/internal/platform/logging"
	"github.com/KarimMFadel/halaqaty/backend/internal/rbac"
)

type circleInviteAuditRecorder struct {
	events []logging.AuditEvent
}

func (r *circleInviteAuditRecorder) Log(_ context.Context, event logging.AuditEvent) {
	r.events = append(r.events, event)
}

func buildCircleInviteRoleRoute(store *circleStoreStub, audit rbac.AuditLogger) http.Handler {
	handler := rbac.NewHandler(rbac.NewService(store, audit))
	repo := &stubSessionRepo{sessionID: testSessionID, userID: testLocalUserID}
	authMW := middleware.NewAuthMiddleware(&alwaysOKVerifier{}, auth.NewSessionService(30*24*time.Hour), repo)
	mux := http.NewServeMux()
	mux.Handle("DELETE /circles/{circleId}/members/{userId}", authMW.Require(http.HandlerFunc(handler.RemoveMember)))
	mux.Handle("POST /circles/{circleId}/invite-code/refresh", authMW.Require(http.HandlerFunc(handler.RefreshInviteCode)))
	return mux
}

func TestCircleInviteRoleContract_RemoveMember(t *testing.T) {
	for _, tc := range []struct {
		name       string
		members    map[string]string
		targetID   string
		wantStatus int
		wantCode   string
	}{
		{
			name:     "teacher removes another member while retaining circle",
			members:  map[string]string{testLocalUserID: rbac.RoleTeacher, contractStudentID: rbac.RoleStudent},
			targetID: contractStudentID, wantStatus: http.StatusNoContent,
		},
		{
			name:     "supervisor cannot remove member",
			members:  map[string]string{testLocalUserID: rbac.RoleSupervisor, contractStudentID: rbac.RoleStudent},
			targetID: contractStudentID, wantStatus: http.StatusForbidden, wantCode: httpconst.ErrorCodeForbidden,
		},
		{
			name:     "student cannot remove member",
			members:  map[string]string{testLocalUserID: rbac.RoleStudent, contractTeacherAID: rbac.RoleTeacher},
			targetID: contractTeacherAID, wantStatus: http.StatusForbidden, wantCode: httpconst.ErrorCodeForbidden,
		},
		{
			name:     "non-member cannot remove member",
			members:  map[string]string{contractTeacherAID: rbac.RoleTeacher, contractStudentID: rbac.RoleStudent},
			targetID: contractStudentID, wantStatus: http.StatusForbidden, wantCode: httpconst.ErrorCodeForbidden,
		},
		{
			name:     "member cannot remove self",
			members:  map[string]string{testLocalUserID: rbac.RoleTeacher, contractStudentID: rbac.RoleStudent},
			targetID: testLocalUserID, wantStatus: http.StatusForbidden, wantCode: httpconst.ErrorCodeForbidden,
		},
		{
			name:     "cannot remove final teacher",
			members:  map[string]string{testLocalUserID: rbac.RoleSupervisor, contractTeacherAID: rbac.RoleTeacher},
			targetID: contractTeacherAID, wantStatus: http.StatusForbidden, wantCode: httpconst.ErrorCodeForbidden,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			store := newCircleStoreStub()
			audit := &circleInviteAuditRecorder{}
			store.circles[contractCircleID] = rbac.Circle{ID: contractCircleID, Name: "Circle", CreatedAt: time.Now().UTC()}
			store.members[contractCircleID] = tc.members

			req := httptest.NewRequest(http.MethodDelete, "/circles/"+contractCircleID+"/members/"+tc.targetID, nil)
			req.Header.Set(httpconst.HeaderAuthorization, bearerValid)
			req.Header.Set(httpconst.HeaderSessionID, testSessionID)
			rec := httptest.NewRecorder()
			buildCircleInviteRoleRoute(store, audit).ServeHTTP(rec, req)

			if rec.Code != tc.wantStatus {
				t.Fatalf("status: got %d, want %d body=%s", rec.Code, tc.wantStatus, rec.Body.String())
			}
			if tc.wantStatus != http.StatusNoContent {
				if tc.wantCode != "" {
					assertCircleInviteRoleErrorCode(t, rec, tc.wantCode)
				}
				return
			}
			if _, exists := store.circles[contractCircleID]; !exists {
				t.Fatal("member removal must retain the circle and its history")
			}
			if _, exists := store.members[contractCircleID][tc.targetID]; exists {
				t.Fatalf("removed member %q still has active membership", tc.targetID)
			}
			if len(audit.events) != 1 ||
				audit.events[0].Action != logging.ActionMemberRemoval ||
				audit.events[0].TargetUser != tc.targetID {
				t.Fatalf("retained removal history: got %+v", audit.events)
			}
		})
	}
}

func TestCircleInviteRoleContract_RefreshInvite(t *testing.T) {
	for _, tc := range []struct {
		name       string
		circle     rbac.Circle
		members    map[string]string
		wantStatus int
		wantCode   string
	}{
		{
			name:    "teacher refreshes invite and invalidates old code",
			circle:  rbac.Circle{ID: contractCircleID, Name: "Circle", InviteCode: "HLQ-7X2K", CreatedAt: time.Now().UTC()},
			members: map[string]string{testLocalUserID: rbac.RoleTeacher}, wantStatus: http.StatusOK,
		},
		{
			name:    "student cannot refresh invite",
			circle:  rbac.Circle{ID: contractCircleID, Name: "Circle", InviteCode: "HLQ-7X2K", CreatedAt: time.Now().UTC()},
			members: map[string]string{testLocalUserID: rbac.RoleStudent}, wantStatus: http.StatusForbidden, wantCode: httpconst.ErrorCodeForbidden,
		},
		{
			name:    "non-member cannot refresh invite",
			circle:  rbac.Circle{ID: contractCircleID, Name: "Circle", InviteCode: "HLQ-7X2K", CreatedAt: time.Now().UTC()},
			members: map[string]string{contractTeacherAID: rbac.RoleTeacher}, wantStatus: http.StatusForbidden, wantCode: httpconst.ErrorCodeForbidden,
		},
		{
			name:    "archived circle rejects invite refresh",
			circle:  rbac.Circle{ID: contractCircleID, Name: "Circle", InviteCode: "HLQ-7X2K", IsArchived: true, CreatedAt: time.Now().UTC()},
			members: map[string]string{testLocalUserID: rbac.RoleTeacher}, wantStatus: http.StatusConflict,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			store := newCircleStoreStub()
			store.circles[contractCircleID] = tc.circle
			store.members[contractCircleID] = tc.members

			req := httptest.NewRequest(http.MethodPost, "/circles/"+contractCircleID+"/invite-code/refresh", nil)
			req.Header.Set(httpconst.HeaderAuthorization, bearerValid)
			req.Header.Set(httpconst.HeaderSessionID, testSessionID)
			rec := httptest.NewRecorder()
			buildCircleInviteRoleRoute(store, nil).ServeHTTP(rec, req)

			if rec.Code != tc.wantStatus {
				t.Fatalf("status: got %d, want %d body=%s", rec.Code, tc.wantStatus, rec.Body.String())
			}
			if tc.wantStatus != http.StatusOK {
				if tc.wantCode != "" {
					assertCircleInviteRoleErrorCode(t, rec, tc.wantCode)
				}
				return
			}

			var response struct {
				InviteCode string `json:"invite_code"`
				InviteLink string `json:"invite_link"`
			}
			if err := json.NewDecoder(rec.Body).Decode(&response); err != nil {
				t.Fatalf("decode invite response: %v", err)
			}
			if response.InviteCode == "" || response.InviteCode == "HLQ-7X2K" {
				t.Fatalf("invite_code: got %q, want a new code", response.InviteCode)
			}
			if response.InviteLink != "https://halaqaty.app/join/"+response.InviteCode {
				t.Fatalf("invite_link: got %q", response.InviteLink)
			}
			if _, err := store.FindCircleByInviteCode(context.Background(), "HLQ-7X2K"); err == nil {
				t.Fatal("old invite code remains valid after refresh")
			}
		})
	}
}

func assertCircleInviteRoleErrorCode(t *testing.T, rec *httptest.ResponseRecorder, wantCode string) {
	t.Helper()
	var env phttp.ErrorEnvelope
	if err := json.NewDecoder(rec.Body).Decode(&env); err != nil {
		t.Fatalf("decode error envelope: %v", err)
	}
	if env.Error.Code != wantCode {
		t.Fatalf("error code: got %q, want %q", env.Error.Code, wantCode)
	}
}
