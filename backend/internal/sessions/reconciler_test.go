package sessions

import (
	"context"
	"errors"
	"strings"
	"testing"
)

type recoveryStoreStub struct {
	candidates map[SessionStatus][]Session
	attempts   []string
}

func (s *recoveryStoreStub) ListRecoveryCandidates(_ context.Context, status SessionStatus, limit int) ([]Session, error) {
	items := s.candidates[status]
	if len(items) > limit {
		items = items[:limit]
	}
	return items, nil
}

func (s *recoveryStoreStub) TrySessionLock(ctx context.Context, id string, fn func(context.Context) error) (bool, error) {
	s.attempts = append(s.attempts, id)
	return true, fn(ctx)
}

type recoveryGatewayStub struct {
	ensured []MediaRoomRef
	closed  []MediaRoomRef
}

func (g *recoveryGatewayStub) EnsureRoom(_ context.Context, ref MediaRoomRef, _ MediaMode) error {
	g.ensured = append(g.ensured, ref)
	return nil
}
func (g *recoveryGatewayStub) CloseRoom(_ context.Context, ref MediaRoomRef) error {
	g.closed = append(g.closed, ref)
	return nil
}
func (g *recoveryGatewayStub) IssueConnection(context.Context, MediaRoomRef, string, MediaGrants) (MediaConnection, error) {
	return MediaConnection{}, errors.New("unused")
}
func (g *recoveryGatewayStub) MuteParticipant(context.Context, MediaRoomRef, string) error {
	return nil
}
func (g *recoveryGatewayStub) UnmuteParticipant(context.Context, MediaRoomRef, string) error {
	return nil
}
func (g *recoveryGatewayStub) MuteAll(context.Context, MediaRoomRef) error { return nil }
func (g *recoveryGatewayStub) RemoveParticipant(context.Context, MediaRoomRef, string) error {
	return nil
}

func TestStableMediaRoomRefIsDeterministicOpaqueAndKeyed(t *testing.T) {
	first, err := StableMediaRoomRef("123e4567-e89b-12d3-a456-426614174000", []byte("server-only-key"))
	if err != nil {
		t.Fatal(err)
	}
	second, err := StableMediaRoomRef("123e4567-e89b-12d3-a456-426614174000", []byte("server-only-key"))
	if err != nil || first != second {
		t.Fatalf("stable refs differ: %q %q (%v)", first, second, err)
	}
	if strings.Contains(string(first), "123e4567-e89b-12d3-a456-426614174000") {
		t.Fatalf("room ref must not contain session ID: %q", first)
	}
	other, err := StableMediaRoomRef("123e4567-e89b-12d3-a456-426614174001", []byte("server-only-key"))
	if err != nil || first == other {
		t.Fatalf("different sessions must have different refs: %q %q", first, other)
	}
}

func TestReconcilerSweepProcessesBoundedLifecycleCandidates(t *testing.T) {
	store := &recoveryStoreStub{candidates: map[SessionStatus][]Session{
		SessionStatusScheduled: {{ID: "scheduled-1", Status: SessionStatusScheduled}},
		SessionStatusActive:    {{ID: "active-1", Status: SessionStatusActive, MediaRoomRef: "persisted-active", MediaMode: MediaModeAudioOnly}},
		SessionStatusEnded:     {{ID: "ended-1", Status: SessionStatusEnded, MediaRoomRef: "persisted-ended"}},
	}}
	gateway := &recoveryGatewayStub{}
	reconciler, err := NewReconciler(store, gateway, []byte("server-only-key"))
	if err != nil {
		t.Fatal(err)
	}
	if err := reconciler.Sweep(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(store.attempts) != 3 {
		t.Fatalf("locked candidates = %d, want 3", len(store.attempts))
	}
	if len(gateway.ensured) != 1 || gateway.ensured[0] != "persisted-active" {
		t.Fatalf("ensured rooms = %v", gateway.ensured)
	}
	if len(gateway.closed) != 2 || gateway.closed[1] != "persisted-ended" {
		t.Fatalf("closed rooms = %v", gateway.closed)
	}
}
