package metrics

import (
	"math"
	"math/bits"
	"sync/atomic"
	"time"
)

// QueueConflictClass enumerates the closed set of queue command conflict
// outcomes (queue_command_conflicts_total breakdown).
type QueueConflictClass int

const (
	ConflictStaleVersion      QueueConflictClass = iota // optimistic-version mismatch
	ConflictInvalidTransition                           // status transition not allowed
	ConflictNoWaitingEntry                              // advance with no waiting entry
	ConflictEntryReciting                               // target entry currently reciting
	ConflictEntryTerminal                               // target entry in a terminal state
	ConflictRoundFinalized                              // round already finalized
	ConflictDuplicateCommand                            // idempotent duplicate receipt
)

// conflictClassCount bounds the per-class conflict breakdown array.
const conflictClassCount = int(ConflictDuplicateCommand) + 1

// InvariantKind enumerates the closed set of queue invariant guards
// (invariant_violations_total breakdown).
type InvariantKind int

const (
	InvariantOneActiveRound InvariantKind = iota // at most one active round per session
	InvariantOneReciter                          // at most one reciting entry per session
	InvariantOneProgress                         // exactly one progress row per entry
)

// invariantKindCount bounds the per-kind invariant breakdown array.
const invariantKindCount = int(InvariantOneProgress) + 1

// latencyBucketCount is the number of fixed power-of-2 latency buckets:
// bucket i covers [2^i, 2^(i+1)) ms; the last bucket holds everything
// >= 8192ms (the 8s cap).
const latencyBucketCount = 14

// latencyBucket maps a millisecond value to its fixed bucket index.
func latencyBucket(ms int64) int {
	i := bits.Len64(uint64(ms)) - 1
	if i < 0 {
		return 0
	}
	if i >= latencyBucketCount {
		return latencyBucketCount - 1
	}
	return i
}

// DurationSummary is a point-in-time snapshot of a duration histogram.
// P95Ms is a coarse lower-bound estimate from the fixed buckets; MaxMs is the
// exact maximum (SC-007 callers alert when it crosses the 10s deadline).
type DurationSummary struct {
	Count int64
	AvgMs int64
	P95Ms int64
	MaxMs int64
}

// durationHistogram accumulates count/total/max plus a fixed power-of-2
// bucket array using lock-free atomics.
type durationHistogram struct {
	count   atomic.Int64
	totalMs atomic.Int64
	maxMs   atomic.Int64
	buckets [latencyBucketCount]atomic.Int64
}

// record adds one duration sample.
func (h *durationHistogram) record(d time.Duration) {
	if h == nil {
		return
	}
	ms := d.Milliseconds()
	h.count.Add(1)
	h.totalMs.Add(ms)
	for {
		old := h.maxMs.Load()
		if ms <= old || h.maxMs.CompareAndSwap(old, ms) {
			break
		}
	}
	h.buckets[latencyBucket(ms)].Add(1)
}

// summary returns a snapshot of the histogram including a bucket-based p95
// lower-bound estimate.
func (h *durationHistogram) summary() DurationSummary {
	if h == nil {
		return DurationSummary{}
	}
	count := h.count.Load()
	avg := int64(0)
	if count > 0 {
		avg = h.totalMs.Load() / count
	}
	return DurationSummary{
		Count: count,
		AvgMs: avg,
		P95Ms: h.percentileMs(0.95),
		MaxMs: h.maxMs.Load(),
	}
}

// percentileMs returns the lower bound of the bucket holding the given
// percentile rank (0 < frac <= 1).
func (h *durationHistogram) percentileMs(frac float64) int64 {
	count := h.count.Load()
	if count == 0 {
		return 0
	}
	rank := int64(math.Ceil(frac * float64(count)))
	if rank < 1 {
		rank = 1
	}
	var cumulative int64
	for i := 0; i < latencyBucketCount; i++ {
		cumulative += h.buckets[i].Load()
		if cumulative >= rank {
			return int64(1) << i
		}
	}
	return 0
}

// QueueMetrics tracks F-003 recitation-queue health with bounded label
// cardinality: breakdowns are fixed-size arrays indexed by closed enums, and
// no method accepts arbitrary label strings (no UUIDs or PII as labels).
// ponytail: plain atomics; wire a Prometheus exporter when scraping is needed.
type QueueMetrics struct {
	commandDuration durationHistogram

	conflictsTotal         atomic.Int64
	conflictsByClass       [conflictClassCount]atomic.Int64
	outboxPending          atomic.Int64
	outboxParkedTotal      atomic.Int64
	eventDeliveryLag       durationHistogram
	sessionEndFinalization durationHistogram

	invariantViolationsTotal  atomic.Int64
	invariantViolationsByKind [invariantKindCount]atomic.Int64

	rateLimitedRequests    atomic.Int64
	rateLimitedConnections atomic.Int64
}

