package metrics

import (
	"reflect"
	"testing"
	"time"
)

func TestChatMetricsRecordsBoundedLatencyOutcomesAndDomainCounters(t *testing.T) {
	m := new(ChatMetrics)
	m.RecordLatencyOutcome(ChatOperationSend, ChatOutcomeAccepted, 100*time.Millisecond)
	m.RecordLatencyOutcome(ChatOperationSend, ChatOutcomeConflict, 2*time.Second)
	m.RecordLatencyOutcome(ChatOperationHistory, ChatOutcomeFailure, 300*time.Millisecond)
	m.RecordDenial(ChatDenialUnauthorized)
	m.RecordUpload(ChatUploadRejected)
	m.RecordOutbox(ChatOutboxParked)
	m.RecordReconnect(ChatReconnectRecovered, 400*time.Millisecond)
	m.RecordSearch(ChatSearchNoResults, 500*time.Millisecond)

	s := m.Summary()
	if s.Latency.Count != 3 || s.Latency.MaxMs != 2000 {
		t.Fatalf("latency = %+v, want three samples and 2000ms max", s.Latency)
	}
	if s.LatencyByOperation[ChatOperationSend].Count != 2 {
		t.Fatalf("send latency = %+v, want two samples", s.LatencyByOperation[ChatOperationSend])
	}
	if s.Outcomes[ChatOutcomeAccepted] != 1 || s.Outcomes[ChatOutcomeConflict] != 1 || s.Outcomes[ChatOutcomeFailure] != 1 {
		t.Fatalf("outcomes = %v", s.Outcomes)
	}
	if s.Denials[ChatDenialUnauthorized] != 1 || s.Uploads[ChatUploadRejected] != 1 || s.Outbox[ChatOutboxParked] != 1 {
		t.Fatalf("domain counters = denials=%v uploads=%v outbox=%v", s.Denials, s.Uploads, s.Outbox)
	}
	if s.Reconnects[ChatReconnectRecovered] != 1 || s.Searches[ChatSearchNoResults] != 1 {
		t.Fatalf("reconnect/search counters = %v/%v", s.Reconnects, s.Searches)
	}
	if s.ReconnectLatency.Count != 1 || s.SearchLatency.Count != 1 {
		t.Fatalf("specialized latency = %+v/%+v", s.ReconnectLatency, s.SearchLatency)
	}
}

func TestChatMetricsIgnoreInvalidEnumsAndNilReceivers(t *testing.T) {
	var m *ChatMetrics
	m.RecordLatencyOutcome(ChatOperationSend, ChatOutcomeAccepted, time.Second)
	m.RecordDenial(ChatDenial(-1))
	if got := m.Summary(); got != (ChatMetricsSummary{}) {
		t.Fatalf("nil summary = %+v, want zero", got)
	}

	m = new(ChatMetrics)
	m.RecordLatencyOutcome(ChatOperation(-1), ChatOutcomeAccepted, time.Second)
	m.RecordLatencyOutcome(ChatOperationSend, ChatOutcome(99), time.Second)
	m.RecordDenial(ChatDenial(99))
	m.RecordUpload(ChatUploadOutcome(99))
	m.RecordOutbox(ChatOutboxOutcome(99))
	m.RecordReconnect(ChatReconnectOutcome(99), time.Second)
	m.RecordSearch(ChatSearchOutcome(99), time.Second)
	if got := m.Summary(); got != (ChatMetricsSummary{}) {
		t.Fatalf("invalid enum summary = %+v, want zero", got)
	}
}

func TestChatMetricsHaveNoArbitraryLabelStorage(t *testing.T) {
	typeInfo := reflect.TypeOf(ChatMetrics{})
	for i := 0; i < typeInfo.NumField(); i++ {
		if typeInfo.Field(i).Type.Kind() == reflect.Map {
			t.Fatalf("ChatMetrics field %q is an unbounded map", typeInfo.Field(i).Name)
		}
	}
	for i := 0; i < reflect.TypeOf(ChatMetricsSummary{}).NumField(); i++ {
		if reflect.TypeOf(ChatMetricsSummary{}).Field(i).Type.Kind() == reflect.Map {
			t.Fatalf("ChatMetricsSummary field %q is an unbounded map", reflect.TypeOf(ChatMetricsSummary{}).Field(i).Name)
		}
	}
}
