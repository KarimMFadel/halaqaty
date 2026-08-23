package metrics

import (
	"reflect"
	"testing"
	"time"
)

func TestQueueMetricsNilReceiverIsSafe(t *testing.T) {
	var m *QueueMetrics
	m.RecordCommandDuration(time.Second)
	m.RecordConflict(ConflictStaleVersion)
	m.SetOutboxPending(5)
	m.RecordOutboxParked()
	m.RecordEventDeliveryLag(time.Second)
	m.RecordAudioConvergenceLag(time.Second)
	m.RecordSessionEndFinalizationLag(time.Second)
	m.RecordInvariantViolation(InvariantOneActiveRound)
	m.RecordRateLimitedRequest()
	m.RecordRateLimitedConnection()
	if got := m.Summary(); got != (QueueMetricsSummary{}) {
		t.Fatalf("nil receiver summary = %+v, want zero value", got)
	}
}

func TestQueueMetricsCounters(t *testing.T) {
	m := new(QueueMetrics)

	m.RecordConflict(ConflictStaleVersion)
	m.RecordConflict(ConflictStaleVersion)
	m.RecordConflict(ConflictDuplicateCommand)
	m.RecordConflict(ConflictRoundFinalized)
	summary := m.Summary()
	if summary.ConflictsTotal != 4 {
		t.Fatalf("ConflictsTotal = %d, want 4", summary.ConflictsTotal)
	}
	if got := summary.ConflictsByClass[ConflictStaleVersion]; got != 2 {
		t.Fatalf("ConflictsByClass[stale_version] = %d, want 2", got)
	}
	if got := summary.ConflictsByClass[ConflictDuplicateCommand]; got != 1 {
		t.Fatalf("ConflictsByClass[duplicate_command] = %d, want 1", got)
	}
	if got := summary.ConflictsByClass[ConflictInvalidTransition]; got != 0 {
		t.Fatalf("untouched conflict class = %d, want 0", got)
	}

	m.RecordInvariantViolation(InvariantOneReciter)
	m.RecordInvariantViolation(InvariantOneProgress)
	summary = m.Summary()
	if summary.InvariantViolationsTotal != 2 {
		t.Fatalf("InvariantViolationsTotal = %d, want 2", summary.InvariantViolationsTotal)
	}
	if got := summary.InvariantViolationsByKind[InvariantOneReciter]; got != 1 {
		t.Fatalf("InvariantViolationsByKind[one_reciter] = %d, want 1", got)
	}

	m.SetOutboxPending(42)
	m.RecordOutboxParked()
	m.RecordRateLimitedRequest()
	m.RecordRateLimitedConnection()
	m.RecordRateLimitedConnection()
	summary = m.Summary()
	if summary.OutboxPending != 42 {
		t.Fatalf("OutboxPending = %d, want 42", summary.OutboxPending)
	}
	if summary.OutboxParkedTotal != 1 {
		t.Fatalf("OutboxParkedTotal = %d, want 1", summary.OutboxParkedTotal)
	}
	if summary.RateLimitedRequests != 1 || summary.RateLimitedConnections != 2 {
		t.Fatalf("rate-limit counters = %d/%d, want 1/2", summary.RateLimitedRequests, summary.RateLimitedConnections)
	}
}

func TestQueueMetricsOutOfClassRangeIgnored(t *testing.T) {
	m := new(QueueMetrics)
	m.RecordConflict(QueueConflictClass(-1))
	m.RecordConflict(QueueConflictClass(99))
	m.RecordInvariantViolation(InvariantKind(-1))
	m.RecordInvariantViolation(InvariantKind(99))
	summary := m.Summary()
	if summary.ConflictsTotal != 0 || summary.InvariantViolationsTotal != 0 {
		t.Fatalf("out-of-range enum leaked into totals: %+v", summary)
	}
}

