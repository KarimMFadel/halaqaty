//go:build integration

package integration

// T040 (late join) — integration coverage for US2 late admission: admitted
// late joiners converge to exactly one durable entry/position under
// concurrent join hooks for both population policies (constraints as the
// final barrier), and the flow never touches media (no media-ish outbox
// events; queue tables carry no media material). The late-join production
// path is the F-005 participant-join observer hook
// (queue.SessionObserver.OnParticipantJoined) — T042's deliverable.

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/KarimMFadel/halaqaty/backend/internal/queue"
)

// ljJoinHook returns the production join hook bound to the fixture session.
func ljJoinHook(f *recitationQueueConcurrencyFixture) *queue.SessionObserver {
	return queue.NewSessionObserver(queue.NewRoundService(f.repo))
}

// ljSnapshot returns the durable entry student IDs and round version.
func ljSnapshot(t *testing.T, f *recitationQueueConcurrencyFixture, roundID string) ([]string, int64) {
	t.Helper()
	state := cqState(t, f, roundID)
	ids := make([]string, 0, len(state.Entries))
	for _, entry := range state.Entries {
		ids = append(ids, entry.StudentID)
	}
	return ids, state.Round.Version
}

func ljAssertExactlyOneEntry(t *testing.T, f *recitationQueueConcurrencyFixture, roundID, studentID string, wantPosition int) {
	t.Helper()
	state := cqState(t, f, roundID)
	count := 0
	for _, entry := range state.Entries {
		if entry.StudentID == studentID {
			count++
			if entry.Position != wantPosition {
				t.Fatalf("late joiner %s position = %d, want %d", studentID, entry.Position, wantPosition)
			}
			if entry.Status != queue.EntryStatusWaiting {
				t.Fatalf("late joiner %s status = %s, want waiting", studentID, entry.Status)
			}
		}
	}
	if count != 1 {
		t.Fatalf("late joiner %s durable entries = %d, want exactly 1 (state=%+v)", studentID, count, state.Entries)
	}
	cqAssertContiguousPositions(t, state)
}

// ljAssertNoMediaInteraction proves the queue tables and outbox rows written
// by the late-join flow carry no media material: no media-ish event types and
// no media-ish metadata keys. The import-graph media-boundary guard is T066.
func ljAssertNoMediaInteraction(t *testing.T, f *recitationQueueConcurrencyFixture) {
	t.Helper()
	rows, err := f.pool.Query(context.Background(), `
		SELECT event_type, event_metadata::text
		FROM queue_event_outbox
		WHERE session_id = $1::uuid
	`, f.session)
	if err != nil {
		t.Fatalf("list late-join outbox events: %v", err)
	}
	defer rows.Close()
	for rows.Next() {
		var eventType, metadata string
		if err := rows.Scan(&eventType, &metadata); err != nil {
			t.Fatalf("scan late-join outbox event: %v", err)
		}
		for _, forbidden := range []string{"audio", "media", "revoke", "grant", "convergence", "room", "credential"} {
			if strings.Contains(eventType, forbidden) || strings.Contains(metadata, forbidden) {
				t.Fatalf("late-join flow wrote media material: event_type=%q metadata=%q", eventType, metadata)
			}
		}
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate late-join outbox events: %v", err)
	}
}

// TestRecitationQueueLateJoin_ConcurrentJoinHooksConvergeToOneEntry races
// parallel committed join facts for one student against the active round
// under both population policies: every hook returns nil (idempotent join
// facts), exactly one entry/position materializes at the end, and the round
// version bumps exactly once.
func TestRecitationQueueLateJoin_ConcurrentJoinHooksConvergeToOneEntry(t *testing.T) {
	for _, policy := range []queue.PopulationPolicy{
		queue.PopulationPolicyPresentAtActivation,
		queue.PopulationPolicyAllActiveStudents,
	} {
		t.Run(string(policy), func(t *testing.T) {
			f := newRecitationQueueConcurrencyFixture(t)
			ctx := context.Background()
			if _, err := f.pool.Exec(ctx, `
				UPDATE sessions SET queue_population_policy = $2 WHERE id = $1::uuid
			`, f.session, policy); err != nil {
				t.Fatalf("set population policy: %v", err)
			}
			first := f.addStudent(t, "lj-first", time.Now().UTC().Add(-2*time.Minute))
			second := f.addStudent(t, "lj-second", time.Now().UTC().Add(-time.Minute))
			round := cqCreateActiveRound(t, f, first, second)
			// Circle membership created after round activation models a
			// member added late — the only append-eligible case under
			// all_active_students, and the normal late-joiner case under
			// present_at_activation.
			late := f.addStudent(t, "lj-late", time.Now().UTC())

			_, versionBefore := ljSnapshot(t, f, round.ID)
			hook := ljJoinHook(f)
			const joinFacts = 8
			operations := make([]func(context.Context) error, joinFacts)
			for i := range operations {
				operations[i] = func(ctx context.Context) error {
					return hook.OnParticipantJoined(ctx, f.session, late)
				}
			}
			results := cqRace(t, operations...)
			for _, result := range results {
				if result.err != nil {
					t.Fatalf("racing join hook failed (all committed join facts must converge): %v", result.err)
				}
			}

			ljAssertExactlyOneEntry(t, f, round.ID, late, 3)
			_, versionAfter := ljSnapshot(t, f, round.ID)
			if versionAfter != versionBefore+1 {
				t.Fatalf("round version after racing joins = %d, want exactly one bump from %d", versionAfter, versionBefore)
			}
			ljAssertNoMediaInteraction(t, f)
		})
	}
}

// TestRecitationQueueLateJoin_AllActiveStudentsPrepopulatedMemberNeverDuplicates
// pins FR-003 under all_active_students: a member already materialized at
// activation keeps exactly one entry; replayed join facts for them are no-ops
// even under concurrency.
func TestRecitationQueueLateJoin_AllActiveStudentsPrepopulatedMemberNeverDuplicates(t *testing.T) {
	f := newRecitationQueueConcurrencyFixture(t)
	ctx := context.Background()
	if _, err := f.pool.Exec(ctx, `
		UPDATE sessions SET queue_population_policy = $2 WHERE id = $1::uuid
	`, f.session, queue.PopulationPolicyAllActiveStudents); err != nil {
		t.Fatalf("set population policy: %v", err)
	}
	first := f.addStudent(t, "lj-pre-first", time.Now().UTC().Add(-2*time.Minute))
	second := f.addStudent(t, "lj-pre-second", time.Now().UTC().Add(-time.Minute))
	round := cqCreateActiveRound(t, f, first, second)

	_, versionBefore := ljSnapshot(t, f, round.ID)
	hook := ljJoinHook(f)
	results := cqRace(t,
		func(ctx context.Context) error { return hook.OnParticipantJoined(ctx, f.session, first) },
		func(ctx context.Context) error { return hook.OnParticipantJoined(ctx, f.session, first) },
		func(ctx context.Context) error { return hook.OnParticipantJoined(ctx, f.session, second) },
	)
	for _, result := range results {
		if result.err != nil {
			t.Fatalf("pre-populated member join fact failed: %v", result.err)
		}
	}

	state := cqState(t, f, round.ID)
	if len(state.Entries) != 2 {
		t.Fatalf("entries after pre-populated member joins = %d, want still 2: %+v", len(state.Entries), state.Entries)
	}
	if state.Round.Version != versionBefore {
		t.Fatalf("round version after no-op joins = %d, want unchanged %d", state.Round.Version, versionBefore)
	}
	ljAssertExactlyOneEntry(t, f, round.ID, first, 1)
	ljAssertNoMediaInteraction(t, f)
}
