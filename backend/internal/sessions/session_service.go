package sessions

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// Per-circle roles as stored in circle_members (F-002). Only teachers and
// supervisors may start or create sessions; every active member may join
// (ADR-010, US1).
const (
	roleTeacher    = "teacher"
	roleSupervisor = "supervisor"
	roleStudent    = "student"
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

type moderationStore interface {
	SetLock(context.Context, string, bool) (Session, error)
	EndSession(context.Context, string, EndReason) (Session, error)
	ReconnectPresence(context.Context, string, string) (Session, error)
	RemoveParticipant(context.Context, string, string) (Session, error)
	SetHandRaised(context.Context, string, string) error
	SetHandLowered(context.Context, string, string) error
	ListSessionParticipants(context.Context, string) ([]ParticipantPresence, error)
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
		if err := s.gateway.CloseRoom(ctx, sess.MediaRoomRef); err != nil {
			return Session{}, fmt.Errorf("end session: close media room: %w", err)
		}
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
		"type": "session.snapshot", "timestamp": time.Now().UTC().Format(time.RFC3339),
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
	raised := command == "cmd.raise_hand"
	if !raised && command != "cmd.lower_hand" {
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
	eventType := "session.hand_lowered"
	if raised {
		eventType = "session.hand_raised"
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
