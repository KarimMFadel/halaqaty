package scheduling

import (
	"errors"
	"testing"
	"time"
)

func mustLocation(t *testing.T, name string) *time.Location {
	t.Helper()
	loc, err := LoadLocation(name)
	if err != nil {
		t.Fatalf("load %s: %v", name, err)
	}
	return loc
}

func TestLoadLocation_RejectsInvalidZones(t *testing.T) {
	t.Parallel()
	for _, name := range []string{"", "   ", "Not/AZone", "Local", " America/New_York", "America/New_York "} {
		if _, err := LoadLocation(name); !errors.Is(err, ErrInvalidTimezone) {
			t.Fatalf("zone %q: got %v, want ErrInvalidTimezone", name, err)
		}
	}
	for _, name := range []string{"UTC", "America/New_York", "Asia/Riyadh"} {
		if _, err := LoadLocation(name); err != nil {
			t.Fatalf("zone %q: got %v, want nil", name, err)
		}
	}
}

func TestResolveStartUTC_DSTGapShiftsForwardByClockChange(t *testing.T) {
	t.Parallel()
	// America/New_York springs forward 2026-03-08 02:00 -> 03:00 (EST to EDT,
	// a one-hour clock change), so the nonexistent 02:30 start resolves to
	// 03:30 EDT = 07:30 UTC.
	ny := mustLocation(t, "America/New_York")

	got := ResolveStartUTC(ny, civilDate(2026, 3, 8), LocalClock{Hour: 2, Minute: 30})
	want := time.Date(2026, 3, 8, 7, 30, 0, 0, time.UTC)
	if !got.Equal(want) {
		t.Fatalf("gap start: got %v, want %v", got, want)
	}
	local := got.In(ny)
	if hour, minute, _ := local.Clock(); hour != 3 || minute != 30 {
		t.Fatalf("gap local clock: got %02d:%02d, want 03:30", hour, minute)
	}
}

func TestResolveStartUTC_RepeatedStartUsesFirstOccurrence(t *testing.T) {
	t.Parallel()
	// America/New_York falls back 2026-11-01 02:00 -> 01:00, so 01:30 happens
	// twice; the first occurrence is the EDT instance at 05:30 UTC.
	ny := mustLocation(t, "America/New_York")

	got := ResolveStartUTC(ny, civilDate(2026, 11, 1), LocalClock{Hour: 1, Minute: 30})
	want := time.Date(2026, 11, 1, 5, 30, 0, 0, time.UTC)
	if !got.Equal(want) {
		t.Fatalf("repeated start: got %v, want %v (first, EDT occurrence)", got, want)
	}
	if _, offset := got.In(ny).Zone(); offset != -4*3600 {
		t.Fatalf("repeated start offset: got %d, want -14400 (EDT)", offset)
	}
}

func TestResolveStartUTC_OrdinaryTimeInZoneWithoutDST(t *testing.T) {
	t.Parallel()
	riyadh := mustLocation(t, "Asia/Riyadh")

	got := ResolveStartUTC(riyadh, civilDate(2026, 6, 15), LocalClock{Hour: 18, Minute: 0})
	want := time.Date(2026, 6, 15, 15, 0, 0, 0, time.UTC)
	if !got.Equal(want) {
		t.Fatalf("riyadh start: got %v, want %v", got, want)
	}
}

func TestValidatePlannedClocks_DurationBounds(t *testing.T) {
	t.Parallel()
	start := LocalClock{Hour: 10, Minute: 0}
	end := LocalClock{Hour: 10, Minute: 1}

	if err := ValidatePlannedClocks(start, end, 0); !errors.Is(err, ErrInvalidDuration) {
		t.Fatalf("duration 0: got %v, want ErrInvalidDuration", err)
	}
	if err := ValidatePlannedClocks(start, end, 1); err != nil {
		t.Fatalf("duration 1: got %v, want nil", err)
	}
	// 44,640 minutes = 31 days, the approved upper bound.
	if err := ValidatePlannedClocks(start, start, 44640); err != nil {
		t.Fatalf("duration 44640: got %v, want nil", err)
	}
	if err := ValidatePlannedClocks(start, start, 44641); !errors.Is(err, ErrInvalidDuration) {
		t.Fatalf("duration 44641: got %v, want ErrInvalidDuration", err)
	}
}

