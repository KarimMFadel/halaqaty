// Package metrics provides lightweight instrumentation counters for auth flows.
package metrics

import (
	"sync/atomic"
	"time"
)

// AuthMetrics tracks auth request latency, rejections, and session-expiry events
// using lock-free atomic counters.
// ponytail: plain atomic counters; wire a Prometheus exporter when scraping is needed.
type AuthMetrics struct {
	requestsTotal   atomic.Int64
	rejectionsTotal atomic.Int64
	sessionExpiries atomic.Int64
	latencyTotalMs  atomic.Int64
}

// RecordRequest increments the request counter and accumulates the latency sample.
func (m *AuthMetrics) RecordRequest(latency time.Duration) {
	if m == nil {
		return
	}
	m.requestsTotal.Add(1)
	m.latencyTotalMs.Add(latency.Milliseconds())
}

// RecordRejection increments the rejection counter (missing/invalid/revoked credentials).
func (m *AuthMetrics) RecordRejection() {
	if m == nil {
		return
	}
	m.rejectionsTotal.Add(1)
}

// RecordSessionExpiry increments the session-expiry eviction counter.
func (m *AuthMetrics) RecordSessionExpiry() {
	if m == nil {
		return
	}
	m.sessionExpiries.Add(1)
}

// MetricsSummary is a point-in-time snapshot of the auth metric counters.
type MetricsSummary struct {
	RequestsTotal   int64
	RejectionsTotal int64
	SessionExpiries int64
	AvgLatencyMs    int64
}

// Summary returns a snapshot of current counters.
func (m *AuthMetrics) Summary() MetricsSummary {
	if m == nil {
		return MetricsSummary{}
	}
	total := m.requestsTotal.Load()
	latencyMs := m.latencyTotalMs.Load()
	avg := int64(0)
	if total > 0 {
		avg = latencyMs / total
	}
	return MetricsSummary{
		RequestsTotal:   total,
		RejectionsTotal: m.rejectionsTotal.Load(),
		SessionExpiries: m.sessionExpiries.Load(),
		AvgLatencyMs:    avg,
	}
}
