package rbac

import (
	"context"
	"errors"
	"strings"
	"testing"
)

const (
	unitCreatorID    = "11111111-1111-1111-1111-111111111111"
	unitTeacherAID   = "22222222-2222-2222-2222-222222222222"
	unitTeacherBID   = "33333333-3333-3333-3333-333333333333"
	unitSupervisorID = "44444444-4444-4444-4444-444444444444"
	unitStudentID    = "55555555-5555-5555-5555-555555555555"
	unitCircleID     = "66666666-6666-6666-6666-666666666666"
)

// stubStore is an in-memory rbac.Store for service unit tests.
type stubStore struct {
	users   map[string]bool
	circles map[string]Circle
	members map[string]map[string]string
	nextID  int
}

func newStubStore() *stubStore {
	return &stubStore{
		users: map[string]bool{
			unitCreatorID:    true,
			unitTeacherAID:   true,
			unitTeacherBID:   true,
			unitSupervisorID: true,
			unitStudentID:    true,
		},
		circles: make(map[string]Circle),
		members: make(map[string]map[string]string),
	}
}

func (s *stubStore) WithinTransaction(ctx context.Context, fn func(Store) error) error {
	return fn(s)
}

func (s *stubStore) UsersExist(_ context.Context, userIDs []string) (map[string]bool, error) {
	existing := make(map[string]bool, len(userIDs))
	for _, id := range userIDs {
		existing[id] = s.users[id]
	}
	return existing, nil
}

func (s *stubStore) InsertCircle(_ context.Context, name, ownerID, inviteCode string) (Circle, error) {
	s.nextID++
	circle := Circle{ID: unitCircleID, Name: name, TeacherID: ownerID, InviteCode: inviteCode}
	s.circles[circle.ID] = circle
	return circle, nil
}

func (s *stubStore) InsertMember(_ context.Context, circleID, userID, role string) error {
	if s.members[circleID] == nil {
		s.members[circleID] = make(map[string]string)
	}
	// Models ON CONFLICT (circle_id, user_id) DO NOTHING.
	if _, exists := s.members[circleID][userID]; !exists {
		s.members[circleID][userID] = role
	}
	return nil
}

func (s *stubStore) CircleExists(_ context.Context, circleID string) (bool, error) {
	_, ok := s.circles[circleID]
	return ok, nil
}

func (s *stubStore) LockMembers(_ context.Context, circleID string) ([]Member, error) {
	members := make([]Member, 0, len(s.members[circleID]))
	for userID, role := range s.members[circleID] {
		members = append(members, Member{UserID: userID, Role: role})
	}
	return members, nil
}

func (s *stubStore) UpdateMemberRole(_ context.Context, circleID, userID, role string) error {
	s.members[circleID][userID] = role
	return nil
}

func TestCreateCircleValidation(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name       string
		req        CreateCircleRequest
		wantFields []string
	}{
		{
			name:       "blank name rejected",
			req:        CreateCircleRequest{Name: "  "},
			wantFields: []string{"name"},
		},
		{
			name:       "single rune name rejected",
			req:        CreateCircleRequest{Name: "A"},
			wantFields: []string{"name"},
		},
		{
			name:       "duplicate teachers rejected",
			req:        CreateCircleRequest{Name: "Quran Circle", TeacherUserIDs: []string{unitTeacherAID, unitTeacherAID}},
			wantFields: []string{"teacher_user_ids"},
		},
		{
			name:       "creator in teacher list rejected",
			req:        CreateCircleRequest{Name: "Quran Circle", TeacherUserIDs: []string{unitCreatorID}},
			wantFields: []string{"teacher_user_ids"},
		},
		{
			name: "supervisor overlapping teacher rejected",
			req: CreateCircleRequest{
				Name:                   "Quran Circle",
				TeacherUserIDs:         []string{unitTeacherAID},
				BackupSupervisorUserID: strPtr(unitTeacherAID),
			},
			wantFields: []string{"backup_supervisor_user_id"},
		},
		{
			name: "creator as backup supervisor rejected",
			req: CreateCircleRequest{
				Name:                   "Quran Circle",
				TeacherUserIDs:         []string{unitTeacherAID},
				BackupSupervisorUserID: strPtr(unitCreatorID),
			},
			wantFields: []string{"backup_supervisor_user_id"},
		},
		{
			name:       "malformed teacher id rejected",
			req:        CreateCircleRequest{Name: "Quran Circle", TeacherUserIDs: []string{"not-a-uuid"}},
			wantFields: []string{"teacher_user_ids"},
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			svc := NewService(newStubStore(), nil)
			_, err := svc.CreateCircle(context.Background(), unitCreatorID, tc.req)
			var validationErr *ValidationError
			if !errors.As(err, &validationErr) {
				t.Fatalf("expected ValidationError, got %v", err)
			}
			for _, field := range tc.wantFields {
				if _, ok := validationErr.Fields[field]; !ok {
					t.Fatalf("expected field %q in Fields=%v", field, validationErr.Fields)
				}
			}
		})
	}
}

