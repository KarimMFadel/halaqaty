package sessions

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/KarimMFadel/halaqaty/backend/internal/rbac"
)

// Per-circle roles as stored in circle_members (F-002). Only teachers and
// supervisors may start or create sessions; every active member may join
// (ADR-010, US1).
const (
	roleTeacher    = rbac.RoleTeacher
	roleSupervisor = rbac.RoleSupervisor
	roleStudent    = rbac.RoleStudent
)

// Store is the consumer-defined persistence port of the session service,
// satisfied by *Repository. PostgreSQL remains the sole source of truth.
type Store interface {
	// CreateAdHocSession persists a new scheduled ad-hoc session.
	CreateAdHocSession(ctx context.Context, circleID, createdBy string) (Session, error)
	// StartSession applies the scheduled→active compare-and-set and persists
	// the opaque media room reference.
	StartSession(ctx context.Context, sessionID string, roomRef MediaRoomRef) (Session, error)
	// StartSessionWithConnection serializes room ensure, activation, credential
	// issuance, and starter admission under the session lock.
	StartSessionWithConnection(ctx context.Context, sessionID, userID string, roomRef MediaRoomRef, grants MediaGrants, ensure func(context.Context, MediaRoomRef, MediaMode) error, issue func(context.Context, MediaRoomRef, MediaGrants) (MediaConnection, error)) (Session, MediaConnection, error)
	// JoinSession admits a participant under state, lock, and capacity gates.
	JoinSession(ctx context.Context, sessionID, userID string) (Session, error)
	// GetSession loads one session by identifier.
	GetSession(ctx context.Context, sessionID string) (Session, error)
}

type circleSessionListStore interface {
	ListCircleSessions(context.Context, string) ([]Session, error)
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
	store         Store
	gateway       SessionMediaGateway
	roles         CircleRoleReader
	roomKey       []byte
	queueObserver QueueObserver
}

type moderationStore interface {
	SetLock(context.Context, string, bool) (Session, error)
	EndSession(context.Context, string, EndReason) (Session, error)
	ReconnectPresence(context.Context, string, string) (Session, error)
	RemoveParticipant(context.Context, string, string) (Session, error)
	SetHandRaised(context.Context, string, string) error
	SetHandLowered(context.Context, string, string) error
	ListSessionParticipants(context.Context, string) ([]ParticipantPresence, error)
}

type connectionAdmissionStore interface {
	JoinSessionWithConnection(context.Context, string, string, MediaGrants, func(context.Context, MediaRoomRef, MediaGrants) (MediaConnection, error)) (Session, MediaConnection, error)
}

// NewService constructs the session service over its persistence, media
// gateway, and circle-role ports.
func NewService(store Store, gateway SessionMediaGateway, roles CircleRoleReader) *Service {
	return &Service{store: store, gateway: gateway, roles: roles}
}

// SetQueueObserver registers an optional post-commit F-003 lifecycle observer.
// Observer failure never changes the authoritative F-005 result.
func (s *Service) SetQueueObserver(observer QueueObserver) {
	if s != nil {
		s.queueObserver = observer
	}
}

