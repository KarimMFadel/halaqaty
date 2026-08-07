package logging

import (
	"context"
	"log/slog"
	"strings"
	"time"
)

// Audit actions are standardized security-relevant event names.
const (
	ActionRegister      = "user.register"
	ActionSessionCreate = "session.create"
	ActionLogout        = "session.logout"
	ActionProfileUpdate = "profile.update"
	ActionCircleCreate  = "circle.create"
	ActionCircleJoin    = "circle.join"
	ActionRoleChange    = "circle.role_change"
	ActionInviteRefresh = "circle.invite_refresh"
	ActionMemberRemoval = "circle.member_remove"
	ActionCircleArchive = "circle.archive"
)

// AuditEvent captures security-relevant state transitions.
type AuditEvent struct {
	Action      string
	ActorUserID string
	TargetUser  string
	CircleID    string
	Metadata    map[string]any
}

// AuditLogger emits structured logs for auth/profile/role-change events.
type AuditLogger struct {
	logger *slog.Logger
	nowFn  func() time.Time
}

// NewAuditLogger creates a structured audit logger.
func NewAuditLogger(logger *slog.Logger) *AuditLogger {
	if logger == nil {
		logger = slog.Default()
	}
	return &AuditLogger{
		logger: logger,
		nowFn:  time.Now,
	}
}

// Log writes one audit event.
func (l *AuditLogger) Log(ctx context.Context, event AuditEvent) {
	event.Metadata = sanitizeAuditMetadata(event.Metadata)
	l.logger.InfoContext(
		ctx,
		"audit_event",
		slog.String("action", event.Action),
		slog.String("actor_user_id", event.ActorUserID),
		slog.String("target_user_id", event.TargetUser),
		slog.String("circle_id", event.CircleID),
		slog.Any("metadata", event.Metadata),
		slog.Time("at", l.nowFn().UTC()),
	)
}

func sanitizeAuditMetadata(metadata map[string]any) map[string]any {
	if len(metadata) == 0 {
		return nil
	}
	clean := make(map[string]any, len(metadata))
	for key, value := range metadata {
		switch strings.ToLower(key) {
		case "invite_code", "token", "access_token", "authorization", "session_id", "password":
			continue
		default:
			clean[key] = sanitizeAuditValue(value)
		}
	}
	return clean
}

func sanitizeAuditValue(value any) any {
	switch value := value.(type) {
	case map[string]any:
		return sanitizeAuditMetadata(value)
	case []any:
		clean := make([]any, len(value))
		for i, item := range value {
			clean[i] = sanitizeAuditValue(item)
		}
		return clean
	default:
		return value
	}
}

// RegisterEvent builds a user registration audit event.
func RegisterEvent(actorUserID, email string) AuditEvent {
	return AuditEvent{
		Action:      ActionRegister,
		ActorUserID: actorUserID,
		Metadata:    map[string]any{"email": email},
	}
}

// SessionCreateEvent builds a backend session creation audit event.
func SessionCreateEvent(actorUserID, sessionID, deviceName string) AuditEvent {
	metadata := map[string]any{"session_id": sessionID}
	if deviceName != "" {
		metadata["device_name"] = deviceName
	}
	return AuditEvent{
		Action:      ActionSessionCreate,
		ActorUserID: actorUserID,
		Metadata:    metadata,
	}
}

// LogoutEvent builds a session logout/revocation audit event.
func LogoutEvent(actorUserID, sessionID string) AuditEvent {
	return AuditEvent{
		Action:      ActionLogout,
		ActorUserID: actorUserID,
		Metadata:    map[string]any{"session_id": sessionID},
	}
}

// ProfileUpdateEvent builds a profile update audit event.
func ProfileUpdateEvent(actorUserID string, changedFields []string) AuditEvent {
	return AuditEvent{
		Action:      ActionProfileUpdate,
		ActorUserID: actorUserID,
		Metadata:    map[string]any{"changed_fields": changedFields},
	}
}

// CircleCreateEvent builds a circle creation audit event.
func CircleCreateEvent(actorUserID, circleID string, teacherCount int, hasSupervisor bool) AuditEvent {
	metadata := map[string]any{
		"circle_id":     circleID,
		"teacher_count": teacherCount,
	}
	if hasSupervisor {
		metadata["has_supervisor"] = true
	}
	return AuditEvent{
		Action:      ActionCircleCreate,
		ActorUserID: actorUserID,
		Metadata:    metadata,
	}
}

// RoleChangeEvent builds a circle role change audit event.
func RoleChangeEvent(actorUserID, targetUserID, circleID, oldRole, newRole string) AuditEvent {
	return AuditEvent{
		Action:      ActionRoleChange,
		ActorUserID: actorUserID,
		TargetUser:  targetUserID,
		CircleID:    circleID,
		Metadata: map[string]any{
			"old_role": oldRole,
			"new_role": newRole,
		},
	}
}

// CircleJoinEvent builds a circle-join audit event without logging invite data.
func CircleJoinEvent(actorUserID, circleID string) AuditEvent {
	return AuditEvent{Action: ActionCircleJoin, ActorUserID: actorUserID, CircleID: circleID}
}

// InviteRefreshEvent builds an invite-refresh audit event without logging the code.
func InviteRefreshEvent(actorUserID, circleID string) AuditEvent {
	return AuditEvent{Action: ActionInviteRefresh, ActorUserID: actorUserID, CircleID: circleID}
}

// MemberRemovalEvent builds a member-removal audit event.
func MemberRemovalEvent(actorUserID, targetUserID, circleID string) AuditEvent {
	return AuditEvent{Action: ActionMemberRemoval, ActorUserID: actorUserID, TargetUser: targetUserID, CircleID: circleID}
}

// CircleArchiveEvent builds a circle-archive audit event.
func CircleArchiveEvent(actorUserID, circleID string) AuditEvent {
	return AuditEvent{Action: ActionCircleArchive, ActorUserID: actorUserID, CircleID: circleID}
}
