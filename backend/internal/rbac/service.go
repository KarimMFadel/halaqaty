package rbac

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/KarimMFadel/halaqaty/backend/internal/platform/httpconst"
	"github.com/KarimMFadel/halaqaty/backend/internal/platform/logging"
)

const (
	circleNameMinLen     = 2
	circleNameMaxLen     = 100
	userSearchMinLen     = 2
	userSearchMaxLen     = 100
	userSearchLimit      = 20
	publicCirclePageSize = 20
	// inviteCodeAlphabet excludes visually ambiguous characters; 32 symbols
	// divide 256 exactly, so byte-to-symbol mapping stays uniform.
	//
	// inviteCodeLength sets the keyspace for newly generated codes:
	// 32^12 ≈ 2^60, which defeats brute-force invite guessing even with
	// rate limiting as a backstop. Circles created before this change keep
	// their stored 4-symbol codes; isInviteCode still accepts that legacy
	// length so existing invite links keep working.
	inviteCodeAlphabet       = "ABCDEFGHJKLMNPQRSTUVWXYZ23456789"
	inviteCodeLength         = 12
	inviteCodePrefix         = "HLQ-"
	inviteGenerationAttempts = 3
	// inviteLinkBase is the public join-link origin shared by all invite responses.
	inviteLinkBase = "https://halaqaty.app/join/"
)

var (
	// ErrCircleNotFound is returned when the target circle does not exist.
	ErrCircleNotFound = errors.New("circle not found")
	// ErrMemberNotFound is returned when the target user is not a circle member.
	ErrMemberNotFound = errors.New("circle member not found")
	// ErrSelfRoleChange is returned when a manager targets their own membership.
	ErrSelfRoleChange = errors.New("members cannot change their own role")
	// ErrFinalTeacher is returned when a change would leave the circle without a teacher.
	ErrFinalTeacher = errors.New("circle must retain at least one teacher")
	// ErrForbidden is returned when the actor may not manage roles in the circle.
	ErrForbidden = errors.New("forbidden")
	// ErrAlreadyMember is returned when a user joins a circle they already belong to.
	ErrAlreadyMember             = errors.New("user is already a circle member")
	ErrCircleArchived            = errors.New("circle is archived")
	ErrCircleFull                = errors.New("circle is full")
	ErrCircleLimit               = errors.New("user has reached the circle limit")
	ErrCirclePrivate             = errors.New("circle is private")
	ErrInvitationNotFound        = errors.New("circle invitation not found")
	ErrInvitationConflict        = errors.New("circle invitation is no longer pending")
	ErrTeacherInvitationRequired = errors.New("teacher role requires recipient acceptance")
)

// ValidationError carries field-level circle validation failures.
type ValidationError struct {
	Fields map[string]string
}

// Error returns a stable top-level validation message.
func (e *ValidationError) Error() string {
	return httpconst.ErrorMessageValidationFailed
}

// Store is the circle RBAC persistence contract.
type Store interface {
	WithinTransaction(ctx context.Context, fn func(tx Store) error) error
	UsersExist(ctx context.Context, userIDs []string) (map[string]bool, error)
	LockUser(ctx context.Context, userID string) error
	InsertCircle(ctx context.Context, name, ownerID, inviteCode string, settings CircleSettings) (Circle, error)
	InsertMember(ctx context.Context, circleID, userID, role string) error
	FindCircleByInviteCode(ctx context.Context, inviteCode string) (Circle, error)
	FindCircleByID(ctx context.Context, circleID string) (Circle, error)
	FindCircleByIDForUpdate(ctx context.Context, circleID string) (Circle, error)
	ListPublicCircles(ctx context.Context, query, cursor string, limit int) ([]PublicCircleSummary, error)
	ListUserCircles(ctx context.Context, userID, role string) ([]PublicCircleSummary, error)
	UpdateCircle(ctx context.Context, circleID, name string, settings CircleSettings) (Circle, error)
	RefreshInviteCode(ctx context.Context, circleID, inviteCode string) error
	RemoveMember(ctx context.Context, circleID, userID string) error
	ArchiveCircle(ctx context.Context, circleID string) error
	ListMembers(ctx context.Context, circleID string) ([]CircleMember, error)
	IsMember(ctx context.Context, circleID, userID string) (bool, error)
	CircleExists(ctx context.Context, circleID string) (bool, error)
	LockMembers(ctx context.Context, circleID string) ([]Member, error)
	CountActiveMemberships(ctx context.Context, userID string) (int, error)
	UpdateMemberRole(ctx context.Context, circleID, userID, role string) error
	SearchUsers(ctx context.Context, query string, limit int) ([]UserSearchResult, error)
}

type circleInvitationStore interface {
	ReplacePendingInvitation(ctx context.Context, circleID, inviterID, recipientID, role string) (CircleInvitation, error)
	ListPendingCircleInvitations(ctx context.Context, circleID string) ([]CircleInvitation, error)
	ListPendingUserInvitations(ctx context.Context, userID string) ([]CircleInvitation, error)
	FindInvitation(ctx context.Context, invitationID string) (CircleInvitation, error)
	LockInvitation(ctx context.Context, circleID, invitationID string) (CircleInvitation, error)
	SetInvitationState(ctx context.Context, invitationID, state string) error
}

type circleTeacherStore interface {
	SetPrimaryTeacher(ctx context.Context, circleID, userID string) error
}

type circleNotificationProjector interface {
	ProjectStudentJoinedCircle(context.Context, string, string) error
	ProjectCircleArchived(context.Context, string) error
}

