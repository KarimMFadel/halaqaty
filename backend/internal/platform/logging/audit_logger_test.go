package logging

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"strings"
	"testing"
)

func TestNewAuditLogger_NilLogger_FallsBackToDefaultLogger(t *testing.T) {
	previous := slog.Default()
	var buf bytes.Buffer
	slog.SetDefault(slog.New(slog.NewTextHandler(&buf, nil)))
	defer slog.SetDefault(previous)

	audit := NewAuditLogger(nil)
	if audit == nil {
		t.Fatal("NewAuditLogger(nil) must fall back to the default logger")
	}
	audit.Log(context.Background(), ProfileUpdateEvent("user-1", []string{"display_name"}))
	if !strings.Contains(buf.String(), "action="+ActionProfileUpdate) {
		t.Fatalf("nil logger must route events to slog.Default(), got %q", buf.String())
	}
}

func TestAuditLogger_LogEmitsSanitizedStructuredEvent(t *testing.T) {
	var buf bytes.Buffer
	audit := NewAuditLogger(slog.New(slog.NewJSONHandler(&buf, nil)))

	audit.Log(context.Background(), AuditEvent{
		Action:      ActionProfileUpdate,
		ActorUserID: "actor-1",
		TargetUser:  "target-1",
		CircleID:    "circle-1",
		SessionID:   "session-1",
		Metadata: map[string]any{
			"token":          "SECRET-TOKEN",
			"changed_fields": []string{"display_name"},
		},
	})

	var record map[string]any
	if err := json.Unmarshal(buf.Bytes(), &record); err != nil {
		t.Fatalf("decode audit record %q: %v", buf.String(), err)
	}
	if record["msg"] != "audit_event" {
		t.Fatalf("message = %v, want audit_event", record["msg"])
	}
	if record["action"] != ActionProfileUpdate || record["actor_user_id"] != "actor-1" {
		t.Fatalf("attribution wrong: %v", record)
	}
	if record["target_user_id"] != "target-1" || record["circle_id"] != "circle-1" || record["session_id"] != "session-1" {
		t.Fatalf("event references wrong: %v", record)
	}
	metadata, ok := record["metadata"].(map[string]any)
	if !ok {
		t.Fatalf("metadata missing from record: %v", record)
	}
	if _, leaked := metadata["token"]; leaked {
		t.Fatal("audit log leaked a credential-bearing metadata key")
	}
	fields, ok := metadata["changed_fields"].([]any)
	if !ok || len(fields) != 1 || fields[0] != "display_name" {
		t.Fatalf("safe metadata must survive sanitization, got %v", metadata["changed_fields"])
	}
	if _, ok := record["at"]; !ok {
		t.Fatal("audit record must carry an event timestamp")
	}
}

func TestProfileUpdateEvent_CarriesChangedFieldsOnly(t *testing.T) {
	event := ProfileUpdateEvent("user-1", []string{"display_name", "avatar_url"})
	if event.Action != ActionProfileUpdate {
		t.Fatalf("action = %q, want %q", event.Action, ActionProfileUpdate)
	}
	if event.ActorUserID != "user-1" {
		t.Fatalf("actor = %q, want user-1", event.ActorUserID)
	}
	fields, ok := event.Metadata["changed_fields"].([]string)
	if !ok || len(fields) != 2 || fields[0] != "display_name" || fields[1] != "avatar_url" {
		t.Fatalf("changed_fields = %v, want field names only", event.Metadata["changed_fields"])
	}
	if event.TargetUser != "" || event.CircleID != "" || event.SessionID != "" {
		t.Fatalf("profile update must not claim other resources: %+v", event)
	}
}

func TestSanitizeAuditValue_PreservesContainersAndDropsNestedDenyKeys(t *testing.T) {
	got := sanitizeAuditValue([]any{
		map[string]any{"token": "SECRET-TOKEN", "safe": "kept"},
		"plain",
		42,
	})
	items, ok := got.([]any)
	if !ok || len(items) != 3 {
		t.Fatalf("slice sanitization = %#v, want three items", got)
	}
	nested, ok := items[0].(map[string]any)
	if !ok {
		t.Fatalf("nested map lost: %#v", items[0])
	}
	if _, leaked := nested["token"]; leaked {
		t.Fatal("deny-listed key inside a slice element was not redacted")
	}
	if nested["safe"] != "kept" {
		t.Fatalf("safe value inside a slice element changed: %v", nested)
	}
	if items[1] != "plain" || items[2] != 42 {
		t.Fatalf("scalar slice items changed: %#v", items)
	}

	if got := sanitizeAuditValue("scalar"); got != "scalar" {
		t.Fatalf("scalar value changed: %#v", got)
	}
	if got := sanitizeAuditValue(nil); got != nil {
		t.Fatalf("nil value changed: %#v", got)
	}
}
