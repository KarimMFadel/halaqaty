// Package livekit is the sole backend LiveKit provider adapter (ADR-015):
// every import of LiveKit SDK types in the backend lives in this package.
// The sessions domain depends only on the provider-neutral
// sessions.SessionMediaGateway boundary.
package livekit

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/KarimMFadel/halaqaty/backend/internal/platform/config"
	"github.com/KarimMFadel/halaqaty/backend/internal/sessions"
	"github.com/livekit/protocol/auth"
	lkmodel "github.com/livekit/protocol/livekit"
	"github.com/livekit/psrpc"
)

// maxCredentialLifetime is the one-hour maximum lifetime of every media
// credential the backend issues (F-005 spec, constitution §IV).
const maxCredentialLifetime = time.Hour

// reciterAudioCallTimeout bounds every reciter grant/revoke provider call at
// five seconds (F-003 plan D8): a slow or hung provider call fails fast while
// the committed PostgreSQL queue truth stays authoritative.
const reciterAudioCallTimeout = 5 * time.Second

// audioPublishSources is the single definition of audio-only publishing
// (constitution §V): microphone only, so video is never publishable. It backs
// both connection tokens and server-side reciter permission updates.
var audioPublishSources = []lkmodel.TrackSource{lkmodel.TrackSource_MICROPHONE}

// roomIdleTimeoutSeconds implements the frozen MVP rule "30-minute idle room
// timeout after last participant leaves" (constitution, key business rules).
const roomIdleTimeoutSeconds uint32 = 30 * 60

// maxRoomParticipants mirrors the sessions capacity of 50 (FR-009) at the
// provider so the room itself refuses the 51st connection.
const maxRoomParticipants uint32 = 50

// roomClient is the narrow LiveKit RoomService seam the adapter needs; it is
// satisfied by *lksdk.RoomServiceClient in production wiring and by fakes in
// tests. Room-level audio fidelity (Opus ≥48 kbps, no NS/AGC/EC) is enforced
// publisher-side by the mobile adapter; the server room API carries no audio
// codec settings, so the backend enforces capacity and idle timeout here.
type roomClient interface {
	CreateRoom(ctx context.Context, req *lkmodel.CreateRoomRequest) (*lkmodel.Room, error)
	DeleteRoom(ctx context.Context, req *lkmodel.DeleteRoomRequest) (*lkmodel.DeleteRoomResponse, error)
	ListParticipants(ctx context.Context, req *lkmodel.ListParticipantsRequest) (*lkmodel.ListParticipantsResponse, error)
	RemoveParticipant(ctx context.Context, req *lkmodel.RoomParticipantIdentity) (*lkmodel.RemoveParticipantResponse, error)
	MutePublishedTrack(ctx context.Context, req *lkmodel.MuteRoomTrackRequest) (*lkmodel.MuteRoomTrackResponse, error)
}

// reciterPermissionClient is the additional RoomService seam the F-003
// ReciterAudioControl implementation needs; it is satisfied by
// *lksdk.RoomServiceClient in production wiring and by the reciter fake in
// tests. It is kept separate from roomClient so the F-005 room-lifecycle seam
// stays unchanged.
type reciterPermissionClient interface {
	UpdateParticipant(ctx context.Context, req *lkmodel.UpdateParticipantRequest) (*lkmodel.ParticipantInfo, error)
}

// Adapter implements sessions.SessionMediaGateway against self-hosted
// LiveKit. There is deliberately no provider selection, registry, or flag
// (FR-021): this package is the one injected implementation.
type Adapter struct {
	cfg    config.LiveKitConfig
	policy config.AudioPolicy
	rooms  roomClient
	// roomKey is the backend-only HMAC key used to derive stable room
	// references from session IDs for the reciter audio boundary.
	roomKey []byte
	// reciterTimeout overrides reciterAudioCallTimeout when positive; it
	// exists so tests can shorten bounded provider calls.
	reciterTimeout time.Duration
}

// NewAdapter constructs the LiveKit adapter from validated configuration and
// an injected RoomService client. The returned adapter does not support
// ReciterAudioControl; use NewAdapterWithRoomKey for that.
func NewAdapter(cfg config.LiveKitConfig, policy config.AudioPolicy, rooms roomClient) *Adapter {
	return &Adapter{cfg: cfg, policy: policy, rooms: rooms}
}

