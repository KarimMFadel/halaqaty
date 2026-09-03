//go:build integration

package integration

import (
	"context"
	"encoding/json"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/KarimMFadel/halaqaty/backend/internal/platform/metrics"
	"github.com/KarimMFadel/halaqaty/backend/internal/queue"
	"github.com/KarimMFadel/halaqaty/backend/internal/realtime"
	"github.com/KarimMFadel/halaqaty/backend/internal/sessions"
)

type phase6FailingProjector struct{}

func (phase6FailingProjector) ProjectAndDeliver(context.Context, queue.OutboxEvent) error {
	return errors.New("connected client unavailable")
}

type phase6DeliveringProjector struct{ delivered []string }

func (p *phase6DeliveringProjector) ProjectAndDeliver(_ context.Context, event queue.OutboxEvent) error {
	p.delivered = append(p.delivered, event.EventID)
	return nil
}

type phase6BlockingProjector struct {
	calls   atomic.Int32
	started chan struct{}
	release chan struct{}
}

func (p *phase6BlockingProjector) ProjectAndDeliver(context.Context, queue.OutboxEvent) error {
	if p.calls.Add(1) == 1 {
		close(p.started)
		<-p.release
	}
	return nil
}

type phase6ParkedAlerter struct{ count *int }

func (a phase6ParkedAlerter) AlertOutboxParked(context.Context, queue.OutboxEvent) { (*a.count)++ }

func TestRecitationQueueReliability_A1RetriesParksAlertsAndOperatorReplaysDurably(t *testing.T) {
	f := setupQueueRBACEnv(t)
	ctx := context.Background()
	now := time.Date(2026, time.September, 3, 12, 0, 0, 0, time.UTC)
	alerts := 0
	queueMetrics := &metrics.QueueMetrics{}
	event := phase6InsertOutboxEvent(t, f, now)
	dispatcher := queue.NewOutboxDispatcher(f.repo, phase6FailingProjector{}, queueMetrics, phase6ParkedAlerter{count: &alerts}, func() time.Time { return now }, func(delay time.Duration) time.Duration { return delay })
	want := []time.Duration{time.Second, 2 * time.Second, 4 * time.Second, 8 * time.Second, 16 * time.Second}
	for attempt, delay := range want {
		event.AttemptCount = attempt
		if err := dispatcher.Dispatch(ctx, event); err != nil {
			t.Fatalf("dispatch attempt %d: %v", attempt, err)
		}
		var availableAt time.Time
		if err := f.pool.QueryRow(ctx, `SELECT available_at FROM queue_event_outbox WHERE event_id = $1::uuid`, event.EventID).Scan(&availableAt); err != nil {
			t.Fatalf("load retry %d: %v", attempt, err)
		}
		if got := availableAt.Sub(now); got != delay {
			t.Fatalf("retry %d delay=%v, want %v", attempt, got, delay)
		}
	}
	event.AttemptCount = len(want)
	if err := dispatcher.Dispatch(ctx, event); err != nil {
		t.Fatalf("dispatch exhausted retry: %v", err)
	}
	var attempts int
	var parkedAt, deliveredAt *time.Time
	if err := f.pool.QueryRow(ctx, `SELECT attempt_count, parked_at, delivered_at FROM queue_event_outbox WHERE event_id = $1::uuid`, event.EventID).Scan(&attempts, &parkedAt, &deliveredAt); err != nil {
		t.Fatalf("load parked outbox event: %v", err)
	}
	if attempts != 5 || parkedAt == nil || deliveredAt != nil || alerts != 1 || queueMetrics.Summary().OutboxParkedTotal != 1 {
		t.Fatalf("attempts=%d parked=%v delivered=%v alerts=%d parked_metric=%d; want 5/true/false/1/1", attempts, parkedAt != nil, deliveredAt != nil, alerts, queueMetrics.Summary().OutboxParkedTotal)
	}

	projector := new(phase6DeliveringProjector)
	if err := queue.NewOutboxDispatcher(f.repo, projector, queueMetrics, nil, func() time.Time { return now }, nil).Replay(ctx, 100); err != nil {
		t.Fatalf("operator replay: %v", err)
	}
	if err := f.pool.QueryRow(ctx, `SELECT delivered_at FROM queue_event_outbox WHERE event_id = $1::uuid`, event.EventID).Scan(&deliveredAt); err != nil || deliveredAt == nil {
		t.Fatalf("parked event was not durably delivered by replay: delivered=%v err=%v", deliveredAt != nil, err)
	}
	if len(projector.delivered) == 0 {
		t.Fatal("operator replay did not invoke the durable projector")
	}
}

func TestRecitationQueueReliability_A1StartupReplaysPendingAndParkedRows(t *testing.T) {
	f := setupQueueRBACEnv(t)
	ctx := context.Background()
	pending := phase6InsertOutboxEvent(t, f, time.Now().UTC())
	parked := phase6InsertOutboxEvent(t, f, time.Now().UTC())
	if err := f.repo.ParkOutboxEvent(ctx, parked.EventID); err != nil {
		t.Fatalf("park durable startup row: %v", err)
	}

	projector := new(phase6DeliveringProjector)
	if err := queue.NewOutboxDispatcher(f.repo, projector, &metrics.QueueMetrics{}, nil, nil, nil).Replay(ctx, 100); err != nil {
		t.Fatalf("startup replay: %v", err)
	}
	for _, eventID := range []string{pending.EventID, parked.EventID} {
		var deliveredAt *time.Time
		if err := f.pool.QueryRow(ctx, `SELECT delivered_at FROM queue_event_outbox WHERE event_id = $1::uuid`, eventID).Scan(&deliveredAt); err != nil || deliveredAt == nil {
			t.Fatalf("startup replay event %s: delivered=%v err=%v", eventID, deliveredAt != nil, err)
		}
	}
}

