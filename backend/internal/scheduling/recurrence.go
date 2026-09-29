package scheduling

import (
	"errors"
	"fmt"
	"sort"
	"time"
)

// RecurrenceMode identifies one approved schedule recurrence mode.
type RecurrenceMode string

const (
	// ModeWeekdayPattern repeats selected weekdays on a weekly/biweekly cadence.
	ModeWeekdayPattern RecurrenceMode = "weekday_pattern"
	// ModeInterval repeats every positive whole-number count of days or weeks.
	ModeInterval RecurrenceMode = "interval"
	// ModeSelectedDates repeats on explicitly selected local dates.
	ModeSelectedDates RecurrenceMode = "selected_dates"
)

// IntervalUnit identifies the stepping unit of ModeInterval.
type IntervalUnit string

const (
	// IntervalUnitDay steps by IntervalCount days.
	IntervalUnitDay IntervalUnit = "day"
	// IntervalUnitWeek steps by IntervalCount weeks.
	IntervalUnitWeek IntervalUnit = "week"
)

var (
	// ErrInvalidRevision means a revision violates a mode-independent invariant.
	ErrInvalidRevision = errors.New("invalid schedule revision")
	// ErrUnknownRecurrenceMode means the mode is not one of the approved modes.
	ErrUnknownRecurrenceMode = errors.New("unknown recurrence mode")
	// ErrInvalidWeekdayPattern means the weekday-pattern selectors are invalid.
	ErrInvalidWeekdayPattern = errors.New("invalid weekday pattern")
	// ErrInvalidInterval means the interval selectors are invalid.
	ErrInvalidInterval = errors.New("invalid recurrence interval")
	// ErrInvalidSelectedDates means the selected dates are invalid.
	ErrInvalidSelectedDates = errors.New("invalid selected dates")
)

// Revision mirrors one effective-dated schedule_revisions row together with
// its schedule_selected_dates. Civil dates are time.Time values whose clock
// components are ignored; weekday numbers follow EXTRACT(DOW) (Sunday = 0).
type Revision struct {
	ScheduleID         string
	Version            int
	EffectiveLocalDate time.Time
	Mode               RecurrenceMode
	AnchorLocalDate    time.Time
	EndLocalDate       *time.Time // inclusive upper bound on local start dates
	WeekCadence        int        // 1 or 2, weekday mode only
	Weekdays           []int      // weekday mode only
	IntervalCount      int        // interval mode only
	IntervalUnit       IntervalUnit
	SelectedDates      []time.Time // selected_dates mode only
}

// Occurrence is one generated occurrence of a schedule. Its stable public
// identity is (ScheduleID, OriginalLocalDate); Version records the governing
// revision so callers can detect supersession.
type Occurrence struct {
	ScheduleID        string
	Version           int
	OriginalLocalDate time.Time
}

// ValidatePattern mirrors the mode-specific schedule_revisions checks.
func (r Revision) ValidatePattern() error {
	if r.Version <= 0 {
		return fmt.Errorf("revision version must be positive: %w", ErrInvalidRevision)
	}
	if r.EndLocalDate != nil && civilOf(*r.EndLocalDate).Before(civilOf(r.AnchorLocalDate)) {
		return fmt.Errorf("end local date before anchor: %w", ErrInvalidRevision)
	}
	switch r.Mode {
	case ModeWeekdayPattern:
		return r.validateWeekdayPattern()
	case ModeInterval:
		return r.validateInterval()
	case ModeSelectedDates:
		return r.validateSelectedDates()
	default:
		return fmt.Errorf("mode %q: %w", r.Mode, ErrUnknownRecurrenceMode)
	}
}

