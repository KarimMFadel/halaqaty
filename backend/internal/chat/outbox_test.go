package chat

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/KarimMFadel/halaqaty/backend/internal/platform/logging"
	"github.com/KarimMFadel/halaqaty/backend/internal/platform/metrics"
	"github.com/KarimMFadel/halaqaty/backend/internal/realtime"
)

type fakeChatOutboxStore struct {
	due       []OutboxEvent
	replay    []OutboxEvent
	load      map[uuid.UUID]Message
	loadErr   map[uuid.UUID]error
	claimErr  error
	replayErr error

	delivered []uuid.UUID
	retried   []retrySchedule
	parked    []uuid.UUID
}

type retrySchedule struct {
	eventID     uuid.UUID
	availableAt time.Time
}

func (s *fakeChatOutboxStore) ClaimDueEvents(context.Context, int) ([]OutboxEvent, error) {
	return s.due, s.claimErr
}

func (s *fakeChatOutboxStore) ClaimReplayEvents(context.Context, int) ([]OutboxEvent, error) {
	if s.replayErr != nil {
		return nil, s.replayErr
	}
	// Mirror real Postgres claim semantics: the replay claim clears parked_at
	// (RETURNING yields post-update values) and reports the pre-claim parked
	// state via was_parked.
	events := make([]OutboxEvent, len(s.replay))
	for i, event := range s.replay {
		event.WasParked = event.ParkedAt != nil
		event.ParkedAt = nil
		events[i] = event
	}
	return events, nil
}

func (s *fakeChatOutboxStore) MarkDelivered(_ context.Context, eventID uuid.UUID) error {
	s.delivered = append(s.delivered, eventID)
	return nil
}

func (s *fakeChatOutboxStore) ScheduleRetry(_ context.Context, eventID uuid.UUID, availableAt time.Time) error {
	s.retried = append(s.retried, retrySchedule{eventID: eventID, availableAt: availableAt})
	return nil
}

func (s *fakeChatOutboxStore) Park(_ context.Context, eventID uuid.UUID) error {
	s.parked = append(s.parked, eventID)
	return nil
}

func (s *fakeChatOutboxStore) LoadMessage(_ context.Context, messageID uuid.UUID) (Message, error) {
	if err := s.loadErr[messageID]; err != nil {
		return Message{}, err
	}
	msg, ok := s.load[messageID]
	if !ok {
		return Message{}, ErrMessageNotProjectable
	}
	return msg, nil
}

type fakeChatProjector struct {
	err       error
	projected []uuid.UUID
	mu        sync.Mutex
}

func (p *fakeChatProjector) ProjectMessage(_ context.Context, event OutboxEvent, _ Message) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.err != nil {
		return p.err
	}
	p.projected = append(p.projected, event.EventID)
	return nil
}

func (p *fakeChatProjector) count() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return len(p.projected)
}

// recordingAuditHandler captures audit records so tests can assert redacted
// outbox outcomes without a live log sink.
type recordingAuditHandler struct {
	mu      sync.Mutex
	records []slog.Record
}

func (h *recordingAuditHandler) Enabled(context.Context, slog.Level) bool { return true }

func (h *recordingAuditHandler) Handle(_ context.Context, r slog.Record) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.records = append(h.records, r)
	return nil
}

func (h *recordingAuditHandler) WithAttrs([]slog.Attr) slog.Handler { return h }

func (h *recordingAuditHandler) WithGroup(string) slog.Handler { return h }

func (h *recordingAuditHandler) actions() []string {
	h.mu.Lock()
	defer h.mu.Unlock()
	actions := make([]string, 0, len(h.records))
	for _, record := range h.records {
		record.Attrs(func(a slog.Attr) bool {
			if a.Key == "action" {
				actions = append(actions, a.Value.String())
			}
			return true
		})
	}
	return actions
}

// rendered flattens every record to a string for redaction assertions.
func (h *recordingAuditHandler) rendered() string {
	h.mu.Lock()
	defer h.mu.Unlock()
	var builder strings.Builder
	for _, record := range h.records {
		builder.WriteString(record.Message)
		record.Attrs(func(a slog.Attr) bool {
			builder.WriteString(" ")
			builder.WriteString(a.Key)
			builder.WriteString("=")
			builder.WriteString(a.Value.String())
			return true
		})
	}
	return builder.String()
}

func identityJitter(delay time.Duration) time.Duration { return delay }

func newTestOutboxMessage() Message {
	circleID := uuid.MustParse(projectorCircleID)
	return Message{ID: uuid.New(), CircleID: &circleID, SenderID: uuid.New(), Type: MessageTypeText, Content: "outbox body must never leak", SentAt: time.Now().UTC()}
}

