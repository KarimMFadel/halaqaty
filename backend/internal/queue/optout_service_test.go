//go:build integration

package queue

// T039 — red service tests for the F-003 US2 opt-out service and the
// late-join append hook. This file is the TDD spec for T042
// (backend/internal/queue/optout_service.go + the late-join append hook in
// round_service.go wired through SessionObserver.OnParticipantJoined).
//
// Pinned API surface for T042:
//
//	type OptOutResult struct {
//	    Request OptOutRequest // always present: pending/approved request row
//	    Entry   QueueEntry    // the student's entry after the operation
//	}
//
//	type OptOutAuditSink interface {
//	    OptOutRequested(ctx context.Context, actorID, sessionID, entryID string)
//	    OptOutDecided(ctx context.Context, actorID, sessionID, requestID, decision string)
//	}
//
//	func NewOptOutService(repo *Repository) *OptOutService
//	func NewOptOutServiceWithAudit(repo *Repository, audit OptOutAuditSink) *OptOutService
//
//	func (s *OptOutService) Request(ctx context.Context, sessionID, studentID string) (OptOutResult, error)
//	func (s *OptOutService) Decide(ctx context.Context, sessionID, requestID, managerID string, decision OptOutRequestStatus, expectedEntryVersion int64) (OptOutResult, error)
//
// Request resolves the caller's own entry in the session's active round (the
// REST surface carries no entry identifier), so student-self is structural.
// Decide reuses the closed OptOutRequestStatus enum (approved/declined only).
// The service performs no media operation and takes no media dependency
// (ADR-020); late-join behavior is pinned through the existing
// SessionObserver.OnParticipantJoined production hook.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// ooFixture is one isolated schema with an active session and an active
// round holding one entry per listed student, under the given opt-out policy.
type ooFixture struct {
	repo       *Repository
	teacher    string
	supervisor string
	session    string
	round      Round
}

func newOptOutFixture(t *testing.T, optOut OptOutPolicy, studentCount int) *ooFixture {
	t.Helper()
	repo := newQueueRepository(t)
	ctx := context.Background()
	base := time.Now().UTC().Add(-time.Hour)
	teacher := qSeedUser(t, repo, "oo-teacher")
	supervisor := qSeedUser(t, repo, "oo-supervisor")
	circle := qSeedCircle(t, repo, teacher)
	qSeedMember(t, repo, circle, teacher, "teacher", base)
	qSeedMember(t, repo, circle, supervisor, "supervisor", base.Add(time.Minute))
	students := make([]string, 0, studentCount)
	for i := 0; i < studentCount; i++ {
		student := qSeedUser(t, repo, fmt.Sprintf("oo-student-%d", i))
		qSeedMember(t, repo, circle, student, "student", base.Add(time.Duration(i+2)*time.Minute))
		students = append(students, student)
	}
	session := qInsertSession(t, repo, circle, teacher, string(GradeVisibilityManagersAndStudent))
	if _, err := repo.pool.Exec(ctx, `
		UPDATE sessions
		SET status = 'active', actual_start = NOW(), queue_opt_out_policy = $2
		WHERE id = $1::uuid
	`, session, optOut); err != nil {
		t.Fatalf("activate opt-out fixture session: %v", err)
	}
	round := qCreateRound(t, repo, session, teacher, "active", students)
	return &ooFixture{repo: repo, teacher: teacher, supervisor: supervisor, session: session, round: round}
}

func (f *ooFixture) entryFor(t *testing.T, studentID string) QueueEntry {
	t.Helper()
	state, err := f.repo.LoadQueueState(context.Background(), f.round.ID, Viewer{UserID: f.teacher, IsManager: true})
	if err != nil {
		t.Fatalf("load opt-out fixture state: %v", err)
	}
	for _, entry := range state.Entries {
		if entry.StudentID == studentID {
			return entry
		}
	}
	t.Fatalf("entry for student %s not found", studentID)
	return QueueEntry{}
}

