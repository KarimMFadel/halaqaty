package chat

import (
	"context"
	"errors"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"

	"github.com/KarimMFadel/halaqaty/backend/internal/rbac"
	"github.com/KarimMFadel/halaqaty/backend/internal/realtime"
)

type archiveRaceMembership struct {
	circleID string
	members  map[string]bool
	lookups  atomic.Int32
}

func (m *archiveRaceMembership) IsMember(_ context.Context, _ string, userID string) (bool, error) {
	return m.members[userID], nil
}

func (m *archiveRaceMembership) MembershipStartedAt(context.Context, string, string) (time.Time, error) {
	return time.Time{}, nil
}

func (m *archiveRaceMembership) FindCircleByID(_ context.Context, circleID string) (rbac.Circle, error) {
	if circleID != m.circleID {
		return rbac.Circle{}, rbac.ErrCircleNotFound
	}
	// Admission observes the first two lookups; archive immediately before the
	// subscribed peer's socket write.
	return rbac.Circle{ID: circleID, IsArchived: m.lookups.Add(1) > 2}, nil
}

const projectorCircleID = "33333333-3333-3333-3333-333333333333"

// fixedCircleReader serves a static user→circles ticket audience.
type fixedCircleReader map[string][]string

// ListCircleIDs implements realtime.CircleTopicReader.
func (r fixedCircleReader) ListCircleIDs(_ context.Context, userID string) ([]string, error) {
	return r[userID], nil
}

// fakeMembershipReader answers membership questions without PostgreSQL; the
// PostgreSQL-backed rbac.Repository is exercised in integration tests.
type fakeMembershipReader struct {
	members map[string]bool
	circles map[string]rbac.Circle
	err     error
}

// MembershipStartedAt implements the production membership-period seam so
// projector tests exercise the same fail-closed authorization path.
func (f fakeMembershipReader) MembershipStartedAt(_ context.Context, _, _ string) (time.Time, error) {
	return time.Time{}, nil
}

func validProjectorSession(context.Context, string, string) (bool, error) { return true, nil }

// IsMember implements MembershipReader.
func (f fakeMembershipReader) IsMember(_ context.Context, circleID, userID string) (bool, error) {
	if f.err != nil {
		return false, f.err
	}
	return f.members[circleID+":"+userID], nil
}

// FindCircleByID implements MembershipReader.
func (f fakeMembershipReader) FindCircleByID(_ context.Context, circleID string) (rbac.Circle, error) {
	circle, ok := f.circles[circleID]
	if !ok {
		return rbac.Circle{}, rbac.ErrCircleNotFound
	}
	return circle, nil
}

func projectorMember(circleID, userID string) string { return circleID + ":" + userID }

func dialProjectorClient(t *testing.T, server *httptest.Server, token string) *websocket.Conn {
	t.Helper()
	url := "ws" + strings.TrimPrefix(server.URL, "http") + "?token=" + token
	conn, _, err := websocket.DefaultDialer.Dial(url, nil)
	if err != nil {
		t.Fatalf("dial hub: %v", err)
	}
	return conn
}

func writeProjectorJSON(t *testing.T, conn *websocket.Conn, value map[string]any) {
	t.Helper()
	if err := conn.WriteJSON(value); err != nil {
		t.Fatalf("write hub message: %v", err)
	}
}

func readProjectorJSON(t *testing.T, conn *websocket.Conn) map[string]any {
	t.Helper()
	if err := conn.SetReadDeadline(time.Now().Add(2 * time.Second)); err != nil {
		t.Fatalf("set read deadline: %v", err)
	}
	var message map[string]any
	if err := conn.ReadJSON(&message); err != nil {
		t.Fatalf("read hub message: %v", err)
	}
	return message
}

func assertNoProjectorEvent(t *testing.T, conn *websocket.Conn) {
	t.Helper()
	if err := conn.SetReadDeadline(time.Now().Add(150 * time.Millisecond)); err != nil {
		t.Fatalf("set read deadline: %v", err)
	}
	if _, _, err := conn.ReadMessage(); err == nil {
		t.Fatal("connection received an event it must not receive")
	}
}

