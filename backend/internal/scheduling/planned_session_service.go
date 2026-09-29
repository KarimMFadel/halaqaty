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
	// ErrPlannedSessionNotFound means the id is not an unlinked planned session.
	ErrPlannedSessionNotFound = errors.New("planned session not found")
)

// PlannedSessionPlan is a one-off plan in its planning timezone.
type PlannedSessionPlan struct {
	LocalDate       time.Time
	StartLocalTime  LocalClock
	EndLocalTime    LocalClock
	DurationMinutes int
	Timezone        string
	Title           string
}

// CreatePlannedSessionCommand carries authenticated scope and one-off details.
type CreatePlannedSessionCommand struct {
	ActorID, CircleID, IdempotencyKey string
	Plan                              PlannedSessionPlan
	ConfirmOverlaps                   bool
	ConfirmedWarningIDs               []string
}

// ChangePlannedSessionCommand edits or cancels an unstarted one-off plan.
// Exactly one of Plan or Cancelled must be supplied.
type ChangePlannedSessionCommand struct {
	ActorID, SessionID, IdempotencyKey string
	ExpectedVersion                    int
	Plan                               *PlannedSessionPlan
	Cancelled                          *bool
	ConfirmOverlaps                    bool
	ConfirmedWarningIDs                []string
}

// PlannedSessionView contains the durable one-off plan and F-005 lifecycle.
type PlannedSessionView struct {
	ID, CircleID, CircleName, Title string
	StartsAt, EndsAt                time.Time
	StartLocalTime, EndLocalTime    LocalClock
	DurationMinutes                 int
	PlanningTimezone                string
	Version                         int
	Status                          string
	Cancelled                       bool
}

// PlannedSessionService persists one-off plans without changing F-005 status semantics.
type PlannedSessionService struct {
	repo    *Repository
	overlap *OverlapService
	now     func() time.Time
}

// NewPlannedSessionService constructs the one-off planning service.
func NewPlannedSessionService(repo *Repository) *PlannedSessionService {
	return &PlannedSessionService{repo: repo, overlap: NewOverlapService(repo), now: time.Now}
}

// Create stores a future one-off plan with its actor-scoped idempotency result.
func (s *PlannedSessionService) Create(ctx context.Context, cmd CreatePlannedSessionCommand) (PlannedSessionView, error) {
	if err := validatePlannedIdempotencyKey(cmd.IdempotencyKey); err != nil {
		return PlannedSessionView{}, err
	}
	fingerprint, err := plannedCommandFingerprint("create-one-off", cmd)
	if err != nil {
		return PlannedSessionView{}, err
	}
	return s.create(ctx, cmd, fingerprint)
}

// Change edits or cancels an unstarted one-off using detail-version CAS.
func (s *PlannedSessionService) Change(ctx context.Context, cmd ChangePlannedSessionCommand) (PlannedSessionView, error) {
	if err := validatePlannedIdempotencyKey(cmd.IdempotencyKey); err != nil {
		return PlannedSessionView{}, err
	}
	if cmd.ExpectedVersion < 1 || (cmd.Plan == nil) == (cmd.Cancelled == nil) {
		return PlannedSessionView{}, ErrInvalidScheduleCommand
	}
	fingerprint, err := plannedCommandFingerprint("change-one-off", cmd)
	if err != nil {
		return PlannedSessionView{}, err
	}
	return s.change(ctx, cmd, fingerprint)
}

func (s *PlannedSessionService) create(ctx context.Context, cmd CreatePlannedSessionCommand, fingerprint string) (PlannedSessionView, error) {
	var view PlannedSessionView
	err := s.repo.withSerializableTx(ctx, func(q dbQuerier) error {
		if _, err := RequireCircleManage(ctx, q, cmd.CircleID, cmd.ActorID); err != nil {
			return err
		}
		record, _, err := ReplayOrExecute(ctx, q, cmd.ActorID, cmd.IdempotencyKey, fingerprint,
			func(context.Context) (int, *string, error) { return s.insertOneOff(ctx, q, cmd) })
		if err != nil {
			return err
		}
		if record.ResponseResourceID == nil {
			return fmt.Errorf("missing replay resource: %w", ErrInvalidScheduleCommand)
		}
		view, err = loadOneOffPlannedSession(ctx, q, *record.ResponseResourceID)
		return err
	})
	return view, err
}

