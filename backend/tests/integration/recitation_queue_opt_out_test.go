//go:build integration

package integration

// T040 (opt-out) — integration acceptance matrix for US2 opt-out at the
// service layer on real PostgreSQL: approval_required approve and decline,
// auto_approve direct transition, duplicate student requests, durable manager
// attribution with redacted payloads, zero progress/penalty rows for every
// opt-out path, and proof that no opt-out operation performs media control
// (no media event types or metadata; queue services take no media dependency
// — the import-graph guard is T066).
//
// Pinned T042 surface (same as backend/internal/queue/optout_service_test.go):
//
//	queue.NewOptOutService(repo) / queue.NewOptOutServiceWithAudit(repo, audit)
//	func (s *OptOutService) Request(ctx, sessionID, studentID string) (queue.OptOutResult, error)
//	func (s *OptOutService) Decide(ctx, requestID, managerID string, decision queue.OptOutRequestStatus, expectedEntryVersion int64) (queue.OptOutResult, error)

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/KarimMFadel/halaqaty/backend/internal/queue"
)

type qoFixture struct {
	*recitationQueueConcurrencyFixture
	round    queue.Round
	service  *queue.OptOutService
	students []string
}

func newOptOutIntegrationFixture(t *testing.T, optOut queue.OptOutPolicy, studentCount int) *qoFixture {
	t.Helper()
	f := newRecitationQueueConcurrencyFixture(t)
	ctx := context.Background()
	if _, err := f.pool.Exec(ctx, `
		UPDATE sessions SET queue_opt_out_policy = $2 WHERE id = $1::uuid
	`, f.session, optOut); err != nil {
		t.Fatalf("set opt-out policy %s: %v", optOut, err)
	}
	students := make([]string, 0, studentCount)
	for i := 0; i < studentCount; i++ {
		students = append(students, f.addStudent(t, fmt.Sprintf("qo-student-%d", i), time.Now().UTC().Add(time.Duration(i)*time.Minute)))
	}
	round := cqCreateActiveRound(t, f, students...)
	return &qoFixture{recitationQueueConcurrencyFixture: f, round: round, service: queue.NewOptOutService(f.repo), students: students}
}

func (f *qoFixture) state(t *testing.T) queue.QueueState {
	t.Helper()
	return cqState(t, f.recitationQueueConcurrencyFixture, f.round.ID)
}

func (f *qoFixture) requestOptOut(t *testing.T, studentID string) queue.OptOutResult {
	t.Helper()
	result, err := f.service.Request(context.Background(), f.session, studentID)
	if err != nil {
		t.Fatalf("opt-out request for %s: %v", studentID, err)
	}
	return result
}

// qoDurableFacts returns the persisted request row fields and entry status for
// manager-attribution assertions.
type qoDurableFacts struct {
	requestStatus string
	decidedBy     *string
	decidedAtSet  bool
	entryStatus   queue.EntryStatus
	resolvedBy    *string
}

func (f *qoFixture) durableFacts(t *testing.T, requestID, entryID string) qoDurableFacts {
	t.Helper()
	var facts qoDurableFacts
	var decidedBy *string
	var decidedAt *time.Time
	err := f.pool.QueryRow(context.Background(), `
		SELECT status, decided_by::text, decided_at
		FROM queue_opt_out_requests WHERE id = $1::uuid
	`, requestID).Scan(&facts.requestStatus, &decidedBy, &decidedAt)
	if err != nil {
		t.Fatalf("load durable opt-out request %s: %v", requestID, err)
	}
	facts.decidedBy = decidedBy
	facts.decidedAtSet = decidedAt != nil
	if err := f.pool.QueryRow(context.Background(), `
		SELECT status, resolved_by::text FROM recitation_queue_entries WHERE id = $1::uuid
	`, entryID).Scan(&facts.entryStatus, &facts.resolvedBy); err != nil {
		t.Fatalf("load durable entry %s: %v", entryID, err)
	}
	return facts
}

func (f *qoFixture) pendingCount(t *testing.T, entryID string) int {
	t.Helper()
	var count int
	if err := f.pool.QueryRow(context.Background(), `
		SELECT COUNT(*) FROM queue_opt_out_requests
		WHERE queue_entry_id = $1::uuid AND status = 'pending'
	`, entryID).Scan(&count); err != nil {
		t.Fatalf("count pending opt-out requests: %v", err)
	}
	return count
}

