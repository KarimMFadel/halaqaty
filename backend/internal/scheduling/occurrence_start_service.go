package scheduling

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/KarimMFadel/halaqaty/backend/internal/sessions"
	"github.com/jackc/pgx/v5"
)

// StartOccurrenceCommand identifies one stable recurring occurrence to start.
type StartOccurrenceCommand struct {
	ActorID, CircleID, ScheduleID string
	OriginalLocalDate             time.Time
	IdempotencyKey                string
}

// OccurrenceStartService materializes recurring occurrences and starts them through F-005.
type OccurrenceStartService struct {
	repo     *Repository
	sessions *sessions.Service
}

type occurrenceStartPlan struct {
	record               RevisionRecord
	effectiveDate        time.Time
	title                string
	startClock, endClock LocalClock
	duration             int
	startAt, endAt       time.Time
}

// NewOccurrenceStartService constructs the transactional occurrence start service.
func NewOccurrenceStartService(repo *Repository, sessionService *sessions.Service) *OccurrenceStartService {
	return &OccurrenceStartService{repo: repo, sessions: sessionService}
}

// StartOccurrence materializes and starts a recurring occurrence under its parent schedule lock.
func (s *OccurrenceStartService) StartOccurrence(ctx context.Context, cmd StartOccurrenceCommand) (sessions.Session, sessions.MediaConnection, error) {
	if err := validatePlannedIdempotencyKey(cmd.IdempotencyKey); err != nil || cmd.ActorID == "" || cmd.CircleID == "" || cmd.ScheduleID == "" || cmd.OriginalLocalDate.IsZero() {
		return sessions.Session{}, sessions.MediaConnection{}, ErrInvalidScheduleCommand
	}
	fingerprint, err := plannedCommandFingerprint("start-occurrence", cmd)
	if err != nil {
		return sessions.Session{}, sessions.MediaConnection{}, err
	}
	var started sessions.Session
	var connection sessions.MediaConnection
	err = s.repo.withTx(ctx, func(q dbQuerier) error {
		started, connection, err = s.startOccurrenceInTx(ctx, q, cmd, fingerprint)
		return err
	})
	if err != nil {
		return sessions.Session{}, sessions.MediaConnection{}, err
	}
	return started, connection, nil
}

func (s *OccurrenceStartService) startOccurrenceInTx(ctx context.Context, q dbQuerier, cmd StartOccurrenceCommand, fingerprint string) (sessions.Session, sessions.MediaConnection, error) {
	if _, err := RequireCircleManage(ctx, q, cmd.CircleID, cmd.ActorID); err != nil {
		return sessions.Session{}, sessions.MediaConnection{}, err
	}
	if prior, err := LoadReplay(ctx, q, cmd.ActorID, cmd.IdempotencyKey); err != nil {
		return sessions.Session{}, sessions.MediaConnection{}, err
	} else if prior != nil && prior.CommandFingerprint != fingerprint {
		return sessions.Session{}, sessions.MediaConnection{}, ErrReplayConflict
	}
	schedule, err := lockSchedule(ctx, q, cmd.CircleID, cmd.ScheduleID)
	if err != nil {
		return sessions.Session{}, sessions.MediaConnection{}, err
	}
	sessionID, err := s.materializeOccurrence(ctx, q, cmd, schedule)
	if err != nil {
		return sessions.Session{}, sessions.MediaConnection{}, err
	}
	replay, _, err := ReplayOrExecute(ctx, q, cmd.ActorID, cmd.IdempotencyKey, fingerprint,
		func(context.Context) (int, *string, error) { return http.StatusOK, &sessionID, nil })
	if err != nil {
		return sessions.Session{}, sessions.MediaConnection{}, err
	}
	if replay.ResponseResourceID == nil {
		return sessions.Session{}, sessions.MediaConnection{}, fmt.Errorf("missing occurrence replay resource: %w", ErrInvalidScheduleCommand)
	}
	return s.sessions.StartSessionWithConnectionInTx(ctx, q.(pgx.Tx), cmd.ActorID, *replay.ResponseResourceID)
}

