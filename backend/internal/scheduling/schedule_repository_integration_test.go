//go:build integration

package scheduling

import (
	"context"
	"errors"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// openRepoTestPool applies the F-006 migration chain into a disposable schema
// and returns a pool whose connections all resolve that schema, so the
// repository under test runs against real tables, checks and indexes.
func openRepoTestPool(t *testing.T, ctx context.Context) *pgxpool.Pool {
	t.Helper()
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		t.Skip("DATABASE_URL is not set")
	}
	schema := uniqueSchemaName(t)

	bootstrap, err := pgxpool.New(ctx, dbURL)
	if err != nil {
		t.Fatalf("open bootstrap pool: %v", err)
	}
	conn := acquireConn(t, bootstrap, ctx)
	createSchema(t, conn, ctx, schema)
	for _, migration := range append(preFeatureMigrations, migration020Up) {
		runMigrationFile(t, conn, ctx, migration)
	}
	conn.Release()
	bootstrap.Close()

	cfg, err := pgxpool.ParseConfig(dbURL)
	if err != nil {
		t.Fatalf("parse pool config: %v", err)
	}
	qualified := pgx.Identifier{schema}.Sanitize()
	cfg.AfterConnect = func(ctx context.Context, c *pgx.Conn) error {
		_, err := c.Exec(ctx, "SET search_path TO "+qualified)
		return err
	}
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatalf("open repo pool: %v", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		t.Fatalf("ping repo pool: %v", err)
	}
	t.Cleanup(func() {
		dropSchema(t, pool, ctx, schema)
		pool.Close()
	})
	return pool
}

