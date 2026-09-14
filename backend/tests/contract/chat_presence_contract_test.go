//go:build contract

package contract

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/KarimMFadel/halaqaty/backend/internal/auth"
	"github.com/KarimMFadel/halaqaty/backend/internal/chat"
	"github.com/KarimMFadel/halaqaty/backend/internal/rbac"
	"github.com/KarimMFadel/halaqaty/backend/internal/realtime"
)

type presenceServiceStub struct {
	groupErr     error
	directErr    error
	directPeerID uuid.UUID
}

func (s presenceServiceStub) MarkGroupMessageRead(context.Context, uuid.UUID, uuid.UUID, uuid.UUID) error {
	return s.groupErr
}

func (s *presenceServiceStub) MarkDirectMessageRead(_ context.Context, _ uuid.UUID, peerID, _ uuid.UUID) error {
	s.directPeerID = peerID
	return s.directErr
}

func presenceRequest(method, target string) *http.Request {
	req := httptest.NewRequest(method, target, nil)
	req.Header.Set("Idempotency-Key", "presence-contract-key")
	req = req.WithContext(auth.WithPrincipal(req.Context(), auth.AuthPrincipal{
		UserID: "11111111-1111-1111-1111-111111111111",
	}))
	return req
}

func TestChatPresenceContract_GroupMarkReadAndArchivedConflict(t *testing.T) {
	circleID := "22222222-2222-2222-2222-222222222222"
	messageID := "33333333-3333-3333-3333-333333333333"
	handler := chat.NewPresenceHandler(&presenceServiceStub{groupErr: chat.ErrCircleArchived})
	req := presenceRequest(http.MethodPost, "/api/v1/circles/"+circleID+"/messages/"+messageID+"/read")
	req.SetPathValue("circleId", circleID)
	req.SetPathValue("messageId", messageID)
	rec := httptest.NewRecorder()
	handler.MarkCircleMessageRead(rec, req)
	if rec.Code != http.StatusConflict {
		t.Fatalf("archived mark-read status = %d, want %d", rec.Code, http.StatusConflict)
	}
}

