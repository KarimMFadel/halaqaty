package livekit

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"time"

	"github.com/KarimMFadel/halaqaty/backend/internal/sessions"
	"github.com/livekit/protocol/auth"
	lkmodel "github.com/livekit/protocol/livekit"
	"github.com/livekit/protocol/webhook"
)

// EventType is the provider-neutral webhook translation consumed by the
// sessions domain; provider event names never leave this package.
type EventType string

// Neutral webhook event types F-005 consumes (US1 presence and room
// lifecycle). Anything else is rejected.
const (
	EventParticipantJoined EventType = "participant_joined"
	EventParticipantLeft   EventType = "participant_left"
	EventRoomFinished      EventType = "room_finished"
)

// neutralEvents maps provider event names to the neutral set.
var neutralEvents = map[string]EventType{
	"participant_joined": EventParticipantJoined,
	"participant_left":   EventParticipantLeft,
	"room_finished":      EventRoomFinished,
}

// WebhookEvent is the verified, provider-neutral translation of one LiveKit
// webhook delivery. ID is stable across duplicate deliveries so downstream
// processing can deduplicate (FR-016 at-least-once semantics).
type WebhookEvent struct {
	// ID is the deterministic dedup identifier of the delivery.
	ID string
	// Type is the neutral event type.
	Type EventType
	// RoomRef is the opaque media room reference the event belongs to.
	RoomRef sessions.MediaRoomRef
	// Identity is the participant user ID; empty for room events.
	Identity string
	// Timestamp is the provider event time (UTC).
	Timestamp time.Time
}

// WebhookVerifier verifies signed LiveKit webhook deliveries and translates
// them to neutral events. It holds only the configured key pair.
type WebhookVerifier struct {
	provider auth.KeyProvider
}

// NewWebhookVerifier constructs the verifier from the LiveKit API key pair.
func NewWebhookVerifier(apiKey, apiSecret string) *WebhookVerifier {
	return &WebhookVerifier{provider: auth.NewSimpleKeyProvider(apiKey, apiSecret)}
}

// Verify authenticates the delivery (signed Authorization token bound to the
// SHA-256 digest of the body) and returns its neutral translation. Room
// events must carry a room reference; participant events must additionally
// carry an identity. Unsupported events are rejected.
func (v *WebhookVerifier) Verify(r *http.Request) (WebhookEvent, error) {
	delivery, err := webhook.ReceiveWebhookEvent(r, v.provider)
	if err != nil {
		return WebhookEvent{}, fmt.Errorf("verify media webhook: %w", err)
	}
	eventType, ok := neutralEvents[delivery.Event]
	if !ok {
		return WebhookEvent{}, fmt.Errorf("verify media webhook: unsupported event %q", delivery.Event)
	}
	neutral := WebhookEvent{
		Type:      eventType,
		Timestamp: time.Unix(delivery.CreatedAt, 0).UTC(),
	}
	if delivery.Room == nil || delivery.Room.Name == "" {
		return WebhookEvent{}, fmt.Errorf("verify media webhook: missing room reference")
	}
	neutral.RoomRef = sessions.MediaRoomRef(delivery.Room.Name)
	if delivery.Participant != nil {
		if delivery.Participant.Identity == "" {
			return WebhookEvent{}, fmt.Errorf("verify media webhook: participant event without identity")
		}
		neutral.Identity = delivery.Participant.Identity
	} else if eventType != EventRoomFinished {
		return WebhookEvent{}, fmt.Errorf("verify media webhook: %s requires a participant", delivery.Event)
	}
	neutral.ID = webhookEventID(delivery)
	return neutral, nil
}

// webhookEventID derives the stable dedup identifier from the delivery's
// provider-visible identity: identical redeliveries hash identically, and
// distinct events do not collide.
func webhookEventID(delivery *lkmodel.WebhookEvent) string {
	sum := sha256.Sum256([]byte(fmt.Sprintf("%s|%s|%s|%d",
		delivery.Event, delivery.Room.Name, delivery.Participant.GetIdentity(), delivery.CreatedAt)))
	return hex.EncodeToString(sum[:])
}