func subscribeProjectorCircle(t *testing.T, conn *websocket.Conn, circleID string) {
	t.Helper()
	writeProjectorJSON(t, conn, map[string]any{"action": "subscribe", "topic": "circle." + circleID})
	if got := readProjectorJSON(t, conn); got["type"] != "subscribed" {
		t.Fatalf("circle subscribe = %v", got)
	}
}

func projectorTestMessage(t *testing.T) (Message, OutboxEvent) {
	t.Helper()
	circleID := uuid.MustParse(projectorCircleID)
	msg := Message{
		ID:       uuid.New(),
		CircleID: &circleID,
		SenderID: uuid.New(),
		Type:     MessageTypeText,
		Content:  "السلام عليكم",
		State:    MessageStateActive,
		SentAt:   time.Date(2026, 9, 3, 12, 0, 0, 0, time.UTC),
	}
	event := OutboxEvent{EventID: uuid.New(), MessageID: msg.ID, EventType: realtime.EventChatMessage}
	return msg, event
}

// TestRealtimeProjector_DuplicateEventIDDeliveredOnce proves effective-once
// projection: re-projecting the same outbox event delivers exactly one
// chat.message envelope carrying the canonical REST projection fields.
func TestRealtimeProjector_DuplicateEventIDDeliveredOnce(t *testing.T) {
	tickets := realtime.NewTicketService(fixedCircleReader{"member-user": {projectorCircleID}})
	hub := realtime.NewHub(tickets, nil)
	server := httptest.NewServer(hub)
	defer server.Close()

	ticket, err := tickets.IssueForSession(context.Background(), "member-user", "session-1")
	if err != nil {
		t.Fatalf("issue ticket: %v", err)
	}
	conn := dialProjectorClient(t, server, ticket.Token)
	defer func() { _ = conn.Close() }()
	subscribeProjectorCircle(t, conn, projectorCircleID)

	projector := NewRealtimeProjector(fakeMembershipReader{
		members: map[string]bool{projectorMember(projectorCircleID, "member-user"): true},
	}, hub, tickets, validProjectorSession)
	msg, event := projectorTestMessage(t)
	for i := 0; i < 2; i++ {
		if err := projector.ProjectMessage(context.Background(), event, msg); err != nil {
			t.Fatalf("projection %d: %v", i+1, err)
		}
	}

	envelope := readProjectorJSON(t, conn)
	if envelope["type"] != realtime.EventChatMessage {
		t.Fatalf("event type = %v, want chat.message", envelope["type"])
	}
	if envelope["event_id"] != event.EventID.String() {
		t.Fatalf("event_id = %v, want %s", envelope["event_id"], event.EventID)
	}
	if occurred, _ := envelope["occurred_at"].(string); occurred == "" {
		t.Fatalf("occurred_at must be present: %v", envelope)
	}
	payload, _ := envelope["payload"].(map[string]any)
	if payload == nil {
		t.Fatalf("payload missing: %v", envelope)
	}
	want := map[string]any{
		"id":              msg.ID.String(),
		"circle_id":       projectorCircleID,
		"sender_id":       msg.SenderID.String(),
		"message_type":    string(MessageTypeText),
		"content":         msg.Content,
		"delivery_status": string(DeliveryStatusDelivered),
	}
	for key, value := range want {
		if payload[key] != value {
			t.Fatalf("payload[%s] = %v, want %v", key, payload[key], value)
		}
	}
	if sentAt, _ := payload["sent_at"].(string); sentAt == "" || !strings.HasPrefix(sentAt, "2026-09-03T12:00:00") {
		t.Fatalf("payload[sent_at] = %v, want the durable acceptance time", payload["sent_at"])
	}
	for _, forbidden := range []string{"object_key", "url", "token", "media_url"} {
		if _, present := payload[forbidden]; present {
			t.Fatalf("payload must never carry %q", forbidden)
		}
	}
	assertNoProjectorEvent(t, conn)
}

