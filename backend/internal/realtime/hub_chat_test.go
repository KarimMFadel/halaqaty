package realtime

import (
	"context"
	"errors"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestHub_ChatCommandMapsAuthorizationDenialToUnauthorized(t *testing.T) {
	tickets := NewTicketService(hubTicketReader{circleID: chatCircleID1})
	hub := NewHub(tickets, nil)
	hub.SetChatCommandHandler(func(context.Context, ChatCommand) error {
		return NewAuthorizationError(errors.New("chat: circle not visible"))
	})
	server := httptest.NewServer(hub)
	defer server.Close()
	ticket, err := tickets.Issue(context.Background(), "user-1")
	if err != nil {
		t.Fatal(err)
	}
	conn := dialHub(t, server, ticket.Token)
	defer func() { _ = conn.Close() }()
	writeHub(t, conn, map[string]any{"type": CommandChatTyping, "request_id": "00000000-0000-0000-0000-000000000001", "payload": map[string]any{"circle_id": chatCircleID1, "is_typing": true}})
	got := readHub(t, conn)
	payload, _ := got["payload"].(map[string]any)
	if got["type"] != realtimeTypeError || payload["code"] != realtimeErrorUnauthorized {
		t.Fatalf("authorization denial = %v, want UNAUTHORIZED", got)
	}
}

const (
	chatCircleID1 = "11111111-1111-1111-1111-111111111111"
	chatCircleID2 = "22222222-2222-2222-2222-222222222222"
)

func TestHub_BroadcastAuthorizedReauthorizesEachCircleSubscriberBeforeWrite(t *testing.T) {
	tickets := NewTicketService(chatTicketReader{
		"allowed-user": {chatCircleID1},
		"revoked-user": {chatCircleID1},
	})
	hub := NewHub(tickets, hubSessionAuthorizer{})
	server := httptest.NewServer(hub)
	defer server.Close()

	allowedTicket, _ := tickets.Issue(context.Background(), "allowed-user")
	revokedTicket, _ := tickets.Issue(context.Background(), "revoked-user")
	allowed := dialHub(t, server, allowedTicket.Token)
	defer func() { _ = allowed.Close() }()
	revoked := dialHub(t, server, revokedTicket.Token)
	defer func() { _ = revoked.Close() }()
	subscribeChatTopic(t, allowed, chatCircleID1)
	subscribeChatTopic(t, revoked, chatCircleID1)

	topic, _ := NewCircleTopic(chatCircleID1)
	var mu sync.Mutex
	authorizedTickets := map[string]int{}
	authorize := func(_ context.Context, connection ConnectionIdentity) (bool, error) {
		mu.Lock()
		authorizedTickets[connection.RealtimeTicket]++
		mu.Unlock()
		return connection.RealtimeTicket != revokedTicket.Token, nil
	}
	event := map[string]any{"type": EventChatMessage, "event_id": "group-event"}
	if err := hub.BroadcastAuthorized(context.Background(), topic, AuthorizedDelivery{
		EventID: "group-event", Payload: event, Authorize: authorize,
	}); err != nil {
		t.Fatalf("authorized broadcast: %v", err)
	}

	if got := readHub(t, allowed); got["type"] != EventChatMessage {
		t.Fatalf("allowed subscriber event = %v", got)
	}
	assertNoChatEvent(t, revoked)
	mu.Lock()
	defer mu.Unlock()
	if authorizedTickets[allowedTicket.Token] != 1 || authorizedTickets[revokedTicket.Token] != 1 {
		t.Fatalf("authorization calls = %v, want once per subscribed connection", authorizedTickets)
	}
}

func TestHub_SendToUsersDeliversDMWithoutSharedTopic(t *testing.T) {
	tickets := NewTicketService(chatTicketReader{
		"sender":    {chatCircleID1},
		"recipient": {chatCircleID2},
	})
	hub := NewHub(tickets, hubSessionAuthorizer{})
	server := httptest.NewServer(hub)
	defer server.Close()

	senderTicket, _ := tickets.Issue(context.Background(), "sender")
	recipientTicket, _ := tickets.Issue(context.Background(), "recipient")
	sender := dialHub(t, server, senderTicket.Token)
	defer func() { _ = sender.Close() }()
	recipient := dialHub(t, server, recipientTicket.Token)
	defer func() { _ = recipient.Close() }()
	subscribeChatTopic(t, sender, chatCircleID1)
	subscribeChatTopic(t, recipient, chatCircleID2)

	event := map[string]any{"type": EventChatMessage, "event_id": "dm-event"}
	if err := hub.SendToUsers(context.Background(), []string{"sender", "recipient"}, AuthorizedDelivery{
		EventID: "dm-event",
		Payload: event,
		Authorize: func(context.Context, ConnectionIdentity) (bool, error) {
			return true, nil
		},
	}); err != nil {
		t.Fatalf("direct delivery: %v", err)
	}
	if got := readHub(t, sender); got["type"] != EventChatMessage {
		t.Fatalf("sender event = %v", got)
	}
	if got := readHub(t, recipient); got["type"] != EventChatMessage {
		t.Fatalf("recipient event = %v", got)
	}
}

func TestHub_SendToUsersSuppressesRevokedConnectionOnly(t *testing.T) {
	tickets := NewTicketService(chatTicketReader{"user-1": {chatCircleID1}})
	hub := NewHub(tickets, hubSessionAuthorizer{})
	server := httptest.NewServer(hub)
	defer server.Close()

	revokedTicket, _ := tickets.Issue(context.Background(), "user-1")
	activeTicket, _ := tickets.Issue(context.Background(), "user-1")
	revoked := dialHub(t, server, revokedTicket.Token)
	defer func() { _ = revoked.Close() }()
	active := dialHub(t, server, activeTicket.Token)
	defer func() { _ = active.Close() }()

	event := map[string]any{"type": EventChatMessageRead, "event_id": "read-event"}
	if err := hub.SendToUsers(context.Background(), []string{"user-1"}, AuthorizedDelivery{
		EventID: "read-event",
		Payload: event,
		Authorize: func(_ context.Context, connection ConnectionIdentity) (bool, error) {
			return connection.RealtimeTicket != revokedTicket.Token, nil
		},
	}); err != nil {
		t.Fatalf("direct delivery: %v", err)
	}
	if got := readHub(t, active); got["type"] != EventChatMessageRead {
		t.Fatalf("active connection event = %v", got)
	}
	assertNoChatEvent(t, revoked)
}

func TestHub_ChatTypingCommandUsesChatHandlerWithoutChangingSessionCommands(t *testing.T) {
	tickets := NewTicketService(chatTicketReader{"user-1": {chatCircleID1}})
	hub := NewHub(tickets, hubSessionAuthorizer{})
	var sessionCommands atomic.Int32
	hub.SetSessionCommandHandler(func(_ context.Context, userID, sessionID, command string) (string, map[string]any, error) {
		sessionCommands.Add(1)
		return "session-event", map[string]any{
			"type":    "session.hand_raised",
			"payload": map[string]any{"session_id": sessionID, "participant_id": userID, "command": command},
		}, nil
	})
	received := make(chan ChatCommand, 1)
	hub.SetChatCommandHandler(func(_ context.Context, command ChatCommand) error {
		received <- command
		return nil
	})
	server := httptest.NewServer(hub)
	defer server.Close()
	ticket, _ := tickets.Issue(context.Background(), "user-1")
	conn := dialHub(t, server, ticket.Token)
	defer func() { _ = conn.Close() }()

	writeHub(t, conn, map[string]any{"action": realtimeActionSubscribe, "topic": "session." + hubSessionID})
	if got := readHub(t, conn); got["type"] != realtimeTypeSubscribed {
		t.Fatalf("session subscribe = %v", got)
	}
	writeHub(t, conn, map[string]any{"type": "cmd.raise_hand", "payload": map[string]any{"session_id": hubSessionID}})
	if got := readHub(t, conn); got["type"] != "session.hand_raised" {
		t.Fatalf("session command event = %v", got)
	}

	writeHub(t, conn, map[string]any{
		"type":       CommandChatTyping,
		"request_id": "typing-request",
		"payload":    map[string]any{"circle_id": chatCircleID1, "is_typing": true},
	})
	select {
	case got := <-received:
		if got.Connection.UserID != "user-1" || got.Connection.RealtimeTicket != ticket.Token || got.RequestID != "typing-request" {
			t.Fatalf("chat command identity = %+v", got)
		}
		if got.Payload["circle_id"] != chatCircleID1 || got.Payload["is_typing"] != true {
			t.Fatalf("chat command payload = %v", got.Payload)
		}
	case <-time.After(time.Second):
		t.Fatal("chat command handler was not called")
	}
	if sessionCommands.Load() != 1 {
		t.Fatalf("session command calls = %d, want 1", sessionCommands.Load())
	}
	writeHub(t, conn, map[string]any{"type": realtimeTypePing})
	if got := readHub(t, conn); got["type"] != realtimeTypePong {
		t.Fatalf("unexpected hub-side typing event or broken heartbeat = %v", got)
	}
}

type chatTicketReader map[string][]string

func (r chatTicketReader) ListCircleIDs(_ context.Context, userID string) ([]string, error) {
	return r[userID], nil
}

func subscribeChatTopic(t *testing.T, conn interface {
	WriteJSON(v any) error
	ReadJSON(v any) error
}, circleID string) {
	t.Helper()
	if err := conn.WriteJSON(map[string]any{"action": realtimeActionSubscribe, "topic": "circle." + circleID}); err != nil {
		t.Fatal(err)
	}
	var response map[string]any
	if err := conn.ReadJSON(&response); err != nil {
		t.Fatal(err)
	}
	if response["type"] != realtimeTypeSubscribed {
		t.Fatalf("circle subscribe = %v", response)
	}
}

func assertNoChatEvent(t *testing.T, conn interface {
	SetReadDeadline(time.Time) error
	ReadMessage() (int, []byte, error)
}) {
	t.Helper()
	if err := conn.SetReadDeadline(time.Now().Add(100 * time.Millisecond)); err != nil {
		t.Fatal(err)
	}
	if _, _, err := conn.ReadMessage(); err == nil {
		t.Fatal("unauthorized connection received chat event")
	}
}
