package scheduling

import (
	"errors"
	"testing"
	"time"
)

func civilDate(year int, month time.Month, day int) time.Time {
	return time.Date(year, month, day, 0, 0, 0, 0, time.UTC)
}

func datePtr(t time.Time) *time.Time { return &t }

func requireDates(t *testing.T, got []time.Time, want []time.Time) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("got %d dates %v, want %d dates %v", len(got), got, len(want), want)
	}
	for i := range want {
		if !got[i].Equal(want[i]) {
			t.Fatalf("date %d: got %v, want %v (all: %v)", i, got[i], want[i], got)
		}
	}
}

func weekdayRevision(anchor time.Time, cadence int, weekdays ...int) Revision {
	return Revision{
		ScheduleID:         "sched-weekday",
		Version:            1,
		EffectiveLocalDate: anchor,
		Mode:               ModeWeekdayPattern,
		AnchorLocalDate:    anchor,
		WeekCadence:        cadence,
		Weekdays:           weekdays,
	}
}

func TestOccurrenceDates_WeeklyWeekdaySet(t *testing.T) {
	t.Parallel()
	// Anchor Monday 2026-01-05, weekly Mondays and Wednesdays.
	rev := weekdayRevision(civilDate(2026, 1, 5), 1, 1, 3)

	got := rev.OccurrenceDates(civilDate(2026, 1, 1), civilDate(2026, 1, 31))
	requireDates(t, got, []time.Time{
		civilDate(2026, 1, 5), civilDate(2026, 1, 7),
		civilDate(2026, 1, 12), civilDate(2026, 1, 14),
		civilDate(2026, 1, 19), civilDate(2026, 1, 21),
		civilDate(2026, 1, 26), civilDate(2026, 1, 28),
	})
}

func TestOccurrenceDates_BiweeklyWeekdaySet(t *testing.T) {
	t.Parallel()
	// Anchor Monday 2026-01-05, every second week on Mondays and Thursdays.
	rev := weekdayRevision(civilDate(2026, 1, 5), 2, 1, 4)

	got := rev.OccurrenceDates(civilDate(2026, 1, 1), civilDate(2026, 2, 28))
	requireDates(t, got, []time.Time{
		civilDate(2026, 1, 5), civilDate(2026, 1, 8),
		civilDate(2026, 1, 19), civilDate(2026, 1, 22),
		civilDate(2026, 2, 2), civilDate(2026, 2, 5),
		civilDate(2026, 2, 16), civilDate(2026, 2, 19),
	})
}

func TestOccurrenceDates_WeekdayWindowBeforeAnchorIsEmpty(t *testing.T) {
	t.Parallel()
	rev := weekdayRevision(civilDate(2026, 1, 5), 1, 1)

	got := rev.OccurrenceDates(civilDate(2025, 12, 1), civilDate(2025, 12, 31))
	if len(got) != 0 {
		t.Fatalf("window before anchor: got %v, want no dates", got)
	}
}

func TestValidatePattern_AnchorWeekdayMustBeSelected(t *testing.T) {
	t.Parallel()
	// Anchor is a Monday but only Tuesday is selected.
	rev := weekdayRevision(civilDate(2026, 1, 5), 1, 2)
	if err := rev.ValidatePattern(); !errors.Is(err, ErrInvalidWeekdayPattern) {
		t.Fatalf("anchor weekday not in set: got %v, want ErrInvalidWeekdayPattern", err)
	}

	rev.Weekdays = []int{1, 3}
	if err := rev.ValidatePattern(); err != nil {
		t.Fatalf("anchor weekday in set: got %v, want nil", err)
	}
}

func TestValidatePattern_WeekdayModeChecks(t *testing.T) {
	t.Parallel()
	base := weekdayRevision(civilDate(2026, 1, 5), 1, 1, 3)

	duplicated := base
	duplicated.Weekdays = []int{1, 1}
	if err := duplicated.ValidatePattern(); !errors.Is(err, ErrInvalidWeekdayPattern) {
		t.Fatalf("duplicate weekday: got %v, want ErrInvalidWeekdayPattern", err)
	}

	outOfRange := base
	outOfRange.Weekdays = []int{1, 7}
	if err := outOfRange.ValidatePattern(); !errors.Is(err, ErrInvalidWeekdayPattern) {
		t.Fatalf("weekday 7: got %v, want ErrInvalidWeekdayPattern", err)
	}

	badCadence := base
	badCadence.WeekCadence = 3
	if err := badCadence.ValidatePattern(); !errors.Is(err, ErrInvalidWeekdayPattern) {
		t.Fatalf("cadence 3: got %v, want ErrInvalidWeekdayPattern", err)
	}

	mixedSelectors := base
	mixedSelectors.IntervalCount = 2
	if err := mixedSelectors.ValidatePattern(); !errors.Is(err, ErrInvalidWeekdayPattern) {
		t.Fatalf("interval selector on weekday mode: got %v, want ErrInvalidWeekdayPattern", err)
	}
}

