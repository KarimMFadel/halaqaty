// Package realtime provides the domain-generic types for Halaqaty's
// authenticated realtime transport: short-lived connection tickets,
// validated subscription topics, and connection states. The package is
// shared transport only — it is independent of any feature domain and of the
// media provider; ticket issuance, authorization, and the WebSocket hub
// itself live in later layers.
package realtime

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

// TicketTTL is how long an issued realtime ticket stays valid, per the
// canonical WebSocket contract (60 seconds).
const TicketTTL = 60 * time.Second

// Queue event names registered on the shared session-topic transport.
const (
	EventQueueState         = "queue.state"
	EventQueueRoundStarted  = "queue.round_started"
	EventQueueReordered     = "queue.reordered"
	EventQueueAdvanced      = "queue.advanced"
	EventQueueEntryUpdated  = "queue.entry_updated"
	EventQueuePolicyChanged = "queue.policy_changed"
	EventQueueYourTurn        = "queue.your_turn"
	EventQueueNextSoon        = "queue.next_soon"
	EventQueueOptOutRequested = "queue.opt_out_requested"
)

// SessionEventProvider supplies one already-authorized event after a session
// topic subscription succeeds. A nil event means the provider has no state for
// that subscription.
type SessionEventProvider func(context.Context, string, string) (map[string]any, error)

// TopicKind distinguishes realtime subscription scopes.
type TopicKind string

const (
	// TopicCircle scopes events to the eligible members of one circle.
	// Circle topics are covered by the generic realtime ticket and may be
	// reused by features such as chat without an active live session.
	TopicCircle TopicKind = "circle"

	// TopicSession scopes events to the joined participants of one live
	// session. Session topics are granted only by the hub after a
	// successful authorized session join (FR-012); they are never part of
	// a generic ticket.
	TopicSession TopicKind = "session"
)

// Topic identifies one authorized realtime subscription target. Topics are
// validated values: the kind must be known and the identifier must be a UUID.
// The wire form is "kind.uuid", for example "circle.00000000-...". Topic
// values carry no authorization by themselves — the hub enforces who may
// subscribe (session topics additionally require a successful join).
type Topic struct {
	kind TopicKind
	id   string
}

// NewCircleTopic builds a validated circle-scoped topic.
func NewCircleTopic(circleID string) (Topic, error) {
	return newTopic(TopicCircle, circleID)
}

// NewSessionTopic builds a validated session-scoped topic.
func NewSessionTopic(sessionID string) (Topic, error) {
	return newTopic(TopicSession, sessionID)
}

// ParseTopic parses the "kind.uuid" wire form into a validated Topic.
func ParseTopic(raw string) (Topic, error) {
	kind, id, found := strings.Cut(raw, ".")
	if !found {
		return Topic{}, fmt.Errorf("realtime topic %q must have form kind.uuid", raw)
	}
	return newTopic(TopicKind(kind), id)
}

func newTopic(kind TopicKind, id string) (Topic, error) {
	if kind != TopicCircle && kind != TopicSession {
		return Topic{}, fmt.Errorf("realtime topic kind %q is not supported", kind)
	}
	if _, err := uuid.Parse(id); err != nil {
		return Topic{}, fmt.Errorf("realtime topic identifier %q is not a UUID: %w", id, err)
	}
	return Topic{kind: kind, id: id}, nil
}

// Kind reports the topic scope.
func (t Topic) Kind() TopicKind { return t.kind }

// ID reports the topic's circle or session identifier.
func (t Topic) ID() string { return t.id }

// String returns the "kind.uuid" wire form.
func (t Topic) String() string { return string(t.kind) + "." + t.id }

// Ticket is a short-lived authenticated authorization covering the caller's
// currently eligible circle topics only. Per FR-012 it never carries session
// topics (those are added by the hub after an authorized join) and it never
// carries media credentials.
type Ticket struct {
	Token     string
	UserID    string
	CircleIDs []string
	ExpiresAt time.Time
}

// ExpiredAt reports whether the ticket is no longer valid at the given time.
func (t Ticket) ExpiredAt(now time.Time) bool {
	return !now.Before(t.ExpiresAt)
}

// Covers reports whether the ticket authorizes the given topic. It is true
// only for circle topics whose identifier is in the ticket; session topics
// always require a separate hub-granted subscription.
func (t Ticket) Covers(topic Topic) bool {
	if topic.Kind() != TopicCircle {
		return false
	}
	for _, id := range t.CircleIDs {
		if id == topic.ID() {
			return true
		}
	}
	return false
}

// ConnectionState describes the lifecycle stage of one client realtime
// connection, mirroring the reconnect semantics in the canonical WebSocket
// contract (a connection is dead after three missed pongs, then reconnects).
type ConnectionState string

const (
	// ConnectionConnecting is the initial handshake state.
	ConnectionConnecting ConnectionState = "connecting"
	// ConnectionConnected means the transport is live and authorized.
	ConnectionConnected ConnectionState = "connected"
	// ConnectionReconnecting means the transport was lost and recovery is
	// in progress (for example after three missed pongs).
	ConnectionReconnecting ConnectionState = "reconnecting"
	// ConnectionDisconnected means the transport dropped without an
	// explicit close; recovery may still be attempted.
	ConnectionDisconnected ConnectionState = "disconnected"
	// ConnectionClosed is the terminal state; no further recovery.
	ConnectionClosed ConnectionState = "closed"
)
