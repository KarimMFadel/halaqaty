//go:build integration

package queue

import (
	"context"
	"encoding/json"
	"errors"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gorilla/websocket"

	"github.com/KarimMFadel/halaqaty/backend/internal/realtime"
)

func TestRealtimeOutboxProjector_DeliversVersionedRedactedEventsToAuthorizedRecipients(t *testing.T) {
	ctx := context.Background()
	repo := newQueueRepository(t)
	teacherID := qSeedUser(t, repo, "realtime-teacher")
	selectedStudentID := qSeedUser(t, repo, "realtime-selected")
	nextStudentID := qSeedUser(t, repo, "realtime-next")
	circleID := qSeedCircle(t, repo, teacherID)
	for _, member := range []struct{ id, role string }{
		{teacherID, "teacher"},
		{selectedStudentID, "student"},
		{nextStudentID, "student"},
	} {
		qSeedMember(t, repo, circleID, member.id, member.role, time.Now().UTC())
	}
	sessionID := qInsertSession(t, repo, circleID, teacherID, "managers_and_student")
	round := qCreateRound(t, repo, sessionID, teacherID, "active", []string{selectedStudentID, nextStudentID})

	tickets := realtime.NewTicketService(outboxRealtimeTicketReader{circleID: circleID})
	hub := realtime.NewHub(tickets, outboxRealtimeAuthorizer{allowed: map[string]bool{
		teacherID: true, selectedStudentID: true, nextStudentID: true,
	}})
	projector := NewRealtimeOutboxProjector(repo, hub)
	hub.RegisterSessionEventProvider(projector.QueueState)
	server := httptest.NewServer(hub)
	defer server.Close()

	teacher := outboxRealtimeConnect(t, server, tickets, teacherID, sessionID)
	defer func() { _ = teacher.Close() }()
	selected := outboxRealtimeConnect(t, server, tickets, selectedStudentID, sessionID)
	defer func() { _ = selected.Close() }()
	next := outboxRealtimeConnect(t, server, tickets, nextStudentID, sessionID)
	defer func() { _ = next.Close() }()

	turns := NewTurnService(repo)
	advanced, err := turns.Advance(ctx, round.ID, round.Version)
	if err != nil {
		t.Fatalf("advance: %v", err)
	}
	if _, err := turns.Start(ctx, *advanced.SelectedEntryID, 1); err != nil {
		t.Fatalf("start: %v", err)
	}
	events, err := repo.ClaimDueOutboxEvents(ctx, 10)
	if err != nil {
		t.Fatalf("claim committed outbox events: %v", err)
	}
	if len(events) != 2 || events[0].RoundVersion != 2 || events[1].RoundVersion != 3 {
		t.Fatalf("claimed event versions = %#v, want [2 3]", events)
	}
	dispatcher := NewOutboxDispatcher(repo, projector, nil, nil, nil, nil)
	for _, event := range events {
		if err := dispatcher.Dispatch(ctx, event); err != nil {
			t.Fatalf("dispatch %s: %v", event.EventType, err)
		}
	}

	teacherEvents := []map[string]any{outboxRealtimeRead(t, teacher), outboxRealtimeRead(t, teacher)}
	selectedEvents := []map[string]any{outboxRealtimeRead(t, selected), outboxRealtimeRead(t, selected), outboxRealtimeRead(t, selected)}
	nextEvents := []map[string]any{outboxRealtimeRead(t, next), outboxRealtimeRead(t, next), outboxRealtimeRead(t, next), outboxRealtimeRead(t, next)}

	outboxRealtimeAssertEvent(t, teacherEvents[0], "queue.advanced", events[0].EventID, 2)
	outboxRealtimeAssertEvent(t, teacherEvents[1], "queue.entry_updated", events[1].EventID, 3)
	outboxRealtimeAssertEvent(t, selectedEvents[0], "queue.advanced", events[0].EventID, 2)
	outboxRealtimeAssertEvent(t, selectedEvents[1], "queue.entry_updated", events[1].EventID, 3)
	outboxRealtimeAssertEvent(t, selectedEvents[2], "queue.your_turn", "", 3)
	outboxRealtimeAssertEvent(t, nextEvents[0], "queue.advanced", events[0].EventID, 2)
	outboxRealtimeAssertEvent(t, nextEvents[1], "queue.next_soon", "", 2)
	outboxRealtimeAssertEvent(t, nextEvents[2], "queue.entry_updated", events[1].EventID, 3)
	outboxRealtimeAssertEvent(t, nextEvents[3], "queue.next_soon", "", 3)

	updatedPayload, ok := teacherEvents[1]["payload"].(map[string]any)
	if !ok || updatedPayload["student_id"] != selectedStudentID || updatedPayload["position"] != float64(1) || updatedPayload["entry_version"] != float64(2) {
		t.Fatalf("entry update must be server-built from committed state: %v", teacherEvents[1])
	}
	for _, event := range append(append([]map[string]any{}, teacherEvents...), append(selectedEvents, nextEvents...)...) {
		outboxRealtimeAssertNoSensitiveFields(t, event)
	}
}