func TestChatPresenceContract_DirectMarkReadRequiresIdempotencyKey(t *testing.T) {
	handler := chat.NewPresenceHandler(&presenceServiceStub{})
	req := presenceRequest(http.MethodPost, "/api/v1/dm/22222222-2222-2222-2222-222222222222/messages/33333333-3333-3333-3333-333333333333/read")
	req.Header.Del("Idempotency-Key")
	req.SetPathValue("userId", "22222222-2222-2222-2222-222222222222")
	req.SetPathValue("messageId", "33333333-3333-3333-3333-333333333333")
	rec := httptest.NewRecorder()
	handler.MarkDirectMessageRead(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("missing idempotency key status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestChatPresenceContract_DirectMarkReadMapsEligibilityDenial(t *testing.T) {
	handler := chat.NewPresenceHandler(&presenceServiceStub{directErr: chat.ErrDMNotEligible})
	req := presenceRequest(http.MethodPost, "/api/v1/dm/22222222-2222-2222-2222-222222222222/messages/33333333-3333-3333-3333-333333333333/read")
	req.SetPathValue("userId", "22222222-2222-2222-2222-222222222222")
	req.SetPathValue("messageId", "33333333-3333-3333-3333-333333333333")
	rec := httptest.NewRecorder()
	handler.MarkDirectMessageRead(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("ineligible DM status = %d, want %d", rec.Code, http.StatusForbidden)
	}
}

// TestChatPresenceContract_DirectMarkReadForwardsPathPeer proves the REST
// peer path is an authorization input, rather than validation-only metadata.
func TestChatPresenceContract_DirectMarkReadForwardsPathPeer(t *testing.T) {
	peerID := uuid.MustParse("22222222-2222-2222-2222-222222222222")
	stub := &presenceServiceStub{}
	handler := chat.NewPresenceHandler(stub)
	req := presenceRequest(http.MethodPost, "/api/v1/dm/"+peerID.String()+"/messages/33333333-3333-3333-3333-333333333333/read")
	req.SetPathValue("userId", peerID.String())
	req.SetPathValue("messageId", "33333333-3333-3333-3333-333333333333")
	rec := httptest.NewRecorder()
	handler.MarkDirectMessageRead(rec, req)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("mark-read status = %d, want %d", rec.Code, http.StatusNoContent)
	}
	if stub.directPeerID != peerID {
		t.Fatalf("service peer = %s, want path peer %s", stub.directPeerID, peerID)
	}
}

// TestChatPresenceContract_RESTReadReceiptsAreVisibleOnlyToTheSender proves
// handlers cannot disclose service-supplied read facts to a recipient.
func TestChatPresenceContract_RESTReadReceiptsAreVisibleOnlyToTheSender(t *testing.T) {
	senderID := uuid.New()
	readerID := uuid.New()
	circleID := uuid.New()
	message := chat.Message{
		ID:       uuid.New(),
		CircleID: &circleID,
		SenderID: senderID,
		Type:     chat.MessageTypeText,
		Content:  "receipt visibility",
		SentAt:   time.Now().UTC(),
		ReadReceipts: []chat.MessageRead{{
			UserID: readerID,
			ReadAt: time.Now().UTC(),
		}},
	}
	handler := chat.NewGroupHandler(&chatGroupServiceStub{history: []chat.Message{message}})

	for _, test := range []struct {
		name         string
		viewerID     uuid.UUID
		wantReceipts bool
	}{
		{name: "sender", viewerID: senderID, wantReceipts: true},
		{name: "reader", viewerID: readerID, wantReceipts: false},
	} {
		t.Run(test.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/api/v1/circles/"+circleID.String()+"/messages", nil)
			req.SetPathValue("circleId", circleID.String())
			req = req.WithContext(auth.WithPrincipal(req.Context(), auth.AuthPrincipal{UserID: test.viewerID.String()}))
			rec := httptest.NewRecorder()
			handler.ListCircleMessages(rec, req)
			if rec.Code != http.StatusOK {
				t.Fatalf("history status = %d, want %d", rec.Code, http.StatusOK)
			}
			var page struct {
				Data []map[string]any `json:"data"`
			}
			if err := json.NewDecoder(rec.Body).Decode(&page); err != nil {
				t.Fatalf("decode history response: %v", err)
			}
			_, hasReceipts := page.Data[0]["read_receipts"]
			if hasReceipts != test.wantReceipts {
				t.Fatalf("read_receipts visible = %t, want %t; response = %v", hasReceipts, test.wantReceipts, page.Data[0])
			}
		})
	}
}

