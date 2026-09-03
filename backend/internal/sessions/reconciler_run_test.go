package sessions

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

// runStoreStub is a concurrency-safe RecoveryStore double; locked=false models
// a session whose advisory lock is already held (busy skip).
type runStoreStub struct {
	mu         sync.Mutex
	candidates map[SessionStatus][]Session
	attempts   int
	locked     bool
}

func (s *runStoreStub) ListRecoveryCandidates(_ context.Context, status SessionStatus, limit int) ([]Session, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	items := s.candidates[status]
	if len(items) > limit {
		items = items[:limit]
	}
	return items, nil
}

func (s *runStoreStub) TrySessionLock(ctx context.Context, _ string, fn func(context.Context) error) (bool, error) {
	s.mu.Lock()
	s.attempts++
	locked := s.locked
	s.mu.Unlock()
	if !locked {
		return false, nil
	}
	return true, fn(ctx)
}

func TestNewReconciler_MissingDependencies_Rejected(t *testing.T) {
	store := &runStoreStub{}
	gateway := &recoveryGatewayStub{}
	roomKey := []byte("server-only-key")
	cases := []struct {
		name    string
		store   RecoveryStore
		gateway SessionMediaGateway
		roomKey []byte
	}{
		{name: "nil store", store: nil, gateway: gateway, roomKey: roomKey},
		{name: "nil gateway", store: store, gateway: nil, roomKey: roomKey},
		{name: "empty room key", store: store, gateway: gateway, roomKey: nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := NewReconciler(tc.store, tc.gateway, tc.roomKey); err == nil {
				t.Fatalf("NewReconciler must reject %s", tc.name)
			}
		})
	}
	if _, err := NewReconciler(store, gateway, roomKey); err != nil {
		t.Fatalf("NewReconciler with valid arguments: %v", err)
	}
}

func TestReconciler_Run_PerformsStartupSweepAndStopsOnCancel(t *testing.T) {
	store := &runStoreStub{locked: true, candidates: map[SessionStatus][]Session{
		SessionStatusEnded: {{ID: "ended-run", Status: SessionStatusEnded, MediaRoomRef: "room-run"}},
	}}
	gateway := &recoveryGatewayStub{}
	reconciler, err := NewReconciler(store, gateway, []byte("server-only-key"))
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	errCh := make(chan error, 1)
	go func() { errCh <- reconciler.Run(ctx) }()

	// The startup sweep must run before the first tick.
	deadline := time.Now().Add(2 * time.Second)
	for {
		store.mu.Lock()
		attempts := store.attempts
		store.mu.Unlock()
		if attempts >= 1 {
			break
		}
		if time.Now().After(deadline) {
			cancel()
			t.Fatal("startup sweep never ran")
		}
		time.Sleep(time.Millisecond)
	}

	cancel()
	if err := <-errCh; !errors.Is(err, context.Canceled) {
		t.Fatalf("Run error = %v, want context.Canceled", err)
	}
	if len(gateway.closed) != 1 || gateway.closed[0] != "room-run" {
		t.Fatalf("startup sweep provider cleanup = %v, want [room-run]", gateway.closed)
	}
}

func TestReconciler_Sweep_BusySession_IsSkippedWithoutProviderCalls(t *testing.T) {
	store := &runStoreStub{locked: false, candidates: map[SessionStatus][]Session{
		SessionStatusEnded: {{ID: "busy-1", Status: SessionStatusEnded, MediaRoomRef: "room-busy"}},
	}}
	gateway := &recoveryGatewayStub{}
	reconciler, err := NewReconciler(store, gateway, []byte("server-only-key"))
	if err != nil {
		t.Fatal(err)
	}

	if err := reconciler.Sweep(context.Background()); err != nil {
		t.Fatalf("a busy session is a skip, not an error: %v", err)
	}
	if len(gateway.closed) != 0 {
		t.Fatalf("busy session must not receive provider calls, got %v", gateway.closed)
	}
}

func TestReconciler_Sweep_CandidateError_ContinuesRemainingCandidates(t *testing.T) {
	store := &runStoreStub{locked: true, candidates: map[SessionStatus][]Session{
		SessionStatusActive: {
			{ID: "active-no-room", Status: SessionStatusActive, MediaRoomRef: ""},
		},
		SessionStatusEnded: {
			{ID: "ended-room", Status: SessionStatusEnded, MediaRoomRef: "persisted"},
			{ID: "ended-no-room", Status: SessionStatusEnded, MediaRoomRef: ""},
		},
	}}
	gateway := &recoveryGatewayStub{}
	reconciler, err := NewReconciler(store, gateway, []byte("server-only-key"))
	if err != nil {
		t.Fatal(err)
	}

	err = reconciler.Sweep(context.Background())
	if err == nil {
		t.Fatal("Sweep must report the failing candidate")
	}
	if len(gateway.closed) != 1 || gateway.closed[0] != "persisted" {
		t.Fatalf("Sweep must keep processing after an error, closed = %v", gateway.closed)
	}
	store.mu.Lock()
	attempts := store.attempts
	store.mu.Unlock()
	if attempts != 3 {
		t.Fatalf("attempted candidates = %d, want all 3", attempts)
	}
}