type circleInvitationNotificationProjector interface {
	ProjectCircleInvitation(context.Context, CircleInvitation) error
}

// ListCircles returns the authenticated user's active circle memberships.
func (s *Service) ListCircles(ctx context.Context, userID, role string) ([]PublicCircleSummary, error) {
	if role != "" && role != RoleStudent && role != RoleTeacher && role != RoleSupervisor {
		return nil, &ValidationError{Fields: map[string]string{"role": "invalid role"}}
	}
	circles, err := s.store.ListUserCircles(ctx, userID, role)
	if err != nil {
		return nil, fmt.Errorf("list user circles: %w", err)
	}
	if circles == nil {
		circles = []PublicCircleSummary{}
	}
	return circles, nil
}

// JoinCircle adds the authenticated user as a student using a circle invite code.
func (s *Service) JoinCircle(ctx context.Context, userID, inviteCode string) (CircleResponse, error) {
	inviteCode = strings.ToUpper(strings.TrimSpace(inviteCode))
	if !isInviteCode(inviteCode) {
		return CircleResponse{}, &ValidationError{Fields: map[string]string{httpconst.FieldInviteCode: httpconst.ErrorMessageInviteCodeInvalid}}
	}

	return s.joinCircle(ctx, userID, func(tx Store) (Circle, error) {
		return tx.FindCircleByInviteCode(ctx, inviteCode)
	})
}

// JoinPublicCircle adds the authenticated user as a student of an active public circle.
func (s *Service) JoinPublicCircle(ctx context.Context, userID, circleID string) (CircleResponse, error) {
	if !isUUID(circleID) {
		return CircleResponse{}, &ValidationError{Fields: map[string]string{httpconst.FieldCircleID: httpconst.ErrorMessageCircleIDInvalid}}
	}
	return s.joinCircle(ctx, userID, func(tx Store) (Circle, error) {
		circle, err := tx.FindCircleByIDForUpdate(ctx, circleID)
		if err != nil {
			return Circle{}, err
		}
		if circle.IsPrivate {
			return Circle{}, ErrCirclePrivate
		}
		return circle, nil
	})
}

func (s *Service) joinCircle(ctx context.Context, userID string, find func(Store) (Circle, error)) (CircleResponse, error) {
	var circle Circle
	err := s.store.WithinTransaction(ctx, func(tx Store) error {
		found, err := find(tx)
		if err != nil {
			return err
		}
		if found.IsArchived {
			return ErrCircleArchived
		}
		members, err := tx.LockMembers(ctx, found.ID)
		if err != nil {
			return err
		}
		if _, exists := memberRole(members, userID); exists {
			return ErrAlreadyMember
		}
		if err := tx.LockUser(ctx, userID); err != nil {
			return err
		}
		capacity := found.MaxCapacity
		if capacity == 0 {
			capacity = 50
		}
		if countStudents(members) >= capacity {
			return ErrCircleFull
		}
		membershipCount, err := tx.CountActiveMemberships(ctx, userID)
		if err != nil {
			return fmt.Errorf("count active memberships: %w", err)
		}
		if membershipCount >= 5 {
			return ErrCircleLimit
		}
		if err := tx.InsertMember(ctx, found.ID, userID, RoleStudent); err != nil {
			return err
		}
		if projector, ok := tx.(circleNotificationProjector); ok {
			if err := projector.ProjectStudentJoinedCircle(ctx, found.ID, userID); err != nil {
				return fmt.Errorf("project student circle-join notification: %w", err)
			}
		}
		circle = found
		return nil
	})
	if err != nil {
		return CircleResponse{}, fmt.Errorf("join circle: %w", err)
	}

	s.audit.Log(ctx, logging.CircleJoinEvent(userID, circle.ID))
	return circleResponse(circle), nil
}

func circleResponse(circle Circle) CircleResponse {
	return CircleResponse{
		ID: circle.ID, Name: circle.Name, InviteCode: circle.InviteCode,
		InviteLink:  inviteLinkBase + circle.InviteCode,
		Description: circle.Description, Rules: circle.Rules, MaxCapacity: circle.MaxCapacity,
		IsPrivate: circle.IsPrivate, GenderRestriction: circle.GenderRestriction,
		Language: circle.Language, IsArchived: circle.IsArchived, CreatedAt: circle.CreatedAt,
		GradingPolicy: circle.GradingPolicy,
	}
}

// AuditLogger records security-relevant circle events.
type AuditLogger interface {
	Log(ctx context.Context, event logging.AuditEvent)
}

// noopAuditLogger discards events when no audit logger is configured.
type noopAuditLogger struct{}

func (noopAuditLogger) Log(context.Context, logging.AuditEvent) {}

// Service groups circle role-management use cases.
type Service struct {
	store Store
	audit AuditLogger
}

// NewService constructs a circle RBAC service.
func NewService(store Store, audit AuditLogger) *Service {
	if audit == nil {
		audit = noopAuditLogger{}
	}
	return &Service{store: store, audit: audit}
}

// SearchUsers returns registered users whose display name contains query.
func (s *Service) SearchUsers(ctx context.Context, query string) ([]UserSearchResult, error) {
	query = strings.TrimSpace(query)
	if length := utf8.RuneCountInString(query); length < userSearchMinLen || length > userSearchMaxLen {
		return nil, &ValidationError{Fields: map[string]string{
			httpconst.FieldQuery: httpconst.ErrorMessageUserSearchQueryInvalid,
		}}
	}
	query = strings.NewReplacer("\\", "\\\\", "%", "\\%", "_", "\\_").Replace(query)
	users, err := s.store.SearchUsers(ctx, query, userSearchLimit)
	if err != nil {
		return nil, fmt.Errorf("search users: %w", err)
	}
	if users == nil {
		users = []UserSearchResult{}
	}
	return users, nil
}

