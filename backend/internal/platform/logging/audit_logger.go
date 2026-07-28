package logging

import (
	"context"
	"log/slog"
	"time"
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
