package realtime

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

const (
	maxConnectionsPerUser     = 3
	maxMessagesPerMinute      = 30
	realtimeActionSubscribe   = "subscribe"
	realtimeTypePing          = "ping"
	realtimeTypePong          = "pong"
	realtimeTypeError         = "error"
	realtimeTypeSubscribed    = "subscribed"
	realtimeErrorRateLimit    = "RATE_LIMITED"
	realtimeErrorInvalid      = "INVALID_PAYLOAD"
	realtimeErrorUnauthorized = "UNAUTHORIZED"
	realtimeErrorSessionEnded = "SESSION_ENDED"
)

// SessionTopicAuthorizer checks whether a user may subscribe to a live
// session topic. Implementations must revalidate current membership/presence.
type SessionTopicAuthorizer interface {
	AuthorizeSessionTopic(context.Context, string, string) error
}

// SessionSnapshotProvider supplies an already-redacted snapshot after a
// participant is authorized for a session topic.
type SessionSnapshotProvider func(context.Context, string, string) (map[string]any, error)

// SessionCommandHandler applies an authorized session command and returns its
// deduplication ID plus an already-redacted event envelope.
type SessionCommandHandler func(context.Context, string, string, string) (string, map[string]any, error)

// AuthorizationError lets a domain command preserve its cause while the hub
// projects its canonical authorization-denial code without importing domains.
type AuthorizationError struct{ Err error }

// Error implements error.
func (e AuthorizationError) Error() string { return e.Err.Error() }

// Unwrap exposes the domain denial sentinel to callers.
func (e AuthorizationError) Unwrap() error { return e.Err }

// NewAuthorizationError marks one command failure as an authorization denial.
func NewAuthorizationError(err error) error { return AuthorizationError{Err: err} }

func isAuthorizationError(err error) bool {
	var authorizationError AuthorizationError
	return errors.As(err, &authorizationError)
}

// Hub is the authenticated, generic WebSocket transport. Domain handlers
// publish already-redacted events through Broadcast; the hub owns topic
// authorization, connection limits, heartbeats, and delivery deduplication.
type Hub struct {
	tickets  *TicketService
	sessions SessionTopicAuthorizer
	upgrader websocket.Upgrader
	snapshot SessionSnapshotProvider
	command  SessionCommandHandler
	chat     ChatCommandHandler
	events   []SessionEventProvider

	mu         sync.Mutex
	clients    map[*hubClient]struct{}
	userCounts map[string]int
	seenEvents map[string]map[string]struct{}
}

type hubClient struct {
	conn   *websocket.Conn
	userID string
	token  string
	topics map[string]struct{}
	mu     sync.Mutex
	window time.Time
	count  int
}

// NewHub constructs a realtime WebSocket hub.
func NewHub(tickets *TicketService, sessions SessionTopicAuthorizer) *Hub {
	return &Hub{tickets: tickets, sessions: sessions, upgrader: websocket.Upgrader{}, clients: map[*hubClient]struct{}{}, userCounts: map[string]int{}, seenEvents: map[string]map[string]struct{}{}}
}

// SetSessionSnapshotProvider configures the sessions-owned snapshot callback.
func (h *Hub) SetSessionSnapshotProvider(provider SessionSnapshotProvider) {
	if h != nil {
		h.snapshot = provider
	}
}

// SetSessionCommandHandler configures the sessions-owned command callback.
func (h *Hub) SetSessionCommandHandler(handler SessionCommandHandler) {
	if h != nil {
		h.command = handler
	}
}

// SetChatCommandHandler configures the chat-owned ephemeral command callback.
func (h *Hub) SetChatCommandHandler(handler ChatCommandHandler) {
	if h != nil {
		h.chat = handler
	}
}