func TestCreateCircle_UnknownAssignee_ReturnsFieldError(t *testing.T) {
	t.Parallel()

	svc := NewService(newStubStore(), nil)
	unknownID := "99999999-9999-9999-9999-999999999999"
	_, err := svc.CreateCircle(context.Background(), unitCreatorID, CreateCircleRequest{
		Name:           "Quran Circle",
		TeacherUserIDs: []string{unknownID},
	})
	var validationErr *ValidationError
	if !errors.As(err, &validationErr) {
		t.Fatalf("expected ValidationError, got %v", err)
	}
	if _, ok := validationErr.Fields["teacher_user_ids"]; !ok {
		t.Fatalf("expected teacher_user_ids field error, got %v", validationErr.Fields)
	}
}

func TestCreateCircle_MembershipAssignment(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name             string
		req              CreateCircleRequest
		wantCreatorRole  string
		wantTeacherCount int
	}{
		{
			name:             "no teachers selected promotes creator to teacher",
			req:              CreateCircleRequest{Name: "Solo Circle"},
			wantCreatorRole:  RoleTeacher,
			wantTeacherCount: 1,
		},
		{
			name: "teachers selected makes creator supervisor",
			req: CreateCircleRequest{
				Name:                   "Group Circle",
				TeacherUserIDs:         []string{unitTeacherAID, unitTeacherBID},
				BackupSupervisorUserID: strPtr(unitSupervisorID),
			},
			wantCreatorRole:  RoleSupervisor,
			wantTeacherCount: 2,
		},
		{
			name: "backup supervisor id is normalized before persistence",
			req: CreateCircleRequest{
				Name:                   "Trimmed Circle",
				TeacherUserIDs:         []string{unitTeacherAID},
				BackupSupervisorUserID: strPtr(" " + unitSupervisorID + " "),
			},
			wantCreatorRole:  RoleSupervisor,
			wantTeacherCount: 1,
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			store := newStubStore()
			svc := NewService(store, nil)

			resp, err := svc.CreateCircle(context.Background(), unitCreatorID, tc.req)
			if err != nil {
				t.Fatalf("CreateCircle: %v", err)
			}
			if resp.ID == "" || resp.InviteCode == "" || resp.Name != tc.req.Name {
				t.Fatalf("unexpected response: %+v", resp)
			}

			members := store.members[resp.ID]
			if got := members[unitCreatorID]; got != tc.wantCreatorRole {
				t.Fatalf("creator role: got %q want %q", got, tc.wantCreatorRole)
			}
			teachers := 0
			for _, role := range members {
				if role == RoleTeacher {
					teachers++
				}
			}
			if teachers != tc.wantTeacherCount {
				t.Fatalf("teacher count: got %d want %d (members=%v)", teachers, tc.wantTeacherCount, members)
			}
			for _, id := range tc.req.TeacherUserIDs {
				if members[id] != RoleTeacher {
					t.Fatalf("assignee %s role: got %q want teacher", id, members[id])
				}
			}
			if tc.req.BackupSupervisorUserID != nil {
				backupID := strings.TrimSpace(*tc.req.BackupSupervisorUserID)
				if members[backupID] != RoleSupervisor {
					t.Fatalf("backup supervisor role: got %q", members[backupID])
				}
			}
		})
	}
}

func assignRoleSetup() (*stubStore, *Service) {
	store := newStubStore()
	store.circles[unitCircleID] = Circle{ID: unitCircleID, Name: "Circle"}
	store.members[unitCircleID] = map[string]string{
		unitCreatorID:    RoleSupervisor,
		unitTeacherAID:   RoleTeacher,
		unitTeacherBID:   RoleTeacher,
		unitSupervisorID: RoleSupervisor,
		unitStudentID:    RoleStudent,
	}
	return store, NewService(store, nil)
}

