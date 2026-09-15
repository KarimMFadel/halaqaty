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
	dmEligible   func(context.Context, uuid.UUID, uuid.UUID) (bool, error)
	readReceipt  func(context.Context, uuid.UUID, uuid.UUID) (MessageRead, error)
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

// SetDMEligibilityChecker supplies the current PostgreSQL-backed DM
// relationship check used immediately before every direct-user write.
func (p *RealtimeProjector) SetDMEligibilityChecker(checker func(context.Context, uuid.UUID, uuid.UUID) (bool, error)) {
	p.dmEligible = checker
}

// SetReadReceiptLoader supplies the authoritative read fact for a targeted
// read event. The outbox stores the reader ID in RecipientID for this event;
// the projector derives the sender target from the message itself.
func (p *RealtimeProjector) SetReadReceiptLoader(loader func(context.Context, uuid.UUID, uuid.UUID) (MessageRead, error)) {
	p.readReceipt = loader
}

// ProjectMessage projects one reloaded group chat.message event through the
// circle topic. The hub rebuilds the audience from current subscribers and
// invokes the per-write authorizer immediately before each socket write;
// duplicate event IDs are delivered effectively once per topic.
func (p *RealtimeProjector) ProjectMessage(ctx context.Context, event OutboxEvent, msg Message) error {
	if p == nil || p.hub == nil || p.membership == nil || p.tickets == nil {
		return errors.New("chat realtime projector is not configured")
	}
	if event.EventType == realtime.EventChatMessageRead {
		return p.projectRead(ctx, event, msg)
	}
	if event.EventType == realtime.EventChatMessageDeleted {
		return p.projectDeletedMessage(ctx, event, msg)
	}
	if event.EventType != realtime.EventChatMessage {
		return fmt.Errorf("project chat event %q: unsupported event type", event.EventType)
	}
	if msg.DMRecipientID != nil {
		if p.dmEligible == nil {
			return errors.New("project direct chat message: eligibility checker is not configured")
		}
		return p.projectDirectMessage(ctx, event, msg)
	}
	if msg.CircleID == nil {
		return errors.New("project chat message: message has no conversation context")
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

func (p *RealtimeProjector) projectDeletedMessage(ctx context.Context, event OutboxEvent, msg Message) error {
	if msg.DeletedAt == nil {
		return errors.New("project deleted chat message: missing deletion time")
	}
	eventID := event.EventID.String()
	payload := map[string]any{"message_id": msg.ID.String(), "circle_id": nil, "dm_peer_id": nil, "deleted_at": msg.DeletedAt.UTC().Format(time.RFC3339Nano)}
	envelope := map[string]any{"type": realtime.EventChatMessageDeleted, "event_id": eventID, "occurred_at": msg.DeletedAt.UTC().Format(time.RFC3339Nano), "payload": payload}
	if msg.CircleID != nil {
		payload["circle_id"] = msg.CircleID.String()
		topic, err := realtime.NewCircleTopic(msg.CircleID.String())
		if err != nil {
			return fmt.Errorf("build deleted chat circle topic: %w", err)
		}
		return p.hub.BroadcastAuthorized(ctx, topic, realtime.AuthorizedDelivery{EventID: eventID, Payload: envelope, Authorize: p.authorizeCircleDelivery(*msg.CircleID, msg.SentAt)})
	}
	if msg.DMRecipientID == nil || p.dmEligible == nil {
		return errors.New("project deleted direct message: invalid context")
	}
	payload["dm_peer_id"] = msg.DMRecipientID.String()
	return p.hub.SendToUsers(ctx, []string{msg.SenderID.String(), msg.DMRecipientID.String()}, realtime.AuthorizedDelivery{EventID: eventID, Payload: envelope, Authorize: func(ctx context.Context, connection realtime.ConnectionIdentity) (bool, error) {
		if connection.UserID != msg.SenderID.String() && connection.UserID != msg.DMRecipientID.String() {
			return false, nil
		}
		allowed, err := p.authorizeSession(ctx, connection)
		if err != nil || !allowed {
			return allowed, err
		}
		viewer, err := uuid.Parse(connection.UserID)
		if err != nil {
			return false, nil
		}
		other := msg.SenderID
		if viewer == other {
			other = *msg.DMRecipientID
		}
		return p.dmEligible(ctx, viewer, other)
	}})
}

func (p *RealtimeProjector) projectRead(ctx context.Context, event OutboxEvent, msg Message) error {
	if event.RecipientID == nil || p.readReceipt == nil {
		return errors.New("project chat read event: receipt is not configured")
	}
	receipt, err := p.readReceipt(ctx, msg.ID, *event.RecipientID)
	if err != nil {
		return fmt.Errorf("load chat read receipt: %w", err)
	}
	eventID := event.EventID.String()
	envelope := map[string]any{
		"type":        realtime.EventChatMessageRead,
		"event_id":    eventID,
		"occurred_at": receipt.ReadAt.UTC().Format(time.RFC3339Nano),
		"payload": map[string]any{
			"message_id": msg.ID.String(),
			"reader_id":  receipt.UserID.String(),
			"read_at":    receipt.ReadAt.UTC().Format(time.RFC3339Nano),
		},
	}
	delivery := realtime.AuthorizedDelivery{EventID: eventID, Payload: envelope, Authorize: p.authorizeReadDelivery(msg)}
	if err := p.hub.SendToUsers(ctx, []string{msg.SenderID.String()}, delivery); err != nil {
		return fmt.Errorf("send chat read event: %w", err)
	}
	return nil
}

func (p *RealtimeProjector) authorizeReadDelivery(msg Message) realtime.DeliveryAuthorizer {
	if msg.CircleID != nil {
		return p.authorizeCircleDelivery(*msg.CircleID, msg.SentAt)
	}
	return func(ctx context.Context, connection realtime.ConnectionIdentity) (bool, error) {
		if p.dmEligible == nil || connection.UserID != msg.SenderID.String() {
			return false, nil
		}
		allowed, err := p.authorizeSession(ctx, connection)
		if err != nil || !allowed {
			return allowed, err
		}
		return p.dmEligible(ctx, msg.SenderID, *msg.DMRecipientID)
	}
}

func (p *RealtimeProjector) projectDirectMessage(ctx context.Context, event OutboxEvent, msg Message) error {
	eventID := event.EventID.String()
	envelope := map[string]any{
		"type":        realtime.EventChatMessage,
		"event_id":    eventID,
		"occurred_at": msg.SentAt.UTC().Format(time.RFC3339Nano),
		"payload":     chatMessagePayload(msg),
	}
	users := []string{msg.SenderID.String(), msg.DMRecipientID.String()}
	if err := p.hub.SendToUsers(ctx, users, realtime.AuthorizedDelivery{
		EventID: eventID,
		Payload: envelope,
		Authorize: func(ctx context.Context, connection realtime.ConnectionIdentity) (bool, error) {
			if connection.UserID != msg.SenderID.String() && connection.UserID != msg.DMRecipientID.String() {
				return false, nil
			}
			allowed, err := p.authorizeSession(ctx, connection)
			if err != nil || !allowed {
				return allowed, err
			}
			viewerID, err := uuid.Parse(connection.UserID)
			if err != nil {
				return false, nil
			}
			otherID := msg.SenderID
			if viewerID == otherID {
				otherID = *msg.DMRecipientID
			}
			return p.dmEligible(ctx, viewerID, otherID)
		},
	}); err != nil {
		return fmt.Errorf("send direct chat message: %w", err)
	}
	return nil
}

func (p *RealtimeProjector) authorizeSession(ctx context.Context, connection realtime.ConnectionIdentity) (bool, error) {
	ticket, err := p.tickets.Validate(connection.RealtimeTicket, connection.UserID)
	if err != nil || p.sessionValid == nil || ticket.SessionID == "" {
		return false, nil
	}
	return p.sessionValid(ctx, ticket.SessionID, connection.UserID)
}

func (p *RealtimeProjector) authorizeTypingCircle(circleID uuid.UUID) realtime.DeliveryAuthorizer {
	return func(ctx context.Context, connection realtime.ConnectionIdentity) (bool, error) {
		allowed, err := p.authorizeSession(ctx, connection)
		if err != nil || !allowed {
			return allowed, err
		}
		circle, err := p.membership.FindCircleByID(ctx, circleID.String())
		if errorsIsCircleNotFound(err) || (err == nil && circle.IsArchived) {
			return false, nil
		}
		if err != nil {
			return false, fmt.Errorf("reauthorize typing circle: %w", err)
		}
		return p.membership.IsMember(ctx, circleID.String(), connection.UserID)
	}
}

func (p *RealtimeProjector) authorizeTypingDirect(peerID uuid.UUID) realtime.DeliveryAuthorizer {
	return func(ctx context.Context, connection realtime.ConnectionIdentity) (bool, error) {
		if p.dmEligible == nil {
			return false, nil
		}
		allowed, err := p.authorizeSession(ctx, connection)
		if err != nil || !allowed {
			return allowed, err
		}
		userID, err := uuid.Parse(connection.UserID)
		if err != nil {
			return false, nil
		}
		return p.dmEligible(ctx, userID, peerID)
	}
}

// authorizeCircleDelivery builds the per-write authorizer for one circle: the
// connection's realtime ticket must still validate for its user, and the user
// must remain a current member of the circle. A failed check suppresses the
// write; an authorization error aborts delivery so the outbox retries.
func (p *RealtimeProjector) authorizeCircleDelivery(circleID uuid.UUID, sentAt time.Time) realtime.DeliveryAuthorizer {
	return func(ctx context.Context, connection realtime.ConnectionIdentity) (bool, error) {
		// One ticket validation per write serves both the eligibility check
		// and the session lookup; validating twice doubled every write's
		// ticket work for no additional guarantee.
		ticket, err := p.tickets.Validate(connection.RealtimeTicket, connection.UserID)
		if err != nil {
			return false, nil
		}
		if p.sessionValid == nil {
			return false, nil
		}
		if ticket.SessionID == "" {
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
		"circle_id":       nil,
		"sender_id":       msg.SenderID.String(),
		"message_type":    string(msg.Type),
		"sent_at":         msg.SentAt.UTC().Format(time.RFC3339Nano),
		"delivery_status": string(DeliveryStatusDelivered),
	}
	if msg.CircleID != nil {
		payload["circle_id"] = msg.CircleID.String()
	}
	if msg.DMRecipientID != nil {
		payload["dm_peer_id"] = msg.DMRecipientID.String()
	}
	if msg.Type == MessageTypeText {
		payload["content"] = msg.Content
	}
	return payload
}
