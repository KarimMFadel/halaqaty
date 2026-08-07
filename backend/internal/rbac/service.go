package rbac

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/google/uuid"

	"github.com/KarimMFadel/halaqaty/backend/internal/platform/httpconst"
	"github.com/KarimMFadel/halaqaty/backend/internal/platform/logging"
)

const (
	circleNameMinLen = 2
	circleNameMaxLen = 100
	// inviteCodeAlphabet excludes visually ambiguous characters; 32 symbols
	// divide 256 exactly, so byte-to-symbol mapping stays uniform.
	inviteCodeAlphabet = "ABCDEFGHJKLMNPQRSTUVWXYZ23456789"
	inviteCodeLength   = 6
	inviteCodePrefix   = "HLQ-"
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
	ErrAlreadyMember = errors.New("user is already a circle member")
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
	InsertCircle(ctx context.Context, name, ownerID, inviteCode string) (Circle, error)
	InsertMember(ctx context.Context, circleID, userID, role string) error
	FindCircleByInviteCode(ctx context.Context, inviteCode string) (Circle, error)
	CircleExists(ctx context.Context, circleID string) (bool, error)
	LockMembers(ctx context.Context, circleID string) ([]Member, error)
	UpdateMemberRole(ctx context.Context, circleID, userID, role string) error
}

