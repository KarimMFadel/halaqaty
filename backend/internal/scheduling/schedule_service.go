package scheduling

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"
)

var (
	// ErrPastPlannedTime rejects writes that would create or rewrite past plans.
	ErrPastPlannedTime = errors.New("planned time is in the past")
	// ErrOccurrenceStarted protects active and completed session history.
	ErrOccurrenceStarted = errors.New("occurrence has already started")
	// ErrOccurrenceNotFound means the original date is not part of this schedule.
	ErrOccurrenceNotFound = errors.New("occurrence not found")
	// ErrInvalidScheduleCommand reports invalid mutation input.
	ErrInvalidScheduleCommand = errors.New("invalid schedule command")
)

// CreateScheduleCommand carries authenticated scope and the first recurrence.
type CreateScheduleCommand struct {
	ActorID, CircleID, IdempotencyKey string
	Plan                              RevisionRecord
	ConfirmOverlaps                   bool
	ConfirmedWarningIDs               []string
}

// ChangeScheduleCommand changes the series from an inclusive local boundary.
type ChangeScheduleCommand struct {
	ActorID, CircleID, ScheduleID, IdempotencyKey string
	ExpectedVersion                               int
	EffectiveLocalDate                            time.Time
	Plan                                          RevisionRecord
	Stop                                          bool
	ConfirmOverlaps                               bool
	ConfirmedWarningIDs                           []string
}

// ChangeOccurrenceCommand patches one stable logical occurrence. Nil fields
// retain earlier individual overrides; false Cancelled explicitly restores it.
type ChangeOccurrenceCommand struct {
	ActorID, CircleID, ScheduleID, IdempotencyKey      string
	OriginalLocalDate                                  time.Time
	ExpectedSeriesVersion, ExpectedOccurrenceVersion   int
	ReplacementLocalDate                               *time.Time
	ReplacementStartLocalTime, ReplacementEndLocalTime *LocalClock
	ReplacementDurationMinutes                         *int
	Title                                              *string
	Cancelled                                          *bool
	ConfirmOverlaps                                    bool
	ConfirmedWarningIDs                                []string
}

// ScheduleView combines stable identity and its latest retained plan.
type ScheduleView struct {
	Schedule
	Plan RevisionRecord
}

// ScheduleService implements US1 persistence, authorization and US4 overlap confirmation.
type ScheduleService struct {
	repo    *Repository
	overlap *OverlapService
	now     func() time.Time
}

// NewScheduleService constructs the recurring-plan service.
func NewScheduleService(repo *Repository) *ScheduleService {
	return &ScheduleService{repo: repo, overlap: NewOverlapService(repo), now: time.Now}
}

// List returns retained schedules to a current member, including archived circles.
func (s *ScheduleService) List(ctx context.Context, circleID, actorID string) ([]ScheduleView, error) {
	views := []ScheduleView{}
	err := s.repo.withTx(ctx, func(q dbQuerier) error {
		if _, err := RequireCircleRead(ctx, q, circleID, actorID); err != nil {
			return err
		}
		repo := &Repository{tx: q}
		schedules, err := repo.ListSchedules(ctx, circleID)
		if err != nil {
			return err
		}
		ids := make([]string, 0, len(schedules))
		for _, sch := range schedules {
			ids = append(ids, sch.ID)
		}
		revisions, err := repo.LoadRevisions(ctx, ids)
		if err != nil {
			return err
		}
		for _, sch := range schedules {
			records := revisions[sch.ID]
			if len(records) == 0 {
				return fmt.Errorf("schedule has no revision: %w", ErrInvalidRevision)
			}
			views = append(views, ScheduleView{Schedule: sch, Plan: records[len(records)-1]})
		}
		return nil
	})
	return views, err
}

// Create atomically stores a plan and its actor-scoped replay identity.
func (s *ScheduleService) Create(ctx context.Context, cmd CreateScheduleCommand) (Schedule, error) {
	var created Schedule
	err := s.mutate(ctx, cmd.ActorID, cmd.CircleID, cmd.IdempotencyKey, "create", true, cmd, func(repo *Repository) (string, error) {
		record := cmd.Plan
		record.Version = 1
		record.EffectiveLocalDate = record.AnchorLocalDate
		if err := validateNewPlan(record, s.now()); err != nil {
			return "", err
		}
		if err := s.overlap.Check(ctx, repo.tx, cmd.ActorID, cmd.CircleID, record, "", cmd.ConfirmOverlaps, cmd.ConfirmedWarningIDs); err != nil {
			return "", err
		}
		var err error
		created, err = repo.CreateSchedule(ctx, cmd.CircleID, cmd.ActorID, record)
		return created.ID, err
	}, func(repo *Repository, id string) error {
		var err error
		created, err = repo.GetSchedule(ctx, id)
		return err
	})
	return created, err
}

