//go:build contract

package contract

import (
	"bytes"
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
	"github.com/KarimMFadel/halaqaty/backend/internal/rbac"
)

const (
	contractCircleID     = "44444444-4444-4444-4444-444444444444"
	contractTeacherAID   = "55555555-5555-5555-5555-555555555555"
	contractTeacherBID   = "66666666-6666-6666-6666-666666666666"
	contractSupervisorID = "77777777-7777-7777-7777-777777777777"
	contractStudentID    = "88888888-8888-8888-8888-888888888888"
)

// circleStoreStub is an in-memory rbac.Store for circle contract tests.
// actorRole/actorRoleErr feed the role middleware independently of the
// membership data the service sees, mirroring the defense-in-depth split.
type circleStoreStub struct {
	users        map[string]bool
	circles      map[string]rbac.Circle
	members      map[string]map[string]string
	actorRole    string
	actorRoleErr bool
}

func newCircleStoreStub() *circleStoreStub {
	return &circleStoreStub{
		users: map[string]bool{
			testLocalUserID:      true,
			contractTeacherAID:   true,
			contractTeacherBID:   true,
			contractSupervisorID: true,
			contractStudentID:    true,
		},
		circles: make(map[string]rbac.Circle),
		members: make(map[string]map[string]string),
	}
}

func (s *circleStoreStub) WithinTransaction(ctx context.Context, fn func(rbac.Store) error) error {
	return fn(s)
}

func (s *circleStoreStub) UsersExist(_ context.Context, userIDs []string) (map[string]bool, error) {
	existing := make(map[string]bool, len(userIDs))
	for _, id := range userIDs {
		existing[id] = s.users[id]
	}
	return existing, nil
}

func (s *circleStoreStub) InsertCircle(_ context.Context, name, ownerID, inviteCode string) (rbac.Circle, error) {
	circle := rbac.Circle{
		ID:         contractCircleID,
		Name:       name,
		TeacherID:  ownerID,
		InviteCode: inviteCode,
		CreatedAt:  time.Now().UTC(),
	}
	s.circles[circle.ID] = circle
	return circle, nil
}

func (s *circleStoreStub) InsertMember(_ context.Context, circleID, userID, role string) error {
	if s.members[circleID] == nil {
		s.members[circleID] = make(map[string]string)
	}
	if _, exists := s.members[circleID][userID]; !exists {
		s.members[circleID][userID] = role
	}
	return nil
}

func (s *circleStoreStub) CircleExists(_ context.Context, circleID string) (bool, error) {
	_, ok := s.circles[circleID]
	return ok, nil
}

func (s *circleStoreStub) LockMembers(_ context.Context, circleID string) ([]rbac.Member, error) {
	members := make([]rbac.Member, 0, len(s.members[circleID]))
	for userID, role := range s.members[circleID] {
		members = append(members, rbac.Member{UserID: userID, Role: role})
	}
	return members, nil
}

func (s *circleStoreStub) UpdateMemberRole(_ context.Context, circleID, userID, role string) error {
	s.members[circleID][userID] = role
	return nil
}

// RoleForUserInCircle satisfies middleware.CircleMembershipRepository.
func (s *circleStoreStub) RoleForUserInCircle(_ context.Context, _, _ string) (string, error) {
	if s.actorRoleErr {
		return "", auth.ErrCircleMembershipNotFound
	}
	return s.actorRole, nil
}

func buildCreateCircleRoute(store *circleStoreStub) http.Handler {
	handler := rbac.NewHandler(rbac.NewService(store, nil))
	repo := &stubSessionRepo{sessionID: testSessionID, userID: testLocalUserID}
	authMW := middleware.NewAuthMiddleware(&alwaysOKVerifier{}, auth.NewSessionService(30*24*time.Hour), repo)
	return authMW.Require(http.HandlerFunc(handler.CreateCircle))
}

