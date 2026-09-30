package scheduling

import (
	"errors"
	"fmt"
	"strings"
	"time"
)

const (
	// MinDurationMinutes is the shortest approved planned duration.
	MinDurationMinutes = 1
	// MaxDurationMinutes is the approved 31-day planned-duration bound.
	MaxDurationMinutes = 44640

	minutesPerDay = 24 * 60
)

var (
	// ErrInvalidTimezone means the name is not a usable IANA time zone.
	ErrInvalidTimezone = errors.New("invalid IANA timezone")
	// ErrInvalidLocalClock means a local clock time is outside 00:00-23:59.
	ErrInvalidLocalClock = errors.New("invalid local clock time")
	// ErrInvalidDuration means duration_minutes is outside 1-44,640.
	ErrInvalidDuration = errors.New("planned duration out of range")
	// ErrEndClockMismatch means the end clock is not start plus duration by
	// nominal 24-hour arithmetic.
	ErrEndClockMismatch = errors.New("end clock must equal start plus duration")
)

// LocalClock is a retained local wall-clock time at minute precision.
type LocalClock struct {
	Hour   int
	Minute int
}

// LoadLocation validates an IANA zone name and loads it, rejecting empty,
// blank-padded, unknown and "Local" values.
func LoadLocation(name string) (*time.Location, error) {
	if name == "" || name == "Local" || strings.TrimSpace(name) != name {
		return nil, fmt.Errorf("timezone %q: %w", name, ErrInvalidTimezone)
	}
	loc, err := time.LoadLocation(name)
	if err != nil {
		return nil, fmt.Errorf("timezone %q: %w", name, ErrInvalidTimezone)
	}
	return loc, nil
}

// ValidateDurationMinutes enforces the approved 1-44,640 minute bound.
func ValidateDurationMinutes(minutes int) error {
	if minutes < MinDurationMinutes || minutes > MaxDurationMinutes {
		return fmt.Errorf("duration %d minutes: %w", minutes, ErrInvalidDuration)
	}
	return nil
}

// NominalEndClock computes start plus duration by nominal 24-hour arithmetic,
// matching the schedule_revisions end-clock check; the resolved local end may
// differ when DST intervenes.
func NominalEndClock(start LocalClock, durationMinutes int) LocalClock {
	total := (start.Hour*60 + start.Minute + durationMinutes) % minutesPerDay
	return LocalClock{Hour: total / 60, Minute: total % 60}
}

// ValidatePlannedClocks validates the retained local start/end clocks and
// duration as one input set.
func ValidatePlannedClocks(start, end LocalClock, durationMinutes int) error {
	if !start.valid() || !end.valid() {
		return fmt.Errorf("start %v end %v: %w", start, end, ErrInvalidLocalClock)
	}
	if err := ValidateDurationMinutes(durationMinutes); err != nil {
		return err
	}
	if NominalEndClock(start, durationMinutes) != end {
		return fmt.Errorf("start %v end %v duration %d: %w", start, end, durationMinutes, ErrEndClockMismatch)
	}
	return nil
}

// ResolveStartUTC resolves a local date and clock in loc to its UTC instant.
// A nonexistent (DST gap) start shifts forward by exactly the clock change; a
// repeated start uses the first occurrence. Go's time.Date deliberately does
// not guarantee these choices, so offsets are enumerated explicitly.
func ResolveStartUTC(loc *time.Location, date time.Time, clock LocalClock) time.Time {
	y, m, d := date.Date()
	nominal := time.Date(y, m, d, clock.Hour, clock.Minute, 0, 0, time.UTC)
	offsetBefore := zoneOffsetAt(loc, nominal.Add(-26*time.Hour))
	offsetAfter := zoneOffsetAt(loc, nominal.Add(26*time.Hour))

	var first time.Time
	found := false
	for _, offset := range []int{offsetBefore, offsetAfter} {
		candidate := nominal.Add(-time.Duration(offset) * time.Second)
		cy, cm, cd := candidate.In(loc).Date()
		ch, cmin, _ := candidate.In(loc).Clock()
		if cy == y && cm == m && cd == d && ch == clock.Hour && cmin == clock.Minute {
			if !found || candidate.Before(first) {
				first = candidate
				found = true
			}
		}
	}
	if found {
		return first
	}
	// Gap: no offset reproduces the requested clock. Shift forward by the
	// clock change and resolve against the post-transition offset.
	clockChange := offsetAfter - offsetBefore
	return nominal.Add(time.Duration(clockChange) * time.Second).Add(-time.Duration(offsetAfter) * time.Second)
}

// ResolveIntervalUTC resolves a planned occurrence to its UTC start and end.
// The end is the resolved start plus the elapsed duration, so an overnight or
// multi-day occurrence crossing DST keeps its planned length. There is no
// future-date cap.
func ResolveIntervalUTC(loc *time.Location, date time.Time, clock LocalClock, durationMinutes int) (time.Time, time.Time, error) {
	if !clock.valid() {
		return time.Time{}, time.Time{}, fmt.Errorf("start clock %v: %w", clock, ErrInvalidLocalClock)
	}
	if err := ValidateDurationMinutes(durationMinutes); err != nil {
		return time.Time{}, time.Time{}, err
	}
	start := ResolveStartUTC(loc, date, clock)
	return start, start.Add(time.Duration(durationMinutes) * time.Minute), nil
}

// InstantInZone converts a stored UTC instant into a viewer's stored zone.
func InstantInZone(instant time.Time, viewerZone string) (time.Time, error) {
	loc, err := LoadLocation(viewerZone)
	if err != nil {
		return time.Time{}, err
	}
	return instant.In(loc), nil
}

func (c LocalClock) valid() bool {
	return c.Hour >= 0 && c.Hour <= 23 && c.Minute >= 0 && c.Minute <= 59
}

func zoneOffsetAt(loc *time.Location, instant time.Time) int {
	_, offset := instant.In(loc).Zone()
	return offset
}
