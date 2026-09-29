//go:build integration

package scheduling

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"
)

func TestScheduleService_AuthorizationReplayAndSeries(t *testing.T) {
	ctx := context.Background()
	pool := openRepoTestPool(t, ctx)
	teacher := "11111111-1111-1111-1111-111111111111"
	student := "22222222-2222-2222-2222-222222222222"
	circle := "33333333-3333-3333-3333-333333333333"
	for _, id := range []string{teacher, student} {
		seedRepoUser(t, ctx, pool, id)
	}
	seedRepoCircle(t, ctx, pool, circle, teacher, "service-circle")
	for _, member := range []struct{ id, role string }{{teacher, "teacher"}, {student, "student"}} {
		if _, err := pool.Exec(ctx, `INSERT INTO circle_members(circle_id,user_id,role) VALUES($1,$2,$3)`, circle, member.id, member.role); err != nil {
			t.Fatal(err)
		}
	}
	svc := NewScheduleService(NewScheduleRepository(pool))
	svc.now = func() time.Time { return civilDate(2030, 1, 1) }
	plan := intervalRecord(civilDate(2030, 1, 2), 1, IntervalUnitDay)
	cmd := CreateScheduleCommand{ActorID: teacher, CircleID: circle, IdempotencyKey: "create", Plan: plan}
	denied := cmd
	denied.ActorID = student
	if _, err := svc.Create(ctx, denied); !errors.Is(err, ErrInsufficientCircleRole) {
		t.Fatalf("student create = %v", err)
	}
	past := cmd
	past.Plan.AnchorLocalDate = civilDate(2029, 1, 1)
	past.Plan.EffectiveLocalDate = past.Plan.AnchorLocalDate
	if _, err := svc.Create(ctx, past); !errors.Is(err, ErrPastPlannedTime) {
		t.Fatalf("past create = %v", err)
	}
	created, err := svc.Create(ctx, cmd)
	if err != nil {
		t.Fatal(err)
	}
	replay, err := svc.Create(ctx, cmd)
	if err != nil || replay.ID != created.ID {
		t.Fatalf("replay = %+v, %v", replay, err)
	}
	changedInput := cmd
	changedInput.Plan.Title = "different"
	if _, err := svc.Create(ctx, changedInput); !errors.Is(err, ErrReplayConflict) {
		t.Fatalf("changed replay=%v", err)
	}
	listed, err := svc.List(ctx, circle, student)
	if err != nil || len(listed) != 1 || listed[0].ID != created.ID {
		t.Fatalf("list=%+v %v", listed, err)
	}
	// The same actor/key cannot replay after losing management rights.
	if _, err := pool.Exec(ctx, `UPDATE circle_members SET role='supervisor' WHERE user_id=$1`, teacher); err != nil {
		t.Fatal(err)
	}
	change := ChangeScheduleCommand{ActorID: teacher, CircleID: circle, ScheduleID: created.ID, IdempotencyKey: "change", ExpectedVersion: 1, EffectiveLocalDate: civilDate(2030, 1, 3), Plan: plan}
	change.Plan.Title = "supervisor revision"
	updated, err := svc.Change(ctx, change)
	if err != nil || updated.CurrentVersion != 2 {
		t.Fatalf("change=%+v %v", updated, err)
	}
	change.IdempotencyKey = "stale"
	if _, err := svc.Change(ctx, change); !errors.Is(err, ErrScheduleConflict) {
		t.Fatalf("stale=%v", err)
	}
	stop := change
	stop.IdempotencyKey = "stop"
	stop.ExpectedVersion = 2
	stop.Stop = true
	stopped, err := svc.Change(ctx, stop)
	if err != nil || stopped.StoppedFromLocalDate == nil || stopped.CurrentVersion != 3 {
		t.Fatalf("stop=%+v %v", stopped, err)
	}
	if _, err := pool.Exec(ctx, `UPDATE circle_members SET role='student' WHERE user_id=$1`, teacher); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Create(ctx, cmd); !errors.Is(err, ErrInsufficientCircleRole) {
		t.Fatalf("revoked replay=%v", err)
	}
	if _, err := pool.Exec(ctx, `UPDATE circles SET is_archived=true WHERE id=$1`, circle); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.List(ctx, circle, teacher); err != nil {
		t.Fatalf("archive read=%v", err)
	}
	if _, err := svc.Create(ctx, cmd); !errors.Is(err, ErrCircleArchived) {
		t.Fatalf("archive write=%v", err)
	}
}

