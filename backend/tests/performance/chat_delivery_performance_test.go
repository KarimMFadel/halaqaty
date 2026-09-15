//go:build integration

package performance

import (
	"context"
	"fmt"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/KarimMFadel/halaqaty/backend/internal/chat"
	"github.com/KarimMFadel/halaqaty/backend/internal/platform/metrics"
	"github.com/KarimMFadel/halaqaty/backend/internal/rbac"
	"github.com/KarimMFadel/halaqaty/backend/internal/realtime"
	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestChatDeliveryPerformance_SC006(t *testing.T) {
	f := newChatPerformanceFixture(t)
	ctx := context.Background()
	userID, circleID := f.users[0][0], f.circles[0]
	tickets := realtime.NewTicketService(chatDeliveryCircleReader{pool: f.pool})
	hub := realtime.NewHub(tickets, nil)
	projector := chat.NewRealtimeProjector(rbac.NewRepository(f.pool), hub, tickets, chatDeliverySessionValid)
	dispatcher := chat.NewOutboxDispatcher(chat.NewPGOutboxStore(f.repo), projector, new(metrics.ChatMetrics), nil, nil, func(delay time.Duration) time.Duration { return delay })
	server := httptest.NewServer(hub)
	defer server.Close()
	connection := subscribeChatDelivery(t, ctx, server.URL, tickets, userID.String(), circleID.String())
	defer connection.Close()

	for i := 0; i < chatPerformanceWarmups; i++ {
		sendAndReceiveChatDelivery(t, ctx, f, dispatcher, connection, userID, circleID, "warmup", i)
	}
	latencies := make([]time.Duration, 0, chatPerformanceSamples)
	for i := 0; i < chatPerformanceSamples; i++ {
		started := time.Now()
		sendAndReceiveChatDelivery(t, ctx, f, dispatcher, connection, userID, circleID, "sample", i)
		latencies = append(latencies, time.Since(started))
	}
	p95 := percentile95(latencies)
	t.Logf("SC-006 commit-to-client warmups=%d samples=%d p95=%s", chatPerformanceWarmups, chatPerformanceSamples, p95)
	if p95 > chatPerformanceP95 {
		t.Fatalf("SC-006 commit-to-client p95=%s, want <=%s", p95, chatPerformanceP95)
	}

	if err := connection.Close(); err != nil {
		t.Fatalf("suppress realtime connection: %v", err)
	}
	message, err := f.service.SendText(ctx, userID, circleID, "suppressed realtime recovery", "suppressed-recovery")
	if err != nil {
		t.Fatalf("commit suppressed realtime message: %v", err)
	}
	if err := dispatcher.DispatchDue(ctx, 1); err != nil {
		t.Fatalf("dispatch suppressed realtime message: %v", err)
	}
	history, err := f.service.History(ctx, userID, circleID, nil, 100)
	if err != nil {
		t.Fatalf("recover REST history after suppression: %v", err)
	}
	for _, recovered := range history {
		if recovered.ID == message.ID {
			return
		}
	}
	t.Fatal("REST recovery did not return the durably committed suppressed event")
}

func chatDeliverySessionValid(context.Context, string, string) (bool, error) { return true, nil }

func sendAndReceiveChatDelivery(t *testing.T, ctx context.Context, f *chatPerformanceFixture, dispatcher *chat.OutboxDispatcher, connection *websocket.Conn, userID, circleID uuid.UUID, prefix string, index int) {
	t.Helper()
	message, err := f.service.SendText(ctx, userID, circleID, fmt.Sprintf("%s %d", prefix, index), fmt.Sprintf("delivery-%s-%d", prefix, index))
	if err != nil {
		t.Fatalf("commit %s %d: %v", prefix, index, err)
	}
	if err := dispatcher.DispatchDue(ctx, 1); err != nil {
		t.Fatalf("dispatch %s %d: %v", prefix, index, err)
	}
	readChatDeliveryEvent(t, connection, message.ID.String())
}

type chatDeliveryCircleReader struct{ pool *pgxpool.Pool }

func (r chatDeliveryCircleReader) ListCircleIDs(ctx context.Context, userID string) ([]string, error) {
	rows, err := r.pool.Query(ctx, "SELECT circle_id::text FROM circle_members WHERE user_id = $1::uuid", userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var circleIDs []string
	for rows.Next() {
		var circleID string
		if err := rows.Scan(&circleID); err != nil {
			return nil, err
		}
		circleIDs = append(circleIDs, circleID)
	}
	return circleIDs, rows.Err()
}

func subscribeChatDelivery(t *testing.T, ctx context.Context, serverURL string, tickets *realtime.TicketService, userID, circleID string) *websocket.Conn {
	t.Helper()
	ticket, err := tickets.IssueForSession(ctx, userID, "chat-performance-session")
	if err != nil {
		t.Fatalf("issue chat delivery ticket: %v", err)
	}
	connection, _, err := websocket.DefaultDialer.Dial(strings.Replace(serverURL, "http://", "ws://", 1)+"?token="+ticket.Token, nil)
	if err != nil {
		t.Fatalf("dial chat delivery client: %v", err)
	}
	if err := connection.WriteJSON(map[string]any{"action": "subscribe", "topic": "circle." + circleID}); err != nil {
		connection.Close()
		t.Fatalf("subscribe chat delivery client: %v", err)
	}
	readChatDeliveryEvent(t, connection, "")
	return connection
}

func readChatDeliveryEvent(t *testing.T, connection *websocket.Conn, expectedMessageID string) {
	t.Helper()
	if err := connection.SetReadDeadline(time.Now().Add(3 * time.Second)); err != nil {
		t.Fatalf("set chat delivery deadline: %v", err)
	}
	for {
		var event map[string]any
		if err := connection.ReadJSON(&event); err != nil {
			t.Fatalf("read chat delivery event: %v", err)
		}
		if expectedMessageID == "" && event["type"] == "subscribed" {
			return
		}
		payload, _ := event["payload"].(map[string]any)
		if event["type"] == realtime.EventChatMessage && payload["id"] == expectedMessageID {
			return
		}
	}
}