// TestChatPresenceContract_MessageReadTargetsOnlyTheAuthorizedSender pins the
// group and direct sender-targeted event contract without broadcasting a
// reader's identity to the conversation audience.
func TestChatPresenceContract_MessageReadTargetsOnlyTheAuthorizedSender(t *testing.T) {
	for _, test := range []struct {
		name   string
		direct bool
	}{
		{name: "group"},
		{name: "direct", direct: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			senderID := uuid.New()
			readerID := uuid.New()
			circleID := uuid.New()
			tickets := realtime.NewTicketService(presenceCircleReader{
				senderID.String(): {circleID.String()},
				readerID.String(): {circleID.String()},
			})
			hub := realtime.NewHub(tickets, nil)
			server := httptest.NewServer(hub)
			defer server.Close()
			projector := chat.NewRealtimeProjector(
				presenceMembership{circleID: circleID.String(), members: map[string]bool{senderID.String(): true, readerID.String(): true}},
				hub,
				tickets,
				func(context.Context, string, string) (bool, error) { return true, nil },
			)
			if test.direct {
				projector.SetDMEligibilityChecker(func(_ context.Context, userA, userB uuid.UUID) (bool, error) {
					return (userA == senderID && userB == readerID) || (userA == readerID && userB == senderID), nil
				})
			}
			readAt := time.Now().UTC().Round(0)
			projector.SetReadReceiptLoader(func(context.Context, uuid.UUID, uuid.UUID) (chat.MessageRead, error) {
				return chat.MessageRead{UserID: readerID, ReadAt: readAt}, nil
			})
			senderTicket, err := tickets.IssueForSession(context.Background(), senderID.String(), "sender-session")
			if err != nil {
				t.Fatalf("issue sender ticket: %v", err)
			}
			readerTicket, err := tickets.IssueForSession(context.Background(), readerID.String(), "reader-session")
			if err != nil {
				t.Fatalf("issue reader ticket: %v", err)
			}
			sender := dialWS(t, server, senderTicket.Token)
			defer func() { _ = sender.Close() }()
			reader := dialWS(t, server, readerTicket.Token)
			defer func() { _ = reader.Close() }()
			message := chat.Message{ID: uuid.New(), SenderID: senderID, Type: chat.MessageTypeText, SentAt: readAt}
			if test.direct {
				message.DMRecipientID = &readerID
			} else {
				message.CircleID = &circleID
			}
			eventID := uuid.New()
			if err := projector.ProjectMessage(context.Background(), chat.OutboxEvent{EventID: eventID, MessageID: message.ID, EventType: realtime.EventChatMessageRead, RecipientID: &readerID}, message); err != nil {
				t.Fatalf("project message-read event: %v", err)
			}
			event := readWS(t, sender)
			payload, ok := event["payload"].(map[string]any)
			if !ok || event["type"] != realtime.EventChatMessageRead || event["event_id"] != eventID.String() || payload["message_id"] != message.ID.String() || payload["reader_id"] != readerID.String() || payload["read_at"] != readAt.Format(time.RFC3339Nano) {
				t.Fatalf("message-read event = %v", event)
			}
			_ = reader.SetReadDeadline(time.Now().Add(100 * time.Millisecond))
			if _, _, err := reader.ReadMessage(); err == nil {
				t.Fatal("reader received sender-targeted message-read event")
			}
		})
	}
}

func TestChatPresenceContract_TypingRejectsMalformedConversationContexts(t *testing.T) {
	handler := chat.NewTypingCommandHandler(chat.NewRealtimeProjector(nil, realtime.NewHub(nil, nil), nil))
	for name, payload := range map[string]map[string]any{
		"missing context": {"is_typing": true},
		"both contexts":   {"circle_id": "22222222-2222-2222-2222-222222222222", "dm_peer_id": "33333333-3333-3333-3333-333333333333", "is_typing": true},
		"missing state":   {"dm_peer_id": "33333333-3333-3333-3333-333333333333"},
	} {
		t.Run(name, func(t *testing.T) {
			err := handler(context.Background(), realtime.ChatCommand{Payload: payload})
			if !errors.Is(err, chat.ErrInvalidContext) {
				t.Fatalf("typing error = %v, want ErrInvalidContext", err)
			}
		})
	}
}

func TestChatPresenceContract_TypingRequiresUUIDRequestID(t *testing.T) {
	userID := uuid.New()
	circleID := uuid.New()
	tickets := realtime.NewTicketService(presenceCircleReader{userID.String(): {circleID.String()}})
	ticket, err := tickets.IssueForSession(context.Background(), userID.String(), "presence-session")
	if err != nil {
		t.Fatalf("issue realtime ticket: %v", err)
	}
	projector := chat.NewRealtimeProjector(
		presenceMembership{circleID: circleID.String(), members: map[string]bool{userID.String(): true}},
		realtime.NewHub(tickets, nil),
		tickets,
		func(context.Context, string, string) (bool, error) { return true, nil },
	)
	handler := chat.NewTypingCommandHandler(projector)
	for _, requestID := range []string{"", "not-a-uuid"} {
		err := handler(context.Background(), realtime.ChatCommand{
			Connection: realtime.ConnectionIdentity{UserID: userID.String(), RealtimeTicket: ticket.Token},
			RequestID:  requestID,
			Payload:    map[string]any{"circle_id": circleID.String(), "is_typing": true},
		})
		if !errors.Is(err, chat.ErrInvalidContext) {
			t.Fatalf("request_id %q error = %v, want ErrInvalidContext", requestID, err)
		}
	}
}

