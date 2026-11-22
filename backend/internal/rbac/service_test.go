package rbac

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgconn"

	"github.com/KarimMFadel/halaqaty/backend/internal/platform/logging"
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
	users            map[string]bool
	circles          map[string]Circle
	members          map[string]map[string]string
	findCircleErr    error
	isMemberErr      error
	listMembersErr   error
	nextID           int
	insertErrors     []error
	searchQuery      string
	searchLimit      int
	invitations      map[string]CircleInvitation
	nextInvitationID int
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
		circles:     make(map[string]Circle),
		members:     make(map[string]map[string]string),
		invitations: make(map[string]CircleInvitation),
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

func (s *stubStore) LockUser(context.Context, string) error { return nil }

func (s *stubStore) InsertCircle(_ context.Context, name, ownerID, inviteCode string, settings CircleSettings) (Circle, error) {
	if len(s.insertErrors) > 0 {
		err := s.insertErrors[0]
		s.insertErrors = s.insertErrors[1:]
		return Circle{}, err
	}
	s.nextID++
	circle := Circle{ID: unitCircleID, Name: name, TeacherID: ownerID, InviteCode: inviteCode, Description: settings.Description, Rules: settings.Rules, MaxCapacity: settings.MaxCapacity, IsPrivate: settings.IsPrivate, GenderRestriction: settings.GenderRestriction, Language: settings.Language, GradingPolicy: settings.GradingPolicy}
	s.circles[circle.ID] = circle
	return circle, nil
}

