package auth

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

var ErrSessionNotFound = errors.New("session not found")
var ErrUserNotFound = errors.New("user not found")

// SessionRepository persists and invalidates user sessions.
type SessionRepository struct {
	pool *pgxpool.Pool
}

// NewSessionRepository constructs a session repository.
func NewSessionRepository(pool *pgxpool.Pool) *SessionRepository {
	return &SessionRepository{pool: pool}
}

// Upsert inserts or updates session state.
func (r *SessionRepository) Upsert(ctx context.Context, session Session) error {
	_, err := r.pool.Exec(
		ctx,
		upsertSessionQuery,
		session.ID,
		session.UserID,
		session.LastActivityAt.UTC(),
		nullIfZero(session.ExpiresAt),
		session.RevokedAt,
	)
	if err != nil {
		return fmt.Errorf("upsert session: %w", err)
	}

	return nil
}

// GetByID fetches a session by ID.
func (r *SessionRepository) GetByID(ctx context.Context, sessionID string) (Session, error) {
	var session Session
	var expiresAt time.Time
	err := r.pool.QueryRow(ctx, getSessionByIDQuery, sessionID).Scan(
		&session.ID,
		&session.UserID,
		&session.LastActivityAt,
		&expiresAt,
		&session.RevokedAt,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Session{}, ErrSessionNotFound
		}
		return Session{}, fmt.Errorf("get session: %w", err)
	}
	if !expiresAt.Equal(time.Unix(0, 0).UTC()) {
		session.ExpiresAt = expiresAt
	}

	return session, nil
}

// Touch updates the session activity timestamp.
func (r *SessionRepository) Touch(ctx context.Context, sessionID string, lastActivityAt time.Time) error {
	commandTag, err := r.pool.Exec(ctx, touchSessionQuery, sessionID, lastActivityAt.UTC())
	if err != nil {
		return fmt.Errorf("touch session: %w", err)
	}
	if commandTag.RowsAffected() == 0 {
		return ErrSessionNotFound
	}

	return nil
}

// GetLocalUserIDByFirebaseUID resolves a local user UUID by Firebase UID.
func (r *SessionRepository) GetLocalUserIDByFirebaseUID(ctx context.Context, firebaseUID string) (string, error) {
	var userID string
	err := r.pool.QueryRow(ctx, getLocalUserIDByFirebaseUIDQuery, firebaseUID).Scan(&userID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", ErrUserNotFound
		}
		return "", fmt.Errorf("get local user id: %w", err)
	}

	return userID, nil
}

// Revoke marks a session revoked for logout/session invalidation.
func (r *SessionRepository) Revoke(ctx context.Context, sessionID string, revokedAt time.Time) error {
	commandTag, err := r.pool.Exec(ctx, revokeSessionQuery, sessionID, revokedAt.UTC())
	if err != nil {
		return fmt.Errorf("revoke session: %w", err)
	}
	if commandTag.RowsAffected() == 0 {
		return ErrSessionNotFound
	}

	return nil
}

// RoleForUserInCircle returns the role of userID in circleID from circle_members.
// Satisfies middleware.CircleMembershipRepository.
func (r *SessionRepository) RoleForUserInCircle(ctx context.Context, circleID string, userID string) (string, error) {
	var role string
	err := r.pool.QueryRow(ctx, getCircleMemberRoleQuery, circleID, userID).Scan(&role)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", ErrSessionNotFound
		}
		return "", fmt.Errorf("get circle member role: %w", err)
	}
	return role, nil
}

func nullIfZero(ts time.Time) *time.Time {
	if ts.IsZero() {
		return nil
	}
	utc := ts.UTC()
	return &utc
}