// Change serializes series edits/stops with occurrence writes on the parent row.
func (s *ScheduleService) Change(ctx context.Context, cmd ChangeScheduleCommand) (Schedule, error) {
	var changed Schedule
	err := s.mutate(ctx, cmd.ActorID, cmd.CircleID, cmd.IdempotencyKey, "change", !cmd.Stop, cmd, func(repo *Repository) (string, error) {
		sch, err := lockSchedule(ctx, repo.tx, cmd.CircleID, cmd.ScheduleID)
		if err != nil {
			return "", err
		}
		if cmd.ExpectedVersion < 1 || sch.CurrentVersion != cmd.ExpectedVersion {
			return "", ErrScheduleConflict
		}
		revisions, err := repo.LoadRevisions(ctx, []string{sch.ID})
		if err != nil {
			return "", err
		}
		records := revisions[sch.ID]
		if err := validateSeriesBoundary(records, cmd.EffectiveLocalDate, s.now()); err != nil {
			return "", err
		}
		if cmd.Stop {
			changed, err = repo.StopSchedule(ctx, sch.ID, cmd.ExpectedVersion, cmd.EffectiveLocalDate)
		} else {
			record := cmd.Plan
			record.Version = cmd.ExpectedVersion + 1
			record.EffectiveLocalDate = cmd.EffectiveLocalDate
			if err = validateNewPlan(record, s.now()); err != nil {
				return "", err
			}
			if err = s.overlap.Check(ctx, repo.tx, cmd.ActorID, cmd.CircleID, record, sch.ID, cmd.ConfirmOverlaps, cmd.ConfirmedWarningIDs); err != nil {
				return "", err
			}
			changed, err = repo.AppendRevision(ctx, sch.ID, cmd.ExpectedVersion, record)
			records = append(records, record)
		}
		if err != nil {
			return "", err
		}
		if err = s.updateSeriesDetails(ctx, repo, changed, records); err != nil {
			return "", err
		}
		return changed.ID, nil
	}, func(repo *Repository, id string) error {
		var err error
		changed, err = repo.GetSchedule(ctx, id)
		return err
	})
	return changed, err
}