func (f *ooFixture) mustErrCode(t *testing.T, err error, code QueueErrorCode) {
	t.Helper()
	var qerr *QueueError
	if !errors.As(err, &qerr) {
		t.Fatalf("expected *QueueError, got %T: %v", err, err)
	}
	if qerr.Code != code {
		t.Fatalf("error code = %s, want %s (%v)", qerr.Code, code, err)
	}
}

func (f *ooFixture) pendingRequestCount(t *testing.T, entryID string) int {
	t.Helper()
	var count int
	if err := f.repo.pool.QueryRow(context.Background(), `
		SELECT COUNT(*) FROM queue_opt_out_requests
		WHERE queue_entry_id = $1::uuid AND status = 'pending'
	`, entryID).Scan(&count); err != nil {
		t.Fatalf("count pending opt-out requests: %v", err)
	}
	return count
}

func (f *ooFixture) requestByID(t *testing.T, requestID string) OptOutRequest {
	t.Helper()
	var request OptOutRequest
	err := f.repo.pool.QueryRow(context.Background(), `
		SELECT id::text, queue_entry_id::text, requested_by::text, status, decided_by::text, requested_at, decided_at
		FROM queue_opt_out_requests
		WHERE id = $1::uuid
	`, requestID).Scan(&request.ID, &request.QueueEntryID, &request.RequestedBy, &request.Status, &request.DecidedBy, &request.RequestedAt, &request.DecidedAt)
	if err != nil {
		t.Fatalf("load opt-out request %s: %v", requestID, err)
	}
	return request
}

func (f *ooFixture) progressCount(t *testing.T) int {
	t.Helper()
	var count int
	if err := f.repo.pool.QueryRow(context.Background(), `
		SELECT COUNT(*) FROM memorization_progress WHERE session_id = $1::uuid
	`, f.session).Scan(&count); err != nil {
		t.Fatalf("count memorization progress rows: %v", err)
	}
	return count
}

// outboxEventTypes returns every committed outbox event type for the session.
func (f *ooFixture) outboxEventTypes(t *testing.T) []string {
	t.Helper()
	rows, err := f.repo.pool.Query(context.Background(), `
		SELECT event_type FROM queue_event_outbox WHERE session_id = $1::uuid
	`, f.session)
	if err != nil {
		t.Fatalf("list opt-out outbox events: %v", err)
	}
	defer rows.Close()
	var types []string
	for rows.Next() {
		var eventType string
		if err := rows.Scan(&eventType); err != nil {
			t.Fatalf("scan outbox event type: %v", err)
		}
		types = append(types, eventType)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate outbox event types: %v", err)
	}
	return types
}

// assertNoMediaEvents fails when any committed outbox row names a media-ish
// event; queue code must never emit audio/convergence intents (ADR-020).
func (f *ooFixture) assertNoMediaEvents(t *testing.T) {
	t.Helper()
	for _, eventType := range f.outboxEventTypes(t) {
		for _, forbidden := range []string{"audio", "media", "revoke", "grant", "convergence"} {
			if strings.Contains(eventType, forbidden) {
				t.Fatalf("queue flow emitted media event %q", eventType)
			}
		}
	}
}

// assertRedactedOutboxMetadata walks every outbox row's metadata for the
// session and fails on any grade/note/name/media key (plan §Privacy map).
func (f *ooFixture) assertRedactedOutboxMetadata(t *testing.T) {
	t.Helper()
	rows, err := f.repo.pool.Query(context.Background(), `
		SELECT event_metadata FROM queue_event_outbox WHERE session_id = $1::uuid
	`, f.session)
	if err != nil {
		t.Fatalf("load outbox metadata: %v", err)
	}
	defer rows.Close()
	for rows.Next() {
		var raw []byte
		if err := rows.Scan(&raw); err != nil {
			t.Fatalf("scan outbox metadata: %v", err)
		}
		var value map[string]any
		if err := json.Unmarshal(raw, &value); err != nil {
			t.Fatalf("decode outbox metadata %s: %v", raw, err)
		}
		for key := range value {
			switch key {
			case "grade", "notes", "note", "name", "student_name", "media", "credential", "room", "url":
				t.Fatalf("outbox metadata leaked prohibited key %q: %s", key, raw)
			}
		}
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate outbox metadata: %v", err)
	}
}

