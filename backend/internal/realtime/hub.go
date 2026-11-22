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
	// realtimeWriteTimeout bounds every outbound frame write. A consumer that
	// cannot accept a frame within this window is closed as a slow client so
	// one stalled connection can never wedge the hub's fan-out.
	realtimeWriteTimeout = 10 * time.Second
	// realtimeSendBuffer is the per-connection outbound queue depth. It is
	// deep enough to absorb legitimate enqueue bursts while the write pump
	// drains at wire speed; a consumer that falls this far behind is closed
	// as a slow client instead of buffering unboundedly. Frames are never
	// silently dropped.
	realtimeSendBuffer = 256
	// maxSeenEvents caps each deduplication set so long-lived topics cannot
	// grow without bound.
	maxSeenEvents = 1024
)

// rememberEvent records one deduplicated event ID, evicting an arbitrary entry
// once the set reaches maxSeenEvents so a long-lived topic cannot grow without
// bound. Eviction makes deduplication best-effort; clients still handle the
// documented at-least-once delivery semantics.
func rememberEvent(seen map[string]struct{}, eventID string) {
	for len(seen) >= maxSeenEvents {
		for id := range seen {
			delete(seen, id)
			break
		}
	}
	seen[eventID] = struct{}{}
}

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

	mu           sync.Mutex
	clients      map[*hubClient]struct{}
	userCounts   map[string]int
	seenEvents   map[string]map[string]struct{}
	writeTimeout time.Duration
}

type hubClient struct {
	conn   *websocket.Conn
	userID string
	token  string
	topics map[string]struct{}
	send   chan []byte
	closed bool
	mu     sync.Mutex
	window time.Time
	count  int

	writeTimeout time.Duration
}

// NewHub constructs a realtime WebSocket hub.
func NewHub(tickets *TicketService, sessions SessionTopicAuthorizer) *Hub {
	return &Hub{tickets: tickets, sessions: sessions, upgrader: websocket.Upgrader{}, writeTimeout: realtimeWriteTimeout, clients: map[*hubClient]struct{}{}, userCounts: map[string]int{}, seenEvents: map[string]map[string]struct{}{}}
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
	client := &hubClient{userID: ticket.UserID, token: r.URL.Query().Get("token"), topics: map[string]struct{}{}, send: make(chan []byte, realtimeSendBuffer), window: time.Now(), writeTimeout: h.writeTimeout}
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
	go client.writePump()
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

// writeJSON queues one JSON reply frame on the connection's outbound pump.
// Every client-bound write shares that queue, so reply frames from the
// connection's read loop and broadcast frames keep strict per-connection FIFO
// order and no goroutine ever writes to the transport concurrently.
func (c *hubClient) writeJSON(v any) error {
	data, err := json.Marshal(v)
	if err != nil {
		return fmt.Errorf("marshal realtime reply: %w", err)
	}
	c.enqueue(data)
	return nil
}

// enqueue queues one pre-encoded frame for the connection's write pump. A
// full queue closes the connection as a slow client — queued frames go with
// it — instead of buffering unboundedly or blocking the caller.
func (c *hubClient) enqueue(frame []byte) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return
	}
	select {
	case c.send <- frame:
	default:
		c.closed = true
		close(c.send)
		_ = c.conn.Close()
	}
}

// closeOutbound releases the connection's write pump: it stops new enqueues,
// closes the queue so the pump drains and exits, and closes the transport.
// It is safe to call more than once.
func (c *hubClient) closeOutbound() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return
	}
	c.closed = true
	close(c.send)
	_ = c.conn.Close()
}

