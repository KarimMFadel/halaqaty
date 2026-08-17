package realtime

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

const (
	maxConnectionsPerUser = 3
	maxMessagesPerMinute  = 30
)

// SessionTopicAuthorizer checks whether a user may subscribe to a live
// session topic. Implementations must revalidate current membership/presence.
type SessionTopicAuthorizer interface {
	AuthorizeSessionTopic(context.Context, string, string) error
}

// Hub is the authenticated, generic WebSocket transport. Domain handlers
// publish already-redacted events through Broadcast; the hub owns topic
// authorization, connection limits, heartbeats, and delivery deduplication.
type Hub struct {
	tickets  *TicketService
	sessions SessionTopicAuthorizer
	upgrader websocket.Upgrader

	mu         sync.Mutex
	clients    map[*hubClient]struct{}
	userCounts map[string]int
	seenEvents map[string]map[string]struct{}
}

type hubClient struct {
	conn   *websocket.Conn
	userID string
	topics map[string]struct{}
	mu     sync.Mutex
	window time.Time
	count  int
}

// NewHub constructs a realtime WebSocket hub.
func NewHub(tickets *TicketService, sessions SessionTopicAuthorizer) *Hub {
	return &Hub{tickets: tickets, sessions: sessions, upgrader: websocket.Upgrader{}, clients: map[*hubClient]struct{}{}, userCounts: map[string]int{}, seenEvents: map[string]map[string]struct{}{}}
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
	client := &hubClient{userID: ticket.UserID, topics: map[string]struct{}{}, window: time.Now()}
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
			_ = conn.WriteJSON(map[string]any{"error": "rate limit exceeded"})
			continue
		}
		var msg struct {
			Action string `json:"action"`
			Topic  string `json:"topic"`
		}
		if err := json.Unmarshal(raw, &msg); err != nil {
			_ = conn.WriteJSON(map[string]any{"error": "invalid message"})
			continue
		}
		if msg.Action == "ping" {
			_ = conn.WriteJSON(map[string]any{"type": "pong"})
			continue
		}
		if msg.Action != "subscribe" {
			_ = conn.WriteJSON(map[string]any{"error": "unsupported action"})
			continue
		}
		topic, err := ParseTopic(msg.Topic)
		if err != nil || !h.authorized(r.Context(), ticket, client.userID, topic) {
			_ = conn.WriteJSON(map[string]any{"error": "topic unauthorized"})
			continue
		}
		client.mu.Lock()
		client.topics[topic.String()] = struct{}{}
		client.mu.Unlock()
		_ = conn.WriteJSON(map[string]any{"type": "subscribed", "topic": topic.String()})
	}
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
		seen := h.seenEvents[topic.String()]
		if seen == nil {
			seen = map[string]struct{}{}
			h.seenEvents[topic.String()] = seen
		}
		if _, ok := seen[eventID]; ok {
			return nil
		}
		seen[eventID] = struct{}{}
	}
	for client := range h.clients {
		client.mu.Lock()
		_, subscribed := client.topics[topic.String()]
		client.mu.Unlock()
		if !subscribed {
			continue
		}
		if err := client.conn.WriteMessage(websocket.TextMessage, encoded); err != nil {
			return err
		}
	}
	return nil
}