// ChangeOccurrence applies one exception and any materialized detail together.
func (s *ScheduleService) ChangeOccurrence(ctx context.Context, cmd ChangeOccurrenceCommand) (OccurrenceException, error) {
	var changed OccurrenceException
	serializable := cmd.Cancelled == nil || !*cmd.Cancelled
	err := s.mutate(ctx, cmd.ActorID, cmd.CircleID, cmd.IdempotencyKey, "occurrence", serializable, cmd, func(repo *Repository) (string, error) {
		sch, err := lockSchedule(ctx, repo.tx, cmd.CircleID, cmd.ScheduleID)
		if err != nil {
			return "", err
		}
		if cmd.ExpectedSeriesVersion < 1 || sch.CurrentVersion != cmd.ExpectedSeriesVersion {
			return "", ErrScheduleConflict
		}
		records, err := repo.LoadRevisions(ctx, []string{sch.ID})
		if err != nil {
			return "", err
		}
		prior, err := findException(ctx, repo, sch.ID, cmd.OriginalLocalDate)
		if err != nil {
			return "", err
		}
		var priorPtr *OccurrenceException
		if prior.ScheduleID != "" {
			priorPtr = &prior
		}
		record, _, ok := resolveOccurrence(sch, records[sch.ID], priorPtr, cmd.OriginalLocalDate)
		if !ok {
			return "", ErrOccurrenceNotFound
		}
		if err = lockUnstartedOccurrence(ctx, repo.tx, sch.ID, cmd.OriginalLocalDate); err != nil {
			return "", err
		}
		if cmd.ExpectedOccurrenceVersion < 0 || prior.Version != cmd.ExpectedOccurrenceVersion {
			return "", ErrScheduleConflict
		}
		prior.ScheduleID = sch.ID
		prior.OriginalLocalDate = cmd.OriginalLocalDate
		prior.UpdatedBy = cmd.ActorID
		applyOccurrencePatch(&prior, cmd, s.now())
		if err = validateException(prior); err != nil {
			return "", err
		}
		if prior.ReplacementTitle != nil {
			if err = validateTitle(*prior.ReplacementTitle); err != nil {
				return "", err
			}
		}
		// A move cannot resurrect an occurrence inside the stopped range.
		if sch.StoppedFromLocalDate != nil {
			_, effective := resolvedException(record, prior)
			if !civilOf(effective).Before(civilOf(*sch.StoppedFromLocalDate)) {
				return "", fmt.Errorf("replacement date at/after stop boundary: %w", ErrInvalidScheduleCommand)
			}
		}
		if err = validateOccurrenceFuture(record, prior, s.now()); err != nil {
			return "", err
		}
		if prior.CancelledAt == nil && (cmd.Cancelled == nil || !*cmd.Cancelled) {
			effectiveRecord, effectiveDate := resolvedException(record, prior)
			plan := RevisionRecord{
				Revision: Revision{Version: effectiveRecord.Version, Mode: ModeSelectedDates, AnchorLocalDate: effectiveDate, EffectiveLocalDate: effectiveDate, SelectedDates: []time.Time{effectiveDate}},
				Title:    effectiveRecord.Title, StartLocalTime: effectiveRecord.StartLocalTime, EndLocalTime: effectiveRecord.EndLocalTime,
				DurationMinutes: effectiveRecord.DurationMinutes, Timezone: effectiveRecord.Timezone,
			}
			if err := s.overlap.Check(ctx, repo.tx, cmd.ActorID, cmd.CircleID, plan, OccurrenceKey(sch.ID, cmd.OriginalLocalDate), cmd.ConfirmOverlaps, cmd.ConfirmedWarningIDs); err != nil {
				return "", err
			}
		}
		changed, err = repo.UpsertException(ctx, prior)
		if err != nil {
			return "", err
		}
		if err = writeMaterializedOccurrence(ctx, repo.tx, record, changed); err != nil {
			return "", err
		}
		return sch.ID, nil
	}, func(repo *Repository, id string) error {
		var err error
		changed, err = findException(ctx, repo, id, cmd.OriginalLocalDate)
		return err
	})
	return changed, err
}

// OccurrenceView is the resolved calendar projection of one logical
// occurrence: the stable identity, caller-visible circle name, effective plan
// values, resolved UTC interval and any materialized F-005 session id.
type OccurrenceView struct {
	ScheduleID        string
	CircleID          string
	CircleName        string
	OriginalLocalDate time.Time
	Title             string
	StartsAt          time.Time
	EndsAt            time.Time
	Timezone          string
	SessionID         *string
	Cancelled         bool
}

// OccurrenceView returns the calendar projection of one logical occurrence to
// a current circle member, resolving the governing revision, any retained
// exception and the stop boundary. It is the read half of the occurrence
// patch response; the mutation runs in ChangeOccurrence beforehand.
func (s *ScheduleService) OccurrenceView(ctx context.Context, circleID, scheduleID, actorID string, original time.Time) (OccurrenceView, error) {
	var view OccurrenceView
	err := s.repo.withTx(ctx, func(q dbQuerier) error {
		if _, err := RequireCircleRead(ctx, q, circleID, actorID); err != nil {
			return err
		}
		repo := &Repository{tx: q}
		sch, err := readSchedule(ctx, q, scheduleID)
		if err != nil {
			return err
		}
		if sch.CircleID != circleID {
			return ErrScheduleNotFound
		}
		revisions, err := repo.LoadRevisions(ctx, []string{sch.ID})
		if err != nil {
			return err
		}
		exception, err := findException(ctx, repo, sch.ID, original)
		if err != nil {
			return err
		}
		var exceptionPtr *OccurrenceException
		if exception.ScheduleID != "" {
			exceptionPtr = &exception
		}
		record, _, ok := resolveOccurrence(sch, revisions[sch.ID], exceptionPtr, original)
		if !ok {
			return ErrOccurrenceNotFound
		}
		// A virtual occurrence has no exception row; resolve it at its
		// original local date, mirroring updateSeriesDetails.
		if exception.ScheduleID == "" {
			exception = OccurrenceException{ScheduleID: sch.ID, OriginalLocalDate: original}
		}
		resolved, date := resolvedException(record, exception)
		loc, err := LoadLocation(resolved.Timezone)
		if err != nil {
			return err
		}
		start, end, err := ResolveIntervalUTC(loc, date, resolved.StartLocalTime, resolved.DurationMinutes)
		if err != nil {
			return err
		}
		var circleName string
		if err := q.QueryRow(ctx, getCircleNameQuery, circleID).Scan(&circleName); err != nil {
			return fmt.Errorf("read circle name: %w", err)
		}
		var sessionID *string
		var materialized string
		if err := q.QueryRow(ctx, getOccurrenceSessionIDQuery, sch.ID, civilOf(original)).Scan(&materialized); err != nil {
			if !errors.Is(err, pgx.ErrNoRows) {
				return fmt.Errorf("read occurrence session: %w", err)
			}
		} else {
			sessionID = &materialized
		}
		view = OccurrenceView{
			ScheduleID:        sch.ID,
			CircleID:          circleID,
			CircleName:        circleName,
			OriginalLocalDate: civilOf(original),
			Title:             resolved.Title,
			StartsAt:          start,
			EndsAt:            end,
			Timezone:          resolved.Timezone,
			SessionID:         sessionID,
			Cancelled:         exception.CancelledAt != nil,
		}
		return nil
	})
	return view, err
}