func TestOutboxDispatcher_DeliversClaimedEventAndRecordsMetrics(t *testing.T) {
	msg := newTestOutboxMessage()
	event := OutboxEvent{EventID: uuid.New(), MessageID: msg.ID, EventType: realtime.EventChatMessage, AttemptCount: 1}
	store := &fakeChatOutboxStore{due: []OutboxEvent{event}, load: map[uuid.UUID]Message{msg.ID: msg}}
	projector := &fakeChatProjector{}
	chatMetrics := &metrics.ChatMetrics{}
	dispatcher := NewOutboxDispatcher(store, projector, chatMetrics, nil, nil, nil)

	if err := dispatcher.DispatchDue(context.Background(), 10); err != nil {
		t.Fatalf("dispatch due: %v", err)
	}
	if len(store.delivered) != 1 || store.delivered[0] != event.EventID {
		t.Fatalf("delivered = %v, want [%s]", store.delivered, event.EventID)
	}
	if projector.count() != 1 {
		t.Fatalf("projection calls = %d, want 1", projector.count())
	}
	if got := chatMetrics.Summary().Outbox[metrics.ChatOutboxDelivered]; got != 1 {
		t.Fatalf("delivered metric = %d, want 1", got)
	}
}

// TestOutboxDispatcher_RetriesWithBoundedBackoffThenParks proves the five
// attempt bound: failures of attempts one to four schedule jittered 1/2/4/8
// second retries and the fifth failure parks the event.
func TestOutboxDispatcher_RetriesWithBoundedBackoffThenParks(t *testing.T) {
	now := time.Date(2026, 9, 7, 12, 0, 0, 0, time.UTC)
	msg := newTestOutboxMessage()
	store := &fakeChatOutboxStore{load: map[uuid.UUID]Message{msg.ID: msg}}
	projector := &fakeChatProjector{err: errors.New("hub unavailable")}
	chatMetrics := &metrics.ChatMetrics{}
	audit := &recordingAuditHandler{}
	dispatcher := NewOutboxDispatcher(store, projector, chatMetrics, logging.NewAuditLogger(slog.New(audit)), func() time.Time { return now }, identityJitter)

	eventID := uuid.New()
	for attempt := 1; attempt <= 5; attempt++ {
		event := OutboxEvent{EventID: eventID, MessageID: msg.ID, EventType: realtime.EventChatMessage, AttemptCount: attempt}
		if err := dispatcher.Dispatch(context.Background(), event); err != nil {
			t.Fatalf("dispatch attempt %d: %v", attempt, err)
		}
	}

	wantBackoffs := []time.Duration{time.Second, 2 * time.Second, 4 * time.Second, 8 * time.Second}
	if len(store.retried) != len(wantBackoffs) {
		t.Fatalf("retries = %d, want %d", len(store.retried), len(wantBackoffs))
	}
	for i, want := range wantBackoffs {
		if got := store.retried[i].availableAt; !got.Equal(now.Add(want)) {
			t.Fatalf("retry %d available_at = %v, want %v", i+1, got, now.Add(want))
		}
	}
	if len(store.parked) != 1 || store.parked[0] != eventID {
		t.Fatalf("parked = %v, want [%s]", store.parked, eventID)
	}
	if len(store.delivered) != 0 {
		t.Fatalf("failed deliveries must not be marked delivered: %v", store.delivered)
	}
	if got := chatMetrics.Summary().Outbox[metrics.ChatOutboxParked]; got != 1 {
		t.Fatalf("parked metric = %d, want 1", got)
	}
	if got := chatMetrics.Summary().Outbox[metrics.ChatOutboxRetried]; got != 4 {
		t.Fatalf("retried metric = %d, want 4", got)
	}
	rendered := audit.rendered()
	if !strings.Contains(rendered, string(logging.ChatOutcomeParked)) || !strings.Contains(rendered, eventID.String()) {
		t.Fatalf("parked audit must record outcome and event id: %s", rendered)
	}
	if strings.Contains(rendered, msg.Content) {
		t.Fatalf("parked audit must never contain message bodies: %s", rendered)
	}
}

