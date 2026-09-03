package performance

import (
	"context"
	"testing"
	"time"

	"github.com/KarimMFadel/halaqaty/backend/internal/platform/metrics"
	"github.com/KarimMFadel/halaqaty/backend/internal/queue"
)

type queuePerformanceStore struct{ delivered int }

func (s *queuePerformanceStore) ClaimDueOutboxEvents(context.Context, int) ([]queue.OutboxEvent, error) {
	return nil, nil
}
func (s *queuePerformanceStore) ClaimReplayOutboxEvents(context.Context, int) ([]queue.OutboxEvent, error) {
	return nil, nil
}
func (s *queuePerformanceStore) MarkOutboxDelivered(context.Context, string) error {
	s.delivered++
	return nil
}
func (s *queuePerformanceStore) RetryOutboxEvent(context.Context, string, time.Time) error {
	return nil
}
func (s *queuePerformanceStore) ParkOutboxEvent(context.Context, string) error { return nil }

type queuePerformanceProjector struct{}

func (queuePerformanceProjector) ProjectAndDeliver(context.Context, queue.OutboxEvent) error {
	return nil
}

func TestRecitationQueueDeliveryPerformance_SC008(t *testing.T) {
	const actions = 100
	store := new(queuePerformanceStore)
	queueMetrics := new(metrics.QueueMetrics)
	dispatcher := queue.NewOutboxDispatcher(store, queuePerformanceProjector{}, queueMetrics, nil, time.Now, func(delay time.Duration) time.Duration { return delay })
	latencies := make([]time.Duration, 0, actions)
	for i := 0; i < actions; i++ {
		started := time.Now()
		if err := dispatcher.Dispatch(context.Background(), queue.OutboxEvent{EventID: "perf-event"}); err != nil {
			t.Fatalf("dispatch action %d: %v", i, err)
		}
		latency := time.Since(started)
		latencies = append(latencies, latency)
		queueMetrics.RecordEventDeliveryLag(latency)
	}
	if store.delivered != actions {
		t.Fatalf("delivered=%d, want %d", store.delivered, actions)
	}
	summary := queueMetrics.Summary().EventDeliveryLag
	t.Logf("SC-008 queue delivery actions=%d p95=%dms max=%dms", actions, summary.P95Ms, summary.MaxMs)
	if summary.P95Ms > 500 {
		t.Fatalf("SC-008 p95=%dms, want <=500ms", summary.P95Ms)
	}
}