// JoinCircle adds the authenticated user as a student using a circle invite code.
func (s *Service) JoinCircle(ctx context.Context, userID, inviteCode string) (CircleResponse, error) {
	inviteCode = strings.ToUpper(strings.TrimSpace(inviteCode))
	if !isInviteCode(inviteCode) {
		return CircleResponse{}, &ValidationError{Fields: map[string]string{httpconst.FieldInviteCode: httpconst.ErrorMessageInviteCodeInvalid}}
	}

	var circle Circle
	err := s.store.WithinTransaction(ctx, func(tx Store) error {
		found, err := tx.FindCircleByInviteCode(ctx, inviteCode)
		if err != nil {
			return err
		}
		members, err := tx.LockMembers(ctx, found.ID)
		if err != nil {
			return err
		}
		if _, exists := memberRole(members, userID); exists {
			return ErrAlreadyMember
		}
		if err := tx.InsertMember(ctx, found.ID, userID, RoleStudent); err != nil {
			return err
		}
		circle = found
		return nil
	})
	if err != nil {
		return CircleResponse{}, fmt.Errorf("join circle: %w", err)
	}

	return CircleResponse{ID: circle.ID, Name: circle.Name, InviteCode: circle.InviteCode, CreatedAt: circle.CreatedAt}, nil
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

// CreateCircle creates a circle and its initial memberships in one transaction.
func (s *Service) CreateCircle(ctx context.Context, creatorID string, req CreateCircleRequest) (CircleResponse, error) {
	name, teacherIDs, backupSupervisorID, fields := validateCreateCircleRequest(creatorID, req)

	if len(fields) == 0 {
		existenceFields, err := s.validateAssigneesExist(ctx, teacherIDs, backupSupervisorID)
		if err != nil {
			return CircleResponse{}, fmt.Errorf("validate circle assignees: %w", err)
		}
		fields = existenceFields
	}
	if len(fields) > 0 {
		return CircleResponse{}, &ValidationError{Fields: fields}
	}

	inviteCode, err := generateInviteCode()
	if err != nil {
		return CircleResponse{}, err
	}

	var circle Circle
	err = s.store.WithinTransaction(ctx, func(tx Store) error {
		created, err := tx.InsertCircle(ctx, name, creatorID, inviteCode)
		if err != nil {
			return err
		}
		circle = created

		for _, teacherID := range teacherIDs {
			if err := tx.InsertMember(ctx, circle.ID, teacherID, RoleTeacher); err != nil {
				return err
			}
		}
		if backupSupervisorID != "" {
			if err := tx.InsertMember(ctx, circle.ID, backupSupervisorID, RoleSupervisor); err != nil {
				return err
			}
		}
		creatorRole := RoleSupervisor
		if len(teacherIDs) == 0 {
			creatorRole = RoleTeacher
		}
		return tx.InsertMember(ctx, circle.ID, creatorID, creatorRole)
	})
	if err != nil {
		return CircleResponse{}, fmt.Errorf("create circle: %w", err)
	}

	teacherCount := len(teacherIDs)
	if teacherCount == 0 {
		teacherCount = 1
	}
	s.audit.Log(ctx, logging.CircleCreateEvent(creatorID, circle.ID, teacherCount, len(teacherIDs) > 0 || backupSupervisorID != ""))

	return CircleResponse{
		ID:         circle.ID,
		Name:       circle.Name,
		InviteCode: circle.InviteCode,
		CreatedAt:  circle.CreatedAt,
	}, nil
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
		exists, err := tx.CircleExists(ctx, circleID)
		if err != nil {
			return err
		}
		if !exists {
			return ErrCircleNotFound
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

		if targetRole == RoleTeacher && role != RoleTeacher && countTeachers(members) <= 1 {
			return ErrFinalTeacher
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

// AddStudentMember inserts a student membership idempotently (invite acceptance).
func (s *Service) AddStudentMember(ctx context.Context, circleID, userID string) error {
	if !isUUID(circleID) {
		return fmt.Errorf("add student member: %w", ErrCircleNotFound)
	}

	err := s.store.WithinTransaction(ctx, func(tx Store) error {
		exists, err := tx.CircleExists(ctx, circleID)
		if err != nil {
			return err
		}
		if !exists {
			return ErrCircleNotFound
		}
		return tx.InsertMember(ctx, circleID, userID, RoleStudent)
	})
	if err != nil {
		return fmt.Errorf("add student member: %w", err)
	}
	return nil
}

// validateCreateCircleRequest checks shape-level rules: name bounds, duplicate
// teachers, creator inclusion, and teacher/supervisor overlap.
func validateCreateCircleRequest(creatorID string, req CreateCircleRequest) (string, []string, string, map[string]string) {
	fields := make(map[string]string)
	backupSupervisorID := ""

	name := strings.TrimSpace(req.Name)
	if length := utf8.RuneCountInString(name); length < circleNameMinLen || length > circleNameMaxLen {
		fields[httpconst.FieldName] = httpconst.ErrorMessageCircleNameInvalid
	}

	seen := make(map[string]struct{}, len(req.TeacherUserIDs))
	teacherIDs := make([]string, 0, len(req.TeacherUserIDs))
	for _, rawID := range req.TeacherUserIDs {
		id := strings.TrimSpace(rawID)
		if !isUUID(id) {
			fields[httpconst.FieldTeacherUserIDs] = httpconst.ErrorMessageCircleAssigneeUnknown
			continue
		}
		if id == creatorID {
			fields[httpconst.FieldTeacherUserIDs] = httpconst.ErrorMessageCircleAssigneeConflict
			continue
		}
		if _, dup := seen[id]; dup {
			fields[httpconst.FieldTeacherUserIDs] = httpconst.ErrorMessageCircleAssigneeConflict
			continue
		}
		seen[id] = struct{}{}
		teacherIDs = append(teacherIDs, id)
	}

	if req.BackupSupervisorUserID != nil {
		backupID := strings.TrimSpace(*req.BackupSupervisorUserID)
		switch {
		case !isUUID(backupID):
			fields[httpconst.FieldBackupSupervisor] = httpconst.ErrorMessageCircleAssigneeUnknown
		case backupID == creatorID:
			fields[httpconst.FieldBackupSupervisor] = httpconst.ErrorMessageCircleAssigneeConflict
		default:
			if _, isTeacher := seen[backupID]; isTeacher {
				fields[httpconst.FieldBackupSupervisor] = httpconst.ErrorMessageCircleAssigneeConflict
			} else {
				backupSupervisorID = backupID
			}
		}
	}

	return name, teacherIDs, backupSupervisorID, fields
}

// validateAssigneesExist rejects assignees that are not registered users.
func (s *Service) validateAssigneesExist(ctx context.Context, teacherIDs []string, backupID string) (map[string]string, error) {
	fields := make(map[string]string)

	candidates := make([]string, 0, len(teacherIDs)+1)
	candidates = append(candidates, teacherIDs...)
	if backupID != "" {
		candidates = append(candidates, backupID)
	}
	if len(candidates) == 0 {
		return fields, nil
	}

	existing, err := s.store.UsersExist(ctx, candidates)
	if err != nil {
		return nil, fmt.Errorf("query assignee existence: %w", err)
	}

	for _, id := range teacherIDs {
		if !existing[id] {
			fields[httpconst.FieldTeacherUserIDs] = httpconst.ErrorMessageCircleAssigneeUnknown
			break
		}
	}
	if backupID != "" && !existing[backupID] {
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

func isInviteCode(value string) bool {
	if len(value) != len(inviteCodePrefix)+inviteCodeLength || !strings.HasPrefix(value, inviteCodePrefix) {
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