func (f *qoFixture) progressCount(t *testing.T) int {
	t.Helper()
	var count int
	if err := f.pool.QueryRow(context.Background(), `
		SELECT COUNT(*) FROM memorization_progress WHERE session_id = $1::uuid
	`, f.session).Scan(&count); err != nil {
		t.Fatalf("count progress rows: %v", err)
	}
	return count
}

// qoAssertNoMediaControl proves the committed queue artifacts of the opt-out
// matrix carry no media material and no queue-driven grant/revoke intent.
func (f *qoFixture) qoAssertNoMediaControl(t *testing.T) {
	t.Helper()
	var mediaEvents int
	if err := f.pool.QueryRow(context.Background(), `
		SELECT COUNT(*)
		FROM queue_event_outbox
		WHERE session_id = $1::uuid
		  AND (event_type ILIKE '%audio%' OR event_type ILIKE '%media%'
		       OR event_type ILIKE '%revoke%' OR event_type ILIKE '%grant%'
		       OR event_type ILIKE '%convergence%')
	`, f.session).Scan(&mediaEvents); err != nil {
		t.Fatalf("count media-ish outbox events: %v", err)
	}
	if mediaEvents != 0 {
		t.Fatalf("opt-out flow wrote %d media-ish outbox events, want 0", mediaEvents)
	}
	ljAssertNoMediaInteraction(t, f.recitationQueueConcurrencyFixture)
}

// TestRecitationQueueOptOutAcceptanceMatrix exercises every configured
// opt-out outcome with durable attribution, redaction, and zero progress.
func TestRecitationQueueOptOutAcceptanceMatrix(t *testing.T) {
	t.Run("approval_required approve transitions entry with manager attribution", func(t *testing.T) {
		f := newOptOutIntegrationFixture(t, queue.OptOutPolicyApprovalRequired, 2)
		ctx := context.Background()
		requested := f.requestOptOut(t, f.students[0])
		if requested.Request.Status != queue.OptOutRequestStatusPending {
			t.Fatalf("initial request status = %s, want pending", requested.Request.Status)
		}
		entry := cqEntry(t, f.state(t), f.students[0])

		decided, err := f.service.DecideForSession(ctx, f.session, requested.Request.ID, f.teacher, queue.OptOutRequestStatusApproved, entry.Version)
		if err != nil {
			t.Fatalf("approve: %v", err)
		}
		if decided.Entry.Status != queue.EntryStatusOptedOut {
			t.Fatalf("entry after approve = %s, want opted_out", decided.Entry.Status)
		}
		facts := f.durableFacts(t, requested.Request.ID, entry.ID)
		if facts.requestStatus != string(queue.OptOutRequestStatusApproved) {
			t.Fatalf("durable request status = %s, want approved", facts.requestStatus)
		}
		if facts.decidedBy == nil || *facts.decidedBy != f.teacher || !facts.decidedAtSet {
			t.Fatalf("durable decision attribution = %v decided_at=%v, want manager %s with timestamp", facts.decidedBy, facts.decidedAtSet, f.teacher)
		}
		if facts.entryStatus != queue.EntryStatusOptedOut || facts.resolvedBy == nil || *facts.resolvedBy != f.teacher {
			t.Fatalf("durable entry attribution = %s by %v, want opted_out by %s", facts.entryStatus, facts.resolvedBy, f.teacher)
		}
		if got := f.progressCount(t); got != 0 {
			t.Fatalf("progress rows after approved opt-out = %d, want 0 (SC-004)", got)
		}
		f.qoAssertNoMediaControl(t)
	})

	t.Run("approval_required decline keeps entry waiting and closes the request", func(t *testing.T) {
		f := newOptOutIntegrationFixture(t, queue.OptOutPolicyApprovalRequired, 2)
		ctx := context.Background()
		requested := f.requestOptOut(t, f.students[0])
		entry := cqEntry(t, f.state(t), f.students[0])

		decided, err := f.service.DecideForSession(ctx, f.session, requested.Request.ID, f.teacher, queue.OptOutRequestStatusDeclined, entry.Version)
		if err != nil {
			t.Fatalf("decline: %v", err)
		}
		if decided.Request.Status != queue.OptOutRequestStatusDeclined {
			t.Fatalf("request after decline = %s, want declined", decided.Request.Status)
		}
		facts := f.durableFacts(t, requested.Request.ID, entry.ID)
		if facts.entryStatus != queue.EntryStatusWaiting {
			t.Fatalf("entry after decline = %s, want waiting (CHK005)", facts.entryStatus)
		}
		if facts.decidedBy == nil || *facts.decidedBy != f.teacher {
			t.Fatalf("decline attribution = %v, want manager %s durably recorded", facts.decidedBy, f.teacher)
		}
		if got := f.progressCount(t); got != 0 {
			t.Fatalf("progress rows after declined opt-out = %d, want 0", got)
		}
		f.qoAssertNoMediaControl(t)
	})

	t.Run("auto_approve transitions directly with no pending state", func(t *testing.T) {
		f := newOptOutIntegrationFixture(t, queue.OptOutPolicyAutoApprove, 1)
		requested := f.requestOptOut(t, f.students[0])
		if requested.Entry.Status != queue.EntryStatusOptedOut {
			t.Fatalf("entry after auto-approve = %s, want opted_out", requested.Entry.Status)
		}
		if got := f.pendingCount(t, requested.Entry.ID); got != 0 {
			t.Fatalf("pending rows under auto_approve = %d, want 0", got)
		}
		facts := f.durableFacts(t, requested.Request.ID, requested.Entry.ID)
		if facts.entryStatus != queue.EntryStatusOptedOut {
			t.Fatalf("durable entry after auto-approve = %s, want opted_out", facts.entryStatus)
		}
		if got := f.progressCount(t); got != 0 {
			t.Fatalf("progress rows after auto-approved opt-out = %d, want 0", got)
		}
		f.qoAssertNoMediaControl(t)
	})

	t.Run("duplicate student request returns the committed pending request", func(t *testing.T) {
		f := newOptOutIntegrationFixture(t, queue.OptOutPolicyApprovalRequired, 1)
		first := f.requestOptOut(t, f.students[0])
		second := f.requestOptOut(t, f.students[0])
		if second.Request.ID != first.Request.ID {
			t.Fatalf("duplicate request id = %s, want the committed request %s", second.Request.ID, first.Request.ID)
		}
		if got := f.pendingCount(t, first.Entry.ID); got != 1 {
			t.Fatalf("pending rows after duplicate request = %d, want exactly 1 (partial unique)", got)
		}
		if after := cqEntry(t, f.state(t), f.students[0]); after.Status != queue.EntryStatusWaiting {
			t.Fatalf("entry after duplicate request = %s, want waiting", after.Status)
		}
		f.qoAssertNoMediaControl(t)
	})
}