func TestRealtimeOutboxProjector_QueueStateUsesWebSocketEntryIdentifiers(t *testing.T) {
	ctx := context.Background()
	repo := newQueueRepository(t)
	teacherID := qSeedUser(t, repo, "queue-state-teacher")
	studentID := qSeedUser(t, repo, "queue-state-student")
	circleID := qSeedCircle(t, repo, teacherID)
	qSeedMember(t, repo, circleID, teacherID, "teacher", time.Now().UTC())
	qSeedMember(t, repo, circleID, studentID, "student", time.Now().UTC())
	sessionID := qInsertSession(t, repo, circleID, teacherID, "managers_and_student")
	qCreateRound(t, repo, sessionID, teacherID, "active", []string{studentID})

	event, err := NewRealtimeOutboxProjector(repo, nil).QueueState(ctx, teacherID, sessionID)
	if err != nil {
		t.Fatalf("project queue state: %v", err)
	}
	payload, ok := event["payload"].(map[string]any)
	if !ok {
		t.Fatalf("queue state payload = %#v, want object", event["payload"])
	}
	entries, ok := payload["entries"].([]map[string]any)
	if !ok || len(entries) != 1 {
		t.Fatalf("queue state entries = %#v, want one entry", payload["entries"])
	}
	entryID, ok := entries[0]["queue_entry_id"].(string)
	if !ok || entryID == "" {
		t.Fatalf("queue state entry = %#v, want queue_entry_id", entries[0])
	}
	if _, present := entries[0]["id"]; present {
		t.Fatalf("queue state entry = %#v, REST id must not leak into WebSocket shape", entries[0])
	}
}

func TestPolicyUpdate_CommitsQueuePolicyChangedOutboxEventForActiveRound(t *testing.T) {
	ctx := context.Background()
	repo := newQueueRepository(t)
	teacherID := qSeedUser(t, repo, "realtime-policy-teacher")
	circleID := qSeedCircle(t, repo, teacherID)
	qSeedMember(t, repo, circleID, teacherID, "teacher", time.Now().UTC())
	sessionID := qInsertSession(t, repo, circleID, teacherID, "managers_and_student")
	round := qCreateRound(t, repo, sessionID, teacherID, "active", nil)
	if _, err := repo.pool.Exec(ctx, `UPDATE sessions SET status = 'active' WHERE id = $1::uuid`, sessionID); err != nil {
		t.Fatalf("activate session: %v", err)
	}
	policyCtx, err := repo.SessionPolicy(ctx, sessionID)
	if err != nil {
		t.Fatalf("load policy: %v", err)
	}
	policy := policyCtx.Policy
	policy.OptOut = OptOutPolicyAutoApprove
	if _, err := NewPolicyService(repo).Update(ctx, sessionID, teacherID, policy.Version, policy); err != nil {
		t.Fatalf("update policy: %v", err)
	}
	events, err := repo.ClaimDueOutboxEvents(ctx, 10)
	if err != nil {
		t.Fatalf("claim policy outbox: %v", err)
	}
	if len(events) != 1 || events[0].EventType != "queue.policy_changed" || events[0].RoundID != round.ID || events[0].RoundVersion != policy.Version+1 {
		t.Fatalf("policy outbox event = %#v, want committed queue.policy_changed for active round", events)
	}
}

