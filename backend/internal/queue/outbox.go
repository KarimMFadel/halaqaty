package queue

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math/rand"
	"strconv"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/KarimMFadel/halaqaty/backend/internal/platform/metrics"
	"github.com/KarimMFadel/halaqaty/backend/internal/realtime"
)

const outboxRetryLimit = 5

// OutboxStore is the small persistence seam required by asynchronous delivery.
type OutboxStore interface {
	ClaimReplayOutboxEvents(context.Context, int) ([]OutboxEvent, error)
	MarkOutboxDelivered(context.Context, string) error
	RetryOutboxEvent(context.Context, string, time.Time) error
	ParkOutboxEvent(context.Context, string) error
}

// OutboxProjector performs one committed outbox operation. Client event
// projectors reconstruct visibility-filtered payloads from PostgreSQL. Metadata
// alone is never a client payload because it excludes grade, notes, and names.
type OutboxProjector interface {
	ProjectAndDeliver(context.Context, OutboxEvent) error
}

// ParkedOutboxAlerter receives retry-exhaustion notifications.
type ParkedOutboxAlerter interface {
	AlertOutboxParked(context.Context, OutboxEvent)
}

// OutboxDispatcher performs at-least-once delivery without changing
// committed queue truth when realtime delivery fails.
type OutboxDispatcher struct {
	store     OutboxStore
	projector OutboxProjector
	metrics   *metrics.QueueMetrics
	alerter   ParkedOutboxAlerter
	now       func() time.Time
	jitter    func(time.Duration) time.Duration
}

// NewOutboxDispatcher constructs a dispatcher. now and jitter are injected
// to keep retry behavior deterministic in tests.
func NewOutboxDispatcher(store OutboxStore, projector OutboxProjector, queueMetrics *metrics.QueueMetrics, alerter ParkedOutboxAlerter, now func() time.Time, jitter func(time.Duration) time.Duration) *OutboxDispatcher {
	if now == nil {
		now = time.Now
	}
	if jitter == nil {
		jitter = defaultOutboxJitter
	}
	return &OutboxDispatcher{store: store, projector: projector, metrics: queueMetrics, alerter: alerter, now: now, jitter: jitter}
}

// defaultOutboxJitter applies bounded +/-10% jitter to avoid synchronized
// retry bursts while preserving the configured exponential schedule.
func defaultOutboxJitter(delay time.Duration) time.Duration {
	if delay <= 0 {
		return 0
	}
	delta := delay / 10
	if delta == 0 {
		return delay
	}
	return delay - delta + time.Duration(rand.Int63n(int64(2*delta)+1))
}

// Replay processes pending and parked rows after startup recovery.
func (d *OutboxDispatcher) Replay(ctx context.Context, limit int) error {
	if d.store == nil || d.projector == nil {
		return fmt.Errorf("outbox dispatcher requires store and projector")
	}
	events, err := d.store.ClaimReplayOutboxEvents(ctx, limit)
	if err != nil {
		return fmt.Errorf("claim outbox replay: %w", err)
	}
	for _, event := range events {
		if err := d.Dispatch(ctx, event); err != nil {
			return err
		}
	}
	return nil
}

// Dispatch delivers one committed event or schedules its retry/parking.
func (d *OutboxDispatcher) Dispatch(ctx context.Context, event OutboxEvent) error {
	if d.store == nil || d.projector == nil {
		return fmt.Errorf("outbox dispatcher requires store and projector")
	}
	if err := d.projector.ProjectAndDeliver(ctx, event); err == nil {
		if err := d.store.MarkOutboxDelivered(ctx, event.EventID); err != nil {
			return fmt.Errorf("mark outbox event delivered: %w", err)
		}
		return nil
	}
	if event.AttemptCount >= outboxRetryLimit {
		if err := d.store.ParkOutboxEvent(ctx, event.EventID); err != nil {
			return fmt.Errorf("park exhausted outbox event: %w", err)
		}
		if d.metrics != nil {
			d.metrics.RecordOutboxParked()
		}
		if d.alerter != nil {
			d.alerter.AlertOutboxParked(ctx, event)
		}
		return nil
	}
	delay := time.Second << event.AttemptCount
	delay = d.jitter(delay)
	if delay < 0 {
		delay = 0
	}
	if err := d.store.RetryOutboxEvent(ctx, event.EventID, d.now().UTC().Add(delay)); err != nil {
		return fmt.Errorf("schedule outbox retry: %w", err)
	}
	return nil
}

// RealtimeOutboxProjector reconstructs redacted client events from committed
// queue state before delivering them through the authenticated session hub.
type RealtimeOutboxProjector struct {
	repo *Repository
	hub  *realtime.Hub
}

// NewRealtimeOutboxProjector constructs the US1 queue-to-hub projector.
func NewRealtimeOutboxProjector(repo *Repository, hub *realtime.Hub) *RealtimeOutboxProjector {
	return &RealtimeOutboxProjector{repo: repo, hub: hub}
}