// NewAdapterWithRoomKey additionally equips the adapter with the backend-only
// HMAC room key that derives stable room references from session IDs, which
// the sessions.ReciterAudioControl implementation requires. It rejects an
// empty key.
func NewAdapterWithRoomKey(cfg config.LiveKitConfig, policy config.AudioPolicy, rooms roomClient, roomKey []byte) (*Adapter, error) {
	if len(roomKey) == 0 {
		return nil, errors.New("livekit adapter room key is required")
	}
	return &Adapter{cfg: cfg, policy: policy, rooms: rooms, roomKey: append([]byte(nil), roomKey...)}, nil
}

// EnsureRoom makes the room exist with the F-005 room policy. CreateRoom is
// idempotent on LiveKit, so retries converge.
func (a *Adapter) EnsureRoom(ctx context.Context, roomRef sessions.MediaRoomRef, mode sessions.MediaMode) error {
	if mode != sessions.MediaModeAudioOnly {
		return fmt.Errorf("ensure media room: media mode %q is not supported in F-005", mode)
	}
	req := &lkmodel.CreateRoomRequest{
		Name:            string(roomRef),
		MaxParticipants: maxRoomParticipants,
		EmptyTimeout:    roomIdleTimeoutSeconds,
	}
	if _, err := a.rooms.CreateRoom(ctx, req); err != nil {
		return fmt.Errorf("ensure media room: %w", err)
	}
	return nil
}

// CloseRoom closes the provider room and disconnects its participants.
func (a *Adapter) CloseRoom(ctx context.Context, roomRef sessions.MediaRoomRef) error {
	if _, err := a.rooms.DeleteRoom(ctx, &lkmodel.DeleteRoomRequest{Room: string(roomRef)}); err != nil {
		if code, ok := psrpc.GetErrorCode(err); ok && code == psrpc.NotFound {
			return nil
		}
		return fmt.Errorf("close media room: %w", err)
	}
	return nil
}

// IssueConnection returns the participant-specific signed credential with
// exactly the granted entitlements. Video publishing is never grantable
// (constitution §V): audio publishers are restricted to the microphone
// source, and listen-only participants may not publish at all.
func (a *Adapter) IssueConnection(_ context.Context, roomRef sessions.MediaRoomRef, userID string, grants sessions.MediaGrants) (sessions.MediaConnection, error) {
	issuedAt := time.Now().UTC().Truncate(time.Second)
	grant := &auth.VideoGrant{RoomJoin: true, Room: string(roomRef)}
	grant.SetCanSubscribe(true)
	if grants.CanPublishAudio {
		grant.SetCanPublish(true)
		grant.SetCanPublishSources(audioPublishSources)
	} else {
		grant.SetCanPublish(false)
	}
	token, err := auth.NewAccessToken(a.cfg.APIKey, a.cfg.APISecret).
		SetIdentity(userID).
		SetValidFor(maxCredentialLifetime).
		SetVideoGrant(grant).
		ToJWT()
	if err != nil {
		return sessions.MediaConnection{}, fmt.Errorf("issue media connection: sign credential: %w", err)
	}
	return sessions.MediaConnection{
		Endpoint:   a.cfg.Endpoint,
		Credential: sessions.MediaCredential(token),
		ExpiresAt:  issuedAt.Add(maxCredentialLifetime),
	}, nil
}

// MuteParticipant mutes the active audio of one connected participant; a
// participant without a published audio track is a no-op.
func (a *Adapter) MuteParticipant(ctx context.Context, roomRef sessions.MediaRoomRef, userID string) error {
	return a.muteListed(ctx, roomRef, userID, true)
}

// UnmuteParticipant restores an existing participant's microphone track; it
// never changes the participant's publish entitlement.
func (a *Adapter) UnmuteParticipant(ctx context.Context, roomRef sessions.MediaRoomRef, userID string) error {
	return a.muteListed(ctx, roomRef, userID, false)
}

// MuteAll mutes the active audio of every connected audio publisher.
func (a *Adapter) MuteAll(ctx context.Context, roomRef sessions.MediaRoomRef) error {
	return a.muteListed(ctx, roomRef, "", true)
}