type outboxRealtimeTicketReader struct{ circleID string }

func (r outboxRealtimeTicketReader) ListCircleIDs(context.Context, string) ([]string, error) {
	return []string{r.circleID}, nil
}

type outboxRealtimeAuthorizer struct{ allowed map[string]bool }

func (a outboxRealtimeAuthorizer) AuthorizeSessionTopic(_ context.Context, userID, _ string) error {
	if !a.allowed[userID] {
		return errors.New("session topic unauthorized")
	}
	return nil
}

func outboxRealtimeConnect(t *testing.T, server *httptest.Server, tickets *realtime.TicketService, userID, sessionID string) *websocket.Conn {
	t.Helper()
	ticket, err := tickets.Issue(context.Background(), userID)
	if err != nil {
		t.Fatalf("issue ticket: %v", err)
	}
	conn, _, err := websocket.DefaultDialer.Dial("ws"+server.URL[len("http"):]+"?token="+ticket.Token, nil)
	if err != nil {
		t.Fatalf("dial hub: %v", err)
	}
	if err := conn.WriteJSON(map[string]any{"action": "subscribe", "topic": "session." + sessionID}); err != nil {
		t.Fatalf("subscribe: %v", err)
	}
	if event := outboxRealtimeRead(t, conn); event["type"] != "subscribed" {
		t.Fatalf("subscribe response = %v", event)
	}
	if event := outboxRealtimeRead(t, conn); event["type"] != "queue.state" {
		t.Fatalf("queue state = %v", event)
	}
	return conn
}

func outboxRealtimeRead(t *testing.T, conn *websocket.Conn) map[string]any {
	t.Helper()
	if err := conn.SetReadDeadline(time.Now().Add(2 * time.Second)); err != nil {
		t.Fatalf("set read deadline: %v", err)
	}
	_, raw, err := conn.ReadMessage()
	if err != nil {
		t.Fatalf("read realtime event: %v", err)
	}
	var event map[string]any
	if err := json.Unmarshal(raw, &event); err != nil {
		t.Fatalf("decode realtime event: %v", err)
	}
	return event
}

func outboxRealtimeAssertEvent(t *testing.T, event map[string]any, eventType, eventID string, version int64) {
	t.Helper()
	if got := event["type"]; got != eventType {
		t.Fatalf("event type = %v, want %q", got, eventType)
	}
	if eventID != "" && event["event_id"] != eventID {
		t.Fatalf("event id = %v, want %q", event["event_id"], eventID)
	}
	if eventID == "" && event["event_id"] == "" {
		t.Fatalf("targeted %s event must have a stable event_id: %v", eventType, event)
	}
	payload, ok := event["payload"].(map[string]any)
	if !ok || payload["version"] != float64(version) {
		t.Fatalf("event payload version = %v, want %d", event["payload"], version)
	}
	if _, ok := event["occurred_at"].(string); !ok {
		t.Fatalf("event occurred_at = %v, want UTC timestamp", event["occurred_at"])
	}
}

func outboxRealtimeAssertNoSensitiveFields(t *testing.T, event map[string]any) {
	t.Helper()
	var walk func(map[string]any)
	walk = func(values map[string]any) {
		for key, value := range values {
			switch key {
			case "grade", "notes", "grade_notes", "media", "credential", "room", "endpoint", "provider", "url":
				t.Fatalf("event leaked prohibited field %q: %v", key, event)
			}
			if child, ok := value.(map[string]any); ok {
				walk(child)
			}
		}
	}
	walk(event)
}