// NewServiceWithRoomKey constructs a service with the backend-only HMAC key
// used to derive stable provider room references. NewService remains available
// for existing callers that have not enabled the recovery configuration yet.
func NewServiceWithRoomKey(store Store, gateway SessionMediaGateway, roles CircleRoleReader, roomKey []byte) (*Service, error) {
	if len(roomKey) == 0 {
		return nil, errors.New("session room key is required")
	}
	return &Service{store: store, gateway: gateway, roles: roles, roomKey: append([]byte(nil), roomKey...)}, nil
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

// ListCircleSessions returns discovery-safe scheduled and active sessions for
// an authorized circle member. Scheduling and attendance remain outside F-005.
func (s *Service) ListCircleSessions(ctx context.Context, actorID, circleID string) ([]Session, error) {
	if _, err := s.authorize(ctx, circleID, actorID, false); err != nil {
		return nil, err
	}
	store, ok := s.store.(circleSessionListStore)
	if !ok {
		return nil, errors.New("session discovery store is not configured")
	}
	sessions, err := store.ListCircleSessions(ctx, circleID)
	if err != nil {
		return nil, fmt.Errorf("list circle sessions: %w", err)
	}
	return sessions, nil
}

// StartSession activates a scheduled session and issues the starting
// moderator their publish-capable connection. It is idempotent: restarting an
// already-active session reuses the persisted room and issues a fresh
// identity-specific connection; concurrent starters converge on exactly one
// room. ADR-017 reconciliation cleans any deterministic orphan left by a
// rolled-back provider operation.
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
		if len(s.roomKey) == 0 {
			return Session{}, MediaConnection{}, errors.New("session room key is required")
		}
		candidate, err := StableMediaRoomRef(sessionID, s.roomKey)
		if err != nil {
			return Session{}, MediaConnection{}, fmt.Errorf("derive media room reference: %w", err)
		}
		roomRef = candidate
	} else if sess.Status != SessionStatusActive {
		return Session{}, MediaConnection{}, ErrSessionAlreadyEnded
	}
	started, conn, err := s.store.StartSessionWithConnection(ctx, sessionID, actorID, roomRef, MediaGrants{CanPublishAudio: true}, func(ensureCtx context.Context, candidate MediaRoomRef, mode MediaMode) error {
		if err := s.gateway.EnsureRoom(ensureCtx, candidate, mode); err != nil {
			return fmt.Errorf("ensure media room: %w: %v", ErrMediaUnavailable, err)
		}
		return nil
	}, func(issueCtx context.Context, persistedRoomRef MediaRoomRef, grants MediaGrants) (MediaConnection, error) {
		connection, err := s.gateway.IssueConnection(issueCtx, persistedRoomRef, actorID, grants)
		if err != nil {
			return MediaConnection{}, fmt.Errorf("issue connection: %w: %v", ErrMediaUnavailable, err)
		}
		return connection, nil
	})
	if err != nil {
		return Session{}, MediaConnection{}, fmt.Errorf("start session: %w", err)
	}
	if s.queueObserver != nil {
		_ = s.queueObserver.OnSessionStarted(ctx, sessionID)
	}
	return started, conn, nil
}

// JoinSession admits any active circle member to an active session and issues
// their least-privilege connection: authorized participants may publish audio,
// while F-003 queue state remains independent of media permission (ADR-020).
func (s *Service) JoinSession(ctx context.Context, actorID, sessionID string) (Session, MediaConnection, error) {
	sess, err := s.store.GetSession(ctx, sessionID)
	if err != nil {
		return Session{}, MediaConnection{}, err
	}
	_, err = s.authorize(ctx, sess.CircleID, actorID, false)
	if err != nil {
		return Session{}, MediaConnection{}, err
	}
	if admission, ok := s.store.(connectionAdmissionStore); ok {
		joined, conn, err := admission.JoinSessionWithConnection(ctx, sessionID, actorID, MediaGrants{CanPublishAudio: true}, func(issueCtx context.Context, roomRef MediaRoomRef, grants MediaGrants) (MediaConnection, error) {
			conn, err := s.gateway.IssueConnection(issueCtx, roomRef, actorID, grants)
			if err != nil {
				return MediaConnection{}, fmt.Errorf("%w: %v", ErrMediaUnavailable, err)
			}
			return conn, nil
		})
		if err != nil {
			return Session{}, MediaConnection{}, err
		}
		if s.queueObserver != nil {
			_ = s.queueObserver.OnParticipantJoined(ctx, sessionID, actorID)
		}
		return joined, conn, nil
	}
	joined, err := s.store.JoinSession(ctx, sessionID, actorID)
	if errors.Is(err, ErrSessionLocked) {
		reconnectStore, ok := s.store.(interface {
			ReconnectPresence(context.Context, string, string) (Session, error)
		})
		if !ok {
			return Session{}, MediaConnection{}, err
		}
		// Durable presence decides whether this is a first join or an eligible
		// pre-lock reconnect; clients never select the path.
		joined, err = reconnectStore.ReconnectPresence(ctx, sessionID, actorID)
	}
	if err != nil {
		return Session{}, MediaConnection{}, err
	}
	conn, err := s.gateway.IssueConnection(ctx, joined.MediaRoomRef, actorID, MediaGrants{CanPublishAudio: true})
	if err != nil {
		if leaver, ok := s.store.(interface {
			LeaveSession(context.Context, string, string) (Session, error)
		}); ok {
			_, _ = leaver.LeaveSession(ctx, sessionID, actorID)
		}
		return Session{}, MediaConnection{}, fmt.Errorf("join session: issue connection: %w: %v", ErrMediaUnavailable, err)
	}
	if s.queueObserver != nil {
		_ = s.queueObserver.OnParticipantJoined(ctx, sessionID, actorID)
	}
	return joined, conn, nil
}

func (s *Service) moderationStore() (moderationStore, error) {
	store, ok := s.store.(moderationStore)
	if !ok {
		return nil, errors.New("session store does not support moderation")
	}
	return store, nil
}