func (s *ScheduleService) mutate(ctx context.Context, actor, circle, key, operation string, serializable bool, input any, execute func(*Repository) (string, error), replay func(*Repository, string) error) error {
	if strings.TrimSpace(key) == "" || len(key) > 200 {
		return ErrInvalidScheduleCommand
	}
	encoded, err := json.Marshal(input)
	if err != nil {
		return fmt.Errorf("encode schedule command: %w", err)
	}
	fingerprint := fmt.Sprintf("%s:%x", operation, sha256.Sum256(encoded))
	mutate := s.repo.withTx
	if serializable {
		mutate = s.repo.withSerializableTx
	}
	return mutate(ctx, func(q dbQuerier) error {
		if _, err := RequireCircleManage(ctx, q, circle, actor); err != nil {
			return err
		}
		repo := &Repository{tx: q}
		record, replayed, err := ReplayOrExecute(ctx, q, actor, key, fingerprint, func(context.Context) (int, *string, error) {
			id, err := execute(repo)
			status := http.StatusOK
			if operation == "create" {
				status = http.StatusCreated
			}
			return status, &id, err
		})
		if err != nil {
			return err
		}
		if replayed {
			if record.ResponseResourceID == nil {
				return fmt.Errorf("missing replay resource: %w", ErrInvalidScheduleCommand)
			}
			return replay(repo, *record.ResponseResourceID)
		}
		return nil
	})
}