// DiscoverPublicCircles returns one redacted public-circle page.
func (s *Service) DiscoverPublicCircles(ctx context.Context, query, cursor string) (PublicCircleListResponse, error) {
	query = strings.TrimSpace(query)
	if utf8.RuneCountInString(query) > userSearchMaxLen {
		return PublicCircleListResponse{}, &ValidationError{Fields: map[string]string{httpconst.FieldDiscoverQuery: httpconst.ErrorMessageDiscoverQueryInvalid}}
	}
	query = strings.NewReplacer("\\", "\\\\", "%", "\\%", "_", "\\_").Replace(query)
	cursor = strings.TrimSpace(cursor)
	if cursor != "" && !isUUID(cursor) {
		return PublicCircleListResponse{}, &ValidationError{Fields: map[string]string{httpconst.FieldCursor: httpconst.ErrorMessageCursorInvalid}}
	}

	circles, err := s.store.ListPublicCircles(ctx, query, cursor, publicCirclePageSize+1)
	if err != nil {
		return PublicCircleListResponse{}, fmt.Errorf("discover public circles: %w", err)
	}
	response := PublicCircleListResponse{Data: circles}
	if len(circles) > publicCirclePageSize {
		response.Data = circles[:publicCirclePageSize]
		nextCursor := response.Data[len(response.Data)-1].ID
		response.NextCursor = &nextCursor
	}
	if response.Data == nil {
		response.Data = []PublicCircleSummary{}
	}
	return response, nil
}

// CreateCircle creates a circle and its initial memberships in one transaction.
func (s *Service) CreateCircle(ctx context.Context, creatorID string, req CreateCircleRequest) (CircleResponse, error) {
	name, backupSupervisorID, fields := validateCreateCircleRequest(creatorID, req)
	settings, settingFields := normalizeCircleSettings(req)
	for field, message := range settingFields {
		fields[field] = message
	}

	if len(fields) == 0 {
		existenceFields, err := s.validateBackupSupervisorExists(ctx, backupSupervisorID)
		if err != nil {
			return CircleResponse{}, fmt.Errorf("validate circle assignees: %w", err)
		}
		fields = existenceFields
	}
	if len(fields) > 0 {
		return CircleResponse{}, &ValidationError{Fields: fields}
	}

	var circle Circle
	var err error
	for attempt := 0; attempt < inviteGenerationAttempts; attempt++ {
		inviteCode, generationErr := generateInviteCode()
		if generationErr != nil {
			return CircleResponse{}, generationErr
		}
		err = s.store.WithinTransaction(ctx, func(tx Store) error {
			created, insertErr := tx.InsertCircle(ctx, name, creatorID, inviteCode, settings)
			if insertErr != nil {
				return insertErr
			}
			circle = created
			if backupSupervisorID != "" {
				if insertErr := tx.InsertMember(ctx, circle.ID, backupSupervisorID, RoleSupervisor); insertErr != nil {
					return insertErr
				}
			}
			return tx.InsertMember(ctx, circle.ID, creatorID, RoleTeacher)
		})
		if !isUniqueViolation(err) {
			break
		}
	}
	if err != nil {
		return CircleResponse{}, fmt.Errorf("create circle: %w", err)
	}

	s.audit.Log(ctx, logging.CircleCreateEvent(creatorID, circle.ID, 1, backupSupervisorID != ""))

	return CircleResponse{
		ID:                circle.ID,
		Name:              circle.Name,
		InviteCode:        circle.InviteCode,
		InviteLink:        inviteLinkBase + circle.InviteCode,
		Description:       settings.Description,
		Rules:             settings.Rules,
		MaxCapacity:       settings.MaxCapacity,
		IsPrivate:         settings.IsPrivate,
		GenderRestriction: settings.GenderRestriction,
		Language:          settings.Language,
		GradingPolicy:     settings.GradingPolicy,
		IsArchived:        circle.IsArchived,
		CreatedAt:         circle.CreatedAt,
	}, nil
}

// UpdateCircle applies a teacher-authorized partial settings update.
func (s *Service) UpdateCircle(ctx context.Context, actorID, circleID string, req UpdateCircleRequest) (CircleResponse, error) {
	if !isUUID(circleID) {
		return CircleResponse{}, ErrCircleNotFound
	}
	var updated Circle
	err := s.store.WithinTransaction(ctx, func(tx Store) error {
		circle, err := tx.FindCircleByIDForUpdate(ctx, circleID)
		if err != nil {
			return err
		}
		if circle.IsArchived {
			return ErrCircleArchived
		}
		members, err := tx.LockMembers(ctx, circleID)
		if err != nil {
			return err
		}
		role, ok := memberRole(members, actorID)
		if !ok || role != RoleTeacher {
			return ErrForbidden
		}

		name, settings, fields := mergeCircleUpdate(circle, req)
		if len(fields) > 0 {
			return &ValidationError{Fields: fields}
		}
		updated, err = tx.UpdateCircle(ctx, circleID, name, settings)
		return err
	})
	if err != nil {
		return CircleResponse{}, fmt.Errorf("update circle: %w", err)
	}
	return circleResponse(updated), nil
}