// SetLock changes the active room lock. Only teachers and supervisors may
// change it; replaying the current value is idempotent.
func (s *Service) SetLock(ctx context.Context, actorID, sessionID string, locked bool) (Session, error) {
	sess, err := s.store.GetSession(ctx, sessionID)
	if err != nil {
		return Session{}, err
	}
	if _, err := s.authorize(ctx, sess.CircleID, actorID, true); err != nil {
		return Session{}, err
	}
	store, err := s.moderationStore()
	if err != nil {
		return Session{}, err
	}
	return store.SetLock(ctx, sessionID, locked)
}

// EndSession ends the durable session and closes its provider room.
func (s *Service) EndSession(ctx context.Context, actorID, sessionID string, reason EndReason) (Session, error) {
	sess, err := s.store.GetSession(ctx, sessionID)
	if err != nil {
		return Session{}, err
	}
	if _, err := s.authorize(ctx, sess.CircleID, actorID, true); err != nil {
		return Session{}, err
	}
	store, err := s.moderationStore()
	if err != nil {
		return Session{}, err
	}
	ended, err := store.EndSession(ctx, sessionID, reason)
	if err != nil {
		return Session{}, err
	}
	if sess.MediaRoomRef != "" {
		// The ended transition is authoritative. Cleanup is idempotently retried
		// by the reconciler, so a provider close failure must not undo success.
		_ = s.gateway.CloseRoom(ctx, sess.MediaRoomRef)
	}
	return ended, nil
}

// MuteParticipant mutes one participant without changing entitlements.
func (s *Service) MuteParticipant(ctx context.Context, actorID, sessionID, targetID string) error {
	sess, err := s.store.GetSession(ctx, sessionID)
	if err != nil {
		return err
	}
	if _, err := s.authorize(ctx, sess.CircleID, actorID, true); err != nil {
		return err
	}
	return s.gateway.MuteParticipant(ctx, sess.MediaRoomRef, targetID)
}

// UnmuteParticipant restores only an existing audio publisher.
func (s *Service) UnmuteParticipant(ctx context.Context, actorID, sessionID, targetID string) error {
	sess, err := s.store.GetSession(ctx, sessionID)
	if err != nil {
		return err
	}
	if _, err := s.authorize(ctx, sess.CircleID, actorID, true); err != nil {
		return err
	}
	return s.gateway.UnmuteParticipant(ctx, sess.MediaRoomRef, targetID)
}

// MuteAll mutes all currently published audio tracks in the room.
func (s *Service) MuteAll(ctx context.Context, actorID, sessionID string) error {
	sess, err := s.store.GetSession(ctx, sessionID)
	if err != nil {
		return err
	}
	if _, err := s.authorize(ctx, sess.CircleID, actorID, true); err != nil {
		return err
	}
	return s.gateway.MuteAll(ctx, sess.MediaRoomRef)
}

// RemoveParticipant durably blocks the participant and disconnects them.
func (s *Service) RemoveParticipant(ctx context.Context, actorID, sessionID, targetID string) (Session, error) {
	sess, err := s.store.GetSession(ctx, sessionID)
	if err != nil {
		return Session{}, err
	}
	if _, err := s.authorize(ctx, sess.CircleID, actorID, true); err != nil {
		return Session{}, err
	}
	store, err := s.moderationStore()
	if err != nil {
		return Session{}, err
	}
	removed, err := store.RemoveParticipant(ctx, sessionID, targetID)
	if err != nil {
		return Session{}, err
	}
	if err := s.gateway.RemoveParticipant(ctx, sess.MediaRoomRef, targetID); err != nil {
		return Session{}, fmt.Errorf("remove participant: disconnect media participant: %w", err)
	}
	return removed, nil
}

// SetHand records a participant's standalone hand state.
func (s *Service) SetHand(ctx context.Context, actorID, sessionID string, raised bool) error {
	sess, err := s.store.GetSession(ctx, sessionID)
	if err != nil {
		return err
	}
	if _, err := s.authorize(ctx, sess.CircleID, actorID, false); err != nil {
		return err
	}
	store, err := s.moderationStore()
	if err != nil {
		return err
	}
	if raised {
		return store.SetHandRaised(ctx, sessionID, actorID)
	}
	return store.SetHandLowered(ctx, sessionID, actorID)
}

// IsModerator reports whether the caller's current circle role permits
// session moderation actions.
func (s *Service) IsModerator(ctx context.Context, actorID, sessionID string) (bool, error) {
	sess, err := s.store.GetSession(ctx, sessionID)
	if err != nil {
		return false, err
	}
	role, err := s.authorize(ctx, sess.CircleID, actorID, false)
	return moderatorRole(role), err
}