func TestOccurrenceDates_IntervalDaysFromAnchor(t *testing.T) {
	t.Parallel()
	rev := Revision{
		ScheduleID:         "sched-interval",
		Version:            1,
		EffectiveLocalDate: civilDate(2026, 1, 10),
		Mode:               ModeInterval,
		AnchorLocalDate:    civilDate(2026, 1, 10),
		IntervalCount:      3,
		IntervalUnit:       IntervalUnitDay,
	}

	got := rev.OccurrenceDates(civilDate(2026, 1, 1), civilDate(2026, 1, 31))
	requireDates(t, got, []time.Time{
		civilDate(2026, 1, 10), civilDate(2026, 1, 13), civilDate(2026, 1, 16),
		civilDate(2026, 1, 19), civilDate(2026, 1, 22), civilDate(2026, 1, 25),
		civilDate(2026, 1, 28), civilDate(2026, 1, 31),
	})
}

func TestOccurrenceDates_IntervalWeeksFromAnchor(t *testing.T) {
	t.Parallel()
	rev := Revision{
		ScheduleID:         "sched-interval",
		Version:            1,
		EffectiveLocalDate: civilDate(2026, 1, 6),
		Mode:               ModeInterval,
		AnchorLocalDate:    civilDate(2026, 1, 6),
		IntervalCount:      2,
		IntervalUnit:       IntervalUnitWeek,
	}

	got := rev.OccurrenceDates(civilDate(2026, 1, 1), civilDate(2026, 2, 28))
	requireDates(t, got, []time.Time{
		civilDate(2026, 1, 6), civilDate(2026, 1, 20),
		civilDate(2026, 2, 3), civilDate(2026, 2, 17),
	})
}

func TestValidatePattern_IntervalModeChecks(t *testing.T) {
	t.Parallel()
	base := Revision{
		ScheduleID:         "sched-interval",
		Version:            1,
		EffectiveLocalDate: civilDate(2026, 1, 10),
		Mode:               ModeInterval,
		AnchorLocalDate:    civilDate(2026, 1, 10),
		IntervalCount:      3,
		IntervalUnit:       IntervalUnitDay,
	}
	if err := base.ValidatePattern(); err != nil {
		t.Fatalf("valid interval: got %v, want nil", err)
	}

	zeroCount := base
	zeroCount.IntervalCount = 0
	if err := zeroCount.ValidatePattern(); !errors.Is(err, ErrInvalidInterval) {
		t.Fatalf("zero count: got %v, want ErrInvalidInterval", err)
	}

	badUnit := base
	badUnit.IntervalUnit = "month"
	if err := badUnit.ValidatePattern(); !errors.Is(err, ErrInvalidInterval) {
		t.Fatalf("month unit: got %v, want ErrInvalidInterval", err)
	}

	mixedSelectors := base
	mixedSelectors.Weekdays = []int{1}
	if err := mixedSelectors.ValidatePattern(); !errors.Is(err, ErrInvalidInterval) {
		t.Fatalf("weekday selector on interval mode: got %v, want ErrInvalidInterval", err)
	}
}

func TestOccurrenceDates_SelectedDatesDeduplicated(t *testing.T) {
	t.Parallel()
	rev := Revision{
		ScheduleID:         "sched-selected",
		Version:            1,
		EffectiveLocalDate: civilDate(2026, 3, 2),
		Mode:               ModeSelectedDates,
		AnchorLocalDate:    civilDate(2026, 3, 2),
		SelectedDates: []time.Time{
			civilDate(2026, 3, 9),
			civilDate(2026, 3, 2),
			civilDate(2026, 3, 9), // duplicate produces one occurrence
			civilDate(2026, 3, 16),
			civilDate(2026, 3, 2), // duplicate of the anchor
		},
	}

	got := rev.OccurrenceDates(civilDate(2026, 2, 1), civilDate(2026, 3, 31))
	requireDates(t, got, []time.Time{
		civilDate(2026, 3, 2), civilDate(2026, 3, 9), civilDate(2026, 3, 16),
	})
}