func TestCreateCircle_RetriesUniqueInviteCollision(t *testing.T) {
	store := newStubStore()
	store.insertErrors = []error{&pgconn.PgError{Code: "23505", ConstraintName: "circles_invite_code_key"}}
	svc := NewService(store, nil)
	response, err := svc.CreateCircle(context.Background(), unitCreatorID, CreateCircleRequest{Name: "Retry Circle"})
	if err != nil {
		t.Fatalf("CreateCircle: %v", err)
	}
	if response.InviteCode == "" || len(store.circles) != 1 {
		t.Fatalf("retry did not create circle: response=%+v circles=%d", response, len(store.circles))
	}
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

func (s *stubStore) CountActiveMemberships(_ context.Context, userID string) (int, error) {
	count := 0
	for circleID, members := range s.members {
		if _, ok := members[userID]; ok && !s.circles[circleID].IsArchived {
			count++
		}
	}
	return count, nil
}

func (s *stubStore) UpdateMemberRole(_ context.Context, circleID, userID, role string) error {
	s.members[circleID][userID] = role
	return nil
}

func (s *stubStore) SetPrimaryTeacher(_ context.Context, circleID, userID string) error {
	circle, ok := s.circles[circleID]
	if !ok {
		return ErrCircleNotFound
	}
	circle.TeacherID = userID
	s.circles[circleID] = circle
	return nil
}

func (s *stubStore) FindCircleByInviteCode(_ context.Context, inviteCode string) (Circle, error) {
	for _, circle := range s.circles {
		if circle.InviteCode == inviteCode {
			return circle, nil
		}
	}
	return Circle{}, ErrCircleNotFound
}

func (s *stubStore) FindCircleByID(_ context.Context, circleID string) (Circle, error) {
	if s.findCircleErr != nil {
		return Circle{}, s.findCircleErr
	}
	circle, ok := s.circles[circleID]
	if !ok {
		return Circle{}, ErrCircleNotFound
	}
	return circle, nil
}

func (s *stubStore) FindCircleByIDForUpdate(ctx context.Context, circleID string) (Circle, error) {
	return s.FindCircleByID(ctx, circleID)
}

func (s *stubStore) ListPublicCircles(_ context.Context, _, _ string, _ int) ([]PublicCircleSummary, error) {
	return nil, nil
}

func (s *stubStore) ListUserCircles(_ context.Context, _, _ string) ([]PublicCircleSummary, error) {
	return nil, nil
}

func (s *stubStore) SearchUsers(_ context.Context, query string, limit int) ([]UserSearchResult, error) {
	s.searchQuery = query
	s.searchLimit = limit
	return nil, nil
}

func (s *stubStore) ReplacePendingInvitation(_ context.Context, circleID, inviterID, recipientID, role string) (CircleInvitation, error) {
	now := time.Now().UTC()
	for id, invitation := range s.invitations {
		if invitation.CircleID == circleID && invitation.RecipientUserID == recipientID && invitation.State == "pending" {
			invitation.State = "superseded"
			invitation.ResolvedAt = &now
			s.invitations[id] = invitation
		}
	}
	s.nextInvitationID++
	invitation := CircleInvitation{
		ID:       fmt.Sprintf("%08x-1111-4111-8111-111111111111", s.nextInvitationID),
		CircleID: circleID, InviterUserID: inviterID, RecipientUserID: recipientID,
		Role: role, State: "pending", CreatedAt: now,
	}
	s.invitations[invitation.ID] = invitation
	return invitation, nil
}

func (s *stubStore) ListPendingCircleInvitations(_ context.Context, circleID string) ([]CircleInvitation, error) {
	var invitations []CircleInvitation
	for _, invitation := range s.invitations {
		if invitation.CircleID == circleID && invitation.State == "pending" {
			invitations = append(invitations, invitation)
		}
	}
	return invitations, nil
}

func (s *stubStore) ListPendingUserInvitations(_ context.Context, userID string) ([]CircleInvitation, error) {
	var invitations []CircleInvitation
	for _, invitation := range s.invitations {
		if invitation.RecipientUserID == userID && invitation.State == "pending" && !s.circles[invitation.CircleID].IsArchived {
			invitations = append(invitations, invitation)
		}
	}
	return invitations, nil
}

func (s *stubStore) FindInvitation(_ context.Context, invitationID string) (CircleInvitation, error) {
	invitation, ok := s.invitations[invitationID]
	if !ok {
		return CircleInvitation{}, ErrInvitationNotFound
	}
	return invitation, nil
}

func (s *stubStore) LockInvitation(_ context.Context, circleID, invitationID string) (CircleInvitation, error) {
	invitation, ok := s.invitations[invitationID]
	if !ok || invitation.CircleID != circleID {
		return CircleInvitation{}, ErrInvitationNotFound
	}
	return invitation, nil
}

func (s *stubStore) SetInvitationState(_ context.Context, invitationID, state string) error {
	invitation, ok := s.invitations[invitationID]
	if !ok || invitation.State != "pending" {
		return ErrInvitationConflict
	}
	now := time.Now().UTC()
	invitation.State = state
	invitation.ResolvedAt = &now
	s.invitations[invitationID] = invitation
	return nil
}

func TestSearchUsers_EscapesLikeWildcards(t *testing.T) {
	store := newStubStore()
	users, err := NewService(store, nil).SearchUsers(context.Background(), `a%_\b`)
	if err != nil {
		t.Fatalf("SearchUsers: %v", err)
	}
	if users == nil {
		t.Fatal("empty search results must encode as an empty array")
	}
	if got, want := store.searchQuery, `a\%\_\\b`; got != want {
		t.Fatalf("search query: got %q want %q", got, want)
	}
	if store.searchLimit != userSearchLimit {
		t.Fatalf("search limit: got %d want %d", store.searchLimit, userSearchLimit)
	}
}
func (s *stubStore) UpdateCircle(_ context.Context, circleID, name string, settings CircleSettings) (Circle, error) {
	circle, ok := s.circles[circleID]
	if !ok {
		return Circle{}, ErrCircleNotFound
	}
	circle.Name, circle.Description, circle.Rules, circle.MaxCapacity = name, settings.Description, settings.Rules, settings.MaxCapacity
	circle.IsPrivate, circle.GenderRestriction, circle.Language, circle.GradingPolicy = settings.IsPrivate, settings.GenderRestriction, settings.Language, settings.GradingPolicy
	s.circles[circleID] = circle
	return circle, nil
}
func (s *stubStore) RefreshInviteCode(_ context.Context, circleID, inviteCode string) error {
	circle, ok := s.circles[circleID]
	if !ok {
		return ErrCircleNotFound
	}
	circle.InviteCode = inviteCode
	s.circles[circleID] = circle
	return nil
}
func (s *stubStore) RemoveMember(_ context.Context, circleID, userID string) error {
	delete(s.members[circleID], userID)
	return nil
}
func (s *stubStore) ArchiveCircle(_ context.Context, circleID string) error {
	circle, ok := s.circles[circleID]
	if !ok {
		return ErrCircleNotFound
	}
	circle.IsArchived = true
	s.circles[circleID] = circle
	return nil
}
func (s *stubStore) ListMembers(_ context.Context, circleID string) ([]CircleMember, error) {
	if s.listMembersErr != nil {
		return nil, s.listMembersErr
	}
	members := s.members[circleID]
	result := make([]CircleMember, 0, len(members))
	for id, role := range members {
		result = append(result, CircleMember{UserID: id, DisplayName: "Stub User", Role: role, JoinedAt: time.Now()})
	}
	return result, nil
}

func (s *stubStore) IsMember(_ context.Context, circleID, userID string) (bool, error) {
	if s.isMemberErr != nil {
		return false, s.isMemberErr
	}
	_, ok := s.members[circleID][userID]
	return ok, nil
}

func TestCircleReads_WrapStoreErrorsWithOperation(t *testing.T) {
	storeErr := errors.New("store unavailable")
	tests := []struct {
		name    string
		prepare func(*stubStore)
		read    func(*Service) error
		want    string
	}{
		{
			name:    "get circle lookup",
			prepare: func(store *stubStore) { store.findCircleErr = storeErr },
			read: func(service *Service) error {
				_, err := service.GetCircle(context.Background(), unitStudentID, unitCircleID)
				return err
			},
			want: "get circle: find circle",
		},
		{
			name:    "get circle membership",
			prepare: func(store *stubStore) { store.isMemberErr = storeErr },
			read: func(service *Service) error {
				_, err := service.GetCircle(context.Background(), unitStudentID, unitCircleID)
				return err
			},
			want: "get circle: check membership",
		},
		{
			name:    "list members circle lookup",
			prepare: func(store *stubStore) { store.findCircleErr = storeErr },
			read: func(service *Service) error {
				_, err := service.ListMembers(context.Background(), unitStudentID, unitCircleID)
				return err
			},
			want: "list members: find circle",
		},
		{
			name:    "list members membership",
			prepare: func(store *stubStore) { store.isMemberErr = storeErr },
			read: func(service *Service) error {
				_, err := service.ListMembers(context.Background(), unitStudentID, unitCircleID)
				return err
			},
			want: "list members: check membership",
		},
		{
			name:    "list members query",
			prepare: func(store *stubStore) { store.listMembersErr = storeErr },
			read: func(service *Service) error {
				_, err := service.ListMembers(context.Background(), unitStudentID, unitCircleID)
				return err
			},
			want: "list members: query members",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			store := newStubStore()
			store.circles[unitCircleID] = Circle{ID: unitCircleID}
			store.members[unitCircleID] = map[string]string{unitStudentID: RoleStudent}
			test.prepare(store)

			err := test.read(NewService(store, nil))
			if !errors.Is(err, storeErr) || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("error: got %v, want wrapped %q preserving cause", err, test.want)
			}
		})
	}
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
			name:       "teacher assignment requires acceptance",
			req:        CreateCircleRequest{Name: "Quran Circle", TeacherUserIDs: []string{unitTeacherAID}},
			wantFields: []string{"teacher_user_ids"},
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
			name:       "malformed teacher id cannot bypass consent",
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