func lockSchedule(ctx context.Context, q dbQuerier, circle, id string) (Schedule, error) {
	sch, err := scanSchedule(q.QueryRow(ctx, lockScheduleQuery, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return Schedule{}, ErrScheduleNotFound
	}
	if err != nil {
		return Schedule{}, fmt.Errorf("lock schedule: %w", err)
	}
	if sch.CircleID != circle {
		return Schedule{}, ErrScheduleNotFound
	}
	return sch, nil
}

func validateTitle(title string) error {
	if strings.TrimSpace(title) != title || title == "" || utf8.RuneCountInString(title) > 200 {
		return ErrInvalidScheduleCommand
	}
	return nil
}

func validateNewPlan(record RevisionRecord, now time.Time) error {
	if record.AnchorLocalDate.IsZero() || record.EffectiveLocalDate.IsZero() {
		return ErrInvalidScheduleCommand
	}
	if err := validateRevisionRecord(record); err != nil {
		return err
	}
	if record.Title != "" {
		if err := validateTitle(record.Title); err != nil {
			return err
		}
	}
	loc, err := LoadLocation(record.Timezone)
	if err != nil {
		return err
	}
	// Recurrence anchors may be retained during series edits; the first affected
	// start, rather than an old anchor, is the present/future boundary.
	date := maxCivil(record.AnchorLocalDate, record.EffectiveLocalDate)
	if ResolveStartUTC(loc, date, record.StartLocalTime).Before(now) {
		return ErrPastPlannedTime
	}
	for _, date := range record.SelectedDates {
		if ResolveStartUTC(loc, date, record.StartLocalTime).Before(now) {
			return ErrPastPlannedTime
		}
	}
	return nil
}

func validateSeriesBoundary(records []RevisionRecord, date, now time.Time) error {
	if date.IsZero() || len(records) == 0 {
		return ErrInvalidScheduleCommand
	}
	for _, record := range records {
		loc, err := LoadLocation(record.Timezone)
		if err != nil {
			return err
		}
		if civilOf(date).Before(civilOf(now.In(loc))) {
			return ErrPastPlannedTime
		}
	}
	// A same-day revision must not rewrite an earlier virtual occurrence.
	if record, ok := governingRecord(records, date); ok && len(record.OccurrenceDates(date, date)) > 0 {
		loc, err := LoadLocation(record.Timezone)
		if err != nil {
			return err
		}
		if ResolveStartUTC(loc, date, record.StartLocalTime).Before(now) {
			return ErrPastPlannedTime
		}
	}
	return nil
}

func governingRecord(records []RevisionRecord, date time.Time) (RevisionRecord, bool) {
	var best RevisionRecord
	found := false
	for _, record := range records {
		if !civilOf(record.EffectiveLocalDate).After(civilOf(date)) && (!found || record.Version > best.Version) {
			best = record
			found = true
		}
	}
	return best, found
}

// resolveOccurrence reports whether the stable occurrence identity
// (schedule_id, original local date) currently exists, and returns the
// governing revision plus the occurrence's effective local date. The governing
// revision is pinned to the original date, but a retained exception moves the
// effective date, and the stop boundary applies to that effective date: an
// occurrence moved ahead of a stop survives, one moved past it does not.
func resolveOccurrence(sch Schedule, records []RevisionRecord, exception *OccurrenceException, originalDate time.Time) (RevisionRecord, time.Time, bool) {
	record, ok := governingRecord(records, originalDate)
	if !ok || len(record.OccurrenceDates(originalDate, originalDate)) != 1 {
		return RevisionRecord{}, time.Time{}, false
	}
	effective := civilOf(originalDate)
	if exception != nil && exception.ReplacementLocalDate != nil {
		effective = civilOf(*exception.ReplacementLocalDate)
	}
	if sch.StoppedFromLocalDate != nil && !effective.Before(civilOf(*sch.StoppedFromLocalDate)) {
		return RevisionRecord{}, time.Time{}, false
	}
	return record, effective, true
}

func findException(ctx context.Context, repo *Repository, id string, date time.Time) (OccurrenceException, error) {
	exceptions, err := repo.ListExceptions(ctx, id)
	if err != nil {
		return OccurrenceException{}, err
	}
	for _, exception := range exceptions {
		if civilOf(exception.OriginalLocalDate).Equal(civilOf(date)) {
			return exception, nil
		}
	}
	return OccurrenceException{}, nil
}

func lockUnstartedOccurrence(ctx context.Context, q dbQuerier, id string, date time.Time) error {
	rows, err := q.Query(ctx, lockOccurrenceSessionQuery, id, date)
	if err != nil {
		return fmt.Errorf("lock occurrence session: %w", err)
	}
	for rows.Next() {
		var started bool
		if err = rows.Scan(&started); err != nil {
			rows.Close()
			return fmt.Errorf("read occurrence lifecycle: %w", err)
		}
		if started {
			rows.Close()
			return ErrOccurrenceStarted
		}
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return fmt.Errorf("lock occurrence session: %w", err)
	}
	return nil
}

func applyOccurrencePatch(exception *OccurrenceException, cmd ChangeOccurrenceCommand, now time.Time) {
	if cmd.ReplacementLocalDate != nil {
		exception.ReplacementLocalDate = cmd.ReplacementLocalDate
	}
	if cmd.ReplacementStartLocalTime != nil {
		exception.ReplacementStartLocalTime = cmd.ReplacementStartLocalTime
	}
	if cmd.ReplacementEndLocalTime != nil {
		exception.ReplacementEndLocalTime = cmd.ReplacementEndLocalTime
	}
	if cmd.ReplacementDurationMinutes != nil {
		exception.ReplacementDurationMinutes = cmd.ReplacementDurationMinutes
	}
	if cmd.Title != nil {
		exception.ReplacementTitle = cmd.Title
	}
	if cmd.Cancelled != nil {
		exception.CancelledAt = nil
		if *cmd.Cancelled {
			exception.CancelledAt = &now
		}
	}
}

func resolvedException(record RevisionRecord, exception OccurrenceException) (RevisionRecord, time.Time) {
	date := exception.OriginalLocalDate
	if exception.ReplacementLocalDate != nil {
		date = *exception.ReplacementLocalDate
	}
	if exception.ReplacementStartLocalTime != nil {
		record.StartLocalTime = *exception.ReplacementStartLocalTime
		record.EndLocalTime = *exception.ReplacementEndLocalTime
		record.DurationMinutes = *exception.ReplacementDurationMinutes
	}
	if exception.ReplacementTitle != nil {
		record.Title = *exception.ReplacementTitle
	}
	if record.Title == "" {
		record.Title = defaultScheduleTitle
	}
	return record, date
}

func validateOccurrenceFuture(record RevisionRecord, exception OccurrenceException, now time.Time) error {
	record, date := resolvedException(record, exception)
	if date.IsZero() {
		return ErrInvalidScheduleCommand
	}
	loc, err := LoadLocation(record.Timezone)
	if err != nil {
		return err
	}
	if ResolveStartUTC(loc, date, record.StartLocalTime).Before(now) {
		return ErrPastPlannedTime
	}
	return nil
}

func writeMaterializedOccurrence(ctx context.Context, q dbQuerier, record RevisionRecord, exception OccurrenceException) error {
	record, date := resolvedException(record, exception)
	loc, err := LoadLocation(record.Timezone)
	if err != nil {
		return err
	}
	start, end, err := ResolveIntervalUTC(loc, date, record.StartLocalTime, record.DurationMinutes)
	if err != nil {
		return err
	}
	if _, err = q.Exec(ctx, updatePlannedDetailQuery, exception.ScheduleID, exception.OriginalLocalDate, record.Title, formatLocalClock(record.StartLocalTime), formatLocalClock(record.EndLocalTime), record.DurationMinutes, end, record.Timezone, exception.CancelledAt); err != nil {
		return fmt.Errorf("update planned detail: %w", err)
	}
	if _, err = q.Exec(ctx, updatePlannedStartQuery, exception.ScheduleID, exception.OriginalLocalDate, start); err != nil {
		return fmt.Errorf("update planned start: %w", err)
	}
	return nil
}

// updateSeriesDetails rewrites or cancels every unstarted materialized detail
// after a series change or stop, inside the same parent-locked transaction.
// Supersession has already removed the exceptions the change replaces, so each
// detail is resolved against the post-change revisions, the stop boundary and
// any retained (moved) exception; details whose occurrence no longer exists
// are cancelled. Started/completed rows are excluded by the lock query.
func (s *ScheduleService) updateSeriesDetails(ctx context.Context, repo *Repository, sch Schedule, records []RevisionRecord) error {
	exceptions, err := repo.ListExceptions(ctx, sch.ID)
	if err != nil {
		return err
	}
	byOriginal := make(map[time.Time]*OccurrenceException, len(exceptions))
	for i := range exceptions {
		byOriginal[civilOf(exceptions[i].OriginalLocalDate)] = &exceptions[i]
	}
	rows, err := repo.tx.Query(ctx, listUnstartedDetailsQuery, sch.ID)
	if err != nil {
		return fmt.Errorf("lock series details: %w", err)
	}
	var dates []time.Time
	for rows.Next() {
		var id string
		var date time.Time
		if err = rows.Scan(&id, &date); err != nil {
			rows.Close()
			return fmt.Errorf("read series detail: %w", err)
		}
		dates = append(dates, date)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return fmt.Errorf("read series details: %w", err)
	}
	for _, date := range dates {
		exception := byOriginal[civilOf(date)]
		record, _, ok := resolveOccurrence(sch, records, exception, date)
		if !ok {
			if _, err = repo.tx.Exec(ctx, cancelPlannedDetailQuery, sch.ID, date, s.now()); err != nil {
				return fmt.Errorf("cancel superseded detail: %w", err)
			}
			continue
		}
		resolved := OccurrenceException{ScheduleID: sch.ID, OriginalLocalDate: date}
		if exception != nil {
			resolved = *exception
		}
		if err = writeMaterializedOccurrence(ctx, repo.tx, record, resolved); err != nil {
			return err
		}
	}
	return nil
}
