//go:build integration

package integration

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/KarimMFadel/halaqaty/backend/internal/queue"
	"github.com/KarimMFadel/halaqaty/backend/internal/sessions"
)

// T058 — ending a session finalizes the active queue round synchronously,
// bounded by the queue observer timeout (≤10s convergence target).

func TestSessionEnd_FinalizesActiveQueueRound(t *testing.T) {
	f := newSessionQueueConvergenceFixture(t)
	ctx := context.Background()

	var roundID string
	var roundVersion int64
	if err := f.pool.QueryRow(ctx, `
		SELECT id::text, version FROM recitation_queue
		WHERE session_id = $1::uuid AND lifecycle = 'active'
	`, f.session).Scan(&roundID, &roundVersion); err != nil {
		t.Fatalf("find active round: %v", err)
	}

	ended, err := f.sessions.EndSession(ctx, f.teacher, f.session, sessions.EndReasonManual)
	if err != nil {
		t.Fatalf("end session: %v", err)
	}
	if ended.Status != sessions.SessionStatusEnded {
		t.Fatalf("session status = %q, want ended", ended.Status)
	}
	if f.observer.endErr != nil {
		t.Fatalf("queue observer OnSessionEnded failed: %v", f.observer.endErr)
	}
	if len(f.observer.ended) != 1 || f.observer.ended[0] != f.session {
		t.Fatalf("observer ended = %v, want [%s]", f.observer.ended, f.session)
	}

	// Convergence runs in a background goroutine; poll until it finishes.
	var lifecycle string
	for start := time.Now(); time.Since(start) < 5*time.Second; {
		if err := f.pool.QueryRow(ctx, `
			SELECT lifecycle FROM recitation_queue WHERE id = $1::uuid
		`, roundID).Scan(&lifecycle); err != nil {
			t.Fatalf("load round after end: %v", err)
		}
		if lifecycle == "finalized" {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if lifecycle != "finalized" {
		t.Fatalf("round lifecycle = %q, want finalized", lifecycle)
	}

	// Contract regression (getSessionQueue): after session end the snapshot
	// resolves to the latest finalized round instead of 404, so late readers
	// still receive the read-only terminal surface.
	current, err := f.repo.CurrentRound(ctx, f.session)
	if err != nil {
		t.Fatalf("current round after end: %v", err)
	}
	if current.ID != roundID || current.Lifecycle != queue.RoundLifecycleFinalized {
		t.Fatalf("current round = %s/%s, want %s/finalized", current.ID, current.Lifecycle, roundID)
	}
}

type sessionQueueConvergenceFixture struct {
	pool     *pgxpool.Pool
	sessions *sessions.Service
	repo     *queue.Repository
	observer *capturingQueueObserver
	teacher  string
	students []string
	session  string
}

type capturingQueueObserver struct {
	next   sessions.QueueObserver
	ended  []string
	endErr error
}

func (c *capturingQueueObserver) OnSessionStarted(ctx context.Context, sessionID string) error {
	return c.next.OnSessionStarted(ctx, sessionID)
}

func (c *capturingQueueObserver) OnParticipantJoined(ctx context.Context, sessionID, userID string) error {
	return c.next.OnParticipantJoined(ctx, sessionID, userID)
}

func (c *capturingQueueObserver) OnSessionEnded(ctx context.Context, sessionID string) error {
	c.ended = append(c.ended, sessionID)
	c.endErr = c.next.OnSessionEnded(ctx, sessionID)
	return c.endErr
}

func newSessionQueueConvergenceFixture(t *testing.T) *sessionQueueConvergenceFixture {
	t.Helper()
	ctx := context.Background()
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		t.Skip("DATABASE_URL is not set; T058 requires PostgreSQL")
	}
	schema := fmt.Sprintf("test_session_conv_%d", time.Now().UnixNano())
	pool := openSchemaPool(t, ctx, schema)
	t.Cleanup(func() {
		cleanupCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
		defer cancel()
		_, _ = pool.Exec(cleanupCtx, fmt.Sprintf("DROP SCHEMA IF EXISTS %s CASCADE", schema))
		pool.Close()
	})
	conn := acquireConn(t, pool, ctx)
	createSchema(t, conn, ctx, schema)
	for _, migration := range recitationQueueHeadMigrations {
		runMigrationFile(t, conn, ctx, migration)
	}
	conn.Release()

	teacher := seedCompletionUser(t, pool, "teacher")
	students := []string{seedCompletionUser(t, pool, "student-a")}
	circleID := seedCompletionCircle(t, pool, teacher)
	seedCompletionMember(t, pool, circleID, teacher, "teacher", time.Now().UTC())
	seedCompletionMember(t, pool, circleID, students[0], "student", time.Now().UTC().Add(time.Second))

	var sessionID string
	if err := pool.QueryRow(ctx, `
		INSERT INTO sessions (circle_id, created_by, queue_grade_visibility)
		VALUES ($1::uuid, $2::uuid, 'managers_only')
		RETURNING id::text
	`, circleID, teacher).Scan(&sessionID); err != nil {
		t.Fatalf("insert session: %v", err)
	}

	// Start the session through the production repository so the queue observer
	// invariant applies, then create and activate a round directly via queue repo.
	repo := sessions.NewSessionRepository(pool)
	if _, err := repo.StartSession(ctx, sessionID, sessions.MediaRoomRef("conv-room-"+sessionID)); err != nil {
		t.Fatalf("start session: %v", err)
	}

	queueRepo := queue.NewQueueRepository(pool)
	roundSvc := queue.NewRoundService(queueRepo)
	round, err := roundSvc.Prepare(ctx, queue.PrepareRoundInput{
		SessionID: sessionID, Type: queue.RoundTypeRevision, SurahID: 1, FromAyah: 1, ToAyah: 7,
		SurahAyahCount: 7, GradingRequired: true, CreatedBy: teacher, Preorder: students,
	})
	if err != nil {
		t.Fatalf("prepare round: %v", err)
	}
	if err := roundSvc.ActivateIfNeeded(ctx, sessionID); err != nil {
		t.Fatalf("activate initial round: %v", err)
	}
	err = queueRepo.WithTx(ctx, func(tx *queue.Tx) error {
		round, err = tx.LockRound(ctx, round.ID)
		if err != nil {
			return err
		}
		entry, err := tx.InsertQueueEntry(ctx, round.ID, students[0], 1)
		if err != nil {
			return err
		}
		_, err = tx.SetRoundSelection(ctx, round.ID, &entry.ID, round.Version)
		return err
	})
	if err != nil {
		t.Fatalf("seed active round entry: %v", err)
	}

	gateway := &integGateway{}
	roles := &integRoles{pool: pool}
	svc, err := sessions.NewServiceWithRoomKey(repo, gateway, roles, []byte("test-room-key-0000000000000000000000000000"))
	if err != nil {
		t.Fatalf("create session service: %v", err)
	}
	obs := &capturingQueueObserver{next: queue.NewSessionObserver(roundSvc, queue.NewConvergence(queueRepo, nil, nil))}
	svc.SetQueueObserver(sessions.NewBoundedQueueObserver(obs, 0))

	return &sessionQueueConvergenceFixture{pool: pool, sessions: svc, repo: queueRepo, teacher: teacher, students: students, session: sessionID, observer: obs}
}

func TestSessionEnd_FinalizesNeverActivatedPreparedRounds(t *testing.T) {
	f := newSessionQueueConvergenceFixture(t)
	ctx := context.Background()

	// Prepare a second round that will never activate before session end.
	roundSvc := queue.NewRoundService(f.repo)
	prepared, err := roundSvc.Prepare(ctx, queue.PrepareRoundInput{
		SessionID: f.session, Type: queue.RoundTypeRevision, SurahID: 1, FromAyah: 1, ToAyah: 7,
		SurahAyahCount: 7, GradingRequired: true, CreatedBy: f.teacher,
	})
	if err != nil {
		t.Fatalf("prepare second round: %v", err)
	}
	if prepared.Lifecycle != queue.RoundLifecyclePrepared {
		t.Fatalf("second round lifecycle = %q, want prepared", prepared.Lifecycle)
	}

	ended, err := f.sessions.EndSession(ctx, f.teacher, f.session, sessions.EndReasonManual)
	if err != nil {
		t.Fatalf("end session: %v", err)
	}
	if ended.Status != sessions.SessionStatusEnded {
		t.Fatalf("session status = %q, want ended", ended.Status)
	}

	convergence := queue.NewConvergence(f.repo, nil, nil)
	if err := convergence.FinalizeSession(ctx, f.session); err != nil {
		t.Fatalf("finalize session: %v", err)
	}

	var count int
	if err := f.pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM recitation_queue
		WHERE session_id = $1::uuid AND lifecycle IN ('active', 'prepared')
	`, f.session).Scan(&count); err != nil {
		t.Fatalf("count unfinalized rounds: %v", err)
	}
	if count != 0 {
		t.Fatalf("unfinalized rounds = %d, want 0", count)
	}

	var preparedLifecycle string
	if err := f.pool.QueryRow(ctx, `
		SELECT lifecycle FROM recitation_queue WHERE id = $1::uuid
	`, prepared.ID).Scan(&preparedLifecycle); err != nil {
		t.Fatalf("load prepared round after end: %v", err)
	}
	if preparedLifecycle != "finalized" {
		t.Fatalf("prepared round lifecycle = %q, want finalized", preparedLifecycle)
	}
}

func TestSessionEnd_StartupReconcileFinalizesLeftoverRounds(t *testing.T) {
	f := newSessionQueueConvergenceFixture(t)
	ctx := context.Background()

	// End the session without the observer firing (nil observer).
	svc, err := sessions.NewServiceWithRoomKey(
		sessions.NewSessionRepository(f.pool),
		&integGateway{},
		&integRoles{pool: f.pool},
		[]byte("test-room-key-0000000000000000000000000000"),
	)
	if err != nil {
		t.Fatalf("create session service without observer: %v", err)
	}
	ended, err := svc.EndSession(ctx, f.teacher, f.session, sessions.EndReasonManual)
	if err != nil {
		t.Fatalf("end session: %v", err)
	}
	if ended.Status != sessions.SessionStatusEnded {
		t.Fatalf("session status = %q, want ended", ended.Status)
	}

	var unfinalized int
	if err := f.pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM recitation_queue
		WHERE session_id = $1::uuid AND lifecycle IN ('active', 'prepared')
	`, f.session).Scan(&unfinalized); err != nil {
		t.Fatalf("count unfinalized rounds: %v", err)
	}
	if unfinalized == 0 {
		t.Fatal("expected leftover active round for reconcile test")
	}

	convergence := queue.NewConvergence(f.repo, nil, nil)
	if err := convergence.Reconcile(ctx); err != nil {
		t.Fatalf("reconcile: %v", err)
	}

	if err := f.pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM recitation_queue
		WHERE session_id = $1::uuid AND lifecycle IN ('active', 'prepared')
	`, f.session).Scan(&unfinalized); err != nil {
		t.Fatalf("count rounds after reconcile: %v", err)
	}
	if unfinalized != 0 {
		t.Fatalf("rounds left unfinalized after reconcile = %d, want 0", unfinalized)
	}
}