// RegisterSessionEventProvider adds an event emitted after an authorized
// session-topic subscription. Providers run after the session snapshot.
func (h *Hub) RegisterSessionEventProvider(provider SessionEventProvider) {
	if h == nil || provider == nil {
		return
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	h.events = append(h.events, provider)
}

// ServeHTTP authenticates a ticket query parameter and serves one connection.
func (h *Hub) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if h == nil || h.tickets == nil {
		http.Error(w, "realtime unavailable", http.StatusServiceUnavailable)
		return
	}
	ticket, err := h.tickets.Validate(r.URL.Query().Get("token"), "")
	if err != nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	client := &hubClient{userID: ticket.UserID, token: r.URL.Query().Get("token"), topics: map[string]struct{}{}, window: time.Now()}
	h.mu.Lock()
	if h.userCounts[client.userID] >= maxConnectionsPerUser {
		h.mu.Unlock()
		http.Error(w, "connection limit exceeded", http.StatusTooManyRequests)
		return
	}
	conn, err := h.upgrader.Upgrade(w, r, nil)
	if err != nil {
		h.mu.Unlock()
		return
	}
	client.conn = conn
	h.clients[client] = struct{}{}
	h.userCounts[client.userID]++
	h.mu.Unlock()
	defer h.remove(client)
	conn.SetReadLimit(64 * 1024)
	_ = conn.SetReadDeadline(time.Now().Add(90 * time.Second))
	conn.SetPongHandler(func(string) error { return conn.SetReadDeadline(time.Now().Add(90 * time.Second)) })
	for {
		_, raw, err := conn.ReadMessage()
		if err != nil {
			return
		}
		if !client.allowMessage() {
			writeRealtimeError(client, realtimeErrorRateLimit, "rate limit exceeded")
			continue
		}
		var msg struct {
			Action    string         `json:"action"`
			Type      string         `json:"type"`
			Topic     string         `json:"topic"`
			RequestID string         `json:"request_id"`
			Payload   map[string]any `json:"payload"`
		}
		if err := json.Unmarshal(raw, &msg); err != nil {
			writeRealtimeError(client, realtimeErrorInvalid, "invalid message")
			continue
		}
		if msg.Action == realtimeTypePing || msg.Type == realtimeTypePing {
			_ = conn.SetReadDeadline(time.Now().Add(90 * time.Second))
			_ = client.writeJSON(map[string]any{"type": realtimeTypePong, "server_time": time.Now().UTC().Format(time.RFC3339)})
			continue
		}
		if msg.Type == CommandChatTyping {
			h.handleChatCommand(r.Context(), client, msg.RequestID, msg.Payload)
			continue
		}
		if strings.HasPrefix(msg.Type, "cmd.") {
			h.handleCommand(r.Context(), client, msg.Type, msg.Payload)
			continue
		}
		if msg.Action != realtimeActionSubscribe {
			writeRealtimeError(client, realtimeErrorInvalid, "unsupported realtime message")
			continue
		}
		topic, err := ParseTopic(msg.Topic)
		// Tickets are intentionally short-lived. Revalidate before every
		// subscription so a connection that outlives its handshake ticket cannot
		// restore a topic with stale authorization after a transport reconnect.
		freshTicket, ticketErr := h.tickets.Validate(client.token, client.userID)
		if err != nil || ticketErr != nil || !h.authorized(r.Context(), freshTicket, client.userID, topic) {
			writeRealtimeError(client, realtimeErrorUnauthorized, "topic unauthorized")
			continue
		}
		var snapshot map[string]any
		if topic.Kind() == TopicSession && h.snapshot != nil {
			snapshot, err = h.snapshot(r.Context(), client.userID, topic.ID())
			if err != nil {
				writeRealtimeError(client, realtimeErrorSessionEnded, "session snapshot unavailable")
				continue
			}
		}
		client.mu.Lock()
		client.topics[topic.String()] = struct{}{}
		client.mu.Unlock()
		_ = client.writeJSON(map[string]any{"type": realtimeTypeSubscribed, "topic": topic.String()})
		if snapshot != nil {
			_ = client.writeJSON(snapshot)
		}
		if topic.Kind() == TopicSession {
			h.mu.Lock()
			providers := append([]SessionEventProvider(nil), h.events...)
			h.mu.Unlock()
			for _, provider := range providers {
				event, eventErr := provider(r.Context(), client.userID, topic.ID())
				if eventErr != nil {
					writeRealtimeError(client, realtimeErrorSessionEnded, "session event unavailable")
					break
				}
				if event != nil {
					_ = client.writeJSON(event)
				}
			}
		}
	}
}