// TestRealtimeProjector_OnlyCurrentlyAuthorizedSubscribersReceive proves the
// audience rebuild plus per-write reauthorization: a removed member's
// connection is suppressed before the write, and a member who never subscribed
// to the circle topic receives nothing.
func TestRealtimeProjector_OnlyCurrentlyAuthorizedSubscribersReceive(t *testing.T) {
	tickets := realtime.NewTicketService(fixedCircleReader{
		"member-user":  {projectorCircleID},
		"removed-user": {projectorCircleID},
		"offline-user": {projectorCircleID},
	})
	hub := realtime.NewHub(tickets, nil)
	server := httptest.NewServer(hub)
	defer server.Close()

	memberTicket, _ := tickets.IssueForSession(context.Background(), "member-user", "session-1")
	removedTicket, _ := tickets.IssueForSession(context.Background(), "removed-user", "session-1")
	offlineTicket, _ := tickets.IssueForSession(context.Background(), "offline-user", "session-1")
	member := dialProjectorClient(t, server, memberTicket.Token)
	defer func() { _ = member.Close() }()
	removed := dialProjectorClient(t, server, removedTicket.Token)
	defer func() { _ = removed.Close() }()
	offline := dialProjectorClient(t, server, offlineTicket.Token)
	defer func() { _ = offline.Close() }()
	subscribeProjectorCircle(t, member, projectorCircleID)
	subscribeProjectorCircle(t, removed, projectorCircleID)

	projector := NewRealtimeProjector(fakeMembershipReader{
		members: map[string]bool{projectorMember(projectorCircleID, "member-user"): true},
	}, hub, tickets, validProjectorSession)
	msg, event := projectorTestMessage(t)
	if err := projector.ProjectMessage(context.Background(), event, msg); err != nil {
		t.Fatalf("projection: %v", err)
	}

	if envelope := readProjectorJSON(t, member); envelope["type"] != realtime.EventChatMessage {
		t.Fatalf("current member event = %v", envelope)
	}
	assertNoProjectorEvent(t, removed)
	assertNoProjectorEvent(t, offline)
}

// TestRealtimeProjector_RevokedTicketSuppressedBeforeWrite proves per-write
// session reauthorization: a connection whose realtime ticket is no longer
// valid for the projector's ticket service receives nothing even though the
// hub accepted its subscription handshake.
func TestRealtimeProjector_RevokedTicketSuppressedBeforeWrite(t *testing.T) {
	tickets := realtime.NewTicketService(fixedCircleReader{"member-user": {projectorCircleID}})
	hub := realtime.NewHub(tickets, nil)
	server := httptest.NewServer(hub)
	defer server.Close()

	ticket, _ := tickets.IssueForSession(context.Background(), "member-user", "session-1")
	conn := dialProjectorClient(t, server, ticket.Token)
	defer func() { _ = conn.Close() }()
	subscribeProjectorCircle(t, conn, projectorCircleID)

	projector := NewRealtimeProjector(fakeMembershipReader{
		members: map[string]bool{projectorMember(projectorCircleID, "member-user"): true},
	}, hub, realtime.NewTicketService(fixedCircleReader{}), validProjectorSession)
	msg, event := projectorTestMessage(t)
	if err := projector.ProjectMessage(context.Background(), event, msg); err != nil {
		t.Fatalf("projection: %v", err)
	}
	assertNoProjectorEvent(t, conn)
}

// TestRealtimeProjector_AuthorizationFailureAbortsDelivery proves a membership
// read failure surfaces as a delivery error (driving outbox retry) instead of
// silently writing to an unverified audience.
func TestRealtimeProjector_AuthorizationFailureAbortsDelivery(t *testing.T) {
	tickets := realtime.NewTicketService(fixedCircleReader{"member-user": {projectorCircleID}})
	hub := realtime.NewHub(tickets, nil)
	server := httptest.NewServer(hub)
	defer server.Close()

	ticket, _ := tickets.IssueForSession(context.Background(), "member-user", "session-1")
	conn := dialProjectorClient(t, server, ticket.Token)
	defer func() { _ = conn.Close() }()
	subscribeProjectorCircle(t, conn, projectorCircleID)

	projector := NewRealtimeProjector(fakeMembershipReader{err: errors.New("membership unavailable")}, hub, tickets, validProjectorSession)
	msg, event := projectorTestMessage(t)
	if err := projector.ProjectMessage(context.Background(), event, msg); err == nil {
		t.Fatal("projection must fail when per-write authorization cannot be verified")
	}
}