func seedRepoUser(t *testing.T, ctx context.Context, pool *pgxpool.Pool, userID string) {
	t.Helper()
	_, err := pool.Exec(ctx,
		`INSERT INTO users (id, firebase_uid, email) VALUES ($1::uuid, $2, $3)`,
		userID, "firebase-"+userID, userID+"@example.com")
	if err != nil {
		t.Fatalf("seed user %s: %v", userID, err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO profiles(user_id, timezone) VALUES($1::uuid, 'UTC')`, userID); err != nil {
		t.Fatalf("seed profile for %s: %v", userID, err)
	}
}

func seedRepoCircle(t *testing.T, ctx context.Context, pool *pgxpool.Pool, circleID, teacherID, inviteCode string) {
	t.Helper()
	_, err := pool.Exec(ctx,
		`INSERT INTO circles (id, name, teacher_id, invite_code) VALUES ($1::uuid, 'Repo Circle', $2::uuid, $3)`,
		circleID, teacherID, inviteCode)
	if err != nil {
		t.Fatalf("seed circle %s: %v", circleID, err)
	}
}

// seedStartedMaterialization records a started F-005 session plus its planned
// detail for one logical occurrence, i.e. retained started history.
func seedStartedMaterialization(t *testing.T, ctx context.Context, pool *pgxpool.Pool, sessionID, scheduleID, circleID, userID string, originalLocalDate time.Time) {
	t.Helper()
	_, err := pool.Exec(ctx,
		`INSERT INTO sessions (id, circle_id, created_by, status, scheduled_at, actual_start)
		 VALUES ($1::uuid, $2::uuid, $3::uuid, 'active', NOW(), NOW())`,
		sessionID, circleID, userID)
	if err != nil {
		t.Fatalf("seed started session: %v", err)
	}
	_, err = pool.Exec(ctx,
		`INSERT INTO planned_session_details (
			session_id, schedule_id, original_local_date, title,
			start_local_time, end_local_time, duration_minutes,
			planned_end_at, planning_timezone, version
		) VALUES ($1::uuid, $2::uuid, $3, 'Circle Session',
			TIME '10:00', TIME '11:00', 60, NOW() + INTERVAL '1 hour', 'UTC', 1)`,
		sessionID, scheduleID, originalLocalDate)
	if err != nil {
		t.Fatalf("seed planned detail: %v", err)
	}
}

// civilDate is shared with recurrence_test.go in this package.

func weekdayRecord(anchor time.Time, weekdays ...int) RevisionRecord {
	return RevisionRecord{
		Revision: Revision{
			Version:            1,
			EffectiveLocalDate: anchor,
			Mode:               ModeWeekdayPattern,
			AnchorLocalDate:    anchor,
			WeekCadence:        1,
			Weekdays:           weekdays,
		},
		Title:           "Weekday Circle",
		StartLocalTime:  LocalClock{Hour: 10, Minute: 0},
		EndLocalTime:    LocalClock{Hour: 11, Minute: 0},
		DurationMinutes: 60,
		Timezone:        "UTC",
	}
}

func intervalRecord(anchor time.Time, count int, unit IntervalUnit) RevisionRecord {
	return RevisionRecord{
		Revision: Revision{
			Version:            1,
			EffectiveLocalDate: anchor,
			Mode:               ModeInterval,
			AnchorLocalDate:    anchor,
			IntervalCount:      count,
			IntervalUnit:       unit,
		},
		Title:           "Interval Circle",
		StartLocalTime:  LocalClock{Hour: 22, Minute: 30},
		EndLocalTime:    LocalClock{Hour: 0, Minute: 0},
		DurationMinutes: 90,
		Timezone:        "Asia/Riyadh",
	}
}

func selectedDatesRecord(anchor time.Time, dates ...time.Time) RevisionRecord {
	return RevisionRecord{
		Revision: Revision{
			Version:            1,
			EffectiveLocalDate: anchor,
			Mode:               ModeSelectedDates,
			AnchorLocalDate:    anchor,
			SelectedDates:      dates,
		},
		Title:           "Selected Dates Circle",
		StartLocalTime:  LocalClock{Hour: 18, Minute: 0},
		EndLocalTime:    LocalClock{Hour: 19, Minute: 30},
		DurationMinutes: 90,
		Timezone:        "UTC",
	}
}

func exceptionDates(exceptions []OccurrenceException) []time.Time {
	dates := make([]time.Time, 0, len(exceptions))
	for _, ex := range exceptions {
		dates = append(dates, ex.OriginalLocalDate)
	}
	return dates
}

func TestScheduleRepository_CreateAndListMultipleEntries(t *testing.T) {
	ctx := context.Background()
	pool := openRepoTestPool(t, ctx)
	repo := NewScheduleRepository(pool)

	userID := "11111111-1111-1111-1111-111111111111"
	circleA := "22222222-2222-2222-2222-222222222222"
	circleB := "33333333-3333-3333-3333-333333333333"
	seedRepoUser(t, ctx, pool, userID)
	seedRepoCircle(t, ctx, pool, circleA, userID, "HLQ-REPOAA")
	seedRepoCircle(t, ctx, pool, circleB, userID, "HLQ-REPOBB")

	anchor := civilDate(2026, 9, 28) // Monday
	weekday := weekdayRecord(anchor, int(anchor.Weekday()), int(anchor.AddDate(0, 0, 2).Weekday()))
	createdWeekday, err := repo.CreateSchedule(ctx, circleA, userID, weekday)
	if err != nil {
		t.Fatalf("create weekday entry: %v", err)
	}
	interval := intervalRecord(anchor, 3, IntervalUnitDay)
	createdInterval, err := repo.CreateSchedule(ctx, circleA, userID, interval)
	if err != nil {
		t.Fatalf("create interval entry: %v", err)
	}
	// Duplicate selected local dates must persist exactly once (FR-001).
	dupe := anchor.AddDate(0, 0, 2)
	selected := selectedDatesRecord(anchor, anchor, dupe, dupe, anchor.AddDate(0, 0, 7))
	createdSelected, err := repo.CreateSchedule(ctx, circleA, userID, selected)
	if err != nil {
		t.Fatalf("create selected-dates entry: %v", err)
	}
	if _, err := repo.CreateSchedule(ctx, circleB, userID, weekdayRecord(anchor, int(anchor.Weekday()))); err != nil {
		t.Fatalf("create other-circle entry: %v", err)
	}

	entries, err := repo.ListSchedules(ctx, circleA)
	if err != nil {
		t.Fatalf("list circle A schedules: %v", err)
	}
	if len(entries) != 3 {
		t.Fatalf("circle A entries: got %d, want 3", len(entries))
	}
	for _, entry := range entries {
		if entry.CircleID != circleA || entry.CurrentVersion != 1 || entry.StoppedFromLocalDate != nil {
			t.Fatalf("unexpected entry projection: %+v", entry)
		}
	}
	other, err := repo.ListSchedules(ctx, circleB)
	if err != nil {
		t.Fatalf("list circle B schedules: %v", err)
	}
	if len(other) != 1 {
		t.Fatalf("circle B entries: got %d, want 1", len(other))
	}

	revisions, err := repo.LoadRevisions(ctx, []string{createdWeekday.ID, createdInterval.ID, createdSelected.ID})
	if err != nil {
		t.Fatalf("load revisions: %v", err)
	}
	if len(revisions) != 3 {
		t.Fatalf("revisions map: got %d schedules, want 3", len(revisions))
	}

	gotWeekday := revisions[createdWeekday.ID][0]
	if gotWeekday.Mode != ModeWeekdayPattern || gotWeekday.WeekCadence != 1 || len(gotWeekday.Weekdays) != 2 {
		t.Fatalf("weekday selectors round-trip: %+v", gotWeekday.Revision)
	}
	if gotWeekday.Title != "Weekday Circle" || gotWeekday.StartLocalTime != (LocalClock{Hour: 10, Minute: 0}) ||
		gotWeekday.EndLocalTime != (LocalClock{Hour: 11, Minute: 0}) || gotWeekday.DurationMinutes != 60 ||
		gotWeekday.Timezone != "UTC" {
		t.Fatalf("weekday display fields round-trip: %+v", gotWeekday)
	}

	gotInterval := revisions[createdInterval.ID][0]
	if gotInterval.Mode != ModeInterval || gotInterval.IntervalCount != 3 || gotInterval.IntervalUnit != IntervalUnitDay {
		t.Fatalf("interval selectors round-trip: %+v", gotInterval.Revision)
	}
	if gotInterval.Timezone != "Asia/Riyadh" || gotInterval.EndLocalTime != (LocalClock{Hour: 0, Minute: 0}) {
		t.Fatalf("overnight interval clocks round-trip: %+v", gotInterval)
	}

	gotSelected := revisions[createdSelected.ID][0]
	if gotSelected.Mode != ModeSelectedDates {
		t.Fatalf("selected mode round-trip: %+v", gotSelected.Revision)
	}
	wantDates := []time.Time{anchor, dupe, anchor.AddDate(0, 0, 7)}
	if len(gotSelected.SelectedDates) != len(wantDates) {
		t.Fatalf("selected dates: got %v, want %v", gotSelected.SelectedDates, wantDates)
	}
	for i, want := range wantDates {
		if !gotSelected.SelectedDates[i].Equal(want) {
			t.Fatalf("selected date %d: got %s, want %s", i, gotSelected.SelectedDates[i], want)
		}
	}

	// An empty title falls back to the approved default (FR-003).
	defaulted := weekdayRecord(anchor, int(anchor.Weekday()))
	defaulted.Title = ""
	createdDefault, err := repo.CreateSchedule(ctx, circleA, userID, defaulted)
	if err != nil {
		t.Fatalf("create defaulted-title entry: %v", err)
	}
	defaultRevisions, err := repo.LoadRevisions(ctx, []string{createdDefault.ID})
	if err != nil {
		t.Fatalf("load defaulted revision: %v", err)
	}
	if got := defaultRevisions[createdDefault.ID][0].Title; got != "Circle Session" {
		t.Fatalf("default title: got %q, want %q", got, "Circle Session")
	}
}

func TestScheduleRepository_ModeSpecificValidation(t *testing.T) {
	ctx := context.Background()
	pool := openRepoTestPool(t, ctx)
	repo := NewScheduleRepository(pool)

	userID := "11111111-1111-1111-1111-111111111111"
	circleID := "22222222-2222-2222-2222-222222222222"
	seedRepoUser(t, ctx, pool, userID)
	seedRepoCircle(t, ctx, pool, circleID, userID, "HLQ-REPOCC")

	anchor := civilDate(2026, 9, 28)
	cases := []struct {
		name   string
		mutate func(*RevisionRecord)
		want   error
	}{
		{"weekday without cadence", func(r *RevisionRecord) { r.WeekCadence = 0 }, ErrInvalidWeekdayPattern},
		{"weekday without anchor weekday", func(r *RevisionRecord) {
			r.Weekdays = []int{(int(anchor.Weekday()) + 1) % 7}
		}, ErrInvalidWeekdayPattern},
		{"interval count zero", func(r *RevisionRecord) {
			*r = intervalRecord(anchor, 0, IntervalUnitDay)
		}, ErrInvalidInterval},
		{"selected dates empty", func(r *RevisionRecord) {
			*r = selectedDatesRecord(anchor)
		}, ErrInvalidSelectedDates},
		{"end clock mismatch", func(r *RevisionRecord) { r.EndLocalTime = LocalClock{Hour: 12, Minute: 0} }, ErrEndClockMismatch},
		{"duration out of range", func(r *RevisionRecord) { r.DurationMinutes = MaxDurationMinutes + 1 }, ErrInvalidDuration},
		{"unknown timezone", func(r *RevisionRecord) { r.Timezone = "Not/AZone" }, ErrInvalidTimezone},
		{"unknown mode", func(r *RevisionRecord) { r.Mode = "bogus" }, ErrUnknownRecurrenceMode},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			record := weekdayRecord(anchor, int(anchor.Weekday()))
			tc.mutate(&record)
			if _, err := repo.CreateSchedule(ctx, circleID, userID, record); !errors.Is(err, tc.want) {
				t.Fatalf("create: got %v, want %v", err, tc.want)
			}
		})
	}

	entries, err := repo.ListSchedules(ctx, circleID)
	if err != nil {
		t.Fatalf("list after rejected creates: %v", err)
	}
	if len(entries) != 0 {
		t.Fatalf("rejected creates left %d schedules behind", len(entries))
	}
}

func TestScheduleRepository_SeriesEditCASConflict(t *testing.T) {
	ctx := context.Background()
	pool := openRepoTestPool(t, ctx)
	repo := NewScheduleRepository(pool)

	userID := "11111111-1111-1111-1111-111111111111"
	circleID := "22222222-2222-2222-2222-222222222222"
	seedRepoUser(t, ctx, pool, userID)
	seedRepoCircle(t, ctx, pool, circleID, userID, "HLQ-REPODD")

	anchor := civilDate(2026, 9, 28)
	schedule, err := repo.CreateSchedule(ctx, circleID, userID, weekdayRecord(anchor, int(anchor.Weekday())))
	if err != nil {
		t.Fatalf("create schedule: %v", err)
	}

	// A stale expected version is a conflict, not a silent overwrite.
	edit := intervalRecord(civilDate(2026, 10, 5), 2, IntervalUnitWeek)
	if _, err := repo.AppendRevision(ctx, schedule.ID, 7, edit); !errors.Is(err, ErrScheduleConflict) {
		t.Fatalf("stale series edit: got %v, want ErrScheduleConflict", err)
	}

	updated, err := repo.AppendRevision(ctx, schedule.ID, 1, edit)
	if err != nil {
		t.Fatalf("series edit: %v", err)
	}
	if updated.CurrentVersion != 2 {
		t.Fatalf("current version after edit: got %d, want 2", updated.CurrentVersion)
	}

	// The earlier revision is retained so past virtual history reproduces.
	revisions, err := repo.LoadRevisions(ctx, []string{schedule.ID})
	if err != nil {
		t.Fatalf("load revisions after edit: %v", err)
	}
	if got := len(revisions[schedule.ID]); got != 2 {
		t.Fatalf("revisions after edit: got %d, want 2", got)
	}
	loaded := make([]Revision, 0, len(revisions[schedule.ID]))
	for _, record := range revisions[schedule.ID] {
		loaded = append(loaded, record.Revision)
	}
	governingPast, ok := SelectRevision(loaded, civilDate(2026, 9, 30))
	if !ok || governingPast.Version != 1 {
		t.Fatalf("past date governed by version %d, want 1", governingPast.Version)
	}
	governingFuture, ok := SelectRevision(loaded, civilDate(2026, 10, 6))
	if !ok || governingFuture.Version != 2 {
		t.Fatalf("future date governed by version %d, want 2", governingFuture.Version)
	}

	// Replaying the same expected version after a successful edit conflicts.
	if _, err := repo.AppendRevision(ctx, schedule.ID, 1, edit); !errors.Is(err, ErrScheduleConflict) {
		t.Fatalf("replayed series edit: got %v, want ErrScheduleConflict", err)
	}

	// Two concurrent edits on the same expected version: exactly one wins.
	var wg sync.WaitGroup
	outcomes := make(chan error, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			race := intervalRecord(civilDate(2026, 10, 12), 1+i, IntervalUnitDay)
			_, err := repo.AppendRevision(ctx, schedule.ID, 2, race)
			outcomes <- err
		}(i)
	}
	wg.Wait()
	close(outcomes)
	succeeded, conflicted := 0, 0
	for err := range outcomes {
		switch {
		case err == nil:
			succeeded++
		case errors.Is(err, ErrScheduleConflict):
			conflicted++
		default:
			t.Fatalf("concurrent edit: unexpected error %v", err)
		}
	}
	if succeeded != 1 || conflicted != 1 {
		t.Fatalf("concurrent edits: %d succeeded, %d conflicted; want 1/1", succeeded, conflicted)
	}
	final, err := repo.GetSchedule(ctx, schedule.ID)
	if err != nil {
		t.Fatalf("get schedule after race: %v", err)
	}
	if final.CurrentVersion != 3 {
		t.Fatalf("current version after race: got %d, want 3", final.CurrentVersion)
	}
	revisions, err = repo.LoadRevisions(ctx, []string{schedule.ID})
	if err != nil {
		t.Fatalf("load revisions after race: %v", err)
	}
	if got := len(revisions[schedule.ID]); got != 3 {
		t.Fatalf("revisions after race: got %d, want 3 (no partial conflict writes)", got)
	}

	if _, err := repo.AppendRevision(ctx, "99999999-9999-9999-9999-999999999999", 1, edit); !errors.Is(err, ErrScheduleNotFound) {
		t.Fatalf("edit missing schedule: got %v, want ErrScheduleNotFound", err)
	}
	if _, err := repo.GetSchedule(ctx, "99999999-9999-9999-9999-999999999999"); !errors.Is(err, ErrScheduleNotFound) {
		t.Fatalf("get missing schedule: got %v, want ErrScheduleNotFound", err)
	}
}

func TestScheduleRepository_SeriesEditSupersedesUnstartedExceptions(t *testing.T) {
	ctx := context.Background()
	pool := openRepoTestPool(t, ctx)
	repo := NewScheduleRepository(pool)

	userID := "11111111-1111-1111-1111-111111111111"
	circleID := "22222222-2222-2222-2222-222222222222"
	seedRepoUser(t, ctx, pool, userID)
	seedRepoCircle(t, ctx, pool, circleID, userID, "HLQ-REPOEE")

	anchor := civilDate(2026, 9, 28)
	schedule, err := repo.CreateSchedule(ctx, circleID, userID, intervalRecord(anchor, 1, IntervalUnitDay))
	if err != nil {
		t.Fatalf("create schedule: %v", err)
	}

	pastDate := civilDate(2026, 9, 29)    // before the new effective date
	futureDate := civilDate(2026, 10, 5)  // unstarted, at/after the new effective date
	startedDate := civilDate(2026, 10, 6) // already materialized and started
	effective := civilDate(2026, 10, 1)

	for _, date := range []time.Time{pastDate, futureDate, startedDate} {
		cancelledAt := time.Now().UTC()
		_, err := repo.UpsertException(ctx, OccurrenceException{
			ScheduleID:        schedule.ID,
			OriginalLocalDate: date,
			CancelledAt:       &cancelledAt,
			UpdatedBy:         userID,
		})
		if err != nil {
			t.Fatalf("insert exception %s: %v", date.Format(time.DateOnly), err)
		}
	}
	seedStartedMaterialization(t, ctx, pool, "55555555-5555-5555-5555-555555555555", schedule.ID, circleID, userID, startedDate)

	edit := weekdayRecord(effective, int(effective.Weekday()))
	if _, err := repo.AppendRevision(ctx, schedule.ID, 1, edit); err != nil {
		t.Fatalf("series edit: %v", err)
	}

	exceptions, err := repo.ListExceptions(ctx, schedule.ID)
	if err != nil {
		t.Fatalf("list exceptions: %v", err)
	}
	gotDates := exceptionDates(exceptions)
	if len(gotDates) != 2 {
		t.Fatalf("exceptions after supersession: got %v, want 2 retained", gotDates)
	}
	if !gotDates[0].Equal(pastDate) || !gotDates[1].Equal(startedDate) {
		t.Fatalf("retained exceptions: got %v, want past %s and started %s", gotDates,
			pastDate.Format(time.DateOnly), startedDate.Format(time.DateOnly))
	}

	// Retained revisions still reproduce past virtual occurrences.
	revisions, err := repo.LoadRevisions(ctx, []string{schedule.ID})
	if err != nil {
		t.Fatalf("load revisions: %v", err)
	}
	if got := len(revisions[schedule.ID]); got != 2 {
		t.Fatalf("revisions retained: got %d, want 2", got)
	}
	loaded := make([]Revision, 0, 2)
	for _, record := range revisions[schedule.ID] {
		loaded = append(loaded, record.Revision)
	}
	window := OccurrenceWindow(loaded, civilDate(2026, 9, 28), civilDate(2026, 9, 30))
	if len(window) == 0 || window[0].Version != 1 {
		t.Fatalf("past window occurrences: got %+v, want version-1 occurrences", window)
	}
}

func TestScheduleRepository_StopScheduleIsolatesEntries(t *testing.T) {
	ctx := context.Background()
	pool := openRepoTestPool(t, ctx)
	repo := NewScheduleRepository(pool)

	userID := "11111111-1111-1111-1111-111111111111"
	circleID := "22222222-2222-2222-2222-222222222222"
	seedRepoUser(t, ctx, pool, userID)
	seedRepoCircle(t, ctx, pool, circleID, userID, "HLQ-REPOFF")

	anchor := civilDate(2026, 9, 28)
	first, err := repo.CreateSchedule(ctx, circleID, userID, intervalRecord(anchor, 1, IntervalUnitDay))
	if err != nil {
		t.Fatalf("create first entry: %v", err)
	}
	second, err := repo.CreateSchedule(ctx, circleID, userID, weekdayRecord(anchor, int(anchor.Weekday())))
	if err != nil {
		t.Fatalf("create second entry: %v", err)
	}

	stopDate := civilDate(2026, 10, 10)
	keptDate := civilDate(2026, 10, 1)
	droppedDate := civilDate(2026, 10, 12)
	for _, date := range []time.Time{keptDate, droppedDate} {
		cancelledAt := time.Now().UTC()
		if _, err := repo.UpsertException(ctx, OccurrenceException{
			ScheduleID:        first.ID,
			OriginalLocalDate: date,
			CancelledAt:       &cancelledAt,
			UpdatedBy:         userID,
		}); err != nil {
			t.Fatalf("insert exception %s: %v", date.Format(time.DateOnly), err)
		}
	}

	// A stale stop is a conflict and changes nothing.
	if _, err := repo.StopSchedule(ctx, first.ID, 9, stopDate); !errors.Is(err, ErrScheduleConflict) {
		t.Fatalf("stale stop: got %v, want ErrScheduleConflict", err)
	}

	stopped, err := repo.StopSchedule(ctx, first.ID, 1, stopDate)
	if err != nil {
		t.Fatalf("stop: %v", err)
	}
	if stopped.StoppedFromLocalDate == nil || !stopped.StoppedFromLocalDate.Equal(stopDate) {
		t.Fatalf("stopped_from_local_date: got %v, want %s", stopped.StoppedFromLocalDate, stopDate.Format(time.DateOnly))
	}
	if stopped.CurrentVersion != 2 {
		t.Fatalf("version after stop: got %d, want 2", stopped.CurrentVersion)
	}

	exceptions, err := repo.ListExceptions(ctx, first.ID)
	if err != nil {
		t.Fatalf("list exceptions after stop: %v", err)
	}
	gotDates := exceptionDates(exceptions)
	if len(gotDates) != 1 || !gotDates[0].Equal(keptDate) {
		t.Fatalf("exceptions after stop: got %v, want only %s", gotDates, keptDate.Format(time.DateOnly))
	}

	// The sibling entry is untouched by the stop.
	other, err := repo.GetSchedule(ctx, second.ID)
	if err != nil {
		t.Fatalf("get sibling entry: %v", err)
	}
	if other.StoppedFromLocalDate != nil || other.CurrentVersion != 1 {
		t.Fatalf("sibling entry changed by stop: %+v", other)
	}
}

func TestScheduleRepository_UpsertExceptionStableIdentity(t *testing.T) {
	ctx := context.Background()
	pool := openRepoTestPool(t, ctx)
	repo := NewScheduleRepository(pool)

	userID := "11111111-1111-1111-1111-111111111111"
	circleID := "22222222-2222-2222-2222-222222222222"
	seedRepoUser(t, ctx, pool, userID)
	seedRepoCircle(t, ctx, pool, circleID, userID, "HLQ-REPOGG")

	anchor := civilDate(2026, 9, 28)
	schedule, err := repo.CreateSchedule(ctx, circleID, userID, intervalRecord(anchor, 1, IntervalUnitDay))
	if err != nil {
		t.Fatalf("create schedule: %v", err)
	}

	original := civilDate(2026, 10, 3)
	movedDate := civilDate(2026, 10, 4)
	start := LocalClock{Hour: 20, Minute: 0}
	end := LocalClock{Hour: 21, Minute: 30}
	duration := 90
	inserted, err := repo.UpsertException(ctx, OccurrenceException{
		ScheduleID:                 schedule.ID,
		OriginalLocalDate:          original,
		ReplacementLocalDate:       &movedDate,
		ReplacementStartLocalTime:  &start,
		ReplacementEndLocalTime:    &end,
		ReplacementDurationMinutes: &duration,
		UpdatedBy:                  userID,
	})
	if err != nil {
		t.Fatalf("insert exception: %v", err)
	}
	if inserted.Version != 1 || inserted.SeriesVersion != 1 {
		t.Fatalf("inserted exception versions: got series=%d version=%d, want 1/1",
			inserted.SeriesVersion, inserted.Version)
	}
	if inserted.ReplacementLocalDate == nil || !inserted.ReplacementLocalDate.Equal(movedDate) {
		t.Fatalf("replacement date round-trip: got %v", inserted.ReplacementLocalDate)
	}
	if inserted.ReplacementStartLocalTime == nil || *inserted.ReplacementStartLocalTime != start ||
		inserted.ReplacementEndLocalTime == nil || *inserted.ReplacementEndLocalTime != end {
		t.Fatalf("replacement clocks round-trip: got %+v", inserted)
	}

	// Updating the same (schedule_id, original_local_date) edits in place:
	// the logical occurrence identity is stable and no second row appears.
	cancelledAt := time.Now().UTC()
	updated, err := repo.UpsertException(ctx, OccurrenceException{
		ScheduleID:        schedule.ID,
		OriginalLocalDate: original,
		CancelledAt:       &cancelledAt,
		UpdatedBy:         userID,
	})
	if err != nil {
		t.Fatalf("update exception to cancellation: %v", err)
	}
	if updated.Version != 2 {
		t.Fatalf("exception version after update: got %d, want 2", updated.Version)
	}
	if updated.CancelledAt == nil {
		t.Fatalf("cancelled_at not persisted: %+v", updated)
	}
	if updated.ReplacementLocalDate != nil || updated.ReplacementStartLocalTime != nil {
		t.Fatalf("cleared replacement fields not persisted: %+v", updated)
	}

	exceptions, err := repo.ListExceptions(ctx, schedule.ID)
	if err != nil {
		t.Fatalf("list exceptions: %v", err)
	}
	if len(exceptions) != 1 || !exceptions[0].OriginalLocalDate.Equal(original) {
		t.Fatalf("stable identity: got %d exceptions, want 1 on %s", len(exceptions), original.Format(time.DateOnly))
	}

	// The series version stamped on the exception follows the schedule CAS.
	if _, err := repo.AppendRevision(ctx, schedule.ID, 1, intervalRecord(civilDate(2026, 10, 1), 2, IntervalUnitDay)); err != nil {
		t.Fatalf("series edit: %v", err)
	}
	// The 2026-10-03 exception is unstarted and at/after the new effective
	// date, so the series change supersedes it; re-insert to check stamping.
	reinserted, err := repo.UpsertException(ctx, OccurrenceException{
		ScheduleID:        schedule.ID,
		OriginalLocalDate: original,
		CancelledAt:       &cancelledAt,
		UpdatedBy:         userID,
	})
	if err != nil {
		t.Fatalf("re-insert exception: %v", err)
	}
	if reinserted.SeriesVersion != 2 || reinserted.Version != 1 {
		t.Fatalf("exception after series edit: got series=%d version=%d, want 2/1",
			reinserted.SeriesVersion, reinserted.Version)
	}
}

func TestScheduleRepository_ExceptionRejectsInconsistentReplacement(t *testing.T) {
	ctx := context.Background()
	pool := openRepoTestPool(t, ctx)
	repo := NewScheduleRepository(pool)

	userID := "11111111-1111-1111-1111-111111111111"
	circleID := "22222222-2222-2222-2222-222222222222"
	seedRepoUser(t, ctx, pool, userID)
	seedRepoCircle(t, ctx, pool, circleID, userID, "HLQ-REPOHH")

	anchor := civilDate(2026, 9, 28)
	schedule, err := repo.CreateSchedule(ctx, circleID, userID, intervalRecord(anchor, 1, IntervalUnitDay))
	if err != nil {
		t.Fatalf("create schedule: %v", err)
	}

	start := LocalClock{Hour: 20, Minute: 0}
	wrongEnd := LocalClock{Hour: 22, Minute: 0} // not start + 90 minutes
	duration := 90
	_, err = repo.UpsertException(ctx, OccurrenceException{
		ScheduleID:                 schedule.ID,
		OriginalLocalDate:          civilDate(2026, 10, 3),
		ReplacementStartLocalTime:  &start,
		ReplacementEndLocalTime:    &wrongEnd,
		ReplacementDurationMinutes: &duration,
		UpdatedBy:                  userID,
	})
	if !errors.Is(err, ErrEndClockMismatch) {
		t.Fatalf("inconsistent replacement: got %v, want ErrEndClockMismatch", err)
	}

	partial := LocalClock{Hour: 20, Minute: 0}
	_, err = repo.UpsertException(ctx, OccurrenceException{
		ScheduleID:                schedule.ID,
		OriginalLocalDate:         civilDate(2026, 10, 3),
		ReplacementStartLocalTime: &partial,
		UpdatedBy:                 userID,
	})
	if !errors.Is(err, ErrInvalidException) {
		t.Fatalf("partial replacement: got %v, want ErrInvalidException", err)
	}
}
