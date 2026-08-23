//go:build integration

package queue

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestPolicyServiceUpdate_CoversAllPolicyDimensionsAndEffectiveVersioning(t *testing.T) {
	repo := newQueueRepository(t)
	ctx := context.Background()
	teacher := qSeedUser(t, repo, "policy-teacher")
	circle := qSeedCircle(t, repo, teacher)
	qSeedMember(t, repo, circle, teacher, "teacher", time.Now().UTC())
	session := qInsertSession(t, repo, circle, teacher, string(GradeVisibilityManagersAndStudent))
	service := NewPolicyService(repo)

	current, err := repo.SessionPolicy(ctx, session)
	if err != nil {
		t.Fatalf("load initial policy: %v", err)
	}

	updates := []struct {
		name  string
		apply func(QueuePolicy) QueuePolicy
		check func(t *testing.T, got QueuePolicy)
	}{
		{
			name: "population all_active_students",
			apply: func(p QueuePolicy) QueuePolicy {
				p.Population = PopulationPolicyAllActiveStudents
				return p
			},
			check: func(t *testing.T, got QueuePolicy) {
				t.Helper()
				if got.Population != PopulationPolicyAllActiveStudents {
					t.Fatalf("population = %s, want %s", got.Population, PopulationPolicyAllActiveStudents)
				}
			},
		},
		{
			name: "population present_at_activation",
			apply: func(p QueuePolicy) QueuePolicy {
				p.Population = PopulationPolicyPresentAtActivation
				return p
			},
			check: func(t *testing.T, got QueuePolicy) {
				t.Helper()
				if got.Population != PopulationPolicyPresentAtActivation {
					t.Fatalf("population = %s, want %s", got.Population, PopulationPolicyPresentAtActivation)
				}
			},
		},
		{
			name: "finalization preserve_last_state",
			apply: func(p QueuePolicy) QueuePolicy {
				p.Finalization = FinalizationPolicyPreserveLastState
				return p
			},
			check: func(t *testing.T, got QueuePolicy) {
				t.Helper()
				if got.Finalization != FinalizationPolicyPreserveLastState {
					t.Fatalf("finalization = %s, want %s", got.Finalization, FinalizationPolicyPreserveLastState)
				}
			},
		},
		{
			name: "finalization mark_unfinished_skipped",
			apply: func(p QueuePolicy) QueuePolicy {
				p.Finalization = FinalizationPolicyMarkUnfinishedSkipped
				return p
			},
			check: func(t *testing.T, got QueuePolicy) {
				t.Helper()
				if got.Finalization != FinalizationPolicyMarkUnfinishedSkipped {
					t.Fatalf("finalization = %s, want %s", got.Finalization, FinalizationPolicyMarkUnfinishedSkipped)
				}
			},
		},
		{
			name: "opt out auto_approve",
			apply: func(p QueuePolicy) QueuePolicy {
				p.OptOut = OptOutPolicyAutoApprove
				return p
			},
			check: func(t *testing.T, got QueuePolicy) {
				t.Helper()
				if got.OptOut != OptOutPolicyAutoApprove {
					t.Fatalf("opt_out = %s, want %s", got.OptOut, OptOutPolicyAutoApprove)
				}
			},
		},
		{
			name: "opt out approval_required",
			apply: func(p QueuePolicy) QueuePolicy {
				p.OptOut = OptOutPolicyApprovalRequired
				return p
			},
			check: func(t *testing.T, got QueuePolicy) {
				t.Helper()
				if got.OptOut != OptOutPolicyApprovalRequired {
					t.Fatalf("opt_out = %s, want %s", got.OptOut, OptOutPolicyApprovalRequired)
				}
			},
		},
		{
			name: "visibility managers_only",
			apply: func(p QueuePolicy) QueuePolicy {
				p.GradeVisibility = GradeVisibilityManagersOnly
				return p
			},
			check: func(t *testing.T, got QueuePolicy) {
				t.Helper()
				if got.GradeVisibility != GradeVisibilityManagersOnly {
					t.Fatalf("grade_visibility = %s, want %s", got.GradeVisibility, GradeVisibilityManagersOnly)
				}
			},
		},
		{
			name: "visibility all_participants",
			apply: func(p QueuePolicy) QueuePolicy {
				p.GradeVisibility = GradeVisibilityAllParticipants
				return p
			},
			check: func(t *testing.T, got QueuePolicy) {
				t.Helper()
				if got.GradeVisibility != GradeVisibilityAllParticipants {
					t.Fatalf("grade_visibility = %s, want %s", got.GradeVisibility, GradeVisibilityAllParticipants)
				}
			},
		},
		{
			name: "visibility managers_and_student",
			apply: func(p QueuePolicy) QueuePolicy {
				p.GradeVisibility = GradeVisibilityManagersAndStudent
				return p
			},
			check: func(t *testing.T, got QueuePolicy) {
				t.Helper()
				if got.GradeVisibility != GradeVisibilityManagersAndStudent {
					t.Fatalf("grade_visibility = %s, want %s", got.GradeVisibility, GradeVisibilityManagersAndStudent)
				}
			},
		},
		{
			name: "correction before_round_finalization",
			apply: func(p QueuePolicy) QueuePolicy {
				p.GradeCorrection = GradeCorrectionBeforeRoundFinalization
				return p
			},
			check: func(t *testing.T, got QueuePolicy) {
				t.Helper()
				if got.GradeCorrection != GradeCorrectionBeforeRoundFinalization {
					t.Fatalf("grade_correction = %s, want %s", got.GradeCorrection, GradeCorrectionBeforeRoundFinalization)
				}
			},
		},
		{
			name: "correction immutable",
			apply: func(p QueuePolicy) QueuePolicy {
				p.GradeCorrection = GradeCorrectionImmutable
				return p
			},
			check: func(t *testing.T, got QueuePolicy) {
				t.Helper()
				if got.GradeCorrection != GradeCorrectionImmutable {
					t.Fatalf("grade_correction = %s, want %s", got.GradeCorrection, GradeCorrectionImmutable)
				}
			},
		},
		{
			name: "correction audited_any_time",
			apply: func(p QueuePolicy) QueuePolicy {
				p.GradeCorrection = GradeCorrectionAuditedAnyTime
				return p
			},
			check: func(t *testing.T, got QueuePolicy) {
				t.Helper()
				if got.GradeCorrection != GradeCorrectionAuditedAnyTime {
					t.Fatalf("grade_correction = %s, want %s", got.GradeCorrection, GradeCorrectionAuditedAnyTime)
				}
			},
		},
	}

	for _, update := range updates {
		t.Run(update.name, func(t *testing.T) {
			next := update.apply(current.Policy)
			got, err := service.Update(ctx, session, teacher, current.Policy.Version, next)
			if err != nil {
				t.Fatalf("update policy: %v", err)
			}
			if got.Version != current.Policy.Version+1 {
				t.Fatalf("version = %d, want %d", got.Version, current.Policy.Version+1)
			}
			update.check(t, got)
			current.Policy = got
		})
	}

	noop, err := service.Update(ctx, session, teacher, current.Policy.Version, current.Policy)
	if err != nil {
		t.Fatalf("noop update: %v", err)
	}
	if noop.Version != current.Policy.Version {
		t.Fatalf("noop version = %d, want unchanged %d", noop.Version, current.Policy.Version)
	}
}

