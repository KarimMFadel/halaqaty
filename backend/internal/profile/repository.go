package profile

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/KarimMFadel/halaqaty/backend/internal/auth"
)

// Record is the profile persistence projection used by the service layer.
type Record struct {
	Profile     auth.UserProfile
	CompletedAt *time.Time
}

// UpdateInput is the persistence payload for profile writes.
type UpdateInput struct {
	UserID            string
	FullName          *string
	DisplayName       *string
	Bio               *string
	Country           *string
	AvatarURL         *string
	Phone             *string
	PreferredLanguage *string
	CompletedAt       *time.Time
}

// Repository is the PostgreSQL persistence boundary for profile data.
type Repository struct {
	pool *pgxpool.Pool
}

// NewRepository constructs a profile repository.
func NewRepository(pool *pgxpool.Pool) *Repository {
	return &Repository{pool: pool}
}

// GetByUserID loads a user's profile projection.
func (r *Repository) GetByUserID(ctx context.Context, userID string) (Record, error) {
	var profile auth.UserProfile
	var fullName, displayName, bio, country, avatarURL, phone sql.NullString
	var completedAt sql.NullTime
	err := r.pool.QueryRow(ctx, getProfileByUserIDQuery, userID).Scan(
		&profile.ID,
		&profile.FirebaseUID,
		&fullName,
		&displayName,
		&bio,
		&country,
		&avatarURL,
		&phone,
		&profile.PreferredLanguage,
		&profile.CreatedAt,
		&completedAt,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Record{}, auth.ErrUserNotFound
		}
		return Record{}, fmt.Errorf("get profile by user id: %w", err)
	}

	profile.FullName = nullStringPtr(fullName)
	profile.DisplayName = nullStringPtr(displayName)
	profile.Bio = nullStringPtr(bio)
	profile.Country = nullStringPtr(country)
	profile.AvatarURL = nullStringPtr(avatarURL)
	profile.Phone = nullStringPtr(phone)

	return Record{
		Profile:     profile,
		CompletedAt: nullTimePtr(completedAt),
	}, nil
}

// UpdateByUserID updates only supplied editable profile fields for one user.
func (r *Repository) UpdateByUserID(ctx context.Context, in UpdateInput) error {
	_, err := r.pool.Exec(
		ctx,
		updateProfileFieldsByUserIDQuery,
		in.UserID,
		derefOrNil(in.FullName),
		derefOrNil(in.DisplayName),
		derefOrNil(in.Bio),
		derefOrNil(in.Country),
		derefOrNil(in.AvatarURL),
		derefOrNil(in.Phone),
		derefOrNil(in.PreferredLanguage),
		in.CompletedAt,
	)
	if err != nil {
		return fmt.Errorf("update profile by user id: %w", err)
	}
	return nil
}

func nullStringPtr(value sql.NullString) *string {
	if !value.Valid {
		return nil
	}
	return &value.String
}

func nullTimePtr(value sql.NullTime) *time.Time {
	if !value.Valid {
		return nil
	}
	t := value.Time
	return &t
}

func derefOrNil(value *string) any {
	if value == nil {
		return nil
	}
	return *value
}