// TestOutboxDispatcher_RetryDelayCappedAtThirtySeconds proves the jittered
// backoff ceiling from the delivery contract.
func TestOutboxDispatcher_RetryDelayCappedAtThirtySeconds(t *testing.T) {
	now := time.Date(2026, 9, 7, 12, 0, 0, 0, time.UTC)
	msg := newTestOutboxMessage()
	event := OutboxEvent{EventID: uuid.New(), MessageID: msg.ID, EventType: realtime.EventChatMessage, AttemptCount: 4}
	store := &fakeChatOutboxStore{load: map[uuid.UUID]Message{msg.ID: msg}}
	dispatcher := NewOutboxDispatcher(store, &fakeChatProjector{err: errors.New("hub unavailable")}, nil, nil,
		func() time.Time { return now }, func(time.Duration) time.Duration { return 45 * time.Second })

	if err := dispatcher.Dispatch(context.Background(), event); err != nil {
		t.Fatalf("dispatch: %v", err)
	}
	if len(store.retried) != 1 {
		t.Fatalf("retries = %d, want 1", len(store.retried))
	}
	if got := store.retried[0].availableAt; !got.Equal(now.Add(30 * time.Second)) {
		t.Fatalf("capped available_at = %v, want %v", got, now.Add(30*time.Second))
	}
}

// TestOutboxDispatcher_SkipsUnprojectableMessages proves payload reload from
// PostgreSQL truth: an event whose message is no longer projectable (deleted)
// completes without a projection instead of retrying forever.
func TestOutboxDispatcher_SkipsUnprojectableMessages(t *testing.T) {
	event := OutboxEvent{EventID: uuid.New(), MessageID: uuid.New(), EventType: realtime.EventChatMessage, AttemptCount: 1}
	store := &fakeChatOutboxStore{due: []OutboxEvent{event}}
	projector := &fakeChatProjector{}
	dispatcher := NewOutboxDispatcher(store, projector, nil, nil, nil, nil)

	if err := dispatcher.DispatchDue(context.Background(), 10); err != nil {
		t.Fatalf("dispatch due: %v", err)
	}
	if len(store.delivered) != 1 || store.delivered[0] != event.EventID {
		t.Fatalf("skipped event must be marked delivered: %v", store.delivered)
	}
	if projector.count() != 0 {
		t.Fatalf("deleted messages must not be projected, got %d calls", projector.count())
	}
	if len(store.retried) != 0 || len(store.parked) != 0 {
		t.Fatalf("skipped event must not retry or park: %v / %v", store.retried, store.parked)
	}
}

// TestOutboxDispatcher_ReplayDeliversParkedEvents proves startup replay:
// parked rows are re-armed and delivered, and the recovery is audited for
// parked rows only. The fake mirrors real Postgres claim semantics (parked_at
// cleared, pre-claim state carried by WasParked), so this exercises the same
// path the production scan takes.
func TestOutboxDispatcher_ReplayDeliversParkedEvents(t *testing.T) {
	msg := newTestOutboxMessage()
	parkedAt := time.Now().UTC().Add(-time.Hour)
	parked := OutboxEvent{EventID: uuid.New(), MessageID: msg.ID, EventType: realtime.EventChatMessage, AttemptCount: 5, ParkedAt: &parkedAt}
	pending := OutboxEvent{EventID: uuid.New(), MessageID: msg.ID, EventType: realtime.EventChatMessage, AttemptCount: 1}
	store := &fakeChatOutboxStore{replay: []OutboxEvent{parked, pending}, load: map[uuid.UUID]Message{msg.ID: msg}}
	audit := &recordingAuditHandler{}
	dispatcher := NewOutboxDispatcher(store, &fakeChatProjector{}, nil, logging.NewAuditLogger(slog.New(audit)), nil, nil)

	if err := dispatcher.Replay(context.Background(), 10); err != nil {
		t.Fatalf("replay: %v", err)
	}
	if len(store.delivered) != 2 {
		t.Fatalf("replayed events must be delivered: %v", store.delivered)
	}
	actions := audit.actions()
	if len(actions) != 1 || actions[0] != logging.ActionChatOutbox {
		t.Fatalf("recovery audit actions = %v, want one chat.outbox record for the parked event only", actions)
	}
	if !strings.Contains(audit.rendered(), string(logging.ChatOutcomeRecovered)) {
		t.Fatalf("recovery audit must record the recovered outcome: %s", audit.rendered())
	}
}