// TestRealtimeProjector_RejectsUnsupportedEventAndDirectMessages pins the US1
// scope: only circle-scoped chat.message projection is supported.
func TestRealtimeProjector_RejectsUnsupportedEventAndDirectMessages(t *testing.T) {
	projector := NewRealtimeProjector(fakeMembershipReader{}, realtime.NewHub(realtime.NewTicketService(fixedCircleReader{}), nil), realtime.NewTicketService(fixedCircleReader{}))
	msg, event := projectorTestMessage(t)

	direct := msg
	circleID := uuid.MustParse(projectorCircleID)
	direct.CircleID = nil
	direct.DMRecipientID = &circleID
	if err := projector.ProjectMessage(context.Background(), event, direct); err == nil {
		t.Fatal("direct messages must be rejected by the group projector")
	}

	unsupported := event
	unsupported.EventType = realtime.EventChatMessageDeleted
	if err := projector.ProjectMessage(context.Background(), unsupported, msg); err == nil {
		t.Fatal("unsupported event types must be rejected")
	}
}

func TestRealtimeProjector_ProjectsReadEventOnlyToSender(t *testing.T) {
	senderID := uuid.MustParse("11111111-1111-1111-1111-111111111111")
	readerID := uuid.MustParse("22222222-2222-2222-2222-222222222222")
	tickets := realtime.NewTicketService(fixedCircleReader{
		senderID.String(): {projectorCircleID},
		readerID.String(): {projectorCircleID},
	})
	hub := realtime.NewHub(tickets, nil)
	server := httptest.NewServer(hub)
	defer server.Close()
	senderTicket, err := tickets.IssueForSession(context.Background(), senderID.String(), "sender-session")
	if err != nil {
		t.Fatalf("issue sender ticket: %v", err)
	}
	readerTicket, err := tickets.IssueForSession(context.Background(), readerID.String(), "reader-session")
	if err != nil {
		t.Fatalf("issue reader ticket: %v", err)
	}
	sender := dialProjectorClient(t, server, senderTicket.Token)
	reader := dialProjectorClient(t, server, readerTicket.Token)
	defer func() { _ = sender.Close() }()
	defer func() { _ = reader.Close() }()

	projector := NewRealtimeProjector(fakeMembershipReader{
		members: map[string]bool{projectorMember(projectorCircleID, senderID.String()): true},
	}, hub, tickets, validProjectorSession)
	readAt := time.Date(2026, 9, 3, 12, 2, 0, 0, time.UTC)
	projector.SetReadReceiptLoader(func(context.Context, uuid.UUID, uuid.UUID) (MessageRead, error) {
		return MessageRead{UserID: readerID, ReadAt: readAt}, nil
	})
	msg, event := projectorTestMessage(t)
	msg.SenderID = senderID
	event.EventType = realtime.EventChatMessageRead
	event.RecipientID = &readerID

	if err := projector.ProjectMessage(context.Background(), event, msg); err != nil {
		t.Fatalf("project read event: %v", err)
	}
	if got := readProjectorJSON(t, sender); got["type"] != realtime.EventChatMessageRead {
		t.Fatalf("sender event = %v", got)
	} else if payload, ok := got["payload"].(map[string]any); !ok || payload["message_id"] != msg.ID.String() || payload["reader_id"] != readerID.String() || payload["read_at"] != readAt.Format(time.RFC3339Nano) {
		t.Fatalf("read payload = %v", got["payload"])
	}
	assertNoProjectorEvent(t, reader)
}

