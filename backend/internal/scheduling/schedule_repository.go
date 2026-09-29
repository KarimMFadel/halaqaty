package scheduling

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	// ErrScheduleNotFound means no schedules row carries the requested id.
	ErrScheduleNotFound = errors.New("schedule not found")
	// ErrScheduleConflict means the schedule's current_version no longer
	// matches the caller's expectation; the conflicting change is preserved.
	ErrScheduleConflict = errors.New("schedule modified concurrently")
	// ErrInvalidException means an occurrence exception violates its
	// all-or-nothing replacement invariants.
	ErrInvalidException = errors.New("invalid occurrence exception")
)

// defaultScheduleTitle mirrors the schedule_revisions title default (FR-003).
const defaultScheduleTitle = "Circle Session"

// dbQuerier is satisfied by *pgxpool.Pool and pgx.Tx, so repository helpers
// can run standalone or inside a caller-managed transaction.
type dbQuerier interface {
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

// Schedule mirrors one schedules row: the stable identity, CAS counter and
// stop boundary of a recurring entry.
type Schedule struct {
	ID                   string
	CircleID             string
	CreatedBy            string
	CurrentVersion       int
	StoppedFromLocalDate *time.Time
	CreatedAt            time.Time
	UpdatedAt            time.Time
}

// RevisionRecord is the persisted form of one effective-dated revision: the
// recurrence pattern plus the retained local clocks, duration, planning
// timezone and title. The embedded Revision feeds the occurrence engine.
type RevisionRecord struct {
	Revision
	Title           string
	StartLocalTime  LocalClock
	EndLocalTime    LocalClock
	DurationMinutes int
	Timezone        string
}

// OccurrenceException mirrors one schedule_occurrence_exceptions row keyed by
// the stable (ScheduleID, OriginalLocalDate) identity. Version is the row CAS
// counter; SeriesVersion stamps the schedule version the edit was made under.
type OccurrenceException struct {
	ScheduleID                 string
	OriginalLocalDate          time.Time
	SeriesVersion              int
	Version                    int
	ReplacementLocalDate       *time.Time
	ReplacementStartLocalTime  *LocalClock
	ReplacementEndLocalTime    *LocalClock
	ReplacementDurationMinutes *int
	ReplacementTitle           *string
	CancelledAt                *time.Time
	UpdatedBy                  string
	CreatedAt                  time.Time
	UpdatedAt                  time.Time
}

// Repository is the PostgreSQL persistence boundary for F-006 schedules.
type Repository struct {
	pool *pgxpool.Pool
	tx   dbQuerier
}

// NewScheduleRepository constructs a schedule repository on a pgx pool.
func NewScheduleRepository(pool *pgxpool.Pool) *Repository {
	return &Repository{pool: pool}
}

// withTx runs fn in one transaction and commits on success; domain errors
// pass through unwrapped so callers can errors.Is them.
func (r *Repository) withTx(ctx context.Context, fn func(q dbQuerier) error) error {
	if r.tx != nil {
		return fn(r.tx)
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin schedule transaction: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := fn(tx); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit schedule transaction: %w", err)
	}
	return nil
}

// CreateSchedule persists a new recurring entry and its first revision (with
// selected dates) in one transaction. The revision version is forced to 1.
func (r *Repository) CreateSchedule(ctx context.Context, circleID, createdBy string, record RevisionRecord) (Schedule, error) {
	record.Version = 1
	if err := validateRevisionRecord(record); err != nil {
		return Schedule{}, err
	}
	var created Schedule
	err := r.withTx(ctx, func(q dbQuerier) error {
		row := q.QueryRow(ctx, insertScheduleQuery, circleID, createdBy)
		schedule, err := scanSchedule(row)
		if err != nil {
			return fmt.Errorf("insert schedule: %w", err)
		}
		record.ScheduleID = schedule.ID
		if err := insertRevisionRecord(ctx, q, record); err != nil {
			return err
		}
		created = schedule
		return nil
	})
	if err != nil {
		return Schedule{}, err
	}
	return created, nil
}

// AppendRevision applies a series change: it CAS-bumps the schedule from
// expectedVersion, appends the new revision (forced to expectedVersion+1) and
// supersedes unstarted exceptions at/after its effective local date, all in
// one transaction. Earlier revisions are retained for past history.
func (r *Repository) AppendRevision(ctx context.Context, scheduleID string, expectedVersion int, record RevisionRecord) (Schedule, error) {
	record.ScheduleID = scheduleID
	record.Version = expectedVersion + 1
	if err := validateRevisionRecord(record); err != nil {
		return Schedule{}, err
	}
	var updated Schedule
	err := r.withTx(ctx, func(q dbQuerier) error {
		if err := bumpScheduleVersion(ctx, q, scheduleID, expectedVersion); err != nil {
			return err
		}
		if err := insertRevisionRecord(ctx, q, record); err != nil {
			return err
		}
		if err := supersedeUnstartedExceptions(ctx, q, scheduleID, record.EffectiveLocalDate); err != nil {
			return err
		}
		schedule, err := readSchedule(ctx, q, scheduleID)
		if err != nil {
			return err
		}
		updated = schedule
		return nil
	})
	if err != nil {
		return Schedule{}, err
	}
	return updated, nil
}

// StopSchedule ends future occurrences from stoppedFromLocalDate: it CAS-bumps
// the schedule, stores the stop boundary and supersedes unstarted exceptions
// at/after it. Past revisions and started history are retained.
func (r *Repository) StopSchedule(ctx context.Context, scheduleID string, expectedVersion int, stoppedFromLocalDate time.Time) (Schedule, error) {
	var updated Schedule
	err := r.withTx(ctx, func(q dbQuerier) error {
		tag, err := q.Exec(ctx, stopScheduleQuery, scheduleID, stoppedFromLocalDate, expectedVersion)
		if err != nil {
			return fmt.Errorf("stop schedule: %w", err)
		}
		if tag.RowsAffected() == 0 {
			return scheduleCASFailure(ctx, q, scheduleID)
		}
		if err := supersedeUnstartedExceptions(ctx, q, scheduleID, stoppedFromLocalDate); err != nil {
			return err
		}
		schedule, err := readSchedule(ctx, q, scheduleID)
		if err != nil {
			return err
		}
		updated = schedule
		return nil
	})
	if err != nil {
		return Schedule{}, err
	}
	return updated, nil
}

// UpsertException inserts or replaces the exception for one logical
// occurrence, keeping the stable (schedule_id, original_local_date) identity:
// the row version increments on update and series_version is stamped with the
// schedule's current version. The caller's SeriesVersion/Version are derived,
// not trusted.
func (r *Repository) UpsertException(ctx context.Context, exception OccurrenceException) (OccurrenceException, error) {
	if err := validateException(exception); err != nil {
		return OccurrenceException{}, err
	}
	row := r.querier().QueryRow(ctx, upsertOccurrenceExceptionQuery,
		exception.ScheduleID,
		exception.OriginalLocalDate,
		optionalDate(exception.ReplacementLocalDate),
		optionalClock(exception.ReplacementStartLocalTime),
		optionalClock(exception.ReplacementEndLocalTime),
		optionalDuration(exception.ReplacementDurationMinutes),
		nullableString(exception.ReplacementTitle),
		exception.CancelledAt,
		exception.UpdatedBy,
	)
	if err := row.Scan(&exception.SeriesVersion, &exception.Version, &exception.CreatedAt, &exception.UpdatedAt); err != nil {
		return OccurrenceException{}, fmt.Errorf("upsert occurrence exception: %w", err)
	}
	return exception, nil
}

// GetSchedule reads one schedule by id.
func (r *Repository) GetSchedule(ctx context.Context, scheduleID string) (Schedule, error) {
	return readSchedule(ctx, r.querier(), scheduleID)
}

// ListSchedules reads a circle's entries through the (circle_id,
// stopped_from_local_date) index, oldest first.
func (r *Repository) ListSchedules(ctx context.Context, circleID string) ([]Schedule, error) {
	rows, err := r.querier().Query(ctx, listSchedulesByCircleQuery, circleID)
	if err != nil {
		return nil, fmt.Errorf("list circle schedules: %w", err)
	}
	defer rows.Close()
	var schedules []Schedule
	for rows.Next() {
		schedule, err := scanSchedule(rows)
		if err != nil {
			return nil, fmt.Errorf("scan circle schedule: %w", err)
		}
		schedules = append(schedules, schedule)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list circle schedules: %w", err)
	}
	return schedules, nil
}

// LoadRevisions reads all retained revisions (and their selected dates) for
// the given schedules, grouped by schedule id and ordered by version, so the
// occurrence engine can reproduce past and future history.
func (r *Repository) LoadRevisions(ctx context.Context, scheduleIDs []string) (map[string][]RevisionRecord, error) {
	if len(scheduleIDs) == 0 {
		return map[string][]RevisionRecord{}, nil
	}
	revisions := make(map[string][]RevisionRecord, len(scheduleIDs))
	rows, err := r.querier().Query(ctx, listScheduleRevisionsQuery, scheduleIDs)
	if err != nil {
		return nil, fmt.Errorf("load schedule revisions: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		record, err := scanRevisionRecord(rows)
		if err != nil {
			return nil, fmt.Errorf("scan schedule revision: %w", err)
		}
		revisions[record.ScheduleID] = append(revisions[record.ScheduleID], record)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("load schedule revisions: %w", err)
	}
	if err := r.loadSelectedDates(ctx, scheduleIDs, revisions); err != nil {
		return nil, err
	}
	return revisions, nil
}

// loadSelectedDates attaches schedule_selected_dates rows to selected-dates
// revisions already loaded into the revisions map.
func (r *Repository) loadSelectedDates(ctx context.Context, scheduleIDs []string, revisions map[string][]RevisionRecord) error {
	rows, err := r.querier().Query(ctx, listScheduleSelectedDatesQuery, scheduleIDs)
	if err != nil {
		return fmt.Errorf("load selected dates: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var scheduleID string
		var version int
		var date time.Time
		if err := rows.Scan(&scheduleID, &version, &date); err != nil {
			return fmt.Errorf("scan selected date: %w", err)
		}
		for i := range revisions[scheduleID] {
			if revisions[scheduleID][i].Version == version {
				revisions[scheduleID][i].SelectedDates = append(revisions[scheduleID][i].SelectedDates, date)
			}
		}
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("load selected dates: %w", err)
	}
	return nil
}

// ListExceptions reads one schedule's occurrence exceptions ordered by the
// stable original local date.
func (r *Repository) ListExceptions(ctx context.Context, scheduleID string) ([]OccurrenceException, error) {
	rows, err := r.querier().Query(ctx, listOccurrenceExceptionsQuery, scheduleID)
	if err != nil {
		return nil, fmt.Errorf("list occurrence exceptions: %w", err)
	}
	defer rows.Close()
	var exceptions []OccurrenceException
	for rows.Next() {
		exception, err := scanOccurrenceException(rows)
		if err != nil {
			return nil, fmt.Errorf("scan occurrence exception: %w", err)
		}
		exceptions = append(exceptions, exception)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list occurrence exceptions: %w", err)
	}
	return exceptions, nil
}

// validateRevisionRecord mirrors the schedule_revisions checks with domain
// errors before the statement reaches PostgreSQL.
func validateRevisionRecord(record RevisionRecord) error {
	if err := record.ValidatePattern(); err != nil {
		return err
	}
	if err := ValidatePlannedClocks(record.StartLocalTime, record.EndLocalTime, record.DurationMinutes); err != nil {
		return err
	}
	if _, err := LoadLocation(record.Timezone); err != nil {
		return err
	}
	return nil
}

// validateException enforces the all-or-nothing replacement clock triple and
// its nominal end-clock rule.
func validateException(exception OccurrenceException) error {
	start, end, duration := exception.ReplacementStartLocalTime, exception.ReplacementEndLocalTime, exception.ReplacementDurationMinutes
	if (start == nil) != (end == nil) || (start == nil) != (duration == nil) {
		return fmt.Errorf("replacement clock triple: %w", ErrInvalidException)
	}
	if start != nil {
		if err := ValidatePlannedClocks(*start, *end, *duration); err != nil {
			return err
		}
	}
	return nil
}

// bumpScheduleVersion applies the series-edit CAS; a zero-row update means a
// conflict (or a missing schedule), resolved by scheduleCASFailure.
func bumpScheduleVersion(ctx context.Context, q dbQuerier, scheduleID string, expectedVersion int) error {
	tag, err := q.Exec(ctx, bumpScheduleVersionQuery, scheduleID, expectedVersion)
	if err != nil {
		return fmt.Errorf("bump schedule version: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return scheduleCASFailure(ctx, q, scheduleID)
	}
	return nil
}

// scheduleCASFailure distinguishes a missing schedule from a lost CAS race so
// conflicting edits surface instead of silently losing a change.
func scheduleCASFailure(ctx context.Context, q dbQuerier, scheduleID string) error {
	_, err := readSchedule(ctx, q, scheduleID)
	if errors.Is(err, ErrScheduleNotFound) {
		return err
	}
	if err != nil {
		return fmt.Errorf("resolve schedule conflict: %w", err)
	}
	return ErrScheduleConflict
}

// supersedeUnstartedExceptions removes unstarted exception values at/after
// the boundary local date; started history is retained (ADR-026).
func supersedeUnstartedExceptions(ctx context.Context, q dbQuerier, scheduleID string, fromLocalDate time.Time) error {
	if _, err := q.Exec(ctx, supersedeUnstartedExceptionsQuery, scheduleID, fromLocalDate); err != nil {
		return fmt.Errorf("supersede unstarted exceptions: %w", err)
	}
	return nil
}

// insertRevisionRecord persists one revision and, for selected-dates mode, its
// deduplicated dates (FR-001: duplicates produce one occurrence).
func insertRevisionRecord(ctx context.Context, q dbQuerier, record RevisionRecord) error {
	title := record.Title
	if title == "" {
		title = defaultScheduleTitle
	}
	_, err := q.Exec(ctx, insertScheduleRevisionQuery,
		record.ScheduleID,
		record.Version,
		record.EffectiveLocalDate,
		string(record.Mode),
		title,
		record.AnchorLocalDate,
		formatLocalClock(record.StartLocalTime),
		formatLocalClock(record.EndLocalTime),
		record.DurationMinutes,
		record.Timezone,
		optionalDate(record.EndLocalDate),
		optionalCadence(record.WeekCadence),
		optionalWeekdays(record.Weekdays),
		optionalIntervalCount(record.IntervalCount),
		optionalIntervalUnit(record.IntervalUnit),
	)
	if err != nil {
		return fmt.Errorf("insert schedule revision: %w", err)
	}
	if record.Mode != ModeSelectedDates {
		return nil
	}
	seen := make(map[time.Time]bool, len(record.SelectedDates))
	for _, date := range record.SelectedDates {
		day := civilOf(date)
		if seen[day] {
			continue
		}
		seen[day] = true
		if _, err := q.Exec(ctx, insertScheduleSelectedDateQuery, record.ScheduleID, record.Version, day); err != nil {
			return fmt.Errorf("insert selected date: %w", err)
		}
	}
	return nil
}

func readSchedule(ctx context.Context, q dbQuerier, scheduleID string) (Schedule, error) {
	schedule, err := scanSchedule(q.QueryRow(ctx, getScheduleByIDQuery, scheduleID))
	if errors.Is(err, pgx.ErrNoRows) {
		return Schedule{}, ErrScheduleNotFound
	}
	if err != nil {
		return Schedule{}, fmt.Errorf("read schedule: %w", err)
	}
	return schedule, nil
}

// scannable is satisfied by pgx.Row and pgx.Rows.
type scannable interface {
	Scan(dest ...any) error
}

func scanSchedule(row scannable) (Schedule, error) {
	var schedule Schedule
	err := row.Scan(
		&schedule.ID, &schedule.CircleID, &schedule.CreatedBy, &schedule.CurrentVersion,
		&schedule.StoppedFromLocalDate, &schedule.CreatedAt, &schedule.UpdatedAt,
	)
	if err != nil {
		return Schedule{}, err
	}
	return schedule, nil
}

func scanRevisionRecord(row scannable) (RevisionRecord, error) {
	var record RevisionRecord
	var mode, startClock, endClock, timezone string
	var endLocalDate *time.Time
	var weekCadence *int16
	var weekdays []int16
	var intervalCount *int32
	var intervalUnit *string
	err := row.Scan(
		&record.ScheduleID, &record.Version, &record.EffectiveLocalDate, &mode, &record.Title,
		&record.AnchorLocalDate, &startClock, &endClock, &record.DurationMinutes, &timezone,
		&endLocalDate, &weekCadence, &weekdays, &intervalCount, &intervalUnit,
	)
	if err != nil {
		return RevisionRecord{}, err
	}
	record.Mode = RecurrenceMode(mode)
	record.Timezone = timezone
	record.EndLocalDate = endLocalDate
	if start, err := parseLocalClock(startClock); err != nil {
		return RevisionRecord{}, fmt.Errorf("start clock %q: %w", startClock, err)
	} else {
		record.StartLocalTime = start
	}
	if end, err := parseLocalClock(endClock); err != nil {
		return RevisionRecord{}, fmt.Errorf("end clock %q: %w", endClock, err)
	} else {
		record.EndLocalTime = end
	}
	if weekCadence != nil {
		record.WeekCadence = int(*weekCadence)
	}
	for _, day := range weekdays {
		record.Weekdays = append(record.Weekdays, int(day))
	}
	if intervalCount != nil {
		record.IntervalCount = int(*intervalCount)
	}
	if intervalUnit != nil {
		record.IntervalUnit = IntervalUnit(*intervalUnit)
	}
	return record, nil
}

func scanOccurrenceException(row scannable) (OccurrenceException, error) {
	var exception OccurrenceException
	var startClock, endClock *string
	err := row.Scan(
		&exception.ScheduleID, &exception.OriginalLocalDate, &exception.SeriesVersion, &exception.Version,
		&exception.ReplacementLocalDate, &startClock, &endClock,
		&exception.ReplacementDurationMinutes, &exception.ReplacementTitle, &exception.CancelledAt,
		&exception.UpdatedBy, &exception.CreatedAt, &exception.UpdatedAt,
	)
	if err != nil {
		return OccurrenceException{}, err
	}
	if startClock != nil {
		start, err := parseLocalClock(*startClock)
		if err != nil {
			return OccurrenceException{}, fmt.Errorf("replacement start clock %q: %w", *startClock, err)
		}
		exception.ReplacementStartLocalTime = &start
	}
	if endClock != nil {
		end, err := parseLocalClock(*endClock)
		if err != nil {
			return OccurrenceException{}, fmt.Errorf("replacement end clock %q: %w", *endClock, err)
		}
		exception.ReplacementEndLocalTime = &end
	}
	return exception, nil
}

// formatLocalClock renders a retained local clock for the TIME columns.
func formatLocalClock(clock LocalClock) string {
	return fmt.Sprintf("%02d:%02d", clock.Hour, clock.Minute)
}

// parseLocalClock parses the 'HH24:MI' projection of a TIME column.
func parseLocalClock(value string) (LocalClock, error) {
	parsed, err := time.Parse("15:04", value)
	if err != nil {
		return LocalClock{}, fmt.Errorf("local clock %q: %w", value, ErrInvalidLocalClock)
	}
	return LocalClock{Hour: parsed.Hour(), Minute: parsed.Minute()}, nil
}

func optionalDate(date *time.Time) any {
	if date == nil {
		return nil
	}
	return *date
}

func optionalClock(clock *LocalClock) any {
	if clock == nil {
		return nil
	}
	return formatLocalClock(*clock)
}

func optionalDuration(minutes *int) any {
	if minutes == nil {
		return nil
	}
	return int32(*minutes)
}

func nullableString(value *string) any {
	if value == nil {
		return nil
	}
	return *value
}

func optionalCadence(cadence int) any {
	if cadence == 0 {
		return nil
	}
	return int16(cadence)
}

func optionalWeekdays(days []int) any {
	if len(days) == 0 {
		return nil
	}
	values := make([]int16, len(days))
	for i, day := range days {
		values[i] = int16(day)
	}
	return values
}

func optionalIntervalCount(count int) any {
	if count == 0 {
		return nil
	}
	return int32(count)
}

func optionalIntervalUnit(unit IntervalUnit) any {
	if unit == "" {
		return nil
	}
	return string(unit)
}

func (r *Repository) querier() dbQuerier {
	if r.tx != nil {
		return r.tx
	}
	return r.pool
}