// ListParticipants returns the durable session presence snapshot after
// verifying that the caller is an active circle member.
func (s *Service) ListParticipants(ctx context.Context, actorID, sessionID string) ([]ParticipantPresence, error) {
	sess, err := s.store.GetSession(ctx, sessionID)
	if err != nil {
		return nil, err
	}
	if _, err := s.authorize(ctx, sess.CircleID, actorID, false); err != nil {
		return nil, err
	}
	store, err := s.moderationStore()
	if err != nil {
		return nil, err
	}
	return store.ListSessionParticipants(ctx, sessionID)
}

// RealtimeSnapshot returns the provider-neutral snapshot sent after an
// authorized session-topic subscription. It contains only public session and
// presence state.
func (s *Service) RealtimeSnapshot(ctx context.Context, actorID, sessionID string) (map[string]any, error) {
	sess, err := s.store.GetSession(ctx, sessionID)
	if err != nil {
		return nil, err
	}
	if err := s.AuthorizeSessionTopic(ctx, actorID, sessionID); err != nil {
		return nil, err
	}
	participants, err := s.ListParticipants(ctx, actorID, sessionID)
	if err != nil {
		return nil, err
	}
	items := make([]map[string]any, 0, len(participants))
	for _, participant := range participants {
		items = append(items, realtimeParticipant(participant))
	}
	return map[string]any{
		"type": sessionEventSnapshot, "timestamp": time.Now().UTC().Format(time.RFC3339),
		"payload": map[string]any{
			"session": map[string]any{
				"id": sess.ID, "status": sess.Status, "is_locked": sess.IsLocked,
			},
			"participants": items,
		},
	}, nil
}

// HandleRealtimeCommand applies a hand command after the hub has authorized
// the session topic and returns a redacted event envelope.
func (s *Service) HandleRealtimeCommand(ctx context.Context, actorID, sessionID, command string) (string, map[string]any, error) {
	raised := command == sessionCommandRaiseHand
	if !raised && command != sessionCommandLowerHand {
		return "", nil, errors.New("unsupported session command")
	}
	if err := s.SetHand(ctx, actorID, sessionID, raised); err != nil {
		return "", nil, err
	}
	participants, err := s.ListParticipants(ctx, actorID, sessionID)
	if err != nil {
		return "", nil, err
	}
	participantName := "Member"
	var handAt *time.Time
	for _, participant := range participants {
		if participant.UserID == actorID {
			if participant.DisplayName != "" {
				participantName = participant.DisplayName
			}
			handAt = participant.HandRaisedAt
			break
		}
	}
	eventType := sessionEventHandLowered
	if raised {
		eventType = sessionEventHandRaised
	}
	handState := "lowered"
	if handAt != nil {
		handState = handAt.UTC().Format(time.RFC3339Nano)
	}
	eventID := fmt.Sprintf("%s:%s:%s:%s", sessionID, command, actorID, handState)
	return eventID, map[string]any{
		"type":      eventType,
		"timestamp": time.Now().UTC().Format(time.RFC3339),
		"payload": map[string]any{
			"session_id": sessionID, "participant_id": actorID, "participant_name": participantName,
			"hand_raised_at": handAt,
		},
	}, nil
}

func realtimeParticipant(p ParticipantPresence) map[string]any {
	role := p.Role
	if role == "" {
		role = roleStudent
	}
	return map[string]any{
		"user_id": p.UserID, "display_name": p.DisplayName, "role": role,
		"is_currently_present": p.IsCurrentlyPresent, "hand_raised_at": p.HandRaisedAt,
	}
}

// AuthorizeSessionTopic revalidates membership and current presence before a
// WebSocket session-topic subscription is accepted.
func (s *Service) AuthorizeSessionTopic(ctx context.Context, actorID, sessionID string) error {
	sess, err := s.store.GetSession(ctx, sessionID)
	if err != nil {
		return err
	}
	if sess.Status != SessionStatusActive {
		return ErrSessionAlreadyEnded
	}
	if _, err := s.authorize(ctx, sess.CircleID, actorID, false); err != nil {
		return err
	}
	store, err := s.moderationStore()
	if err != nil {
		return err
	}
	participants, err := store.ListSessionParticipants(ctx, sessionID)
	if err != nil {
		return err
	}
	for _, participant := range participants {
		if participant.UserID == actorID && participant.IsCurrentlyPresent && participant.RemovedAt == nil {
			return nil
		}
	}
	return ErrParticipantRemoved
}
