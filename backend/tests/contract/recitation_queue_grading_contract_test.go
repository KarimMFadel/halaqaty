//go:build contract

package contract

import (
	"fmt"
	"net/http"
	"strings"
	"testing"
)

// T053 — behavioral REST contract tests for F-003 US3 grading and correction
// flows, pinned to specs/003-recitation-queue-system/contracts/recitation-queue.openapi.yaml.
// These run queue.Handler against real PostgreSQL and are the red spec for the
// grade-related surface introduced in Phase 5.

// selectAndStartFirstEntry drives advance→start for the first waiting entry of
// a fresh live round and returns the session ID, the entry, and the entry's
// post-start version. Each test then performs its own completion request.
func selectAndStartFirstEntry(t *testing.T, env *rqcEnv) (string, map[string]any, int) {
	t.Helper()
	sessionID, state := env.liveRound(t)
	entries := rqcAssertEntries(t, state, "waiting", 3)
	entry := entries[0]

	rec := env.req(t, env.teacherID, http.MethodPost, "/api/v1/sessions/"+sessionID+"/queue/advance",
		fmt.Sprintf(`{"expected_version":%d}`, int(rqcNum(t, state, "version"))), "")
	if rec.Code != http.StatusOK {
		t.Fatalf("advance: got %d want 200 body=%s", rec.Code, rec.Body.String())
	}
	state = rqcDecode(t, rec)
	entryVersion := int(rqcEntryVersion(t, state, rqcStr(t, entry, "id")))

	rec = env.req(t, env.teacherID, http.MethodPut,
		"/api/v1/sessions/"+sessionID+"/queue/entries/"+rqcStr(t, entry, "id")+"/status",
		fmt.Sprintf(`{"status":"reciting","expected_entry_version":%d}`, entryVersion), "")
	if rec.Code != http.StatusOK {
		t.Fatalf("start: got %d want 200 body=%s", rec.Code, rec.Body.String())
	}
	state = rqcDecode(t, rec)
	return sessionID, entry, int(rqcEntryVersion(t, state, rqcStr(t, entry, "id")))
}

func completeEntry(t *testing.T, env *rqcEnv, sessionID string, entry map[string]any, entryVersion int, body string) map[string]any {
	t.Helper()
	rec := env.req(t, env.teacherID, http.MethodPut,
		"/api/v1/sessions/"+sessionID+"/queue/entries/"+rqcStr(t, entry, "id")+"/status", body, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("complete: got %d want 200 body=%s", rec.Code, rec.Body.String())
	}
	state := rqcDecode(t, rec)
	rqcAssertQueueState(t, state)
	return state
}

func findEntryInState(t *testing.T, state map[string]any, entry map[string]any) map[string]any {
	t.Helper()
	for _, e := range rqcObjects(t, state, "entries") {
		if rqcStr(t, e, "id") == rqcStr(t, entry, "id") {
			return e
		}
	}
	t.Fatal("entry missing from queue state")
	return nil
}

func TestRecitationQueueGrading_CompleteRecitingEntry(t *testing.T) {
	env := setupRqcEnv(t)
	sessionID, entry, entryVersion := selectAndStartFirstEntry(t, env)

	state := completeEntry(t, env, sessionID, entry, entryVersion,
		fmt.Sprintf(`{"status":"completed","grade":"good","notes":"well done","expected_entry_version":%d}`, entryVersion))

	completed := findEntryInState(t, state, entry)
	if rqcStr(t, completed, "status") != "completed" {
		t.Fatalf("entry status = %q, want completed", rqcStr(t, completed, "status"))
	}
	if rqcStr(t, completed, "grade") != "good" {
		t.Fatalf("entry grade = %q, want good", rqcStr(t, completed, "grade"))
	}
	if rqcStr(t, completed, "grade_notes") != "well done" {
		t.Fatalf("entry grade_notes = %q, want well done", rqcStr(t, completed, "grade_notes"))
	}
}

func TestRecitationQueueGrading_GradingRequiredMissingGradeReturns422(t *testing.T) {
	env := setupRqcEnv(t)
	sessionID, entry, entryVersion := selectAndStartFirstEntry(t, env)

	rec := env.req(t, env.teacherID, http.MethodPut,
		"/api/v1/sessions/"+sessionID+"/queue/entries/"+rqcStr(t, entry, "id")+"/status",
		fmt.Sprintf(`{"status":"completed","expected_entry_version":%d}`, entryVersion), "")
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("missing grade: got %d want 422 body=%s", rec.Code, rec.Body.String())
	}
	envl := rqcDecodeError(t, rec)
	if envl.Error.Code == "" {
		t.Fatal("422 must carry a standard error code")
	}
}

