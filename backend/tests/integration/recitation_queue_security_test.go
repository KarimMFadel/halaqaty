//go:build integration

package integration

import (
	"fmt"
	"net/http"
	"strings"
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
	entryID, entryVersion := env.entryRefFor(t, env.sessionID)
	requests := []struct {
		name, method, path, body string
		headers                  map[string]string
	}{
		{"malformed session id", http.MethodGet, "/api/v1/sessions/not-a-uuid/queue", "", headers},
		{"invalid round enum", http.MethodPost, "/api/v1/sessions/" + env.sessionID + "/queue/rounds", `{"round_type":"invalid","surah_id":1,"from_ayah":1,"to_ayah":7}`, headers},
		{"invalid Quran range", http.MethodPost, "/api/v1/sessions/" + env.sessionID + "/queue/rounds", `{"round_type":"revision","surah_id":1,"from_ayah":8,"to_ayah":7}`, headers},
		{"invalid Quran surah", http.MethodPost, "/api/v1/sessions/" + env.sessionID + "/queue/rounds", `{"round_type":"revision","surah_id":0,"from_ayah":1,"to_ayah":7}`, headers},
		{"invalid JSON", http.MethodPost, "/api/v1/sessions/" + env.sessionID + "/queue/rounds", `{`, headers},
		{"invalid body field type", http.MethodPost, "/api/v1/sessions/" + env.sessionID + "/queue/advance", `{"expected_version":"one"}`, headers},
		{"invalid policy enum", http.MethodPatch, "/api/v1/sessions/" + env.sessionID + "/queue/policy", fmt.Sprintf(`{"opt_out":"invalid","expected_version":%d}`, env.policyVersion(t, env.sessionID)), headers},
		{"invalid grade visibility policy", http.MethodPatch, "/api/v1/sessions/" + env.sessionID + "/queue/policy", fmt.Sprintf(`{"grade_visibility":"invalid","expected_version":%d}`, env.policyVersion(t, env.sessionID)), headers},
		{"duplicate queue order", http.MethodPut, "/api/v1/sessions/" + env.sessionID + "/queue/order", fmt.Sprintf(`{"ordered_ids":[%q,%q],"expected_version":%d}`, env.userIDs["student"], env.userIDs["student"], env.roundVersion(t, env.sessionID, "prepared")), headers},
		{"non-member in queue order", http.MethodPut, "/api/v1/sessions/" + env.sessionID + "/queue/order", fmt.Sprintf(`{"ordered_ids":[%q,%q],"expected_version":%d}`, env.userIDs["student"], env.userIDs["outsider"], env.roundVersion(t, env.sessionID, "prepared")), headers},
		{"stale round version", http.MethodPost, "/api/v1/sessions/" + env.sessionID + "/queue/advance", fmt.Sprintf(`{"expected_version":%d}`, env.roundVersion(t, env.sessionID, "prepared")+100), headers},
		{"invalid entry UUID", http.MethodPost, "/api/v1/sessions/" + env.sessionID + "/queue/entries/not-a-uuid/move", `{"new_position":1,"expected_version":1}`, headers},
		{"invalid entry position", http.MethodPost, "/api/v1/sessions/" + env.sessionID + "/queue/entries/" + entryID + "/move", fmt.Sprintf(`{"new_position":0,"expected_version":%d}`, env.roundVersion(t, env.sessionID, "prepared")), headers},
		{"stale entry version", http.MethodPut, "/api/v1/sessions/" + env.sessionID + "/queue/entries/" + entryID + "/status", fmt.Sprintf(`{"status":"skipped","expected_entry_version":%d}`, entryVersion+100), headers},
		{"invalid transition status", http.MethodPut, "/api/v1/sessions/" + env.sessionID + "/queue/entries/" + entryID + "/status", fmt.Sprintf(`{"status":"not-a-status","expected_entry_version":%d}`, entryVersion), headers},
		{"invalid grade enum", http.MethodPut, "/api/v1/sessions/" + env.sessionID + "/queue/entries/" + entryID + "/status", fmt.Sprintf(`{"status":"completed","grade":"invalid","expected_entry_version":%d}`, entryVersion), headers},
		{"notes on non-completed transition", http.MethodPut, "/api/v1/sessions/" + env.sessionID + "/queue/entries/" + entryID + "/status", fmt.Sprintf(`{"status":"skipped","notes":"must not persist","expected_entry_version":%d}`, entryVersion), headers},
		{"overlong note", http.MethodPut, "/api/v1/sessions/" + env.sessionID + "/queue/entries/" + entryID + "/status", fmt.Sprintf(`{"status":"completed","grade":"good","notes":%q,"expected_entry_version":%d}`, strings.Repeat("x", 501), entryVersion), headers},
		{"unsupported method", http.MethodDelete, "/api/v1/sessions/" + env.sessionID + "/queue", "", headers},
		{"unauthenticated", http.MethodGet, "/api/v1/sessions/" + env.sessionID + "/queue", "", nil},
		{"missing current-device session", http.MethodGet, "/api/v1/sessions/" + env.sessionID + "/queue", "", map[string]string{httpconst.HeaderAuthorization: env.tokens["teacher"]}},
		{"mismatched current-device session", http.MethodGet, "/api/v1/sessions/" + env.sessionID + "/queue", "", map[string]string{httpconst.HeaderAuthorization: env.tokens["teacher"], httpconst.HeaderSessionID: env.backendSessions["student"]}},
		{"non-member", http.MethodGet, "/api/v1/sessions/" + env.sessionID + "/queue", "", map[string]string{httpconst.HeaderAuthorization: env.tokens["outsider"], httpconst.HeaderSessionID: env.backendSessions["outsider"]}},
		{"non-member mutation", http.MethodPost, "/api/v1/sessions/" + env.sessionID + "/queue/advance", fmt.Sprintf(`{"expected_version":%d}`, env.roundVersion(t, env.sessionID, "prepared")), map[string]string{httpconst.HeaderAuthorization: env.tokens["outsider"], httpconst.HeaderSessionID: env.backendSessions["outsider"], httpconst.HeaderContentType: httpconst.ContentTypeApplicationJSON}},
	}
	for _, request := range requests {
		t.Run(request.name, func(t *testing.T) {
			response := doJSONRequest(t, env.mux, request.method, request.path, request.body, request.headers)
			if response.Code < http.StatusBadRequest {
				t.Fatalf("status=%d, want 4xx/5xx", response.Code)
			}
			if after := env.queueTablesSnapshot(t, env.sessionID); after != before {
				t.Fatalf("invalid request mutated queue tables: before=%v after=%v", before, after)
			}
		})
	}

	t.Run("reused idempotency key with a different command does not mutate twice", func(t *testing.T) {
		keyHeaders := make(map[string]string, len(headers)+1)
		for name, value := range headers {
			keyHeaders[name] = value
		}
		keyHeaders["Idempotency-Key"] = "phase6-reused-command"
		path := "/api/v1/sessions/" + env.sessionID + "/queue/rounds"
		first := doJSONRequest(t, env.mux, http.MethodPost, path, `{"round_type":"test","surah_id":2,"from_ayah":1,"to_ayah":3,"grading_required":false}`, keyHeaders)
		if first.Code != http.StatusCreated {
			t.Fatalf("first idempotent command status=%d: %s", first.Code, first.Body.String())
		}
		beforeReplay := env.queueTablesSnapshot(t, env.sessionID)
		replay := doJSONRequest(t, env.mux, http.MethodPost, path, `{"round_type":"old_revision","surah_id":2,"from_ayah":4,"to_ayah":8,"grading_required":false}`, keyHeaders)
		if replay.Code != http.StatusConflict {
			t.Fatalf("reused key status=%d, want 409: %s", replay.Code, replay.Body.String())
		}
		if afterReplay := env.queueTablesSnapshot(t, env.sessionID); afterReplay != beforeReplay {
			t.Fatalf("reused idempotency key mutated queue tables: before=%v after=%v", beforeReplay, afterReplay)
		}
	})
}