func TestQueueMetricsCommandDurationHistogram(t *testing.T) {
	m := new(QueueMetrics)
	for i := 0; i < 95; i++ {
		m.RecordCommandDuration(100 * time.Millisecond)
	}
	for i := 0; i < 5; i++ {
		m.RecordCommandDuration(3000 * time.Millisecond)
	}
	summary := m.Summary()
	got := summary.CommandDuration
	if got.Count != 100 {
		t.Fatalf("Count = %d, want 100", got.Count)
	}
	if got.AvgMs != 245 {
		t.Fatalf("AvgMs = %d, want 245", got.AvgMs)
	}
	if got.MaxMs != 3000 {
		t.Fatalf("MaxMs = %d, want 3000", got.MaxMs)
	}
	// p95 rank 95 of 100 falls in the [64,128)ms bucket; estimate is the
	// bucket lower bound.
	if got.P95Ms != 64 {
		t.Fatalf("P95Ms = %d, want 64", got.P95Ms)
	}

	// Adding more slow samples pushes p95 into the [2048,4096)ms bucket.
	for i := 0; i < 5; i++ {
		m.RecordCommandDuration(3000 * time.Millisecond)
	}
	got = m.Summary().CommandDuration
	if got.P95Ms != 2048 {
		t.Fatalf("P95Ms after slow tail = %d, want 2048", got.P95Ms)
	}
	if got.MaxMs != 3000 {
		t.Fatalf("MaxMs = %d, want 3000", got.MaxMs)
	}
}

func TestQueueMetricsLagHistogramsAlertPastTenSeconds(t *testing.T) {
	m := new(QueueMetrics)
	m.RecordEventDeliveryLag(200 * time.Millisecond)
	m.RecordAudioConvergenceLag(11 * time.Second)
	m.RecordSessionEndFinalizationLag(15 * time.Second)
	summary := m.Summary()
	if summary.EventDeliveryLag.Count != 1 || summary.EventDeliveryLag.MaxMs != 200 {
		t.Fatalf("EventDeliveryLag = %+v", summary.EventDeliveryLag)
	}
	// SC-007: callers alert when MaxMs crosses the 10s convergence deadline.
	if summary.AudioConvergenceLag.MaxMs <= 10000 {
		t.Fatalf("AudioConvergenceLag.MaxMs = %d, want >10000", summary.AudioConvergenceLag.MaxMs)
	}
	if summary.SessionEndFinalizationLag.MaxMs != 15000 {
		t.Fatalf("SessionEndFinalizationLag.MaxMs = %d, want 15000", summary.SessionEndFinalizationLag.MaxMs)
	}
}

func TestLatencyBucketBoundaries(t *testing.T) {
	cases := []struct {
		ms   int64
		want int
	}{
		{0, 0}, {1, 0}, {2, 1}, {3, 1}, {1023, 9}, {1024, 10},
		{8191, 12}, {8192, 13}, {90000, 13},
	}
	for _, tc := range cases {
		if got := latencyBucket(tc.ms); got != tc.want {
			t.Fatalf("latencyBucket(%d) = %d, want %d", tc.ms, got, tc.want)
		}
	}
}

// TestQueueMetricsBoundedCardinality proves no unbounded label dimension can
// exist: no map fields, and no exported method accepts an arbitrary string.
func TestQueueMetricsBoundedCardinality(t *testing.T) {
	valueType := reflect.TypeOf(QueueMetrics{})
	for i := 0; i < valueType.NumField(); i++ {
		if valueType.Field(i).Type.Kind() == reflect.Map {
			t.Fatalf("QueueMetrics field %q is a map", valueType.Field(i).Name)
		}
	}
	summaryType := reflect.TypeOf(QueueMetricsSummary{})
	for i := 0; i < summaryType.NumField(); i++ {
		if summaryType.Field(i).Type.Kind() == reflect.Map {
			t.Fatalf("QueueMetricsSummary field %q is a map", summaryType.Field(i).Name)
		}
	}
	pointerType := reflect.TypeOf(&QueueMetrics{})
	for i := 0; i < pointerType.NumMethod(); i++ {
		method := pointerType.Method(i)
		for j := 0; j < method.Type.NumIn(); j++ {
			if method.Type.In(j).Kind() == reflect.String {
				t.Fatalf("method %q accepts a string label", method.Name)
			}
		}
	}
}