func TestChatPresenceContract_TypingDeliversCanonicalGroupAndDirectPayloads(t *testing.T) {
	for _, test := range []struct {
		name    string
		payload map[string]any
		setup   func(*chat.RealtimeProjector, uuid.UUID, uuid.UUID)
	}{
		{
			name:    "group",
			payload: map[string]any{"is_typing": true},
			setup:   func(*chat.RealtimeProjector, uuid.UUID, uuid.UUID) {},
		},
		{
			name:    "eligible direct message",
			payload: map[string]any{"is_typing": false},
			setup: func(projector *chat.RealtimeProjector, senderID, peerID uuid.UUID) {
				projector.SetDMEligibilityChecker(func(_ context.Context, userA, userB uuid.UUID) (bool, error) {
					return (userA == senderID && userB == peerID) || (userA == peerID && userB == senderID), nil
				})
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			senderID := uuid.New()
			peerID := uuid.New()
			circleID := uuid.New()
			tickets := realtime.NewTicketService(presenceCircleReader{
				senderID.String(): {circleID.String()},
				peerID.String():   {circleID.String()},
			})
			hub := realtime.NewHub(tickets, nil)
			server := httptest.NewServer(hub)
			defer server.Close()
			projector := chat.NewRealtimeProjector(
				presenceMembership{circleID: circleID.String(), members: map[string]bool{senderID.String(): true, peerID.String(): true}},
				hub,
				tickets,
				func(context.Context, string, string) (bool, error) { return true, nil },
			)
			test.setup(projector, senderID, peerID)
			hub.SetChatCommandHandler(chat.NewTypingCommandHandler(projector))
			senderTicket, err := tickets.IssueForSession(context.Background(), senderID.String(), "sender-session")
			if err != nil {
				t.Fatalf("issue sender ticket: %v", err)
			}
			peerTicket, err := tickets.IssueForSession(context.Background(), peerID.String(), "peer-session")
			if err != nil {
				t.Fatalf("issue peer ticket: %v", err)
			}
			sender := dialWS(t, server, senderTicket.Token)
			defer func() { _ = sender.Close() }()
			peer := dialWS(t, server, peerTicket.Token)
			defer func() { _ = peer.Close() }()
			if test.name == "group" {
				writeWS(t, sender, map[string]any{"action": "subscribe", "topic": "circle." + circleID.String()})
				_ = readWS(t, sender)
				writeWS(t, peer, map[string]any{"action": "subscribe", "topic": "circle." + circleID.String()})
				_ = readWS(t, peer)
				test.payload["circle_id"] = circleID.String()
			} else {
				test.payload["dm_peer_id"] = peerID.String()
			}

			requestID := uuid.NewString()
			before := time.Now().UTC()
			writeWS(t, sender, map[string]any{"type": realtime.CommandChatTyping, "request_id": requestID, "payload": test.payload})
			event := readWS(t, peer)
			if event["type"] != realtime.EventChatTyping || event["event_id"] != requestID {
				t.Fatalf("typing event = %v", event)
			}
			payload, ok := event["payload"].(map[string]any)
			if !ok || payload["user_id"] != senderID.String() || payload["is_typing"] != test.payload["is_typing"] {
				t.Fatalf("typing payload = %v", event["payload"])
			}
			expiresAt, err := time.Parse(time.RFC3339Nano, payload["expires_at"].(string))
			if err != nil || expiresAt.Before(before.Add(4*time.Second)) || expiresAt.After(time.Now().UTC().Add(6*time.Second)) {
				t.Fatalf("typing expiry = %v, err = %v", payload["expires_at"], err)
			}
		})
	}
}

