package auth

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	// ErrSessionNotFound is returned when a session ID does not exist.
	ErrSessionNotFound = errors.New("session not found")
	// ErrUserNotFound is returned when a Firebase UID or email cannot be resolved.
	ErrUserNotFound = errors.New("user not found")
	// ErrDuplicateEmail is returned when a user record with the same email already exists.
	ErrDuplicateEmail = errors.New("email already exists")
)

// SessionRepository persists and invalidates user sessions and identity mappings.
type SessionRepository struct {
	pool *pgxpool.Pool
}

// NewSessionRepository constructs a session repository.
func NewSessionRepository(pool *pgxpool.Pool) *SessionRepository {
	return &SessionRepository{pool: pool}
}

// UpsertUserByFirebaseUID provisions or refreshes a local user mapped to a Firebase UID.
func (r *SessionRepository) UpsertUserByFirebaseUID(ctx context.Context, firebaseUID, email string) (User, error) {
	var user User
	row := r.pool.QueryRow(ctx, upsertUserByFirebaseUIDQuery, firebaseUID, email)
	if err := row.Scan(&user.ID, &user.FirebaseUID, &user.Email, &user.CreatedAt, &user.UpdatedAt); err != nil {
		return User{}, fmt.Errorf("upsert user by firebase uid: %w", err)
	}
	return user, nil
}

// GetUserByFirebaseUID resolves a full user record by Firebase UID.
func (r *SessionRepository) GetUserByFirebaseUID(ctx context.Context, firebaseUID string) (User, error) {
	var user User
	err := r.pool.QueryRow(ctx, getUserByFirebaseUIDQuery, firebaseUID).Scan(
		&user.ID, &user.FirebaseUID, &user.Email, &user.CreatedAt, &user.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return User{}, ErrUserNotFound
		}
		return User{}, fmt.Errorf("get user by firebase uid: %w", err)
	}
	return user, nil
}

// GetUserByEmail resolves a user record by email address.
func (r *SessionRepository) GetUserByEmail(ctx context.Context, email string) (User, error) {
	var user User
	err := r.pool.QueryRow(ctx, getUserByEmailQuery, email).Scan(
		&user.ID, &user.FirebaseUID, &user.Email, &user.CreatedAt, &user.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return User{}, ErrUserNotFound
		}
		return User{}, fmt.Errorf("get user by email: %w", err)
	}
	return user, nil
}

// CreateEmptyProfile ensures a profile row exists for a user.
func (r *SessionRepository) CreateEmptyProfile(ctx context.Context, userID string) error {
	_, err := r.pool.Exec(ctx, createEmptyProfileQuery, userID)
	if err != nil {
		return fmt.Errorf("create empty profile: %w", err)
	}
	return nil
}

// CreateSession persists a new backend session.
func (r *SessionRepository) CreateSession(ctx context.Context, session Session) error {
	_, err := r.pool.Exec(
		ctx,
		createSessionQuery,
		session.ID,
		session.UserID,
		session.DeviceName,
		session.LastActivityAt.UTC(),
		session.ExpiresAt.UTC(),
	)
	if err != nil {
		return fmt.Errorf("create session: %w", err)
	}
	return nil
}

// GetByID fetches a session by its opaque UUID.
func (r *SessionRepository) GetByID(ctx context.Context, sessionID string) (Session, error) {
	return r.scanSession(ctx, r.pool.QueryRow(ctx, getSessionByIDQuery, sessionID))
}

// GetByIDAndUserID fetches a session only if it is owned by the given user.
func (r *SessionRepository) GetByIDAndUserID(ctx context.Context, sessionID, userID string) (Session, error) {
	return r.scanSession(ctx, r.pool.QueryRow(ctx, getSessionByIDAndUserIDQuery, sessionID, userID))
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

func (r *SessionRepository) scanSession(ctx context.Context, row pgx.Row) (Session, error) {
	var session Session
	var deviceName sql.NullString
	err := row.Scan(
		&session.ID,
		&session.UserID,
		&deviceName,
		&session.LastActivityAt,
		&session.ExpiresAt,
		&session.RevokedAt,
		&session.CreatedAt,
		&session.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Session{}, ErrSessionNotFound
		}
		return Session{}, fmt.Errorf("scan session: %w", err)
	}
	if deviceName.Valid {
		session.DeviceName = &deviceName.String
	}
	return session, nil
}