func (h *Hub) handleChatCommand(ctx context.Context, client *hubClient, requestID string, payload map[string]any) {
	if h.chat == nil {
		writeRealtimeError(client, realtimeErrorInvalid, "invalid chat command")
		return
	}
	command := ChatCommand{
		Connection: client.identity(),
		RequestID:  requestID,
		Payload:    payload,
	}
	if err := h.chat(ctx, command); err != nil {
		writeRealtimeError(client, chatCommandErrorCode(err), "chat command rejected")
	}
}

// chatCommandErrorCode preserves the realtime package's transport boundary:
// chat owns its sentinels, while the hub owns canonical wire error codes.
func chatCommandErrorCode(err error) string {
	if isAuthorizationError(err) {
		return realtimeErrorUnauthorized
	}
	return realtimeErrorInvalid
}

func (h *Hub) handleCommand(ctx context.Context, client *hubClient, command string, payload map[string]any) {
	sessionID, _ := payload["session_id"].(string)
	topic, err := NewSessionTopic(sessionID)
	if err != nil || h.command == nil {
		writeRealtimeError(client, realtimeErrorInvalid, "invalid session command")
		return
	}
	client.mu.Lock()
	_, subscribed := client.topics[topic.String()]
	client.mu.Unlock()
	if !subscribed {
		writeRealtimeError(client, realtimeErrorUnauthorized, "session topic is not subscribed")
		return
	}
	eventID, event, err := h.command(ctx, client.userID, sessionID, command)
	if err != nil {
		writeRealtimeError(client, realtimeErrorInvalid, "session command rejected")
		return
	}
	if err := h.Broadcast(topic, eventID, event); err != nil {
		writeRealtimeError(client, realtimeErrorInvalid, "realtime event unavailable")
	}
}

func writeRealtimeError(client *hubClient, code, message string) {
	_ = client.writeJSON(map[string]any{"type": realtimeTypeError, "payload": map[string]any{"code": code, "message": message}})
}

// writeJSON serializes one value to the client connection. It must be used
// for every client-bound write so concurrent Broadcast calls cannot interleave
// frames with replies written from the connection's read loop.
func (c *hubClient) writeJSON(v any) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.conn.WriteJSON(v)
}

// writeText writes a pre-encoded text frame under the same lock used by
// writeJSON, keeping all outbound traffic on one connection serializable.
func (c *hubClient) writeText(data []byte) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.conn.WriteMessage(websocket.TextMessage, data)
}

func (c *hubClient) writeTextAuthorized(ctx context.Context, data []byte, authorize DeliveryAuthorizer) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	allowed, err := authorize(ctx, c.identity())
	if err != nil {
		return fmt.Errorf("authorize realtime delivery: %w", err)
	}
	if !allowed {
		return nil
	}
	return c.conn.WriteMessage(websocket.TextMessage, data)
}

func (c *hubClient) identity() ConnectionIdentity {
	return ConnectionIdentity{UserID: c.userID, RealtimeTicket: c.token}
}

func (h *Hub) authorized(ctx context.Context, ticket Ticket, userID string, topic Topic) bool {
	if topic.Kind() == TopicCircle {
		return ticket.Covers(topic)
	}
	return h.sessions != nil && h.sessions.AuthorizeSessionTopic(ctx, userID, topic.ID()) == nil
}

func (c *hubClient) allowMessage() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	now := time.Now()
	if now.Sub(c.window) >= time.Minute {
		c.window, c.count = now, 0
	}
	if c.count >= maxMessagesPerMinute {
		return false
	}
	c.count++
	return true
}

func (h *Hub) remove(client *hubClient) {
	_ = client.conn.Close()
	h.mu.Lock()
	defer h.mu.Unlock()
	delete(h.clients, client)
	h.userCounts[client.userID]--
}