func TestChatPresenceContract_MessageProjectionUsesServerDeliveryStatus(t *testing.T) {
	senderID := uuid.New()
	peerID := uuid.New()
	circleID := uuid.New()
	tickets := realtime.NewTicketService(presenceCircleReader{
		senderID.String(): {circleID.String()},
		peerID.String():   {circleID.String()},
	})
	hub := realtime.NewHub(tickets, nil)
	server := httptest.NewServer(hub)
	defer server.Close()
	projector := chat.NewRealtimeProjector(
		presenceMembership{circleID: circleID.String(), members: map[string]bool{senderID.String(): true, peerID.String(): true}},
		hub,
		tickets,
		func(context.Context, string, string) (bool, error) { return true, nil },
	)
	peerTicket, err := tickets.IssueForSession(context.Background(), peerID.String(), "peer-session")
	if err != nil {
		t.Fatalf("issue peer ticket: %v", err)
	}
	peer := dialWS(t, server, peerTicket.Token)
	defer func() { _ = peer.Close() }()
	writeWS(t, peer, map[string]any{"action": "subscribe", "topic": "circle." + circleID.String()})
	_ = readWS(t, peer)
	message := chat.Message{ID: uuid.New(), CircleID: &circleID, SenderID: senderID, Type: chat.MessageTypeText, Content: "contract", SentAt: time.Now().UTC()}
	eventID := uuid.New()
	if err := projector.ProjectMessage(context.Background(), chat.OutboxEvent{EventID: eventID, MessageID: message.ID, EventType: realtime.EventChatMessage}, message); err != nil {
		t.Fatalf("project message: %v", err)
	}
	event := readWS(t, peer)
	payload, ok := event["payload"].(map[string]any)
	if !ok || payload["delivery_status"] != "delivered" {
		t.Fatalf("server delivery projection = %v", event["payload"])
	}
	for _, localStatus := range []string{"pending", "sent", "read"} {
		if payload["delivery_status"] == localStatus {
			t.Fatalf("server event exposed local delivery status %q", localStatus)
		}
	}
	if _, exposed := payload["read_receipts"]; exposed {
		t.Fatalf("recipient projection exposed sender-only read receipts: %v", payload)
	}
}

// TestChatPresenceContract_TypingRBACDenialMapsToUnauthorized proves the
// socket, not just the domain handler, maps a current-membership denial to the
// canonical non-enumerating wire code.
func TestChatPresenceContract_TypingRBACDenialMapsToUnauthorized(t *testing.T) {
	senderID := uuid.New()
	circleID := uuid.New()
	tickets := realtime.NewTicketService(presenceCircleReader{senderID.String(): {circleID.String()}})
	hub := realtime.NewHub(tickets, nil)
	server := httptest.NewServer(hub)
	defer server.Close()
	projector := chat.NewRealtimeProjector(
		presenceMembership{circleID: circleID.String(), members: map[string]bool{senderID.String(): false}},
		hub,
		tickets,
		func(context.Context, string, string) (bool, error) { return true, nil },
	)
	hub.SetChatCommandHandler(chat.NewTypingCommandHandler(projector))
	ticket, err := tickets.IssueForSession(context.Background(), senderID.String(), "denied-session")
	if err != nil {
		t.Fatalf("issue realtime ticket: %v", err)
	}
	client := dialWS(t, server, ticket.Token)
	defer func() { _ = client.Close() }()

	writeWS(t, client, map[string]any{"type": realtime.CommandChatTyping, "request_id": uuid.NewString(), "payload": map[string]any{"circle_id": circleID.String(), "is_typing": true}})
	event := readWS(t, client)
	payload, ok := event["payload"].(map[string]any)
	if !ok || event["type"] != "error" || payload["code"] != "UNAUTHORIZED" {
		t.Fatalf("typing RBAC denial event = %v, want UNAUTHORIZED error", event)
	}
}

