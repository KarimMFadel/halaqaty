package logging

import (
	"context"
	"log/slog"
	"strings"
)

// ChatAuditOutcome is the closed set of outcomes used by chat audit events.
type ChatAuditOutcome string

const (
	ChatOutcomeAccepted  ChatAuditOutcome = "accepted"
	ChatOutcomeRejected  ChatAuditOutcome = "rejected"
	ChatOutcomeDenied    ChatAuditOutcome = "denied"
	ChatOutcomeParked    ChatAuditOutcome = "parked"
	ChatOutcomeRecovered ChatAuditOutcome = "recovered"
	ChatOutcomeNoResults ChatAuditOutcome = "no_results"
)

const (
	ActionChatMessage   = "chat.message"
	ActionChatUpload    = "chat.upload"
	ActionChatOutbox    = "chat.outbox"
	ActionChatReconnect = "chat.reconnect"
	ActionChatSearch    = "chat.search"
	ActionChatDenied    = "chat.denied"
)

// ChatMessageAuditEvent builds a redacted message audit event.
func ChatMessageAuditEvent(actorUserID, circleID, messageID string, outcome ChatAuditOutcome) AuditEvent {
	return chatAuditEvent(ActionChatMessage, actorUserID, circleID, map[string]any{
		"message_id": messageID,
		"outcome":    string(outcome),
	})
}

// ChatUploadAuditEvent builds a redacted upload audit event.
func ChatUploadAuditEvent(actorUserID, circleID, uploadID string, outcome ChatAuditOutcome) AuditEvent {
	return chatAuditEvent(ActionChatUpload, actorUserID, circleID, map[string]any{
		"upload_id": uploadID,
		"outcome":   string(outcome),
	})
}

// ChatOutboxAuditEvent builds a redacted outbox audit event.
func ChatOutboxAuditEvent(actorUserID, eventID string, outcome ChatAuditOutcome) AuditEvent {
	return chatAuditEvent(ActionChatOutbox, actorUserID, "", map[string]any{
		"event_id": eventID,
		"outcome":  string(outcome),
	})
}

// ChatReconnectAuditEvent builds a redacted reconnect audit event.
func ChatReconnectAuditEvent(actorUserID, circleID string, outcome ChatAuditOutcome) AuditEvent {
	return chatAuditEvent(ActionChatReconnect, actorUserID, circleID, map[string]any{
		"outcome": string(outcome),
	})
}

// ChatSearchAuditEvent builds a redacted search audit event.
func ChatSearchAuditEvent(actorUserID, circleID string, outcome ChatAuditOutcome) AuditEvent {
	return chatAuditEvent(ActionChatSearch, actorUserID, circleID, map[string]any{
		"outcome": string(outcome),
	})
}

// ChatDenialAuditEvent builds a redacted authorization-denial event.
func ChatDenialAuditEvent(actorUserID, circleID, resourceID string, outcome ChatAuditOutcome) AuditEvent {
	return chatAuditEvent(ActionChatDenied, actorUserID, circleID, map[string]any{
		"resource_id": resourceID,
		"outcome":     string(outcome),
	})
}

// LogChat writes a chat audit event without the generic live-session field.
// Chat audit records intentionally never contain an auth or backend session ID.
func (l *AuditLogger) LogChat(ctx context.Context, event AuditEvent) {
	event.Metadata = sanitizeChatAuditMetadata(event.Metadata)
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

func chatAuditEvent(action, actorUserID, circleID string, metadata map[string]any) AuditEvent {
	return AuditEvent{
		Action:      action,
		ActorUserID: actorUserID,
		CircleID:    circleID,
		Metadata:    sanitizeChatAuditMetadata(metadata),
	}
}

func sanitizeChatAuditMetadata(metadata map[string]any) map[string]any {
	if len(metadata) == 0 {
		return nil
	}
	clean := make(map[string]any, len(metadata))
	for key, value := range metadata {
		switch strings.ToLower(key) {
		case "body", "message_body", "filename", "file_name", "object_key", "key", "url", "presigned_url", "token", "authorization", "credential", "auth_session_id", "backend_session_id", "session_id":
			continue
		default:
			clean[key] = sanitizeChatAuditValue(value)
		}
	}
	return clean
}

func sanitizeChatAuditValue(value any) any {
	switch value := value.(type) {
	case map[string]any:
		return sanitizeChatAuditMetadata(value)
	case []any:
		clean := make([]any, len(value))
		for i, item := range value {
			clean[i] = sanitizeChatAuditValue(item)
		}
		return clean
	default:
		return value
	}
}
