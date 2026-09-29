package scheduling

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// CalendarItem is one authorized logical planned occurrence in the viewer's calendar.
type CalendarItem struct {
	OccurrenceKey    string    `json:"occurrence_key"`
	SessionID        *string   `json:"session_id"`
	CircleID         string    `json:"circle_id"`
	CircleName       string    `json:"circle_name"`
	Title            string    `json:"title"`
	StartsAt         time.Time `json:"starts_at"`
	EndsAt           time.Time `json:"ends_at"`
	PlanningTimezone string    `json:"planning_timezone"`
	State            string    `json:"state"`
	Version          *int      `json:"version"`
}

// CalendarMonth is the response for one requested viewer-local month.
type CalendarMonth struct {
	Items    []CalendarItem `json:"items"`
	Warnings WarningResult  `json:"warnings"`
}

// WarningResult is the approved calendar warning envelope. US4 fills its list.
type WarningResult struct {
	Warnings []OverlapWarning `json:"warnings"`
}

// CalendarService reads the bounded personal calendar from PostgreSQL.
type CalendarService struct {
	schedules *Repository
	pool      *pgxpool.Pool
}

// NewCalendarService creates a calendar service using the schedule repository's pool.
func NewCalendarService(schedules *Repository) *CalendarService {
	return &CalendarService{schedules: schedules, pool: schedules.pool}
}

