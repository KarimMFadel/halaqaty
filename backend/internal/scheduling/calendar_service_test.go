//go:build integration

package scheduling

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

func TestCalendarService_MonthUsesStoredViewerTimezoneAcrossCircles(t *testing.T) {
	ctx := context.Background()
	pool := openRepoTestPool(t, ctx)
	actor := "11111111-1111-1111-1111-111111111111"
	circleA := "22222222-2222-2222-2222-222222222222"
	circleB := "33333333-3333-3333-3333-333333333333"
	seedRepoUser(t, ctx, pool, actor)
	seedRepoCircle(t, ctx, pool, circleA, actor, "HLQ-CAL001")
	seedRepoCircle(t, ctx, pool, circleB, actor, "HLQ-CAL002")
	seedCalendarMember(t, ctx, pool, circleA, actor)
	seedCalendarMember(t, ctx, pool, circleB, actor)
	seedCalendarTimezone(t, ctx, pool, actor, "Africa/Cairo")

	// UTC timestamps straddling Cairo's local January boundary prove that the
	// requested month and returned display times use the stored viewer zone.
	janStart := time.Date(2029, 12, 31, 22, 30, 0, 0, time.UTC)
	janSecondCircle := time.Date(2030, 1, 20, 12, 0, 0, 0, time.UTC)
	febStart := time.Date(2030, 1, 31, 22, 30, 0, 0, time.UTC)
	seedCalendarOneOff(t, ctx, pool, "44444444-4444-4444-4444-444444444441", circleA, actor, janStart, janStart.Add(time.Hour), "January A", "UTC", false)
	seedCalendarOneOff(t, ctx, pool, "44444444-4444-4444-4444-444444444442", circleB, actor, janSecondCircle, janSecondCircle.Add(time.Hour), "January B", "UTC", false)
	seedCalendarOneOff(t, ctx, pool, "44444444-4444-4444-4444-444444444443", circleA, actor, febStart, febStart.Add(time.Hour), "February", "UTC", false)

	month, err := NewCalendarService(NewScheduleRepository(pool)).Month(ctx, actor, "2030-01")
	if err != nil {
		t.Fatalf("read January calendar: %v", err)
	}
	if len(month.Items) != 2 {
		t.Fatalf("January items = %d, want two across both circles: %+v", len(month.Items), month.Items)
	}
	gotCircles := map[string]bool{}
	for _, item := range month.Items {
		gotCircles[item.CircleID] = true
		if item.SessionID == nil || *item.SessionID != item.OccurrenceKey {
			t.Errorf("one-off %q session_id = %v, want its session UUID", item.OccurrenceKey, item.SessionID)
		}
		if item.StartsAt.In(mustLoadCalendarLocation(t, "Africa/Cairo")).Month() != time.January {
			t.Errorf("item %q is outside January in stored viewer timezone: %v", item.OccurrenceKey, item.StartsAt)
		}
		if item.PlanningTimezone != "UTC" {
			t.Errorf("item %q planning timezone = %q, want UTC", item.OccurrenceKey, item.PlanningTimezone)
		}
	}
	if !gotCircles[circleA] || !gotCircles[circleB] {
		t.Fatalf("calendar circle IDs = %v, want %s and %s", gotCircles, circleA, circleB)
	}
}

func TestCalendarService_MonthAllowsArchivedHistoryAndExcludesFormerMembers(t *testing.T) {
	ctx := context.Background()
	pool := openRepoTestPool(t, ctx)
	actor := "11111111-1111-1111-1111-111111111111"
	archivedCircle := "22222222-2222-2222-2222-222222222222"
	revokedCircle := "33333333-3333-3333-3333-333333333333"
	seedRepoUser(t, ctx, pool, actor)
	seedRepoCircle(t, ctx, pool, archivedCircle, actor, "HLQ-CAL003")
	seedRepoCircle(t, ctx, pool, revokedCircle, actor, "HLQ-CAL004")
	seedCalendarMember(t, ctx, pool, archivedCircle, actor)
	seedCalendarMember(t, ctx, pool, revokedCircle, actor)
	seedCalendarTimezone(t, ctx, pool, actor, "UTC")
	if _, err := pool.Exec(ctx, `UPDATE circles SET is_archived=TRUE WHERE id=$1::uuid`, archivedCircle); err != nil {
		t.Fatalf("archive circle: %v", err)
	}
	if _, err := pool.Exec(ctx, `DELETE FROM circle_members WHERE circle_id=$1::uuid AND user_id=$2::uuid`, revokedCircle, actor); err != nil {
		t.Fatalf("revoke membership: %v", err)
	}
	start := time.Date(2029, 1, 4, 10, 0, 0, 0, time.UTC)
	seedCalendarOneOff(t, ctx, pool, "44444444-4444-4444-4444-444444444451", archivedCircle, actor, start, start.Add(time.Hour), "Completed history", "UTC", false)
	seedCalendarOneOff(t, ctx, pool, "44444444-4444-4444-4444-444444444452", archivedCircle, actor, start.AddDate(0, 0, 1), start.AddDate(0, 0, 1).Add(time.Hour), "Cancelled history", "UTC", true)
	seedCalendarEnded(t, ctx, pool, "44444444-4444-4444-4444-444444444451", start.Add(time.Hour))
	revokedStart := start.AddDate(0, 0, 2)
	seedCalendarOneOff(t, ctx, pool, "44444444-4444-4444-4444-444444444453", revokedCircle, actor, revokedStart, revokedStart.Add(time.Hour), "Former member", "UTC", false)

	month, err := NewCalendarService(NewScheduleRepository(pool)).Month(ctx, actor, "2029-01")
	if err != nil {
		t.Fatalf("read archived history: %v", err)
	}
	if len(month.Items) != 2 {
		t.Fatalf("history items = %d, want completed and cancelled archived history only: %+v", len(month.Items), month.Items)
	}
	states := map[string]bool{}
	for _, item := range month.Items {
		if item.CircleID != archivedCircle {
			t.Errorf("item leaked from inaccessible circle: %+v", item)
		}
		states[item.State] = true
	}
	if !states["completed"] || !states["cancelled"] {
		t.Fatalf("archived history states = %v, want completed and cancelled", states)
	}
}

