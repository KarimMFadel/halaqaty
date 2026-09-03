package livekit

import (
	"context"
	"errors"
	"testing"

	"github.com/KarimMFadel/halaqaty/backend/internal/sessions"
	lkmodel "github.com/livekit/protocol/livekit"
)

func TestUnmuteParticipant_UnmutesOnlyTargetMicrophoneTrack(t *testing.T) {
	rooms := &fakeRoomClient{partics: []*lkmodel.ParticipantInfo{
		{Identity: "student-1", Tracks: []*lkmodel.TrackInfo{audioTrack("tr-1")}},
		{Identity: "student-2", Tracks: []*lkmodel.TrackInfo{audioTrack("tr-2")}},
		{Identity: "student-3", Tracks: []*lkmodel.TrackInfo{audioTrack("tr-3")}},
	}}
	adapter, _ := newTestAdapter(rooms)

	if err := adapter.UnmuteParticipant(context.Background(), "room-ref-unmute", "student-2"); err != nil {
		t.Fatalf("UnmuteParticipant: %v", err)
	}
	if len(rooms.muted) != 1 {
		t.Fatalf("unmute calls = %d, want exactly one for the targeted participant", len(rooms.muted))
	}
	req := rooms.muted[0]
	if req.Room != "room-ref-unmute" || req.Identity != "student-2" || req.TrackSid != "tr-2" {
		t.Fatalf("unmute request = %+v, want room/identity/track of student-2", req)
	}
	if req.Muted {
		t.Fatal("unmute must request Muted=false without changing entitlements")
	}
}

func TestUnmuteParticipant_WithoutAudioTrack_IsNoOp(t *testing.T) {
	rooms := &fakeRoomClient{partics: []*lkmodel.ParticipantInfo{
		{Identity: "student-1"},
	}}
	adapter, _ := newTestAdapter(rooms)

	if err := adapter.UnmuteParticipant(context.Background(), "room-ref-unmute-2", "student-1"); err != nil {
		t.Fatalf("UnmuteParticipant without a published track must be a no-op, got %v", err)
	}
	if len(rooms.muted) != 0 {
		t.Fatalf("no track should be unmuted, got %d calls", len(rooms.muted))
	}
}

func TestRemoveParticipant_ProviderFailure_SurfacesError(t *testing.T) {
	rooms := &fakeRoomClient{removeErr: errors.New("participant disconnect failed")}
	adapter, _ := newTestAdapter(rooms)

	err := adapter.RemoveParticipant(context.Background(), sessions.MediaRoomRef("room-ref-rm-err"), "student-9")
	if err == nil {
		t.Fatal("RemoveParticipant must surface provider failures")
	}
	if !errors.Is(err, rooms.removeErr) {
		t.Fatalf("RemoveParticipant error = %v, want wrapped provider error", err)
	}
}
