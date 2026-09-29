package scheduling

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// OverlapService computes advisory conflicts only from commitments visible to
// the authenticated manager.
type OverlapService struct {
	calendar *CalendarService
	pool     *pgxpool.Pool
}

// NewOverlapService constructs the overlap preview service.
func NewOverlapService(repo *Repository) *OverlapService {
	return &OverlapService{calendar: NewCalendarService(repo), pool: repo.pool}
}

// Preview validates a proposed series and compares its bounded occurrences
// with calendar items visible to the current manager.
func (s *OverlapService) Preview(ctx context.Context, actorID, circleID string, record RevisionRecord) (WarningResult, error) {
	return s.Warnings(ctx, s.pool, actorID, circleID, record, "")
}

func (s *OverlapService) Check(ctx context.Context, q dbQuerier, actorID, circleID string, record RevisionRecord, exclude string, confirmed bool, reviewedIDs []string) error {
	if _, err := q.Exec(ctx, lockOverlapChecksQuery); err != nil {
		return fmt.Errorf("lock overlap confirmation check: %w", err)
	}
	result, err := s.Warnings(ctx, q, actorID, circleID, record, exclude)
	if err != nil {
		return err
	}
	return requireOverlapConfirmation(result.Warnings, confirmed, reviewedIDs)
}

func (s *OverlapService) Warnings(ctx context.Context, q dbQuerier, actorID, circleID string, record RevisionRecord, exclude string) (WarningResult, error) {
	if record.Version == 0 {
		record.Version = 1
	}
	if record.EffectiveLocalDate.IsZero() {
		record.EffectiveLocalDate = record.AnchorLocalDate
	}
	if err := validateNewPlan(record, time.Now()); err != nil {
		return WarningResult{}, err
	}
	if _, err := RequireCircleManage(ctx, q, circleID, actorID); err != nil {
		return WarningResult{}, err
	}
	var circleName string
	if err := q.QueryRow(ctx, overlapCircleNameQuery, circleID).Scan(&circleName); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return WarningResult{}, ErrCircleNotFoundOrDenied
		}
		return WarningResult{}, fmt.Errorf("read preview circle: %w", err)
	}
	proposed, err := previewWindow(record)
	if err != nil {
		return WarningResult{}, err
	}
	for i := range proposed {
		proposed[i].CircleID, proposed[i].CircleName = circleID, circleName
	}
	var viewerZone string
	if err := q.QueryRow(ctx, calendarViewerQuery, actorID).Scan(&viewerZone); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return WarningResult{}, ErrCalendarProfileNotFound
		}
		return WarningResult{}, fmt.Errorf("read preview viewer timezone: %w", err)
	}
	viewerLoc, err := LoadLocation(viewerZone)
	if err != nil {
		return WarningResult{}, err
	}
	months := make(map[string]struct{})
	for _, candidate := range proposed {
		first := candidate.StartsAt.In(viewerLoc)
		last := candidate.EndsAt.Add(-time.Nanosecond).In(viewerLoc)
		for month := time.Date(first.Year(), first.Month(), 1, 0, 0, 0, 0, viewerLoc); !month.After(last); month = month.AddDate(0, 1, 0) {
			months[month.Format("2006-01")] = struct{}{}
		}
	}
	var existing []OverlapInterval
	for month := range months {
		result, err := s.calendar.month(ctx, actorID, month, q)
		if err != nil {
			return WarningResult{}, err
		}
		for _, item := range result.Items {
			if shouldExcludeOverlap(item.OccurrenceKey, exclude) {
				continue
			}
			existing = append(existing, eligibleOverlapIntervals([]CalendarItem{item})...)
		}
	}
	return WarningResult{Warnings: evaluateOverlapWarnings(proposed, existing)}, nil
}

func shouldExcludeOverlap(key, exclude string) bool {
	if exclude == "" {
		return false
	}
	if key == exclude {
		return true
	}
	if strings.Contains(exclude, ":") {
		return false
	}
	return strings.HasPrefix(key, exclude+":")
}

func requireOverlapConfirmation(current []OverlapWarning, confirmed bool, reviewedIDs []string) error {
	if len(current) == 0 {
		return nil
	}
	if !confirmed {
		return &OverlapConfirmationError{Warnings: current}
	}
	reviewed := make(map[string]struct{}, len(reviewedIDs))
	for _, id := range reviewedIDs {
		reviewed[id] = struct{}{}
	}
	for _, warning := range current {
		if _, ok := reviewed[warning.WarningID]; !ok {
			return &OverlapConfirmationError{Warnings: current}
		}
	}
	return nil
}

// OverlapConfirmationError carries only the safe warnings that must be reviewed.
type OverlapConfirmationError struct{ Warnings []OverlapWarning }

func (e *OverlapConfirmationError) Error() string { return "overlap warnings require confirmation" }

