package attendance

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var ErrStudentMembershipRequired = errors.New("active student membership required")

// Repository persists the eligible roster and its derived attendance.
type Repository struct {
	pool *pgxpool.Pool
}

// NewRepository constructs an attendance repository on a pgx pool.
func NewRepository(pool *pgxpool.Pool) *Repository { return &Repository{pool: pool} }

// SnapshotStartRoster records current active students in the session's start transaction.
func (r *Repository) SnapshotStartRoster(ctx context.Context, tx pgx.Tx, sessionID string) error {
	if _, err := tx.Exec(ctx, snapshotStartRosterQuery, sessionID); err != nil {
		return fmt.Errorf("snapshot attendance roster: %w", err)
	}
	return nil
}

// CheckStudentJoin locks and rechecks the current circle role in the join transaction.
func (r *Repository) CheckStudentJoin(ctx context.Context, tx pgx.Tx, sessionID, userID string) (bool, error) {
	var role string
	err := tx.QueryRow(ctx, lockStudentMembershipQuery, sessionID, userID).Scan(&role)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, ErrStudentMembershipRequired
	}
	if err != nil {
		return false, fmt.Errorf("check attendance join membership: %w", err)
	}
	return role == "student", nil
}

// RecordStudentJoin inserts a later participant into the durable attendance roster.
func (r *Repository) RecordStudentJoin(ctx context.Context, tx pgx.Tx, sessionID, userID string) error {
	if _, err := tx.Exec(ctx, addLaterParticipantQuery, sessionID, userID); err != nil {
		return fmt.Errorf("record later attendance participant: %w", err)
	}
	return nil
}

// FinalizeSession classifies every eligible roster row from its first presence.
func (r *Repository) FinalizeSession(ctx context.Context, tx pgx.Tx, sessionID string) error {
	var actualStart, actualEnd *time.Time
	var sessionStatus string
	if err := tx.QueryRow(ctx, attendanceSessionFactsQuery, sessionID).Scan(&actualStart, &actualEnd, &sessionStatus); err != nil {
		return fmt.Errorf("load attendance session facts: %w", err)
	}
	if sessionStatus != "ended" {
		return nil
	}
	if actualStart == nil || actualEnd == nil {
		if _, err := tx.Exec(ctx, refreshEndedSessionRecoveryOrderQuery, sessionID); err != nil {
			return fmt.Errorf("refresh ended attendance recovery order: %w", err)
		}
		return nil
	}
	rows, err := tx.Query(ctx, attendanceRosterQuery, sessionID)
	if err != nil {
		return fmt.Errorf("load attendance roster: %w", err)
	}
	entries := make([]RosterEntry, 0)
	for rows.Next() {
		var entry RosterEntry
		if err := rows.Scan(&entry.UserID, &entry.FirstPresenceAt); err != nil {
			rows.Close()
			return fmt.Errorf("scan attendance roster: %w", err)
		}
		entries = append(entries, entry)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return fmt.Errorf("iterate attendance roster: %w", err)
	}
	rows.Close()
	for _, entry := range entries {
		status := ClassifyEndedSession(FinalizationInput{ActualStart: actualStart, Ended: true, Roster: []RosterEntry{entry}})[entry.UserID]
		if _, err := tx.Exec(ctx, finalizeAttendanceRowQuery, sessionID, entry.UserID, entry.FirstPresenceAt, status, *actualEnd); err != nil {
			return fmt.Errorf("finalize attendance row: %w", err)
		}
	}
	if _, err := tx.Exec(ctx, refreshEndedSessionRecoveryOrderQuery, sessionID); err != nil {
		return fmt.Errorf("refresh ended attendance recovery order: %w", err)
	}
	return nil
}

// Finalize retries attendance finalization in its own transaction.
func (r *Repository) Finalize(ctx context.Context, sessionID string) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin attendance finalization: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := r.FinalizeSession(ctx, tx, sessionID); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit attendance finalization: %w", err)
	}
	return nil
}

// List returns completed attendance visible to a current circle member.
func (r *Repository) List(ctx context.Context, sessionID, actorID string) ([]Record, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin attendance read: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var sessionStatus, role string
	var archived bool
	err = tx.QueryRow(ctx, attendanceReadScopeQuery, sessionID, actorID).Scan(&sessionStatus, &archived, &role)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrAttendanceNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("authorize attendance read: %w", err)
	}
	if sessionStatus != "ended" || role == "" {
		return nil, ErrAttendanceNotFound
	}
	rows, err := tx.Query(ctx, listAttendanceQuery, sessionID, role, actorID)
	if err != nil {
		return nil, fmt.Errorf("list attendance: %w", err)
	}
	defer rows.Close()
	records := make([]Record, 0)
	for rows.Next() {
		record, err := scanAttendanceRecord(rows)
		if err != nil {
			return nil, fmt.Errorf("scan attendance: %w", err)
		}
		records = append(records, record)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate attendance: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit attendance read: %w", err)
	}
	_ = archived
	return records, nil
}

