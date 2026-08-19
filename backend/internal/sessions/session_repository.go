package sessions

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// querier is satisfied by both *pgxpool.Pool and pgx.Tx.
type querier interface {
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

// Repository is the PostgreSQL persistence boundary for live-session state.
// PostgreSQL is the sole source of truth; no session state is cached in
// memory (constitution §III).
type Repository struct {
	pool *pgxpool.Pool
}

// NewSessionRepository constructs a live-session repository on a pgx pool.
func NewSessionRepository(pool *pgxpool.Pool) *Repository {
	return &Repository{pool: pool}
}

// withTx runs fn against a transaction and commits on success. Errors
// returned by fn pass through unwrapped so domain errors stay errors.Is-able.
func (r *Repository) withTx(ctx context.Context, fn func(q querier) error) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin session transaction: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := fn(tx); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit session transaction: %w", err)
	}
	return nil
}

// scanSession scans the canonical session projection; pgx.ErrNoRows is
// returned unwrapped so callers can map it to the domain error that matches
// their operation.
func scanSession(row pgx.Row) (Session, error) {
	var s Session
	err := row.Scan(
		&s.ID, &s.CircleID, &s.CreatedBy, &s.Status, &s.ScheduledAt, &s.ActualStart, &s.ActualEnd,
		&s.EndReason, &s.MediaMode, &s.MediaRoomRef, &s.IsLocked, &s.ParticipantCount, &s.CreatedAt, &s.UpdatedAt,
	)
	if err != nil {
		return Session{}, err
	}
	return s, nil
}

// sessionStateError explains why a compare-and-set lifecycle transition did
// not apply by loading the current session state.
func sessionStateError(ctx context.Context, q querier, sessionID, op string) error {
	current, err := scanSession(q.QueryRow(ctx, findSessionByIDQuery, sessionID))
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrSessionNotFound
	}
	if err != nil {
		return fmt.Errorf("%s: load session state: %w", op, err)
	}
	switch current.Status {
	case SessionStatusActive:
		return ErrSessionAlreadyActive
	case SessionStatusEnded:
		return ErrSessionAlreadyEnded
	default:
		return ErrSessionNotStartable
	}
}

// validateAdmission enforces the gates for a genuinely-new admission on a
// locked session row: the room must be unlocked first, then have remaining
// capacity (FR-009, FR-026). Active status and the already-present duplicate
// short-circuit are checked by the caller before these gates apply.
func validateAdmission(sess Session) error {
	if sess.IsLocked {
		return ErrSessionLocked
	}
	if sess.ParticipantCount >= maxParticipants {
		return ErrSessionFull
	}
	return nil
}

// validateActive enforces that the session is still joinable by status.
func validateActive(sess Session) error {
	switch sess.Status {
	case SessionStatusActive:
		return nil
	case SessionStatusEnded:
		return ErrSessionAlreadyEnded
	default:
		return ErrSessionNotStartable
	}
}

// presenceFacts are the eligibility facts of one participant for a session:
// whether a presence row exists, whether they are currently present, and
// whether they were durably removed.
type presenceFacts struct {
	found   bool
	present bool
	removed bool
}

// loadPresenceEligibility reports whether the participant has a presence row
// for the session and, if so, whether they are currently present or durably
// removed.
func loadPresenceEligibility(ctx context.Context, q querier, sessionID, userID string) (presenceFacts, error) {
	var removedAt *time.Time
	var facts presenceFacts
	err := q.QueryRow(ctx, findPresenceQuery, sessionID, userID).Scan(&facts.present, &removedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return presenceFacts{}, nil
	}
	if err != nil {
		return presenceFacts{}, fmt.Errorf("load presence: %w", err)
	}
	facts.found = true
	facts.removed = removedAt != nil
	return facts, nil
}

// currentlyPresent reports an existing, non-removed, currently present row —
// the idempotent duplicate-delivery state (FR-016).
func (f presenceFacts) currentlyPresent() bool {
	return f.found && f.present && !f.removed
}