func TestPolicyServiceUpdate_RequiresManagerAndEditableSession(t *testing.T) {
	repo := newQueueRepository(t)
	ctx := context.Background()
	teacher := qSeedUser(t, repo, "guard-teacher")
	supervisor := qSeedUser(t, repo, "guard-supervisor")
	student := qSeedUser(t, repo, "guard-student")
	circle := qSeedCircle(t, repo, teacher)
	now := time.Now().UTC()
	qSeedMember(t, repo, circle, teacher, "teacher", now)
	qSeedMember(t, repo, circle, supervisor, "supervisor", now.Add(time.Minute))
	qSeedMember(t, repo, circle, student, "student", now.Add(2*time.Minute))
	session := qInsertSession(t, repo, circle, teacher, string(GradeVisibilityManagersAndStudent))
	service := NewPolicyService(repo)

	policyCtx, err := repo.SessionPolicy(ctx, session)
	if err != nil {
		t.Fatalf("load policy: %v", err)
	}
	next := policyCtx.Policy
	next.OptOut = OptOutPolicyAutoApprove

	got, err := service.Update(ctx, session, supervisor, policyCtx.Policy.Version, next)
	if err != nil {
		t.Fatalf("supervisor update: %v", err)
	}
	if got.OptOut != OptOutPolicyAutoApprove {
		t.Fatalf("supervisor update opt_out = %s, want %s", got.OptOut, OptOutPolicyAutoApprove)
	}

	_, err = service.Update(ctx, session, student, got.Version, next)
	if queueErrCode(t, err) != QueueErrorCodeInvalidTransition {
		t.Fatalf("student update error = %v, want invalid_transition", err)
	}

	if _, err := repo.pool.Exec(ctx, `UPDATE sessions SET status = 'ended' WHERE id = $1::uuid`, session); err != nil {
		t.Fatalf("end session: %v", err)
	}
	next.GradeVisibility = GradeVisibilityAllParticipants
	_, err = service.Update(ctx, session, teacher, got.Version, next)
	if queueErrCode(t, err) != QueueErrorCodeRoundFinalized {
		t.Fatalf("ended session update error = %v, want round_finalized", err)
	}
}

