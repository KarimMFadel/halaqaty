package queue

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Repository persists F-003 queue truth in PostgreSQL.
type Repository struct{ pool *pgxpool.Pool }

// Tx is one queue mutation transaction.
type Tx struct{ tx pgx.Tx }

// NewQueueRepository constructs a queue repository from a PostgreSQL pool.
func NewQueueRepository(pool *pgxpool.Pool) *Repository { return &Repository{pool: pool} }

// WithTx runs fn atomically and leaves domain errors unwrapped.
func (r *Repository) WithTx(ctx context.Context, fn func(*Tx) error) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin queue transaction: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := fn(&Tx{tx: tx}); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit queue transaction: %w", err)
	}
	return nil
}

func scanRound(row pgx.Row) (Round, error) {
	var r Round
	err := row.Scan(&r.ID, &r.SessionID, &r.RoundNumber, &r.Type, &r.SurahID, &r.FromAyah, &r.ToAyah,
		&r.GradingRequired, &r.Lifecycle, &r.SelectedEntryID, &r.Version, &r.CreatedBy, &r.CreatedAt,
		&r.ActivatedAt, &r.FinalizedAt)
	return r, err
}

func scanEntry(row pgx.Row) (QueueEntry, error) {
	var e QueueEntry
	var grade *string
	err := row.Scan(&e.ID, &e.QueueID, &e.StudentID, &e.Position, &e.Status, &grade, &e.TeacherNotes,
		&e.Version, &e.StartedAt, &e.CompletedAt, &e.ResolvedBy, &e.CreatedAt, &e.UpdatedAt)
	if grade != nil {
		value := Grade(*grade)
		e.Grade = &value
	}
	return e, err
}

func scanReceipt(row pgx.Row) (CommandReceipt, error) {
	var receipt CommandReceipt
	err := row.Scan(&receipt.SessionID, &receipt.ActorID, &receipt.IdempotencyKey, &receipt.Command,
		&receipt.ResourceID, &receipt.ResultVersion, &receipt.CreatedAt)
	return receipt, err
}

func scanOutbox(row pgx.Row) (OutboxEvent, error) {
	var event OutboxEvent
	err := row.Scan(&event.EventID, &event.SessionID, &event.RoundID, &event.EventType, &event.ResourceID,
		&event.RoundVersion, &event.EventMetadata, &event.AvailableAt, &event.DeliveredAt, &event.AttemptCount, &event.ParkedAt)
	return event, err
}

func staleVersionError() error {
	return &QueueError{Code: QueueErrorCodeStaleVersion, Message: "queue state changed"}
}

// LockRoundAllocation serializes per-session round number allocation.
func (t *Tx) LockRoundAllocation(ctx context.Context, sessionID string) error {
	if _, err := t.tx.Exec(ctx, lockRoundAllocationQuery, sessionID); err != nil {
		return fmt.Errorf("lock round allocation: %w", err)
	}
	return nil
}

// NextRoundNumber returns the next sequential round number under allocation lock.
func (t *Tx) NextRoundNumber(ctx context.Context, sessionID string) (int, error) {
	var n int
	if err := t.tx.QueryRow(ctx, nextRoundNumberQuery, sessionID).Scan(&n); err != nil {
		return 0, fmt.Errorf("next round number: %w", err)
	}
	return n, nil
}

// CreateRound persists one round.
func (t *Tx) CreateRound(ctx context.Context, in NewRound) (Round, error) {
	round, err := scanRound(t.tx.QueryRow(ctx, insertRoundQuery, in.SessionID, in.RoundNumber, in.Type, in.SurahID,
		in.FromAyah, in.ToAyah, in.GradingRequired, in.Lifecycle, in.CreatedBy))
	if err != nil {
		return Round{}, fmt.Errorf("create queue round: %w", err)
	}
	return round, nil
}

// LockRound loads a round while holding its row lock.
func (t *Tx) LockRound(ctx context.Context, roundID string) (Round, error) {
	round, err := scanRound(t.tx.QueryRow(ctx, lockRoundByIDQuery, roundID))
	if err != nil {
		return Round{}, fmt.Errorf("lock queue round: %w", err)
	}
	return round, nil
}

// LockEntry loads an entry while holding its row lock.
func (t *Tx) LockEntry(ctx context.Context, entryID string) (QueueEntry, error) {
	entry, err := scanEntry(t.tx.QueryRow(ctx, lockEntryByIDQuery, entryID))
	if err != nil {
		return QueueEntry{}, fmt.Errorf("lock queue entry: %w", err)
	}
	return entry, nil
}

