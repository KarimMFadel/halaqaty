package sessions

import "context"

// MediaGrants carries the audio-only connection entitlement the sessions
// domain grants to an authorized participant. Video publishing is never
// grantable, so no video field exists.
type MediaGrants struct {
	// CanPublishAudio is true for an authorized participant's audio connection.
	CanPublishAudio bool
}

// SessionMediaGateway is the provider-neutral audio media boundary owned by
// the sessions package (FR-019, ADR-015). Only the LiveKit adapter under
// backend/internal/sessions/livekit implements it; there is deliberately no
// provider registry, selection flag, or capability map (FR-021).
type SessionMediaGateway interface {
	// EnsureRoom makes the room referenced by roomRef exist with the audio
	// policy for mode. It is idempotent.
	EnsureRoom(ctx context.Context, roomRef MediaRoomRef, mode MediaMode) error
	// CloseRoom closes the room and disconnects its participants. It is
	// idempotent and safe to retry during cleanup.
	CloseRoom(ctx context.Context, roomRef MediaRoomRef) error
	// IssueConnection returns the participant-specific media connection for
	// userID with exactly the granted entitlements. On failure it returns an
	// error and never a partial connection (FR-017).
	IssueConnection(ctx context.Context, roomRef MediaRoomRef, userID string, grants MediaGrants) (MediaConnection, error)
	// MuteParticipant mutes the active audio of one connected participant.
	MuteParticipant(ctx context.Context, roomRef MediaRoomRef, userID string) error
	// UnmuteParticipant restores an existing publisher's audio only.
	UnmuteParticipant(ctx context.Context, roomRef MediaRoomRef, userID string) error
	// MuteAll mutes the active audio of every connected participant.
	MuteAll(ctx context.Context, roomRef MediaRoomRef) error
	// RemoveParticipant disconnects one participant from the room.
	RemoveParticipant(ctx context.Context, roomRef MediaRoomRef, userID string) error
}
