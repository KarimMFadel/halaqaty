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

	// Queue audit actions carry redacted metadata only: no note text, no grade
	// values, no student display names, no media/room references.
	ActionQueuePolicyChange    = "queue.policy_change"
	ActionQueueOptOutRequest   = "queue.opt_out_request"
	ActionQueueOptOutDecision  = "queue.opt_out_decision"
	ActionQueueGradeCorrection = "queue.grade_correction"
)

// AuditEvent captures security-relevant state transitions.
type AuditEvent struct {
	Action      string
	ActorUserID string
	TargetUser  string
	CircleID    string
	// SessionID is the live-session (halaqah) resource ID for queue events.
	// It is distinct from auth-session identifiers, which stay on the
	// metadata deny-list as credential material.
	SessionID string
	Metadata  map[string]any
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
		slog.String("session_id", event.SessionID),
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
		case "invite_code", "token", "access_token", "authorization", "session_id", "password",
			"note", "notes", "teacher_notes", "note_text", "grade", "grades", "grade_value",
			"media", "room", "room_ref", "media_room_ref", "credential", "media_credential",
			"endpoint", "provider", "provider_id", "provider_identifier", "url":
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

// QueuePolicyChangeEvent builds a queue policy-change audit event. Each
// changes entry maps a policy dimension to its {prior, current} values; values
// must be manager-controlled closed enums (never free text).
func QueuePolicyChangeEvent(actorUserID, sessionID string, changes map[string][2]string) AuditEvent {
	return AuditEvent{
		Action:      ActionQueuePolicyChange,
		ActorUserID: actorUserID,
		SessionID:   sessionID,
		Metadata:    map[string]any{"changes": changes},
	}
}

// QueueOptOutRequestEvent builds a queue opt-out request audit event.
func QueueOptOutRequestEvent(actorUserID, sessionID, entryID string) AuditEvent {
	return AuditEvent{
		Action:      ActionQueueOptOutRequest,
		ActorUserID: actorUserID,
		SessionID:   sessionID,
		Metadata:    map[string]any{"entry_id": entryID},
	}
}

// QueueOptOutDecisionEvent builds a queue opt-out decision audit event.
// decision must be a closed status enum (e.g. approved/declined/auto).
func QueueOptOutDecisionEvent(actorUserID, sessionID, requestID, decision string) AuditEvent {
	return AuditEvent{
		Action:      ActionQueueOptOutDecision,
		ActorUserID: actorUserID,
		SessionID:   sessionID,
		Metadata:    map[string]any{"request_id": requestID, "decision": decision},
	}
}

// GradeCorrectionShape records only the change shape of a grade/note
// correction — never note text or grade values.
type GradeCorrectionShape struct {
	// FieldsChanged names the corrected fields ("grade" and/or "note").
	FieldsChanged []string
	// GradeChanged reports whether the grade value was replaced.
	GradeChanged bool
	// NoteChanged reports whether the note was set or replaced.
	NoteChanged bool
	// NoteCleared reports whether the note was cleared to null.
	NoteCleared bool
	// PriorGradePresent reports whether a grade existed before the correction.
	PriorGradePresent bool
}

// QueueGradeCorrectionEvent builds a redacted grade/note correction audit
// event carrying only the change shape.
func QueueGradeCorrectionEvent(actorUserID, sessionID, entryID string, shape GradeCorrectionShape) AuditEvent {
	return AuditEvent{
		Action:      ActionQueueGradeCorrection,
		ActorUserID: actorUserID,
		SessionID:   sessionID,
		Metadata: map[string]any{
			"entry_id":            entryID,
			"fields_changed":      shape.FieldsChanged,
			"grade_changed":       shape.GradeChanged,
			"note_changed":        shape.NoteChanged,
			"note_cleared":        shape.NoteCleared,
			"prior_grade_present": shape.PriorGradePresent,
		},
	}
}
