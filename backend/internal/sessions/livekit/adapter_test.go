package livekit

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/KarimMFadel/halaqaty/backend/internal/platform/config"
	"github.com/KarimMFadel/halaqaty/backend/internal/sessions"
	lkmodel "github.com/livekit/protocol/livekit"
)

// ---- test doubles -----------------------------------------------------------

// fakeRoomClient records RoomService traffic without a LiveKit server.
type fakeRoomClient struct {
	created  []*lkmodel.CreateRoomRequest
	deleted  []*lkmodel.DeleteRoomRequest
	removed  []*lkmodel.RoomParticipantIdentity
	muted    []*lkmodel.MuteRoomTrackRequest
	listed   int
	partics  []*lkmodel.ParticipantInfo
	listErr  error
	createE2 error
}

func (f *fakeRoomClient) CreateRoom(_ context.Context, req *lkmodel.CreateRoomRequest) (*lkmodel.Room, error) {
	if f.createE2 != nil {
		return nil, f.createE2
	}
	f.created = append(f.created, req)
	return &lkmodel.Room{Name: req.Name}, nil
}

func (f *fakeRoomClient) DeleteRoom(_ context.Context, req *lkmodel.DeleteRoomRequest) (*lkmodel.DeleteRoomResponse, error) {
	f.deleted = append(f.deleted, req)
	return &lkmodel.DeleteRoomResponse{}, nil
}

func (f *fakeRoomClient) ListParticipants(_ context.Context, _ *lkmodel.ListParticipantsRequest) (*lkmodel.ListParticipantsResponse, error) {
	f.listed++
	if f.listErr != nil {
		return nil, f.listErr
	}
	return &lkmodel.ListParticipantsResponse{Participants: f.partics}, nil
}

func (f *fakeRoomClient) RemoveParticipant(_ context.Context, req *lkmodel.RoomParticipantIdentity) (*lkmodel.RemoveParticipantResponse, error) {
	f.removed = append(f.removed, req)
	return &lkmodel.RemoveParticipantResponse{}, nil
}

func (f *fakeRoomClient) MutePublishedTrack(_ context.Context, req *lkmodel.MuteRoomTrackRequest) (*lkmodel.MuteRoomTrackResponse, error) {
	f.muted = append(f.muted, req)
	return &lkmodel.MuteRoomTrackResponse{}, nil
}

func newTestAdapter(rooms *fakeRoomClient) (*Adapter, config.AudioPolicy) {
	cfg := config.LiveKitConfig{
		Endpoint:  "wss://media.example.com",
		APIKey:    "test-key",
		APISecret: "test-secret-value-not-a-real-credential",
	}
	policy := config.DefaultAudioPolicy()
	return NewAdapter(cfg, policy, rooms), policy
}

// decodeJWTClaims extracts the payload claims of a signed JWT without
// verifying it (verification is LiveKit's server job).
func decodeJWTClaims(t *testing.T, token string) map[string]any {
	t.Helper()
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		t.Fatalf("credential is not a JWS, got %d segments", len(parts))
	}
	raw, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		t.Fatalf("decode JWT payload: %v", err)
	}
	claims := map[string]any{}
	if err := json.Unmarshal(raw, &claims); err != nil {
		t.Fatalf("parse JWT claims: %v", err)
	}
	return claims
}

func videoGrant(t *testing.T, claims map[string]any) map[string]any {
	t.Helper()
	video, ok := claims["video"].(map[string]any)
	if !ok {
		t.Fatalf("token has no video grant: %v", claims)
	}
	return video
}

// ---- T018: connection issuance ------------------------------------------------

func TestIssueConnectionStudentIsListenOnly(t *testing.T) {
	rooms := &fakeRoomClient{}
	adapter, _ := newTestAdapter(rooms)

	conn, err := adapter.IssueConnection(context.Background(), "room-ref-1", "student-1", sessions.MediaGrants{CanPublishAudio: false})
	if err != nil {
		t.Fatalf("IssueConnection: %v", err)
	}
	if conn.Endpoint != "wss://media.example.com" {
		t.Fatalf("endpoint = %q, want configured endpoint", conn.Endpoint)
	}
	if conn.Credential == "" {
		t.Fatal("credential must be a non-empty signed token")
	}

	claims := decodeJWTClaims(t, string(conn.Credential))
	grant := videoGrant(t, claims)
	if grant["roomJoin"] != true {
		t.Fatalf("roomJoin = %v, want true", grant["roomJoin"])
	}
	if grant["room"] != "room-ref-1" {
		t.Fatalf("room = %v, want room-ref-1", grant["room"])
	}
	if canPublish, ok := grant["canPublish"].(bool); !ok || canPublish {
		t.Fatalf("student canPublish = %v, want explicit false", grant["canPublish"])
	}
	if claims["sub"] != "student-1" && claims["identity"] != "student-1" {
		t.Fatalf("token identity = %v, want student-1", claims["sub"])
	}
}

