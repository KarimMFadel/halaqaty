//go:build integration

package integration

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/KarimMFadel/halaqaty/backend/internal/platform/metrics"
	"github.com/KarimMFadel/halaqaty/backend/internal/queue"
)

type phase6OutboxStore struct {
	retries []time.Time
	parked  int
	alerts  int
}

func (s *phase6OutboxStore) ClaimDueOutboxEvents(context.Context, int) ([]queue.OutboxEvent, error) {
	return nil, nil
}
func (s *phase6OutboxStore) ClaimReplayOutboxEvents(context.Context, int) ([]queue.OutboxEvent, error) {
	return nil, nil
}
func (s *phase6OutboxStore) MarkOutboxDelivered(context.Context, string) error { return nil }
func (s *phase6OutboxStore) RetryOutboxEvent(_ context.Context, _ string, at time.Time) error {
	s.retries = append(s.retries, at)
	return nil
}
func (s *phase6OutboxStore) ParkOutboxEvent(context.Context, string) error { s.parked++; return nil }

type phase6OutboxProjector struct{}

func (phase6OutboxProjector) ProjectAndDeliver(context.Context, queue.OutboxEvent) error {
	return errors.New("connected client unavailable")
}

type phase6ParkedAlerter struct{ count *int }

func (a phase6ParkedAlerter) AlertOutboxParked(context.Context, queue.OutboxEvent) { (*a.count)++ }

func TestRecitationQueueReliability_A1RetriesAndParks(t *testing.T) {
	now := time.Date(2026, time.September, 3, 12, 0, 0, 0, time.UTC)
	store := new(phase6OutboxStore)
	alerts := 0
	dispatcher := queue.NewOutboxDispatcher(store, phase6OutboxProjector{}, &metrics.QueueMetrics{}, phase6ParkedAlerter{count: &alerts}, func() time.Time { return now }, func(delay time.Duration) time.Duration { return delay })

	for attempt := 0; attempt <= 5; attempt++ {
		if err := dispatcher.Dispatch(context.Background(), queue.OutboxEvent{EventID: "phase6-event", AttemptCount: attempt}); err != nil {
			t.Fatalf("dispatch attempt %d: %v", attempt, err)
		}
	}

	want := []time.Duration{time.Second, 2 * time.Second, 4 * time.Second, 8 * time.Second, 16 * time.Second}
	if len(store.retries) != len(want) || store.parked != 1 || alerts != 1 {
		t.Fatalf("retries=%d parked=%d alerts=%d, want 5/1/1", len(store.retries), store.parked, alerts)
	}
	for i, delay := range want {
		if got := store.retries[i].Sub(now); got != delay {
			t.Fatalf("retry %d delay=%v, want %v", i, got, delay)
		}
	}
}
