package rbac

import (
	"context"
	"encoding/json"
	"errors"
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
	users          map[string]bool
	circles        map[string]Circle
	members        map[string]map[string]string
	findCircleErr  error
	isMemberErr    error
	listMembersErr error
	nextID         int
	insertErrors   []error
	searchQuery    string
	searchLimit    int
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

func (s *stubStore) SearchUsers(_ context.Context, query string, limit int) ([]UserSearchResult, error) {
	s.searchQuery = query
	s.searchLimit = limit
	return nil, nil
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

func TestCreateCircle_SelectedTeacherOwnsLegacyTeacherID(t *testing.T) {
	store := newStubStore()
	_, err := NewService(store, nil).CreateCircle(context.Background(), unitCreatorID, CreateCircleRequest{
		Name: "Quran Circle", TeacherUserIDs: []string{unitTeacherAID},
	})
	if err != nil {
		t.Fatalf("CreateCircle: %v", err)
	}
	if got := store.circles[unitCircleID].TeacherID; got != unitTeacherAID {
		t.Fatalf("legacy teacher_id = %q, want %q", got, unitTeacherAID)
	}
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