func TestCalendarService_MonthIncludesLongPriorOccurrenceAndJumpsDistantAnchor(t *testing.T) {
	ctx := context.Background()
	pool := openRepoTestPool(t, ctx)
	actor := "11111111-1111-1111-1111-111111111111"
	circle := "22222222-2222-2222-2222-222222222222"
	seedRepoUser(t, ctx, pool, actor)
	seedRepoCircle(t, ctx, pool, circle, actor, "HLQ-CAL005")
	seedCalendarMember(t, ctx, pool, circle, actor)
	seedCalendarTimezone(t, ctx, pool, actor, "UTC")

	longStart := time.Date(2029, 12, 15, 10, 0, 0, 0, time.UTC)
	seedCalendarOneOff(t, ctx, pool, "44444444-4444-4444-4444-444444444461", circle, actor, longStart, longStart.Add(31*24*time.Hour), "Thirty-one days", "UTC", false)

	anchor := civilDate(1900, 1, 1)
	schedule, err := NewScheduleRepository(pool).CreateSchedule(ctx, circle, actor, intervalRecord(anchor, 1, IntervalUnitDay))
	if err != nil {
		t.Fatalf("create distant daily recurrence: %v", err)
	}

	month, err := NewCalendarService(NewScheduleRepository(pool)).Month(ctx, actor, "2030-01")
	if err != nil {
		t.Fatalf("read January calendar with distant anchor: %v", err)
	}
	if len(month.Items) != 32 {
		t.Fatalf("January items = %d, want 31 daily occurrences plus the December-starting 31-day item", len(month.Items))
	}
	longFound, firstDistant, lastDistant := false, false, false
	for _, item := range month.Items {
		if item.OccurrenceKey == "44444444-4444-4444-4444-444444444461" {
			longFound = item.StartsAt.Equal(longStart)
		}
		if item.OccurrenceKey == fmt.Sprintf("%s:2030-01-01", schedule.ID) {
			firstDistant = true
		}
		if item.OccurrenceKey == fmt.Sprintf("%s:2030-01-31", schedule.ID) {
			lastDistant = true
		}
	}
	if !longFound || !firstDistant || !lastDistant {
		t.Fatalf("long prior item=%v, first distant=%v, last distant=%v", longFound, firstDistant, lastDistant)
	}
}