type optOutAuditCapture struct {
	requested      []string // entryID per request event
	decided        []string // requestID=decision per decision event
	decisionActors []string // actor per decision event
}

func (c *optOutAuditCapture) OptOutRequested(_ context.Context, actorID, sessionID, entryID string) {
	c.requested = append(c.requested, entryID)
}

func (c *optOutAuditCapture) OptOutDecided(_ context.Context, actorID, sessionID, requestID, decision string) {
	c.decided = append(c.decided, requestID+"="+decision)
	c.decisionActors = append(c.decisionActors, actorID)
}

// TestOptOutServiceLateJoinAppendsOnceAtEndUnderBothPolicies pins FR-003:
// a committed F-005 join fact appends exactly one durable waiting position at
// the end of the active round under present_at_activation, and under
// all_active_students fires only for members added after activation.
func TestOptOutServiceLateJoinAppendsOnceAtEndUnderBothPolicies(t *testing.T) {
	t.Run("present_at_activation appends once idempotently at the end", func(t *testing.T) {
		f := newOptOutFixture(t, OptOutPolicyApprovalRequired, 2)
		ctx := context.Background()
		var circleID string
		if err := f.repo.pool.QueryRow(ctx, `SELECT circle_id::text FROM sessions WHERE id = $1::uuid`, f.session).Scan(&circleID); err != nil {
			t.Fatalf("load fixture circle: %v", err)
		}
		late := qSeedUser(t, f.repo, "oo-late-present")
		qSeedMember(t, f.repo, circleID, late, "student", time.Now().UTC())
		observer := NewSessionObserver(NewRoundService(f.repo), nil)

		var beforeVersion int64
		var beforeCount int
		if err := f.repo.pool.QueryRow(ctx, `
			SELECT version, (SELECT COUNT(*) FROM recitation_queue_entries WHERE queue_id = $1::uuid)
			FROM recitation_queue WHERE id = $1::uuid
		`, f.round.ID).Scan(&beforeVersion, &beforeCount); err != nil {
			t.Fatalf("snapshot round before late join: %v", err)
		}

		if err := observer.OnParticipantJoined(ctx, f.session, late); err != nil {
			t.Fatalf("first late join append: %v", err)
		}
		// Duplicate delivery of the same committed join fact must converge.
		if err := observer.OnParticipantJoined(ctx, f.session, late); err != nil {
			t.Fatalf("duplicate late join append: %v", err)
		}

		state, err := f.repo.LoadQueueState(ctx, f.round.ID, Viewer{UserID: f.teacher, IsManager: true})
		if err != nil {
			t.Fatalf("load state after late join: %v", err)
		}
		if len(state.Entries) != beforeCount+1 {
			t.Fatalf("entries after late join = %d, want %d (exactly one appended)", len(state.Entries), beforeCount+1)
		}
		appended := state.Entries[len(state.Entries)-1]
		if appended.StudentID != late || appended.Position != beforeCount+1 || appended.Status != EntryStatusWaiting {
			t.Fatalf("appended late entry = %+v, want student %s waiting at position %d", appended, late, beforeCount+1)
		}
		if state.Round.Version != beforeVersion+1 {
			t.Fatalf("round version after idempotent late join = %d, want exactly one bump from %d", state.Round.Version, beforeVersion)
		}
		f.assertNoMediaEvents(t)
	})

	t.Run("all_active_students skips members already populated at activation", func(t *testing.T) {
		f := newOptOutFixture(t, OptOutPolicyApprovalRequired, 2)
		ctx := context.Background()
		var circleID string
		if err := f.repo.pool.QueryRow(ctx, `SELECT circle_id::text FROM sessions WHERE id = $1::uuid`, f.session).Scan(&circleID); err != nil {
			t.Fatalf("load fixture circle: %v", err)
		}
		if _, err := f.repo.pool.Exec(ctx, `UPDATE sessions SET queue_population_policy = $2 WHERE id = $1::uuid`, f.session, PopulationPolicyAllActiveStudents); err != nil {
			t.Fatalf("set population policy: %v", err)
		}
		observer := NewSessionObserver(NewRoundService(f.repo), nil)
		early := f.entryFor(t, qFirstStudent(t, f))

		// The early member was active at activation and already materialized;
		// their join fact must not duplicate the existing entry.
		var beforeVersion int64
		if err := f.repo.pool.QueryRow(ctx, `SELECT version FROM recitation_queue WHERE id = $1::uuid`, f.round.ID).Scan(&beforeVersion); err != nil {
			t.Fatalf("load round version: %v", err)
		}
		if err := observer.OnParticipantJoined(ctx, f.session, early.StudentID); err != nil {
			t.Fatalf("already-populated member join: %v", err)
		}
		state, err := f.repo.LoadQueueState(ctx, f.round.ID, Viewer{UserID: f.teacher, IsManager: true})
		if err != nil {
			t.Fatalf("load state after populated-member join: %v", err)
		}
		if len(state.Entries) != 2 || state.Round.Version != beforeVersion {
			t.Fatalf("all_active_students duplicated pre-populated member: entries=%d version=%d (want 2, %d)", len(state.Entries), state.Round.Version, beforeVersion)
		}

		// A member added to the circle after activation is appended once.
		addedLater := qSeedUser(t, f.repo, "oo-late-all-active")
		qSeedMember(t, f.repo, circleID, addedLater, "student", time.Now().UTC().Add(time.Minute))
		if err := observer.OnParticipantJoined(ctx, f.session, addedLater); err != nil {
			t.Fatalf("post-activation member join: %v", err)
		}
		state, err = f.repo.LoadQueueState(ctx, f.round.ID, Viewer{UserID: f.teacher, IsManager: true})
		if err != nil {
			t.Fatalf("load state after post-activation join: %v", err)
		}
		if len(state.Entries) != 3 || state.Entries[2].StudentID != addedLater {
			t.Fatalf("post-activation member not appended exactly once at the end: %+v", state.Entries)
		}
		f.assertNoMediaEvents(t)
	})
}