func TestValidatePlannedClocks_EndClockNominalRule(t *testing.T) {
	t.Parallel()
	// Overnight: 23:00 + 120 minutes nominally ends at 01:00.
	if err := ValidatePlannedClocks(LocalClock{Hour: 23, Minute: 0}, LocalClock{Hour: 1, Minute: 0}, 120); err != nil {
		t.Fatalf("overnight end clock: got %v, want nil", err)
	}
	if err := ValidatePlannedClocks(LocalClock{Hour: 23, Minute: 0}, LocalClock{Hour: 1, Minute: 1}, 120); !errors.Is(err, ErrEndClockMismatch) {
		t.Fatalf("mismatched end clock: got %v, want ErrEndClockMismatch", err)
	}
	// Multi-day: 10:00 + 2 days nominally ends at 10:00.
	if err := ValidatePlannedClocks(LocalClock{Hour: 10, Minute: 0}, LocalClock{Hour: 10, Minute: 0}, 2880); err != nil {
		t.Fatalf("multi-day end clock: got %v, want nil", err)
	}
	if err := ValidatePlannedClocks(LocalClock{Hour: 24, Minute: 0}, LocalClock{Hour: 1, Minute: 0}, 60); !errors.Is(err, ErrInvalidLocalClock) {
		t.Fatalf("hour 24: got %v, want ErrInvalidLocalClock", err)
	}
}

func TestPlannedClocks_RetainedInputClocksSurviveDSTResolution(t *testing.T) {
	t.Parallel()
	// The revision retains the entered local clocks; resolution must not
	// rewrite them, and the resolved local end may truthfully differ from the
	// retained end clock when DST intervenes mid-occurrence.
	ny := mustLocation(t, "America/New_York")
	startClock := LocalClock{Hour: 23, Minute: 30} // 2026-03-07 23:30 EST
	endClock := LocalClock{Hour: 2, Minute: 30}    // retained nominal end input

	if err := ValidatePlannedClocks(startClock, endClock, 180); err != nil {
		t.Fatalf("retained clocks must validate: %v", err)
	}
	startUTC, endUTC, err := ResolveIntervalUTC(ny, civilDate(2026, 3, 7), startClock, 180)
	if err != nil {
		t.Fatalf("resolve interval: %v", err)
	}
	if startClock != (LocalClock{Hour: 23, Minute: 30}) || endClock != (LocalClock{Hour: 2, Minute: 30}) {
		t.Fatalf("retained clocks changed: start=%v end=%v", startClock, endClock)
	}
	if want := time.Date(2026, 3, 8, 4, 30, 0, 0, time.UTC); !startUTC.Equal(want) {
		t.Fatalf("start: got %v, want %v", startUTC, want)
	}
	// The spring-forward falls inside the 3-hour occurrence, so the resolved
	// local end is 03:30 EDT, not the retained 02:30.
	localEnd := endUTC.In(ny)
	if hour, minute, _ := localEnd.Clock(); hour != 3 || minute != 30 {
		t.Fatalf("resolved local end: got %02d:%02d, want 03:30 (differs from retained 02:30)", hour, minute)
	}
	if NominalEndClock(startClock, 180) != endClock {
		t.Fatal("retained end clock must equal start plus duration by nominal 24-hour arithmetic")
	}
}

func TestResolveIntervalUTC_OvernightDuration(t *testing.T) {
	t.Parallel()
	riyadh := mustLocation(t, "Asia/Riyadh")

	startUTC, endUTC, err := ResolveIntervalUTC(riyadh, civilDate(2026, 6, 15), LocalClock{Hour: 22, Minute: 0}, 240)
	if err != nil {
		t.Fatalf("resolve overnight interval: %v", err)
	}
	if !endUTC.Equal(startUTC.Add(4 * time.Hour)) {
		t.Fatalf("overnight end: got %v, want start + 4h (%v)", endUTC, startUTC.Add(4*time.Hour))
	}
	localEnd := endUTC.In(riyadh)
	if y, m, d := localEnd.Date(); y != 2026 || m != 6 || d != 16 {
		t.Fatalf("overnight local end date: got %v, want 2026-06-16", localEnd)
	}
	if hour, minute, _ := localEnd.Clock(); hour != 2 || minute != 0 {
		t.Fatalf("overnight local end clock: got %02d:%02d, want 02:00", hour, minute)
	}
}