func TestScheduleService_RequiresFreshReviewedOverlapWarnings(t *testing.T) {
	ctx := context.Background()
	pool := openRepoTestPool(t, ctx)
	teacher := "11111111-1111-1111-1111-111111111111"
	circles := []string{"22222222-2222-2222-2222-222222222222", "33333333-3333-3333-3333-333333333333", "66666666-6666-6666-6666-666666666666", "77777777-7777-7777-7777-777777777777"}
	seedRepoUser(t, ctx, pool, teacher)
	for i, circle := range circles {
		seedRepoCircle(t, ctx, pool, circle, teacher, fmt.Sprintf("HLQ-OVERLAP%d", i))
		if _, err := pool.Exec(ctx, `INSERT INTO circle_members(circle_id,user_id,role) VALUES($1::uuid,$2::uuid,'teacher')`, circle, teacher); err != nil {
			t.Fatal(err)
		}
	}
	svc := NewScheduleService(NewScheduleRepository(pool))
	svc.now = func() time.Time { return civilDate(2030, 1, 1) }
	date := civilDate(2030, 1, 2)
	base := intervalRecord(date, 1, IntervalUnitDay)
	if _, err := svc.Create(ctx, CreateScheduleCommand{ActorID: teacher, CircleID: circles[0], IdempotencyKey: "overlap-base", Plan: base}); err != nil {
		t.Fatalf("create base schedule: %v", err)
	}
	proposal := selectedDatesRecord(date, date)
	proposal.StartLocalTime, proposal.EndLocalTime = LocalClock{Hour: 20, Minute: 30}, LocalClock{Hour: 21, Minute: 30}
	proposal.DurationMinutes = 60
	preview, err := svc.overlap.Preview(ctx, teacher, circles[1], proposal)
	if err != nil || len(preview.Warnings) != 1 {
		t.Fatalf("initial preview warnings=%#v err=%v; want one", preview.Warnings, err)
	}
	exactProposal := selectedDatesRecord(civilDate(2030, 1, 3), civilDate(2030, 1, 3))
	exactProposal.StartLocalTime, exactProposal.EndLocalTime = proposal.StartLocalTime, proposal.EndLocalTime
	exactProposal.DurationMinutes = proposal.DurationMinutes
	exactPreview, err := svc.overlap.Preview(ctx, teacher, circles[3], exactProposal)
	if err != nil || len(exactPreview.Warnings) != 1 {
		t.Fatalf("write preview warnings=%#v err=%v; want one", exactPreview.Warnings, err)
	}
	if _, err := svc.Create(ctx, CreateScheduleCommand{ActorID: teacher, CircleID: circles[3], IdempotencyKey: "overlap-preview-confirmed", Plan: exactProposal,
		ConfirmOverlaps: true, ConfirmedWarningIDs: []string{exactPreview.Warnings[0].WarningID}}); err != nil {
		t.Fatalf("write with exact preview warning ID: %v", err)
	}
	newCommitment := selectedDatesRecord(date, date)
	newCommitment.StartLocalTime, newCommitment.EndLocalTime = LocalClock{Hour: 21}, LocalClock{Hour: 21, Minute: 45}
	newCommitment.DurationMinutes = 45
	if _, err := svc.Create(ctx, CreateScheduleCommand{ActorID: teacher, CircleID: circles[2], IdempotencyKey: "overlap-later", Plan: newCommitment}); err != nil {
		t.Fatalf("create commitment touching existing schedule: %v", err)
	}
	cmd := CreateScheduleCommand{ActorID: teacher, CircleID: circles[1], IdempotencyKey: "overlap-proposal", Plan: proposal,
		ConfirmOverlaps: true, ConfirmedWarningIDs: []string{preview.Warnings[0].WarningID}}
	if _, err := svc.Create(ctx, cmd); err == nil {
		t.Fatal("write with stale reviewed warnings succeeded")
	} else {
		var conflict *OverlapConfirmationError
		if !errors.As(err, &conflict) || len(conflict.Warnings) != 2 {
			t.Fatalf("stale warning error=%v, want refreshed two-warning conflict", err)
		}
		cmd.ConfirmedWarningIDs = []string{conflict.Warnings[0].WarningID, conflict.Warnings[1].WarningID}
	}
	if _, err := svc.Create(ctx, cmd); err != nil {
		t.Fatalf("write with refreshed reviewed warning IDs: %v", err)
	}
}