func TestCircleCreateContract(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name       string
		body       string
		authHeader string
		sessionID  string
		wantStatus int
		wantCode   string
		wantField  string
	}{
		{
			name:       "missing bearer returns 401",
			body:       `{"name":"Quran Circle"}`,
			sessionID:  testSessionID,
			wantStatus: http.StatusUnauthorized,
			wantCode:   httpconst.ErrorCodeUnauthorized,
		},
		{
			name:       "missing session returns 401",
			body:       `{"name":"Quran Circle"}`,
			authHeader: bearerValid,
			wantStatus: http.StatusUnauthorized,
			wantCode:   httpconst.ErrorCodeSessionMissing,
		},
		{
			name:       "missing name returns 400",
			body:       `{}`,
			authHeader: bearerValid,
			sessionID:  testSessionID,
			wantStatus: http.StatusBadRequest,
			wantCode:   httpconst.ErrorCodeValidationFailed,
			wantField:  httpconst.FieldName,
		},
		{
			name:       "single rune name returns 400",
			body:       `{"name":"A"}`,
			authHeader: bearerValid,
			sessionID:  testSessionID,
			wantStatus: http.StatusBadRequest,
			wantCode:   httpconst.ErrorCodeValidationFailed,
			wantField:  httpconst.FieldName,
		},
		{
			name:       "duplicate teachers return 400",
			body:       `{"name":"Quran Circle","teacher_user_ids":["` + contractTeacherAID + `","` + contractTeacherAID + `"]}`,
			authHeader: bearerValid,
			sessionID:  testSessionID,
			wantStatus: http.StatusBadRequest,
			wantCode:   httpconst.ErrorCodeValidationFailed,
			wantField:  httpconst.FieldTeacherUserIDs,
		},
		{
			name:       "overlapping backup supervisor returns 400",
			body:       `{"name":"Quran Circle","teacher_user_ids":["` + contractTeacherAID + `"],"backup_supervisor_user_id":"` + contractTeacherAID + `"}`,
			authHeader: bearerValid,
			sessionID:  testSessionID,
			wantStatus: http.StatusBadRequest,
			wantCode:   httpconst.ErrorCodeValidationFailed,
			wantField:  httpconst.FieldBackupSupervisor,
		},
		{
			name:       "unknown teacher returns 400",
			body:       `{"name":"Quran Circle","teacher_user_ids":["99999999-9999-9999-9999-999999999999"]}`,
			authHeader: bearerValid,
			sessionID:  testSessionID,
			wantStatus: http.StatusBadRequest,
			wantCode:   httpconst.ErrorCodeValidationFailed,
			wantField:  httpconst.FieldTeacherUserIDs,
		},
		{
			name:       "multiple teachers and backup supervisor return 201",
			body:       `{"name":"Quran Circle","teacher_user_ids":["` + contractTeacherAID + `","` + contractTeacherBID + `"],"backup_supervisor_user_id":"` + contractSupervisorID + `"}`,
			authHeader: bearerValid,
			sessionID:  testSessionID,
			wantStatus: http.StatusCreated,
		},
		{
			name:       "no teachers returns 201 with creator fallback",
			body:       `{"name":"Solo Circle"}`,
			authHeader: bearerValid,
			sessionID:  testSessionID,
			wantStatus: http.StatusCreated,
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			store := newCircleStoreStub()
			route := buildCreateCircleRoute(store)

			req := httptest.NewRequest(http.MethodPost, "/circles", bytes.NewBufferString(tc.body))
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

			var resp rbac.CircleResponse
			if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
				t.Fatalf("decode circle response: %v", err)
			}
			if resp.ID == "" || resp.Name == "" || resp.InviteCode == "" || resp.CreatedAt.IsZero() {
				t.Fatalf("incomplete circle response: %+v", resp)
			}
		})
	}
}

func TestCircleCreateContract_CreatorRoleFallback(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name            string
		body            string
		wantCreatorRole string
	}{
		{
			name:            "no teachers selected assigns creator as teacher",
			body:            `{"name":"Solo Circle"}`,
			wantCreatorRole: rbac.RoleTeacher,
		},
		{
			name:            "teachers selected assigns creator as supervisor",
			body:            `{"name":"Group Circle","teacher_user_ids":["` + contractTeacherAID + `"]}`,
			wantCreatorRole: rbac.RoleSupervisor,
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			store := newCircleStoreStub()
			route := buildCreateCircleRoute(store)

			req := httptest.NewRequest(http.MethodPost, "/circles", bytes.NewBufferString(tc.body))
			req.Header.Set(httpconst.HeaderContentType, httpconst.ContentTypeApplicationJSON)
			req.Header.Set(httpconst.HeaderAuthorization, bearerValid)
			req.Header.Set(httpconst.HeaderSessionID, testSessionID)

			rec := httptest.NewRecorder()
			route.ServeHTTP(rec, req)

			if rec.Code != http.StatusCreated {
				t.Fatalf("status: got %d body=%s", rec.Code, rec.Body.String())
			}
			if got := store.members[contractCircleID][testLocalUserID]; got != tc.wantCreatorRole {
				t.Fatalf("creator role: got %q want %q", got, tc.wantCreatorRole)
			}
		})
	}
}