func mergeCircleUpdate(circle Circle, req UpdateCircleRequest) (string, CircleSettings, map[string]string) {
	name := circle.Name
	if req.Name != nil {
		name = strings.TrimSpace(*req.Name)
	}
	create := CreateCircleRequest{
		Name: name, Description: circle.Description, Rules: circle.Rules,
		MaxCapacity: circle.MaxCapacity, IsPrivate: circle.IsPrivate,
		GenderRestriction: circle.GenderRestriction, Language: circle.Language,
		GradingPolicy: circle.GradingPolicy,
	}
	if req.Description.Set {
		create.Description = req.Description.Value
	}
	if req.Rules.Set {
		create.Rules = req.Rules.Value
	}
	if req.MaxCapacity != nil {
		create.MaxCapacity = *req.MaxCapacity
	}
	if req.IsPrivate != nil {
		create.IsPrivate = *req.IsPrivate
	}
	if req.GenderRestriction != nil {
		create.GenderRestriction = *req.GenderRestriction
	}
	if req.Language != nil {
		create.Language = *req.Language
	}
	if req.GradingPolicy != nil {
		create.GradingPolicy = *req.GradingPolicy
	}

	_, _, fields := validateCreateCircleRequest("", create)
	settings, settingFields := normalizeCircleSettings(create)
	for field, message := range settingFields {
		fields[field] = message
	}
	return name, settings, fields
}

func normalizeCircleSettings(req CreateCircleRequest) (CircleSettings, map[string]string) {
	settings := CircleSettings{
		Description:       req.Description,
		Rules:             req.Rules,
		MaxCapacity:       req.MaxCapacity,
		IsPrivate:         req.IsPrivate,
		GenderRestriction: req.GenderRestriction,
		Language:          req.Language,
		GradingPolicy:     req.GradingPolicy,
	}
	if settings.MaxCapacity == 0 {
		settings.MaxCapacity = 50
	}
	if settings.GenderRestriction == "" {
		settings.GenderRestriction = "unspecified"
	}
	if settings.Language == "" {
		settings.Language = "ar"
	}
	if settings.GradingPolicy == "" {
		settings.GradingPolicy = "required"
	}
	fields := make(map[string]string)
	if settings.Description != nil && utf8.RuneCountInString(*settings.Description) > 500 {
		fields[httpconst.FieldDescription] = httpconst.ErrorMessageCircleDescriptionInvalid
	}
	if settings.Rules != nil && utf8.RuneCountInString(*settings.Rules) > 1000 {
		fields[httpconst.FieldRules] = httpconst.ErrorMessageCircleRulesInvalid
	}
	if settings.MaxCapacity < 2 || settings.MaxCapacity > 200 {
		fields[httpconst.FieldMaxCapacity] = httpconst.ErrorMessageCircleCapacityInvalid
	}
	if settings.GenderRestriction != "male" && settings.GenderRestriction != "female" && settings.GenderRestriction != "mixed" && settings.GenderRestriction != "unspecified" {
		fields[httpconst.FieldGenderRestriction] = httpconst.ErrorMessageCircleGenderInvalid
	}
	if utf8.RuneCountInString(settings.Language) > 10 {
		fields[httpconst.FieldLanguage] = httpconst.ErrorMessageCircleLanguageInvalid
	}
	if settings.GradingPolicy != "required" && settings.GradingPolicy != "optional" {
		fields[httpconst.FieldGradingPolicy] = httpconst.ErrorMessageCircleGradingInvalid
	}
	return settings, fields
}

// AssignRole changes another member's role under circle row locks.
func (s *Service) AssignRole(ctx context.Context, actorID, circleID, targetUserID, role string) (RoleAssignment, error) {
	if role != RoleStudent && role != RoleSupervisor && role != RoleTeacher {
		return RoleAssignment{}, &ValidationError{Fields: map[string]string{
			httpconst.FieldRole: httpconst.ErrorMessageCircleRoleInvalid,
		}}
	}
	if actorID == targetUserID {
		return RoleAssignment{}, ErrSelfRoleChange
	}
	if !isUUID(circleID) {
		return RoleAssignment{}, ErrCircleNotFound
	}
	if !isUUID(targetUserID) {
		return RoleAssignment{}, ErrMemberNotFound
	}

	var oldRole string
	err := s.store.WithinTransaction(ctx, func(tx Store) error {
		circle, err := tx.FindCircleByIDForUpdate(ctx, circleID)
		if err != nil {
			return err
		}
		if circle.IsArchived {
			return ErrCircleArchived
		}

		members, err := tx.LockMembers(ctx, circleID)
		if err != nil {
			return err
		}

		actorRole, ok := memberRole(members, actorID)
		if !ok || (actorRole != RoleTeacher && actorRole != RoleSupervisor) {
			return ErrForbidden
		}

		targetRole, ok := memberRole(members, targetUserID)
		if !ok {
			return ErrMemberNotFound
		}
		oldRole = targetRole
		if targetRole == role {
			return nil
		}
		if role == RoleTeacher {
			return ErrTeacherInvitationRequired
		}

		if targetRole == RoleTeacher && role != RoleTeacher && countTeachers(members) <= 1 {
			return ErrFinalTeacher
		}
		if targetRole == RoleTeacher && role != RoleTeacher && circle.TeacherID == targetUserID {
			if err := setPrimaryTeacher(ctx, tx, circleID, members, targetUserID); err != nil {
				return err
			}
		}
		return tx.UpdateMemberRole(ctx, circleID, targetUserID, role)
	})
	if err != nil {
		return RoleAssignment{}, fmt.Errorf("assign role: %w", err)
	}

	if oldRole != role {
		s.audit.Log(ctx, logging.RoleChangeEvent(actorID, targetUserID, circleID, oldRole, role))
	}
	return RoleAssignment{CircleID: circleID, UserID: targetUserID, Role: role}, nil
}

