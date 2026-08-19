//go:build integration

package integration

import (
	"bytes"
	"context"
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

func TestLiveSessionsObservability_RequestTimeoutUsesCanonicalEnvelope(t *testing.T) {
	handler := phttp.TimeoutMiddleware(2*time.Millisecond, http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		<-r.Context().Done()
	}))
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/api/v1/sessions/session-1/join", nil))

	if recorder.Code != http.StatusServiceUnavailable {
		t.Fatalf("timeout status = %d, want %d", recorder.Code, http.StatusServiceUnavailable)
	}
	if !strings.Contains(recorder.Body.String(), httpconst.ErrorCodeRequestTimeout) {
		t.Fatalf("timeout body = %s, want %s", recorder.Body.String(), httpconst.ErrorCodeRequestTimeout)
	}
}

func TestLiveSessionsObservability_AuditRedactsSessionSecrets(t *testing.T) {
	var output bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&output, nil))
	logging.NewAuditLogger(logger).Log(context.Background(), logging.SessionCreateEvent(
		"user-1", "backend-session-secret", "mobile",
	))
	logging.NewAuditLogger(logger).Log(context.Background(), logging.AuditEvent{
		Action:      "session.join",
		ActorUserID: "user-1",
		Metadata: map[string]any{
			"session_id":    "backend-session-secret",
			"token":         "media-credential-secret",
			"authorization": "Bearer firebase-secret",
			"safe_reason":   "reconnect",
		},
	})

	logs := output.String()
	for _, secret := range []string{"backend-session-secret", "media-credential-secret", "firebase-secret"} {
		if strings.Contains(logs, secret) {
			t.Fatalf("audit logs contain secret %q: %s", secret, logs)
		}
	}
	if !strings.Contains(logs, "safe_reason") || !strings.Contains(logs, "reconnect") {
		t.Fatalf("safe audit metadata missing: %s", logs)
	}
}

func TestLiveSessionsObservability_AuthMetricsRecordRecoverySignals(t *testing.T) {
	metricStore := new(metrics.AuthMetrics)
	metricStore.RecordRequest(12 * time.Millisecond)
	metricStore.RecordRequest(8 * time.Millisecond)
	metricStore.RecordRejection()
	metricStore.RecordSessionExpiry()

	summary := metricStore.Summary()
	if summary.RequestsTotal != 2 || summary.AvgLatencyMs != 10 || summary.RejectionsTotal != 1 || summary.SessionExpiries != 1 {
		t.Fatalf("metrics summary = %+v", summary)
	}
}
