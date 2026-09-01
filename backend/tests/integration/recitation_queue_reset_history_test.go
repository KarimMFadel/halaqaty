//go:build integration

package integration

import (
	"context"
	"errors"
	"testing"

	"github.com/KarimMFadel/halaqaty/backend/internal/queue"
)

// T051 — reset finalization and history preservation: both finalization
// policies, next-round numbering, immutable prior rounds with no reused
// positions/states, cleared selection, and an inert successor activation with
// no queue-driven media call.

func resetTestInput(sessionID, teacher string) queue.PrepareRoundInput {
	return queue.PrepareRoundInput{
		SessionID: sessionID, Type: queue.RoundTypeOldRevision, SurahID: 2,
		FromAyah: 1, ToAyah: 10, SurahAyahCount: 286, GradingRequired: true, CreatedBy: teacher,
	}
}

func entryByStudent(t *testing.T, f *completionFixture, roundID, studentID string) queue.QueueEntry {
	t.Helper()
	state, err := f.repo.LoadQueueState(context.Background(), roundID, queue.Viewer{UserID: f.teacher, IsManager: true})
	if err != nil {
		t.Fatalf("load queue state: %v", err)
	}
	for _, entry := range state.Entries {
		if entry.StudentID == studentID {
			return entry
		}
	}
	t.Fatalf("entry for student %s not found in round %s", studentID, roundID)
	return queue.QueueEntry{}
}

func setFinalizationPolicy(t *testing.T, f *completionFixture, policy queue.FinalizationPolicy) {
	t.Helper()
	if _, err := f.pool.Exec(context.Background(), `
		UPDATE sessions SET queue_finalization_policy = $1, queue_policy_version = queue_policy_version + 1
		WHERE id = $2::uuid
	`, policy, f.session); err != nil {
		t.Fatalf("set finalization policy %s: %v", policy, err)
	}
}

func TestRecitationQueueResetHistory_MarkUnfinishedSkippedPreservesHistory(t *testing.T) {
	f := newRecitationQueueCompletionFixture(t, true)
	ctx := context.Background()
	rounds := queue.NewRoundService(f.repo)

	round, reciting := f.activeRecitingEntryWithStudents(t, f.students, true)
	next, err := rounds.Reset(ctx, resetTestInput(f.session, f.teacher))
	if err != nil {
		t.Fatalf("reset: %v", err)
	}

	// Prior round: finalized, selection cleared, unfinished entries skipped
	// with manager attribution.
	prior, err := f.repo.Round(ctx, round.ID)
	if err != nil {
		t.Fatalf("load prior round: %v", err)
	}
	if prior.Lifecycle != queue.RoundLifecycleFinalized || prior.FinalizedAt == nil {
		t.Fatalf("prior round lifecycle = %s, want finalized", prior.Lifecycle)
	}
	if prior.SelectedEntryID != nil {
		t.Fatalf("prior selection = %v, want cleared", prior.SelectedEntryID)
	}
	for _, studentID := range f.students {
		entry := entryByStudent(t, f, round.ID, studentID)
		if entry.Status != queue.EntryStatusSkipped {
			t.Fatalf("student %s entry status = %s, want skipped", studentID, entry.Status)
		}
		if entry.ResolvedBy == nil || *entry.ResolvedBy != f.teacher {
			t.Fatalf("student %s resolved_by = %v, want teacher attribution", studentID, entry.ResolvedBy)
		}
	}

	// Successor: next sequential number, automatically activated.
	if next.RoundNumber != prior.RoundNumber+1 {
		t.Fatalf("successor round number = %d, want %d", next.RoundNumber, prior.RoundNumber+1)
	}
	if next.Lifecycle != queue.RoundLifecycleActive {
		t.Fatalf("successor lifecycle = %s, want active", next.Lifecycle)
	}

	// History immutability: no operation can mutate the finalized round's
	// entries (no reused positions or states).
	turns := queue.NewTurnService(f.repo)
	_, err = turns.Complete(ctx, reciting.ID, reciting.Version, f.teacher, nil, nil)
	if !queueErrorIs(err, queue.QueueErrorCodeRoundFinalized) {
		t.Fatalf("complete on finalized round error = %v, want round_finalized", err)
	}
	skippedEntry := entryByStudent(t, f, round.ID, f.students[0])
	_, err = turns.Skip(ctx, skippedEntry.ID, skippedEntry.Version, f.teacher)
	if !queueErrorIs(err, queue.QueueErrorCodeRoundFinalized) {
		t.Fatalf("skip on finalized round error = %v, want round_finalized", err)
	}

	// A second reset advances numbering again and leaves round 1 untouched.
	next2, err := rounds.Reset(ctx, resetTestInput(f.session, f.teacher))
	if err != nil {
		t.Fatalf("second reset: %v", err)
	}
	if next2.RoundNumber != next.RoundNumber+1 {
		t.Fatalf("third round number = %d, want %d", next2.RoundNumber, next.RoundNumber+1)
	}
	stillPrior := entryByStudent(t, f, round.ID, f.students[0])
	if stillPrior.Status != queue.EntryStatusSkipped || stillPrior.ID != skippedEntry.ID {
		t.Fatalf("prior entry mutated after second reset: %+v", stillPrior)
	}
}

func TestRecitationQueueResetHistory_PreserveLastStateKeepsEntryStates(t *testing.T) {
	f := newRecitationQueueCompletionFixture(t, true)
	ctx := context.Background()
	setFinalizationPolicy(t, f, queue.FinalizationPolicyPreserveLastState)
	rounds := queue.NewRoundService(f.repo)

	round, _ := f.activeRecitingEntryWithStudents(t, f.students, true)
	if _, err := rounds.Reset(ctx, resetTestInput(f.session, f.teacher)); err != nil {
		t.Fatalf("reset under preserve_last_state: %v", err)
	}

	prior, err := f.repo.Round(ctx, round.ID)
	if err != nil {
		t.Fatalf("load prior round: %v", err)
	}
	if prior.Lifecycle != queue.RoundLifecycleFinalized {
		t.Fatalf("prior round lifecycle = %s, want finalized", prior.Lifecycle)
	}
	if prior.SelectedEntryID != nil {
		t.Fatalf("prior selection = %v, want cleared", prior.SelectedEntryID)
	}

	// Last entry states survive as immutable historical facts: the reciting
	// student stays reciting, the waiting student stays waiting, and neither
	// gains manager attribution.
	recitingEntry := entryByStudent(t, f, round.ID, f.students[0])
	if recitingEntry.Status != queue.EntryStatusReciting || recitingEntry.ResolvedBy != nil {
		t.Fatalf("reciting entry after preserve reset = %+v, want unchanged state without attribution", recitingEntry)
	}
	waitingEntry := entryByStudent(t, f, round.ID, f.students[1])
	if waitingEntry.Status != queue.EntryStatusWaiting || waitingEntry.ResolvedBy != nil {
		t.Fatalf("waiting entry after preserve reset = %+v, want unchanged state without attribution", waitingEntry)
	}

	// The finalized round remains non-actionable despite the preserved state.
	turns := queue.NewTurnService(f.repo)
	_, err = turns.Skip(ctx, waitingEntry.ID, waitingEntry.Version, f.teacher)
	if !queueErrorIs(err, queue.QueueErrorCodeRoundFinalized) {
		t.Fatalf("skip on finalized preserve-state round error = %v, want round_finalized", err)
	}
}

func queueErrorIs(err error, code queue.QueueErrorCode) bool {
	var qerr *queue.QueueError
	return errors.As(err, &qerr) && qerr.Code == code
}
