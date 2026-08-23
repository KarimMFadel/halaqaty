package livekit

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/KarimMFadel/halaqaty/backend/internal/platform/config"
	"github.com/KarimMFadel/halaqaty/backend/internal/sessions"
	lkmodel "github.com/livekit/protocol/livekit"
	"github.com/livekit/psrpc"
)

// ---- T016 test doubles -------------------------------------------------------

// reciterTestKey is a 32-byte stand-in for the backend-only HMAC room key.
var reciterTestKey = []byte("unit-test-room-key-0123456789abcdef")

// reciterRoomClient extends the shared fake with the participant-permission
// seam the reciter audio control needs.
type reciterRoomClient struct {
	*fakeRoomClient
	updated    []*lkmodel.UpdateParticipantRequest
	updateErr  error
	updateWait chan struct{} // when non-nil, blocks until closed
}

func (f *reciterRoomClient) UpdateParticipant(ctx context.Context, req *lkmodel.UpdateParticipantRequest) (*lkmodel.ParticipantInfo, error) {
	f.updated = append(f.updated, req)
	if f.updateWait != nil {
		// A real network client returns when its context expires; model that.
		select {
		case <-f.updateWait:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	if f.updateErr != nil {
		return nil, f.updateErr
	}
	return &lkmodel.ParticipantInfo{Identity: req.Identity}, nil
}

func newReciterAdapter(rooms *reciterRoomClient) (*Adapter, error) {
	cfg := config.LiveKitConfig{
		Endpoint:  "wss://media.example.com",
		APIKey:    "test-key",
		APISecret: "test-secret-value-not-a-real-credential",
	}
	return NewAdapterWithRoomKey(cfg, config.DefaultAudioPolicy(), rooms, reciterTestKey)
}

const reciterTestSessionID = "123e4567-e89b-12d3-a456-426614174000"

// wantReciterRoomRef is the deterministic room the session service itself
// would derive for the same session and key.
func wantReciterRoomRef(t *testing.T) sessions.MediaRoomRef {
	t.Helper()
	ref, err := sessions.StableMediaRoomRef(reciterTestSessionID, reciterTestKey)
	if err != nil {
		t.Fatalf("derive expected room reference: %v", err)
	}
	return ref
}

func TestAdapterImplementsReciterAudioControl(t *testing.T) {
	var _ sessions.ReciterAudioControl = (*Adapter)(nil)
}

// ---- T016: deterministic room derivation ---------------------------------------

func TestReciterAudioControlDerivesRoomFromSessionID(t *testing.T) {
	rooms := &reciterRoomClient{fakeRoomClient: &fakeRoomClient{}}
	adapter, err := newReciterAdapter(rooms)
	if err != nil {
		t.Fatalf("NewAdapterWithRoomKey: %v", err)
	}
	if err := adapter.GrantReciterAudio(context.Background(), reciterTestSessionID, "round-1", "entry-1", "student-1"); err != nil {
		t.Fatalf("GrantReciterAudio: %v", err)
	}
	if err := adapter.RevokeReciterAudio(context.Background(), reciterTestSessionID, "round-1", "entry-1", "student-1"); err != nil {
		t.Fatalf("RevokeReciterAudio: %v", err)
	}
	want := string(wantReciterRoomRef(t))
	for i, req := range rooms.updated {
		if req.Room != want {
			t.Fatalf("update %d room = %q, want the session-service derivation %q", i, req.Room, want)
		}
		if req.Identity != "student-1" {
			t.Fatalf("update %d identity = %q, want student-1", i, req.Identity)
		}
	}
}

// ---- T016: audio-only entitlement ----------------------------------------------

func TestGrantReciterAudioIsAudioOnlyPublish(t *testing.T) {
	rooms := &reciterRoomClient{fakeRoomClient: &fakeRoomClient{}}
	adapter, err := newReciterAdapter(rooms)
	if err != nil {
		t.Fatalf("NewAdapterWithRoomKey: %v", err)
	}
	if err := adapter.GrantReciterAudio(context.Background(), reciterTestSessionID, "round-1", "entry-1", "student-1"); err != nil {
		t.Fatalf("GrantReciterAudio: %v", err)
	}
	perm := rooms.updated[0].Permission
	if perm == nil {
		t.Fatal("grant must carry an explicit participant permission")
	}
	if !perm.CanPublish {
		t.Fatal("reciter grant must allow publishing")
	}
	if !perm.CanSubscribe {
		t.Fatal("reciter must keep subscribing (listening) rights")
	}
	if perm.CanPublishData {
		t.Fatal("reciter grant must not allow data publishing")
	}
	if len(perm.CanPublishSources) != 1 || perm.CanPublishSources[0] != lkmodel.TrackSource_MICROPHONE {
		t.Fatalf("reciter grant sources = %v, want microphone only (video never publishable)", perm.CanPublishSources)
	}
}

func TestRevokeReciterAudioRemovesPublishKeepsSubscribe(t *testing.T) {
	rooms := &reciterRoomClient{fakeRoomClient: &fakeRoomClient{}}
	adapter, err := newReciterAdapter(rooms)
	if err != nil {
		t.Fatalf("NewAdapterWithRoomKey: %v", err)
	}
	if err := adapter.RevokeReciterAudio(context.Background(), reciterTestSessionID, "round-1", "entry-1", "student-1"); err != nil {
		t.Fatalf("RevokeReciterAudio: %v", err)
	}
	perm := rooms.updated[0].Permission
	if perm == nil {
		t.Fatal("revoke must carry an explicit participant permission")
	}
	if perm.CanPublish {
		t.Fatal("revoke must remove publishing")
	}
	if !perm.CanSubscribe {
		t.Fatal("revoked reciter must keep listening rights")
	}
}

// ---- T016: idempotency ----------------------------------------------------------

func TestGrantReciterAudioTwiceIsOneEffectiveGrant(t *testing.T) {
	rooms := &reciterRoomClient{fakeRoomClient: &fakeRoomClient{}}
	adapter, err := newReciterAdapter(rooms)
	if err != nil {
		t.Fatalf("NewAdapterWithRoomKey: %v", err)
	}
	if err := adapter.GrantReciterAudio(context.Background(), reciterTestSessionID, "round-1", "entry-1", "student-1"); err != nil {
		t.Fatalf("first grant: %v", err)
	}
	if err := adapter.GrantReciterAudio(context.Background(), reciterTestSessionID, "round-1", "entry-1", "student-1"); err != nil {
		t.Fatalf("second grant: %v", err)
	}
	if len(rooms.updated) != 2 {
		t.Fatalf("permission updates = %d, want 2", len(rooms.updated))
	}
	first, second := rooms.updated[0].Permission, rooms.updated[1].Permission
	if fmt.Sprint(first.CanPublishSources) != fmt.Sprint(second.CanPublishSources) ||
		first.CanPublish != second.CanPublish || first.CanSubscribe != second.CanSubscribe {
		t.Fatal("repeated grants must converge to one identical effective permission")
	}
}

func TestRevokeReciterAudioOfNonGrantedUserSucceeds(t *testing.T) {
	rooms := &reciterRoomClient{
		fakeRoomClient: &fakeRoomClient{},
		updateErr:      psrpc.NewErrorf(psrpc.NotFound, "participant not found"),
	}
	adapter, err := newReciterAdapter(rooms)
	if err != nil {
		t.Fatalf("NewAdapterWithRoomKey: %v", err)
	}
	if err := adapter.RevokeReciterAudio(context.Background(), reciterTestSessionID, "round-1", "entry-1", "student-1"); err != nil {
		t.Fatalf("revoking a non-granted user must succeed, got %v", err)
	}
}

// ---- T016: bounded provider calls -----------------------------------------------

func TestGrantReciterAudioTimesOutBoundedProviderCall(t *testing.T) {
	rooms := &reciterRoomClient{
		fakeRoomClient: &fakeRoomClient{},
		updateWait:     make(chan struct{}),
	}
	adapter, err := newReciterAdapter(rooms)
	if err != nil {
		t.Fatalf("NewAdapterWithRoomKey: %v", err)
	}
	adapter.reciterTimeout = 40 * time.Millisecond
	defer close(rooms.updateWait)

	start := time.Now()
	err = adapter.GrantReciterAudio(context.Background(), reciterTestSessionID, "round-1", "entry-1", "student-1")
	elapsed := time.Since(start)
	if err == nil {
		t.Fatal("hanging provider call must fail, not block")
	}
	if elapsed >= 500*time.Millisecond {
		t.Fatalf("grant blocked for %v; the per-call bound (shortened here) must bound it", elapsed)
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("hanging provider call must surface a deadline error, got %v", err)
	}
}

func TestRevokeReciterAudioTimesOutBoundedProviderCall(t *testing.T) {
	rooms := &reciterRoomClient{
		fakeRoomClient: &fakeRoomClient{},
		updateWait:     make(chan struct{}),
	}
	adapter, err := newReciterAdapter(rooms)
	if err != nil {
		t.Fatalf("NewAdapterWithRoomKey: %v", err)
	}
	adapter.reciterTimeout = 40 * time.Millisecond
	defer close(rooms.updateWait)

	start := time.Now()
	err = adapter.RevokeReciterAudio(context.Background(), reciterTestSessionID, "round-1", "entry-1", "student-1")
	if err == nil || time.Since(start) >= 500*time.Millisecond {
		t.Fatalf("revoke must fail fast on a hanging provider call, err=%v elapsed=%v", err, time.Since(start))
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("hanging provider call must surface a deadline error, got %v", err)
	}
}

// ---- T016: secret-safe errors ----------------------------------------------------

func TestReciterAudioControlErrorsAreSecretSafe(t *testing.T) {
	wantRoom := string(wantReciterRoomRef(t))
	rooms := &reciterRoomClient{
		fakeRoomClient: &fakeRoomClient{},
		updateErr: fmt.Errorf("livekit server at wss://media.example.com rejected api key test-key "+
			"secret test-secret-value-not-a-real-credential for room %s identity student-1", wantRoom),
	}
	adapter, err := newReciterAdapter(rooms)
	if err != nil {
		t.Fatalf("NewAdapterWithRoomKey: %v", err)
	}

	grantErr := adapter.GrantReciterAudio(context.Background(), reciterTestSessionID, "round-1", "entry-1", "student-1")
	if grantErr == nil {
		t.Fatal("failing provider call must surface an error")
	}
	revokeErr := adapter.RevokeReciterAudio(context.Background(), reciterTestSessionID, "round-1", "entry-1", "student-1")
	if revokeErr == nil {
		t.Fatal("failing provider call must surface an error even on revoke when the failure is not not-found")
	}
	for name, err := range map[string]error{"grant": grantErr, "revoke": revokeErr} {
		msg := err.Error()
		for _, secret := range []string{
			"wss://media.example.com", "test-key", "test-secret-value-not-a-real-credential",
			wantRoom, "livekit", "LiveKit", "hlq-",
		} {
			if strings.Contains(msg, secret) {
				t.Fatalf("%s error leaks provider material %q: %s", name, secret, msg)
			}
		}
	}
}