func TestScheduleService_OccurrenceCASMaterializedAndHistory(t *testing.T) {
	ctx := context.Background()
	pool := openRepoTestPool(t, ctx)
	teacher := "11111111-1111-1111-1111-111111111111"
	circle := "33333333-3333-3333-3333-333333333333"
	seedRepoUser(t, ctx, pool, teacher)
	seedRepoCircle(t, ctx, pool, circle, teacher, "occ-circle")
	if _, err := pool.Exec(ctx, `INSERT INTO circle_members(circle_id,user_id,role) VALUES($1,$2,'teacher')`, circle, teacher); err != nil {
		t.Fatal(err)
	}
	repo := NewScheduleRepository(pool)
	svc := NewScheduleService(repo)
	svc.now = func() time.Time { return civilDate(2030, 1, 1) }
	plan := intervalRecord(civilDate(2030, 1, 2), 1, IntervalUnitDay)
	sch, err := svc.Create(ctx, CreateScheduleCommand{ActorID: teacher, CircleID: circle, IdempotencyKey: "create", Plan: plan})
	if err != nil {
		t.Fatal(err)
	}
	session := "44444444-4444-4444-4444-444444444444"
	seedStartedMaterialization(t, ctx, pool, session, sch.ID, circle, teacher, civilDate(2030, 1, 2))
	cmd := ChangeOccurrenceCommand{ActorID: teacher, CircleID: circle, ScheduleID: sch.ID, IdempotencyKey: "cancel", OriginalLocalDate: civilDate(2030, 1, 2), ExpectedSeriesVersion: 1, Cancelled: boolPtr(true)}
	if _, err := svc.ChangeOccurrence(ctx, cmd); !errors.Is(err, ErrOccurrenceStarted) {
		t.Fatalf("started cancel=%v", err)
	}
	if _, err := pool.Exec(ctx, `UPDATE sessions SET status='scheduled',actual_start=NULL WHERE id=$1`, session); err != nil {
		t.Fatal(err)
	}
	cancelled, err := svc.ChangeOccurrence(ctx, cmd)
	if err != nil || cancelled.CancelledAt == nil || cancelled.Version != 1 {
		t.Fatalf("cancel=%+v %v", cancelled, err)
	}
	var detailCancelled bool
	if err := pool.QueryRow(ctx, `SELECT cancelled_at IS NOT NULL FROM planned_session_details WHERE session_id=$1`, session).Scan(&detailCancelled); err != nil || !detailCancelled {
		t.Fatalf("detail cancelled=%v %v", detailCancelled, err)
	}
	cmd.IdempotencyKey = "stale"
	if _, err := svc.ChangeOccurrence(ctx, cmd); !errors.Is(err, ErrScheduleConflict) {
		t.Fatalf("stale occurrence=%v", err)
	}
	cmd.ExpectedOccurrenceVersion = 1
	cmd.Cancelled = boolPtr(false)
	moved := civilDate(2030, 1, 5)
	cmd.ReplacementLocalDate = &moved
	cmd.IdempotencyKey = "move"
	movePlan := plan
	movePlan.Mode, movePlan.AnchorLocalDate, movePlan.EffectiveLocalDate = ModeSelectedDates, moved, moved
	movePlan.SelectedDates, movePlan.IntervalCount, movePlan.IntervalUnit = []time.Time{moved}, 0, ""
	warnings, err := svc.overlap.Warnings(ctx, pool, teacher, circle, movePlan, OccurrenceKey(sch.ID, cmd.OriginalLocalDate))
	if err != nil {
		t.Fatal(err)
	}
	cmd.ConfirmOverlaps = len(warnings.Warnings) > 0
	for _, warning := range warnings.Warnings {
		cmd.ConfirmedWarningIDs = append(cmd.ConfirmedWarningIDs, warning.WarningID)
	}
	movedEx, err := svc.ChangeOccurrence(ctx, cmd)
	if err != nil || movedEx.Version != 2 || !movedEx.OriginalLocalDate.Equal(civilDate(2030, 1, 2)) {
		t.Fatalf("move=%+v %v", movedEx, err)
	}
	var scheduled time.Time
	// The plan resolves in Asia/Riyadh (UTC+3): 22:30 local is 19:30 UTC.
	riyadh, err := LoadLocation("Asia/Riyadh")
	if err != nil {
		t.Fatal(err)
	}
	wantStart := ResolveStartUTC(riyadh, moved, LocalClock{Hour: 22, Minute: 30})
	if err := pool.QueryRow(ctx, `SELECT scheduled_at FROM sessions WHERE id=$1`, session).Scan(&scheduled); err != nil || !scheduled.Equal(wantStart) {
		t.Fatalf("materialized move=%v %v, want %v", scheduled, err, wantStart)
	}
	change := ChangeScheduleCommand{ActorID: teacher, CircleID: circle, ScheduleID: sch.ID, IdempotencyKey: "series", ExpectedVersion: 1, EffectiveLocalDate: civilDate(2030, 1, 2), Plan: plan}
	change.Plan.Title = "new series"
	if _, err := svc.Change(ctx, change); err != nil {
		t.Fatal(err)
	}
	var title string
	if err := pool.QueryRow(ctx, `SELECT title,cancelled_at IS NOT NULL FROM planned_session_details WHERE session_id=$1`, session).Scan(&title, &detailCancelled); err != nil || title != "new series" || detailCancelled {
		t.Fatalf("superseded detail=%q %v %v", title, detailCancelled, err)
	}
	exceptions, err := repo.ListExceptions(ctx, sch.ID)
	if err != nil || len(exceptions) != 0 {
		t.Fatalf("superseded exceptions=%+v %v", exceptions, err)
	}
	// Two different keys with the same expected version cannot both win.
	var wg sync.WaitGroup
	errs := make(chan error, 2)
	for _, key := range []string{"race-a", "race-b"} {
		wg.Add(1)
		go func(key string) {
			defer wg.Done()
			c := change
			c.IdempotencyKey = key
			c.ExpectedVersion = 2
			_, e := svc.Change(ctx, c)
			errs <- e
		}(key)
	}
	wg.Wait()
	close(errs)
	success, conflict := 0, 0
	for err := range errs {
		if err == nil {
			success++
		} else if errors.Is(err, ErrScheduleConflict) {
			conflict++
		} else {
			t.Fatal(err)
		}
	}
	if success != 1 || conflict != 1 {
		t.Fatalf("race success/conflict=%d/%d", success, conflict)
	}
}

