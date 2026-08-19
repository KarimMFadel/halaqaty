package realtime

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

const hubSessionID = "99999999-9999-9999-9999-999999999999"

func TestHub_SubscriptionSendsAuthorizedSessionSnapshot(t *testing.T) {
	tickets := NewTicketService(hubTicketReader{circleID: "11111111-1111-1111-1111-111111111111"})
	hub := NewHub(tickets, hubSessionAuthorizer{})
	hub.SetSessionSnapshotProvider(func(context.Context, string, string) (map[string]any, error) {
		return map[string]any{
			"type":      "session.snapshot",
			"timestamp": time.Now().UTC().Format(time.RFC3339),
			"payload": map[string]any{
				"session":      map[string]any{"id": hubSessionID, "status": "active", "is_locked": false},
				"participants": []any{},
			},
		}, nil
	})

	server := httptest.NewServer(hub)
	defer server.Close()
	ticket, err := tickets.Issue(context.Background(), "user-1")
	if err != nil {
		t.Fatal(err)
	}
	conn := dialHub(t, server, ticket.Token)
	defer func() { _ = conn.Close() }()

	writeHub(t, conn, map[string]any{"action": "subscribe", "topic": "session." + hubSessionID})
	if got := readHub(t, conn); got["type"] != "subscribed" {
		t.Fatalf("subscribe response = %v", got)
	}
	snapshot := readHub(t, conn)
	if snapshot["type"] != "session.snapshot" {
		t.Fatalf("snapshot = %v", snapshot)
	}
	if _, ok := snapshot["timestamp"].(string); !ok {
		t.Fatalf("snapshot timestamp = %v, want RFC3339 string", snapshot["timestamp"])
	}
	if _, ok := snapshot["credential"]; ok {
		t.Fatal("snapshot leaked credential")
	}
}

func TestHub_HandlesContractCommandsAndHeartbeat(t *testing.T) {
	tickets := NewTicketService(hubTicketReader{circleID: "11111111-1111-1111-1111-111111111111"})
	hub := NewHub(tickets, hubSessionAuthorizer{})
	hub.SetSessionCommandHandler(func(_ context.Context, userID, sessionID, command string) (string, map[string]any, error) {
		eventType := "session.hand_raised"
		if command == "cmd.lower_hand" {
			eventType = "session.hand_lowered"
		}
		return command + ":" + userID, map[string]any{
			"type":    eventType,
			"payload": map[string]any{"session_id": sessionID, "participant_id": userID},
		}, nil
	})

	server := httptest.NewServer(hub)
	defer server.Close()
	ticket, _ := tickets.Issue(context.Background(), "user-1")
	conn := dialHub(t, server, ticket.Token)
	defer func() { _ = conn.Close() }()
	writeHub(t, conn, map[string]any{"action": "subscribe", "topic": "session." + hubSessionID})
	if got := readHub(t, conn); got["type"] != "subscribed" {
		t.Fatalf("subscribe response = %v", got)
	}

	writeHub(t, conn, map[string]any{"type": "cmd.raise_hand", "payload": map[string]any{"session_id": hubSessionID}})
	if got := readHub(t, conn); got["type"] != "session.hand_raised" {
		t.Fatalf("hand event = %v", got)
	}

	writeHub(t, conn, map[string]any{"type": "ping"})
	if got := readHub(t, conn); got["type"] != "pong" {
		t.Fatalf("heartbeat = %v", got)
	} else if _, ok := got["server_time"]; !ok {
		t.Fatalf("heartbeat lacks server_time: %v", got)
	}
}

func TestHub_UsesCanonicalErrorEnvelopeForRateLimit(t *testing.T) {
	tickets := NewTicketService(hubTicketReader{circleID: "11111111-1111-1111-1111-111111111111"})
	hub := NewHub(tickets, hubSessionAuthorizer{})
	server := httptest.NewServer(hub)
	defer server.Close()
	ticket, _ := tickets.Issue(context.Background(), "user-1")
	conn := dialHub(t, server, ticket.Token)
	defer func() { _ = conn.Close() }()
	for i := 0; i < maxMessagesPerMinute; i++ {
		writeHub(t, conn, map[string]any{"type": "ping"})
		_ = readHub(t, conn)
	}
	writeHub(t, conn, map[string]any{"type": "ping"})
	got := readHub(t, conn)
	if got["type"] != "error" {
		t.Fatalf("rate-limit type = %v, want error", got["type"])
	}
	payload, ok := got["payload"].(map[string]any)
	if !ok || payload["code"] != "RATE_LIMITED" {
		t.Fatalf("rate-limit payload = %v, want RATE_LIMITED", got["payload"])
	}
}

func TestHub_UsesCanonicalErrorEnvelopeForMalformedMessages(t *testing.T) {
	tickets := NewTicketService(hubTicketReader{circleID: "11111111-1111-1111-1111-111111111111"})
	hub := NewHub(tickets, hubSessionAuthorizer{})
	server := httptest.NewServer(hub)
	defer server.Close()
	ticket, _ := tickets.Issue(context.Background(), "user-1")
	conn := dialHub(t, server, ticket.Token)
	defer func() { _ = conn.Close() }()
	if err := conn.WriteMessage(websocket.TextMessage, []byte("{")); err != nil {
		t.Fatal(err)
	}
	got := readHub(t, conn)
	payload, _ := got["payload"].(map[string]any)
	if got["type"] != "error" || payload["code"] != "INVALID_PAYLOAD" {
		t.Fatalf("malformed response = %v, want canonical INVALID_PAYLOAD", got)
	}
}

