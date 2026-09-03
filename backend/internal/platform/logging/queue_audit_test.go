package logging

import (
	"strings"
	"testing"
)

// forbiddenQueueAuditValues are canary strings that must never appear anywhere
// in queue audit metadata: note text, grade values, student names, media/room
// references.
var forbiddenQueueAuditValues = []string{
	"SECRET-NOTE-TEXT",
	"GRADE-VALUE",
	"STUDENT-NAME",
	"wss://media.example",
	"LIVEKIT-ROOM",
}

func TestQueueAuditBuildersUseStableActionsAndRedactSensitiveData(t *testing.T) {
	policy := QueuePolicyChangeEvent("actor-1", "session-1", map[string][2]string{
		"queue_population": {"all_active_students", "manual_selection"},
		"grade_correction": {"audited_any_time", "immutable"},
	})
	request := QueueOptOutRequestEvent("student-1", "session-1", "entry-1")
	decision := QueueOptOutDecisionEvent("manager-1", "session-1", "request-1", "approved")
	correction := QueueGradeCorrectionEvent("teacher-1", "session-1", "entry-1", GradeCorrectionShape{
		FieldsChanged:     []string{"grade"},
		GradeChanged:      true,
		NoteChanged:       false,
		NoteCleared:       false,
		PriorGradePresent: true,
	})

	events := []AuditEvent{policy, request, decision, correction}
	wantActions := []string{ActionQueuePolicyChange, ActionQueueOptOutRequest, ActionQueueOptOutDecision, ActionQueueGradeCorrection}
	wantSessions := []string{"session-1", "session-1", "session-1", "session-1"}
	for i, event := range events {
		if event.Action != wantActions[i] {
			t.Fatalf("event %d action = %q, want %q", i, event.Action, wantActions[i])
		}
		if event.ActorUserID == "" {
			t.Fatalf("event %q missing actor attribution", event.Action)
		}
		if event.SessionID != wantSessions[i] {
			t.Fatalf("event %q session id = %q, want %q", event.Action, event.SessionID, wantSessions[i])
		}
		assertNoForbiddenQueueValues(t, event.Action, event.Metadata)
	}

	// Policy changes record prior/current closed-enum values per dimension.
	changes, ok := policy.Metadata["changes"].(map[string][2]string)
	if !ok {
		t.Fatalf("policy event metadata missing changes map: %v", policy.Metadata)
	}
	if got := changes["queue_population"]; got != [2]string{"all_active_students", "manual_selection"} {
		t.Fatalf("queue_population change = %v, want prior/current enum pair", got)
	}
	if got := changes["grade_correction"]; got != [2]string{"audited_any_time", "immutable"} {
		t.Fatalf("grade_correction change = %v, want prior/current enum pair", got)
	}

	// Opt-out request records only the entry reference.
	if request.Metadata["entry_id"] != "entry-1" {
		t.Fatalf("opt-out request metadata = %v, want entry_id", request.Metadata)
	}

	// Opt-out decision records the request reference and decision enum only.
	if decision.Metadata["request_id"] != "request-1" || decision.Metadata["decision"] != "approved" {
		t.Fatalf("opt-out decision metadata = %v, want request_id + decision enum", decision.Metadata)
	}

	// Grade correction records exactly the change shape — no grade values, no note text.
	wantKeys := []string{"entry_id", "fields_changed", "grade_changed", "note_changed", "note_cleared", "prior_grade_present"}
	if len(correction.Metadata) != len(wantKeys) {
		t.Fatalf("correction metadata keys = %v, want exactly %v", correction.Metadata, wantKeys)
	}
	for _, key := range wantKeys {
		if _, ok := correction.Metadata[key]; !ok {
			t.Fatalf("correction metadata missing %q: %v", key, correction.Metadata)
		}
	}
	if correction.Metadata["grade_changed"] != true || correction.Metadata["prior_grade_present"] != true {
		t.Fatalf("correction shape flags wrong: %v", correction.Metadata)
	}
	if correction.Metadata["note_changed"] != false || correction.Metadata["note_cleared"] != false {
		t.Fatalf("correction shape flags wrong: %v", correction.Metadata)
	}
}

// assertNoForbiddenQueueValues fails when any canary value appears anywhere in
// the metadata tree (nested maps and slices included).
func assertNoForbiddenQueueValues(t *testing.T, action string, value any) {
	t.Helper()
	switch value := value.(type) {
	case map[string]any:
		for _, v := range value {
			assertNoForbiddenQueueValues(t, action, v)
		}
	case map[string][2]string:
		for _, pair := range value {
			for _, v := range pair {
				assertNoForbiddenQueueValues(t, action, v)
			}
		}
	case []any:
		for _, v := range value {
			assertNoForbiddenQueueValues(t, action, v)
		}
	case []string:
		for _, v := range value {
			assertNoForbiddenQueueValues(t, action, v)
		}
	case string:
		for _, forbidden := range forbiddenQueueAuditValues {
			if strings.Contains(value, forbidden) {
				t.Fatalf("event %q metadata leaked forbidden value %q", action, forbidden)
			}
		}
	}
}

func TestSanitizeAuditMetadataDropsQueueSensitiveKeys(t *testing.T) {
	metadata := sanitizeAuditMetadata(map[string]any{
		"note":           "SECRET-NOTE-TEXT",
		"notes":          "SECRET-NOTE-TEXT",
		"teacher_notes":  "SECRET-NOTE-TEXT",
		"grade":          "GRADE-VALUE",
		"grades":         "GRADE-VALUE",
		"grade_value":    "GRADE-VALUE",
		"note_text":      "SECRET-NOTE-TEXT",
		"media":          "wss://media.example",
		"room":           "LIVEKIT-ROOM",
		"media_room_ref": "LIVEKIT-ROOM",
		"entry_id":       "keep",
		"nested":         map[string]any{"note": "SECRET-NOTE-TEXT", "ok": "nested-ok"},
	})
	for _, key := range []string{"note", "notes", "teacher_notes", "grade", "grades", "grade_value", "note_text", "media", "room", "media_room_ref"} {
		if _, ok := metadata[key]; ok {
			t.Fatalf("queue-sensitive key %q was not redacted", key)
		}
	}
	if metadata["entry_id"] != "keep" {
		t.Fatalf("safe metadata changed: %v", metadata)
	}
	nested, ok := metadata["nested"].(map[string]any)
	if !ok {
		t.Fatalf("nested map lost: %v", metadata)
	}
	if _, ok := nested["note"]; ok {
		t.Fatal("nested note was not redacted")
	}
	if nested["ok"] != "nested-ok" {
		t.Fatalf("nested safe metadata changed: %v", nested)
	}
}