// SetRoundSelection replaces the selected entry with an optimistic version guard.
func (t *Tx) SetRoundSelection(ctx context.Context, roundID string, entryID *string, expectedVersion int64) (Round, error) {
	round, err := scanRound(t.tx.QueryRow(ctx, updateRoundSelectionQuery, roundID, stringOrNil(entryID), expectedVersion))
	if errors.Is(err, pgx.ErrNoRows) {
		return Round{}, staleVersionError()
	}
	if err != nil {
		return Round{}, fmt.Errorf("set queue selection: %w", err)
	}
	return round, nil
}

// BumpRoundVersion increments the queue-visible round version.
func (t *Tx) BumpRoundVersion(ctx context.Context, roundID string) error {
	if _, err := t.tx.Exec(ctx, bumpRoundVersionQuery, roundID); err != nil {
		return fmt.Errorf("bump queue round version: %w", err)
	}
	return nil
}

// InsertQueueEntry materializes a waiting student entry.
func (t *Tx) InsertQueueEntry(ctx context.Context, roundID, studentID string, position int) (QueueEntry, error) {
	entry, err := scanEntry(t.tx.QueryRow(ctx, insertQueueEntryQuery, roundID, studentID, position))
	if err != nil {
		return QueueEntry{}, fmt.Errorf("insert queue entry: %w", err)
	}
	return entry, nil
}

// SelectNextWaitingEntry locks and returns the earliest waiting entry.
func (t *Tx) SelectNextWaitingEntry(ctx context.Context, roundID string) (*QueueEntry, error) {
	entry, err := scanEntry(t.tx.QueryRow(ctx, selectNextWaitingEntryQuery, roundID))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("select next waiting entry: %w", err)
	}
	return &entry, nil
}

// TransitionEntry applies a legal optimistic entry transition.
func (t *Tx) TransitionEntry(ctx context.Context, entryID string, from EntryStatus, expectedVersion int64, to EntryStatus, grade *Grade, notes *string, resolvedBy *string) (QueueEntry, error) {
	if !CanTransitionEntry(from, to) {
		return QueueEntry{}, &QueueError{Code: QueueErrorCodeInvalidTransition, Message: "invalid entry transition"}
	}
	var startedAt, completedAt *time.Time
	now := time.Now().UTC()
	if to == EntryStatusReciting {
		startedAt = &now
	}
	if to == EntryStatusCompleted {
		completedAt = &now
	}
	entry, err := scanEntry(t.tx.QueryRow(ctx, updateEntryTransitionQuery, entryID, expectedVersion, from, to, gradeOrNil(grade), stringOrNil(notes), startedAt, completedAt, stringOrNil(resolvedBy)))
	if errors.Is(err, pgx.ErrNoRows) {
		return QueueEntry{}, staleVersionError()
	}
	if err != nil {
		return QueueEntry{}, fmt.Errorf("transition queue entry: %w", err)
	}
	return entry, nil
}

// FinalizeRound finalizes a round and applies its unfinished-entry policy.
func (t *Tx) FinalizeRound(ctx context.Context, roundID string, expectedVersion int64, policy FinalizationPolicy, resolvedBy string) (Round, error) {
	if policy == FinalizationPolicyMarkUnfinishedSkipped {
		if _, err := t.tx.Exec(ctx, markUnfinishedEntriesSkippedQuery, roundID, resolvedBy); err != nil {
			return Round{}, fmt.Errorf("skip unfinished entries: %w", err)
		}
	}
	round, err := scanRound(t.tx.QueryRow(ctx, finalizeRoundQuery, roundID, expectedVersion))
	if errors.Is(err, pgx.ErrNoRows) {
		return Round{}, staleVersionError()
	}
	if err != nil {
		return Round{}, fmt.Errorf("finalize queue round: %w", err)
	}
	return round, nil
}

// SetEntryPosition rewrites one entry position.
func (t *Tx) SetEntryPosition(ctx context.Context, entryID string, position int) error {
	if _, err := t.tx.Exec(ctx, updateEntryPositionQuery, entryID, position); err != nil {
		return fmt.Errorf("set queue entry position: %w", err)
	}
	return nil
}

// SetPreorderPosition rewrites one pre-activation candidate position.
func (t *Tx) SetPreorderPosition(ctx context.Context, roundID, studentID string, position int) error {
	if _, err := t.tx.Exec(ctx, updatePreorderPositionQuery, roundID, studentID, position); err != nil {
		return fmt.Errorf("set queue preorder position: %w", err)
	}
	return nil
}

