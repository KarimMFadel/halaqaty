//go:build integration

package integration

import (
	"net/http"
	"testing"

	"github.com/KarimMFadel/halaqaty/backend/internal/platform/httpconst"
)

func TestRecitationQueueSecurity_InvalidRequestsDoNotMutateState(t *testing.T) {
	env := setupQueueRBACEnv(t)
	before := env.queueTablesSnapshot(t, env.sessionID)
	headers := map[string]string{
		httpconst.HeaderAuthorization: env.tokens["teacher"],
		httpconst.HeaderSessionID:     env.backendSessions["teacher"],
		httpconst.HeaderContentType:   "application/json",
	}
	requests := []struct {
		name, method, path, body string
	}{
		{"malformed session id", http.MethodGet, "/api/v1/sessions/not-a-uuid/queue", ""},
		{"invalid round enum", http.MethodPost, "/api/v1/sessions/" + env.sessionID + "/queue/rounds", `{"round_type":"invalid","surah_id":1,"from_ayah":1,"to_ayah":7}`},
		{"invalid JSON", http.MethodPost, "/api/v1/sessions/" + env.sessionID + "/queue/rounds", `{`},
		{"unsupported method", http.MethodDelete, "/api/v1/sessions/" + env.sessionID + "/queue", ""},
	}
	for _, request := range requests {
		t.Run(request.name, func(t *testing.T) {
			response := doJSONRequest(t, env.mux, request.method, request.path, request.body, headers)
			if response.Code < http.StatusBadRequest {
				t.Fatalf("status=%d, want 4xx/5xx", response.Code)
			}
			if after := env.queueTablesSnapshot(t, env.sessionID); after != before {
				t.Fatalf("invalid request mutated queue tables: before=%v after=%v", before, after)
			}
		})
	}
}
