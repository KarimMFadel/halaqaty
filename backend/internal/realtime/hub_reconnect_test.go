package realtime

import (
	"context"
	"errors"
	"net/http/httptest"
	"testing"
	"time"
)

type reconnectSessionAuthorizer func(context.Context, string, string) error

func (authorize reconnectSessionAuthorizer) AuthorizeSessionTopic(ctx context.Context, userID, sessionID string) error {
	return authorize(ctx, userID, sessionID)
}

func TestHub_ReconnectWithNewTicket_ReauthorizesAndRehydratesCurrentSnapshot(t *testing.T) {
	authorizationRevision := 0
	snapshotRevision := 0
	tickets := NewTicketService(hubTicketReader{circleID: "11111111-1111-1111-1111-111111111111"})
	hub := NewHub(tickets, reconnectSessionAuthorizer(func(context.Context, string, string) error {
		authorizationRevision++
		return nil
	}))
	hub.SetSessionSnapshotProvider(func(context.Context, string, string) (map[string]any, error) {
		snapshotRevision++
		return map[string]any{
			"type":      "session.snapshot",
			"timestamp": time.Now().UTC().Format(time.RFC3339),
			"payload": map[string]any{
				"session":      map[string]any{"id": hubSessionID, "status": "active", "is_locked": false, "authorization_revision": authorizationRevision},
				"participants": []any{},
			},
		}, nil
	})

	server := httptest.NewServer(hub)
	defer server.Close()

	var previousTicket string
	for reconnectAttempt := 1; reconnectAttempt <= 2; reconnectAttempt++ {
		ticket, err := tickets.Issue(context.Background(), "user-1")
		if err != nil {
			t.Fatalf("issue reconnect ticket: %v", err)
		}
		if ticket.Token == previousTicket {
			t.Fatal("reconnect must use a newly issued realtime ticket")
		}
		previousTicket = ticket.Token
		connection := dialHub(t, server, ticket.Token)
		writeHub(t, connection, map[string]any{"action": realtimeActionSubscribe, "topic": "session." + hubSessionID})
		if got := readHub(t, connection); got["type"] != realtimeTypeSubscribed {
			t.Fatalf("reconnect subscribe response = %v", got)
		}
		snapshot := readHub(t, connection)
		if snapshot["type"] != "session.snapshot" {
			t.Fatalf("reconnect snapshot = %v", snapshot)
		}
		payload, ok := snapshot["payload"].(map[string]any)
		if !ok {
			t.Fatalf("reconnect snapshot payload = %v", snapshot["payload"])
		}
		session, ok := payload["session"].(map[string]any)
		if !ok || session["authorization_revision"] != float64(reconnectAttempt) {
			t.Fatalf("reconnect snapshot session = %v, want authorization revision %d", payload["session"], reconnectAttempt)
		}
		if snapshotRevision != reconnectAttempt {
			t.Fatalf("snapshot rehydrations = %d, want %d", snapshotRevision, reconnectAttempt)
		}
		if err := connection.Close(); err != nil {
			t.Fatalf("close reconnect connection: %v", err)
		}
	}
}

func TestHub_ReconnectRejectedAuthorization_ReturnsCanonicalUnauthorizedError(t *testing.T) {
	allowSession := true
	snapshotCalls := 0
	authorizer := reconnectSessionAuthorizer(func(context.Context, string, string) error {
		if allowSession {
			return nil
		}
		return errors.New("authorization rejected")
	})
	tickets := NewTicketService(hubTicketReader{circleID: "11111111-1111-1111-1111-111111111111"})
	hub := NewHub(tickets, authorizer)
	hub.SetSessionSnapshotProvider(func(context.Context, string, string) (map[string]any, error) {
		snapshotCalls++
		return map[string]any{"type": "session.snapshot"}, nil
	})
	server := httptest.NewServer(hub)
	defer server.Close()

	firstTicket, err := tickets.Issue(context.Background(), "user-1")
	if err != nil {
		t.Fatalf("issue initial ticket: %v", err)
	}
	firstConnection := dialHub(t, server, firstTicket.Token)
	writeHub(t, firstConnection, map[string]any{"action": realtimeActionSubscribe, "topic": "session." + hubSessionID})
	_ = readHub(t, firstConnection)
	_ = readHub(t, firstConnection)
	if err := firstConnection.Close(); err != nil {
		t.Fatalf("close initial connection: %v", err)
	}
	if snapshotCalls != 1 {
		t.Fatalf("initial snapshot calls = %d, want 1", snapshotCalls)
	}

	allowSession = false
	reconnectTicket, err := tickets.Issue(context.Background(), "user-1")
	if err != nil {
		t.Fatalf("issue reconnect ticket: %v", err)
	}
	reconnect := dialHub(t, server, reconnectTicket.Token)
	defer func() { _ = reconnect.Close() }()
	writeHub(t, reconnect, map[string]any{"action": realtimeActionSubscribe, "topic": "session." + hubSessionID})
	if got := readHub(t, reconnect); got["type"] != realtimeTypeError {
		t.Fatalf("rejected reconnect response = %v, want error", got)
	} else if payload, ok := got["payload"].(map[string]any); !ok || payload["code"] != realtimeErrorUnauthorized {
		t.Fatalf("rejected reconnect payload = %v, want %s", got["payload"], realtimeErrorUnauthorized)
	}
	if snapshotCalls != 1 {
		t.Fatalf("terminal authorization must not rehydrate a snapshot, got %d calls", snapshotCalls)
	}
}

func TestHub_SubscriptionRejectsExpiredHandshakeTicket(t *testing.T) {
	tickets := NewTicketService(hubTicketReader{circleID: "11111111-1111-1111-1111-111111111111"})
	tickets.now = func() time.Time { return time.Unix(100, 0).UTC() }
	hub := NewHub(tickets, reconnectSessionAuthorizer(func(context.Context, string, string) error {
		return nil
	}))
	server := httptest.NewServer(hub)
	defer server.Close()

	ticket, err := tickets.Issue(context.Background(), "user-1")
	if err != nil {
		t.Fatalf("issue ticket: %v", err)
	}
	conn := dialHub(t, server, ticket.Token)
	defer func() { _ = conn.Close() }()

	tickets.now = func() time.Time { return time.Unix(100, 0).UTC().Add(TicketTTL) }
	writeHub(t, conn, map[string]any{"action": realtimeActionSubscribe, "topic": "session." + hubSessionID})
	got := readHub(t, conn)
	if got["type"] != realtimeTypeError {
		t.Fatalf("expired ticket response = %v, want error", got)
	}
	payload, ok := got["payload"].(map[string]any)
	if !ok || payload["code"] != realtimeErrorUnauthorized {
		t.Fatalf("expired ticket payload = %v, want %s", got["payload"], realtimeErrorUnauthorized)
	}
}
