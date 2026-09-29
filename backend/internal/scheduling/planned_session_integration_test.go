//go:build integration

package scheduling

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

func TestPlannedSessionService_CreateIsFutureOnlyAndDurationIsInformational(t *testing.T) {
	ctx := context.Background()
	pool := openRepoTestPool(t, ctx)
	svc, teacher, circle := newPlannedSessionTestService(t, ctx, pool)
	svc.now = func() time.Time { return time.Date(2030, 1, 1, 9, 0, 0, 0, time.UTC) }

	plan := plannedSessionTestPlan(time.Date(2030, 1, 2, 0, 0, 0, 0, time.UTC), 300, "")
	created, err := svc.Create(ctx, CreatePlannedSessionCommand{
		ActorID: teacher, CircleID: circle, IdempotencyKey: "create-long-plan", Plan: plan,
	})
	if err != nil {
		t.Fatalf("create future one-off: %v", err)
	}
	if created.Title != defaultScheduleTitle {
		t.Fatalf("default title = %q, want %q", created.Title, defaultScheduleTitle)
	}
	if created.EndsAt.Sub(created.StartsAt) != 300*time.Minute {
		t.Fatalf("planned interval = %s, want 5 hours", created.EndsAt.Sub(created.StartsAt))
	}

	var status string
	var title string
	var scheduleID, originalDate *string
	var scheduledAt, actualEnd *time.Time
	var duration int
	err = pool.QueryRow(ctx, `
		SELECT s.status, s.scheduled_at, s.actual_end, d.duration_minutes,
		       d.title, d.schedule_id::text, d.original_local_date::text
		FROM sessions s JOIN planned_session_details d ON d.session_id = s.id
		WHERE s.id = $1::uuid`, created.ID).Scan(&status, &scheduledAt, &actualEnd, &duration, &title, &scheduleID, &originalDate)
	if err != nil {
		t.Fatalf("read created one-off: %v", err)
	}
	if scheduledAt == nil || !scheduledAt.Equal(created.StartsAt) || status != "scheduled" || actualEnd != nil || duration != 300 || title != defaultScheduleTitle || scheduleID != nil || originalDate != nil {
		t.Fatalf("planned state = status %q, scheduled_at %v, actual_end %v, duration %d, title %q, schedule_id %v, original_date %v", status, scheduledAt, actualEnd, duration, title, scheduleID, originalDate)
	}
}

func TestPlannedSessionService_CreateRejectsPastStart(t *testing.T) {
	ctx := context.Background()
	pool := openRepoTestPool(t, ctx)
	svc, teacher, circle := newPlannedSessionTestService(t, ctx, pool)
	now := time.Date(2030, 1, 2, 10, 0, 0, 0, time.UTC)
	svc.now = func() time.Time { return now }

	plan := plannedSessionTestPlan(time.Date(2030, 1, 2, 0, 0, 0, 0, time.UTC), 60, "Past plan")
	plan.StartLocalTime = LocalClock{Hour: 9}
	plan.EndLocalTime = LocalClock{Hour: 10}
	_, err := svc.Create(ctx, CreatePlannedSessionCommand{
		ActorID: teacher, CircleID: circle, IdempotencyKey: "past-one-off", Plan: plan,
	})
	if !errors.Is(err, ErrPastPlannedTime) {
		t.Fatalf("create past one-off = %v, want ErrPastPlannedTime", err)
	}
}

func TestPlannedSessionService_CreateReplayReturnsSameSession(t *testing.T) {
	ctx := context.Background()
	pool := openRepoTestPool(t, ctx)
	svc, teacher, circle := newPlannedSessionTestService(t, ctx, pool)
	svc.now = func() time.Time { return time.Date(2030, 1, 1, 9, 0, 0, 0, time.UTC) }
	cmd := CreatePlannedSessionCommand{
		ActorID: teacher, CircleID: circle, IdempotencyKey: "retry-one-off",
		Plan: plannedSessionTestPlan(time.Date(2030, 1, 2, 0, 0, 0, 0, time.UTC), 60, "Retry safe"),
	}
	created, err := svc.Create(ctx, cmd)
	if err != nil {
		t.Fatalf("create one-off: %v", err)
	}
	replayed, err := svc.Create(ctx, cmd)
	if err != nil || replayed.ID != created.ID {
		t.Fatalf("replay = %+v, %v; want same session %q", replayed, err, created.ID)
	}
	var sessionCount, detailCount int
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM sessions WHERE circle_id = $1::uuid AND created_by = $2::uuid`, circle, teacher).Scan(&sessionCount); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM planned_session_details WHERE session_id = $1::uuid`, created.ID).Scan(&detailCount); err != nil {
		t.Fatal(err)
	}
	if sessionCount != 1 || detailCount != 1 {
		t.Fatalf("replayed create rows = %d sessions, %d details; want 1 each", sessionCount, detailCount)
	}
	changed := cmd
	changed.Plan.Title = "Different request"
	if _, err := svc.Create(ctx, changed); !errors.Is(err, ErrReplayConflict) {
		t.Fatalf("replay with changed input = %v, want ErrReplayConflict", err)
	}
}