// QueueMetricsSummary is a point-in-time snapshot of all queue counters and
// duration histograms.
type QueueMetricsSummary struct {
	CommandDuration           DurationSummary
	ConflictsTotal            int64
	ConflictsByClass          [conflictClassCount]int64
	OutboxPending             int64
	OutboxParkedTotal         int64
	EventDeliveryLag          DurationSummary
	SessionEndFinalizationLag DurationSummary
	InvariantViolationsTotal  int64
	InvariantViolationsByKind [invariantKindCount]int64
	RateLimitedRequests       int64
	RateLimitedConnections    int64
}

// RecordCommandDuration adds one queue_command_duration sample.
func (m *QueueMetrics) RecordCommandDuration(d time.Duration) {
	if m == nil {
		return
	}
	m.commandDuration.record(d)
}

// RecordConflict increments queue_command_conflicts_total and the breakdown
// slot of the given closed conflict class. Out-of-range classes are ignored
// so a bad caller can never panic the request path.
func (m *QueueMetrics) RecordConflict(class QueueConflictClass) {
	if m == nil || class < 0 || int(class) >= conflictClassCount {
		return
	}
	m.conflictsTotal.Add(1)
	m.conflictsByClass[class].Add(1)
}

// SetOutboxPending sets the outbox_pending gauge.
func (m *QueueMetrics) SetOutboxPending(pending int64) {
	if m == nil {
		return
	}
	m.outboxPending.Store(pending)
}

// RecordOutboxParked increments outbox_parked_total (delivery retries
// exhausted; operator replay required).
func (m *QueueMetrics) RecordOutboxParked() {
	if m == nil {
		return
	}
	m.outboxParkedTotal.Add(1)
}

// RecordEventDeliveryLag adds one event_delivery_lag sample (SC-008
// commit-to-dispatch latency).
func (m *QueueMetrics) RecordEventDeliveryLag(d time.Duration) {
	if m == nil {
		return
	}
	m.eventDeliveryLag.record(d)
}

// RecordSessionEndFinalizationLag adds one session_end_finalization_lag
// sample (SC-007 convergence deadline is 10s).
func (m *QueueMetrics) RecordSessionEndFinalizationLag(d time.Duration) {
	if m == nil {
		return
	}
	m.sessionEndFinalization.record(d)
}

// RecordInvariantViolation increments invariant_violations_total and the
// breakdown slot of the given closed invariant kind. Out-of-range kinds are
// ignored so a bad caller can never panic the request path.
func (m *QueueMetrics) RecordInvariantViolation(kind InvariantKind) {
	if m == nil || kind < 0 || int(kind) >= invariantKindCount {
		return
	}
	m.invariantViolationsTotal.Add(1)
	m.invariantViolationsByKind[kind].Add(1)
}

// RecordRateLimitedRequest increments the queue rate-limited-request counter.
func (m *QueueMetrics) RecordRateLimitedRequest() {
	if m == nil {
		return
	}
	m.rateLimitedRequests.Add(1)
}

// RecordRateLimitedConnection increments the queue rate-limited-connection
// counter.
func (m *QueueMetrics) RecordRateLimitedConnection() {
	if m == nil {
		return
	}
	m.rateLimitedConnections.Add(1)
}

// Summary returns a snapshot of all queue metrics.
func (m *QueueMetrics) Summary() QueueMetricsSummary {
	if m == nil {
		return QueueMetricsSummary{}
	}
	summary := QueueMetricsSummary{
		CommandDuration:           m.commandDuration.summary(),
		ConflictsTotal:            m.conflictsTotal.Load(),
		OutboxPending:             m.outboxPending.Load(),
		OutboxParkedTotal:         m.outboxParkedTotal.Load(),
		EventDeliveryLag:          m.eventDeliveryLag.summary(),
		SessionEndFinalizationLag: m.sessionEndFinalization.summary(),
		InvariantViolationsTotal:  m.invariantViolationsTotal.Load(),
		RateLimitedRequests:       m.rateLimitedRequests.Load(),
		RateLimitedConnections:    m.rateLimitedConnections.Load(),
	}
	for i := 0; i < conflictClassCount; i++ {
		summary.ConflictsByClass[i] = m.conflictsByClass[i].Load()
	}
	for i := 0; i < invariantKindCount; i++ {
		summary.InvariantViolationsByKind[i] = m.invariantViolationsByKind[i].Load()
	}
	return summary
}