// PopulationOrder returns the deterministic activation order for one policy.
func (t *Tx) PopulationOrder(ctx context.Context, sessionID, roundID string, policy PopulationPolicy) ([]string, error) {
	query := listPresentAtActivationPopulationQuery
	if policy == PopulationPolicyAllActiveStudents {
		query = listAllActiveStudentsPopulationQuery
	}
	rows, err := t.tx.Query(ctx, query, sessionID, roundID)
	if err != nil {
		return nil, fmt.Errorf("list activation population: %w", err)
	}
	defer rows.Close()
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("scan activation population: %w", err)
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate activation population: %w", err)
	}
	return ids, nil
}

// ReserveCommandReceipt reserves an idempotency key or returns its replay.
func (t *Tx) ReserveCommandReceipt(ctx context.Context, sessionID, actorID, key, command string) (*CommandReceipt, bool, error) {
	tag, err := t.tx.Exec(ctx, insertCommandReceiptQuery, sessionID, actorID, key, command)
	if err != nil {
		return nil, false, fmt.Errorf("reserve queue receipt: %w", err)
	}
	if tag.RowsAffected() == 1 {
		return nil, true, nil
	}
	receipt, err := scanReceipt(t.tx.QueryRow(ctx, findCommandReceiptQuery, sessionID, actorID, key))
	if err != nil {
		return nil, false, fmt.Errorf("load queue receipt: %w", err)
	}
	if receipt.Command != command {
		return nil, false, &QueueError{Code: QueueErrorCodeDuplicateCommand, Message: "idempotency key was used by another command"}
	}
	return &receipt, false, nil
}

// UpdateCommandReceiptResult stores the committed replay result.
func (t *Tx) UpdateCommandReceiptResult(ctx context.Context, sessionID, actorID, key string, resourceID *string, version *int64) error {
	if _, err := t.tx.Exec(ctx, updateCommandReceiptResultQuery, sessionID, actorID, key, stringOrNil(resourceID), int64OrNil(version)); err != nil {
		return fmt.Errorf("update queue receipt: %w", err)
	}
	return nil
}

var sensitiveOutboxKeys = map[string]struct{}{"grade": {}, "notes": {}, "note": {}, "name": {}, "media": {}, "credential": {}, "room": {}, "url": {}}

func validateOutboxMetadata(raw json.RawMessage) error {
	var value any
	if len(raw) == 0 || json.Unmarshal(raw, &value) != nil {
		return validationError("invalid outbox metadata")
	}
	var walk func(any) bool
	walk = func(v any) bool {
		switch typed := v.(type) {
		case map[string]any:
			for key, child := range typed {
				if _, bad := sensitiveOutboxKeys[strings.ToLower(key)]; bad || !walk(child) {
					return false
				}
			}
		case []any:
			for _, child := range typed {
				if !walk(child) {
					return false
				}
			}
		}
		return true
	}
	if !walk(value) {
		return validationError("sensitive outbox metadata")
	}
	return nil
}

// InsertOutboxEvent persists a redacted event in the enclosing queue transaction.
func (t *Tx) InsertOutboxEvent(ctx context.Context, event OutboxEvent) error {
	if err := validateOutboxMetadata(event.EventMetadata); err != nil {
		return err
	}
	availableAt := event.AvailableAt
	if availableAt.IsZero() {
		availableAt = time.Now().UTC()
	}
	if _, err := t.tx.Exec(ctx, insertOutboxEventQuery, event.EventID, event.SessionID, event.RoundID, event.EventType, stringOrNil(event.ResourceID), event.RoundVersion, event.EventMetadata, availableAt, event.AttemptCount); err != nil {
		return fmt.Errorf("insert queue outbox event: %w", err)
	}
	return nil
}

// UpsertProgress persists one completion record keyed by queue entry.
func (t *Tx) UpsertProgress(ctx context.Context, record NewProgress) error {
	circleID := record.CircleID
	if circleID == "" {
		if err := t.tx.QueryRow(ctx, findSessionCircleIDQuery, record.SessionID).Scan(&circleID); err != nil {
			return fmt.Errorf("derive progress circle: %w", err)
		}
	}
	if _, err := t.tx.Exec(ctx, upsertProgressQuery, record.StudentID, circleID, record.SessionID, record.QueueEntryID, record.SurahID, record.SurahName, record.FromAyah, record.ToAyah, record.Type, gradeOrNil(record.Grade), stringOrNil(record.Notes), record.Date); err != nil {
		return fmt.Errorf("upsert queue progress: %w", err)
	}
	return nil
}