func eligibleOverlapIntervals(items []CalendarItem) []OverlapInterval {
	intervals := make([]OverlapInterval, 0, len(items))
	for _, item := range items {
		if item.State != "scheduled" && item.State != "active" {
			continue
		}
		intervals = append(intervals, OverlapInterval{OccurrenceKey: item.OccurrenceKey, CircleID: item.CircleID, CircleName: item.CircleName, StartsAt: item.StartsAt, EndsAt: item.EndsAt})
	}
	return intervals
}

// OverlapInterval is a planned occurrence visible to the caller, or one
// occurrence in a proposed plan.
type OverlapInterval struct {
	OccurrenceKey     string
	OriginalLocalDate time.Time
	CircleID          string
	CircleName        string
	StartsAt          time.Time
	EndsAt            time.Time
}

func plannedRevision(plan PlannedSessionPlan) RevisionRecord {
	return RevisionRecord{
		Revision: Revision{Version: 1, Mode: ModeSelectedDates, AnchorLocalDate: plan.LocalDate, EffectiveLocalDate: plan.LocalDate, SelectedDates: []time.Time{plan.LocalDate}},
		Title:    plan.Title, StartLocalTime: plan.StartLocalTime, EndLocalTime: plan.EndLocalTime,
		DurationMinutes: plan.DurationMinutes, Timezone: plan.Timezone,
	}
}

func previewWindow(record RevisionRecord) ([]OverlapInterval, error) {
	if err := validateRevisionRecord(record); err != nil {
		return nil, err
	}
	zone, err := LoadLocation(record.Timezone)
	if err != nil {
		return nil, err
	}
	from := maxCivil(record.AnchorLocalDate, record.EffectiveLocalDate)
	to := from.AddDate(0, 0, 30)
	if record.EndLocalDate != nil && civilOf(*record.EndLocalDate).Before(to) {
		to = civilOf(*record.EndLocalDate)
	}
	dates := record.OccurrenceDates(from, to)
	if record.Mode == ModeSelectedDates {
		dates = record.OccurrenceDates(from, time.Date(9999, 12, 31, 0, 0, 0, 0, time.UTC))
	}
	intervals := make([]OverlapInterval, 0, len(dates))
	for _, date := range dates {
		start, end, err := ResolveIntervalUTC(zone, date, record.StartLocalTime, record.DurationMinutes)
		if err != nil {
			return nil, fmt.Errorf("resolve preview occurrence %s: %w", date.Format(time.DateOnly), err)
		}
		intervals = append(intervals, OverlapInterval{
			OccurrenceKey: "preview:" + date.Format(time.DateOnly), OriginalLocalDate: date,
			StartsAt: start, EndsAt: end,
		})
	}
	return intervals, nil
}

func evaluateOverlapWarnings(proposed, existing []OverlapInterval) []OverlapWarning {
	warnings := make([]OverlapWarning, 0)
	seen := make(map[string]struct{})
	for _, first := range proposed {
		for _, second := range existing {
			if first.OccurrenceKey == second.OccurrenceKey || !first.StartsAt.Before(second.EndsAt) || !second.StartsAt.Before(first.EndsAt) {
				continue
			}
			start, end := maxTime(first.StartsAt, second.StartsAt), minTime(first.EndsAt, second.EndsAt)
			firstKey, secondKey := first.OccurrenceKey, second.OccurrenceKey
			firstName, secondName := first.CircleName, second.CircleName
			swapped := secondKey < firstKey
			if swapped {
				firstKey, secondKey, firstName, secondName = secondKey, firstKey, secondName, firstName
			}
			startText, endText := start.UTC().Format(time.RFC3339Nano), end.UTC().Format(time.RFC3339Nano)
			firstCircleID, secondCircleID := first.CircleID, second.CircleID
			if swapped {
				firstCircleID, secondCircleID = secondCircleID, firstCircleID
			}
			id := warningID(firstCircleID, secondCircleID, firstKey, secondKey, startText, endText)
			if _, ok := seen[id]; ok {
				continue
			}
			seen[id] = struct{}{}
			warnings = append(warnings, OverlapWarning{
				WarningID: id, FirstOccurrenceKey: firstKey, SecondOccurrenceKey: secondKey,
				FirstCircleName: firstName, SecondCircleName: secondName,
				OverlapStartsAt: startText, OverlapEndsAt: endText,
			})
		}
	}
	sort.Slice(warnings, func(i, j int) bool { return warnings[i].WarningID < warnings[j].WarningID })
	return warnings
}

func warningID(firstCircle, secondCircle, first, second, start, end string) string {
	sum := sha256.Sum256([]byte(firstCircle + "\x00" + secondCircle + "\x00" + first + "\x00" + second + "\x00" + start + "\x00" + end))
	return hex.EncodeToString(sum[:])
}

func maxTime(a, b time.Time) time.Time {
	if a.After(b) {
		return a
	}
	return b
}

func minTime(a, b time.Time) time.Time {
	if a.Before(b) {
		return a
	}
	return b
}