func (r Revision) validateWeekdayPattern() error {
	if r.WeekCadence != 1 && r.WeekCadence != 2 {
		return fmt.Errorf("week cadence %d: %w", r.WeekCadence, ErrInvalidWeekdayPattern)
	}
	if len(r.Weekdays) == 0 || r.IntervalCount != 0 || r.IntervalUnit != "" {
		return fmt.Errorf("weekday mode selectors: %w", ErrInvalidWeekdayPattern)
	}
	seen := make(map[int]bool, len(r.Weekdays))
	for _, day := range r.Weekdays {
		if day < 0 || day > 6 || seen[day] {
			return fmt.Errorf("weekday %d: %w", day, ErrInvalidWeekdayPattern)
		}
		seen[day] = true
	}
	if !seen[int(civilOf(r.AnchorLocalDate).Weekday())] {
		return fmt.Errorf("anchor weekday not selected: %w", ErrInvalidWeekdayPattern)
	}
	return nil
}

func (r Revision) validateInterval() error {
	if r.IntervalCount <= 0 || (r.IntervalUnit != IntervalUnitDay && r.IntervalUnit != IntervalUnitWeek) {
		return fmt.Errorf("interval %d %q: %w", r.IntervalCount, r.IntervalUnit, ErrInvalidInterval)
	}
	if r.WeekCadence != 0 || len(r.Weekdays) != 0 {
		return fmt.Errorf("interval mode selectors: %w", ErrInvalidInterval)
	}
	return nil
}

func (r Revision) validateSelectedDates() error {
	if r.WeekCadence != 0 || len(r.Weekdays) != 0 || r.IntervalCount != 0 || r.IntervalUnit != "" {
		return fmt.Errorf("selected-dates mode selectors: %w", ErrInvalidSelectedDates)
	}
	if len(r.SelectedDates) == 0 {
		return fmt.Errorf("no selected dates: %w", ErrInvalidSelectedDates)
	}
	anchor := civilOf(r.AnchorLocalDate)
	first := civilOf(r.SelectedDates[0])
	for _, d := range r.SelectedDates {
		day := civilOf(d)
		if day.Before(anchor) {
			return fmt.Errorf("selected date %s before anchor: %w", day.Format(time.DateOnly), ErrInvalidSelectedDates)
		}
		if r.EndLocalDate != nil && day.After(civilOf(*r.EndLocalDate)) {
			return fmt.Errorf("selected date %s beyond end date: %w", day.Format(time.DateOnly), ErrInvalidSelectedDates)
		}
		if day.Before(first) {
			first = day
		}
	}
	if !first.Equal(anchor) {
		return fmt.Errorf("first selected date must equal the anchor: %w", ErrInvalidSelectedDates)
	}
	return nil
}

// OccurrenceDates returns the revision's original local dates whose local
// start falls in the inclusive [from, to] window. It jumps arithmetically to
// the first candidate at or after the lower bound instead of iterating from
// a distant anchor, so a far-future window costs the same as a near one.
func (r Revision) OccurrenceDates(from, to time.Time) []time.Time {
	lower := maxCivil(civilOf(from), civilOf(r.AnchorLocalDate), civilOf(r.EffectiveLocalDate))
	upper := civilOf(to)
	if r.EndLocalDate != nil && civilOf(*r.EndLocalDate).Before(upper) {
		upper = civilOf(*r.EndLocalDate)
	}
	if lower.After(upper) {
		return nil
	}
	switch r.Mode {
	case ModeWeekdayPattern:
		return r.weekdayDates(lower, upper)
	case ModeInterval:
		return r.intervalDates(lower, upper)
	case ModeSelectedDates:
		return r.selectedDates(lower, upper)
	default:
		return nil
	}
}

func (r Revision) weekdayDates(lower, upper time.Time) []time.Time {
	anchorWeek := weekStart(civilOf(r.AnchorLocalDate))
	inSet := make(map[int]bool, len(r.Weekdays))
	for _, day := range r.Weekdays {
		inSet[day] = true
	}
	week := daysBetween(anchorWeek, weekStart(lower)) / 7
	if rem := week % r.WeekCadence; rem != 0 {
		week += r.WeekCadence - rem
	}
	var dates []time.Time
	for start := anchorWeek.AddDate(0, 0, week*7); !start.After(upper); start = start.AddDate(0, 0, r.WeekCadence*7) {
		for day := 0; day < 7; day++ {
			if !inSet[day] {
				continue
			}
			d := start.AddDate(0, 0, day)
			if !d.Before(lower) && !d.After(upper) {
				dates = append(dates, d)
			}
		}
	}
	return dates
}