func TestValidatePattern_SelectedDatesFirstDateIsAnchor(t *testing.T) {
	t.Parallel()
	base := Revision{
		ScheduleID:         "sched-selected",
		Version:            1,
		EffectiveLocalDate: civilDate(2026, 3, 2),
		Mode:               ModeSelectedDates,
		AnchorLocalDate:    civilDate(2026, 3, 2),
		SelectedDates:      []time.Time{civilDate(2026, 3, 2), civilDate(2026, 3, 9)},
	}
	if err := base.ValidatePattern(); err != nil {
		t.Fatalf("valid selected dates: got %v, want nil", err)
	}

	notAnchored := base
	notAnchored.SelectedDates = []time.Time{civilDate(2026, 3, 9), civilDate(2026, 3, 16)}
	if err := notAnchored.ValidatePattern(); !errors.Is(err, ErrInvalidSelectedDates) {
		t.Fatalf("first date != anchor: got %v, want ErrInvalidSelectedDates", err)
	}

	beforeAnchor := base
	beforeAnchor.SelectedDates = []time.Time{civilDate(2026, 3, 2), civilDate(2026, 3, 1)}
	if err := beforeAnchor.ValidatePattern(); !errors.Is(err, ErrInvalidSelectedDates) {
		t.Fatalf("date before anchor: got %v, want ErrInvalidSelectedDates", err)
	}

	beyondEnd := base
	beyondEnd.EndLocalDate = datePtr(civilDate(2026, 3, 8))
	if err := beyondEnd.ValidatePattern(); !errors.Is(err, ErrInvalidSelectedDates) {
		t.Fatalf("date beyond end: got %v, want ErrInvalidSelectedDates", err)
	}
}

func TestOccurrenceDates_InclusiveEndLocalDate(t *testing.T) {
	t.Parallel()
	// An occurrence starting ON the end date is included; the end date applies
	// to local starts, so an occurrence ending overnight the next local day
	// (any positive duration) is still generated on its start date.
	endOnOccurrence := weekdayRevision(civilDate(2026, 1, 5), 1, 1)
	endOnOccurrence.EndLocalDate = datePtr(civilDate(2026, 2, 2)) // a Monday

	got := endOnOccurrence.OccurrenceDates(civilDate(2026, 1, 1), civilDate(2026, 3, 31))
	wantTail := civilDate(2026, 2, 2)
	if len(got) == 0 || !got[len(got)-1].Equal(wantTail) {
		t.Fatalf("end date on occurrence day: got %v, want last date %v", got, wantTail)
	}

	endOffOccurrence := weekdayRevision(civilDate(2026, 1, 5), 1, 1)
	endOffOccurrence.EndLocalDate = datePtr(civilDate(2026, 2, 3)) // a Tuesday

	got = endOffOccurrence.OccurrenceDates(civilDate(2026, 1, 1), civilDate(2026, 3, 31))
	if len(got) == 0 || !got[len(got)-1].Equal(civilDate(2026, 2, 2)) {
		t.Fatalf("end date off occurrence day: got %v, want last date 2026-02-02", got)
	}
}

func TestOccurrenceDates_NoEndDateIsOpenEnded(t *testing.T) {
	t.Parallel()
	rev := weekdayRevision(civilDate(2026, 1, 5), 1, 1)

	got := rev.OccurrenceDates(civilDate(2026, 12, 1), civilDate(2026, 12, 31))
	requireDates(t, got, []time.Time{
		civilDate(2026, 12, 7), civilDate(2026, 12, 14),
		civilDate(2026, 12, 21), civilDate(2026, 12, 28),
	})
}

func TestOccurrenceDates_DistantFutureWindow(t *testing.T) {
	t.Parallel()
	// Weekly Mondays anchored in 2020; a 2089 window must be answered by
	// jumping to the first candidate at/after the lower bound, not by
	// iterating from the anchor. The expected set is derived by naive
	// stepping, which the arithmetic jump must reproduce exactly.
	rev := weekdayRevision(civilDate(2020, 1, 6), 1, 1)

	from := civilDate(2089, 3, 1)
	to := civilDate(2089, 3, 31)
	var want []time.Time
	for d := civilDate(2020, 1, 6); !d.After(to); d = d.AddDate(0, 0, 7) {
		if !d.Before(from) {
			want = append(want, d)
		}
	}
	if len(want) == 0 {
		t.Fatal("test setup produced no expected dates")
	}

	got := rev.OccurrenceDates(from, to)
	requireDates(t, got, want)
}