func TestRecitationQueueReliability_ConcurrentWorkersProjectOneClaimOnce(t *testing.T) {
	f := setupQueueRBACEnv(t)
	ctx := context.Background()
	if _, err := f.pool.Exec(ctx, `DELETE FROM queue_event_outbox`); err != nil {
		t.Fatalf("clear fixture outbox: %v", err)
	}
	phase6InsertOutboxEvent(t, f, time.Now().UTC())
	projector := &phase6BlockingProjector{started: make(chan struct{}), release: make(chan struct{})}
	dispatcher := queue.NewOutboxDispatcher(f.repo, projector, nil, nil, nil, nil)

	firstDone := make(chan error, 1)
	go func() { firstDone <- dispatcher.DispatchDue(ctx, 1) }()
	select {
	case <-projector.started:
	case <-time.After(2 * time.Second):
		t.Fatal("first worker did not begin projecting its claimed event")
	}

	secondErr := dispatcher.DispatchDue(ctx, 1)
	close(projector.release)
	if firstErr := <-firstDone; firstErr != nil {
		t.Fatalf("first worker dispatch: %v", firstErr)
	}
	if secondErr != nil {
		t.Fatalf("second worker dispatch: %v", secondErr)
	}
	if got := projector.calls.Load(); got != 1 {
		t.Fatalf("projector calls = %d, want 1 durable claim across concurrent workers", got)
	}
}

func TestRecitationQueueReliability_A1SessionEndConvergesIdempotentlyWithoutMediaMutation(t *testing.T) {
	f := newSessionQueueConvergenceFixture(t)
	ctx := context.Background()
	var entryID string
	var entryVersion int64
	var mediaRoomRef string
	if err := f.pool.QueryRow(ctx, `
		SELECT e.id::text, e.version, s.media_room_ref
		FROM recitation_queue_entries e
		JOIN recitation_queue q ON q.id = e.queue_id
		JOIN sessions s ON s.id = q.session_id
		WHERE s.id = $1::uuid
		ORDER BY e.position LIMIT 1
	`, f.session).Scan(&entryID, &entryVersion, &mediaRoomRef); err != nil {
		t.Fatalf("load queue and media state: %v", err)
	}
	if _, err := queue.NewTurnService(f.repo).Skip(ctx, entryID, entryVersion, f.teacher); err != nil {
		t.Fatalf("queue mutation: %v", err)
	}
	var afterMediaRoomRef string
	if err := f.pool.QueryRow(ctx, `SELECT media_room_ref FROM sessions WHERE id = $1::uuid`, f.session).Scan(&afterMediaRoomRef); err != nil {
		t.Fatalf("reload media state: %v", err)
	}
	if afterMediaRoomRef != mediaRoomRef {
		t.Fatalf("queue mutation changed media room from %q to %q", mediaRoomRef, afterMediaRoomRef)
	}

	started := time.Now()
	if _, err := f.sessions.EndSession(ctx, f.teacher, f.session, sessions.EndReasonManual); err != nil {
		t.Fatalf("end session: %v", err)
	}
	convergence := queue.NewConvergence(f.repo, nil, nil)
	if err := convergence.FinalizeSession(ctx, f.session); err != nil {
		t.Fatalf("session-end convergence: %v", err)
	}
	if elapsed := time.Since(started); elapsed > 10*time.Second {
		t.Fatalf("session-end convergence took %v, want <= 10s", elapsed)
	}
	if err := convergence.FinalizeSession(ctx, f.session); err != nil {
		t.Fatalf("idempotent session-end convergence: %v", err)
	}
	var remaining int
	if err := f.pool.QueryRow(ctx, `SELECT COUNT(*) FROM recitation_queue WHERE session_id = $1::uuid AND lifecycle IN ('active', 'prepared')`, f.session).Scan(&remaining); err != nil {
		t.Fatalf("count unfinalized rounds: %v", err)
	}
	if remaining != 0 {
		t.Fatalf("unfinalized rounds after convergence = %d, want 0", remaining)
	}
}

func phase6InsertOutboxEvent(t *testing.T, f *queueRBACEnv, availableAt time.Time) queue.OutboxEvent {
	t.Helper()
	ctx := context.Background()
	var roundID string
	var roundVersion int64
	if err := f.pool.QueryRow(ctx, `SELECT id::text, version FROM recitation_queue WHERE session_id = $1::uuid AND lifecycle = 'active'`, f.sessionID).Scan(&roundID, &roundVersion); err != nil {
		t.Fatalf("load active round: %v", err)
	}
	event := queue.OutboxEvent{EventID: uuid.NewString(), SessionID: f.sessionID, RoundID: roundID, EventType: realtime.EventQueueAdvanced, RoundVersion: roundVersion, EventMetadata: json.RawMessage(`{}`), AvailableAt: availableAt}
	if err := f.repo.WithTx(ctx, func(tx *queue.Tx) error { return tx.InsertOutboxEvent(ctx, event) }); err != nil {
		t.Fatalf("insert durable outbox event: %v", err)
	}
	return event
}