// admitPresence transitions one participant to currently present under the
// session row lock, using eligibility facts already loaded by the caller. It
// reports whether the participant newly became present; duplicate deliveries
// for an already-present participant report false without mutating durable
// state (FR-016). A removed participant is never readmitted.
func admitPresence(ctx context.Context, q querier, sessionID, userID string, facts presenceFacts) (bool, error) {
	if facts.removed {
		return false, ErrParticipantRemoved
	}
	if facts.present {
		return false, nil
	}
	if !facts.found {
		if _, err := q.Exec(ctx, insertPresenceQuery, sessionID, userID); err != nil {
			return false, fmt.Errorf("admit presence: insert presence: %w", err)
		}
		return true, nil
	}
	if _, err := q.Exec(ctx, markPresencePresentQuery, sessionID, userID); err != nil {
		return false, fmt.Errorf("admit presence: mark present: %w", err)
	}
	return true, nil
}

// admitIfCapacity admits the participant and increments the durable count
// only when they newly transitioned to present.
func admitIfCapacity(ctx context.Context, q querier, sess Session, userID string, facts presenceFacts) (Session, error) {
	admitted, err := admitPresence(ctx, q, sess.ID, userID, facts)
	if err != nil {
		return Session{}, err
	}
	if !admitted {
		return sess, nil
	}
	updated, err := scanSession(q.QueryRow(ctx, incrementParticipantCountQuery, sess.ID))
	if err != nil {
		return Session{}, fmt.Errorf("admit participant: increment count: %w", err)
	}
	return updated, nil
}

// CreateAdHocSession persists a new F-005 ad-hoc session in scheduled status
// with scheduled_at NULL (FR-002). Circle membership authorization is a
// service-layer concern.
func (r *Repository) CreateAdHocSession(ctx context.Context, circleID, createdBy string) (Session, error) {
	created, err := scanSession(r.pool.QueryRow(ctx, insertAdHocSessionQuery, circleID, createdBy))
	if err != nil {
		return Session{}, fmt.Errorf("create ad-hoc session: %w", err)
	}
	return created, nil
}

// StartSession applies the scheduled→active compare-and-set and persists the
// opaque media room reference (data-model invariant 2). A zero-row update is
// mapped to the domain error matching the current state.
func (r *Repository) StartSession(ctx context.Context, sessionID string, roomRef MediaRoomRef) (Session, error) {
	started, err := scanSession(r.pool.QueryRow(ctx, startSessionQuery, sessionID, string(roomRef)))
	if errors.Is(err, pgx.ErrNoRows) {
		return Session{}, sessionStateError(ctx, r.pool, sessionID, "start session")
	}
	if err != nil {
		return Session{}, fmt.Errorf("start session: %w", err)
	}
	return started, nil
}

// EndSession applies the active→ended compare-and-set with its durable end
// reason, then clears only the transient current-presence and hand state
// while retaining durable history (FR-015). Provider room close is a service
// concern that happens after this persists.
func (r *Repository) EndSession(ctx context.Context, sessionID string, reason EndReason) (Session, error) {
	var ended Session
	err := r.withTx(ctx, func(q querier) error {
		s, err := scanSession(q.QueryRow(ctx, endSessionQuery, sessionID, string(reason)))
		if errors.Is(err, pgx.ErrNoRows) {
			return sessionStateError(ctx, q, sessionID, "end session")
		}
		if err != nil {
			return fmt.Errorf("end session: %w", err)
		}
		if _, err := q.Exec(ctx, clearSessionPresenceQuery, sessionID); err != nil {
			return fmt.Errorf("end session: clear presence: %w", err)
		}
		ended = s
		return nil
	})
	if err != nil {
		return Session{}, err
	}
	return ended, nil
}

// SetLock sets the room lock of an active session; replaying the current
// value converges to the same state (FR-016). The participant_count and
// lifecycle columns are untouched.
func (r *Repository) SetLock(ctx context.Context, sessionID string, locked bool) (Session, error) {
	updated, err := scanSession(r.pool.QueryRow(ctx, setSessionLockQuery, sessionID, locked))
	if errors.Is(err, pgx.ErrNoRows) {
		return Session{}, sessionStateError(ctx, r.pool, sessionID, "set session lock")
	}
	if err != nil {
		return Session{}, fmt.Errorf("set session lock: %w", err)
	}
	return updated, nil
}