// QueueState supplies the visibility-filtered snapshot sent after an authorized
// session-topic subscription.
func (p *RealtimeOutboxProjector) QueueState(ctx context.Context, actorID, sessionID string) (map[string]any, error) {
	if p == nil || p.repo == nil {
		return nil, errors.New("queue realtime projector is not configured")
	}
	role, err := p.repo.SessionRole(ctx, sessionID, actorID)
	if err != nil {
		return nil, err
	}
	if role == "" {
		return nil, errors.New("queue session topic unauthorized")
	}
	round, err := p.repo.CurrentRound(ctx, sessionID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	state, err := p.repo.LoadQueueState(ctx, round.ID, Viewer{UserID: actorID, IsManager: role == "teacher" || role == "supervisor"})
	if err != nil {
		return nil, err
	}
	payload, err := queueStateResponse(ctx, p.repo, state)
	if err != nil {
		return nil, err
	}
	return realtimeEnvelope(realtime.EventQueueState, projectedEventID(round.ID+":"+strconv.FormatInt(state.Round.Version, 10), actorID, realtime.EventQueueState), payload), nil
}

// ProjectAndDeliver projects one committed US1 outbox row through the hub.
func (p *RealtimeOutboxProjector) ProjectAndDeliver(ctx context.Context, event OutboxEvent) error {
	if p == nil || p.repo == nil || p.hub == nil {
		return errors.New("queue realtime projector is not configured")
	}
	if !isUS1QueueEvent(event.EventType) {
		return nil
	}
	topic, err := realtime.NewSessionTopic(event.SessionID)
	if err != nil {
		return fmt.Errorf("build queue session topic: %w", err)
	}
	state, err := p.repo.LoadQueueState(ctx, event.RoundID, Viewer{IsManager: true})
	if err != nil {
		return fmt.Errorf("load queue event state: %w", err)
	}
	metadata, err := queueEventMetadata(event.EventMetadata)
	if err != nil {
		return err
	}
	payload, err := p.eventPayload(ctx, event, state, metadata)
	if err != nil {
		return err
	}
	envelope := realtimeEnvelope(event.EventType, event.EventID, payload)
	if event.EventType == realtime.EventQueueReordered && metadataString(metadata, "order_kind") == "preorder_students" {
		managers, err := p.repo.SessionManagerIDs(ctx, event.SessionID)
		if err != nil {
			return err
		}
		for _, managerID := range managers {
			if err := p.hub.SendToUser(topic, managerID, event.EventID, envelope); err != nil {
				return err
			}
		}
	} else if err := p.hub.Broadcast(topic, event.EventID, envelope); err != nil {
		return err
	}
	return p.deliverTurnPrompts(ctx, topic, event, state, metadata)
}

func (p *RealtimeOutboxProjector) eventPayload(ctx context.Context, event OutboxEvent, state QueueState, metadata map[string]any) (map[string]any, error) {
	round := state.Round
	base := map[string]any{"session_id": event.SessionID, "round_id": event.RoundID, "version": event.RoundVersion}
	switch event.EventType {
	case realtime.EventQueueRoundStarted:
		name, err := p.repo.SurahName(ctx, round.SurahID)
		if err != nil {
			return nil, err
		}
		base["round_number"] = round.RoundNumber
		base["round_type"] = round.Type
		base["lifecycle"] = round.Lifecycle
		base["surah_id"] = round.SurahID
		base["surah_name"] = name
		base["from_ayah"] = round.FromAyah
		base["to_ayah"] = round.ToAyah
		base["grading_required"] = round.GradingRequired
	case realtime.EventQueueReordered:
		kind := metadataString(metadata, "order_kind")
		base["order_kind"] = kind
		base["ordered_ids"] = orderedQueueIDs(state, kind)
	case realtime.EventQueueAdvanced:
		base["selected_entry_id"] = selectedEntryID(event, round, metadata)
	case realtime.EventQueueEntryUpdated:
		entry, ok := queueEntry(state, event.ResourceID)
		if !ok {
			return nil, errors.New("queue entry update resource is unavailable")
		}
		base["queue_entry_id"] = entry.ID
		base["student_id"] = entry.StudentID
		base["old_status"] = metadataString(metadata, "old_status")
		base["new_status"] = metadataString(metadata, "new_status")
		base["position"] = entry.Position
		base["entry_version"] = entry.Version
	case realtime.EventQueuePolicyChanged:
		policy, err := p.repo.SessionPolicy(ctx, event.SessionID)
		if err != nil {
			return nil, err
		}
		base = map[string]any{"session_id": event.SessionID, "policy": policyResponse(policy.Policy)}
	default:
		return nil, errors.New("unsupported queue event")
	}
	return base, nil
}

func (p *RealtimeOutboxProjector) deliverTurnPrompts(ctx context.Context, topic realtime.Topic, event OutboxEvent, state QueueState, metadata map[string]any) error {
	if event.EventType == realtime.EventQueueAdvanced || event.EventType == realtime.EventQueueReordered {
		entry, ok := nextSoonEntry(state)
		if ok {
			return p.sendNextSoon(topic, event, entry)
		}
		return nil
	}
	if event.EventType != realtime.EventQueueEntryUpdated || metadataString(metadata, "new_status") != string(EntryStatusReciting) {
		return nil
	}
	entry, ok := queueEntry(state, event.ResourceID)
	if !ok {
		return nil
	}
	name, err := p.repo.SurahName(ctx, state.Round.SurahID)
	if err != nil {
		return err
	}
	yourTurn := map[string]any{"session_id": event.SessionID, "round_id": event.RoundID, "queue_entry_id": entry.ID,
		"round_type": state.Round.Type, "surah_id": state.Round.SurahID, "surah_name": name,
		"from_ayah": state.Round.FromAyah, "to_ayah": state.Round.ToAyah, "version": event.RoundVersion}
	if err := p.hub.SendToUser(topic, entry.StudentID, projectedEventID(event.EventID, realtime.EventQueueYourTurn, entry.StudentID), realtimeEnvelope(realtime.EventQueueYourTurn, projectedEventID(event.EventID, realtime.EventQueueYourTurn, entry.StudentID), yourTurn)); err != nil {
		return err
	}
	if candidate, ok := nextSoonEntry(state); ok {
		return p.sendNextSoon(topic, event, candidate)
	}
	return nil
}

func nextSoonEntry(state QueueState) (QueueEntry, bool) {
	selectedID := ""
	if state.Round.SelectedEntryID != nil {
		selectedID = *state.Round.SelectedEntryID
	}
	for _, candidate := range state.Entries {
		if candidate.Status == EntryStatusWaiting && candidate.ID != selectedID {
			return candidate, true
		}
	}
	return QueueEntry{}, false
}

func (p *RealtimeOutboxProjector) sendNextSoon(topic realtime.Topic, event OutboxEvent, entry QueueEntry) error {
	payload := map[string]any{"session_id": event.SessionID, "round_id": event.RoundID, "queue_entry_id": entry.ID, "position": entry.Position, "version": event.RoundVersion}
	eventID := projectedEventID(event.EventID, realtime.EventQueueNextSoon, entry.StudentID)
	return p.hub.SendToUser(topic, entry.StudentID, eventID, realtimeEnvelope(realtime.EventQueueNextSoon, eventID, payload))
}

func isUS1QueueEvent(eventType string) bool {
	switch eventType {
	case realtime.EventQueueRoundStarted, realtime.EventQueueReordered, realtime.EventQueueAdvanced, realtime.EventQueueEntryUpdated, realtime.EventQueuePolicyChanged:
		return true
	default:
		return false
	}
}

func queueEventMetadata(raw json.RawMessage) (map[string]any, error) {
	var metadata map[string]any
	if err := json.Unmarshal(raw, &metadata); err != nil {
		return nil, fmt.Errorf("decode queue event metadata: %w", err)
	}
	return metadata, nil
}

func metadataString(metadata map[string]any, key string) string {
	value, _ := metadata[key].(string)
	return value
}

func selectedEntryID(event OutboxEvent, round Round, metadata map[string]any) string {
	if event.ResourceID != nil {
		return *event.ResourceID
	}
	if id := metadataString(metadata, "selected_entry_id"); id != "" {
		return id
	}
	if round.SelectedEntryID != nil {
		return *round.SelectedEntryID
	}
	return ""
}

func orderedQueueIDs(state QueueState, kind string) []string {
	if kind == "preorder_students" {
		ids := make([]string, 0, len(state.Preorder))
		for _, candidate := range state.Preorder {
			ids = append(ids, candidate.StudentID)
		}
		return ids
	}
	ids := make([]string, 0, len(state.Entries))
	for _, entry := range state.Entries {
		ids = append(ids, entry.ID)
	}
	return ids
}

func queueEntry(state QueueState, entryID *string) (QueueEntry, bool) {
	if entryID == nil {
		return QueueEntry{}, false
	}
	for _, entry := range state.Entries {
		if entry.ID == *entryID {
			return entry, true
		}
	}
	return QueueEntry{}, false
}

func realtimeEnvelope(eventType, eventID string, payload map[string]any) map[string]any {
	return map[string]any{"type": eventType, "event_id": eventID, "occurred_at": time.Now().UTC().Format(time.RFC3339Nano), "payload": payload}
}

func projectedEventID(parentID, eventType, recipient string) string {
	return uuid.NewSHA1(uuid.NameSpaceURL, []byte(parentID+"|"+eventType+"|"+recipient)).String()
}
