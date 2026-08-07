package auth

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/KarimMFadel/halaqaty/backend/internal/platform/logging"
)

// defaultSessionAbsoluteTTL mirrors config.DefaultAuthConfig when no TTL is injected.
const defaultSessionAbsoluteTTL = 90 * 24 * time.Hour

// Store is the persistence contract required by Service. SessionRepository
// satisfies it against PostgreSQL; contract tests provide in-memory stubs.
type Store interface {
	UpsertUserByFirebaseUID(ctx context.Context, firebaseUID, email string) (User, bool, error)
	UpsertProfileOnRegister(ctx context.Context, userID, displayName, preferredLanguage string) error
	GetUserProfileByUserID(ctx context.Context, userID string) (UserProfile, error)
	CreateSession(ctx context.Context, session Session) error
	Revoke(ctx context.Context, sessionID string, revokedAt time.Time) error
}

// AuditLogger records security-relevant auth events.
type AuditLogger interface {
	Log(ctx context.Context, event logging.AuditEvent)
}

// noopAuditLogger discards events when no audit logger is configured.
type noopAuditLogger struct{}

func (noopAuditLogger) Log(context.Context, logging.AuditEvent) {}

// RegisterInput carries the verified Firebase identity and the registration
// payload. FirebaseUID/Email always come from the middleware principal, never
// from the request body.
type RegisterInput struct {
	FirebaseUID       string
	Email             string
	DisplayName       string
	PreferredLanguage string
}

// RegisterResult is the outcome of a registration call. Created is true for a
// first-time provisioning (HTTP 201) and false for an idempotent replay of the
// same Firebase identity (HTTP 409 with a fresh device session in the body).
type RegisterResult struct {
	Response BackendSessionResponse
	Created  bool
}

// Service groups authentication use cases.
type Service struct {
	store              Store
	audit              AuditLogger
	sessionAbsoluteTTL time.Duration
	nowFn              func() time.Time
}

// NewService constructs the auth application service.
func NewService(store Store, audit AuditLogger, sessionAbsoluteTTL time.Duration) *Service {
	if sessionAbsoluteTTL <= 0 {
		sessionAbsoluteTTL = defaultSessionAbsoluteTTL
	}
	if audit == nil {
		audit = noopAuditLogger{}
	}
	return &Service{
		store:              store,
		audit:              audit,
		sessionAbsoluteTTL: sessionAbsoluteTTL,
		nowFn:              time.Now,
	}
}

// Register provisions a new user (with profile) or replays an existing one,
// then issues a fresh current-device session in both cases. Replays never
// overwrite the stored profile. Returns ErrDuplicateEmail when the email is
// bound to a different Firebase UID.
func (s *Service) Register(ctx context.Context, in RegisterInput) (RegisterResult, error) {
	user, inserted, err := s.store.UpsertUserByFirebaseUID(ctx, in.FirebaseUID, in.Email)
	if err != nil {
		return RegisterResult{}, fmt.Errorf("register user: %w", err)
	}

	if inserted {
		if err := s.store.UpsertProfileOnRegister(ctx, user.ID, in.DisplayName, in.PreferredLanguage); err != nil {
			return RegisterResult{}, fmt.Errorf("register profile: %w", err)
		}
		s.audit.Log(ctx, logging.RegisterEvent(user.ID, user.Email))
	}

	response, err := s.newDeviceSession(ctx, user.ID, nil)
	if err != nil {
		return RegisterResult{}, err
	}
	return RegisterResult{Response: response, Created: inserted}, nil
}

// CreateSession issues a new current-device session for an already-registered user.
func (s *Service) CreateSession(ctx context.Context, userID string, deviceName *string) (BackendSessionResponse, error) {
	return s.newDeviceSession(ctx, userID, deviceName)
}

// Logout revokes exactly the given session. Session ownership and validity are
// enforced by the auth middleware before the handler runs.
func (s *Service) Logout(ctx context.Context, userID, sessionID string) error {
	if err := s.store.Revoke(ctx, sessionID, s.nowFn().UTC()); err != nil {
		return fmt.Errorf("logout revoke session: %w", err)
	}
	s.audit.Log(ctx, logging.LogoutEvent(userID, sessionID))
	return nil
}

func (s *Service) newDeviceSession(ctx context.Context, userID string, deviceName *string) (BackendSessionResponse, error) {
	now := s.nowFn().UTC().Truncate(time.Second)
	session := Session{
		ID:             uuid.NewString(),
		UserID:         userID,
		DeviceName:     deviceName,
		LastActivityAt: now,
		ExpiresAt:      now.Add(s.sessionAbsoluteTTL),
	}
	if err := s.store.CreateSession(ctx, session); err != nil {
		return BackendSessionResponse{}, fmt.Errorf("create session: %w", err)
	}

	profile, err := s.store.GetUserProfileByUserID(ctx, userID)
	if err != nil {
		return BackendSessionResponse{}, fmt.Errorf("load user profile: %w", err)
	}

	deviceLabel := ""
	if deviceName != nil {
		deviceLabel = *deviceName
	}
	s.audit.Log(ctx, logging.SessionCreateEvent(userID, session.ID, deviceLabel))

	return BackendSessionResponse{
		SessionID: session.ID,
		ExpiresAt: session.ExpiresAt,
		User:      profile,
	}, nil
}