func boolPtr(v bool) *bool { return &v }

// OccurrenceView resolves the calendar projection against real tables: the
// circle name, the governing plan, the resolved UTC interval, the materialized
// session id and the cancellation state, with membership and identity checks.
func TestScheduleService_OccurrenceViewProjection(t *testing.T) {
	ctx := context.Background()
	pool := openRepoTestPool(t, ctx)
	teacher := "11111111-1111-1111-1111-111111111111"
	student := "22222222-2222-2222-2222-222222222222"
	outsider := "66666666-6666-6666-6666-666666666666"
	circle := "33333333-3333-3333-3333-333333333333"
	for _, id := range []string{teacher, student, outsider} {
		seedRepoUser(t, ctx, pool, id)
	}
	seedRepoCircle(t, ctx, pool, circle, teacher, "view-circle")
	for _, member := range []struct{ id, role string }{{teacher, "teacher"}, {student, "student"}} {
		if _, err := pool.Exec(ctx, `INSERT INTO circle_members(circle_id,user_id,role) VALUES($1,$2,$3)`, circle, member.id, member.role); err != nil {
			t.Fatal(err)
		}
	}
	svc := NewScheduleService(NewScheduleRepository(pool))
	svc.now = func() time.Time { return civilDate(2030, 1, 1) }
	plan := intervalRecord(civilDate(2030, 1, 2), 1, IntervalUnitDay)
	sch, err := svc.Create(ctx, CreateScheduleCommand{ActorID: teacher, CircleID: circle, IdempotencyKey: "create", Plan: plan})
	if err != nil {
		t.Fatal(err)
	}

	// A virtual occurrence resolves from the plan alone, with no session id.
	view, err := svc.OccurrenceView(ctx, circle, sch.ID, student, civilDate(2030, 1, 2))
	if err != nil {
		t.Fatal(err)
	}
	wantStart := time.Date(2030, 1, 2, 19, 30, 0, 0, time.UTC) // Asia/Riyadh 22:30
	if view.CircleName != "Repo Circle" || view.Title != "Interval Circle" || view.Timezone != "Asia/Riyadh" {
		t.Fatalf("view identity=%+v", view)
	}
	if !view.StartsAt.Equal(wantStart) || !view.EndsAt.Equal(wantStart.Add(90*time.Minute)) {
		t.Fatalf("view interval=%v-%v", view.StartsAt, view.EndsAt)
	}
	if view.SessionID != nil || view.Cancelled || view.ScheduleID != sch.ID || view.CircleID != circle {
		t.Fatalf("view state=%+v", view)
	}
	if !view.OriginalLocalDate.Equal(civilDate(2030, 1, 2)) {
		t.Fatalf("view original date=%v", view.OriginalLocalDate)
	}

	// A materialized occurrence exposes its F-005 session id.
	session := "44444444-4444-4444-4444-444444444444"
	seedStartedMaterialization(t, ctx, pool, session, sch.ID, circle, teacher, civilDate(2030, 1, 5))
	materialized, err := svc.OccurrenceView(ctx, circle, sch.ID, teacher, civilDate(2030, 1, 5))
	if err != nil {
		t.Fatal(err)
	}
	if materialized.SessionID == nil || *materialized.SessionID != session {
		t.Fatalf("materialized session=%+v", materialized.SessionID)
	}

	// Cancellation flips the projection state.
	if _, err := svc.ChangeOccurrence(ctx, ChangeOccurrenceCommand{ActorID: teacher, CircleID: circle, ScheduleID: sch.ID, IdempotencyKey: "cancel",
		OriginalLocalDate: civilDate(2030, 1, 2), ExpectedSeriesVersion: 1, Cancelled: boolPtr(true)}); err != nil {
		t.Fatal(err)
	}
	cancelled, err := svc.OccurrenceView(ctx, circle, sch.ID, student, civilDate(2030, 1, 2))
	if err != nil || !cancelled.Cancelled {
		t.Fatalf("cancelled view=%+v %v", cancelled, err)
	}

	// Identity and membership boundaries hold.
	if _, err := svc.OccurrenceView(ctx, circle, sch.ID, outsider, civilDate(2030, 1, 2)); !errors.Is(err, ErrCircleNotFoundOrDenied) {
		t.Fatalf("outsider view=%v", err)
	}
	if _, err := svc.OccurrenceView(ctx, circle, sch.ID, student, civilDate(2030, 1, 1)); !errors.Is(err, ErrOccurrenceNotFound) {
		t.Fatalf("pre-anchor view=%v", err)
	}
	if _, err := svc.OccurrenceView(ctx, circle, "77777777-7777-7777-7777-777777777777", student, civilDate(2030, 1, 2)); !errors.Is(err, ErrScheduleNotFound) {
		t.Fatalf("unknown schedule view=%v", err)
	}
}