func qFirstStudent(t *testing.T, f *ooFixture) string {
	t.Helper()
	state, err := f.repo.LoadQueueState(context.Background(), f.round.ID, Viewer{UserID: f.teacher, IsManager: true})
	if err != nil {
		t.Fatalf("load fixture state: %v", err)
	}
	if len(state.Entries) == 0 {
		t.Fatal("fixture round has no entries")
	}
	return state.Entries[0].StudentID
}

// TestOptOutServiceRequestCreatesSinglePendingRequestPerEntry pins the
// one-pending-per-entry partial unique index and service-level idempotency.
func TestOptOutServiceRequestCreatesSinglePendingRequestPerEntry(t *testing.T) {
	f := newOptOutFixture(t, OptOutPolicyApprovalRequired, 2)
	ctx := context.Background()
	service := NewOptOutService(f.repo)
	student := qFirstStudent(t, f)
	entry := f.entryFor(t, student)

	first, err := service.Request(ctx, f.session, student)
	if err != nil {
		t.Fatalf("first opt-out request: %v", err)
	}
	if first.Request.Status != OptOutRequestStatusPending || first.Request.ID == "" {
		t.Fatalf("first request = %+v, want a pending request row", first.Request)
	}
	if first.Request.RequestedBy != student || first.Request.QueueEntryID != entry.ID {
		t.Fatalf("request attribution = %+v, want requester %s on entry %s", first.Request, student, entry.ID)
	}
	if first.Entry.Status != EntryStatusWaiting {
		t.Fatalf("entry after pending request = %s, want waiting", first.Entry.Status)
	}

	second, err := service.Request(ctx, f.session, student)
	if err != nil {
		t.Fatalf("duplicate opt-out request: %v", err)
	}
	if second.Request.ID != first.Request.ID {
		t.Fatalf("duplicate request id = %s, want the committed pending request %s", second.Request.ID, first.Request.ID)
	}
	if got := f.pendingRequestCount(t, entry.ID); got != 1 {
		t.Fatalf("pending requests after duplicate = %d, want exactly 1", got)
	}
	f.assertNoMediaEvents(t)
	f.assertRedactedOutboxMetadata(t)
}

