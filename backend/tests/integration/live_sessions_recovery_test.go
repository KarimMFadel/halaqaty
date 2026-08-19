//go:build integration

package integration

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/KarimMFadel/halaqaty/backend/internal/sessions"
)

// recoveryIntegrationGateway models the provider boundary while keeping the
// integration tests focused on durable PostgreSQL lifecycle behavior.
type recoveryIntegrationGateway struct {
	mu          sync.Mutex
	ensureCalls []sessions.MediaRoomRef
	closeCalls  []sessions.MediaRoomRef
	issueCalls  int
	closeFails  int
	missing     bool
}

func (g *recoveryIntegrationGateway) EnsureRoom(_ context.Context, ref sessions.MediaRoomRef, _ sessions.MediaMode) error {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.ensureCalls = append(g.ensureCalls, ref)
	return nil
}

func (g *recoveryIntegrationGateway) CloseRoom(_ context.Context, ref sessions.MediaRoomRef) error {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.closeCalls = append(g.closeCalls, ref)
	if g.missing {
		// The provider adapter normalizes a missing room to idempotent success.
		return nil
	}
	if g.closeFails > 0 {
		g.closeFails--
		return errors.New("provider close failed")
	}
	return nil
}

func (g *recoveryIntegrationGateway) IssueConnection(_ context.Context, _ sessions.MediaRoomRef, userID string, _ sessions.MediaGrants) (sessions.MediaConnection, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.issueCalls++
	return sessions.MediaConnection{
		Endpoint:   "wss://media.test.halaqaty.app/room",
		Credential: sessions.MediaCredential(fmt.Sprintf("recovery-%s-%d", userID, g.issueCalls)),
		ExpiresAt:  time.Now().Add(time.Hour),
	}, nil
}

func (g *recoveryIntegrationGateway) MuteParticipant(context.Context, sessions.MediaRoomRef, string) error {
	return nil
}
func (g *recoveryIntegrationGateway) UnmuteParticipant(context.Context, sessions.MediaRoomRef, string) error {
	return nil
}
func (g *recoveryIntegrationGateway) MuteAll(context.Context, sessions.MediaRoomRef) error {
	return nil
}
func (g *recoveryIntegrationGateway) RemoveParticipant(context.Context, sessions.MediaRoomRef, string) error {
	return nil
}

func recoveryService(t *testing.T, env *sessionIntegEnv, gateway *recoveryIntegrationGateway) *sessions.Service {
	t.Helper()
	service, err := sessions.NewServiceWithRoomKey(
		env.repo,
		gateway,
		&integRoles{pool: env.pool},
		[]byte("integration-only-recovery-room-key"),
	)
	if err != nil {
		t.Fatalf("new recovery service: %v", err)
	}
	return service
}

func recoveryReconciler(t *testing.T, env *sessionIntegEnv, gateway *recoveryIntegrationGateway) *sessions.Reconciler {
	t.Helper()
	reconciler, err := sessions.NewReconciler(env.repo, gateway, []byte("integration-only-recovery-room-key"))
	if err != nil {
		t.Fatalf("new reconciler: %v", err)
	}
	return reconciler
}

func TestRecoveryIntegration_CreateCrashWindowClosesStableOrphan(t *testing.T) {
	env := setupSessionIntegEnv(t)
	ctx := context.Background()
	gateway := &recoveryIntegrationGateway{}
	reconciler := recoveryReconciler(t, env, gateway)

	created, err := env.repo.CreateAdHocSession(ctx, env.circleID, env.userIDs["teacher"])
	if err != nil {
		t.Fatalf("create scheduled session: %v", err)
	}
	want, err := sessions.StableMediaRoomRef(created.ID, []byte("integration-only-recovery-room-key"))
	if err != nil {
		t.Fatalf("derive stable room ref: %v", err)
	}

	// Simulate provider room creation succeeding before the process crashes:
	// durable state is still scheduled and has no persisted room reference.
	if err := gateway.EnsureRoom(ctx, want, sessions.MediaModeAudioOnly); err != nil {
		t.Fatalf("simulate provider create: %v", err)
	}
	if err := reconciler.Sweep(ctx); err != nil {
		t.Fatalf("reconcile orphan room: %v", err)
	}
	if len(gateway.closeCalls) != 1 || gateway.closeCalls[0] != want {
		t.Fatalf("closed rooms = %v, want stable orphan %q", gateway.closeCalls, want)
	}
	current, err := env.repo.GetSession(ctx, created.ID)
	if err != nil {
		t.Fatalf("reload scheduled session: %v", err)
	}
	if current.Status != sessions.SessionStatusScheduled || current.MediaRoomRef != "" {
		t.Fatalf("reconciliation changed durable scheduled state: %+v", current)
	}
}