func TestCreateCircle_RejectsDirectTeacherAssignmentWithoutCreatingCircle(t *testing.T) {
	t.Parallel()

	store := newStubStore()
	svc := NewService(store, nil)
	_, err := svc.CreateCircle(context.Background(), unitCreatorID, CreateCircleRequest{
		Name:           "Quran Circle",
		TeacherUserIDs: []string{unitTeacherAID},
	})
	var validationErr *ValidationError
	if !errors.As(err, &validationErr) {
		t.Fatalf("expected ValidationError, got %v", err)
	}
	if _, ok := validationErr.Fields["teacher_user_ids"]; !ok {
		t.Fatalf("expected teacher_user_ids field error, got %v", validationErr.Fields)
	}
	if len(store.circles) != 0 {
		t.Fatal("circle must not be created before the selected teacher accepts")
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
			name: "backup supervisor id is normalized before persistence",
			req: CreateCircleRequest{
				Name:                   "Trimmed Circle",
				BackupSupervisorUserID: strPtr(" " + unitSupervisorID + " "),
			},
			wantCreatorRole:  RoleTeacher,
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
			if tc.req.BackupSupervisorUserID != nil {
				backupID := strings.TrimSpace(*tc.req.BackupSupervisorUserID)
				if members[backupID] != RoleSupervisor {
					t.Fatalf("backup supervisor role: got %q", members[backupID])
				}
			}
		})
	}
}