func (s *PlannedSessionService) insertOneOff(ctx context.Context, q dbQuerier, cmd CreatePlannedSessionCommand) (int, *string, error) {
	plan, start, end, err := validatePlannedSessionPlan(cmd.Plan, s.now())
	if err != nil {
		return 0, nil, err
	}
	if err := s.overlap.Check(ctx, q, cmd.ActorID, cmd.CircleID, plannedRevision(plan), "", cmd.ConfirmOverlaps, cmd.ConfirmedWarningIDs); err != nil {
		return 0, nil, err
	}
	var sessionID string
	if err := q.QueryRow(ctx, insertPlannedSessionQuery, cmd.CircleID, cmd.ActorID, start).Scan(&sessionID); err != nil {
		return 0, nil, fmt.Errorf("insert planned session: %w", err)
	}
	_, err = q.Exec(ctx, insertOneOffPlannedDetailsQuery, sessionID, plan.Title,
		formatLocalClock(plan.StartLocalTime), formatLocalClock(plan.EndLocalTime), plan.DurationMinutes, end, plan.Timezone)
	if err != nil {
		return 0, nil, fmt.Errorf("insert planned session details: %w", err)
	}
	return http.StatusCreated, &sessionID, nil
}

func (s *PlannedSessionService) change(ctx context.Context, cmd ChangePlannedSessionCommand, fingerprint string) (PlannedSessionView, error) {
	var view PlannedSessionView
	mutate := s.repo.withTx
	if cmd.Plan != nil || (cmd.Cancelled != nil && !*cmd.Cancelled) {
		mutate = s.repo.withSerializableTx
	}
	err := mutate(ctx, func(q dbQuerier) error {
		var err error
		view, err = s.changeInTransaction(ctx, q, cmd, fingerprint)
		return err
	})
	return view, err
}

func (s *PlannedSessionService) changeInTransaction(ctx context.Context, q dbQuerier, cmd ChangePlannedSessionCommand, fingerprint string) (PlannedSessionView, error) {
	circleID, err := oneOffCircle(ctx, q, cmd.SessionID)
	if err != nil {
		return PlannedSessionView{}, err
	}
	if _, err := q.Exec(ctx, lockOneOffSessionAdvisoryQuery, cmd.SessionID); err != nil {
		return PlannedSessionView{}, fmt.Errorf("lock planned session start: %w", err)
	}
	if _, err := RequireCircleManage(ctx, q, circleID, cmd.ActorID); err != nil {
		return PlannedSessionView{}, err
	}
	record, _, err := ReplayOrExecute(ctx, q, cmd.ActorID, cmd.IdempotencyKey, fingerprint,
		func(context.Context) (int, *string, error) { return s.applyOneOffChange(ctx, q, circleID, cmd) })
	if err != nil {
		return PlannedSessionView{}, err
	}
	if record.ResponseResourceID == nil {
		return PlannedSessionView{}, fmt.Errorf("missing replay resource: %w", ErrInvalidScheduleCommand)
	}
	return loadOneOffPlannedSession(ctx, q, *record.ResponseResourceID)
}

func oneOffCircle(ctx context.Context, q dbQuerier, sessionID string) (string, error) {
	var circleID string
	err := q.QueryRow(ctx, getOneOffCircleQuery, sessionID).Scan(&circleID)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", ErrPlannedSessionNotFound
	}
	if err != nil {
		return "", fmt.Errorf("find planned session circle: %w", err)
	}
	return circleID, nil
}

type oneOffLock struct {
	circleID, status string
	actualStart      *time.Time
	version          int
}

func lockOneOff(ctx context.Context, q dbQuerier, sessionID string) (oneOffLock, error) {
	var locked oneOffLock
	err := q.QueryRow(ctx, lockOneOffPlannedSessionQuery, sessionID).Scan(
		&locked.circleID, &locked.status, &locked.actualStart, &locked.version)
	if errors.Is(err, pgx.ErrNoRows) {
		return oneOffLock{}, ErrPlannedSessionNotFound
	}
	if err != nil {
		return oneOffLock{}, fmt.Errorf("lock planned session: %w", err)
	}
	return locked, nil
}