// CorrectAttendance serializes a teacher correction and its retry record in one transaction.
func (r *Repository) CorrectAttendance(ctx context.Context, command CorrectionCommand, fingerprint string) (Record, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return Record{}, fmt.Errorf("begin attendance correction: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var circleID, sessionStatus string
	var archived bool
	err = tx.QueryRow(ctx, lockAttendanceCorrectionScopeQuery, command.SessionID).Scan(&circleID, &sessionStatus, &archived)
	if errors.Is(err, pgx.ErrNoRows) {
		return Record{}, ErrAttendanceNotFound
	}
	if err != nil {
		return Record{}, fmt.Errorf("lock attendance correction scope: %w", err)
	}
	if archived {
		return Record{}, ErrAttendanceArchived
	}
	if sessionStatus != "ended" {
		return Record{}, ErrAttendanceNotComplete
	}
	var role string
	err = tx.QueryRow(ctx, lockAttendanceCorrectionMemberQuery, circleID, command.ActorID).Scan(&role)
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && role != "teacher") {
		return Record{}, ErrAttendanceForbidden
	}
	if err != nil {
		return Record{}, fmt.Errorf("authorize attendance correction: %w", err)
	}
	if _, err := tx.Exec(ctx, lockAttendanceCorrectionReplayQuery, command.ActorID, command.IdempotencyKey); err != nil {
		return Record{}, fmt.Errorf("lock attendance correction retry: %w", err)
	}
	var priorFingerprint, priorID string
	err = tx.QueryRow(ctx, loadAttendanceCorrectionReplayQuery, command.ActorID, command.IdempotencyKey).Scan(&priorFingerprint, &priorID)
	if err == nil {
		if priorFingerprint != fingerprint {
			return Record{}, ErrCorrectionReplay
		}
		record, err := scanAttendanceRecord(tx.QueryRow(ctx, getAttendanceCorrectionByIDQuery, priorID))
		if err != nil {
			return Record{}, fmt.Errorf("load replayed attendance correction: %w", err)
		}
		if err := tx.Commit(ctx); err != nil {
			return Record{}, fmt.Errorf("commit attendance correction retry: %w", err)
		}
		return record, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return Record{}, fmt.Errorf("load attendance correction retry: %w", err)
	}
	var previous string
	err = tx.QueryRow(ctx, lockEffectiveAttendanceQuery, command.SessionID, command.UserID).Scan(&previous)
	if errors.Is(err, pgx.ErrNoRows) {
		return Record{}, ErrAttendanceNotFound
	}
	if err != nil {
		return Record{}, fmt.Errorf("load current attendance status: %w", err)
	}
	var correctionID string
	if err := tx.QueryRow(ctx, insertAttendanceCorrectionQuery, command.SessionID, command.UserID, command.ActorID, command.Reason, previous, string(command.Status)).Scan(&correctionID, new(time.Time)); err != nil {
		return Record{}, fmt.Errorf("append attendance correction: %w", err)
	}
	if _, err := tx.Exec(ctx, updateEffectiveAttendanceQuery, command.SessionID, command.UserID, string(command.Status)); err != nil {
		return Record{}, fmt.Errorf("update effective attendance: %w", err)
	}
	if _, err := tx.Exec(ctx, insertAttendanceCorrectionReplayQuery, command.ActorID, command.IdempotencyKey, fingerprint, correctionID); err != nil {
		return Record{}, fmt.Errorf("store attendance correction retry: %w", err)
	}
	record, err := scanAttendanceRecord(tx.QueryRow(ctx, getAttendanceCorrectionByIDQuery, correctionID))
	if err != nil {
		return Record{}, fmt.Errorf("load corrected attendance: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return Record{}, fmt.Errorf("commit attendance correction: %w", err)
	}
	return record, nil
}

type attendanceRowScanner interface{ Scan(...any) error }

func scanAttendanceRecord(row attendanceRowScanner) (Record, error) {
	var record Record
	var status, source string
	var firstPresence, correctionAt sql.NullTime
	var correctionID, correctionActor, reason, previous, next sql.NullString
	if err := row.Scan(&record.SessionID, &record.UserID, &status, &source, &firstPresence, &correctionID, &correctionActor, &correctionAt, &reason, &previous, &next); err != nil {
		return Record{}, err
	}
	record.Status = Status(status)
	record.Source = source
	if firstPresence.Valid {
		record.FirstPresence = &firstPresence.Time
	}
	if correctionID.Valid {
		record.CorrectionID = &correctionID.String
		record.Correction = &CorrectionAudit{ActorID: correctionActor.String, At: correctionAt.Time, Reason: reason.String, PreviousStatus: Status(previous.String), NewStatus: Status(next.String)}
	}
	return record, nil
}