func TestAssignRoleValidation(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name      string
		actorID   string
		circleID  string
		targetID  string
		role      string
		wantError error
	}{
		{
			name:      "self change rejected",
			actorID:   unitTeacherAID,
			circleID:  unitCircleID,
			targetID:  unitTeacherAID,
			role:      RoleStudent,
			wantError: ErrSelfRoleChange,
		},
		{
			name:      "unknown circle rejected",
			actorID:   unitTeacherAID,
			circleID:  "77777777-7777-7777-7777-777777777777",
			targetID:  unitStudentID,
			role:      RoleTeacher,
			wantError: ErrCircleNotFound,
		},
		{
			name:      "target outside circle rejected",
			actorID:   unitTeacherAID,
			circleID:  unitCircleID,
			targetID:  "99999999-9999-9999-9999-999999999999",
			role:      RoleStudent,
			wantError: ErrMemberNotFound,
		},
		{
			name:      "student actor rejected",
			actorID:   unitStudentID,
			circleID:  unitCircleID,
			targetID:  unitTeacherBID,
			role:      RoleStudent,
			wantError: ErrForbidden,
		},
		{
			name:      "malformed circle id treated as unknown circle",
			actorID:   unitTeacherAID,
			circleID:  "not-a-uuid",
			targetID:  unitStudentID,
			role:      RoleTeacher,
			wantError: ErrCircleNotFound,
		},
		{
			name:      "malformed target id treated as non-member",
			actorID:   unitTeacherAID,
			circleID:  unitCircleID,
			targetID:  "not-a-uuid",
			role:      RoleTeacher,
			wantError: ErrMemberNotFound,
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, svc := assignRoleSetup()
			_, err := svc.AssignRole(context.Background(), tc.actorID, tc.circleID, tc.targetID, tc.role)
			if !errors.Is(err, tc.wantError) {
				t.Fatalf("error: got %v want %v", err, tc.wantError)
			}
		})
	}
}

func TestAssignRole_InvalidRole_ReturnsValidationError(t *testing.T) {
	t.Parallel()

	_, svc := assignRoleSetup()
	_, err := svc.AssignRole(context.Background(), unitTeacherAID, unitCircleID, unitStudentID, "owner")
	var validationErr *ValidationError
	if !errors.As(err, &validationErr) {
		t.Fatalf("expected ValidationError, got %v", err)
	}
	if _, ok := validationErr.Fields["role"]; !ok {
		t.Fatalf("expected role field error, got %v", validationErr.Fields)
	}
}

func TestAssignRole_FinalTeacherProtection(t *testing.T) {
	t.Parallel()

	store := newStubStore()
	store.circles[unitCircleID] = Circle{ID: unitCircleID, Name: "Circle"}
	store.members[unitCircleID] = map[string]string{
		unitCreatorID:  RoleSupervisor,
		unitTeacherAID: RoleTeacher,
		unitStudentID:  RoleStudent,
	}
	svc := NewService(store, nil)

	_, err := svc.AssignRole(context.Background(), unitCreatorID, unitCircleID, unitTeacherAID, RoleStudent)
	if !errors.Is(err, ErrFinalTeacher) {
		t.Fatalf("error: got %v want %v", err, ErrFinalTeacher)
	}
	if store.members[unitCircleID][unitTeacherAID] != RoleTeacher {
		t.Fatal("final teacher role must remain unchanged after rejection")
	}
}

func TestAssignRole_ManagerUpdatesMember(t *testing.T) {
	t.Parallel()

	store, svc := assignRoleSetup()
	assignment, err := svc.AssignRole(context.Background(), unitSupervisorID, unitCircleID, unitStudentID, RoleTeacher)
	if err != nil {
		t.Fatalf("AssignRole: %v", err)
	}
	if assignment.Role != RoleTeacher || assignment.UserID != unitStudentID || assignment.CircleID != unitCircleID {
		t.Fatalf("unexpected assignment: %+v", assignment)
	}
	if store.members[unitCircleID][unitStudentID] != RoleTeacher {
		t.Fatalf("target role not updated: %v", store.members[unitCircleID])
	}
}

func TestAddStudentMember_Idempotent(t *testing.T) {
	t.Parallel()

	store, svc := assignRoleSetup()
	ctx := context.Background()
	if err := svc.AddStudentMember(ctx, unitCircleID, unitStudentID); err != nil {
		t.Fatalf("first add: %v", err)
	}
	if err := svc.AddStudentMember(ctx, unitCircleID, unitStudentID); err != nil {
		t.Fatalf("idempotent replay: %v", err)
	}

	// An existing non-student membership must survive a student re-add.
	store.members[unitCircleID][unitStudentID] = RoleTeacher
	if err := svc.AddStudentMember(ctx, unitCircleID, unitStudentID); err != nil {
		t.Fatalf("replay over existing membership: %v", err)
	}
	if store.members[unitCircleID][unitStudentID] != RoleTeacher {
		t.Fatalf("existing role overwritten: got %q", store.members[unitCircleID][unitStudentID])
	}
}

func TestAddStudentMember_UnknownCircle_ReturnsNotFound(t *testing.T) {
	t.Parallel()

	_, svc := assignRoleSetup()
	err := svc.AddStudentMember(context.Background(), "99999999-9999-9999-9999-999999999999", unitStudentID)
	if !errors.Is(err, ErrCircleNotFound) {
		t.Fatalf("error: got %v want %v", err, ErrCircleNotFound)
	}
}

func strPtr(value string) *string {
	return &value
}
