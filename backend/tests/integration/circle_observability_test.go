//go:build integration

package integration

import (
	"bytes"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	phttp "github.com/KarimMFadel/halaqaty/backend/internal/platform/http"
	"github.com/KarimMFadel/halaqaty/backend/internal/platform/httpconst"
	"github.com/KarimMFadel/halaqaty/backend/internal/platform/logging"
	"github.com/KarimMFadel/halaqaty/backend/internal/platform/metrics"
)

func TestCircleObservability_RequestAndMutationLogsCarryCorrelationFields(t *testing.T) {
	var output bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&output, nil))
	handler := phttp.RequestIDMiddleware(phttp.LoggerMiddleware(logger, http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			logging.NewAuditLogger(logger).Log(r.Context(), logging.CircleArchiveEvent("teacher-1", "circle-1"))
			w.WriteHeader(http.StatusNoContent)
		},
	)))
	request := httptest.NewRequest(http.MethodDelete, "/circles/circle-1", nil)
	request.Header.Set(httpconst.HeaderRequestID, "request-circle-1")
	recorder := httptest.NewRecorder()

	handler.ServeHTTP(recorder, request)

	if recorder.Header().Get(httpconst.HeaderRequestID) != "request-circle-1" {
		t.Fatal("request ID was not echoed")
	}
	logs := output.String()
	for _, field := range []string{
		`"msg":"http_request"`,
		`"request_id":"request-circle-1"`,
		`"method":"DELETE"`,
		`"path":"/circles/circle-1"`,
		`"duration":`,
		`"msg":"audit_event"`,
		`"action":"circle.archive"`,
		`"circle_id":"circle-1"`,
	} {
		if !strings.Contains(logs, field) {
			t.Errorf("logs missing %s: %s", field, logs)
		}
	}
}

func TestCircleObservability_LatencyAndRejectionMetrics(t *testing.T) {
	metricStore := new(metrics.AuthMetrics)
	metricStore.RecordRequest(12 * time.Millisecond)
	metricStore.RecordRequest(8 * time.Millisecond)
	metricStore.RecordRejection()

	summary := metricStore.Summary()
	if summary.RequestsTotal != 2 || summary.AvgLatencyMs != 10 || summary.RejectionsTotal != 1 {
		t.Fatalf("metrics summary: %+v", summary)
	}
}