func (s *PlannedSessionService) applyOneOffChange(ctx context.Context, q dbQuerier, circleID string, cmd ChangePlannedSessionCommand) (int, *string, error) {
	locked, err := lockOneOff(ctx, q, cmd.SessionID)
	if err != nil {
		return 0, nil, err
	}
	if locked.circleID != circleID {
		return 0, nil, ErrPlannedSessionNotFound
	}
	if locked.actualStart != nil || locked.status != "scheduled" {
		return 0, nil, ErrOccurrenceStarted
	}
	if locked.version != cmd.ExpectedVersion {
		return 0, nil, ErrScheduleConflict
	}
	var proposed *PlannedSessionPlan
	if cmd.Plan != nil {
		plan, _, _, err := validatePlannedSessionPlan(*cmd.Plan, s.now())
		if err != nil {
			return 0, nil, err
		}
		proposed = &plan
	} else if cmd.Cancelled != nil && !*cmd.Cancelled {
		view, err := loadOneOffPlannedSession(ctx, q, cmd.SessionID)
		if err != nil {
			return 0, nil, err
		}
		zone, err := LoadLocation(view.PlanningTimezone)
		if err != nil {
			return 0, nil, err
		}
		localDate := view.StartsAt.In(zone)
		proposed = &PlannedSessionPlan{LocalDate: time.Date(localDate.Year(), localDate.Month(), localDate.Day(), 0, 0, 0, 0, zone),
			StartLocalTime: view.StartLocalTime, EndLocalTime: view.EndLocalTime, DurationMinutes: view.DurationMinutes,
			Timezone: view.PlanningTimezone, Title: view.Title}
	}
	if proposed != nil {
		if err := s.overlap.Check(ctx, q, cmd.ActorID, circleID, plannedRevision(*proposed), cmd.SessionID, cmd.ConfirmOverlaps, cmd.ConfirmedWarningIDs); err != nil {
			return 0, nil, err
		}
	}
	if err := s.persistOneOffChange(ctx, q, cmd); err != nil {
		return 0, nil, err
	}
	id := cmd.SessionID
	return http.StatusOK, &id, nil
}

func (s *PlannedSessionService) persistOneOffChange(ctx context.Context, q dbQuerier, cmd ChangePlannedSessionCommand) error {
	if cmd.Plan != nil {
		return s.updateOneOffPlan(ctx, q, cmd.SessionID, *cmd.Plan)
	}
	var cancelledAt any
	if *cmd.Cancelled {
		cancelledAt = s.now()
	}
	return setOneOffCancellation(ctx, q, cmd.SessionID, cancelledAt)
}

func (s *PlannedSessionService) updateOneOffPlan(ctx context.Context, q dbQuerier, sessionID string, input PlannedSessionPlan) error {
	plan, start, end, err := validatePlannedSessionPlan(input, s.now())
	if err != nil {
		return err
	}
	if err = writeOneOffDetails(ctx, q, sessionID, plan, end); err != nil {
		return err
	}
	return writeOneOffStart(ctx, q, sessionID, start)
}

func writeOneOffDetails(ctx context.Context, q dbQuerier, sessionID string, plan PlannedSessionPlan, end time.Time) error {
	tag, err := q.Exec(ctx, updateOneOffPlanQuery, sessionID, plan.Title,
		formatLocalClock(plan.StartLocalTime), formatLocalClock(plan.EndLocalTime), plan.DurationMinutes, end, plan.Timezone)
	if err != nil {
		return fmt.Errorf("update planned session details: %w", err)
	}
	if tag.RowsAffected() != 1 {
		return ErrOccurrenceStarted
	}
	return nil
}

func writeOneOffStart(ctx context.Context, q dbQuerier, sessionID string, start time.Time) error {
	tag, err := q.Exec(ctx, updateOneOffStartQuery, sessionID, start)
	if err != nil {
		return fmt.Errorf("update planned session start: %w", err)
	}
	if tag.RowsAffected() != 1 {
		return ErrOccurrenceStarted
	}
	return nil
}