func TestPolicyServiceUpdateRejectsEndedSessionRace(t *testing.T) {
	repo := newQueueRepository(t)
	teacher := qSeedUser(t, repo, "policy-ended-race-teacher")
	circle := qSeedCircle(t, repo, teacher)
	qSeedMember(t, repo, circle, teacher, "teacher", time.Now().UTC())
	session := qInsertSession(t, repo, circle, teacher, string(GradeVisibilityManagersAndStudent))
	current, err := repo.SessionPolicy(context.Background(), session)
	if err != nil {
		t.Fatalf("load policy before race: %v", err)
	}
	next := current.Policy
	next.GradeVisibility = GradeVisibilityAllParticipants
	lock := qLockSession(t, repo, session)
	result := make(chan error, 1)
	go func() {
		_, err := NewPolicyService(repo).Update(context.Background(), session, teacher, current.Policy.Version, next)
		result <- err
	}()
	qWaitForQueueTransaction(t, repo)
	if _, err := lock.Exec(context.Background(), `UPDATE sessions SET status = 'ended' WHERE id = $1::uuid`, session); err != nil {
		t.Fatalf("end session while policy update waits: %v", err)
	}
	if err := lock.Commit(context.Background()); err != nil {
		t.Fatalf("commit session end: %v", err)
	}
	if err := <-result; queueErrCode(t, err) != QueueErrorCodeRoundFinalized {
		t.Fatalf("policy update ended-session error = %v, want round_finalized", err)
	}

	after, err := repo.SessionPolicy(context.Background(), session)
	if err != nil {
		t.Fatalf("load policy after rejected race: %v", err)
	}
	if after.Policy.Version != current.Policy.Version || after.Policy.GradeVisibility != current.Policy.GradeVisibility {
		t.Fatalf("policy changed after ended-session race: before=%+v after=%+v", current.Policy, after.Policy)
	}
}

func TestPolicyServiceUpdateRechecksManagerRoleAfterSessionLock(t *testing.T) {
	repo := newQueueRepository(t)
	teacher := qSeedUser(t, repo, "policy-role-race-teacher")
	circle := qSeedCircle(t, repo, teacher)
	qSeedMember(t, repo, circle, teacher, "teacher", time.Now().UTC())
	session := qInsertSession(t, repo, circle, teacher, string(GradeVisibilityManagersAndStudent))
	current, err := repo.SessionPolicy(context.Background(), session)
	if err != nil {
		t.Fatalf("load policy before race: %v", err)
	}
	next := current.Policy
	next.GradeVisibility = GradeVisibilityAllParticipants
	lock := qLockSession(t, repo, session)
	result := make(chan error, 1)
	go func() {
		_, err := NewPolicyService(repo).Update(context.Background(), session, teacher, current.Policy.Version, next)
		result <- err
	}()
	qWaitForQueueTransaction(t, repo)
	if _, err := repo.pool.Exec(context.Background(), `
		UPDATE circle_members
		SET role = 'student', updated_at = NOW()
		WHERE circle_id = $1::uuid AND user_id = $2::uuid
	`, circle, teacher); err != nil {
		t.Fatalf("demote manager while policy update waits: %v", err)
	}
	if err := lock.Commit(context.Background()); err != nil {
		t.Fatalf("release session lock: %v", err)
	}
	if queueErrCode(t, <-result) != QueueErrorCodeInvalidTransition {
		t.Fatalf("policy update after role demotion was allowed, want invalid_transition")
	}
	after, err := repo.SessionPolicy(context.Background(), session)
	if err != nil {
		t.Fatalf("load policy after role race: %v", err)
	}
	if after.Policy.Version != current.Policy.Version || after.Policy.GradeVisibility != current.Policy.GradeVisibility {
		t.Fatalf("policy changed after role-demotion race: before=%+v after=%+v", current.Policy, after.Policy)
	}
}