// CreateCircleInvitation records a recipient-bound role offer and its notification atomically.
func (s *Service) CreateCircleInvitation(ctx context.Context, actorID, circleID string, request CreateCircleInvitationRequest) (CircleInvitation, error) {
	fields := validateCircleInvitationRequest(actorID, circleID, request)
	if len(fields) > 0 {
		return CircleInvitation{}, &ValidationError{Fields: fields}
	}
	var invitation CircleInvitation
	err := s.store.WithinTransaction(ctx, func(tx Store) error {
		circle, err := tx.FindCircleByIDForUpdate(ctx, circleID)
		if err != nil {
			return err
		}
		if circle.IsArchived {
			return ErrCircleArchived
		}
		members, err := tx.LockMembers(ctx, circleID)
		if err != nil {
			return err
		}
		actorRole, ok := memberRole(members, actorID)
		if !ok || !canInviteRole(actorRole, request.Role) {
			return ErrForbidden
		}
		existing, err := tx.UsersExist(ctx, []string{request.RecipientUserID})
		if err != nil {
			return fmt.Errorf("check invitation recipient: %w", err)
		}
		if !existing[request.RecipientUserID] {
			return &ValidationError{Fields: map[string]string{httpconst.FieldRecipientUserID: httpconst.ErrorMessageCircleAssigneeUnknown}}
		}
		invitationStore, ok := tx.(circleInvitationStore)
		if !ok {
			return errors.New("circle invitation store is not configured")
		}
		invitation, err = invitationStore.ReplacePendingInvitation(ctx, circleID, actorID, request.RecipientUserID, request.Role)
		if err != nil {
			return err
		}
		if projector, ok := tx.(circleInvitationNotificationProjector); ok {
			if err := projector.ProjectCircleInvitation(ctx, invitation); err != nil {
				return fmt.Errorf("project circle invitation notification: %w", err)
			}
		}
		return nil
	})
	if err != nil {
		return CircleInvitation{}, fmt.Errorf("create circle invitation: %w", err)
	}
	s.audit.Log(ctx, logging.CircleInvitationSentEvent(actorID, invitation.RecipientUserID, circleID, invitation.Role))
	return invitation, nil
}

// ListCircleInvitations returns pending invitations visible to an active member.
func (s *Service) ListCircleInvitations(ctx context.Context, actorID, circleID string) (CircleInvitationListResponse, error) {
	if !isUUID(circleID) {
		return CircleInvitationListResponse{}, ErrCircleNotFound
	}
	if _, err := s.store.FindCircleByID(ctx, circleID); err != nil {
		return CircleInvitationListResponse{}, fmt.Errorf("find circle for invitations: %w", err)
	}
	members, err := s.store.LockMembers(ctx, circleID)
	if err != nil {
		return CircleInvitationListResponse{}, fmt.Errorf("list circle invitation members: %w", err)
	}
	role, ok := memberRole(members, actorID)
	if !ok {
		return CircleInvitationListResponse{}, ErrForbidden
	}
	invitationStore, ok := s.store.(circleInvitationStore)
	if !ok {
		return CircleInvitationListResponse{}, errors.New("circle invitation store is not configured")
	}
	invitations, err := invitationStore.ListPendingCircleInvitations(ctx, circleID)
	if err != nil {
		return CircleInvitationListResponse{}, fmt.Errorf("list circle invitations: %w", err)
	}
	if role == RoleStudent {
		visible := invitations[:0]
		for _, invitation := range invitations {
			if invitation.InviterUserID == actorID {
				visible = append(visible, invitation)
			}
		}
		invitations = visible
	}
	return CircleInvitationListResponse{Data: invitations}, nil
}

// ListMyCircleInvitations returns only pending invitations addressed to the caller.
func (s *Service) ListMyCircleInvitations(ctx context.Context, userID string) (CircleInvitationListResponse, error) {
	invitationStore, ok := s.store.(circleInvitationStore)
	if !ok {
		return CircleInvitationListResponse{}, errors.New("circle invitation store is not configured")
	}
	invitations, err := invitationStore.ListPendingUserInvitations(ctx, userID)
	if err != nil {
		return CircleInvitationListResponse{}, fmt.Errorf("list invitations addressed to user: %w", err)
	}
	return CircleInvitationListResponse{Data: invitations}, nil
}