func TestOccurrenceDates_BeyondDurationRange(t *testing.T) {
	t.Parallel()
	anchor := civilDate(2026, 1, 1)
	from, to := civilDate(2500, 3, 1), civilDate(2500, 3, 31)
	rev := Revision{Version: 1, Mode: ModeInterval, AnchorLocalDate: anchor,
		EffectiveLocalDate: anchor, IntervalCount: 3, IntervalUnit: IntervalUnitDay}
	var want []time.Time
	for d := from; !d.After(to); d = d.AddDate(0, 0, 1) {
		if (d.Unix()-anchor.Unix())/86400%3 == 0 {
			want = append(want, d)
		}
	}
	got := rev.OccurrenceDates(from, to)
	if len(got) != len(want) {
		t.Fatalf("got %d dates, want %d within the requested month", len(got), len(want))
	}
	requireDates(t, got, want)
}

func TestSelectRevision_HighestEffectiveVersionWins(t *testing.T) {
	t.Parallel()
	revisions := []Revision{
		{ScheduleID: "sched", Version: 1, EffectiveLocalDate: civilDate(2026, 1, 1)},
		{ScheduleID: "sched", Version: 2, EffectiveLocalDate: civilDate(2026, 2, 1)},
		// Equal effective dates are permitted; the higher version wins.
		{ScheduleID: "sched", Version: 3, EffectiveLocalDate: civilDate(2026, 2, 1)},
	}

	rev, ok := SelectRevision(revisions, civilDate(2026, 1, 15))
	if !ok || rev.Version != 1 {
		t.Fatalf("2026-01-15: got version %d ok=%v, want version 1", rev.Version, ok)
	}

	rev, ok = SelectRevision(revisions, civilDate(2026, 2, 1))
	if !ok || rev.Version != 3 {
		t.Fatalf("2026-02-01: got version %d ok=%v, want version 3 (equal effective date)", rev.Version, ok)
	}

	rev, ok = SelectRevision(revisions, civilDate(2026, 6, 30))
	if !ok || rev.Version != 3 {
		t.Fatalf("2026-06-30: got version %d ok=%v, want version 3", rev.Version, ok)
	}

	if _, ok = SelectRevision(revisions, civilDate(2025, 12, 31)); ok {
		t.Fatal("date before every effective date: got a revision, want none")
	}
}

func TestOccurrenceWindow_SeriesRevisionSupersedesFutureDates(t *testing.T) {
	t.Parallel()
	// v1 schedules Mondays from 2026-01-05; v2 is effective 2026-02-01 and
	// schedules Tuesdays from 2026-02-03. Dates governed by v2 follow only
	// v2's pattern, including Monday 2026-02-02 which v1 would have produced.
	v1 := Revision{
		ScheduleID:         "sched",
		Version:            1,
		EffectiveLocalDate: civilDate(2026, 1, 1),
		Mode:               ModeWeekdayPattern,
		AnchorLocalDate:    civilDate(2026, 1, 5),
		WeekCadence:        1,
		Weekdays:           []int{1},
	}
	v2 := Revision{
		ScheduleID:         "sched",
		Version:            2,
		EffectiveLocalDate: civilDate(2026, 2, 1),
		Mode:               ModeWeekdayPattern,
		AnchorLocalDate:    civilDate(2026, 2, 3),
		WeekCadence:        1,
		Weekdays:           []int{2},
	}

	got := OccurrenceWindow([]Revision{v2, v1}, civilDate(2026, 1, 1), civilDate(2026, 2, 28))
	wantDates := []time.Time{
		civilDate(2026, 1, 5), civilDate(2026, 1, 12),
		civilDate(2026, 1, 19), civilDate(2026, 1, 26),
		civilDate(2026, 2, 3), civilDate(2026, 2, 10),
		civilDate(2026, 2, 17), civilDate(2026, 2, 24),
	}
	if len(got) != len(wantDates) {
		t.Fatalf("got %d occurrences %v, want %d", len(got), got, len(wantDates))
	}
	for i, want := range wantDates {
		if !got[i].OriginalLocalDate.Equal(want) {
			t.Fatalf("occurrence %d: got %v, want %v (all: %v)", i, got[i].OriginalLocalDate, want, got)
		}
		wantVersion := 1
		if !want.Before(civilDate(2026, 2, 1)) {
			wantVersion = 2
		}
		if got[i].Version != wantVersion || got[i].ScheduleID != "sched" {
			t.Fatalf("occurrence %d identity: got (%s, v%d), want (sched, v%d)", i, got[i].ScheduleID, got[i].Version, wantVersion)
		}
	}
}

func TestOccurrenceWindow_ReversedWindowIsEmpty(t *testing.T) {
	t.Parallel()
	rev := weekdayRevision(civilDate(2026, 1, 5), 1, 1)

	got := OccurrenceWindow([]Revision{rev}, civilDate(2026, 2, 1), civilDate(2026, 1, 1))
	if len(got) != 0 {
		t.Fatalf("reversed window: got %v, want no occurrences", got)
	}
}