func TestPlannedSessionService_EditAndCancelRequireUnstartedSession(t *testing.T) {
	ctx := context.Background()
	pool := openRepoTestPool(t, ctx)
	svc, teacher, circle := newPlannedSessionTestService(t, ctx, pool)
	svc.now = func() time.Time { return time.Date(2030, 1, 1, 9, 0, 0, 0, time.UTC) }

	created, err := svc.Create(ctx, CreatePlannedSessionCommand{
		ActorID: teacher, CircleID: circle, IdempotencyKey: "editable-one-off",
		Plan: plannedSessionTestPlan(time.Date(2030, 1, 2, 0, 0, 0, 0, time.UTC), 60, "Original"),
	})
	if err != nil {
		t.Fatalf("create editable one-off: %v", err)
	}
	newPlan := plannedSessionTestPlan(time.Date(2030, 1, 3, 0, 0, 0, 0, time.UTC), 90, "Changed")
	updated, err := svc.Change(ctx, ChangePlannedSessionCommand{
		ActorID: teacher, SessionID: created.ID, IdempotencyKey: "edit-one-off", ExpectedVersion: 1, Plan: &newPlan,
	})
	if err != nil || updated.Title != "Changed" || updated.Version != 2 {
		t.Fatalf("edit unstarted one-off = %+v, %v", updated, err)
	}
	cancelled := true
	cancelCmd := ChangePlannedSessionCommand{
		ActorID: teacher, SessionID: created.ID, IdempotencyKey: "cancel-one-off", ExpectedVersion: 2, Cancelled: &cancelled,
	}
	cancelledView, err := svc.Change(ctx, cancelCmd)
	if err != nil || !cancelledView.Cancelled {
		t.Fatalf("cancel unstarted one-off = %+v, %v", cancelledView, err)
	}
	cancelReplay, err := svc.Change(ctx, cancelCmd)
	if err != nil || !cancelReplay.Cancelled || cancelReplay.Version != cancelledView.Version {
		t.Fatalf("cancel replay = %+v, %v; want unchanged cancellation version %d", cancelReplay, err, cancelledView.Version)
	}
	var status string
	var cancelledAt *time.Time
	if err := pool.QueryRow(ctx, `
		SELECT s.status, d.cancelled_at
		FROM sessions s JOIN planned_session_details d ON d.session_id = s.id
		WHERE s.id = $1::uuid`, created.ID).Scan(&status, &cancelledAt); err != nil {
		t.Fatal(err)
	}
	if status != "scheduled" || cancelledAt == nil {
		t.Fatalf("cancellation changed F-005 lifecycle: status=%q cancelled_at=%v", status, cancelledAt)
	}

	active, err := svc.Create(ctx, CreatePlannedSessionCommand{
		ActorID: teacher, CircleID: circle, IdempotencyKey: "active-one-off",
		Plan: plannedSessionTestPlan(time.Date(2030, 1, 4, 0, 0, 0, 0, time.UTC), 60, "Started"),
	})
	if err != nil {
		t.Fatalf("create start-eligibility fixture: %v", err)
	}
	if _, err := pool.Exec(ctx, `UPDATE sessions SET status='active', actual_start=NOW() WHERE id=$1::uuid`, active.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Change(ctx, ChangePlannedSessionCommand{
		ActorID: teacher, SessionID: active.ID, IdempotencyKey: "edit-started-one-off", ExpectedVersion: 1, Plan: &newPlan,
	}); !errors.Is(err, ErrOccurrenceStarted) {
		t.Fatalf("edit started one-off = %v, want ErrOccurrenceStarted", err)
	}
	if _, err := svc.Change(ctx, ChangePlannedSessionCommand{
		ActorID: teacher, SessionID: active.ID, IdempotencyKey: "cancel-started-one-off", ExpectedVersion: 1, Cancelled: &cancelled,
	}); !errors.Is(err, ErrOccurrenceStarted) {
		t.Fatalf("cancel started one-off = %v, want ErrOccurrenceStarted", err)
	}
}

func newPlannedSessionTestService(t *testing.T, ctx context.Context, pool *pgxpool.Pool) (*PlannedSessionService, string, string) {
	t.Helper()
	teacher := "11111111-1111-1111-1111-111111111111"
	circle := "22222222-2222-2222-2222-222222222222"
	seedRepoUser(t, ctx, pool, teacher)
	seedRepoCircle(t, ctx, pool, circle, teacher, "HLQ-PLAN1")
	if _, err := pool.Exec(ctx, `INSERT INTO circle_members(circle_id,user_id,role) VALUES($1::uuid,$2::uuid,'teacher')`, circle, teacher); err != nil {
		t.Fatalf("seed teacher membership: %v", err)
	}
	return NewPlannedSessionService(NewScheduleRepository(pool)), teacher, circle
}

func plannedSessionTestPlan(date time.Time, duration int, title string) PlannedSessionPlan {
	return PlannedSessionPlan{
		LocalDate:       date,
		StartLocalTime:  LocalClock{Hour: 10},
		EndLocalTime:    LocalClock{Hour: 10 + duration/60, Minute: duration % 60},
		DurationMinutes: duration,
		Timezone:        "UTC",
		Title:           title,
	}
}