// AcceptCircleInvitation grants the stored role only to the named recipient.
func (s *Service) AcceptCircleInvitation(ctx context.Context, userID, invitationID string) (CircleInvitation, error) {
	if !isUUID(invitationID) {
		return CircleInvitation{}, ErrInvitationNotFound
	}
	invitationStore, ok := s.store.(circleInvitationStore)
	if !ok {
		return CircleInvitation{}, errors.New("circle invitation store is not configured")
	}
	initial, err := invitationStore.FindInvitation(ctx, invitationID)
	if err != nil {
		return CircleInvitation{}, fmt.Errorf("find circle invitation: %w", err)
	}
	if initial.RecipientUserID != userID {
		return CircleInvitation{}, ErrInvitationNotFound
	}
	var accepted CircleInvitation
	err = s.store.WithinTransaction(ctx, func(tx Store) error {
		circle, err := tx.FindCircleByIDForUpdate(ctx, initial.CircleID)
		if err != nil {
			return err
		}
		if circle.IsArchived {
			return ErrCircleArchived
		}
		transactionInvitationStore, ok := tx.(circleInvitationStore)
		if !ok {
			return errors.New("circle invitation store is not configured")
		}
		invitation, err := transactionInvitationStore.LockInvitation(ctx, initial.CircleID, invitationID)
		if err != nil {
			return err
		}
		if invitation.RecipientUserID != userID {
			return ErrInvitationNotFound
		}
		if invitation.State != "pending" {
			return ErrInvitationConflict
		}
		members, err := tx.LockMembers(ctx, invitation.CircleID)
		if err != nil {
			return err
		}
		inviterRole, ok := memberRole(members, invitation.InviterUserID)
		if !ok || !canInviteRole(inviterRole, invitation.Role) {
			return ErrInvitationConflict
		}
		if err := s.acceptInvitationMembership(ctx, tx, circle, members, invitation); err != nil {
			return err
		}
		if invitation.Role == RoleTeacher && !memberRoleIsTeacher(members, circle.TeacherID) {
			teacherStore, ok := tx.(circleTeacherStore)
			if !ok {
				return errors.New("circle teacher store is not configured")
			}
			if err := teacherStore.SetPrimaryTeacher(ctx, circle.ID, invitation.RecipientUserID); err != nil {
				return err
			}
		}
		if err := transactionInvitationStore.SetInvitationState(ctx, invitation.ID, "accepted"); err != nil {
			return err
		}
		invitation.State = "accepted"
		now := time.Now().UTC()
		invitation.ResolvedAt = &now
		accepted = invitation
		return nil
	})
	if err != nil {
		return CircleInvitation{}, fmt.Errorf("accept circle invitation: %w", err)
	}
	s.audit.Log(ctx, logging.CircleInvitationAcceptedEvent(userID, accepted.CircleID, accepted.Role))
	return accepted, nil
}

func (s *Service) acceptInvitationMembership(ctx context.Context, tx Store, circle Circle, members []Member, invitation CircleInvitation) error {
	currentRole, isMember := memberRole(members, invitation.RecipientUserID)
	if invitation.Role == RoleTeacher {
		if isMember {
			if currentRole != RoleTeacher {
				return tx.UpdateMemberRole(ctx, invitation.CircleID, invitation.RecipientUserID, RoleTeacher)
			}
			return nil
		}
		return s.insertInvitedMember(ctx, tx, circle, members, invitation, RoleTeacher)
	}
	if isMember {
		return nil
	}
	if countTeachers(members) == 0 {
		return ErrFinalTeacher
	}
	capacity := circle.MaxCapacity
	if capacity == 0 {
		capacity = 50
	}
	if countStudents(members) >= capacity {
		return ErrCircleFull
	}
	return s.insertInvitedMember(ctx, tx, circle, members, invitation, RoleStudent)
}

func (s *Service) insertInvitedMember(ctx context.Context, tx Store, circle Circle, members []Member, invitation CircleInvitation, role string) error {
	if err := tx.LockUser(ctx, invitation.RecipientUserID); err != nil {
		return err
	}
	count, err := tx.CountActiveMemberships(ctx, invitation.RecipientUserID)
	if err != nil {
		return fmt.Errorf("count recipient memberships: %w", err)
	}
	if count >= 5 {
		return ErrCircleLimit
	}
	return tx.InsertMember(ctx, circle.ID, invitation.RecipientUserID, role)
}

// RevokeCircleInvitation revokes a pending invitation as an active manager.
func (s *Service) RevokeCircleInvitation(ctx context.Context, actorID, circleID, invitationID string) error {
	if !isUUID(circleID) || !isUUID(invitationID) {
		return ErrInvitationNotFound
	}
	err := s.store.WithinTransaction(ctx, func(tx Store) error {
		circle, err := tx.FindCircleByIDForUpdate(ctx, circleID)
		if err != nil {
			return err
		}
		if circle.IsArchived {
			return ErrCircleArchived
		}
		members, err := tx.LockMembers(ctx, circleID)
		if err != nil {
			return err
		}
		actorRole, ok := memberRole(members, actorID)
		if !ok || (actorRole != RoleTeacher && actorRole != RoleSupervisor) {
			return ErrForbidden
		}
		invitationStore, ok := tx.(circleInvitationStore)
		if !ok {
			return errors.New("circle invitation store is not configured")
		}
		invitation, err := invitationStore.LockInvitation(ctx, circleID, invitationID)
		if err != nil {
			return err
		}
		if invitation.State != "pending" {
			return ErrInvitationConflict
		}
		return invitationStore.SetInvitationState(ctx, invitation.ID, "revoked")
	})
	if err != nil {
		return fmt.Errorf("revoke circle invitation: %w", err)
	}
	s.audit.Log(ctx, logging.CircleInvitationRevokedEvent(actorID, circleID, invitationID))
	return nil
}

func validateCircleInvitationRequest(actorID, circleID string, request CreateCircleInvitationRequest) map[string]string {
	fields := make(map[string]string)
	if !isUUID(circleID) {
		fields[httpconst.FieldCircleID] = httpconst.ErrorMessageCircleIDInvalid
	}
	if request.Role != RoleStudent && request.Role != RoleTeacher {
		fields[httpconst.FieldRole] = httpconst.ErrorMessageCircleInvitationRoleInvalid
	}
	if !isUUID(request.RecipientUserID) || request.RecipientUserID == actorID {
		fields[httpconst.FieldRecipientUserID] = httpconst.ErrorMessageCircleInvitationRecipientInvalid
	}
	return fields
}

func canInviteRole(actorRole, inviteRole string) bool {
	if inviteRole == RoleStudent {
		return actorRole == RoleStudent || actorRole == RoleSupervisor || actorRole == RoleTeacher
	}
	return inviteRole == RoleTeacher && (actorRole == RoleSupervisor || actorRole == RoleTeacher)
}

