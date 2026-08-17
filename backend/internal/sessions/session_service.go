package sessions

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
)

// Per-circle roles as stored in circle_members (F-002). Only teachers and
// supervisors may start or create sessions; every active member may join
// (ADR-010, US1).
const (
	roleTeacher    = "teacher"
	roleSupervisor = "supervisor"
)

// Store is the consumer-defined persistence port of the session service,
// satisfied by *Repository. PostgreSQL remains the sole source of truth.
type Store interface {
	// CreateAdHocSession persists a new scheduled ad-hoc session.
	CreateAdHocSession(ctx context.Context, circleID, createdBy string) (Session, error)
	// StartSession applies the scheduled→active compare-and-set and persists
	// the opaque media room reference.
	StartSession(ctx context.Context, sessionID string, roomRef MediaRoomRef) (Session, error)
	// JoinSession admits a participant under state, lock, and capacity gates.
	JoinSession(ctx context.Context, sessionID, userID string) (Session, error)
	// GetSession loads one session by identifier.
	GetSession(ctx context.Context, sessionID string) (Session, error)
}

// CircleRoleReader is the F-002 authorization port: it returns the caller's
// role in the circle, or "" when the caller is not an active member.
type CircleRoleReader interface {
	RoleInCircle(ctx context.Context, circleID, userID string) (string, error)
}

// Service owns the provider-neutral start/join policy: role authorization,
// lifecycle compare-and-set, capacity, and least-privilege media connection
// issuance (US1). LiveKit specifics stay behind SessionMediaGateway (ADR-015).
type Service struct {
	store   Store
	gateway SessionMediaGateway
	roles   CircleRoleReader
}

// NewService constructs the session service over its persistence, media
// gateway, and circle-role ports.
func NewService(store Store, gateway SessionMediaGateway, roles CircleRoleReader) *Service {
	return &Service{store: store, gateway: gateway, roles: roles}
}

// moderatorRole reports whether the role may perform moderator lifecycle
// actions.
func moderatorRole(role string) bool {
	return role == roleTeacher || role == roleSupervisor
}

// authorize loads the caller's role in the session's circle and enforces the
// minimum membership requirement. It returns the role for grant decisions.
func (s *Service) authorize(ctx context.Context, circleID, actorID string, moderatorOnly bool) (string, error) {
	role, err := s.roles.RoleInCircle(ctx, circleID, actorID)
	if err != nil {
		return "", fmt.Errorf("authorize session access: %w", err)
	}
	if role == "" {
		return "", ErrNotCircleMember
	}
	if moderatorOnly && !moderatorRole(role) {
		return "", ErrModeratorRoleRequired
	}
	return role, nil
}

// CreateAdHocSession creates a scheduled audio-only session in the caller's
// circle. Only teachers and supervisors may create sessions (US1).
func (s *Service) CreateAdHocSession(ctx context.Context, actorID, circleID string) (Session, error) {
	if _, err := s.authorize(ctx, circleID, actorID, true); err != nil {
		return Session{}, err
	}
	created, err := s.store.CreateAdHocSession(ctx, circleID, actorID)
	if err != nil {
		return Session{}, fmt.Errorf("create ad-hoc session: %w", err)
	}
	return created, nil
}

// StartSession activates a scheduled session and issues the starting
// moderator their publish-capable connection. It is idempotent: restarting an
// already-active session reuses the persisted room and issues a fresh
// identity-specific connection; concurrent starters converge on exactly one
// room, and orphan rooms lost to a start race are closed.
func (s *Service) StartSession(ctx context.Context, actorID, sessionID string) (Session, MediaConnection, error) {
	sess, err := s.store.GetSession(ctx, sessionID)
	if err != nil {
		return Session{}, MediaConnection{}, err
	}
	if _, err := s.authorize(ctx, sess.CircleID, actorID, true); err != nil {
		return Session{}, MediaConnection{}, err
	}

	roomRef := sess.MediaRoomRef
	if sess.Status == SessionStatusScheduled {
		candidate := MediaRoomRef(uuid.NewString())
		if err := s.gateway.EnsureRoom(ctx, candidate, sess.MediaMode); err != nil {
			return Session{}, MediaConnection{}, fmt.Errorf("ensure media room: %w", err)
		}
		started, err := s.store.StartSession(ctx, sessionID, candidate)
		switch {
		case err == nil:
			sess, roomRef = started, candidate
		case errors.Is(err, ErrSessionAlreadyActive):
			// Lost the start race: close the orphan room and converge on the
			// persisted one.
			if err := s.gateway.CloseRoom(ctx, candidate); err != nil {
				return Session{}, MediaConnection{}, fmt.Errorf("close orphan media room: %w", err)
			}
			current, err := s.store.GetSession(ctx, sessionID)
			if err != nil {
				return Session{}, MediaConnection{}, err
			}
			if current.Status != SessionStatusActive || current.MediaRoomRef == "" {
				return Session{}, MediaConnection{}, fmt.Errorf("start session: %w", err)
			}
			sess, roomRef = current, current.MediaRoomRef
		default:
			// Terminal state or infrastructure failure: the candidate room is
			// an orphan here too.
			_ = s.gateway.CloseRoom(ctx, candidate)
			return Session{}, MediaConnection{}, err
		}
	} else if sess.Status != SessionStatusActive {
		return Session{}, MediaConnection{}, ErrSessionAlreadyEnded
	}

	// Admit the starter as a present participant; idempotent on restart.
	joined, err := s.store.JoinSession(ctx, sessionID, actorID)
	if err != nil {
		return Session{}, MediaConnection{}, fmt.Errorf("start session: admit starter: %w", err)
	}
	conn, err := s.gateway.IssueConnection(ctx, roomRef, actorID, MediaGrants{CanPublishAudio: true})
	if err != nil {
		return Session{}, MediaConnection{}, fmt.Errorf("start session: issue connection: %w", err)
	}
	return joined, conn, nil
}

// JoinSession admits any active circle member to an active session and issues
// their least-privilege connection: students never receive audio publishing
// outside an F-003 reciter turn (constitution §IV.4).
func (s *Service) JoinSession(ctx context.Context, actorID, sessionID string) (Session, MediaConnection, error) {
	sess, err := s.store.GetSession(ctx, sessionID)
	if err != nil {
		return Session{}, MediaConnection{}, err
	}
	role, err := s.authorize(ctx, sess.CircleID, actorID, false)
	if err != nil {
		return Session{}, MediaConnection{}, err
	}
	joined, err := s.store.JoinSession(ctx, sessionID, actorID)
	if err != nil {
		return Session{}, MediaConnection{}, err
	}
	conn, err := s.gateway.IssueConnection(ctx, joined.MediaRoomRef, actorID, MediaGrants{CanPublishAudio: moderatorRole(role)})
	if err != nil {
		return Session{}, MediaConnection{}, fmt.Errorf("join session: issue connection: %w", err)
	}
	return joined, conn, nil
}
