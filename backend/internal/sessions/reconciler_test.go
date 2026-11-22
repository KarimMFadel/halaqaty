package sessions

import (
	"context"
	"errors"
	"fmt"
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

type recoveryAttendanceStub struct {
	finalized []string
	fail      map[string]bool
}

func (s *recoveryAttendanceStub) Finalize(_ context.Context, sessionID string) error {
	s.finalized = append(s.finalized, sessionID)
	if s.fail[sessionID] {
		return errors.New("finalize failed")
	}
	return nil
}

// pagedRecoveryStoreStub adds keyset pagination to the base store double so a
// sweep can walk past a first page whose candidates never complete.
type pagedRecoveryStoreStub struct {
	recoveryStoreStub
	all []Session
}

func (s *pagedRecoveryStoreStub) ListRecoveryCandidatesAfterID(_ context.Context, status SessionStatus, afterID string, limit int) ([]Session, error) {
	items := make([]Session, 0, limit)
	for _, sess := range s.all {
		if sess.Status != status || sess.ID <= afterID {
			continue
		}
		items = append(items, sess)
		if len(items) == limit {
			break
		}
	}
	return items, nil
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

func TestReconcilerSweepRetriesEndedAttendanceFinalization(t *testing.T) {
	store := &recoveryStoreStub{candidates: map[SessionStatus][]Session{
		SessionStatusEnded: {{ID: "ended-recovery", Status: SessionStatusEnded}},
	}}
	attendance := &recoveryAttendanceStub{}
	reconciler, err := NewReconciler(store, &recoveryGatewayStub{}, []byte("server-only-key"))
	if err != nil {
		t.Fatal(err)
	}
	reconciler.SetAttendanceFinalizer(attendance)
	if err := reconciler.Sweep(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(attendance.finalized) != 1 || attendance.finalized[0] != "ended-recovery" {
		t.Fatalf("finalized sessions = %v, want [ended-recovery]", attendance.finalized)
	}
}

func TestReconcilerSweepPaginatesPastUnfinishedFirstPage(t *testing.T) {
	const total = RecoveryCandidateLimit + 5
	all := make([]Session, 0, total)
	fail := make(map[string]bool, RecoveryCandidateLimit)
	for i := 0; i < total; i++ {
		id := fmt.Sprintf("ended-%03d", i)
		all = append(all, Session{ID: id, Status: SessionStatusEnded})
		if i < RecoveryCandidateLimit {
			// The first page never reaches its completion condition.
			fail[id] = true
		}
	}
	store := &pagedRecoveryStoreStub{
		recoveryStoreStub: recoveryStoreStub{candidates: map[SessionStatus][]Session{SessionStatusEnded: all}},
		all:               all,
	}
	attendance := &recoveryAttendanceStub{fail: fail}
	reconciler, err := NewReconciler(store, &recoveryGatewayStub{}, []byte("server-only-key"))
	if err != nil {
		t.Fatal(err)
	}
	reconciler.SetAttendanceFinalizer(attendance)

	if err := reconciler.Sweep(context.Background()); err == nil {
		t.Fatal("a first page that never completes must surface the failures")
	}
	if len(attendance.finalized) != total {
		t.Fatalf("finalized %d candidates, want all %d: later pages must be reached", len(attendance.finalized), total)
	}
	finalized := make(map[string]bool, total)
	for _, id := range attendance.finalized {
		finalized[id] = true
	}
	for i := RecoveryCandidateLimit; i < total; i++ {
		if id := fmt.Sprintf("ended-%03d", i); !finalized[id] {
			t.Fatalf("candidate %s beyond the first page was never processed: %v", id, attendance.finalized)
		}
	}
}

func TestReconcilerSweepContinuesBeyondPageCapAndWraps(t *testing.T) {
	const total = maxRecoveryPages*RecoveryCandidateLimit + 1
	store := &pagedRecoveryStoreStub{}
	for i := 0; i < total; i++ {
		store.all = append(store.all, Session{ID: fmt.Sprintf("ended-%05d", i), Status: SessionStatusEnded})
	}
	reconciler, err := NewReconciler(store, &recoveryGatewayStub{}, []byte("key"))
	if err != nil {
		t.Fatal(err)
	}
	if err := reconciler.Sweep(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := reconciler.Sweep(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(store.attempts) != total {
		t.Fatalf("processed %d candidates after two sweeps, want %d", len(store.attempts), total)
	}
	if err := reconciler.Sweep(context.Background()); err != nil {
		t.Fatal(err)
	}
	if store.attempts[total] != "ended-00000" {
		t.Fatalf("did not wrap: %s", store.attempts[total])
	}
}

type failingPageStore struct {
	pagedRecoveryStoreStub
	fail bool
}

func (s *failingPageStore) ListRecoveryCandidatesAfterID(ctx context.Context, status SessionStatus, afterID string, limit int) ([]Session, error) {
	if s.fail && status == SessionStatusEnded && afterID != "" {
		s.fail = false
		return nil, errors.New("page unavailable")
	}
	return s.pagedRecoveryStoreStub.ListRecoveryCandidatesAfterID(ctx, status, afterID, limit)
}

func TestReconcilerSweepRetriesPageAfterQueryError(t *testing.T) {
	store := &failingPageStore{fail: true}
	for i := 0; i <= RecoveryCandidateLimit; i++ {
		store.all = append(store.all, Session{ID: fmt.Sprintf("ended-%03d", i), Status: SessionStatusEnded})
	}
	r, err := NewReconciler(store, &recoveryGatewayStub{}, []byte("key"))
	if err != nil {
		t.Fatal(err)
	}
	if err := r.Sweep(context.Background()); err == nil {
		t.Fatal("expected page error")
	}
	if err := r.Sweep(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(store.attempts) != RecoveryCandidateLimit+1 {
		t.Fatalf("processed %d candidates, want %d", len(store.attempts), RecoveryCandidateLimit+1)
	}
}

func TestReconcilerSweepCanceledContextDoesNoWork(t *testing.T) {
	store := &pagedRecoveryStoreStub{all: []Session{{ID: "ended-001", Status: SessionStatusEnded}}}
	r, err := NewReconciler(store, &recoveryGatewayStub{}, []byte("key"))
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if !errors.Is(r.Sweep(ctx), context.Canceled) {
		t.Fatal("expected cancellation")
	}
	if len(store.attempts) != 0 {
		t.Fatalf("processed canceled sweep: %v", store.attempts)
	}
}
