package auth

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	// pgUniqueViolation is the PostgreSQL SQLSTATE for unique-constraint breaches.
	pgUniqueViolation = "23505"
	// usersEmailConstraint is the unique index on users.email from migration 000010.
	usersEmailConstraint = "users_email_key"
)

var (
	// ErrSessionNotFound is returned when a session ID does not exist.
	ErrSessionNotFound = errors.New("session not found")
	// ErrCircleMembershipNotFound is returned when a user is not a member of a circle.
	ErrCircleMembershipNotFound = errors.New("circle membership not found")
	// ErrUserNotFound is returned when a Firebase UID or email cannot be resolved.
	ErrUserNotFound = errors.New("user not found")
	// ErrDuplicateEmail is returned when a user record with the same email already exists.
	ErrDuplicateEmail = errors.New("email already exists")
	// ErrAccountDeleted is returned when a tombstoned identity tries to access an account.
	ErrAccountDeleted = errors.New("account is deleted")
	// ErrAccountIneligible is returned when a manager account tries student-only closure.
	ErrAccountIneligible = errors.New("account is not eligible for student deletion")
	// ErrAccountActiveSession is returned while the user has an active live-session credential.
	ErrAccountActiveSession = errors.New("account has an active live session")
)

// SessionRepository persists and invalidates user sessions and identity mappings.
type SessionRepository struct {
	pool *pgxpool.Pool
}

// NewSessionRepository constructs a session repository.
func NewSessionRepository(pool *pgxpool.Pool) *SessionRepository {
	return &SessionRepository{pool: pool}
}

// UpsertUserByFirebaseUID provisions or refreshes a local user mapped to a
// Firebase UID. inserted is true when a new user row was created, false when
// an existing user replayed registration. Returns ErrDuplicateEmail when the
// email belongs to a different Firebase UID.
func (r *SessionRepository) UpsertUserByFirebaseUID(ctx context.Context, firebaseUID, email string) (User, bool, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return User{}, false, fmt.Errorf("upsert user by firebase uid: begin transaction: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err := tx.Exec(ctx, lockFirebaseUIDQuery, firebaseUID); err != nil {
		return User{}, false, fmt.Errorf("upsert user by firebase uid: lock identity: %w", err)
	}
	var user User
	var inserted bool
	uidHash := sha256.Sum256([]byte(firebaseUID))
	row := tx.QueryRow(ctx, upsertUserByFirebaseUIDQuery, firebaseUID, email, uidHash[:])
	if err := row.Scan(&user.ID, &user.FirebaseUID, &user.Email, &user.CreatedAt, &user.UpdatedAt, &inserted); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return User{}, false, ErrAccountDeleted
		}
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == pgUniqueViolation && pgErr.ConstraintName == usersEmailConstraint {
			return User{}, false, ErrDuplicateEmail
		}
		return User{}, false, fmt.Errorf("upsert user by firebase uid: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return User{}, false, fmt.Errorf("commit user upsert: %w", err)
	}
	return user, inserted, nil
}

// UpsertProfileOnRegister writes display_name/preferred_language exactly once
// per user; replays leave an existing profile untouched.
func (r *SessionRepository) UpsertProfileOnRegister(ctx context.Context, userID, displayName, preferredLanguage string) error {
	_, err := r.pool.Exec(ctx, upsertProfileOnRegisterQuery, userID, displayName, preferredLanguage)
	if err != nil {
		return fmt.Errorf("upsert profile on register: %w", err)
	}
	return nil
}

// GetUserProfileByUserID reads the API profile projection for a user.
func (r *SessionRepository) GetUserProfileByUserID(ctx context.Context, userID string) (UserProfile, error) {
	var profile UserProfile
	var fullName, displayName, bio, country, avatarURL, phone sql.NullString
	err := r.pool.QueryRow(ctx, getUserProfileByUserIDQuery, userID).Scan(
		&profile.ID,
		&profile.FirebaseUID,
		&fullName,
		&displayName,
		&bio,
		&country,
		&avatarURL,
		&phone,
		&profile.PreferredLanguage,
		&profile.Timezone,
		&profile.CreatedAt,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return UserProfile{}, ErrUserNotFound
		}
		return UserProfile{}, fmt.Errorf("get user profile by user id: %w", err)
	}
	profile.FullName = nullStringPtr(fullName)
	profile.DisplayName = nullStringPtr(displayName)
	profile.Bio = nullStringPtr(bio)
	profile.Country = nullStringPtr(country)
	profile.AvatarURL = nullStringPtr(avatarURL)
	profile.Phone = nullStringPtr(phone)
	return profile, nil
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
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("create session: begin transaction: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var lockedUserID string
	if err := tx.QueryRow(ctx, lockActiveAccountQuery, session.UserID).Scan(&lockedUserID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrAccountDeleted
		}
		return fmt.Errorf("create session: lock account: %w", err)
	}
	if _, err := tx.Exec(ctx, createSessionQuery, session.ID, session.UserID, session.DeviceName, session.LastActivityAt.UTC(), session.ExpiresAt.UTC()); err != nil {
		return fmt.Errorf("create session: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit create session: %w", err)
	}
	return nil
}

