package chat

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/KarimMFadel/halaqaty/backend/internal/realtime"
)

// RealtimeProjector projects committed group chat events to currently
// authorized realtime clients through the shared F-005 hub. The chat package
// imports realtime only; realtime never imports chat.
type RealtimeProjector struct {
	membership   MembershipReader
	hub          *realtime.Hub
	tickets      *realtime.TicketService
	sessionValid func(context.Context, string, string) (bool, error)
}

type SessionValidator func(context.Context, string, string) (bool, error)

// NewRealtimeProjector constructs the group chat.message projector. membership
// is reconsulted from PostgreSQL immediately before every client write
// (FR-010, SR-002), so removed members and stale sessions never receive later
// events even if an earlier audience snapshot included them.
func NewRealtimeProjector(membership MembershipReader, hub *realtime.Hub, tickets *realtime.TicketService, validators ...SessionValidator) *RealtimeProjector {
	var validator SessionValidator
	if len(validators) > 0 {
		validator = validators[0]
	}
	return &RealtimeProjector{membership: membership, hub: hub, tickets: tickets, sessionValid: validator}
}

// ProjectMessage projects one reloaded group chat.message event through the
// circle topic. The hub rebuilds the audience from current subscribers and
// invokes the per-write authorizer immediately before each socket write;
// duplicate event IDs are delivered effectively once per topic.
func (p *RealtimeProjector) ProjectMessage(ctx context.Context, event OutboxEvent, msg Message) error {
	if p == nil || p.hub == nil || p.membership == nil || p.tickets == nil {
		return errors.New("chat realtime projector is not configured")
	}
	if event.EventType != realtime.EventChatMessage {
		return fmt.Errorf("project chat event %q: unsupported event type", event.EventType)
	}
	if msg.CircleID == nil {
		return errors.New("project chat message: group projection requires a circle-scoped message")
	}
	topic, err := realtime.NewCircleTopic(msg.CircleID.String())
	if err != nil {
		return fmt.Errorf("build chat circle topic: %w", err)
	}
	eventID := event.EventID.String()
	envelope := map[string]any{
		"type":        realtime.EventChatMessage,
		"event_id":    eventID,
		"occurred_at": msg.SentAt.UTC().Format(time.RFC3339Nano),
		"payload":     chatMessagePayload(msg),
	}
	delivery := realtime.AuthorizedDelivery{EventID: eventID, Payload: envelope, Authorize: p.authorizeCircleDelivery(*msg.CircleID, msg.SentAt)}
	if err := p.hub.BroadcastAuthorized(ctx, topic, delivery); err != nil {
		return fmt.Errorf("broadcast chat message: %w", err)
	}
	return nil
}

// authorizeCircleDelivery builds the per-write authorizer for one circle: the
// connection's realtime ticket must still validate for its user, and the user
// must remain a current member of the circle. A failed check suppresses the
// write; an authorization error aborts delivery so the outbox retries.
func (p *RealtimeProjector) authorizeCircleDelivery(circleID uuid.UUID, sentAt time.Time) realtime.DeliveryAuthorizer {
	return func(ctx context.Context, connection realtime.ConnectionIdentity) (bool, error) {
		if _, err := p.tickets.Validate(connection.RealtimeTicket, connection.UserID); err != nil {
			return false, nil
		}
		if p.sessionValid == nil {
			return false, nil
		}
		ticket, err := p.tickets.Validate(connection.RealtimeTicket, connection.UserID)
		if err != nil || ticket.SessionID == "" {
			return false, nil
		}
		valid, err := p.sessionValid(ctx, ticket.SessionID, connection.UserID)
		if err != nil {
			return false, fmt.Errorf("reauthorize backend session: %w", err)
		}
		if !valid {
			return false, nil
		}
		member, err := p.membership.IsMember(ctx, circleID.String(), connection.UserID)
		if err != nil {
			return false, fmt.Errorf("reauthorize chat circle delivery: %w", err)
		}
		if !member {
			return false, nil
		}
		periods, ok := p.membership.(membershipPeriodReader)
		if !ok {
			return false, nil
		}
		joined, err := periods.MembershipStartedAt(ctx, circleID.String(), connection.UserID)
		if err != nil {
			return false, fmt.Errorf("load chat membership period: %w", err)
		}
		if sentAt.Before(joined) {
			return false, nil
		}
		return true, nil
	}
}

// chatMessagePayload builds the redacted canonical REST Message projection
// carried by chat.message: identifiers, server-authoritative timestamps, and
// state only — never object keys, URLs, or credentials (SR-006).
func chatMessagePayload(msg Message) map[string]any {
	payload := map[string]any{
		"id":              msg.ID.String(),
		"circle_id":       msg.CircleID.String(),
		"sender_id":       msg.SenderID.String(),
		"message_type":    string(msg.Type),
		"sent_at":         msg.SentAt.UTC().Format(time.RFC3339Nano),
		"delivery_status": string(DeliveryStatusDelivered),
	}
	if msg.Type == MessageTypeText {
		payload["content"] = msg.Content
	}
	return payload
}