func TestRecitationQueueGrading_CorrectCompletedEntry(t *testing.T) {
	env := setupRqcEnv(t)
	sessionID, entry, entryVersion := selectAndStartFirstEntry(t, env)

	state := completeEntry(t, env, sessionID, entry, entryVersion,
		fmt.Sprintf(`{"status":"completed","grade":"acceptable","notes":"initial","expected_entry_version":%d}`, entryVersion))
	entryVersion = int(rqcEntryVersion(t, state, rqcStr(t, entry, "id")))

	rec := env.req(t, env.teacherID, http.MethodPost,
		"/api/v1/sessions/"+sessionID+"/queue/entries/"+rqcStr(t, entry, "id")+"/grade",
		fmt.Sprintf(`{"grade":"good","notes":"corrected","expected_entry_version":%d}`, entryVersion), "")
	if rec.Code != http.StatusOK {
		t.Fatalf("correct: got %d want 200 body=%s", rec.Code, rec.Body.String())
	}
	updated := rqcDecode(t, rec)
	if rqcStr(t, updated, "status") != "completed" {
		t.Fatalf("corrected entry status = %q, want completed", rqcStr(t, updated, "status"))
	}
	if rqcStr(t, updated, "grade") != "good" {
		t.Fatalf("corrected grade = %q, want good", rqcStr(t, updated, "grade"))
	}
	if rqcStr(t, updated, "grade_notes") != "corrected" {
		t.Fatalf("corrected notes = %q, want corrected", rqcStr(t, updated, "grade_notes"))
	}
}

func TestRecitationQueueGrading_NotesTooLongReturns422(t *testing.T) {
	env := setupRqcEnv(t)
	sessionID, entry, entryVersion := selectAndStartFirstEntry(t, env)

	longNotes := strings.Repeat("x", 501)
	rec := env.req(t, env.teacherID, http.MethodPut,
		"/api/v1/sessions/"+sessionID+"/queue/entries/"+rqcStr(t, entry, "id")+"/status",
		fmt.Sprintf(`{"status":"completed","grade":"good","notes":%q,"expected_entry_version":%d}`, longNotes, entryVersion), "")
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("long notes: got %d want 422 body=%s", rec.Code, rec.Body.String())
	}
}

func TestRecitationQueueGrading_NullNoteClearsExistingNote(t *testing.T) {
	env := setupRqcEnv(t)
	sessionID, entry, entryVersion := selectAndStartFirstEntry(t, env)

	state := completeEntry(t, env, sessionID, entry, entryVersion,
		fmt.Sprintf(`{"status":"completed","grade":"good","notes":"initial note","expected_entry_version":%d}`, entryVersion))
	entryVersion = int(rqcEntryVersion(t, state, rqcStr(t, entry, "id")))

	rec := env.req(t, env.teacherID, http.MethodPost,
		"/api/v1/sessions/"+sessionID+"/queue/entries/"+rqcStr(t, entry, "id")+"/grade",
		fmt.Sprintf(`{"grade":"excellent","notes":null,"expected_entry_version":%d}`, entryVersion), "")
	if rec.Code != http.StatusOK {
		t.Fatalf("clear-note correction: got %d want 200 body=%s", rec.Code, rec.Body.String())
	}
	updated := rqcDecode(t, rec)
	if rqcStr(t, updated, "grade") != "excellent" {
		t.Fatalf("corrected grade = %q, want excellent", rqcStr(t, updated, "grade"))
	}
	if updated["grade_notes"] != nil {
		t.Fatalf("corrected notes = %v, want cleared (null)", updated["grade_notes"])
	}
}

func TestRecitationQueueGrading_CorrectionWithoutGradeOrNotesReturns422(t *testing.T) {
	env := setupRqcEnv(t)
	sessionID, entry, entryVersion := selectAndStartFirstEntry(t, env)

	state := completeEntry(t, env, sessionID, entry, entryVersion,
		fmt.Sprintf(`{"status":"completed","grade":"good","expected_entry_version":%d}`, entryVersion))
	entryVersion = int(rqcEntryVersion(t, state, rqcStr(t, entry, "id")))

	rec := env.req(t, env.teacherID, http.MethodPost,
		"/api/v1/sessions/"+sessionID+"/queue/entries/"+rqcStr(t, entry, "id")+"/grade",
		fmt.Sprintf(`{"expected_entry_version":%d}`, entryVersion), "")
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("empty correction: got %d want 422 body=%s", rec.Code, rec.Body.String())
	}
	envl := rqcDecodeError(t, rec)
	if envl.Error.Code == "" {
		t.Fatal("422 must carry a standard error code")
	}
}