func (s *OccurrenceStartService) materializeOccurrence(ctx context.Context, q dbQuerier, cmd StartOccurrenceCommand, schedule Schedule) (string, error) {
	plan, err := s.resolveOccurrenceStartPlan(ctx, q, cmd, schedule)
	if err != nil {
		return "", err
	}
	date := civilOf(cmd.OriginalLocalDate)
	var sessionID string
	var cancelledAt *time.Time
	lookupErr := q.QueryRow(ctx, getOccurrenceDetailForStartQuery, cmd.ScheduleID, date).Scan(&sessionID, &cancelledAt)
	if lookupErr == nil {
		if cancelledAt != nil {
			return "", ErrOccurrenceNotFound
		}
		return sessionID, nil
	}
	if !errors.Is(lookupErr, pgx.ErrNoRows) {
		return "", fmt.Errorf("load materialized occurrence: %w", lookupErr)
	}
	if plan.title == "" {
		plan.title = defaultScheduleTitle
	}
	if err := q.QueryRow(ctx, insertOccurrenceSessionQuery, cmd.CircleID, cmd.ActorID, plan.startAt).Scan(&sessionID); err != nil {
		return "", fmt.Errorf("materialize occurrence session: %w", err)
	}
	if _, err := q.Exec(ctx, insertOccurrenceDetailsQuery, sessionID, cmd.ScheduleID, date,
		plan.title, formatLocalClock(plan.startClock), formatLocalClock(plan.endClock), plan.duration,
		plan.endAt, plan.record.Timezone); err != nil {
		return "", fmt.Errorf("materialize occurrence details: %w", err)
	}
	return sessionID, nil
}

func (s *OccurrenceStartService) resolveOccurrenceStartPlan(ctx context.Context, q dbQuerier, cmd StartOccurrenceCommand, schedule Schedule) (occurrenceStartPlan, error) {
	date := civilOf(cmd.OriginalLocalDate)
	if dateIsStopped(date, schedule.StoppedFromLocalDate) {
		return occurrenceStartPlan{}, ErrOccurrenceNotFound
	}
	repo := &Repository{tx: q}
	revisions, err := repo.LoadRevisions(ctx, []string{cmd.ScheduleID})
	if err != nil {
		return occurrenceStartPlan{}, err
	}
	record, found := governingRecord(revisions[cmd.ScheduleID], date)
	if !found || len(record.OccurrenceDates(date, date)) == 0 {
		return occurrenceStartPlan{}, ErrOccurrenceNotFound
	}
	plan := occurrenceStartPlan{record: record, effectiveDate: date, title: record.Title,
		startClock: record.StartLocalTime, endClock: record.EndLocalTime, duration: record.DurationMinutes}
	if err := applyOccurrenceStartException(ctx, q, cmd.ScheduleID, date, schedule.CurrentVersion, &plan); err != nil {
		return occurrenceStartPlan{}, err
	}
	if dateIsStopped(plan.effectiveDate, schedule.StoppedFromLocalDate) {
		return occurrenceStartPlan{}, ErrOccurrenceNotFound
	}
	loc, err := LoadLocation(record.Timezone)
	if err != nil {
		return occurrenceStartPlan{}, err
	}
	plan.startAt, plan.endAt, err = ResolveIntervalUTC(loc, plan.effectiveDate, plan.startClock, plan.duration)
	return plan, err
}

func applyOccurrenceStartException(ctx context.Context, q dbQuerier, scheduleID string, date time.Time, currentVersion int, plan *occurrenceStartPlan) error {
	var seriesVersion int
	var replacementDate, cancelledAt *time.Time
	var replacementStart, replacementEnd, replacementTitle *string
	var replacementDuration *int
	err := q.QueryRow(ctx, getOccurrenceExceptionForStartQuery, scheduleID, date).Scan(
		&seriesVersion, &replacementDate, &replacementStart, &replacementEnd,
		&replacementDuration, &replacementTitle, &cancelledAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("load occurrence exception: %w", err)
	}
	if seriesVersion != currentVersion {
		return nil
	}
	if cancelledAt != nil {
		return ErrOccurrenceNotFound
	}
	if replacementDate != nil {
		plan.effectiveDate = civilOf(*replacementDate)
	}
	if replacementStart != nil {
		var err error
		plan.startClock, err = parseLocalClock(*replacementStart)
		if err != nil {
			return err
		}
		plan.endClock, err = parseLocalClock(*replacementEnd)
		if err != nil {
			return err
		}
		plan.duration = *replacementDuration
	}
	if replacementTitle != nil {
		plan.title = *replacementTitle
	}
	return nil
}

func dateIsStopped(date time.Time, boundary *time.Time) bool {
	return boundary != nil && !date.Before(civilOf(*boundary))
}
