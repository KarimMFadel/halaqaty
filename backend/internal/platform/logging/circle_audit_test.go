package logging

import "testing"

func TestCircleAuditEventsUseStableActionsAndRedactSecrets(t *testing.T) {
	events := []AuditEvent{
		CircleJoinEvent("actor", "circle"),
		InviteRefreshEvent("actor", "circle"),
		MemberRemovalEvent("actor", "target", "circle"),
		CircleArchiveEvent("actor", "circle"),
	}
	want := []string{ActionCircleJoin, ActionInviteRefresh, ActionMemberRemoval, ActionCircleArchive}
	for i, event := range events {
		if event.Action != want[i] {
			t.Fatalf("event %d action = %q, want %q", i, event.Action, want[i])
		}
		if _, ok := event.Metadata["invite_code"]; ok {
			t.Fatalf("event %q leaked invite code", event.Action)
		}
	}
}

func TestSanitizeAuditMetadataDropsCredentialKeys(t *testing.T) {
	metadata := sanitizeAuditMetadata(map[string]any{"invite_code": "secret", "token": "secret", "safe": "ok", "nested": map[string]any{"password": "secret", "safe": "nested-ok"}})
	if _, ok := metadata["invite_code"]; ok {
		t.Fatal("invite_code was not redacted")
	}
	if _, ok := metadata["token"]; ok {
		t.Fatal("token was not redacted")
	}
	if metadata["safe"] != "ok" {
		t.Fatalf("safe metadata changed: %v", metadata)
	}
	if _, ok := metadata["nested"].(map[string]any)["password"]; ok {
		t.Fatal("nested password was not redacted")
	}
}