// A stop or series change supersedes an individual edit by the occurrence's
// effective (moved) date, not just its original date: a move into the
// superseded range is removed, a move out of it is retained and stays
// editable, and no move may resurrect an occurrence inside a stopped range.
func TestScheduleService_MovedExceptionsAcrossStopBoundary(t *testing.T) {
	ctx := context.Background()
	pool := openRepoTestPool(t, ctx)
	teacher := "11111111-1111-1111-1111-111111111111"
	circle := "33333333-3333-3333-3333-333333333333"
	seedRepoUser(t, ctx, pool, teacher)
	seedRepoCircle(t, ctx, pool, circle, teacher, "move-circle")
	if _, err := pool.Exec(ctx, `INSERT INTO circle_members(circle_id,user_id,role) VALUES($1,$2,'teacher')`, circle, teacher); err != nil {
		t.Fatal(err)
	}
	repo := NewScheduleRepository(pool)
	svc := NewScheduleService(repo)
	svc.now = func() time.Time { return civilDate(2030, 1, 1) }
	plan := intervalRecord(civilDate(2030, 1, 2), 1, IntervalUnitDay)
	sch, err := svc.Create(ctx, CreateScheduleCommand{ActorID: teacher, CircleID: circle, IdempotencyKey: "create", Plan: plan})
	if err != nil {
		t.Fatal(err)
	}
	move := func(key string, original, replacement time.Time) error {
		cmd := ChangeOccurrenceCommand{ActorID: teacher, CircleID: circle, ScheduleID: sch.ID, IdempotencyKey: key,
			OriginalLocalDate: original, ExpectedSeriesVersion: 1, ReplacementLocalDate: &replacement}
		candidate := selectedDatesRecord(replacement, replacement)
		candidate.StartLocalTime, candidate.EndLocalTime = plan.StartLocalTime, plan.EndLocalTime
		candidate.DurationMinutes, candidate.Timezone = plan.DurationMinutes, plan.Timezone
		preview, err := svc.overlap.Preview(ctx, teacher, circle, candidate)
		if err != nil {
			return err
		}
		if len(preview.Warnings) > 0 {
			cmd.ConfirmOverlaps = true
			for _, warning := range preview.Warnings {
				cmd.ConfirmedWarningIDs = append(cmd.ConfirmedWarningIDs, warning.WarningID)
			}
		}
		_, err = svc.ChangeOccurrence(ctx, cmd)
		return err
	}
	// Occurrence moved ahead of the stop boundary must survive the stop.
	if err := move("move-kept", civilDate(2030, 1, 10), civilDate(2030, 1, 6)); err != nil {
		t.Fatalf("move kept: %v", err)
	}
	// Occurrence moved into the stopped range must be superseded by the stop.
	if err := move("move-dropped", civilDate(2030, 1, 6), civilDate(2030, 1, 19)); err != nil {
		t.Fatalf("move dropped: %v", err)
	}
	stopped, err := svc.Change(ctx, ChangeScheduleCommand{ActorID: teacher, CircleID: circle, ScheduleID: sch.ID, IdempotencyKey: "stop",
		ExpectedVersion: 1, EffectiveLocalDate: civilDate(2030, 1, 8), Stop: true})
	if err != nil || stopped.CurrentVersion != 2 {
		t.Fatalf("stop=%+v %v", stopped, err)
	}
	exceptions, err := repo.ListExceptions(ctx, sch.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(exceptions) != 1 || !exceptions[0].OriginalLocalDate.Equal(civilDate(2030, 1, 10)) ||
		exceptions[0].ReplacementLocalDate == nil || !exceptions[0].ReplacementLocalDate.Equal(civilDate(2030, 1, 6)) {
		t.Fatalf("exceptions after stop=%+v, want only the kept move", exceptions)
	}
	// The retained moved occurrence stays editable across the stop boundary.
	cancelled, err := svc.ChangeOccurrence(ctx, ChangeOccurrenceCommand{ActorID: teacher, CircleID: circle, ScheduleID: sch.ID, IdempotencyKey: "cancel-kept",
		OriginalLocalDate: civilDate(2030, 1, 10), ExpectedSeriesVersion: 2, ExpectedOccurrenceVersion: 1, Cancelled: boolPtr(true)})
	if err != nil || cancelled.CancelledAt == nil {
		t.Fatalf("cancel kept=%+v %v", cancelled, err)
	}
	uncancel := ChangeOccurrenceCommand{ActorID: teacher, CircleID: circle, ScheduleID: sch.ID, IdempotencyKey: "uncancel-kept",
		OriginalLocalDate: civilDate(2030, 1, 10), ExpectedSeriesVersion: 2, ExpectedOccurrenceVersion: cancelled.Version, Cancelled: boolPtr(false)}
	if _, err := svc.ChangeOccurrence(ctx, uncancel); err == nil {
		t.Fatal("restoring an overlapping occurrence succeeded without confirmation")
	} else {
		var overlapErr *OverlapConfirmationError
		if !errors.As(err, &overlapErr) || len(overlapErr.Warnings) == 0 {
			t.Fatalf("restore overlap error=%v, want refreshed overlap warnings", err)
		}
	}
	// A move may not resurrect an occurrence inside the stopped range.
	moveIntoStop := ChangeOccurrenceCommand{ActorID: teacher, CircleID: circle, ScheduleID: sch.ID, IdempotencyKey: "move-into-stop",
		OriginalLocalDate: civilDate(2030, 1, 7), ExpectedSeriesVersion: 2, ReplacementLocalDate: ptrDate(civilDate(2030, 1, 9))}
	if _, err := svc.ChangeOccurrence(ctx, moveIntoStop); !errors.Is(err, ErrInvalidScheduleCommand) {
		t.Fatalf("move into stopped range=%v", err)
	}
}

// A series change supersedes a move whose target lands in the changed range,
// even when the occurrence's original date predates the change.
func TestScheduleService_SeriesChangeSupersedesMovedExceptions(t *testing.T) {
	ctx := context.Background()
	pool := openRepoTestPool(t, ctx)
	teacher := "11111111-1111-1111-1111-111111111111"
	circle := "33333333-3333-3333-3333-333333333333"
	seedRepoUser(t, ctx, pool, teacher)
	seedRepoCircle(t, ctx, pool, circle, teacher, "series-move-circle")
	if _, err := pool.Exec(ctx, `INSERT INTO circle_members(circle_id,user_id,role) VALUES($1,$2,'teacher')`, circle, teacher); err != nil {
		t.Fatal(err)
	}
	repo := NewScheduleRepository(pool)
	svc := NewScheduleService(repo)
	svc.now = func() time.Time { return civilDate(2030, 1, 1) }
	plan := intervalRecord(civilDate(2030, 1, 2), 1, IntervalUnitDay)
	sch, err := svc.Create(ctx, CreateScheduleCommand{ActorID: teacher, CircleID: circle, IdempotencyKey: "create", Plan: plan})
	if err != nil {
		t.Fatal(err)
	}
	move := func(key string, original, replacement time.Time) {
		cmd := ChangeOccurrenceCommand{ActorID: teacher, CircleID: circle, ScheduleID: sch.ID, IdempotencyKey: key,
			OriginalLocalDate: original, ExpectedSeriesVersion: 1, ReplacementLocalDate: &replacement}
		movePlan := plan
		movePlan.Mode, movePlan.AnchorLocalDate, movePlan.EffectiveLocalDate = ModeSelectedDates, replacement, replacement
		movePlan.SelectedDates, movePlan.IntervalCount, movePlan.IntervalUnit = []time.Time{replacement}, 0, ""
		warnings, err := svc.overlap.Warnings(ctx, pool, teacher, circle, movePlan, OccurrenceKey(sch.ID, original))
		if err != nil {
			t.Fatalf("preview move %s: %v", key, err)
		}
		cmd.ConfirmOverlaps = len(warnings.Warnings) > 0
		for _, warning := range warnings.Warnings {
			cmd.ConfirmedWarningIDs = append(cmd.ConfirmedWarningIDs, warning.WarningID)
		}
		if _, err := svc.ChangeOccurrence(ctx, cmd); err != nil {
			t.Fatalf("move %s: %v", key, err)
		}
	}
	move("move-into-change", civilDate(2030, 1, 6), civilDate(2030, 1, 20))   // target in changed range
	move("move-before-change", civilDate(2030, 1, 10), civilDate(2030, 1, 5)) // target before effective date
	change := ChangeScheduleCommand{ActorID: teacher, CircleID: circle, ScheduleID: sch.ID, IdempotencyKey: "series",
		ExpectedVersion: 1, EffectiveLocalDate: civilDate(2030, 1, 15), Plan: plan}
	change.Plan.Title = "new series"
	if _, err := svc.Change(ctx, change); err != nil {
		t.Fatal(err)
	}
	exceptions, err := repo.ListExceptions(ctx, sch.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(exceptions) != 1 || !exceptions[0].OriginalLocalDate.Equal(civilDate(2030, 1, 10)) {
		t.Fatalf("exceptions after series change=%+v, want only the pre-boundary move", exceptions)
	}
}

func ptrDate(v time.Time) *time.Time { return &v }