// Broadcast sends one redacted event to subscribed clients. Duplicate event
// IDs are ignored per topic; empty IDs disable deduplication.
func (h *Hub) Broadcast(topic Topic, eventID string, payload any) error {
	return h.broadcast(context.Background(), topic, eventID, payload, nil)
}

// BroadcastAuthorized sends one redacted event to currently subscribed
// clients that pass the injected authorization immediately before write.
func (h *Hub) BroadcastAuthorized(ctx context.Context, topic Topic, delivery AuthorizedDelivery) error {
	if delivery.Authorize == nil {
		return errors.New("realtime delivery authorizer is not configured")
	}
	return h.broadcast(ctx, topic, delivery.EventID, delivery.Payload, delivery.Authorize)
}

func (h *Hub) broadcast(ctx context.Context, topic Topic, eventID string, payload any, authorize DeliveryAuthorizer) error {
	if h == nil {
		return errors.New("realtime hub is not configured")
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	alreadySeen := false
	if eventID != "" {
		seen := h.seenEvents[topic.String()]
		if seen == nil {
			seen = map[string]struct{}{}
			h.seenEvents[topic.String()] = seen
		}
		_, alreadySeen = seen[eventID]
		if alreadySeen {
			return nil
		}
	}
	for client := range h.clients {
		client.mu.Lock()
		_, subscribed := client.topics[topic.String()]
		client.mu.Unlock()
		if !subscribed {
			continue
		}
		var err error
		if authorize == nil {
			err = client.writeText(encoded)
		} else {
			err = client.writeTextAuthorized(ctx, encoded, authorize)
		}
		if err != nil {
			return err
		}
	}
	if eventID != "" {
		h.seenEvents[topic.String()][eventID] = struct{}{}
	}
	return nil
}

// SendToUsers sends one redacted event directly to every authenticated
// connection of the listed users, independent of circle subscriptions. Each
// connection must pass the injected authorization immediately before write.
func (h *Hub) SendToUsers(ctx context.Context, userIDs []string, delivery AuthorizedDelivery) error {
	if h == nil {
		return errors.New("realtime hub is not configured")
	}
	if delivery.Authorize == nil {
		return errors.New("realtime delivery authorizer is not configured")
	}
	encoded, err := json.Marshal(delivery.Payload)
	if err != nil {
		return fmt.Errorf("marshal realtime delivery: %w", err)
	}
	targets := make(map[string]struct{}, len(userIDs))
	for _, userID := range userIDs {
		if userID != "" {
			targets[userID] = struct{}{}
		}
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	if delivery.EventID != "" {
		for userID := range targets {
			key := "direct:" + userID
			seen := h.seenEvents[key]
			if seen == nil {
				seen = map[string]struct{}{}
				h.seenEvents[key] = seen
			}
			if _, ok := seen[delivery.EventID]; ok {
				delete(targets, userID)
				continue
			}
		}
	}
	for client := range h.clients {
		if _, ok := targets[client.userID]; !ok {
			continue
		}
		if err := client.writeTextAuthorized(ctx, encoded, delivery.Authorize); err != nil {
			return err
		}
	}
	if delivery.EventID != "" {
		for userID := range targets {
			h.seenEvents["direct:"+userID][delivery.EventID] = struct{}{}
		}
	}
	return nil
}

// SendToUser sends one redacted event to every subscribed device of one user.
// Duplicate IDs are ignored per topic and recipient.
func (h *Hub) SendToUser(topic Topic, userID, eventID string, payload any) error {
	if h == nil {
		return errors.New("realtime hub is not configured")
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	if eventID != "" {
		key := topic.String() + ":" + userID
		seen := h.seenEvents[key]
		if seen == nil {
			seen = map[string]struct{}{}
			h.seenEvents[key] = seen
		}
		if _, ok := seen[eventID]; ok {
			return nil
		}
		seen[eventID] = struct{}{}
	}
	for client := range h.clients {
		if client.userID != userID {
			continue
		}
		client.mu.Lock()
		_, subscribed := client.topics[topic.String()]
		client.mu.Unlock()
		if subscribed {
			if err := client.writeText(encoded); err != nil {
				return err
			}
		}
	}
	return nil
}