// CloseStudentAccount irreversibly tombstones an eligible student, erases
// non-retained account data, and revokes every backend session atomically.
// It returns the Firebase UID for post-commit identity cleanup.
func (r *SessionRepository) CloseStudentAccount(ctx context.Context, userID string) (string, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return "", fmt.Errorf("begin account closure transaction: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var firebaseUID string
	if err := tx.QueryRow(ctx, getFirebaseUIDForActiveAccountQuery, userID).Scan(&firebaseUID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", ErrAccountDeleted
		}
		return "", fmt.Errorf("close account: load user identity: %w", err)
	}
	if _, err := tx.Exec(ctx, lockFirebaseUIDQuery, firebaseUID); err != nil {
		return "", fmt.Errorf("close account: lock Firebase identity: %w", err)
	}
	var lockedFirebaseUID string
	if err := tx.QueryRow(ctx, lockActiveAccountQuery, userID).Scan(&lockedFirebaseUID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", ErrAccountDeleted
		}
		return "", fmt.Errorf("close account: lock user: %w", err)
	}
	if lockedFirebaseUID != firebaseUID {
		return "", errors.New("close account: Firebase identity changed while locking")
	}
	var managerOrOwner bool
	if err := tx.QueryRow(ctx, hasActiveManagerCircleQuery, userID).Scan(&managerOrOwner); err != nil {
		return "", fmt.Errorf("close account: check manager circles: %w", err)
	}
	if managerOrOwner {
		return "", ErrAccountIneligible
	}
	var activeParticipant bool
	if err := tx.QueryRow(ctx, hasActiveSessionParticipantQuery, userID).Scan(&activeParticipant); err != nil {
		return "", fmt.Errorf("close account: check active sessions: %w", err)
	}
	if activeParticipant {
		return "", ErrAccountActiveSession
	}
	uidHash := sha256.Sum256([]byte(firebaseUID))
	if _, err := tx.Exec(ctx, closeAccountQuery, userID, uidHash[:]); err != nil {
		return "", fmt.Errorf("close account: tombstone user: %w", err)
	}
	if _, err := tx.Exec(ctx, scrubClosedProfileQuery, userID); err != nil {
		return "", fmt.Errorf("close account: scrub profile: %w", err)
	}
	if _, err := tx.Exec(ctx, deleteUserSessionsQuery, userID); err != nil {
		return "", fmt.Errorf("close account: revoke sessions: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return "", fmt.Errorf("commit account closure: %w", err)
	}
	return firebaseUID, nil
}

// ClearDeletedAccountFirebaseUID removes the Firebase UID after identity deletion.
func (r *SessionRepository) ClearDeletedAccountFirebaseUID(ctx context.Context, userID, firebaseUID string) error {
	if _, err := r.pool.Exec(ctx, clearDeletedAccountFirebaseUIDQuery, userID, firebaseUID); err != nil {
		return fmt.Errorf("clear deleted account Firebase UID: %w", err)
	}
	return nil
}

// ListPendingFirebaseDeletions pages tombstones by user ID for fair retries.
func (r *SessionRepository) ListPendingFirebaseDeletions(ctx context.Context, afterID *string, limit int) ([]PendingFirebaseDeletion, error) {
	rows, err := r.pool.Query(ctx, listPendingFirebaseDeletionsQuery, afterID, limit)
	if err != nil {
		return nil, fmt.Errorf("query pending Firebase deletions: %w", err)
	}
	defer rows.Close()
	items := make([]PendingFirebaseDeletion, 0)
	for rows.Next() {
		var item PendingFirebaseDeletion
		if err := rows.Scan(&item.UserID, &item.FirebaseUID); err != nil {
			return nil, fmt.Errorf("scan pending Firebase deletion: %w", err)
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate pending Firebase deletions: %w", err)
	}
	return items, nil
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
			return "", ErrCircleMembershipNotFound
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

func nullStringPtr(value sql.NullString) *string {
	if !value.Valid {
		return nil
	}
	return &value.String
}