func TestOptOutServicePendingEventUsesCurrentRoundVersion(t *testing.T) {
	f := newOptOutFixture(t, OptOutPolicyApprovalRequired, 1)
	ctx := context.Background()
	if err := f.repo.WithTx(ctx, func(tx *Tx) error {
		if err := tx.BumpRoundVersion(ctx, f.round.ID); err != nil {
			return err
		}
		return tx.BumpRoundVersion(ctx, f.round.ID)
	}); err != nil {
		t.Fatalf("advance round version: %v", err)
	}
	student := qFirstStudent(t, f)
	if _, err := NewOptOutService(f.repo).Request(ctx, f.session, student); err != nil {
		t.Fatalf("request opt-out: %v", err)
	}

	round, err := f.repo.Round(ctx, f.round.ID)
	if err != nil {
		t.Fatalf("load current round: %v", err)
	}
	var eventVersion int64
	if err := f.repo.pool.QueryRow(ctx, `
		SELECT round_version FROM queue_event_outbox
		WHERE session_id = $1::uuid AND event_type = 'queue.opt_out_requested'
	`, f.session).Scan(&eventVersion); err != nil {
		t.Fatalf("load opt-out event version: %v", err)
	}
	if eventVersion != round.Version {
		t.Fatalf("opt-out event version = %d, want current round version %d", eventVersion, round.Version)
	}
}

// TestOptOutServiceRequestRejectsMissingAndTerminalEntries pins the request
// guards: no resolvable own entry and terminal entries never create requests.
func TestOptOutServiceRequestRejectsMissingAndTerminalEntries(t *testing.T) {
	f := newOptOutFixture(t, OptOutPolicyApprovalRequired, 2)
	ctx := context.Background()
	service := NewOptOutService(f.repo)
	outsider := qSeedUser(t, f.repo, "oo-no-entry")

	if _, err := service.Request(ctx, f.session, outsider); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("request without an entry error = %v, want pgx.ErrNoRows (404)", err)
	}

	student := qFirstStudent(t, f)
	entry := f.entryFor(t, student)
	err := f.repo.WithTx(ctx, func(tx *Tx) error {
		locked, err := tx.LockEntry(ctx, entry.ID)
		if err != nil {
			return err
		}
		_, err = tx.TransitionEntry(ctx, locked.ID, locked.Status, locked.Version, EntryStatusSkipped, nil, nil, &f.teacher)
		return err
	})
	if err != nil {
		t.Fatalf("skip entry for terminal request case: %v", err)
	}
	_, err = service.Request(ctx, f.session, student)
	f.mustErrCode(t, err, QueueErrorCodeEntryTerminal)
	if got := f.pendingRequestCount(t, entry.ID); got != 0 {
		t.Fatalf("pending requests after terminal rejection = %d, want 0", got)
	}
}

