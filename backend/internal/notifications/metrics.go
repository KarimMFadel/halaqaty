package notifications

import (
	"sync/atomic"
)

// DispatchOutcome identifies a bounded push-dispatch result. Dimensions are
// fixed-size and identifier-free, matching the platform metrics convention.
type DispatchOutcome int

const (
	// DispatchAccepted counts provider-accepted sends (never handset receipt).
	DispatchAccepted DispatchOutcome = iota
	// DispatchRetried counts transient failures rescheduled with backoff.
	DispatchRetried
	// DispatchParked counts device attempts parked after five failures.
	DispatchParked
	// DispatchInvalidToken counts unregistered tokens deleted via the F-001 seam.
	DispatchInvalidToken
	// DispatchSuppressedPreference counts terminal preference suppressions.
	DispatchSuppressedPreference
	// DispatchSuppressedQuietHours counts terminal quiet-hours suppressions.
	DispatchSuppressedQuietHours
	// DispatchExpired counts intents swept past their push expiry.
	DispatchExpired
)

const dispatchOutcomeCount = int(DispatchExpired) + 1

// NotificationMetrics tracks dispatcher and retention observability with
// bounded counters. It intentionally has no arbitrary label or identifier
// input; a nil receiver is safe so wiring can defer metrics setup.
type NotificationMetrics struct {
	outcomes        [dispatchOutcomeCount]atomic.Int64
	cleanupAttempts atomic.Int64
	cleanupHistory  atomic.Int64
}

// NotificationMetricsSummary is a point-in-time snapshot of dispatch and
// retention metrics.
type NotificationMetricsSummary struct {
	Outcomes               [dispatchOutcomeCount]int64
	CleanupAttemptsDeleted int64
	CleanupHistoryDeleted  int64
}

// RecordDispatch records one bounded dispatch outcome.
func (m *NotificationMetrics) RecordDispatch(outcome DispatchOutcome) {
	m.RecordDispatchBatch(outcome, 1)
}

// RecordDispatchBatch records count occurrences of one bounded outcome.
func (m *NotificationMetrics) RecordDispatchBatch(outcome DispatchOutcome, count int64) {
	if m == nil || outcome < 0 || int(outcome) >= dispatchOutcomeCount || count <= 0 {
		return
	}
	m.outcomes[outcome].Add(count)
}

// RecordCleanupDeleted records one retention cleanup cycle's deleted counts.
func (m *NotificationMetrics) RecordCleanupDeleted(attempts, history int64) {
	if m == nil {
		return
	}
	if attempts > 0 {
		m.cleanupAttempts.Add(attempts)
	}
	if history > 0 {
		m.cleanupHistory.Add(history)
	}
}

// Summary returns a snapshot of all dispatch and retention counters.
func (m *NotificationMetrics) Summary() NotificationMetricsSummary {
	if m == nil {
		return NotificationMetricsSummary{}
	}
	summary := NotificationMetricsSummary{
		CleanupAttemptsDeleted: m.cleanupAttempts.Load(),
		CleanupHistoryDeleted:  m.cleanupHistory.Load(),
	}
	for i := 0; i < dispatchOutcomeCount; i++ {
		summary.Outcomes[i] = m.outcomes[i].Load()
	}
	return summary
}