func TestResolveIntervalUTC_MultiDayDuration(t *testing.T) {
	t.Parallel()
	riyadh := mustLocation(t, "Asia/Riyadh")

	startUTC, endUTC, err := ResolveIntervalUTC(riyadh, civilDate(2026, 6, 15), LocalClock{Hour: 10, Minute: 0}, 2880)
	if err != nil {
		t.Fatalf("resolve multi-day interval: %v", err)
	}
	if !endUTC.Equal(startUTC.Add(48 * time.Hour)) {
		t.Fatalf("multi-day end: got %v, want start + 48h (%v)", endUTC, startUTC.Add(48*time.Hour))
	}
	localEnd := endUTC.In(riyadh)
	if y, m, d := localEnd.Date(); y != 2026 || m != 6 || d != 17 {
		t.Fatalf("multi-day local end date: got %v, want 2026-06-17", localEnd)
	}
}

func TestResolveIntervalUTC_EndPreservesElapsedAcrossDSTGap(t *testing.T) {
	t.Parallel()
	// 2026-03-08 00:30 EST + 180 minutes: the UTC end is start + elapsed
	// duration, so the displayed local end (04:30 EDT) truthfully differs
	// from the stored nominal end clock (03:30).
	ny := mustLocation(t, "America/New_York")

	startUTC, endUTC, err := ResolveIntervalUTC(ny, civilDate(2026, 3, 8), LocalClock{Hour: 0, Minute: 30}, 180)
	if err != nil {
		t.Fatalf("resolve DST interval: %v", err)
	}
	if want := time.Date(2026, 3, 8, 5, 30, 0, 0, time.UTC); !startUTC.Equal(want) {
		t.Fatalf("start: got %v, want %v", startUTC, want)
	}
	if want := startUTC.Add(180 * time.Minute); !endUTC.Equal(want) {
		t.Fatalf("end: got %v, want %v", endUTC, want)
	}
	localEnd := endUTC.In(ny)
	if hour, minute, _ := localEnd.Clock(); hour != 4 || minute != 30 {
		t.Fatalf("DST local end: got %02d:%02d, want 04:30", hour, minute)
	}
	if stored := NominalEndClock(LocalClock{Hour: 0, Minute: 30}, 180); stored != (LocalClock{Hour: 3, Minute: 30}) {
		t.Fatalf("stored nominal end clock: got %v, want 03:30", stored)
	}
}

func TestInstantInZone_ConvertsStoredUTCToViewerZone(t *testing.T) {
	t.Parallel()
	instant := time.Date(2026, 1, 15, 5, 30, 0, 0, time.UTC)

	eastern, err := InstantInZone(instant, "America/New_York")
	if err != nil {
		t.Fatalf("convert to America/New_York: %v", err)
	}
	if y, m, d := eastern.Date(); y != 2026 || m != 1 || d != 15 {
		t.Fatalf("eastern date: got %v, want 2026-01-15", eastern)
	}
	if hour, minute, _ := eastern.Clock(); hour != 0 || minute != 30 {
		t.Fatalf("eastern clock: got %02d:%02d, want 00:30", hour, minute)
	}

	tokyo, err := InstantInZone(instant, "Asia/Tokyo")
	if err != nil {
		t.Fatalf("convert to Asia/Tokyo: %v", err)
	}
	if hour, minute, _ := tokyo.Clock(); hour != 14 || minute != 30 {
		t.Fatalf("tokyo clock: got %02d:%02d, want 14:30", hour, minute)
	}

	if _, err := InstantInZone(instant, "Not/AZone"); !errors.Is(err, ErrInvalidTimezone) {
		t.Fatalf("invalid viewer zone: got %v, want ErrInvalidTimezone", err)
	}
}