func TestChatPresenceContract_DirectTypingDenialMapsToUnauthorized(t *testing.T) {
	senderID := uuid.New()
	peerID := uuid.New()
	circleID := uuid.New()
	tickets := realtime.NewTicketService(presenceCircleReader{senderID.String(): {circleID.String()}})
	hub := realtime.NewHub(tickets, nil)
	server := httptest.NewServer(hub)
	defer server.Close()
	projector := chat.NewRealtimeProjector(presenceMembership{circleID: circleID.String(), members: map[string]bool{senderID.String(): true}}, hub, tickets, func(context.Context, string, string) (bool, error) { return true, nil })
	projector.SetDMEligibilityChecker(func(context.Context, uuid.UUID, uuid.UUID) (bool, error) { return false, nil })
	hub.SetChatCommandHandler(chat.NewTypingCommandHandler(projector))
	ticket, err := tickets.IssueForSession(context.Background(), senderID.String(), "denied-direct-session")
	if err != nil {
		t.Fatalf("issue realtime ticket: %v", err)
	}
	client := dialWS(t, server, ticket.Token)
	defer func() { _ = client.Close() }()

	writeWS(t, client, map[string]any{"type": realtime.CommandChatTyping, "request_id": uuid.NewString(), "payload": map[string]any{"dm_peer_id": peerID.String(), "is_typing": true}})
	event := readWS(t, client)
	payload, ok := event["payload"].(map[string]any)
	if !ok || event["type"] != "error" || payload["code"] != "UNAUTHORIZED" {
		t.Fatalf("direct typing denial event = %v, want UNAUTHORIZED error", event)
	}
}

// TestChatPresenceContract_TypingCommandUsesHubRateLimit proves chat typing
// uses the shared inbound WebSocket budget and returns the canonical error
// after its command limit is exhausted.
func TestChatPresenceContract_TypingCommandUsesHubRateLimit(t *testing.T) {
	senderID := uuid.New()
	circleID := uuid.New()
	tickets := realtime.NewTicketService(presenceCircleReader{senderID.String(): {circleID.String()}})
	hub := realtime.NewHub(tickets, nil)
	server := httptest.NewServer(hub)
	defer server.Close()
	projector := chat.NewRealtimeProjector(
		presenceMembership{circleID: circleID.String(), members: map[string]bool{senderID.String(): true}},
		hub,
		tickets,
		func(context.Context, string, string) (bool, error) { return true, nil },
	)
	hub.SetChatCommandHandler(chat.NewTypingCommandHandler(projector))
	ticket, err := tickets.IssueForSession(context.Background(), senderID.String(), "rate-limited-session")
	if err != nil {
		t.Fatalf("issue realtime ticket: %v", err)
	}
	client := dialWS(t, server, ticket.Token)
	defer func() { _ = client.Close() }()
	for attempt := 0; attempt < 31; attempt++ {
		writeWS(t, client, map[string]any{"type": realtime.CommandChatTyping, "request_id": uuid.NewString(), "payload": map[string]any{"circle_id": circleID.String(), "is_typing": true}})
	}
	event := readWS(t, client)
	payload, ok := event["payload"].(map[string]any)
	if !ok || event["type"] != "error" || payload["code"] != "RATE_LIMITED" {
		t.Fatalf("typing rate-limit event = %v, want RATE_LIMITED error", event)
	}
}

type presenceCircleReader map[string][]string

func (r presenceCircleReader) ListCircleIDs(_ context.Context, userID string) ([]string, error) {
	return r[userID], nil
}

type presenceMembership struct {
	circleID string
	members  map[string]bool
}

func (m presenceMembership) IsMember(_ context.Context, _ string, userID string) (bool, error) {
	return m.members[userID], nil
}

func (presenceMembership) MembershipStartedAt(context.Context, string, string) (time.Time, error) {
	return time.Time{}, nil
}

func (m presenceMembership) FindCircleByID(_ context.Context, circleID string) (rbac.Circle, error) {
	if circleID != m.circleID {
		return rbac.Circle{}, rbac.ErrCircleNotFound
	}
	return rbac.Circle{ID: circleID}, nil
}