func TestRecoveryIntegration_CloseCrashWindowRetriesBounded(t *testing.T) {
	env := setupSessionIntegEnv(t)
	ctx := context.Background()
	gateway := &recoveryIntegrationGateway{closeFails: 2}
	service := recoveryService(t, env, gateway)
	reconciler := recoveryReconciler(t, env, gateway)

	created, err := service.CreateAdHocSession(ctx, env.userIDs["teacher"], env.circleID)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	started, _, err := service.StartSession(ctx, env.userIDs["teacher"], created.ID)
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	ended, err := service.EndSession(ctx, env.userIDs["teacher"], started.ID, sessions.EndReasonManual)
	if err != nil {
		t.Fatalf("end must remain successful after provider close failure: %v", err)
	}
	if ended.Status != sessions.SessionStatusEnded {
		t.Fatalf("end status = %q, want ended", ended.Status)
	}
	if err := reconciler.Sweep(ctx); err == nil {
		t.Fatal("first bounded provider close attempt should report the transient failure")
	}
	if err := reconciler.Sweep(ctx); err != nil {
		t.Fatalf("second sweep should retry and converge: %v", err)
	}
	if len(gateway.closeCalls) != 3 {
		t.Fatalf("close attempts = %d, want foreground close plus one attempt per sweep", len(gateway.closeCalls))
	}
	current, err := env.repo.GetSession(ctx, started.ID)
	if err != nil {
		t.Fatalf("reload ended session: %v", err)
	}
	if current.Status != sessions.SessionStatusEnded {
		t.Fatalf("durable state after cleanup = %q, want ended", current.Status)
	}
}

func TestRecoveryIntegration_MissingProviderRoomIsIdempotent(t *testing.T) {
	env := setupSessionIntegEnv(t)
	ctx := context.Background()
	gateway := &recoveryIntegrationGateway{missing: true}
	service := recoveryService(t, env, gateway)
	reconciler := recoveryReconciler(t, env, gateway)

	created, err := service.CreateAdHocSession(ctx, env.userIDs["teacher"], env.circleID)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	started, _, err := service.StartSession(ctx, env.userIDs["teacher"], created.ID)
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	if _, err := service.EndSession(ctx, env.userIDs["teacher"], started.ID, sessions.EndReasonManual); err != nil {
		t.Fatalf("end: %v", err)
	}
	if err := reconciler.Sweep(ctx); err != nil {
		t.Fatalf("missing provider room must be idempotent: %v", err)
	}
	if err := reconciler.Sweep(ctx); err != nil {
		t.Fatalf("repeated missing-room cleanup must remain idempotent: %v", err)
	}
}

func TestRecoveryIntegration_AdvisoryLockSkipsBusyCandidate(t *testing.T) {
	env := setupSessionIntegEnv(t)
	ctx := context.Background()
	created, err := env.repo.CreateAdHocSession(ctx, env.circleID, env.userIDs["teacher"])
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	conn, err := env.pool.Acquire(ctx)
	if err != nil {
		t.Fatalf("acquire lock connection: %v", err)
	}
	defer conn.Release()
	tx, err := conn.Begin(ctx)
	if err != nil {
		t.Fatalf("begin lock transaction: %v", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1, 0))`, created.ID); err != nil {
		t.Fatalf("hold session advisory lock: %v", err)
	}

	called := false
	locked, err := env.repo.TrySessionLock(ctx, created.ID, func(context.Context) error {
		called = true
		return nil
	})
	if err != nil {
		t.Fatalf("try session lock: %v", err)
	}
	if locked || called {
		t.Fatalf("busy candidate must be skipped: locked=%v callback=%v", locked, called)
	}
}

func TestRecoveryIntegration_ReconnectRestoresEligiblePresenceUnderLock(t *testing.T) {
	env := setupSessionIntegEnv(t)
	ctx := context.Background()
	gateway := &recoveryIntegrationGateway{}
	service := recoveryService(t, env, gateway)

	created, err := service.CreateAdHocSession(ctx, env.userIDs["teacher"], env.circleID)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	started, _, err := service.StartSession(ctx, env.userIDs["teacher"], created.ID)
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	if _, _, err := service.JoinSession(ctx, env.userIDs["student"], started.ID); err != nil {
		t.Fatalf("initial join: %v", err)
	}
	if _, err := env.repo.LeaveSession(ctx, started.ID, env.userIDs["student"]); err != nil {
		t.Fatalf("leave before reconnect: %v", err)
	}
	if _, err := env.repo.SetLock(ctx, started.ID, true); err != nil {
		t.Fatalf("lock room: %v", err)
	}

	reconnected, err := env.repo.ReconnectPresence(ctx, started.ID, env.userIDs["student"])
	if err != nil {
		t.Fatalf("eligible pre-lock reconnect: %v", err)
	}
	if reconnected.ParticipantCount != 2 {
		t.Fatalf("participant count after reconnect = %d, want 2", reconnected.ParticipantCount)
	}
	var present bool
	if err := env.pool.QueryRow(ctx, `SELECT is_currently_present FROM session_participant_presence WHERE session_id = $1::uuid AND user_id = $2::uuid`, started.ID, env.userIDs["student"]).Scan(&present); err != nil {
		t.Fatalf("read reconnected presence: %v", err)
	}
	if !present {
		t.Fatal("eligible participant must be currently present after reconnect")
	}
}
