package auth

import "time"

// User is the local identity mapped to a verified Firebase account.
type User struct {
	ID          string
	FirebaseUID string
	Email       string
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// Profile is the 1:1 extension of a user identity.
type Profile struct {
	UserID            string
	FullName          *string
	DisplayName       *string
	Country           *string
	Phone             *string
	Bio               *string
	AvatarURL         *string
	PreferredLanguage *string
	CompletedAt       *time.Time
	CreatedAt         time.Time
	UpdatedAt         time.Time
}

// Session is the domain model for a backend-managed device session.
type Session struct {
	ID             string
	UserID         string
	DeviceName     *string
	LastActivityAt time.Time
	ExpiresAt      time.Time
	RevokedAt      *time.Time
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

// IsRevoked reports whether the session has been explicitly revoked.
func (s Session) IsRevoked() bool {
	return s.RevokedAt != nil
}

// IsExpired reports whether the absolute expiry has passed.
func (s Session) IsExpired(now time.Time) bool {
	return !s.ExpiresAt.IsZero() && !s.ExpiresAt.After(now)
}