func TestIssueConnectionModeratorPublishesAudioOnly(t *testing.T) {
	rooms := &fakeRoomClient{}
	adapter, _ := newTestAdapter(rooms)

	conn, err := adapter.IssueConnection(context.Background(), "room-ref-2", "teacher-1", sessions.MediaGrants{CanPublishAudio: true})
	if err != nil {
		t.Fatalf("IssueConnection: %v", err)
	}
	grant := videoGrant(t, decodeJWTClaims(t, string(conn.Credential)))

	sources, _ := grant["canPublishSources"].([]any)
	if len(sources) == 0 {
		t.Fatal("moderator grant must restrict publishing to explicit audio sources (constitution §V: video never publishable)")
	}
	for _, s := range sources {
		name := strings.ToUpper(fmt.Sprint(s))
		if name != "MICROPHONE" {
			t.Fatalf("moderator may only publish microphone audio, found source %v", s)
		}
	}
}

func TestIssueConnectionCredentialLifetimeCappedAtOneHour(t *testing.T) {
	rooms := &fakeRoomClient{}
	adapter, _ := newTestAdapter(rooms)

	before := time.Now()
	conn, err := adapter.IssueConnection(context.Background(), "room-ref-3", "teacher-1", sessions.MediaGrants{CanPublishAudio: true})
	if err != nil {
		t.Fatalf("IssueConnection: %v", err)
	}
	claims := decodeJWTClaims(t, string(conn.Credential))
	iat, exp := int64(claims["iat"].(float64)), int64(claims["exp"].(float64))
	if lifetime := time.Duration(exp-iat) * time.Second; lifetime > time.Hour {
		t.Fatalf("credential lifetime %v exceeds the one-hour maximum", lifetime)
	}
	if !conn.ExpiresAt.After(before) {
		t.Fatal("connection expiry must be in the future")
	}
	if until := conn.ExpiresAt.Sub(before); until > time.Hour {
		t.Fatalf("connection expiry %v exceeds one hour from issuance", until)
	}
}

func TestIssueConnectionFailureReturnsNoPartialConnection(t *testing.T) {
	rooms := &fakeRoomClient{createE2: fmt.Errorf("boom")}
	adapter, _ := newTestAdapter(rooms)
	if err := adapter.EnsureRoom(context.Background(), "room-ref-4", sessions.MediaModeAudioOnly); err == nil {
		t.Fatal("EnsureRoom must surface provider failures")
	}
}

// ---- T018: room lifecycle ------------------------------------------------------

func TestEnsureRoomAppliesCapacityAndIdleTimeout(t *testing.T) {
	rooms := &fakeRoomClient{}
	adapter, _ := newTestAdapter(rooms)

	if err := adapter.EnsureRoom(context.Background(), "room-ref-5", sessions.MediaModeAudioOnly); err != nil {
		t.Fatalf("EnsureRoom: %v", err)
	}
	if len(rooms.created) != 1 {
		t.Fatalf("EnsureRoom issued %d create calls, want 1", len(rooms.created))
	}
	req := rooms.created[0]
	if req.Name != "room-ref-5" {
		t.Fatalf("room name = %q, want room-ref-5", req.Name)
	}
	if req.MaxParticipants != 50 {
		t.Fatalf("max participants = %d, want 50", req.MaxParticipants)
	}
	if req.EmptyTimeout != roomIdleTimeoutSeconds {
		t.Fatalf("empty timeout = %d, want %d (30-minute idle rule)", req.EmptyTimeout, roomIdleTimeoutSeconds)
	}
	// Idempotent: ensuring an already-ensured room succeeds again.
	if err := adapter.EnsureRoom(context.Background(), "room-ref-5", sessions.MediaModeAudioOnly); err != nil {
		t.Fatalf("idempotent EnsureRoom: %v", err)
	}
}