// TestOptOutServiceApproveTransitionsToOptedOutWithAuditAndNoProgress pins
// approve → entry opted_out, durable attribution, audit events, no progress,
// and selection clearing when the opted-out entry was the current selection.
func TestOptOutServiceApproveTransitionsToOptedOutWithAuditAndNoProgress(t *testing.T) {
	f := newOptOutFixture(t, OptOutPolicyApprovalRequired, 2)
	ctx := context.Background()
	audit := &optOutAuditCapture{}
	service := NewOptOutServiceWithAudit(f.repo, audit)
	student := qFirstStudent(t, f)

	// Make the student's entry the current selection so approve must clear it.
	selected, err := NewTurnService(f.repo).Advance(ctx, f.round.ID, f.round.Version)
	if err != nil {
		t.Fatalf("advance to select first entry: %v", err)
	}
	if selected.SelectedEntryID == nil {
		t.Fatal("advance left no selection")
	}

	requested, err := service.Request(ctx, f.session, student)
	if err != nil {
		t.Fatalf("opt-out request: %v", err)
	}
	entryBefore := f.entryFor(t, student)

	decided, err := service.Decide(ctx, f.session, requested.Request.ID, f.teacher, OptOutRequestStatusApproved, entryBefore.Version)
	if err != nil {
		t.Fatalf("approve opt-out: %v", err)
	}
	if decided.Entry.Status != EntryStatusOptedOut {
		t.Fatalf("entry after approve = %s, want opted_out", decided.Entry.Status)
	}
	if decided.Entry.ResolvedBy == nil || *decided.Entry.ResolvedBy != f.teacher {
		t.Fatalf("approve attribution = %v, want manager %s", decided.Entry.ResolvedBy, f.teacher)
	}

	persisted := f.requestByID(t, requested.Request.ID)
	if persisted.Status != OptOutRequestStatusApproved || persisted.DecidedBy == nil || *persisted.DecidedBy != f.teacher || persisted.DecidedAt == nil {
		t.Fatalf("approved request row = %+v, want decided by %s with decided_at", persisted, f.teacher)
	}

	state, err := f.repo.LoadQueueState(ctx, f.round.ID, Viewer{UserID: f.teacher, IsManager: true})
	if err != nil {
		t.Fatalf("load state after approve: %v", err)
	}
	if state.Round.SelectedEntryID != nil {
		t.Fatalf("selection after approved opt-out = %v, want cleared", state.Round.SelectedEntryID)
	}
	if state.Round.Version <= selected.Version {
		t.Fatalf("round version = %d must advance past %d on the terminal transition", state.Round.Version, selected.Version)
	}

	if len(audit.requested) != 1 || len(audit.decided) != 1 {
		t.Fatalf("audit events: requested=%d decided=%d, want 1 and 1", len(audit.requested), len(audit.decided))
	}
	if audit.decided[0] != requested.Request.ID+"="+string(OptOutRequestStatusApproved) {
		t.Fatalf("decision audit = %q, want request %s approved", audit.decided[0], requested.Request.ID)
	}
	if audit.decisionActors[0] != f.teacher {
		t.Fatalf("decision audit actor = %s, want manager %s", audit.decisionActors[0], f.teacher)
	}

	if got := f.progressCount(t); got != 0 {
		t.Fatalf("progress rows after approved opt-out = %d, want 0", got)
	}
	f.assertNoMediaEvents(t)
	f.assertRedactedOutboxMetadata(t)
}

func TestOptOutServiceRejectsDecisionForDifferentSession(t *testing.T) {
	f := newOptOutFixture(t, OptOutPolicyApprovalRequired, 1)
	ctx := context.Background()
	service := NewOptOutService(f.repo)
	student := qFirstStudent(t, f)
	requested, err := service.Request(ctx, f.session, student)
	if err != nil {
		t.Fatalf("opt-out request: %v", err)
	}
	entry := f.entryFor(t, student)

	_, err = service.Decide(ctx, uuid.NewString(), requested.Request.ID, f.teacher, OptOutRequestStatusApproved, entry.Version)
	f.mustErrCode(t, err, QueueErrorCodeInvalidTransition)
	if got := f.requestByID(t, requested.Request.ID); got.Status != OptOutRequestStatusPending {
		t.Fatalf("request after wrong-session decision = %s, want pending", got.Status)
	}
	if got := f.entryFor(t, student); got.Status != EntryStatusWaiting {
		t.Fatalf("entry after wrong-session decision = %s, want waiting", got.Status)
	}
}