func TestRepositoryUpdateSessionPolicyRejectsEndedSession(t *testing.T) {
	repo := newQueueRepository(t)
	teacher := qSeedUser(t, repo, "repository-ended-policy-teacher")
	circle := qSeedCircle(t, repo, teacher)
	qSeedMember(t, repo, circle, teacher, "teacher", time.Now().UTC())
	session := qInsertSession(t, repo, circle, teacher, string(GradeVisibilityManagersAndStudent))
	current, err := repo.SessionPolicy(context.Background(), session)
	if err != nil {
		t.Fatalf("load policy before ended update: %v", err)
	}
	if _, err := repo.pool.Exec(context.Background(), `UPDATE sessions SET status = 'ended' WHERE id = $1::uuid`, session); err != nil {
		t.Fatalf("end session: %v", err)
	}
	next := current.Policy
	next.GradeVisibility = GradeVisibilityAllParticipants
	if _, err := repo.UpdateSessionPolicy(context.Background(), session, current.Policy.Version, next); queueErrCode(t, err) != QueueErrorCodeRoundFinalized {
		t.Fatalf("repository ended-session error = %v, want round_finalized", err)
	}
}

func TestPolicyServiceUpdate_WorkflowProspectiveAndVisibilityImmediate(t *testing.T) {
	repo := newQueueRepository(t)
	ctx := context.Background()
	teacher := qSeedUser(t, repo, "visibility-teacher")
	viewer := qSeedUser(t, repo, "visibility-viewer")
	otherA := qSeedUser(t, repo, "visibility-other-a")
	otherB := qSeedUser(t, repo, "visibility-other-b")
	circle := qSeedCircle(t, repo, teacher)
	now := time.Now().UTC()
	qSeedMember(t, repo, circle, teacher, "teacher", now)
	qSeedMember(t, repo, circle, viewer, "student", now.Add(time.Minute))
	qSeedMember(t, repo, circle, otherA, "student", now.Add(2*time.Minute))
	qSeedMember(t, repo, circle, otherB, "student", now.Add(3*time.Minute))
	session := qInsertSession(t, repo, circle, teacher, string(GradeVisibilityManagersAndStudent))
	if _, err := repo.pool.Exec(ctx, `UPDATE sessions SET status = 'active' WHERE id = $1::uuid`, session); err != nil {
		t.Fatalf("activate session: %v", err)
	}
	round := qCreateRound(t, repo, session, teacher, string(RoundLifecycleActive), []string{otherA, otherB, viewer})
	firstCompleted, _ := qCompleteEntry(t, repo, round, teacher, GradeGood, "first-private-note")

	before, err := repo.LoadQueueState(ctx, round.ID, Viewer{UserID: viewer, IsManager: false})
	if err != nil {
		t.Fatalf("load state before visibility update: %v", err)
	}
	firstBefore := entryByStudentID(t, before.Entries, otherA)
	if firstBefore.Grade != nil || firstBefore.TeacherNotes != nil {
		t.Fatalf("viewer saw other student's details before visibility change: %+v", firstBefore)
	}

	policyCtx, err := repo.SessionPolicy(ctx, session)
	if err != nil {
		t.Fatalf("load policy for update: %v", err)
	}
	next := policyCtx.Policy
	next.Population = PopulationPolicyAllActiveStudents
	next.Finalization = FinalizationPolicyPreserveLastState
	next.OptOut = OptOutPolicyAutoApprove
	next.GradeVisibility = GradeVisibilityAllParticipants
	next.GradeCorrection = GradeCorrectionImmutable
	updated, err := NewPolicyService(repo).Update(ctx, session, teacher, policyCtx.Policy.Version, next)
	if err != nil {
		t.Fatalf("update policy: %v", err)
	}
	if updated.Version != policyCtx.Policy.Version+1 {
		t.Fatalf("updated policy version = %d, want %d", updated.Version, policyCtx.Policy.Version+1)
	}

	after, err := repo.LoadQueueState(ctx, round.ID, Viewer{UserID: viewer, IsManager: false})
	if err != nil {
		t.Fatalf("load state after visibility update: %v", err)
	}
	firstAfter := entryByStudentID(t, after.Entries, otherA)
	if firstAfter.Grade == nil || *firstAfter.Grade != GradeGood {
		t.Fatalf("viewer did not gain immediate grade visibility: %+v", firstAfter)
	}
	if firstAfter.TeacherNotes == nil || *firstAfter.TeacherNotes != "first-private-note" {
		t.Fatalf("viewer did not gain immediate note visibility: %+v", firstAfter)
	}

	var roundVersionBefore int64
	var statusBefore EntryStatus
	if err := repo.pool.QueryRow(ctx, `
		SELECT q.version, e.status
		FROM recitation_queue q
		JOIN recitation_queue_entries e ON e.id = $2::uuid
		WHERE q.id = $1::uuid
	`, round.ID, firstCompleted.ID).Scan(&roundVersionBefore, &statusBefore); err != nil {
		t.Fatalf("load persisted round/entry state: %v", err)
	}
	if statusBefore != EntryStatusCompleted {
		t.Fatalf("completed entry status = %s, want completed", statusBefore)
	}

	secondCompleted, _ := qCompleteEntry(t, repo, round, teacher, GradeRepeat, "second-private-note")
	later, err := repo.LoadQueueState(ctx, round.ID, Viewer{UserID: viewer, IsManager: false})
	if err != nil {
		t.Fatalf("load state after future completion: %v", err)
	}
	secondAfter := entryByStudentID(t, later.Entries, otherB)
	if secondAfter.ID != secondCompleted.ID {
		t.Fatalf("wrong later entry loaded: got %s, want %s", secondAfter.ID, secondCompleted.ID)
	}
	if secondAfter.Grade == nil || *secondAfter.Grade != GradeRepeat {
		t.Fatalf("viewer did not gain prospective grade visibility: %+v", secondAfter)
	}
	if secondAfter.TeacherNotes == nil || *secondAfter.TeacherNotes != "second-private-note" {
		t.Fatalf("viewer did not gain prospective note visibility: %+v", secondAfter)
	}

	var roundVersionAfter int64
	var statusAfter EntryStatus
	if err := repo.pool.QueryRow(ctx, `
		SELECT q.version, e.status
		FROM recitation_queue q
		JOIN recitation_queue_entries e ON e.id = $2::uuid
		WHERE q.id = $1::uuid
	`, round.ID, firstCompleted.ID).Scan(&roundVersionAfter, &statusAfter); err != nil {
		t.Fatalf("reload persisted round/entry state: %v", err)
	}
	if statusAfter != EntryStatusCompleted {
		t.Fatalf("existing entry status after workflow-policy change = %s, want completed", statusAfter)
	}
	if roundVersionAfter < roundVersionBefore {
		t.Fatalf("round version moved backwards: before=%d after=%d", roundVersionBefore, roundVersionAfter)
	}
}