// writePump is the connection's sole frame writer. It drains the outbound
// queue in enqueue order — broadcast order per connection — so fan-out never
// writes while the hub mutex is held. Every frame write is bounded by the
// connection's write timeout; a stalled or failing write closes the transport
// and the read loop then unregisters the client through remove.
func (c *hubClient) writePump() {
	for frame := range c.send {
		_ = c.conn.SetWriteDeadline(time.Now().Add(c.writeTimeout))
		if err := c.conn.WriteMessage(websocket.TextMessage, frame); err != nil {
			_ = c.conn.Close()
			return
		}
	}
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
	client.closeOutbound()
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
// clients that pass the injected authorization immediately before delivery.
func (h *Hub) BroadcastAuthorized(ctx context.Context, topic Topic, delivery AuthorizedDelivery) error {
	if delivery.Authorize == nil {
		return errors.New("realtime delivery authorizer is not configured")
	}
	return h.broadcast(ctx, topic, delivery.EventID, delivery.Payload, delivery.Authorize)
}

// subscribedTargets snapshots the clients currently subscribed to the topic
// under the hub mutex, without holding it across any delivery work.
func (h *Hub) subscribedTargets(topic Topic) []*hubClient {
	targets := make([]*hubClient, 0, len(h.clients))
	for client := range h.clients {
		client.mu.Lock()
		_, subscribed := client.topics[topic.String()]
		client.mu.Unlock()
		if subscribed {
			targets = append(targets, client)
		}
	}
	return targets
}

// authorizeTargets verifies every candidate connection outside the hub mutex
// so a slow membership read cannot stall other deliveries. Verifying all
// targets before any enqueue aborts the whole delivery on a verification
// failure — callers retry it and deduplication stays unmarked — while denials
// suppress only their own connection.
func authorizeTargets(ctx context.Context, targets []*hubClient, authorize DeliveryAuthorizer) ([]*hubClient, error) {
	allowed := make(map[*hubClient]struct{}, len(targets))
	for _, client := range targets {
		ok, err := authorize(ctx, client.identity())
		if err != nil {
			return nil, fmt.Errorf("authorize realtime delivery: %w", err)
		}
		if ok {
			allowed[client] = struct{}{}
		}
	}
	verified := targets[:0]
	for _, client := range targets {
		if _, ok := allowed[client]; ok {
			verified = append(verified, client)
		}
	}
	return verified, nil
}

func (h *Hub) broadcast(ctx context.Context, topic Topic, eventID string, payload any, authorize DeliveryAuthorizer) error {
	if h == nil {
		return errors.New("realtime hub is not configured")
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	// deliver enqueues and marks deduplication atomically under the hub mutex,
	// never holding it across a transport write, so a slow consumer cannot
	// block fan-out to other users or upgrade handshakes. Enqueue order under
	// h.mu keeps per-connection frame order equal to broadcast order.
	deliver := func(targets []*hubClient) {
		h.mu.Lock()
		defer h.mu.Unlock()
		if eventID != "" {
			if _, alreadySeen := h.seenEvents[topic.String()][eventID]; alreadySeen {
				return
			}
		}
		for _, client := range targets {
			if _, registered := h.clients[client]; !registered {
				continue
			}
			client.enqueue(encoded)
		}
		if eventID != "" {
			rememberEvent(h.seenEvents[topic.String()], eventID)
		}
	}
	h.mu.Lock()
	if eventID != "" {
		seen := h.seenEvents[topic.String()]
		if seen == nil {
			seen = map[string]struct{}{}
			h.seenEvents[topic.String()] = seen
		}
		if _, alreadySeen := seen[eventID]; alreadySeen {
			h.mu.Unlock()
			return nil
		}
	}
	targets := h.subscribedTargets(topic)
	h.mu.Unlock()
	if authorize != nil {
		targets, err = authorizeTargets(ctx, targets, authorize)
		if err != nil {
			return err
		}
	}
	deliver(targets)
	return nil
}

// SendToUsers sends one redacted event directly to every authenticated
// connection of the listed users, independent of circle subscriptions. Each
// connection must pass the injected authorization immediately before delivery.
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
			}
		}
	}
	clients := make([]*hubClient, 0, len(h.clients))
	for client := range h.clients {
		if _, ok := targets[client.userID]; ok {
			clients = append(clients, client)
		}
	}
	h.mu.Unlock()
	if len(targets) > 0 && len(clients) > 0 {
		clients, err = authorizeTargets(ctx, clients, delivery.Authorize)
		if err != nil {
			return err
		}
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	if delivery.EventID != "" {
		for userID := range targets {
			if _, alreadySeen := h.seenEvents["direct:"+userID][delivery.EventID]; alreadySeen {
				delete(targets, userID)
			}
		}
	}
	for _, client := range clients {
		if _, registered := h.clients[client]; !registered {
			continue
		}
		if _, ok := targets[client.userID]; !ok {
			continue
		}
		client.enqueue(encoded)
	}
	if delivery.EventID != "" {
		for userID := range targets {
			rememberEvent(h.seenEvents["direct:"+userID], delivery.EventID)
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
		rememberEvent(seen, eventID)
	}
	for client := range h.clients {
		if client.userID != userID {
			continue
		}
		client.mu.Lock()
		_, subscribed := client.topics[topic.String()]
		client.mu.Unlock()
		if subscribed {
			client.enqueue(encoded)
		}
	}
	return nil
}
