// Package livekit is the sole backend LiveKit provider adapter (ADR-015):
// every import of LiveKit SDK types in the backend lives in this package.
// The sessions domain depends only on the provider-neutral
// sessions.SessionMediaGateway boundary.
package livekit

import (
	"context"
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

// audioPublishSources is the single definition of audio-only publishing
// (constitution §V): microphone only, so video is never publishable. It backs
// connection tokens.
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

// Adapter implements sessions.SessionMediaGateway against self-hosted
// LiveKit. There is deliberately no provider selection, registry, or flag
// (FR-021): this package is the one injected implementation.
type Adapter struct {
	cfg    config.LiveKitConfig
	policy config.AudioPolicy
	rooms  roomClient
}

// NewAdapter constructs the LiveKit adapter from validated configuration and
// an injected RoomService client.
func NewAdapter(cfg config.LiveKitConfig, policy config.AudioPolicy, rooms roomClient) *Adapter {
	return &Adapter{cfg: cfg, policy: policy, rooms: rooms}
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