func TestEnsureRoomRejectsNonAudioModes(t *testing.T) {
	rooms := &fakeRoomClient{}
	adapter, _ := newTestAdapter(rooms)
	if err := adapter.EnsureRoom(context.Background(), "room-ref-6", sessions.MediaModeAudioVideo); err == nil {
		t.Fatal("video media mode must be rejected in F-005 (constitution §V)")
	}
	if len(rooms.created) != 0 {
		t.Fatal("no room may be created for a rejected media mode")
	}
}

func TestCloseRoomDeletesProviderRoom(t *testing.T) {
	rooms := &fakeRoomClient{}
	adapter, _ := newTestAdapter(rooms)
	if err := adapter.CloseRoom(context.Background(), "room-ref-7"); err != nil {
		t.Fatalf("CloseRoom: %v", err)
	}
	if len(rooms.deleted) != 1 || rooms.deleted[0].Room != "room-ref-7" {
		t.Fatalf("CloseRoom must delete the provider room, got %+v", rooms.deleted)
	}
}

// ---- T018: moderation surface ---------------------------------------------------

func audioTrack(sid string) *lkmodel.TrackInfo {
	return &lkmodel.TrackInfo{Sid: sid, Source: lkmodel.TrackSource_MICROPHONE, Type: lkmodel.TrackType_AUDIO}
}

func TestMuteParticipantMutesAudioTrack(t *testing.T) {
	rooms := &fakeRoomClient{partics: []*lkmodel.ParticipantInfo{
		{Identity: "student-1", Tracks: []*lkmodel.TrackInfo{audioTrack("tr-1")}},
	}}
	adapter, _ := newTestAdapter(rooms)

	if err := adapter.MuteParticipant(context.Background(), "room-ref-8", "student-1"); err != nil {
		t.Fatalf("MuteParticipant: %v", err)
	}
	if len(rooms.muted) != 1 {
		t.Fatalf("mute calls = %d, want 1", len(rooms.muted))
	}
	req := rooms.muted[0]
	if req.Room != "room-ref-8" || req.Identity != "student-1" || req.TrackSid != "tr-1" || !req.Muted {
		t.Fatalf("mute request = %+v, want room/identity/track muted", req)
	}
}

func TestMuteParticipantWithoutAudioTrackIsNoOp(t *testing.T) {
	rooms := &fakeRoomClient{partics: []*lkmodel.ParticipantInfo{
		{Identity: "student-1", Tracks: []*lkmodel.TrackInfo{{Sid: "tr-v", Source: lkmodel.TrackSource_CAMERA, Type: lkmodel.TrackType_VIDEO}}},
	}}
	adapter, _ := newTestAdapter(rooms)
	if err := adapter.MuteParticipant(context.Background(), "room-ref-9", "student-1"); err != nil {
		t.Fatalf("MuteParticipant without an audio track must be a no-op, got %v", err)
	}
	if len(rooms.muted) != 0 {
		t.Fatalf("no audio track should be muted, got %d calls", len(rooms.muted))
	}
}

func TestMuteAllMutesEveryAudioPublisher(t *testing.T) {
	rooms := &fakeRoomClient{partics: []*lkmodel.ParticipantInfo{
		{Identity: "student-1", Tracks: []*lkmodel.TrackInfo{audioTrack("tr-1")}},
		{Identity: "student-2", Tracks: []*lkmodel.TrackInfo{audioTrack("tr-2")}},
		{Identity: "student-3"},
	}}
	adapter, _ := newTestAdapter(rooms)

	if err := adapter.MuteAll(context.Background(), "room-ref-10"); err != nil {
		t.Fatalf("MuteAll: %v", err)
	}
	if len(rooms.muted) != 2 {
		t.Fatalf("mute calls = %d, want one per audio publisher (2)", len(rooms.muted))
	}
}

func TestRemoveParticipantDisconnectsByIdentity(t *testing.T) {
	rooms := &fakeRoomClient{}
	adapter, _ := newTestAdapter(rooms)
	if err := adapter.RemoveParticipant(context.Background(), "room-ref-11", "student-9"); err != nil {
		t.Fatalf("RemoveParticipant: %v", err)
	}
	if len(rooms.removed) != 1 || rooms.removed[0].Room != "room-ref-11" || rooms.removed[0].Identity != "student-9" {
		t.Fatalf("remove request = %+v", rooms.removed)
	}
}

// ---- interface compliance --------------------------------------------------------

func TestAdapterImplementsSessionMediaGateway(t *testing.T) {
	var _ sessions.SessionMediaGateway = (*Adapter)(nil)
}