// Month returns authorized one-off and recurring items intersecting a month in the
// viewer's stored timezone. A local 31-day lookback covers the maximum plan duration.
func (s *CalendarService) Month(ctx context.Context, actorID, month string) (CalendarMonth, error) {
	parsed, err := time.Parse("2006-01", month)
	if err != nil || len(month) != 7 || parsed.Format("2006-01") != month || parsed.Year() < 1 {
		return CalendarMonth{}, fmt.Errorf("month %q: %w", month, ErrInvalidCalendarMonth)
	}
	var zone string
	if err := s.pool.QueryRow(ctx, calendarViewerQuery, actorID).Scan(&zone); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return CalendarMonth{}, ErrCalendarProfileNotFound
		}
		return CalendarMonth{}, fmt.Errorf("read calendar timezone: %w", err)
	}
	loc, err := LoadLocation(zone)
	if err != nil {
		return CalendarMonth{}, err
	}
	monthStart := time.Date(parsed.Year(), parsed.Month(), 1, 0, 0, 0, 0, loc)
	monthEnd := monthStart.AddDate(0, 1, 0)
	fromDate := civilOf(monthStart).AddDate(0, 0, -MaxDurationMinutes/minutesPerDay)
	toDate := civilOf(monthEnd).AddDate(0, 0, -1)

	rows, err := s.pool.Query(ctx, calendarCirclesQuery, actorID)
	if err != nil {
		return CalendarMonth{}, fmt.Errorf("list calendar circles: %w", err)
	}
	circleNames := map[string]string{}
	var circleIDs []string
	for rows.Next() {
		var id, name string
		if err := rows.Scan(&id, &name); err != nil {
			rows.Close()
			return CalendarMonth{}, fmt.Errorf("scan calendar circle: %w", err)
		}
		circleIDs, circleNames[id] = append(circleIDs, id), name
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return CalendarMonth{}, fmt.Errorf("list calendar circles: %w", err)
	}
	rows.Close()
	items := map[string]CalendarItem{}
	if len(circleIDs) == 0 {
		return CalendarMonth{Items: []CalendarItem{}, Warnings: WarningResult{Warnings: []OverlapWarning{}}}, nil
	}
	monthStartUTC, monthEndUTC := monthStart.UTC(), monthEnd.UTC()
	rows, err = s.pool.Query(ctx, calendarOneOffsQuery, circleIDs, monthStartUTC, monthEndUTC)
	if err != nil {
		return CalendarMonth{}, fmt.Errorf("query one-off calendar items: %w", err)
	}
	for rows.Next() {
		var item CalendarItem
		var status string
		var cancelledAt *time.Time
		if err := rows.Scan(&item.OccurrenceKey, &item.CircleID, &item.CircleName, &item.Title, &status,
			&item.StartsAt, &item.EndsAt, &item.PlanningTimezone, &cancelledAt, &item.Version); err != nil {
			rows.Close()
			return CalendarMonth{}, fmt.Errorf("scan one-off calendar item: %w", err)
		}
		item.SessionID = &item.OccurrenceKey
		item.State = calendarState(status, cancelledAt)
		items[item.OccurrenceKey] = item
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return CalendarMonth{}, fmt.Errorf("query one-off calendar items: %w", err)
	}
	rows.Close()

	var schedules []Schedule
	for _, circleID := range circleIDs {
		entries, err := s.schedules.ListSchedules(ctx, circleID)
		if err != nil {
			return CalendarMonth{}, err
		}
		schedules = append(schedules, entries...)
	}
	scheduleIDs := make([]string, 0, len(schedules))
	for _, schedule := range schedules {
		scheduleIDs = append(scheduleIDs, schedule.ID)
	}
	revisionsByID, err := s.schedules.LoadRevisions(ctx, scheduleIDs)
	if err != nil {
		return CalendarMonth{}, err
	}
	for _, schedule := range schedules {
		records := revisionsByID[schedule.ID]
		revisions := make([]Revision, len(records))
		for i := range records {
			revisions[i] = records[i].Revision
		}
		for _, occurrence := range OccurrenceWindow(revisions, fromDate, toDate) {
			if schedule.StoppedFromLocalDate != nil && !occurrence.OriginalLocalDate.Before(civilOf(*schedule.StoppedFromLocalDate)) {
				continue
			}
			record, ok := revisionFor(records, occurrence.Version)
			if !ok {
				continue
			}
			zone, err := LoadLocation(record.Timezone)
			if err != nil {
				return CalendarMonth{}, fmt.Errorf("schedule %s timezone: %w", schedule.ID, err)
			}
			start, end, err := ResolveIntervalUTC(zone, occurrence.OriginalLocalDate, record.StartLocalTime, record.DurationMinutes)
			if err != nil {
				return CalendarMonth{}, fmt.Errorf("resolve occurrence %s: %w", occurrence.ScheduleID, err)
			}
			if !start.Before(monthEndUTC) || !end.After(monthStartUTC) {
				continue
			}
			key := occurrenceKey(occurrence.ScheduleID, occurrence.OriginalLocalDate)
			items[key] = CalendarItem{OccurrenceKey: key, CircleID: schedule.CircleID, CircleName: circleNames[schedule.CircleID], Title: record.Title, StartsAt: start, EndsAt: end, PlanningTimezone: record.Timezone, State: "scheduled"}
		}
	}
	if len(scheduleIDs) > 0 {
		rows, err = s.pool.Query(ctx, calendarExceptionsQuery, scheduleIDs, fromDate, toDate)
		if err != nil {
			return CalendarMonth{}, fmt.Errorf("query calendar exceptions: %w", err)
		}
		for rows.Next() {
			var scheduleID string
			var original time.Time
			var moved *time.Time
			var startClock, endClock *string
			var duration *int32
			var title *string
			var cancelledAt *time.Time
			if err := rows.Scan(&scheduleID, &original, &moved, &startClock, &endClock, &duration, &title, &cancelledAt); err != nil {
				rows.Close()
				return CalendarMonth{}, fmt.Errorf("scan calendar exception: %w", err)
			}
			key := occurrenceKey(scheduleID, original)
			item, exists := items[key]
			if !exists {
				schedule, ok := calendarSchedule(schedules, scheduleID)
				if !ok || (schedule.StoppedFromLocalDate != nil && !original.Before(civilOf(*schedule.StoppedFromLocalDate))) {
					continue
				}
				records := revisionsByID[scheduleID]
				record, ok := occurrenceRevision(records, original)
				if !ok {
					continue
				}
				zone, err := LoadLocation(record.Timezone)
				if err != nil {
					rows.Close()
					return CalendarMonth{}, fmt.Errorf("schedule %s timezone: %w", scheduleID, err)
				}
				start, end, err := ResolveIntervalUTC(zone, original, record.StartLocalTime, record.DurationMinutes)
				if err != nil {
					rows.Close()
					return CalendarMonth{}, err
				}
				item = CalendarItem{OccurrenceKey: key, CircleID: schedule.CircleID, CircleName: circleNames[schedule.CircleID], Title: record.Title, StartsAt: start, EndsAt: end, PlanningTimezone: record.Timezone, State: "scheduled"}
			}
			if cancelledAt != nil {
				item.State = "cancelled"
			}
			if moved != nil || startClock != nil {
				var records = revisionsByID[scheduleID]
				rev, ok := SelectRevision(revisionValues(records), original)
				if !ok {
					continue
				}
				date := original
				if moved != nil {
					date = *moved
				}
				record, ok := revisionFor(records, rev.Version)
				if !ok {
					continue
				}
				clock, minutes := record.StartLocalTime, record.DurationMinutes
				if startClock != nil {
					clock, err = parseLocalClock(*startClock)
					if err != nil {
						rows.Close()
						return CalendarMonth{}, err
					}
				}
				if duration != nil {
					minutes = int(*duration)
				}
				if title != nil {
					item.Title = *title
				}
				item.PlanningTimezone = record.Timezone
				zone, err := LoadLocation(item.PlanningTimezone)
				if err != nil {
					rows.Close()
					return CalendarMonth{}, err
				}
				item.StartsAt, item.EndsAt, err = ResolveIntervalUTC(zone, date, clock, minutes)
				if err != nil {
					rows.Close()
					return CalendarMonth{}, err
				}
			}
			if item.StartsAt.Before(monthEndUTC) && item.EndsAt.After(monthStartUTC) {
				items[key] = item
			} else {
				delete(items, key)
			}
		}
		if err := rows.Err(); err != nil {
			rows.Close()
			return CalendarMonth{}, fmt.Errorf("query calendar exceptions: %w", err)
		}
		rows.Close()
		rows, err = s.pool.Query(ctx, calendarMaterializedQuery, scheduleIDs, monthStartUTC, monthEndUTC)
		if err != nil {
			return CalendarMonth{}, fmt.Errorf("query materialized calendar items: %w", err)
		}
		for rows.Next() {
			var scheduleID string
			var original time.Time
			var item CalendarItem
			var status string
			var cancelledAt *time.Time
			if err := rows.Scan(&scheduleID, &original, &item.SessionID, &item.CircleID, &item.CircleName, &item.Title, &status, &item.StartsAt, &item.EndsAt, &item.PlanningTimezone, &cancelledAt); err != nil {
				rows.Close()
				return CalendarMonth{}, fmt.Errorf("scan materialized calendar item: %w", err)
			}
			item.OccurrenceKey = occurrenceKey(scheduleID, original)
			item.State = calendarState(status, cancelledAt)
			items[item.OccurrenceKey] = item
		}
		if err := rows.Err(); err != nil {
			rows.Close()
			return CalendarMonth{}, fmt.Errorf("query materialized calendar items: %w", err)
		}
		rows.Close()
	}
	result := CalendarMonth{Items: make([]CalendarItem, 0, len(items)), Warnings: WarningResult{Warnings: []OverlapWarning{}}}
	for _, item := range items {
		result.Items = append(result.Items, item)
	}
	sort.Slice(result.Items, func(i, j int) bool {
		if result.Items[i].StartsAt.Equal(result.Items[j].StartsAt) {
			return result.Items[i].OccurrenceKey < result.Items[j].OccurrenceKey
		}
		return result.Items[i].StartsAt.Before(result.Items[j].StartsAt)
	})
	return result, nil
}

