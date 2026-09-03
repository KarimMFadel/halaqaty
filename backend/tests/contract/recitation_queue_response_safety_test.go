//go:build contract

package contract

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/KarimMFadel/halaqaty/backend/internal/platform/logging"
	"github.com/KarimMFadel/halaqaty/backend/internal/platform/metrics"
	"github.com/KarimMFadel/halaqaty/backend/internal/queue"
)

// TestRecitationQueueResponseSafety exercises public queue responses and
// persisted delivery metadata rather than inspecting implementation files.
func TestRecitationQueueResponseSafety(t *testing.T) {
	env := setupRqcEnv(t)
	sessionID, state := env.liveRound(t)
	responses := []struct {
		name, actor, method, path, body string
		want                            int
	}{
		{"snapshot", env.teacherID, http.MethodGet, "/api/v1/sessions/" + sessionID + "/queue", "", http.StatusOK},
		{"invalid request error", env.teacherID, http.MethodPost, "/api/v1/sessions/" + sessionID + "/queue/rounds", `{`, http.StatusBadRequest},
		{"queue mutation", env.teacherID, http.MethodPost, "/api/v1/sessions/" + sessionID + "/queue/advance", `{"expected_version":` + strconv.FormatInt(int64(rqcNum(t, state, "version")), 10) + `}`, http.StatusOK},
	}
	for _, tc := range responses {
		t.Run(tc.name, func(t *testing.T) {
			idempotencyKey := ""
			if tc.name == "queue mutation" {
				idempotencyKey = "response-safety-advance"
			}
			response := env.req(t, tc.actor, tc.method, tc.path, tc.body, idempotencyKey)
			if response.Code != tc.want {
				t.Fatalf("status=%d, want %d: %s", response.Code, tc.want, response.Body.String())
			}
			assertNoQueueMediaMaterial(t, response.Body.String())
			for name, values := range response.Header() {
				assertNoQueueMediaMaterial(t, name+":"+strings.Join(values, ","))
			}
			if response.Code >= http.StatusMultipleChoices && response.Code < http.StatusBadRequest {
				t.Fatalf("queue response unexpectedly redirected: status=%d location=%q", response.Code, response.Header().Get("Location"))
			}
		})
	}

	stateEvent, err := queue.NewRealtimeOutboxProjector(queue.NewQueueRepository(env.pool), nil).QueueState(context.Background(), env.teacherID, sessionID)
	if err != nil {
		t.Fatalf("project client-visible queue.state: %v", err)
	}
	stateJSON, err := json.Marshal(stateEvent)
	if err != nil {
		t.Fatalf("marshal client-visible queue.state: %v", err)
	}
	assertNoQueueMediaMaterial(t, string(stateJSON))

	rows, err := env.pool.Query(context.Background(), `
		SELECT event_type || ':' || event_metadata::text
		FROM queue_event_outbox
		WHERE session_id = $1::uuid
	`, sessionID)
	if err != nil {
		t.Fatalf("read queue delivery metadata: %v", err)
	}
	defer rows.Close()
	for rows.Next() {
		var record string
		if err := rows.Scan(&record); err != nil {
			t.Fatalf("scan queue delivery metadata: %v", err)
		}
		assertNoQueueMediaMaterial(t, record)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate queue delivery metadata: %v", err)
	}

	var receipt string
	if err := env.pool.QueryRow(context.Background(), `
		SELECT row_to_json(r)::text
		FROM queue_command_receipts r
		WHERE session_id = $1::uuid AND idempotency_key = 'response-safety-advance'
	`, sessionID).Scan(&receipt); err != nil {
		t.Fatalf("read queue command receipt: %v", err)
	}
	assertNoQueueMediaMaterial(t, receipt)

	var auditOutput bytes.Buffer
	auditLogger := logging.NewAuditLogger(slog.New(slog.NewJSONHandler(&auditOutput, nil)))
	auditLogger.Log(context.Background(), logging.AuditEvent{
		Action:      logging.ActionQueueGradeCorrection,
		ActorUserID: env.teacherID,
		SessionID:   sessionID,
		Metadata: map[string]any{
			"entry_id":       "safe-entry-id",
			"credential":     "provider-secret-canary",
			"media_room_ref": "room-secret-canary",
			"notes":          "private-note-canary",
		},
	})
	assertNoQueueMediaMaterial(t, auditOutput.String())
	for _, canary := range []string{"provider-secret-canary", "room-secret-canary", "private-note-canary"} {
		if strings.Contains(auditOutput.String(), canary) {
			t.Fatalf("structured queue audit log leaked %q: %s", canary, auditOutput.String())
		}
	}

	queueMetrics := new(metrics.QueueMetrics)
	queueMetrics.RecordCommandDuration(12 * time.Millisecond)
	queueMetrics.RecordEventDeliveryLag(20 * time.Millisecond)
	queueMetrics.RecordConflict(metrics.ConflictStaleVersion)
	metricsJSON, err := json.Marshal(queueMetrics.Summary())
	if err != nil {
		t.Fatalf("marshal queue metrics summary: %v", err)
	}
	assertNoQueueMediaMaterial(t, string(metricsJSON))
}

func assertNoQueueMediaMaterial(t *testing.T, value string) {
	t.Helper()
	lower := strings.ToLower(value)
	for _, forbidden := range []string{"livekit", "media_credential", "credential", "media_room_ref", "room_ref", "wss://", "provider_secret", "stack trace", "select *", "pq:"} {
		if strings.Contains(lower, forbidden) {
			t.Fatalf("client-visible queue value leaked forbidden detail %q: %s", forbidden, value)
		}
	}
}