func TestHub_EnforcesThreeConnectionsPerUser(t *testing.T) {
	tickets := NewTicketService(hubTicketReader{circleID: "11111111-1111-1111-1111-111111111111"})
	hub := NewHub(tickets, hubSessionAuthorizer{})
	server := httptest.NewServer(hub)
	defer server.Close()
	ticket, _ := tickets.Issue(context.Background(), "user-1")
	connections := make([]*websocket.Conn, 0, maxConnectionsPerUser)
	for i := 0; i < maxConnectionsPerUser; i++ {
		conn := dialHub(t, server, ticket.Token)
		connections = append(connections, conn)
	}
	defer func() {
		for _, conn := range connections {
			_ = conn.Close()
		}
	}()
	url := "ws" + server.URL[len("http"):]
	_, response, err := websocket.DefaultDialer.Dial(url+"?token="+ticket.Token, nil)
	if err == nil || response == nil || response.StatusCode != 429 {
		t.Fatalf("fourth connection = err %v status %v, want 429", err, responseStatus(response))
	}
}

func TestHub_EnforcesThirtyMessagesPerMinute(t *testing.T) {
	tickets := NewTicketService(hubTicketReader{circleID: "11111111-1111-1111-1111-111111111111"})
	hub := NewHub(tickets, hubSessionAuthorizer{})
	server := httptest.NewServer(hub)
	defer server.Close()
	ticket, _ := tickets.Issue(context.Background(), "user-1")
	conn := dialHub(t, server, ticket.Token)
	defer func() { _ = conn.Close() }()
	for i := 0; i < maxMessagesPerMinute; i++ {
		writeHub(t, conn, map[string]any{"type": "ping"})
		if got := readHub(t, conn); got["type"] != "pong" {
			t.Fatalf("message %d response = %v", i+1, got)
		}
	}
	writeHub(t, conn, map[string]any{"type": "ping"})
	got := readHub(t, conn)
	payload, _ := got["payload"].(map[string]any)
	if got["type"] != "error" || payload["code"] != "RATE_LIMITED" {
		t.Fatalf("31st message = %v, want rate-limit error", got)
	}
}

func responseStatus(response *http.Response) any {
	if response == nil {
		return nil
	}
	return response.StatusCode
}

func TestHub_BroadcastDeduplicatesEventIDsPerTopic(t *testing.T) {
	tickets := NewTicketService(hubTicketReader{circleID: "11111111-1111-1111-1111-111111111111"})
	hub := NewHub(tickets, hubSessionAuthorizer{})
	topic, err := NewCircleTopic("11111111-1111-1111-1111-111111111111")
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(hub)
	defer server.Close()
	ticket, _ := tickets.Issue(context.Background(), "user-1")
	conn := dialHub(t, server, ticket.Token)
	defer func() { _ = conn.Close() }()
	writeHub(t, conn, map[string]any{"action": "subscribe", "topic": topic.String()})
	if got := readHub(t, conn); got["type"] != "subscribed" {
		t.Fatalf("subscribe response = %v", got)
	}

	payload := map[string]any{"type": "session.hand_raised", "payload": map[string]any{"session_id": hubSessionID}}
	if err := hub.Broadcast(topic, "event-1", payload); err != nil {
		t.Fatal(err)
	}
	if err := hub.Broadcast(topic, "event-1", payload); err != nil {
		t.Fatal(err)
	}
	if got := readHub(t, conn); got["type"] != "session.hand_raised" {
		t.Fatalf("broadcast = %v", got)
	}
	_ = conn.SetReadDeadline(time.Now().Add(100 * time.Millisecond))
	if _, _, err := conn.ReadMessage(); err == nil {
		t.Fatal("duplicate broadcast was delivered")
	}
}

type hubTicketReader struct{ circleID string }

func (r hubTicketReader) ListCircleIDs(context.Context, string) ([]string, error) {
	return []string{r.circleID}, nil
}

type hubSessionAuthorizer struct{}

func (hubSessionAuthorizer) AuthorizeSessionTopic(context.Context, string, string) error { return nil }

func dialHub(t *testing.T, server *httptest.Server, token string) *websocket.Conn {
	t.Helper()
	conn, _, err := websocket.DefaultDialer.Dial("ws"+server.URL[len("http"):]+"?token="+token, nil)
	if err != nil {
		t.Fatal(err)
	}
	return conn
}

func writeHub(t *testing.T, conn *websocket.Conn, value map[string]any) {
	t.Helper()
	if err := conn.WriteJSON(value); err != nil {
		t.Fatal(err)
	}
}

func readHub(t *testing.T, conn *websocket.Conn) map[string]any {
	t.Helper()
	_, raw, err := conn.ReadMessage()
	if err != nil {
		t.Fatal(err)
	}
	var value map[string]any
	if err := json.Unmarshal(raw, &value); err != nil {
		t.Fatal(err)
	}
	return value
}
