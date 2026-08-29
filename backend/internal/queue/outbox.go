package queue

import (
	"context"
	"fmt"
	"math/rand"
	"time"

	"github.com/KarimMFadel/halaqaty/backend/internal/platform/metrics"
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