// JoinSession admits a participant to an active, unlocked session under the
// capacity limit (FR-009). The session row is locked FOR UPDATE so join
// decisions serialize; the 0..50 CHECK constraint is the concurrency
// backstop. Duplicate joins for an already-present participant are idempotent
// no-ops even at capacity or under a lock (FR-016), and removed participants
// are rejected. For genuinely-new joins the lock gate takes precedence over
// capacity.
func (r *Repository) JoinSession(ctx context.Context, sessionID, userID string) (Session, error) {
	var joined Session
	err := r.withTx(ctx, func(q querier) error {
		sess, err := scanSession(q.QueryRow(ctx, lockSessionByIDQuery, sessionID))
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrSessionNotFound
		}
		if err != nil {
			return fmt.Errorf("join session: lock session: %w", err)
		}
		if err := validateActive(sess); err != nil {
			return err
		}
		facts, err := loadPresenceEligibility(ctx, q, sessionID, userID)
		if err != nil {
			return fmt.Errorf("join session: load presence: %w", err)
		}
		if facts.currentlyPresent() {
			// Already-present duplicate delivery: idempotent no-op that
			// bypasses the lock and capacity gates (FR-016).
			joined = sess
			return nil
		}
		if err := validateAdmission(sess); err != nil {
			return err
		}
		joined, err = admitIfCapacity(ctx, q, sess, userID, facts)
		return err
	})
	if err != nil {
		return Session{}, err
	}
	return joined, nil
}

// ReconnectPresence restores a previously joined participant to currently
// present. A reconnect for a currently-present participant is an idempotent
// no-op even at capacity (FR-016). Unlike a join, a room lock still permits
// an eligible pre-lock presence record to reconnect; a lock always rejects a
// first-time participant (FR-026). Removed or ended participants never
// reconnect.
func (r *Repository) ReconnectPresence(ctx context.Context, sessionID, userID string) (Session, error) {
	var reconnected Session
	err := r.withTx(ctx, func(q querier) error {
		sess, err := scanSession(q.QueryRow(ctx, lockSessionByIDQuery, sessionID))
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrSessionNotFound
		}
		if err != nil {
			return fmt.Errorf("reconnect presence: lock session: %w", err)
		}
		if err := validateActive(sess); err != nil {
			return err
		}
		facts, err := loadPresenceEligibility(ctx, q, sessionID, userID)
		if err != nil {
			return fmt.Errorf("reconnect presence: load presence: %w", err)
		}
		if facts.currentlyPresent() {
			reconnected = sess
			return nil
		}
		if sess.ParticipantCount >= maxParticipants {
			return ErrSessionFull
		}
		if sess.IsLocked && !facts.found {
			// Any existing eligible row necessarily predates the lock,
			// because locked rooms admit no new joins.
			return ErrSessionLocked
		}
		reconnected, err = admitIfCapacity(ctx, q, sess, userID, facts)
		return err
	})
	if err != nil {
		return Session{}, err
	}
	return reconnected, nil
}

// LeaveSession transitions a present participant out of the room and
// decrements the durable count. Leaving while absent is an idempotent no-op
// (FR-016).
func (r *Repository) LeaveSession(ctx context.Context, sessionID, userID string) (Session, error) {
	var left Session
	err := r.withTx(ctx, func(q querier) error {
		sess, err := scanSession(q.QueryRow(ctx, lockSessionByIDQuery, sessionID))
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrSessionNotFound
		}
		if err != nil {
			return fmt.Errorf("leave session: lock session: %w", err)
		}
		tag, err := q.Exec(ctx, markPresenceLeftQuery, sessionID, userID)
		if err != nil {
			return fmt.Errorf("leave session: mark presence left: %w", err)
		}
		if tag.RowsAffected() == 0 {
			left = sess
			return nil
		}
		left, err = scanSession(q.QueryRow(ctx, decrementParticipantCountQuery, sessionID))
		if err != nil {
			return fmt.Errorf("leave session: decrement count: %w", err)
		}
		return nil
	})
	if err != nil {
		return Session{}, err
	}
	return left, nil
}