func (r Revision) intervalDates(lower, upper time.Time) []time.Time {
	anchor := civilOf(r.AnchorLocalDate)
	stepDays := r.IntervalCount
	if r.IntervalUnit == IntervalUnitWeek {
		stepDays *= 7
	}
	start := anchor
	if lower.After(anchor) {
		steps := (daysBetween(anchor, lower) + stepDays - 1) / stepDays
		start = anchor.AddDate(0, 0, steps*stepDays)
	}
	var dates []time.Time
	for d := start; !d.After(upper); d = d.AddDate(0, 0, stepDays) {
		dates = append(dates, d)
	}
	return dates
}

func (r Revision) selectedDates(lower, upper time.Time) []time.Time {
	seen := make(map[time.Time]bool, len(r.SelectedDates))
	var dates []time.Time
	for _, d := range r.SelectedDates {
		day := civilOf(d)
		if seen[day] || day.Before(lower) || day.After(upper) {
			continue
		}
		seen[day] = true
		dates = append(dates, day)
	}
	sort.Slice(dates, func(i, j int) bool { return dates[i].Before(dates[j]) })
	return dates
}

// SelectRevision returns the revision governing a local date: the highest
// version whose effective_local_date is at or before that date. Equal
// effective dates are permitted and resolved by version.
func SelectRevision(revisions []Revision, date time.Time) (Revision, bool) {
	day := civilOf(date)
	var best Revision
	found := false
	for _, rev := range revisions {
		if civilOf(rev.EffectiveLocalDate).After(day) {
			continue
		}
		if !found || rev.Version > best.Version {
			best = rev
			found = true
		}
	}
	return best, found
}

// OccurrenceWindow generates one schedule's occurrences whose original local
// date falls in the inclusive [from, to] window, choosing the governing
// effective-dated revision per date. The window bounds all work: callers must
// pass a bounded range (for example one calendar month plus duration lookback)
// and open-ended rules are never expanded beyond it.
func OccurrenceWindow(revisions []Revision, from, to time.Time) []Occurrence {
	if civilOf(from).After(civilOf(to)) {
		return nil
	}
	var occurrences []Occurrence
	for _, rev := range revisions {
		for _, d := range rev.OccurrenceDates(from, to) {
			governing, ok := SelectRevision(revisions, d)
			if !ok || governing.ScheduleID != rev.ScheduleID || governing.Version != rev.Version {
				continue
			}
			occurrences = append(occurrences, Occurrence{
				ScheduleID:        rev.ScheduleID,
				Version:           rev.Version,
				OriginalLocalDate: d,
			})
		}
	}
	sort.Slice(occurrences, func(i, j int) bool {
		return occurrences[i].OriginalLocalDate.Before(occurrences[j].OriginalLocalDate)
	})
	return occurrences
}

// civilOf strips clock components so civil dates compare by calendar day.
func civilOf(t time.Time) time.Time {
	y, m, d := t.Date()
	return time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
}

func maxCivil(dates ...time.Time) time.Time {
	max := dates[0]
	for _, d := range dates[1:] {
		if d.After(max) {
			max = d
		}
	}
	return max
}

// weekStart returns the Sunday starting d's week; weekday numbers follow
// EXTRACT(DOW), so week arithmetic aligns with the stored weekdays array.
func weekStart(d time.Time) time.Time {
	return d.AddDate(0, 0, -int(d.Weekday()))
}

func daysBetween(from, to time.Time) int {
	// Civil UTC midnights can span more than time.Duration's 292-year range.
	return int((to.Unix() - from.Unix()) / 86400)
}