func TestCalendarService_MonthKeepsMovedIdentityAndPastCancelledOccurrence(t *testing.T) {
	ctx := context.Background()
	pool := openRepoTestPool(t, ctx)
	actor := "11111111-1111-1111-1111-111111111111"
	circle := "22222222-2222-2222-2222-222222222222"
	seedRepoUser(t, ctx, pool, actor)
	seedRepoCircle(t, ctx, pool, circle, actor, "HLQ-CAL006")
	seedCalendarMember(t, ctx, pool, circle, actor)
	seedCalendarTimezone(t, ctx, pool, actor, "UTC")
	anchor := civilDate(2025, 1, 1)
	repo := NewScheduleRepository(pool)
	schedule, err := repo.CreateSchedule(ctx, circle, actor, weekdayRecord(anchor, int(anchor.Weekday())))
	if err != nil {
		t.Fatalf("create Wednesday schedule: %v", err)
	}
	original, moved := civilDate(2025, 1, 8), civilDate(2025, 1, 14)
	start, end, duration := LocalClock{Hour: 11}, LocalClock{Hour: 12}, 60
	if _, err := repo.UpsertException(ctx, OccurrenceException{
		ScheduleID: schedule.ID, OriginalLocalDate: original, ReplacementLocalDate: &moved,
		ReplacementStartLocalTime: &start, ReplacementEndLocalTime: &end,
		ReplacementDurationMinutes: &duration, UpdatedBy: actor,
	}); err != nil {
		t.Fatalf("move occurrence: %v", err)
	}
	cancelledDate := civilDate(2025, 1, 15)
	cancelledAt := time.Date(2025, 1, 14, 9, 0, 0, 0, time.UTC)
	if _, err := repo.UpsertException(ctx, OccurrenceException{
		ScheduleID: schedule.ID, OriginalLocalDate: cancelledDate,
		CancelledAt: &cancelledAt, UpdatedBy: actor,
	}); err != nil {
		t.Fatalf("cancel occurrence: %v", err)
	}

	month, err := NewCalendarService(repo).Month(ctx, actor, "2025-01")
	if err != nil {
		t.Fatalf("read January calendar: %v", err)
	}
	movedKey := fmt.Sprintf("%s:2025-01-08", schedule.ID)
	var movedItem, cancelledItem *CalendarItem
	for i := range month.Items {
		item := &month.Items[i]
		if item.OccurrenceKey == movedKey {
			movedItem = item
		}
		if item.OccurrenceKey == fmt.Sprintf("%s:2025-01-15", schedule.ID) {
			cancelledItem = item
		}
	}
	if movedItem == nil || movedItem.StartsAt.Day() != 14 || movedItem.StartsAt.Hour() != 11 {
		t.Fatalf("moved occurrence = %+v, want original key %q on Jan 14 at 11:00", movedItem, movedKey)
	}
	if cancelledItem == nil || cancelledItem.State != "cancelled" {
		t.Fatalf("past cancelled occurrence = %+v, want retained cancelled history", cancelledItem)
	}
}

func seedCalendarMember(t *testing.T, ctx context.Context, pool *pgxpool.Pool, circleID, userID string) {
	t.Helper()
	if _, err := pool.Exec(ctx, `INSERT INTO circle_members(circle_id,user_id,role) VALUES($1::uuid,$2::uuid,'student')`, circleID, userID); err != nil {
		t.Fatalf("seed calendar membership: %v", err)
	}
}

func seedCalendarTimezone(t *testing.T, ctx context.Context, pool *pgxpool.Pool, userID, timezone string) {
	t.Helper()
	if _, err := pool.Exec(ctx, `INSERT INTO profiles(user_id,timezone) VALUES($1::uuid,$2) ON CONFLICT (user_id) DO UPDATE SET timezone=EXCLUDED.timezone`, userID, timezone); err != nil {
		t.Fatalf("seed profile timezone: %v", err)
	}
}

func seedCalendarOneOff(t *testing.T, ctx context.Context, pool *pgxpool.Pool, sessionID, circleID, creatorID string, startsAt, endsAt time.Time, title, timezone string, cancelled bool) {
	t.Helper()
	if _, err := pool.Exec(ctx, `INSERT INTO sessions(id,circle_id,created_by,status,scheduled_at) VALUES($1::uuid,$2::uuid,$3::uuid,'scheduled',$4)`, sessionID, circleID, creatorID, startsAt); err != nil {
		t.Fatalf("seed calendar session: %v", err)
	}
	var cancelledAt any
	if cancelled {
		cancelledAt = startsAt.Add(-time.Hour)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO planned_session_details(session_id,title,start_local_time,end_local_time,duration_minutes,planned_end_at,planning_timezone,cancelled_at,version)
		VALUES($1::uuid,$2,$3::time,$4::time,$5,$6,$7,$8,1)`,
		sessionID, title, startsAt.Format("15:04"), endsAt.Format("15:04"), int(endsAt.Sub(startsAt).Minutes()), endsAt, timezone, cancelledAt); err != nil {
		t.Fatalf("seed calendar plan detail: %v", err)
	}
}

func seedCalendarEnded(t *testing.T, ctx context.Context, pool *pgxpool.Pool, sessionID string, endedAt time.Time) {
	t.Helper()
	if _, err := pool.Exec(ctx, `UPDATE sessions SET status='ended',actual_start=scheduled_at,actual_end=$2 WHERE id=$1::uuid`, sessionID, endedAt); err != nil {
		t.Fatalf("mark calendar session ended: %v", err)
	}
}

func mustLoadCalendarLocation(t *testing.T, name string) *time.Location {
	t.Helper()
	location, err := time.LoadLocation(name)
	if err != nil {
		t.Fatalf("load timezone %q: %v", name, err)
	}
	return location
}
