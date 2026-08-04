//go:build contract

package contract

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/KarimMFadel/halaqaty/backend/internal/auth"
	"github.com/KarimMFadel/halaqaty/backend/internal/middleware"
	phttp "github.com/KarimMFadel/halaqaty/backend/internal/platform/http"
	"github.com/KarimMFadel/halaqaty/backend/internal/platform/httpconst"
	"github.com/KarimMFadel/halaqaty/backend/internal/rbac"
)

func buildAssignRoleRoute(store *circleStoreStub) http.Handler {
	handler := rbac.NewHandler(rbac.NewService(store, nil))
	repo := &stubSessionRepo{sessionID: testSessionID, userID: testLocalUserID}
	authMW := middleware.NewAuthMiddleware(&alwaysOKVerifier{}, auth.NewSessionService(30*24*time.Hour), repo)
	roleMW := middleware.NewRoleMiddleware(store)
	return authMW.Require(roleMW.RequireAny("supervisor", "teacher")(http.HandlerFunc(handler.AssignRole)))
}

func seedCircle(store *circleStoreStub, members map[string]string) {
	store.circles[contractCircleID] = rbac.Circle{ID: contractCircleID, Name: "Circle", CreatedAt: time.Now().UTC()}
	store.members[contractCircleID] = members
}