func setOneOffCancellation(ctx context.Context, q dbQuerier, sessionID string, cancelledAt any) error {
	tag, err := q.Exec(ctx, updateOneOffCancellationQuery, sessionID, cancelledAt)
	if err != nil {
		return fmt.Errorf("update planned session cancellation: %w", err)
	}
	if tag.RowsAffected() != 1 {
		return ErrPlannedSessionNotFound
	}
	return nil
}

func validatePlannedIdempotencyKey(key string) error {
	if strings.TrimSpace(key) != key || key == "" || utf8.RuneCountInString(key) > 128 {
		return ErrInvalidScheduleCommand
	}
	return nil
}

func plannedCommandFingerprint(operation string, command any) (string, error) {
	encoded, err := json.Marshal(command)
	if err != nil {
		return "", fmt.Errorf("encode planned session command: %w", err)
	}
	return fmt.Sprintf("%s:%x", operation, sha256.Sum256(encoded)), nil
}

func validatePlannedSessionPlan(plan PlannedSessionPlan, now time.Time) (PlannedSessionPlan, time.Time, time.Time, error) {
	if plan.LocalDate.IsZero() {
		return plan, time.Time{}, time.Time{}, ErrInvalidScheduleCommand
	}
	if err := ValidatePlannedClocks(plan.StartLocalTime, plan.EndLocalTime, plan.DurationMinutes); err != nil {
		return plan, time.Time{}, time.Time{}, err
	}
	if plan.Title == "" {
		plan.Title = defaultScheduleTitle
	}
	if err := validateTitle(plan.Title); err != nil {
		return plan, time.Time{}, time.Time{}, err
	}
	start, end, err := resolvePlannedInterval(plan, now)
	return plan, start, end, err
}

func resolvePlannedInterval(plan PlannedSessionPlan, now time.Time) (time.Time, time.Time, error) {
	loc, err := LoadLocation(plan.Timezone)
	if err != nil {
		return time.Time{}, time.Time{}, err
	}
	start, end, err := ResolveIntervalUTC(loc, civilOf(plan.LocalDate), plan.StartLocalTime, plan.DurationMinutes)
	if err != nil {
		return time.Time{}, time.Time{}, err
	}
	if start.Before(now) {
		return time.Time{}, time.Time{}, ErrPastPlannedTime
	}
	return start, end, nil
}

func loadOneOffPlannedSession(ctx context.Context, q dbQuerier, sessionID string) (PlannedSessionView, error) {
	return scanOneOffPlannedSession(q.QueryRow(ctx, getOneOffPlannedSessionQuery, sessionID))
}

func scanOneOffPlannedSession(row scannable) (PlannedSessionView, error) {
	var view PlannedSessionView
	var status, startClock, endClock string
	var startsAt, endsAt *time.Time
	var cancelledAt *time.Time
	err := row.Scan(
		&view.ID, &view.CircleID, &view.CircleName, &view.Title,
		&status, &startsAt, &endsAt, &startClock, &endClock,
		&view.DurationMinutes, &view.PlanningTimezone, &cancelledAt, &view.Version,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return PlannedSessionView{}, ErrPlannedSessionNotFound
	}
	if err != nil {
		return PlannedSessionView{}, fmt.Errorf("read planned session: %w", err)
	}
	if startsAt == nil || endsAt == nil {
		return PlannedSessionView{}, fmt.Errorf("one-off session lacks planned interval: %w", ErrInvalidScheduleCommand)
	}
	view.StartsAt = *startsAt
	view.EndsAt = *endsAt
	view.StartLocalTime, view.EndLocalTime, err = parseOneOffClocks(startClock, endClock)
	if err != nil {
		return PlannedSessionView{}, err
	}
	view.Cancelled = cancelledAt != nil
	view.Status = status
	return view, nil
}

func parseOneOffClocks(start, end string) (LocalClock, LocalClock, error) {
	startClock, err := parseLocalClock(start)
	if err != nil {
		return LocalClock{}, LocalClock{}, fmt.Errorf("read planned start time: %w", err)
	}
	endClock, err := parseLocalClock(end)
	if err != nil {
		return LocalClock{}, LocalClock{}, fmt.Errorf("read planned end time: %w", err)
	}
	return startClock, endClock, nil
}