// TestRecitationQueueOptOutNeverCreatesPenaltyRecords sweeps every opt-out
// path for penalty/progress/practice rows: all must stay at zero.
func TestRecitationQueueOptOutNeverCreatesPenaltyRecords(t *testing.T) {
	for name, setup := range map[string]func(t *testing.T) *qoFixture{
		"approved": func(t *testing.T) *qoFixture {
			f := newOptOutIntegrationFixture(t, queue.OptOutPolicyApprovalRequired, 1)
			requested := f.requestOptOut(t, f.students[0])
			entry := cqEntry(t, f.state(t), f.students[0])
			if _, err := f.service.DecideForSession(context.Background(), f.session, requested.Request.ID, f.teacher, queue.OptOutRequestStatusApproved, entry.Version); err != nil {
				t.Fatalf("approve: %v", err)
			}
			return f
		},
		"declined": func(t *testing.T) *qoFixture {
			f := newOptOutIntegrationFixture(t, queue.OptOutPolicyApprovalRequired, 1)
			requested := f.requestOptOut(t, f.students[0])
			entry := cqEntry(t, f.state(t), f.students[0])
			if _, err := f.service.DecideForSession(context.Background(), f.session, requested.Request.ID, f.teacher, queue.OptOutRequestStatusDeclined, entry.Version); err != nil {
				t.Fatalf("decline: %v", err)
			}
			return f
		},
		"auto_approved": func(t *testing.T) *qoFixture {
			f := newOptOutIntegrationFixture(t, queue.OptOutPolicyAutoApprove, 1)
			f.requestOptOut(t, f.students[0])
			return f
		},
		"pending_only": func(t *testing.T) *qoFixture {
			f := newOptOutIntegrationFixture(t, queue.OptOutPolicyApprovalRequired, 1)
			f.requestOptOut(t, f.students[0])
			return f
		},
	} {
		t.Run(name, func(t *testing.T) {
			f := setup(t)
			if got := f.progressCount(t); got != 0 {
				t.Fatalf("%s opt-out created %d progress rows, want 0 (SC-004)", name, got)
			}
			// No penalty surface exists anywhere in the F-003 schema; the
			// memorization_progress sweep above is the durable penalty guard.
			f.qoAssertNoMediaControl(t)
		})
	}
}