// AddStudentMember inserts a student membership idempotently (invite acceptance).
func (s *Service) AddStudentMember(ctx context.Context, circleID, userID string) error {
	if !isUUID(circleID) {
		return fmt.Errorf("add student member: %w", ErrCircleNotFound)
	}

	err := s.store.WithinTransaction(ctx, func(tx Store) error {
		circle, err := tx.FindCircleByIDForUpdate(ctx, circleID)
		if err != nil {
			return err
		}
		if circle.IsArchived {
			return ErrCircleArchived
		}
		members, err := tx.LockMembers(ctx, circleID)
		if err != nil {
			return err
		}
		if _, exists := memberRole(members, userID); exists {
			return nil
		}
		if err := tx.LockUser(ctx, userID); err != nil {
			return err
		}
		capacity := circle.MaxCapacity
		if capacity == 0 {
			capacity = 50
		}
		if countStudents(members) >= capacity {
			return ErrCircleFull
		}
		count, err := tx.CountActiveMemberships(ctx, userID)
		if err != nil {
			return err
		}
		if count >= 5 {
			return ErrCircleLimit
		}
		return tx.InsertMember(ctx, circleID, userID, RoleStudent)
	})
	if err != nil {
		return fmt.Errorf("add student member: %w", err)
	}
	return nil
}

// RefreshInviteCode rotates the current code for a circle manager.
func (s *Service) RefreshInviteCode(ctx context.Context, actorID, circleID string) (string, error) {
	if !isUUID(circleID) {
		return "", ErrCircleNotFound
	}
	code, err := generateInviteCode()
	if err != nil {
		return "", err
	}
	err = s.store.WithinTransaction(ctx, func(tx Store) error {
		circle, err := tx.FindCircleByIDForUpdate(ctx, circleID)
		if err != nil {
			return err
		}
		if circle.IsArchived {
			return ErrCircleArchived
		}
		members, err := tx.LockMembers(ctx, circleID)
		if err != nil {
			return err
		}
		role, ok := memberRole(members, actorID)
		if !ok || role != RoleTeacher {
			return ErrForbidden
		}
		return tx.RefreshInviteCode(ctx, circleID, code)
	})
	if err != nil {
		return "", fmt.Errorf("refresh invite code: %w", err)
	}
	s.audit.Log(ctx, logging.InviteRefreshEvent(actorID, circleID))
	return code, nil
}

// RemoveMember removes a non-self member while preserving circle history.
func (s *Service) RemoveMember(ctx context.Context, actorID, circleID, targetID string) error {
	if actorID == targetID {
		return ErrSelfRoleChange
	}
	if !isUUID(circleID) {
		return ErrCircleNotFound
	}
	if !isUUID(targetID) {
		return ErrMemberNotFound
	}
	err := s.store.WithinTransaction(ctx, func(tx Store) error {
		circle, err := tx.FindCircleByIDForUpdate(ctx, circleID)
		if err != nil {
			return err
		}
		if circle.IsArchived {
			return ErrCircleArchived
		}
		members, err := tx.LockMembers(ctx, circleID)
		if err != nil {
			return err
		}
		role, ok := memberRole(members, actorID)
		if !ok || role != RoleTeacher {
			return ErrForbidden
		}
		targetRole, ok := memberRole(members, targetID)
		if !ok {
			return ErrMemberNotFound
		}
		if targetRole == RoleTeacher && countTeachers(members) <= 1 {
			return ErrFinalTeacher
		}
		if targetRole == RoleTeacher && circle.TeacherID == targetID {
			if err := setPrimaryTeacher(ctx, tx, circleID, members, targetID); err != nil {
				return err
			}
		}
		return tx.RemoveMember(ctx, circleID, targetID)
	})
	if err != nil {
		return fmt.Errorf("remove circle member: %w", err)
	}
	s.audit.Log(ctx, logging.MemberRemovalEvent(actorID, targetID, circleID))
	return nil
}

func memberRoleIsTeacher(members []Member, userID string) bool {
	role, ok := memberRole(members, userID)
	return ok && role == RoleTeacher
}

func setPrimaryTeacher(ctx context.Context, tx Store, circleID string, members []Member, excludingID string) error {
	store, ok := tx.(circleTeacherStore)
	if !ok {
		return errors.New("circle teacher store is not configured")
	}
	replacement := ""
	for _, member := range members {
		if member.Role == RoleTeacher && member.UserID != excludingID && (replacement == "" || member.UserID < replacement) {
			replacement = member.UserID
		}
	}
	if replacement == "" {
		return ErrFinalTeacher
	}
	if err := store.SetPrimaryTeacher(ctx, circleID, replacement); err != nil {
		return fmt.Errorf("set primary circle teacher: %w", err)
	}
	return nil
}

// ArchiveCircle retires a circle without deleting its memberships or history.
func (s *Service) ArchiveCircle(ctx context.Context, actorID, circleID string) error {
	if !isUUID(circleID) {
		return ErrCircleNotFound
	}
	archived := false
	err := s.store.WithinTransaction(ctx, func(tx Store) error {
		circle, err := tx.FindCircleByIDForUpdate(ctx, circleID)
		if err != nil {
			return err
		}
		members, err := tx.LockMembers(ctx, circleID)
		if err != nil {
			return err
		}
		role, ok := memberRole(members, actorID)
		if !ok || role != RoleTeacher {
			return ErrForbidden
		}
		if circle.IsArchived {
			return nil
		}
		if err := tx.ArchiveCircle(ctx, circleID); err != nil {
			return err
		}
		if projector, ok := tx.(circleNotificationProjector); ok {
			if err := projector.ProjectCircleArchived(ctx, circleID); err != nil {
				return fmt.Errorf("project circle archive notifications: %w", err)
			}
		}
		archived = true
		return nil
	})
	if err != nil {
		return fmt.Errorf("archive circle: %w", err)
	}
	if archived {
		s.audit.Log(ctx, logging.CircleArchiveEvent(actorID, circleID))
	}
	return nil
}