// TestOptOutServiceDeclineTerminallyClosesRequestAndKeepsEntryWaiting pins
// CHK005: decline closes the request terminally; the entry remains waiting.
func TestOptOutServiceDeclineTerminallyClosesRequestAndKeepsEntryWaiting(t *testing.T) {
	f := newOptOutFixture(t, OptOutPolicyApprovalRequired, 2)
	ctx := context.Background()
	audit := &optOutAuditCapture{}
	service := NewOptOutServiceWithAudit(f.repo, audit)
	student := qFirstStudent(t, f)

	requested, err := service.Request(ctx, f.session, student)
	if err != nil {
		t.Fatalf("opt-out request: %v", err)
	}
	entry := f.entryFor(t, student)

	decided, err := service.Decide(ctx, f.session, requested.Request.ID, f.supervisor, OptOutRequestStatusDeclined, entry.Version)
	if err != nil {
		t.Fatalf("decline opt-out: %v", err)
	}
	if decided.Request.Status != OptOutRequestStatusDeclined {
		t.Fatalf("request after decline = %s, want declined", decided.Request.Status)
	}
	if decided.Entry.Status != EntryStatusWaiting || decided.Entry.ID != entry.ID {
		t.Fatalf("entry after decline = %+v, want still waiting", decided.Entry)
	}

	persisted := f.requestByID(t, requested.Request.ID)
	if persisted.Status != OptOutRequestStatusDeclined || persisted.DecidedBy == nil || *persisted.DecidedBy != f.supervisor || persisted.DecidedAt == nil {
		t.Fatalf("declined request row = %+v, want terminally closed by %s", persisted, f.supervisor)
	}

	// Deciding an already-decided request is a clean conflict with no mutation.
	_, err = service.Decide(ctx, f.session, requested.Request.ID, f.teacher, OptOutRequestStatusApproved, entry.Version)
	f.mustErrCode(t, err, QueueErrorCodeInvalidTransition)
	after := f.requestByID(t, requested.Request.ID)
	if after.Status != OptOutRequestStatusDeclined || after.DecidedBy == nil || *after.DecidedBy != f.supervisor {
		t.Fatalf("re-decision mutated the closed request: %+v", after)
	}
	if got := f.progressCount(t); got != 0 {
		t.Fatalf("progress rows after declined opt-out = %d, want 0", got)
	}
	f.assertNoMediaEvents(t)
}