func TestTypingCommandHandler_AllowsEligibleDMOnlyAfterSessionReauthorization(t *testing.T) {
	senderID := uuid.New()
	peerID := uuid.New()
	tickets := realtime.NewTicketService(fixedCircleReader{senderID.String(): {projectorCircleID}, peerID.String(): {projectorCircleID}})
	hub := realtime.NewHub(tickets, nil)
	projector := NewRealtimeProjector(fakeMembershipReader{}, hub, tickets, validProjectorSession)
	projector.SetDMEligibilityChecker(func(context.Context, uuid.UUID, uuid.UUID) (bool, error) { return true, nil })
	handler := NewTypingCommandHandler(projector)
	senderTicket, err := tickets.IssueForSession(context.Background(), senderID.String(), "sender-session")
	if err != nil {
		t.Fatalf("issue sender ticket: %v", err)
	}

	if err := handler(context.Background(), realtime.ChatCommand{
		Connection: realtime.ConnectionIdentity{UserID: senderID.String(), RealtimeTicket: senderTicket.Token},
		RequestID:  uuid.NewString(),
		Payload: map[string]any{
			"dm_peer_id": peerID.String(),
			"is_typing":  true,
		},
	}); err != nil {
		t.Fatalf("eligible DM typing command: %v", err)
	}
	projector.sessionValid = func(context.Context, string, string) (bool, error) { return false, nil }
	if err := handler(context.Background(), realtime.ChatCommand{
		Connection: realtime.ConnectionIdentity{UserID: senderID.String(), RealtimeTicket: senderTicket.Token},
		RequestID:  uuid.NewString(),
		Payload:    map[string]any{"dm_peer_id": peerID.String(), "is_typing": true},
	}); !errors.Is(err, ErrDMNotEligible) {
		t.Fatalf("revoked session DM typing error = %v, want ErrDMNotEligible", err)
	}
}

func TestTypingCommandHandler_DirectTypingExcludesSenderAndTargetsEligiblePeer(t *testing.T) {
	senderID := uuid.New()
	peerID := uuid.New()
	tickets := realtime.NewTicketService(fixedCircleReader{senderID.String(): {projectorCircleID}, peerID.String(): {projectorCircleID}})
	hub := realtime.NewHub(tickets, nil)
	server := httptest.NewServer(hub)
	defer server.Close()
	projector := NewRealtimeProjector(fakeMembershipReader{}, hub, tickets, validProjectorSession)
	projector.SetDMEligibilityChecker(func(_ context.Context, userA, userB uuid.UUID) (bool, error) {
		return (userA == senderID && userB == peerID) || (userA == peerID && userB == senderID), nil
	})
	handler := NewTypingCommandHandler(projector)
	senderTicket, err := tickets.IssueForSession(context.Background(), senderID.String(), "sender-session")
	if err != nil {
		t.Fatalf("issue sender ticket: %v", err)
	}
	peerTicket, err := tickets.IssueForSession(context.Background(), peerID.String(), "peer-session")
	if err != nil {
		t.Fatalf("issue peer ticket: %v", err)
	}
	sender := dialProjectorClient(t, server, senderTicket.Token)
	peer := dialProjectorClient(t, server, peerTicket.Token)
	defer func() { _ = sender.Close() }()
	defer func() { _ = peer.Close() }()

	if err := handler(context.Background(), realtime.ChatCommand{Connection: realtime.ConnectionIdentity{UserID: senderID.String(), RealtimeTicket: senderTicket.Token}, RequestID: uuid.NewString(), Payload: map[string]any{"dm_peer_id": peerID.String(), "is_typing": true}}); err != nil {
		t.Fatalf("direct typing command: %v", err)
	}
	if got := readProjectorJSON(t, peer); got["type"] != realtime.EventChatTyping {
		t.Fatalf("peer typing event = %v", got)
	} else if payload, ok := got["payload"].(map[string]any); !ok || payload["user_id"] != senderID.String() || payload["dm_peer_id"] != senderID.String() {
		t.Fatalf("peer typing payload = %v", got["payload"])
	}
	assertNoProjectorEvent(t, sender)
}