func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}

// validateCreateCircleRequest checks shape-level rules: name bounds, duplicate
// teachers, creator inclusion, and teacher/supervisor overlap.
func validateCreateCircleRequest(creatorID string, req CreateCircleRequest) (string, string, map[string]string) {
	fields := make(map[string]string)
	backupSupervisorID := ""

	name := strings.TrimSpace(req.Name)
	if length := utf8.RuneCountInString(name); length < circleNameMinLen || length > circleNameMaxLen {
		fields[httpconst.FieldName] = httpconst.ErrorMessageCircleNameInvalid
	}

	if len(req.TeacherUserIDs) > 0 {
		fields[httpconst.FieldTeacherUserIDs] = httpconst.ErrorMessageTeacherConsentRequired
	}

	if req.BackupSupervisorUserID != nil {
		backupID := strings.TrimSpace(*req.BackupSupervisorUserID)
		switch {
		case !isUUID(backupID):
			fields[httpconst.FieldBackupSupervisor] = httpconst.ErrorMessageCircleAssigneeUnknown
		case backupID == creatorID:
			fields[httpconst.FieldBackupSupervisor] = httpconst.ErrorMessageCircleAssigneeConflict
		default:
			backupSupervisorID = backupID
		}
	}

	return name, backupSupervisorID, fields
}

// validateBackupSupervisorExists rejects an unknown optional supervisor.
func (s *Service) validateBackupSupervisorExists(ctx context.Context, backupID string) (map[string]string, error) {
	fields := make(map[string]string)
	if backupID == "" {
		return fields, nil
	}

	existing, err := s.store.UsersExist(ctx, []string{backupID})
	if err != nil {
		return nil, fmt.Errorf("query assignee existence: %w", err)
	}

	if !existing[backupID] {
		fields[httpconst.FieldBackupSupervisor] = httpconst.ErrorMessageCircleAssigneeUnknown
	}
	return fields, nil
}

func memberRole(members []Member, userID string) (string, bool) {
	for _, member := range members {
		if member.UserID == userID {
			return member.Role, true
		}
	}
	return "", false
}

func countTeachers(members []Member) int {
	count := 0
	for _, member := range members {
		if member.Role == RoleTeacher {
			count++
		}
	}
	return count
}

func countStudents(members []Member) int {
	count := 0
	for _, member := range members {
		if member.Role == RoleStudent {
			count++
		}
	}
	return count
}

func generateInviteCode() (string, error) {
	buf := make([]byte, inviteCodeLength)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("generate invite code: %w", err)
	}
	var code strings.Builder
	code.WriteString(inviteCodePrefix)
	for _, b := range buf {
		code.WriteByte(inviteCodeAlphabet[int(b)&31])
	}
	return code.String(), nil
}

// legacyInviteCodeLength is the symbol count of codes generated before the
// 12-symbol format; existing circles keep these codes until refreshed.
const legacyInviteCodeLength = 4

func isInviteCode(value string) bool {
	symbolCount := len(value) - len(inviteCodePrefix)
	if symbolCount != inviteCodeLength && symbolCount != legacyInviteCodeLength {
		return false
	}
	if !strings.HasPrefix(value, inviteCodePrefix) {
		return false
	}
	for _, char := range value[len(inviteCodePrefix):] {
		if !strings.ContainsRune(inviteCodeAlphabet, char) {
			return false
		}
	}
	return true
}

func isUUID(value string) bool {
	_, err := uuid.Parse(value)
	return err == nil
}

// GetCircle returns full circle details to authorized members.
func (s *Service) GetCircle(ctx context.Context, userID, circleID string) (CircleResponse, error) {
	if !isUUID(circleID) {
		return CircleResponse{}, &ValidationError{Fields: map[string]string{httpconst.FieldCircleID: httpconst.ErrorMessageCircleIDInvalid}}
	}

	circle, err := s.store.FindCircleByID(ctx, circleID)
	if err != nil {
		return CircleResponse{}, fmt.Errorf("get circle: find circle: %w", err)
	}

	ok, err := s.store.IsMember(ctx, circleID, userID)
	if err != nil {
		return CircleResponse{}, fmt.Errorf("get circle: check membership: %w", err)
	}
	if !ok {
		return CircleResponse{}, ErrForbidden
	}

	return circleResponse(circle), nil
}

// ListMembers returns the circle's member list to authorized members.
func (s *Service) ListMembers(ctx context.Context, userID, circleID string) ([]CircleMember, error) {
	if !isUUID(circleID) {
		return nil, &ValidationError{Fields: map[string]string{httpconst.FieldCircleID: httpconst.ErrorMessageCircleIDInvalid}}
	}

	_, err := s.store.FindCircleByID(ctx, circleID)
	if err != nil {
		return nil, fmt.Errorf("list members: find circle: %w", err)
	}

	ok, err := s.store.IsMember(ctx, circleID, userID)
	if err != nil {
		return nil, fmt.Errorf("list members: check membership: %w", err)
	}
	if !ok {
		return nil, ErrForbidden
	}

	members, err := s.store.ListMembers(ctx, circleID)
	if err != nil {
		return nil, fmt.Errorf("list members: query members: %w", err)
	}
	return members, nil
}