// RemoveParticipant durably removes a participant: it sets removed_at, clears
// current presence and hand state, and decrements the count for a present
// participant. Duplicate removals converge without error; removing a
// participant who never joined reports ErrParticipantRemoved (FR-016).
func (r *Repository) RemoveParticipant(ctx context.Context, sessionID, userID string) (Session, error) {
	var removed Session
	err := r.withTx(ctx, func(q querier) error {
		sess, err := scanSession(q.QueryRow(ctx, lockSessionByIDQuery, sessionID))
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrSessionNotFound
		}
		if err != nil {
			return fmt.Errorf("remove participant: lock session: %w", err)
		}
		facts, err := loadPresenceEligibility(ctx, q, sessionID, userID)
		if err != nil {
			return fmt.Errorf("remove participant: %w", err)
		}
		if !facts.found {
			return ErrParticipantRemoved
		}
		if facts.removed {
			removed = sess
			return nil
		}
		if _, err := q.Exec(ctx, removePresenceQuery, sessionID, userID); err != nil {
			return fmt.Errorf("remove participant: apply removal: %w", err)
		}
		if !facts.present {
			removed = sess
			return nil
		}
		removed, err = scanSession(q.QueryRow(ctx, decrementParticipantCountQuery, sessionID))
		if err != nil {
			return fmt.Errorf("remove participant: decrement count: %w", err)
		}
		return nil
	})
	if err != nil {
		return Session{}, err
	}
	return removed, nil
}

// SetHandRaised records a raised hand for a currently present participant.
// Duplicate deliveries converge on the single raised state (FR-016). A
// participant who never joined, is not currently present, or was removed
// reports ErrParticipantRemoved.
func (r *Repository) SetHandRaised(ctx context.Context, sessionID, userID string) error {
	tag, err := r.pool.Exec(ctx, raiseHandQuery, sessionID, userID)
	if err != nil {
		return fmt.Errorf("raise hand: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrParticipantRemoved
	}
	return nil
}

// SetHandLowered clears the hand state of a currently present participant;
// duplicate deliveries converge on the lowered state (FR-016).
func (r *Repository) SetHandLowered(ctx context.Context, sessionID, userID string) error {
	tag, err := r.pool.Exec(ctx, lowerHandQuery, sessionID, userID)
	if err != nil {
		return fmt.Errorf("lower hand: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrParticipantRemoved
	}
	return nil
}

// GetSession loads one session by its identifier.
func (r *Repository) GetSession(ctx context.Context, sessionID string) (Session, error) {
	found, err := scanSession(r.pool.QueryRow(ctx, findSessionByIDQuery, sessionID))
	if errors.Is(err, pgx.ErrNoRows) {
		return Session{}, ErrSessionNotFound
	}
	if err != nil {
		return Session{}, fmt.Errorf("get session: %w", err)
	}
	return found, nil
}

// ListSessionParticipants returns the authoritative presence snapshot for a
// session ordered by first join (FR-013). It reports the durable rows;
// filtering by current presence is a projection concern.
func (r *Repository) ListSessionParticipants(ctx context.Context, sessionID string) ([]ParticipantPresence, error) {
	rows, err := r.pool.Query(ctx, listSessionParticipantsQuery, sessionID)
	if err != nil {
		return nil, fmt.Errorf("list session participants: %w", err)
	}
	defer rows.Close()
	participants := []ParticipantPresence{}
	for rows.Next() {
		var p ParticipantPresence
		if err := rows.Scan(
			&p.SessionID, &p.UserID, &p.DisplayName, &p.Role, &p.FirstJoinedAt, &p.LastJoinedAt, &p.LastLeftAt,
			&p.ReconnectCount, &p.IsCurrentlyPresent, &p.RemovedAt, &p.HandRaisedAt,
		); err != nil {
			return nil, fmt.Errorf("scan session participant: %w", err)
		}
		participants = append(participants, p)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate session participants: %w", err)
	}
	return participants, nil
}