func TestJoinCircle_CapacityCountsStudentsOnly(t *testing.T) {
	store := newStubStore()
	store.circles[unitCircleID] = Circle{ID: unitCircleID, Name: "Circle", InviteCode: "HLQ-7X2K", MaxCapacity: 2}
	store.members[unitCircleID] = map[string]string{
		unitCreatorID:    RoleTeacher,
		unitSupervisorID: RoleSupervisor,
	}

	_, err := NewService(store, nil).JoinCircle(context.Background(), unitStudentID, "HLQ-7X2K")
	if err != nil {
		t.Fatalf("JoinCircle: %v", err)
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
			name:      "teacher promotion requires recipient acceptance",
			actorID:   unitTeacherAID,
			circleID:  unitCircleID,
			targetID:  unitStudentID,
			role:      RoleTeacher,
			wantError: ErrTeacherInvitationRequired,
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
	assignment, err := svc.AssignRole(context.Background(), unitSupervisorID, unitCircleID, unitStudentID, RoleSupervisor)
	if err != nil {
		t.Fatalf("AssignRole: %v", err)
	}
	if assignment.Role != RoleSupervisor || assignment.UserID != unitStudentID || assignment.CircleID != unitCircleID {
		t.Fatalf("unexpected assignment: %+v", assignment)
	}
	if store.members[unitCircleID][unitStudentID] != RoleSupervisor {
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

func TestJoinCircle_ValidInvite_AddsStudentMembership(t *testing.T) {
	t.Parallel()

	store := newStubStore()
	store.circles[unitCircleID] = Circle{
		ID:         unitCircleID,
		Name:       "Quran Circle",
		InviteCode: "HLQ-7X2K",
	}
	svc := NewService(store, nil)

	circle, err := svc.JoinCircle(context.Background(), unitStudentID, "hlq-7x2k")
	if err != nil {
		t.Fatalf("JoinCircle: %v", err)
	}
	if circle.ID != unitCircleID {
		t.Fatalf("circle ID: got %q, want %q", circle.ID, unitCircleID)
	}
	if role := store.members[unitCircleID][unitStudentID]; role != RoleStudent {
		t.Fatalf("membership role: got %q, want %q", role, RoleStudent)
	}
}

func TestJoinCircle_LegacyFourSymbolInviteStillResolves(t *testing.T) {
	t.Parallel()

	store := newStubStore()
	store.circles[unitCircleID] = Circle{
		ID:         unitCircleID,
		Name:       "Legacy Circle",
		InviteCode: "HLQ-7X2K",
	}

	joined, err := NewService(store, nil).JoinCircle(context.Background(), unitStudentID, "HLQ-7X2K")
	if err != nil {
		t.Fatalf("JoinCircle: %v", err)
	}
	if joined.ID != unitCircleID {
		t.Fatalf("circle ID: got %q, want %q", joined.ID, unitCircleID)
	}
}

func TestInviteCode_IsExactlyEightCharacters(t *testing.T) {
	if !isInviteCode("HLQ-7X2K") {
		t.Fatal("expected the approved eight-character invite code to be valid")
	}
}

func TestCircleSettings_DefaultToApprovedMVPValues(t *testing.T) {
	settings, fields := normalizeCircleSettings(CreateCircleRequest{})
	if len(fields) != 0 {
		t.Fatalf("unexpected validation fields: %v", fields)
	}
	if settings.MaxCapacity != 50 || settings.GenderRestriction != "unspecified" || settings.Language != "ar" || settings.GradingPolicy != "required" {
		t.Fatalf("unexpected defaults: %+v", settings)
	}
}

func TestUpdateCircle_ExplicitNullClearsNullableFields(t *testing.T) {
	description, rules := "description", "rules"
	store := newStubStore()
	store.circles[unitCircleID] = Circle{
		ID: unitCircleID, Name: "Circle", Description: &description, Rules: &rules,
		MaxCapacity: 50, GenderRestriction: "unspecified", Language: "ar", GradingPolicy: "required",
	}
	store.members[unitCircleID] = map[string]string{unitCreatorID: RoleTeacher}
	var request UpdateCircleRequest
	if err := json.Unmarshal([]byte(`{"description":null,"rules":null}`), &request); err != nil {
		t.Fatalf("decode update request: %v", err)
	}

	updated, err := NewService(store, nil).UpdateCircle(context.Background(), unitCreatorID, unitCircleID, request)
	if err != nil {
		t.Fatalf("UpdateCircle: %v", err)
	}
	if updated.Description != nil || updated.Rules != nil {
		t.Fatalf("nullable fields were not cleared: description=%v rules=%v", updated.Description, updated.Rules)
	}
}

func TestJoinCircle_RejectsArchivedCircle(t *testing.T) {
	store := newStubStore()
	store.circles[unitCircleID] = Circle{ID: unitCircleID, Name: "Archived", InviteCode: "HLQ-7X2K", IsArchived: true}
	_, err := NewService(store, nil).JoinCircle(context.Background(), unitStudentID, "HLQ-7X2K")
	if !errors.Is(err, ErrCircleArchived) {
		t.Fatalf("expected ErrCircleArchived, got %v", err)
	}
}

func TestRefreshInviteCode_SupervisorIsForbidden(t *testing.T) {
	store := newStubStore()
	store.circles[unitCircleID] = Circle{ID: unitCircleID, InviteCode: "HLQ-7X2K"}
	store.members[unitCircleID] = map[string]string{unitSupervisorID: RoleSupervisor}

	_, err := NewService(store, nil).RefreshInviteCode(context.Background(), unitSupervisorID, unitCircleID)
	if !errors.Is(err, ErrForbidden) {
		t.Fatalf("RefreshInviteCode: got %v want ErrForbidden", err)
	}
}

func TestJoinCircle_RejectsFullCircle(t *testing.T) {
	store := newStubStore()
	store.circles[unitCircleID] = Circle{ID: unitCircleID, Name: "Full", InviteCode: "HLQ-7X2K", MaxCapacity: 1}
	store.members[unitCircleID] = map[string]string{unitTeacherBID: RoleStudent}
	_, err := NewService(store, nil).JoinCircle(context.Background(), unitStudentID, "HLQ-7X2K")
	if !errors.Is(err, ErrCircleFull) {
		t.Fatalf("expected ErrCircleFull, got %v", err)
	}
}

func TestCreateCircle_CreatorOwnsLegacyTeacherID(t *testing.T) {
	store := newStubStore()
	circle, err := NewService(store, nil).CreateCircle(context.Background(), unitCreatorID, CreateCircleRequest{Name: "Quran Circle"})
	if err != nil {
		t.Fatalf("CreateCircle: %v", err)
	}
	if got := store.circles[circle.ID].TeacherID; got != unitCreatorID {
		t.Fatalf("legacy teacher_id = %q, want creator %q", got, unitCreatorID)
	}
	if got := store.members[circle.ID][unitCreatorID]; got != RoleTeacher {
		t.Fatalf("creator membership role = %q, want teacher", got)
	}
}

func TestCreateCircleInvitation_ReplacesPreviousAndBindsRole(t *testing.T) {
	store, service := invitationService()
	request := CreateCircleInvitationRequest{RecipientUserID: unitTeacherBID, Role: RoleTeacher}
	first, err := service.CreateCircleInvitation(context.Background(), unitCreatorID, unitCircleID, request)
	if err != nil {
		t.Fatalf("create first invitation: %v", err)
	}
	second, err := service.CreateCircleInvitation(context.Background(), unitTeacherAID, unitCircleID, request)
	if err != nil {
		t.Fatalf("replace invitation: %v", err)
	}
	if first.ID == second.ID || second.Role != RoleTeacher || second.State != "pending" {
		t.Fatalf("replacement is not a new role-bound pending invitation: first=%+v second=%+v", first, second)
	}
	if got := store.invitations[first.ID].State; got != "superseded" {
		t.Fatalf("prior invitation state: got %q want superseded", got)
	}
}

func TestCreateCircleInvitation_StudentCanInviteOnlyStudents(t *testing.T) {
	_, service := invitationService()
	if _, err := service.CreateCircleInvitation(context.Background(), unitStudentID, unitCircleID, CreateCircleInvitationRequest{
		RecipientUserID: unitTeacherBID, Role: RoleStudent,
	}); err != nil {
		t.Fatalf("student-to-student invitation: %v", err)
	}
	if _, err := service.CreateCircleInvitation(context.Background(), unitStudentID, unitCircleID, CreateCircleInvitationRequest{
		RecipientUserID: unitTeacherBID, Role: RoleTeacher,
	}); !errors.Is(err, ErrForbidden) {
		t.Fatalf("student teacher invitation error: got %v want forbidden", err)
	}
}

func TestAcceptCircleInvitation_RecipientBecomesTeacher(t *testing.T) {
	store, service := invitationService()
	invitation, err := service.CreateCircleInvitation(context.Background(), unitCreatorID, unitCircleID, CreateCircleInvitationRequest{
		RecipientUserID: unitStudentID, Role: RoleTeacher,
	})
	if err != nil {
		t.Fatalf("create invitation: %v", err)
	}
	if _, err := service.AcceptCircleInvitation(context.Background(), unitTeacherBID, invitation.ID); !errors.Is(err, ErrInvitationNotFound) {
		t.Fatalf("non-recipient accept error: got %v want not found", err)
	}
	accepted, err := service.AcceptCircleInvitation(context.Background(), unitStudentID, invitation.ID)
	if err != nil {
		t.Fatalf("recipient accepts: %v", err)
	}
	if accepted.State != "accepted" || store.members[unitCircleID][unitStudentID] != RoleTeacher {
		t.Fatalf("accepted invitation did not promote recipient: invitation=%+v members=%v", accepted, store.members[unitCircleID])
	}
	if _, err := service.AcceptCircleInvitation(context.Background(), unitStudentID, invitation.ID); !errors.Is(err, ErrInvitationConflict) {
		t.Fatalf("replayed acceptance error: got %v want conflict", err)
	}
}

func TestAcceptCircleInvitation_RechecksInviterAuthority(t *testing.T) {
	store, service := invitationService()
	invitation, err := service.CreateCircleInvitation(context.Background(), unitCreatorID, unitCircleID, CreateCircleInvitationRequest{
		RecipientUserID: unitTeacherBID, Role: RoleTeacher,
	})
	if err != nil {
		t.Fatalf("create invitation: %v", err)
	}
	store.members[unitCircleID][unitCreatorID] = RoleStudent
	if _, err := service.AcceptCircleInvitation(context.Background(), unitTeacherBID, invitation.ID); !errors.Is(err, ErrInvitationConflict) {
		t.Fatalf("accept after inviter lost authority: got %v want conflict", err)
	}
	if _, exists := store.members[unitCircleID][unitTeacherBID]; exists {
		t.Fatal("stale invitation created a membership")
	}
}

func TestRevokeCircleInvitation_RequiresManagerAndPendingState(t *testing.T) {
	store, service := invitationService()
	invitation, err := service.CreateCircleInvitation(context.Background(), unitCreatorID, unitCircleID, CreateCircleInvitationRequest{
		RecipientUserID: unitTeacherBID, Role: RoleStudent,
	})
	if err != nil {
		t.Fatalf("create invitation: %v", err)
	}
	if err := service.RevokeCircleInvitation(context.Background(), unitStudentID, unitCircleID, invitation.ID); !errors.Is(err, ErrForbidden) {
		t.Fatalf("student revocation error: got %v want forbidden", err)
	}
	if err := service.RevokeCircleInvitation(context.Background(), unitTeacherAID, unitCircleID, invitation.ID); err != nil {
		t.Fatalf("manager revocation: %v", err)
	}
	if got := store.invitations[invitation.ID].State; got != "revoked" {
		t.Fatalf("invitation state: got %q want revoked", got)
	}
}

func TestListCircleInvitations_StudentSeesOnlyOwnInvitations(t *testing.T) {
	store, service := invitationService()
	for _, recipient := range []string{unitTeacherBID, unitSupervisorID} {
		if _, err := service.CreateCircleInvitation(context.Background(), unitCreatorID, unitCircleID, CreateCircleInvitationRequest{
			RecipientUserID: recipient, Role: RoleStudent,
		}); err != nil {
			t.Fatalf("create manager invitation: %v", err)
		}
	}
	own, err := service.CreateCircleInvitation(context.Background(), unitStudentID, unitCircleID, CreateCircleInvitationRequest{
		RecipientUserID: unitTeacherBID, Role: RoleStudent,
	})
	if err != nil {
		t.Fatalf("create student invitation: %v", err)
	}
	list, err := service.ListCircleInvitations(context.Background(), unitStudentID, unitCircleID)
	if err != nil {
		t.Fatalf("list student invitations: %v", err)
	}
	if len(list.Data) != 1 || list.Data[0].ID != own.ID {
		t.Fatalf("student saw invitations they did not create: %+v; total=%d", list.Data, len(store.invitations))
	}
}

func invitationService() (*stubStore, *Service) {
	store := newStubStore()
	store.circles[unitCircleID] = Circle{ID: unitCircleID, MaxCapacity: 2}
	store.members[unitCircleID] = map[string]string{
		unitCreatorID: RoleTeacher, unitTeacherAID: RoleSupervisor, unitStudentID: RoleStudent,
	}
	return store, NewService(store, nil)
}

func TestArchiveCircle_TeacherArchivesIdempotently(t *testing.T) {
	store := newStubStore()
	store.circles[unitCircleID] = Circle{ID: unitCircleID, Name: "Circle", InviteCode: "HLQ-7X2K"}
	store.members[unitCircleID] = map[string]string{unitTeacherAID: RoleTeacher, unitStudentID: RoleStudent}
	svc := NewService(store, nil)

	if err := svc.ArchiveCircle(context.Background(), unitTeacherAID, unitCircleID); err != nil {
		t.Fatalf("ArchiveCircle: %v", err)
	}
	if !store.circles[unitCircleID].IsArchived {
		t.Fatal("circle must be archived")
	}
	if _, ok := store.members[unitCircleID][unitStudentID]; !ok {
		t.Fatal("archive must retain membership history")
	}
	if err := svc.ArchiveCircle(context.Background(), unitTeacherAID, unitCircleID); err != nil {
		t.Fatalf("second archive must be idempotent: %v", err)
	}
}

func TestArchiveCircle_NonTeacherIsForbidden(t *testing.T) {
	store := newStubStore()
	store.circles[unitCircleID] = Circle{ID: unitCircleID, Name: "Circle", InviteCode: "HLQ-7X2K"}
	store.members[unitCircleID] = map[string]string{unitSupervisorID: RoleSupervisor, unitTeacherAID: RoleTeacher}

	if err := NewService(store, nil).ArchiveCircle(context.Background(), unitSupervisorID, unitCircleID); !errors.Is(err, ErrForbidden) {
		t.Fatalf("ArchiveCircle: got %v want ErrForbidden", err)
	}
	if store.circles[unitCircleID].IsArchived {
		t.Fatal("forbidden archive must not change the circle")
	}
}

func strPtr(value string) *string {
	return &value
}

func TestNoopAuditLogger_Log_DoesNotPanic(t *testing.T) {
	// The noop logger is selected by NewService when no audit logger is
	// supplied. This guards against a future change that accidentally adds
	// side effects (or a panic) to the discard implementation.
	var audit noopAuditLogger
	audit.Log(context.Background(), logging.CircleCreateEvent(unitCreatorID, unitCircleID, 1, false))
}