// LoadQueueState returns one visibility-filtered snapshot.
func (r *Repository) LoadQueueState(ctx context.Context, roundID string, viewer Viewer) (QueueState, error) {
	round, err := scanRound(r.pool.QueryRow(ctx, findQueueRoundQuery, roundID))
	if err != nil {
		return QueueState{}, fmt.Errorf("load queue round: %w", err)
	}
	policyCtx, err := r.SessionPolicy(ctx, round.SessionID)
	if err != nil {
		return QueueState{}, err
	}
	rows, err := r.pool.Query(ctx, listQueueEntriesQuery, roundID)
	if err != nil {
		return QueueState{}, fmt.Errorf("list queue entries: %w", err)
	}
	defer rows.Close()
	state := QueueState{Round: round, Entries: []QueueEntry{}, Preorder: []PreorderCandidate{}}
	for rows.Next() {
		entry, err := scanEntry(rows)
		if err != nil {
			return QueueState{}, fmt.Errorf("scan queue entry: %w", err)
		}
		if !canViewEntryDetails(policyCtx.Policy.GradeVisibility, viewer, entry.StudentID) {
			entry.Grade = nil
			entry.TeacherNotes = nil
		}
		state.Entries = append(state.Entries, entry)
	}
	if err := rows.Err(); err != nil {
		return QueueState{}, fmt.Errorf("iterate queue entries: %w", err)
	}
	if !viewer.IsManager {
		return state, nil
	}
	rows, err = r.pool.Query(ctx, listPreorderCandidatesQuery, roundID)
	if err != nil {
		return QueueState{}, fmt.Errorf("list queue preorder: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var candidate PreorderCandidate
		if err := rows.Scan(&candidate.QueueID, &candidate.StudentID, &candidate.Position, &candidate.AddedBy, &candidate.CreatedAt); err != nil {
			return QueueState{}, fmt.Errorf("scan queue preorder: %w", err)
		}
		state.Preorder = append(state.Preorder, candidate)
	}
	if err := rows.Err(); err != nil {
		return QueueState{}, fmt.Errorf("iterate queue preorder: %w", err)
	}
	return state, nil
}

func canViewEntryDetails(policy GradeVisibility, viewer Viewer, studentID string) bool {
	return viewer.IsManager || policy == GradeVisibilityAllParticipants || (policy == GradeVisibilityManagersAndStudent && viewer.UserID == studentID)
}

// SessionRole returns the current active circle role for a session, or an
// empty string when the user has no active membership in that session's circle.
func (r *Repository) SessionRole(ctx context.Context, sessionID, userID string) (string, error) {
	var role string
	if err := r.pool.QueryRow(ctx, findSessionCircleRoleQuery, sessionID, userID).Scan(&role); err != nil {
		return "", fmt.Errorf("load session circle role: %w", err)
	}
	return role, nil
}

// SessionPolicy reads the five queue-policy dimensions for a session.
func (r *Repository) SessionPolicy(ctx context.Context, sessionID string) (SessionPolicyContext, error) {
	var result SessionPolicyContext
	err := r.pool.QueryRow(ctx, findSessionQueuePolicyQuery, sessionID).Scan(&result.Policy.Population, &result.Policy.Finalization, &result.Policy.OptOut, &result.Policy.GradeVisibility, &result.Policy.GradeCorrection, &result.Policy.Version, &result.Status, &result.CircleID)
	if err != nil {
		return SessionPolicyContext{}, fmt.Errorf("load session queue policy: %w", err)
	}
	return result, nil
}

// UpdateSessionPolicy applies an effective policy change with an optimistic guard.
func (r *Repository) UpdateSessionPolicy(ctx context.Context, sessionID string, expectedVersion int64, next QueuePolicy) (QueuePolicy, error) {
	current, err := r.SessionPolicy(ctx, sessionID)
	if err != nil {
		return QueuePolicy{}, err
	}
	if current.Policy.Version != expectedVersion {
		return QueuePolicy{}, staleVersionError()
	}
	if samePolicy(current.Policy, next) {
		return current.Policy, nil
	}
	var updated QueuePolicy
	err = r.pool.QueryRow(ctx, updateSessionPolicyQuery, sessionID, next.Population, next.Finalization, next.OptOut, next.GradeVisibility, next.GradeCorrection, expectedVersion).Scan(&updated.Population, &updated.Finalization, &updated.OptOut, &updated.GradeVisibility, &updated.GradeCorrection, &updated.Version)
	if errors.Is(err, pgx.ErrNoRows) {
		return QueuePolicy{}, staleVersionError()
	}
	if err != nil {
		return QueuePolicy{}, fmt.Errorf("update session queue policy: %w", err)
	}
	return updated, nil
}

func samePolicy(a, b QueuePolicy) bool {
	return a.Population == b.Population && a.Finalization == b.Finalization && a.OptOut == b.OptOut && a.GradeVisibility == b.GradeVisibility && a.GradeCorrection == b.GradeCorrection
}

// ClaimDueOutboxEvents returns due undelivered, unparked events.
func (r *Repository) ClaimDueOutboxEvents(ctx context.Context, limit int) ([]OutboxEvent, error) {
	if limit < 1 {
		return []OutboxEvent{}, nil
	}
	var events []OutboxEvent
	err := r.WithTx(ctx, func(t *Tx) error {
		rows, err := t.tx.Query(ctx, claimDueOutboxEventsQuery, limit)
		if err != nil {
			return fmt.Errorf("claim queue outbox events: %w", err)
		}
		defer rows.Close()
		for rows.Next() {
			event, err := scanOutbox(rows)
			if err != nil {
				return fmt.Errorf("scan claimed outbox event: %w", err)
			}
			events = append(events, event)
		}
		if err := rows.Err(); err != nil {
			return fmt.Errorf("iterate claimed outbox events: %w", err)
		}
		return nil
	})
	return events, err
}

// ClaimReplayOutboxEvents returns pending and parked rows for startup replay.
func (r *Repository) ClaimReplayOutboxEvents(ctx context.Context, limit int) ([]OutboxEvent, error) {
	if limit < 1 {
		return []OutboxEvent{}, nil
	}
	var events []OutboxEvent
	err := r.WithTx(ctx, func(t *Tx) error {
		rows, err := t.tx.Query(ctx, claimReplayOutboxEventsQuery, limit)
		if err != nil {
			return fmt.Errorf("claim replay outbox events: %w", err)
		}
		defer rows.Close()
		for rows.Next() {
			event, err := scanOutbox(rows)
			if err != nil {
				return fmt.Errorf("scan replay outbox event: %w", err)
			}
			events = append(events, event)
		}
		if err := rows.Err(); err != nil {
			return fmt.Errorf("iterate replay outbox events: %w", err)
		}
		return nil
	})
	return events, err
}

// MarkOutboxDelivered records successful delivery.
func (r *Repository) MarkOutboxDelivered(ctx context.Context, eventID string) error {
	if _, err := r.pool.Exec(ctx, markOutboxEventDeliveredQuery, eventID); err != nil {
		return fmt.Errorf("mark outbox delivered: %w", err)
	}
	return nil
}

// RetryOutboxEvent schedules another delivery attempt.
func (r *Repository) RetryOutboxEvent(ctx context.Context, eventID string, availableAt time.Time) error {
	if _, err := r.pool.Exec(ctx, retryOutboxEventQuery, eventID, availableAt); err != nil {
		return fmt.Errorf("retry outbox event: %w", err)
	}
	return nil
}

// ParkOutboxEvent records retry exhaustion for operator replay.
func (r *Repository) ParkOutboxEvent(ctx context.Context, eventID string) error {
	if _, err := r.pool.Exec(ctx, parkOutboxEventQuery, eventID); err != nil {
		return fmt.Errorf("park outbox event: %w", err)
	}
	return nil
}

// OutboxCounts returns undelivered pending and parked event counts for a session.
func (r *Repository) OutboxCounts(ctx context.Context, sessionID string) (int64, int64, error) {
	var pending, parked int64
	if err := r.pool.QueryRow(ctx, countOutboxEventsQuery, sessionID).Scan(&pending, &parked); err != nil {
		return 0, 0, fmt.Errorf("count queue outbox events: %w", err)
	}
	return pending, parked, nil
}

func stringOrNil(value *string) any {
	if value == nil {
		return nil
	}
	return *value
}
func gradeOrNil(value *Grade) any {
	if value == nil {
		return nil
	}
	return string(*value)
}
func int64OrNil(value *int64) any {
	if value == nil {
		return nil
	}
	return *value
}