// TestOptOutServiceAutoApproveTransitionsDirectlyAndIdempotently pins
// auto_approve: direct entry transition, no pending row ever, stable replay.
func TestOptOutServiceAutoApproveTransitionsDirectlyAndIdempotently(t *testing.T) {
	f := newOptOutFixture(t, OptOutPolicyAutoApprove, 2)
	ctx := context.Background()
	audit := &optOutAuditCapture{}
	service := NewOptOutServiceWithAudit(f.repo, audit)
	student := qFirstStudent(t, f)
	entry := f.entryFor(t, student)

	first, err := service.Request(ctx, f.session, student)
	if err != nil {
		t.Fatalf("auto-approve opt-out request: %v", err)
	}
	if first.Entry.Status != EntryStatusOptedOut {
		t.Fatalf("entry after auto-approve = %s, want opted_out", first.Entry.Status)
	}
	if first.Request.ID == "" || first.Request.Status != OptOutRequestStatusApproved {
		t.Fatalf("auto-approve request row = %+v, want an approved request for the response", first.Request)
	}
	if got := f.pendingRequestCount(t, entry.ID); got != 0 {
		t.Fatalf("pending requests under auto_approve = %d, want 0", got)
	}

	second, err := service.Request(ctx, f.session, student)
	if err != nil {
		t.Fatalf("auto-approve replay: %v", err)
	}
	if second.Request.ID != first.Request.ID || second.Entry.Status != EntryStatusOptedOut {
		t.Fatalf("auto-approve replay = request %s entry %s, want the committed request %s with opted_out", second.Request.ID, second.Entry.Status, first.Request.ID)
	}
	var requestRows int
	if err := f.repo.pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM queue_opt_out_requests WHERE queue_entry_id = $1::uuid
	`, entry.ID).Scan(&requestRows); err != nil {
		t.Fatalf("count auto-approve request rows: %v", err)
	}
	if requestRows != 1 {
		t.Fatalf("auto-approve request rows = %d, want exactly 1", requestRows)
	}
	if len(audit.decided) != 1 {
		t.Fatalf("auto-approve decision audit events = %d, want 1", len(audit.decided))
	}
	if got := f.progressCount(t); got != 0 {
		t.Fatalf("progress rows after auto-approved opt-out = %d, want 0", got)
	}
	f.assertNoMediaEvents(t)
	f.assertRedactedOutboxMetadata(t)
}

// TestOptOutServiceDecideStaleVersionConflicts pins the optimistic entry
// version guard on decisions.
func TestOptOutServiceDecideStaleVersionConflicts(t *testing.T) {
	f := newOptOutFixture(t, OptOutPolicyApprovalRequired, 2)
	ctx := context.Background()
	service := NewOptOutService(f.repo)
	student := qFirstStudent(t, f)

	requested, err := service.Request(ctx, f.session, student)
	if err != nil {
		t.Fatalf("opt-out request: %v", err)
	}
	entry := f.entryFor(t, student)

	_, err = service.Decide(ctx, f.session, requested.Request.ID, f.teacher, OptOutRequestStatusApproved, entry.Version+100)
	f.mustErrCode(t, err, QueueErrorCodeStaleVersion)
	if got := f.pendingRequestCount(t, entry.ID); got != 1 {
		t.Fatalf("pending requests after stale decision = %d, want still 1", got)
	}
	if after := f.entryFor(t, student); after.Status != EntryStatusWaiting {
		t.Fatalf("stale decision mutated entry to %s", after.Status)
	}
}

// TestOptOutServicePendingAtFinalizationBecomesNonActionable pins the clean
// 409 when a request outlives its round: round_finalized, no state mutation.
func TestOptOutServicePendingAtFinalizationBecomesNonActionable(t *testing.T) {
	f := newOptOutFixture(t, OptOutPolicyApprovalRequired, 2)
	ctx := context.Background()
	service := NewOptOutService(f.repo)
	student := qFirstStudent(t, f)

	requested, err := service.Request(ctx, f.session, student)
	if err != nil {
		t.Fatalf("opt-out request: %v", err)
	}
	entry := f.entryFor(t, student)

	// Finalize with preserve_last_state so the entry itself stays waiting and
	// the round-finalized guard is the only possible rejection reason.
	if _, err := f.repo.pool.Exec(ctx, `UPDATE sessions SET queue_finalization_policy = $2 WHERE id = $1::uuid`, f.session, FinalizationPolicyPreserveLastState); err != nil {
		t.Fatalf("set finalization policy: %v", err)
	}
	err = f.repo.WithTx(ctx, func(tx *Tx) error {
		round, err := tx.LockRound(ctx, f.round.ID)
		if err != nil {
			return err
		}
		_, err = tx.FinalizeRound(ctx, round.ID, round.Version, FinalizationPolicyPreserveLastState, f.teacher)
		return err
	})
	if err != nil {
		t.Fatalf("finalize round: %v", err)
	}

	_, err = service.Decide(ctx, f.session, requested.Request.ID, f.teacher, OptOutRequestStatusApproved, entry.Version)
	f.mustErrCode(t, err, QueueErrorCodeRoundFinalized)

	persisted := f.requestByID(t, requested.Request.ID)
	if persisted.Status != OptOutRequestStatusPending || persisted.DecidedBy != nil {
		t.Fatalf("rejected post-finalization decision mutated the request: %+v", persisted)
	}
	if after := f.entryFor(t, student); after.Status != EntryStatusWaiting {
		t.Fatalf("rejected post-finalization decision mutated the entry: %+v", after)
	}
	if got := f.progressCount(t); got != 0 {
		t.Fatalf("progress rows after rejected decision = %d, want 0", got)
	}
}