func TestCircleAssignRoleContract(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name       string
		setup      func(*circleStoreStub)
		circleID   string
		targetID   string
		body       string
		authHeader string
		sessionID  string
		wantStatus int
		wantCode   string
		wantField  string
	}{
		{
			name: "teacher changes another member returns 200",
			setup: func(s *circleStoreStub) {
				s.actorRole = rbac.RoleTeacher
				seedCircle(s, map[string]string{testLocalUserID: rbac.RoleTeacher, contractStudentID: rbac.RoleStudent})
			},
			circleID:   contractCircleID,
			targetID:   contractStudentID,
			body:       `{"role":"teacher"}`,
			authHeader: bearerValid,
			sessionID:  testSessionID,
			wantStatus: http.StatusOK,
		},
		{
			name: "supervisor changes another member returns 200",
			setup: func(s *circleStoreStub) {
				s.actorRole = rbac.RoleSupervisor
				seedCircle(s, map[string]string{testLocalUserID: rbac.RoleSupervisor, contractStudentID: rbac.RoleStudent})
			},
			circleID:   contractCircleID,
			targetID:   contractStudentID,
			body:       `{"role":"supervisor"}`,
			authHeader: bearerValid,
			sessionID:  testSessionID,
			wantStatus: http.StatusOK,
		},
		{
			name: "student actor returns 403",
			setup: func(s *circleStoreStub) {
				s.actorRole = rbac.RoleStudent
				seedCircle(s, map[string]string{testLocalUserID: rbac.RoleStudent, contractTeacherAID: rbac.RoleTeacher})
			},
			circleID:   contractCircleID,
			targetID:   contractTeacherAID,
			body:       `{"role":"student"}`,
			authHeader: bearerValid,
			sessionID:  testSessionID,
			wantStatus: http.StatusForbidden,
			wantCode:   httpconst.ErrorCodeForbidden,
		},
		{
			name: "non-member actor returns 403",
			setup: func(s *circleStoreStub) {
				s.actorRoleErr = true
				seedCircle(s, map[string]string{contractTeacherAID: rbac.RoleTeacher})
			},
			circleID:   contractCircleID,
			targetID:   contractTeacherAID,
			body:       `{"role":"student"}`,
			authHeader: bearerValid,
			sessionID:  testSessionID,
			wantStatus: http.StatusForbidden,
			wantCode:   httpconst.ErrorCodeForbidden,
		},
		{
			name: "self change returns 403",
			setup: func(s *circleStoreStub) {
				s.actorRole = rbac.RoleTeacher
				seedCircle(s, map[string]string{testLocalUserID: rbac.RoleTeacher, contractTeacherAID: rbac.RoleTeacher})
			},
			circleID:   contractCircleID,
			targetID:   testLocalUserID,
			body:       `{"role":"student"}`,
			authHeader: bearerValid,
			sessionID:  testSessionID,
			wantStatus: http.StatusForbidden,
			wantCode:   httpconst.ErrorCodeForbidden,
		},
		{
			name: "final teacher removal returns 403",
			setup: func(s *circleStoreStub) {
				s.actorRole = rbac.RoleSupervisor
				seedCircle(s, map[string]string{testLocalUserID: rbac.RoleSupervisor, contractTeacherAID: rbac.RoleTeacher})
			},
			circleID:   contractCircleID,
			targetID:   contractTeacherAID,
			body:       `{"role":"student"}`,
			authHeader: bearerValid,
			sessionID:  testSessionID,
			wantStatus: http.StatusForbidden,
			wantCode:   httpconst.ErrorCodeForbidden,
		},
		{
			name: "target in another circle returns 404",
			setup: func(s *circleStoreStub) {
				s.actorRole = rbac.RoleTeacher
				seedCircle(s, map[string]string{testLocalUserID: rbac.RoleTeacher})
				s.members["99999999-9999-9999-9999-999999999999"] = map[string]string{contractStudentID: rbac.RoleStudent}
			},
			circleID:   contractCircleID,
			targetID:   contractStudentID,
			body:       `{"role":"teacher"}`,
			authHeader: bearerValid,
			sessionID:  testSessionID,
			wantStatus: http.StatusNotFound,
			wantCode:   httpconst.ErrorCodeNotFound,
		},
		{
			name: "invalid role returns 400",
			setup: func(s *circleStoreStub) {
				s.actorRole = rbac.RoleTeacher
				seedCircle(s, map[string]string{testLocalUserID: rbac.RoleTeacher, contractStudentID: rbac.RoleStudent})
			},
			circleID:   contractCircleID,
			targetID:   contractStudentID,
			body:       `{"role":"owner"}`,
			authHeader: bearerValid,
			sessionID:  testSessionID,
			wantStatus: http.StatusBadRequest,
			wantCode:   httpconst.ErrorCodeValidationFailed,
			wantField:  httpconst.FieldRole,
		},
		{
			name: "unknown circle returns 404",
			setup: func(s *circleStoreStub) {
				s.actorRole = rbac.RoleTeacher
			},
			circleID:   contractCircleID,
			targetID:   contractStudentID,
			body:       `{"role":"teacher"}`,
			authHeader: bearerValid,
			sessionID:  testSessionID,
			wantStatus: http.StatusNotFound,
			wantCode:   httpconst.ErrorCodeNotFound,
		},
		{
			name: "missing bearer returns 401",
			setup: func(s *circleStoreStub) {
				s.actorRole = rbac.RoleTeacher
			},
			circleID:   contractCircleID,
			targetID:   contractStudentID,
			body:       `{"role":"teacher"}`,
			sessionID:  testSessionID,
			wantStatus: http.StatusUnauthorized,
			wantCode:   httpconst.ErrorCodeUnauthorized,
		},
		{
			name: "missing session returns 401",
			setup: func(s *circleStoreStub) {
				s.actorRole = rbac.RoleTeacher
			},
			circleID:   contractCircleID,
			targetID:   contractStudentID,
			body:       `{"role":"teacher"}`,
			authHeader: bearerValid,
			wantStatus: http.StatusUnauthorized,
			wantCode:   httpconst.ErrorCodeSessionMissing,
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			store := newCircleStoreStub()
			tc.setup(store)
			route := buildAssignRoleRoute(store)

			req := httptest.NewRequest(http.MethodPut, "/circles/"+tc.circleID+"/members/"+tc.targetID+"/role", bytes.NewBufferString(tc.body))
			req.SetPathValue("circleId", tc.circleID)
			req.SetPathValue("userId", tc.targetID)
			req.Header.Set(httpconst.HeaderContentType, httpconst.ContentTypeApplicationJSON)
			if tc.authHeader != "" {
				req.Header.Set(httpconst.HeaderAuthorization, tc.authHeader)
			}
			if tc.sessionID != "" {
				req.Header.Set(httpconst.HeaderSessionID, tc.sessionID)
			}

			rec := httptest.NewRecorder()
			route.ServeHTTP(rec, req)

			if rec.Code != tc.wantStatus {
				t.Fatalf("status: got %d, want %d body=%s", rec.Code, tc.wantStatus, rec.Body.String())
			}

			if tc.wantCode != "" {
				var env phttp.ErrorEnvelope
				if err := json.NewDecoder(rec.Body).Decode(&env); err != nil {
					t.Fatalf("decode error envelope: %v", err)
				}
				if env.Error.Code != tc.wantCode {
					t.Fatalf("error code: got %q want %q", env.Error.Code, tc.wantCode)
				}
				if tc.wantField != "" {
					if _, ok := env.Error.Fields[tc.wantField]; !ok {
						t.Fatalf("expected field %q in Fields=%v", tc.wantField, env.Error.Fields)
					}
				}
				return
			}

			var assignment rbac.RoleAssignment
			if err := json.NewDecoder(rec.Body).Decode(&assignment); err != nil {
				t.Fatalf("decode assignment response: %v", err)
			}
			if assignment.CircleID != tc.circleID || assignment.UserID != tc.targetID || assignment.Role == "" {
				t.Fatalf("unexpected assignment: %+v", assignment)
			}
		})
	}
}
