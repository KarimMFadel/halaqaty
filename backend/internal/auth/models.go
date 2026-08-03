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

// RegisterRequest is the payload for POST /auth/register. Identity is derived
// solely from the verified Firebase bearer token; this body never carries
// credentials or Firebase identifiers.
type RegisterRequest struct {
	DisplayName       string `json:"display_name"`
	PreferredLanguage string `json:"preferred_language"`
}

// CreateBackendSessionRequest is the optional payload for POST /auth/sessions.
type CreateBackendSessionRequest struct {
	DeviceName string `json:"device_name"`
}

// UserProfile is the API projection joining users and profiles.
type UserProfile struct {
	ID                string    `json:"id"`
	FirebaseUID       string    `json:"firebase_uid"`
	FullName          *string   `json:"full_name"`
	DisplayName       *string   `json:"display_name"`
	Bio               *string   `json:"bio"`
	Country           *string   `json:"country"`
	AvatarURL         *string   `json:"avatar_url"`
	Phone             *string   `json:"phone"`
	PreferredLanguage string    `json:"preferred_language"`
	CreatedAt         time.Time `json:"created_at"`
}

// BackendSessionResponse is returned by registration and session creation.
type BackendSessionResponse struct {
	SessionID string      `json:"session_id"`
	ExpiresAt time.Time   `json:"expires_at"`
	User      UserProfile `json:"user"`
}