// TestOutboxDispatcher_RunSurvivesStartupReplayFailure proves a startup
// replay store error is non-fatal: the failure outcome is recorded once and
// the periodic loop keeps dispatching due events until ctx cancel.
func TestOutboxDispatcher_RunSurvivesStartupReplayFailure(t *testing.T) {
	msg := newTestOutboxMessage()
	dueEvent := OutboxEvent{EventID: uuid.New(), MessageID: msg.ID, EventType: realtime.EventChatMessage, AttemptCount: 1}
	store := &fakeChatOutboxStore{replayErr: errors.New("db restarting"), due: []OutboxEvent{dueEvent}, load: map[uuid.UUID]Message{msg.ID: msg}}
	chatMetrics := &metrics.ChatMetrics{}
	dispatcher := NewOutboxDispatcher(store, &fakeChatProjector{}, chatMetrics, nil, nil, nil)

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		<-time.After(30 * time.Millisecond)
		cancel()
	}()
	if err := dispatcher.Run(ctx, 10, 5*time.Millisecond); err != nil {
		t.Fatalf("run must survive startup replay failure: %v", err)
	}
	if !containsUUID(store.delivered, dueEvent.EventID) {
		t.Fatalf("periodic loop must keep dispatching after replay failure: %v", store.delivered)
	}
	if got := chatMetrics.Summary().Outcomes[metrics.ChatOutcomeFailure]; got != 1 {
		t.Fatalf("replay failure outcome = %d, want 1", got)
	}
}

// TestOutboxDispatcher_RunReplaysThenDispatchesUntilCancel proves the
// dispatcher lifecycle: startup replay runs before the periodic loop and the
// loop stops on context cancellation.
func TestOutboxDispatcher_RunReplaysThenDispatchesUntilCancel(t *testing.T) {
	msg := newTestOutboxMessage()
	replayEvent := OutboxEvent{EventID: uuid.New(), MessageID: msg.ID, EventType: realtime.EventChatMessage, AttemptCount: 1}
	dueEvent := OutboxEvent{EventID: uuid.New(), MessageID: msg.ID, EventType: realtime.EventChatMessage, AttemptCount: 1}
	store := &fakeChatOutboxStore{replay: []OutboxEvent{replayEvent}, due: []OutboxEvent{dueEvent}, load: map[uuid.UUID]Message{msg.ID: msg}}
	dispatcher := NewOutboxDispatcher(store, &fakeChatProjector{}, nil, nil, nil, nil)

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		<-time.After(30 * time.Millisecond)
		cancel()
	}()
	if err := dispatcher.Run(ctx, 10, 5*time.Millisecond); err != nil {
		t.Fatalf("run: %v", err)
	}
	if !containsUUID(store.delivered, replayEvent.EventID) {
		t.Fatalf("startup replay must deliver the parked backlog: %v", store.delivered)
	}
	if !containsUUID(store.delivered, dueEvent.EventID) {
		t.Fatalf("periodic loop must deliver due events: %v", store.delivered)
	}
}

func containsUUID(ids []uuid.UUID, want uuid.UUID) bool {
	for _, id := range ids {
		if id == want {
			return true
		}
	}
	return false
}

// TestDefaultChatOutboxJitterStaysWithinTenPercent proves jitter never breaks
// the configured exponential schedule by more than ±10%.
func TestDefaultChatOutboxJitterStaysWithinTenPercent(t *testing.T) {
	delay := 10 * time.Second
	for range 100 {
		got := defaultChatOutboxJitter(delay)
		if got < 9*time.Second || got > 11*time.Second {
			t.Fatalf("jittered delay = %v, want within [9s, 11s]", got)
		}
	}
	if got := defaultChatOutboxJitter(0); got != 0 {
		t.Fatalf("zero delay must stay zero, got %v", got)
	}
}

// TestChatOutboxQueries_NeverReferenceMessageBodies is a supplemental
// source-policy guard for the ADR-021 security requirement that chat outbox
// persistence stays identifier-only (SR-006: no bodies, object keys, URLs, or
// credentials in events). Behavioral proof lives in the type system (the
// OutboxEvent struct carries no content field) and the migration schema (the
// chat_event_outbox table has no content column); this bounded scan covers
// only the named SQL constants and catches a future edit adding a
// content-bearing column to them. It is not project-wide proof.
func TestChatOutboxQueries_NeverReferenceMessageBodies(t *testing.T) {
	const outboxQueries = "outboxQueries"
	queries := map[string]string{
		"insert":    insertChatOutboxEventQuery,
		"claim":     claimOutboxEventsQuery,
		"replay":    claimReplayChatOutboxEventsQuery,
		"delivered": markChatOutboxEventDeliveredQuery,
		"retry":     retryChatOutboxEventQuery,
		"park":      parkChatOutboxEventQuery,
		"columns":   outboxColumns,
	}
	for name, query := range queries {
		lower := strings.ToLower(query)
		for _, forbidden := range []string{"content", "body", "object_key", "url", "token", "credential"} {
			if strings.Contains(lower, forbidden) {
				t.Errorf("%s query %q must never reference %q", outboxQueries, name, forbidden)
			}
		}
	}
}
