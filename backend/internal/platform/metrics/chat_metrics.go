package metrics

import (
	"sync/atomic"
	"time"
)

// ChatOperation identifies a bounded chat latency dimension.
type ChatOperation int

const (
	ChatOperationSend ChatOperation = iota
	ChatOperationHistory
	ChatOperationRead
	ChatOperationUpload
	ChatOperationOutbox
	ChatOperationReconnect
	ChatOperationSearch
)

const chatOperationCount = int(ChatOperationSearch) + 1

// ChatOutcome identifies a bounded chat request result.
type ChatOutcome int

const (
	ChatOutcomeAccepted ChatOutcome = iota
	ChatOutcomeRejected
	ChatOutcomeDenied
	ChatOutcomeConflict
	ChatOutcomeFailure
	ChatOutcomeRetry
	ChatOutcomeParked
	ChatOutcomeRecovered
	ChatOutcomeNoResults
)

const chatOutcomeCount = int(ChatOutcomeNoResults) + 1

// ChatDenial identifies a bounded authorization-denial reason.
type ChatDenial int

const (
	ChatDenialUnauthorized ChatDenial = iota
	ChatDenialForbidden
	ChatDenialArchived
	ChatDenialRemoved
	ChatDenialIneligible
)

const chatDenialCount = int(ChatDenialIneligible) + 1

// ChatUploadOutcome identifies a bounded upload result.
type ChatUploadOutcome int

const (
	ChatUploadAccepted ChatUploadOutcome = iota
	ChatUploadRejected
	ChatUploadStorageFailure
)

const chatUploadCount = int(ChatUploadStorageFailure) + 1

// ChatOutboxOutcome identifies a bounded outbox delivery result.
type ChatOutboxOutcome int

const (
	ChatOutboxDelivered ChatOutboxOutcome = iota
	ChatOutboxRetried
	ChatOutboxParked
)

const chatOutboxCount = int(ChatOutboxParked) + 1

// ChatReconnectOutcome identifies a bounded reconnect result.
type ChatReconnectOutcome int

const (
	ChatReconnectRecovered ChatReconnectOutcome = iota
	ChatReconnectGap
	ChatReconnectFailure
)

const chatReconnectCount = int(ChatReconnectFailure) + 1

// ChatSearchOutcome identifies a bounded search result.
type ChatSearchOutcome int

const (
	ChatSearchResults ChatSearchOutcome = iota
	ChatSearchNoResults
	ChatSearchFailure
)

const chatSearchCount = int(ChatSearchFailure) + 1

// ChatMetrics tracks chat observability with fixed-size, identifier-free
// dimensions. It intentionally has no arbitrary label or identifier input.
type ChatMetrics struct {
	latency          durationHistogram
	latencyByOp      [chatOperationCount]durationHistogram
	outcomes         [chatOutcomeCount]atomic.Int64
	denials          [chatDenialCount]atomic.Int64
	uploads          [chatUploadCount]atomic.Int64
	outbox           [chatOutboxCount]atomic.Int64
	reconnects       [chatReconnectCount]atomic.Int64
	searches         [chatSearchCount]atomic.Int64
	reconnectLatency durationHistogram
	searchLatency    durationHistogram
}

// ChatMetricsSummary is a point-in-time snapshot of chat metrics.
type ChatMetricsSummary struct {
	Latency            DurationSummary
	LatencyByOperation [chatOperationCount]DurationSummary
	Outcomes           [chatOutcomeCount]int64
	Denials            [chatDenialCount]int64
	Uploads            [chatUploadCount]int64
	Outbox             [chatOutboxCount]int64
	Reconnects         [chatReconnectCount]int64
	Searches           [chatSearchCount]int64
	ReconnectLatency   DurationSummary
	SearchLatency      DurationSummary
}

// RecordLatencyOutcome records one bounded chat operation latency and outcome.
func (m *ChatMetrics) RecordLatencyOutcome(operation ChatOperation, outcome ChatOutcome, latency time.Duration) {
	if m == nil || operation < 0 || int(operation) >= chatOperationCount || outcome < 0 || int(outcome) >= chatOutcomeCount {
		return
	}
	m.latency.record(latency)
	m.latencyByOp[operation].record(latency)
	m.outcomes[outcome].Add(1)
}

// RecordDenial records one bounded authorization denial.
func (m *ChatMetrics) RecordDenial(reason ChatDenial) {
	if m == nil || reason < 0 || int(reason) >= chatDenialCount {
		return
	}
	m.denials[reason].Add(1)
}

// RecordUpload records one bounded upload outcome.
func (m *ChatMetrics) RecordUpload(outcome ChatUploadOutcome) {
	if m == nil || outcome < 0 || int(outcome) >= chatUploadCount {
		return
	}
	m.uploads[outcome].Add(1)
}

// RecordOutbox records one bounded outbox outcome.
func (m *ChatMetrics) RecordOutbox(outcome ChatOutboxOutcome) {
	if m == nil || outcome < 0 || int(outcome) >= chatOutboxCount {
		return
	}
	m.outbox[outcome].Add(1)
}

// RecordReconnect records one bounded reconnect outcome and latency.
func (m *ChatMetrics) RecordReconnect(outcome ChatReconnectOutcome, latency time.Duration) {
	if m == nil || outcome < 0 || int(outcome) >= chatReconnectCount {
		return
	}
	m.reconnects[outcome].Add(1)
	m.reconnectLatency.record(latency)
}

// RecordSearch records one bounded search outcome and latency.
func (m *ChatMetrics) RecordSearch(outcome ChatSearchOutcome, latency time.Duration) {
	if m == nil || outcome < 0 || int(outcome) >= chatSearchCount {
		return
	}
	m.searches[outcome].Add(1)
	m.searchLatency.record(latency)
}

// Summary returns a snapshot of all chat metrics.
func (m *ChatMetrics) Summary() ChatMetricsSummary {
	if m == nil {
		return ChatMetricsSummary{}
	}
	s := ChatMetricsSummary{
		Latency:          m.latency.summary(),
		ReconnectLatency: m.reconnectLatency.summary(),
		SearchLatency:    m.searchLatency.summary(),
	}
	for i := 0; i < chatOperationCount; i++ {
		s.LatencyByOperation[i] = m.latencyByOp[i].summary()
	}
	for i := 0; i < chatOutcomeCount; i++ {
		s.Outcomes[i] = m.outcomes[i].Load()
	}
	for i := 0; i < chatDenialCount; i++ {
		s.Denials[i] = m.denials[i].Load()
	}
	for i := 0; i < chatUploadCount; i++ {
		s.Uploads[i] = m.uploads[i].Load()
	}
	for i := 0; i < chatOutboxCount; i++ {
		s.Outbox[i] = m.outbox[i].Load()
	}
	for i := 0; i < chatReconnectCount; i++ {
		s.Reconnects[i] = m.reconnects[i].Load()
	}
	for i := 0; i < chatSearchCount; i++ {
		s.Searches[i] = m.searches[i].Load()
	}
	return s
}