// TestTypingCommandHandler_SuppressesGroupTypingArchivedBeforeSocketWrite
// covers the archive race between command admission and peer delivery.
func TestTypingCommandHandler_SuppressesGroupTypingArchivedBeforeSocketWrite(t *testing.T) {
	senderID := uuid.New()
	peerID := uuid.New()
	tickets := realtime.NewTicketService(fixedCircleReader{senderID.String(): {projectorCircleID}, peerID.String(): {projectorCircleID}})
	hub := realtime.NewHub(tickets, nil)
	server := httptest.NewServer(hub)
	defer server.Close()
	membership := &archiveRaceMembership{circleID: projectorCircleID, members: map[string]bool{senderID.String(): true, peerID.String(): true}}
	projector := NewRealtimeProjector(membership, hub, tickets, validProjectorSession)
	handler := NewTypingCommandHandler(projector)
	senderTicket, err := tickets.IssueForSession(context.Background(), senderID.String(), "sender-session")
	if err != nil {
		t.Fatalf("issue sender ticket: %v", err)
	}
	peerTicket, err := tickets.IssueForSession(context.Background(), peerID.String(), "peer-session")
	if err != nil {
		t.Fatalf("issue peer ticket: %v", err)
	}
	sender := dialProjectorClient(t, server, senderTicket.Token)
	defer func() { _ = sender.Close() }()
	peer := dialProjectorClient(t, server, peerTicket.Token)
	defer func() { _ = peer.Close() }()
	subscribeProjectorCircle(t, sender, projectorCircleID)
	subscribeProjectorCircle(t, peer, projectorCircleID)

	if err := handler(context.Background(), realtime.ChatCommand{Connection: realtime.ConnectionIdentity{UserID: senderID.String(), RealtimeTicket: senderTicket.Token}, RequestID: uuid.NewString(), Payload: map[string]any{"circle_id": projectorCircleID, "is_typing": true}}); err != nil {
		t.Fatalf("typing command: %v", err)
	}
	assertNoProjectorEvent(t, peer)
}

func TestRealtimeProjector_DirectMessageTargetsBothUsersAcrossDifferentCircleSubscriptions(t *testing.T) {
	senderID := uuid.New()
	peerID := uuid.New()
	circleA := "44444444-4444-4444-4444-444444444444"
	circleB := "55555555-5555-5555-5555-555555555555"
	tickets := realtime.NewTicketService(fixedCircleReader{
		senderID.String(): {circleA},
		peerID.String():   {circleB},
	})
	hub := realtime.NewHub(tickets, nil)
	server := httptest.NewServer(hub)
	defer server.Close()

	senderTicket, err := tickets.IssueForSession(context.Background(), senderID.String(), "sender-session")
	if err != nil {
		t.Fatalf("issue sender ticket: %v", err)
	}
	peerTicket, err := tickets.IssueForSession(context.Background(), peerID.String(), "peer-session")
	if err != nil {
		t.Fatalf("issue peer ticket: %v", err)
	}
	sender := dialProjectorClient(t, server, senderTicket.Token)
	defer func() { _ = sender.Close() }()
	peer := dialProjectorClient(t, server, peerTicket.Token)
	defer func() { _ = peer.Close() }()
	subscribeProjectorCircle(t, sender, circleA)
	subscribeProjectorCircle(t, peer, circleB)

	projector := NewRealtimeProjector(fakeMembershipReader{}, hub, tickets, validProjectorSession)
	projector.SetDMEligibilityChecker(func(context.Context, uuid.UUID, uuid.UUID) (bool, error) {
		return true, nil
	})
	msg, event := projectorTestMessage(t)
	msg.SenderID = senderID
	msg.CircleID = nil
	msg.DMRecipientID = &peerID
	if err := projector.ProjectMessage(context.Background(), event, msg); err != nil {
		t.Fatalf("direct projection: %v", err)
	}
	if got := readProjectorJSON(t, sender); got["type"] != realtime.EventChatMessage {
		t.Fatalf("sender direct event = %v", got)
	}
	if got := readProjectorJSON(t, peer); got["type"] != realtime.EventChatMessage {
		t.Fatalf("peer direct event = %v", got)
	}
}
