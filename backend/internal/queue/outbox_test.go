package queue

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/KarimMFadel/halaqaty/backend/internal/platform/metrics"
)

type fakeOutboxStore struct {
	claimed    []OutboxEvent
	delivered  []string
	retried    []retryCall
	parked     []string
	claimError error
}

type retryCall struct {
	eventID     string
	availableAt time.Time
}

func (s *fakeOutboxStore) ClaimReplayOutboxEvents(_ context.Context, _ int) ([]OutboxEvent, error) {
	return s.claimed, s.claimError
}
func (s *fakeOutboxStore) MarkOutboxDelivered(_ context.Context, eventID string) error {
	s.delivered = append(s.delivered, eventID)
	return nil
}
func (s *fakeOutboxStore) RetryOutboxEvent(_ context.Context, eventID string, availableAt time.Time) error {
	s.retried = append(s.retried, retryCall{eventID: eventID, availableAt: availableAt})
	return nil
}
func (s *fakeOutboxStore) ParkOutboxEvent(_ context.Context, eventID string) error {
	s.parked = append(s.parked, eventID)
	return nil
}

type fakeOutboxProjector struct{ err error }

func (p fakeOutboxProjector) ProjectAndDeliver(_ context.Context, _ OutboxEvent) error { return p.err }

type fakeParkedAlerter struct{ eventIDs []string }

func (a *fakeParkedAlerter) AlertOutboxParked(_ context.Context, event OutboxEvent) {
	a.eventIDs = append(a.eventIDs, event.EventID)
}

func TestOutboxDispatcherDeliversAndReplays(t *testing.T) {
	store := &fakeOutboxStore{claimed: []OutboxEvent{{EventID: "event-1"}}}
	dispatcher := NewOutboxDispatcher(store, fakeOutboxProjector{}, nil, nil, func() time.Time { return time.Unix(0, 0).UTC() }, func(d time.Duration) time.Duration { return d })

	if err := dispatcher.Replay(context.Background(), 10); err != nil {
		t.Fatalf("replay: %v", err)
	}
	if len(store.delivered) != 1 || store.delivered[0] != "event-1" {
		t.Fatalf("delivered = %v, want [event-1]", store.delivered)
	}
}

func TestOutboxDispatcherRetriesFiveTimesThenParksWithAlert(t *testing.T) {
	now := time.Date(2026, time.August, 23, 12, 0, 0, 0, time.UTC)
	store := &fakeOutboxStore{}
	queueMetrics := &metrics.QueueMetrics{}
	alerter := &fakeParkedAlerter{}
	dispatcher := NewOutboxDispatcher(store, fakeOutboxProjector{err: errors.New("hub unavailable")}, queueMetrics, alerter, func() time.Time { return now }, func(d time.Duration) time.Duration { return d })

	for attempt := 0; attempt <= 5; attempt++ {
		err := dispatcher.Dispatch(context.Background(), OutboxEvent{EventID: "event-2", AttemptCount: attempt})
		if err != nil {
			t.Fatalf("dispatch attempt %d: %v", attempt, err)
		}
	}

	wantBackoffs := []time.Duration{time.Second, 2 * time.Second, 4 * time.Second, 8 * time.Second, 16 * time.Second}
	if len(store.retried) != len(wantBackoffs) {
		t.Fatalf("retries = %d, want %d", len(store.retried), len(wantBackoffs))
	}
	for i, want := range wantBackoffs {
		if got := store.retried[i].availableAt; !got.Equal(now.Add(want)) {
			t.Fatalf("retry %d available_at = %v, want %v", i, got, now.Add(want))
		}
	}
	if len(store.parked) != 1 || store.parked[0] != "event-2" {
		t.Fatalf("parked = %v, want [event-2]", store.parked)
	}
	if len(alerter.eventIDs) != 1 || alerter.eventIDs[0] != "event-2" {
		t.Fatalf("alerts = %v, want [event-2]", alerter.eventIDs)
	}
	if got := queueMetrics.Summary().OutboxParkedTotal; got != 1 {
		t.Fatalf("outbox parked metric = %d, want 1", got)
	}
}

func TestDefaultOutboxJitterStaysWithinTenPercent(t *testing.T) {
	delay := 10 * time.Second
	for range 100 {
		got := defaultOutboxJitter(delay)
		if got < 9*time.Second || got > 11*time.Second {
			t.Fatalf("jittered delay = %v, want within [9s, 11s]", got)
		}
	}
}
