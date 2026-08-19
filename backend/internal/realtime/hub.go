package realtime

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
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

// SessionSnapshotProvider supplies an already-redacted snapshot after a
// participant is authorized for a session topic.
type SessionSnapshotProvider func(context.Context, string, string) (map[string]any, error)

// SessionCommandHandler applies an authorized session command and returns its
// deduplication ID plus an already-redacted event envelope.
type SessionCommandHandler func(context.Context, string, string, string) (string, map[string]any, error)

// Hub is the authenticated, generic WebSocket transport. Domain handlers
// publish already-redacted events through Broadcast; the hub owns topic
// authorization, connection limits, heartbeats, and delivery deduplication.
type Hub struct {
	tickets  *TicketService
	sessions SessionTopicAuthorizer
	upgrader websocket.Upgrader
	snapshot SessionSnapshotProvider
	command  SessionCommandHandler

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
			writeRealtimeError(conn, "RATE_LIMITED", "rate limit exceeded")
			continue
		}
		var msg struct {
			Action  string         `json:"action"`
			Type    string         `json:"type"`
			Topic   string         `json:"topic"`
			Payload map[string]any `json:"payload"`
		}
		if err := json.Unmarshal(raw, &msg); err != nil {
			writeRealtimeError(conn, "INVALID_PAYLOAD", "invalid message")
			continue
		}
		if msg.Action == "ping" || msg.Type == "ping" {
			_ = conn.SetReadDeadline(time.Now().Add(90 * time.Second))
			_ = conn.WriteJSON(map[string]any{"type": "pong", "server_time": time.Now().UTC().Format(time.RFC3339)})
			continue
		}
		if strings.HasPrefix(msg.Type, "cmd.") {
			h.handleCommand(r.Context(), client, msg.Type, msg.Payload)
			continue
		}
		if msg.Action != "subscribe" {
			writeRealtimeError(conn, "INVALID_PAYLOAD", "unsupported realtime message")
			continue
		}
		topic, err := ParseTopic(msg.Topic)
		if err != nil || !h.authorized(r.Context(), ticket, client.userID, topic) {
			writeRealtimeError(conn, "UNAUTHORIZED", "topic unauthorized")
			continue
		}
		client.mu.Lock()
		client.topics[topic.String()] = struct{}{}
		client.mu.Unlock()
		_ = conn.WriteJSON(map[string]any{"type": "subscribed", "topic": topic.String()})
		if topic.Kind() == TopicSession && h.snapshot != nil {
			snapshot, err := h.snapshot(r.Context(), client.userID, topic.ID())
			if err != nil {
				writeRealtimeError(conn, "SESSION_ENDED", "session snapshot unavailable")
				continue
			}
			_ = conn.WriteJSON(snapshot)
		}
	}
}

func (h *Hub) handleCommand(ctx context.Context, client *hubClient, command string, payload map[string]any) {
	sessionID, _ := payload["session_id"].(string)
	topic, err := NewSessionTopic(sessionID)
	if err != nil || h.command == nil {
		writeRealtimeError(client.conn, "INVALID_PAYLOAD", "invalid session command")
		return
	}
	client.mu.Lock()
	_, subscribed := client.topics[topic.String()]
	client.mu.Unlock()
	if !subscribed {
		writeRealtimeError(client.conn, "UNAUTHORIZED", "session topic is not subscribed")
		return
	}
	eventID, event, err := h.command(ctx, client.userID, sessionID, command)
	if err != nil {
		writeRealtimeError(client.conn, "INVALID_PAYLOAD", "session command rejected")
		return
	}
	if err := h.Broadcast(topic, eventID, event); err != nil {
		writeRealtimeError(client.conn, "INVALID_PAYLOAD", "realtime event unavailable")
	}
}

func writeRealtimeError(conn *websocket.Conn, code, message string) {
	_ = conn.WriteJSON(map[string]any{"type": "error", "payload": map[string]any{"code": code, "message": message}})
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