// muteListed mutes userID's audio track, or every audio publisher's when
// userID is empty.
func (a *Adapter) muteListed(ctx context.Context, roomRef sessions.MediaRoomRef, userID string, muted bool) error {
	listed, err := a.rooms.ListParticipants(ctx, &lkmodel.ListParticipantsRequest{Room: string(roomRef)})
	if err != nil {
		return fmt.Errorf("mute audio: list participants: %w", err)
	}
	for _, p := range listed.Participants {
		if userID != "" && p.Identity != userID {
			continue
		}
		for _, track := range p.Tracks {
			if track.Type != lkmodel.TrackType_AUDIO && track.Source != lkmodel.TrackSource_MICROPHONE {
				continue
			}
			if _, err := a.rooms.MutePublishedTrack(ctx, &lkmodel.MuteRoomTrackRequest{
				Room:     string(roomRef),
				Identity: p.Identity,
				TrackSid: track.Sid,
				Muted:    muted,
			}); err != nil {
				return fmt.Errorf("mute audio: %w", err)
			}
		}
	}
	return nil
}

// RemoveParticipant disconnects one participant from the room.
func (a *Adapter) RemoveParticipant(ctx context.Context, roomRef sessions.MediaRoomRef, userID string) error {
	if _, err := a.rooms.RemoveParticipant(ctx, &lkmodel.RoomParticipantIdentity{
		Room:     string(roomRef),
		Identity: userID,
	}); err != nil {
		return fmt.Errorf("remove media participant: %w", err)
	}
	return nil
}

// GrantReciterAudio grants userID temporary audio-only publishing for the
// recitation turn identified by roundID and queueEntryID inside the live
// session sessionID (sessions.ReciterAudioControl, F-003 plan D8). The room is
// derived deterministically from sessionID with the same StableMediaRoomRef
// derivation the session service uses; roundID and queueEntryID are neutral
// correlation identifiers that appear in returned error context only. The
// provider call is bounded by reciterAudioCallTimeout and is idempotent:
// granting an already-granted user applies the same permission again.
func (a *Adapter) GrantReciterAudio(ctx context.Context, sessionID, roundID, queueEntryID, userID string) error {
	return a.updateReciterPermission(ctx, "grant reciter audio", sessionID, roundID, queueEntryID, userID,
		&lkmodel.ParticipantPermission{
			CanSubscribe:      true,
			CanPublish:        true,
			CanPublishData:    false,
			CanPublishSources: audioPublishSources, // audio-only: video never publishable (constitution §V)
		})
}

// RevokeReciterAudio removes the reciter audio publishing entitlement while
// keeping listening rights (sessions.ReciterAudioControl, F-003 plan D8). It
// is idempotent: revoking a participant who holds no grant, or whose room is
// already gone, succeeds.
func (a *Adapter) RevokeReciterAudio(ctx context.Context, sessionID, roundID, queueEntryID, userID string) error {
	return a.updateReciterPermission(ctx, "revoke reciter audio", sessionID, roundID, queueEntryID, userID,
		&lkmodel.ParticipantPermission{
			CanSubscribe:   true,
			CanPublish:     false,
			CanPublishData: false,
		})
}

// updateReciterPermission applies the reciter permission through the
// participant-permission seam under the bounded call timeout. Errors are
// secret-safe: no provider identifier, endpoint, key, or derived room
// reference is ever returned.
func (a *Adapter) updateReciterPermission(ctx context.Context, op, sessionID, roundID, queueEntryID, userID string, permission *lkmodel.ParticipantPermission) error {
	updater, ok := a.rooms.(reciterPermissionClient)
	if !ok {
		return fmt.Errorf("%s: room client does not support participant permission updates", op)
	}
	roomRef, err := sessions.StableMediaRoomRef(sessionID, a.roomKey)
	if err != nil {
		return fmt.Errorf("%s: derive media room reference: %w", op, err)
	}
	timeout := a.reciterTimeout
	if timeout <= 0 {
		timeout = reciterAudioCallTimeout
	}
	callCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	if _, err := updater.UpdateParticipant(callCtx, &lkmodel.UpdateParticipantRequest{
		Room:       string(roomRef),
		Identity:   userID,
		Permission: permission,
	}); err != nil {
		if !permission.CanPublish {
			if code, ok := psrpc.GetErrorCode(err); ok && code == psrpc.NotFound {
				// The desired end state (no grant, or no room) already holds.
				return nil
			}
		}
		if callCtx.Err() != nil {
			return fmt.Errorf("%s round=%s entry=%s: %w", op, roundID, queueEntryID, callCtx.Err())
		}
		// Provider errors may carry endpoints, keys, or room references;
		// they are deliberately not wrapped into the returned error (SC-005).
		return fmt.Errorf("%s round=%s entry=%s: media provider operation failed", op, roundID, queueEntryID)
	}
	return nil
}
