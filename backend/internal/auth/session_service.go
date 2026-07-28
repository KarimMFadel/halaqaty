package auth

import "time"

// Session is the domain model for backend-managed user sessions.
type Session struct {
	ID             string
	UserID         string
	LastActivityAt time.Time
	ExpiresAt      time.Time
	RevokedAt      *time.Time
}

// SessionService enforces inactivity and absolute session expiration.
type SessionService struct {
	inactivityTimeout time.Duration
	nowFn             func() time.Time
}

// NewSessionService creates a service with the inactivity timeout.
func NewSessionService(inactivityTimeout time.Duration) *SessionService {
	return &SessionService{
		inactivityTimeout: inactivityTimeout,
		nowFn:             time.Now,
	}
}

// IsExpired returns true when the session is revoked, expired, or inactive too long.
func (s *SessionService) IsExpired(session Session) bool {
	now := s.nowFn().UTC()
	if session.RevokedAt != nil {
		return true
	}
	if !session.ExpiresAt.IsZero() && !session.ExpiresAt.UTC().After(now) {
		return true
	}
	return now.Sub(session.LastActivityAt.UTC()) > s.inactivityTimeout
}

// Touch updates last activity to current time.
func (s *SessionService) Touch(session *Session) {
	session.LastActivityAt = s.nowFn().UTC()
}
