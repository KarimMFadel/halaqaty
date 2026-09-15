//go:build integration

package performance

import (
	"context"
	"sort"
	"testing"
	"time"
)

const (
	chatPerformanceWarmups = 100
	chatPerformanceSamples = 1000
	chatPerformanceP95     = 2 * time.Second
)

func TestChatHistoryPagePerformance_SC005(t *testing.T) {
	f := newChatPerformanceFixture(t)
	ctx := context.Background()

	for i := 0; i < chatPerformanceWarmups; i++ {
		circleIndex := i % len(f.circles)
		if _, err := f.service.History(ctx, f.users[circleIndex][0], f.circles[circleIndex], nil, 100); err != nil {
			t.Fatalf("history warm-up %d: %v", i, err)
		}
	}

	latencies := make([]time.Duration, 0, chatPerformanceSamples)
	for i := 0; i < chatPerformanceSamples; i++ {
		circleIndex := i % len(f.circles)
		started := time.Now()
		messages, err := f.service.History(ctx, f.users[circleIndex][0], f.circles[circleIndex], nil, 100)
		latencies = append(latencies, time.Since(started))
		if err != nil {
			t.Fatalf("history sample %d: %v", i, err)
		}
		if len(messages) != 100 {
			t.Fatalf("history sample %d returned %d messages, want 100", i, len(messages))
		}
	}

	p95 := percentile95(latencies)
	t.Logf("SC-005 history page: circles=%d messages_per_circle=%d warmups=%d samples=%d p95=%s", chatPerformanceCircleCount, chatPerformanceMessagesPerCircle, chatPerformanceWarmups, chatPerformanceSamples, p95)
	if p95 > chatPerformanceP95 {
		t.Fatalf("SC-005 history p95=%s, want <=%s", p95, chatPerformanceP95)
	}
}

func TestChatSearchPagePerformance_SC005(t *testing.T) {
	f := newChatPerformanceFixture(t)
	ctx := context.Background()

	for i := 0; i < chatPerformanceWarmups; i++ {
		circleIndex := i % len(f.circles)
		if _, err := f.service.Search(ctx, f.users[circleIndex][0], f.circles[circleIndex], "needle", nil, 100); err != nil {
			t.Fatalf("search warm-up %d: %v", i, err)
		}
	}

	latencies := make([]time.Duration, 0, chatPerformanceSamples)
	for i := 0; i < chatPerformanceSamples; i++ {
		circleIndex := i % len(f.circles)
		started := time.Now()
		messages, err := f.service.Search(ctx, f.users[circleIndex][0], f.circles[circleIndex], "needle", nil, 100)
		latencies = append(latencies, time.Since(started))
		if err != nil {
			t.Fatalf("search sample %d: %v", i, err)
		}
		if len(messages) == 0 {
			t.Fatalf("search sample %d returned no matching messages", i)
		}
	}

	p95 := percentile95(latencies)
	t.Logf("SC-005 first-page search: circles=%d messages_per_circle=%d warmups=%d samples=%d p95=%s", chatPerformanceCircleCount, chatPerformanceMessagesPerCircle, chatPerformanceWarmups, chatPerformanceSamples, p95)
	if p95 > chatPerformanceP95 {
		t.Fatalf("SC-005 search p95=%s, want <=%s", p95, chatPerformanceP95)
	}
}

func percentile95(values []time.Duration) time.Duration {
	sorted := append([]time.Duration(nil), values...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i] < sorted[j] })
	index := (len(sorted)*95)/100 - 1
	if index < 0 {
		index = 0
	}
	return sorted[index]
}
