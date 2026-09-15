package logging

import (
	"bytes"
	"context"
	"log/slog"
	"strings"
	"testing"
)

func TestChatAuditEventsExposeSafeIdentifiersAndOutcomes(t *testing.T) {
	var buf bytes.Buffer
	audit := NewAuditLogger(slog.New(slog.NewJSONHandler(&buf, nil)))
	events := []AuditEvent{
		ChatMessageAuditEvent("actor-1", "circle-1", "message-1", ChatOutcomeAccepted),
		ChatUploadAuditEvent("actor-1", "circle-1", "upload-1", ChatOutcomeRejected),
		ChatOutboxAuditEvent("worker", "event-1", ChatOutcomeParked),
		ChatReconnectAuditEvent("actor-1", "circle-1", ChatOutcomeRecovered),
		ChatSearchAuditEvent("actor-1", "circle-1", ChatOutcomeNoResults),
		ChatDenialAuditEvent("actor-1", "circle-1", "message-1", ChatOutcomeDenied),
	}
	for _, event := range events {
		buf.Reset()
		audit.LogChat(context.Background(), event)
		output := buf.String()
		for _, want := range []string{event.Action, event.ActorUserID} {
			if !strings.Contains(output, want) {
				t.Fatalf("audit output %q missing safe value %q", output, want)
			}
		}
		if !strings.Contains(output, "outcome") {
			t.Fatalf("audit output %q missing outcome metadata", output)
		}
	}
}

func TestChatAuditEventsNeverEmitSensitiveChatValues(t *testing.T) {
	var buf bytes.Buffer
	audit := NewAuditLogger(slog.New(slog.NewJSONHandler(&buf, nil)))
	audit.LogChat(context.Background(), ChatMessageAuditEvent("actor-1", "circle-1", "message-1", ChatOutcomeAccepted))
	output := buf.String()
	for _, forbidden := range []string{
		"MESSAGE-BODY", "private-filename.pdf", "bucket/chat/object-key", "https://signed.example/file",
		"FIREBASE-TOKEN", "MINIO-CREDENTIAL", "auth-session-1", "backend-session-1",
	} {
		if strings.Contains(output, forbidden) {
			t.Fatalf("chat audit output leaked %q: %s", forbidden, output)
		}
	}
	if strings.Contains(output, "session_id") || strings.Contains(output, "filename") || strings.Contains(output, "object_key") {
		t.Fatalf("chat audit output contains forbidden field name: %s", output)
	}
}

func TestChatAuditBuildersDiscardSensitiveMetadata(t *testing.T) {
	event := ChatMessageAuditEvent("actor-1", "circle-1", "message-1", ChatOutcomeAccepted)
	event.Metadata["body"] = "MESSAGE-BODY"
	event.Metadata["filename"] = "private-filename.pdf"
	event.Metadata["object_key"] = "bucket/chat/object-key"
	event.Metadata["url"] = "https://signed.example/file"
	event.Metadata["token"] = "FIREBASE-TOKEN"
	event.Metadata["credential"] = "MINIO-CREDENTIAL"
	event.Metadata["auth_session_id"] = "auth-session-1"
	event.Metadata["session_id"] = "backend-session-1"
	event.Metadata["nested"] = map[string]any{"FILENAME": "private-filename.pdf", "safe": "kept"}

	clean := sanitizeChatAuditMetadata(event.Metadata)
	for _, key := range []string{"body", "filename", "object_key", "url", "token", "credential", "auth_session_id", "session_id"} {
		if _, ok := clean[key]; ok {
			t.Fatalf("sensitive chat metadata key %q survived: %v", key, clean)
		}
	}
	nested, ok := clean["nested"].(map[string]any)
	if !ok || nested["safe"] != "kept" {
		t.Fatalf("safe nested metadata changed: %v", clean["nested"])
	}
	if _, ok := nested["FILENAME"]; ok {
		t.Fatal("nested filename survived chat sanitization")
	}
}