// ErrInvalidCalendarMonth means the requested month is not YYYY-MM.
var ErrInvalidCalendarMonth = errors.New("invalid calendar month")

// ErrCalendarProfileNotFound means the actor has no stored timezone profile.
var ErrCalendarProfileNotFound = errors.New("calendar profile not found")

func revisionFor(records []RevisionRecord, version int) (RevisionRecord, bool) {
	for _, r := range records {
		if r.Version == version {
			return r, true
		}
	}
	return RevisionRecord{}, false
}
func revisionValues(records []RevisionRecord) []Revision {
	out := make([]Revision, len(records))
	for i := range records {
		out[i] = records[i].Revision
	}
	return out
}
func occurrenceKey(scheduleID string, date time.Time) string {
	return scheduleID + ":" + civilOf(date).Format(time.DateOnly)
}
func calendarState(status string, cancelledAt *time.Time) string {
	if cancelledAt != nil {
		return "cancelled"
	}
	switch status {
	case "active":
		return "active"
	case "ended":
		return "completed"
	default:
		return "scheduled"
	}
}
func calendarSchedule(schedules []Schedule, id string) (Schedule, bool) {
	for _, schedule := range schedules {
		if schedule.ID == id {
			return schedule, true
		}
	}
	return Schedule{}, false
}
func occurrenceRevision(records []RevisionRecord, date time.Time) (RevisionRecord, bool) {
	revisions := revisionValues(records)
	selected, ok := SelectRevision(revisions, date)
	if !ok || len(selected.OccurrenceDates(date, date)) == 0 {
		return RevisionRecord{}, false
	}
	return revisionFor(records, selected.Version)
}