func TestPolicyServiceUpdate_EmitsRedactedAuditEvent(t *testing.T) {
	repo := newQueueRepository(t)
	ctx := context.Background()
	teacher := qSeedUser(t, repo, "audit-teacher")
	circle := qSeedCircle(t, repo, teacher)
	qSeedMember(t, repo, circle, teacher, "teacher", time.Now().UTC())
	session := qInsertSession(t, repo, circle, teacher, string(GradeVisibilityManagersAndStudent))
	audit := &policyAuditCapture{}
	service := NewPolicyServiceWithAudit(repo, audit)

	policyCtx, err := repo.SessionPolicy(ctx, session)
	if err != nil {
		t.Fatalf("load policy: %v", err)
	}
	next := policyCtx.Policy
	next.GradeCorrection = GradeCorrectionImmutable

	if _, err := service.Update(ctx, session, teacher, policyCtx.Policy.Version, next); err != nil {
		t.Fatalf("update policy: %v", err)
	}

	if len(audit.changes) != 1 {
		t.Fatalf("policy audit events = %d, want 1", len(audit.changes))
	}
	if _, ok := audit.changes[0]["grade_correction"]; !ok {
		t.Fatalf("policy audit omitted grade_correction change: %+v", audit.changes[0])
	}
	for key, change := range audit.changes[0] {
		if change[0] == "" || change[1] == "" {
			t.Fatalf("policy audit change %q was not redacted closed-enum data: %+v", key, change)
		}
	}
}

type policyAuditCapture struct{ changes []map[string][2]string }

func (c *policyAuditCapture) PolicyChanged(_ context.Context, _, _ string, changes map[string][2]string) {
	c.changes = append(c.changes, changes)
}

func entryByStudentID(t *testing.T, entries []QueueEntry, studentID string) QueueEntry {
	t.Helper()
	for _, entry := range entries {
		if entry.StudentID == studentID {
			return entry
		}
	}
	t.Fatalf("entry for student %s not found", studentID)
	return QueueEntry{}
}

func TestPolicyServiceUpdate_RedValidationEvidence(t *testing.T) {
	repo := newQueueRepository(t)
	service := NewPolicyService(repo)
	ctx := context.Background()

	_, err := service.Update(ctx, "session-id", "actor-id", 0, QueuePolicy{})
	if err == nil {
		t.Fatal("expected validation error for expected version below 1")
	}
	var qerr *QueueError
	if !errors.As(err, &qerr) {
		t.Fatalf("expected queue error, got %T: %v", err, err)
	}
	if qerr.Code != QueueErrorCodeValidation {
		t.Fatalf("error code = %s, want %s", qerr.Code, QueueErrorCodeValidation)
	}
}
