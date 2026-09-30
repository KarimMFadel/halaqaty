//go:build integration

package scheduling

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/KarimMFadel/halaqaty/backend/internal/rbac"
	"github.com/KarimMFadel/halaqaty/backend/internal/sessions"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestOccurrenceStart_VirtualCancellationWaitsForMaterializationAndActivation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	pool := openRepoTestPool(t, ctx)
	fx := newOccurrenceStartFixture(t, ctx, pool)
	gate := newOccurrenceStartGateway()
	starts := fx.occurrenceStartService(gate)

	started := make(chan occurrenceStartResult, 1)
	go func() {
		session, _, err := starts.StartOccurrence(ctx, occurrenceStartCommand(fx, "start-cancel"))
		started <- occurrenceStartResult{session: session, err: err}
	}()
	awaitOccurrenceStartGate(t, pool, gate, started)

	cancelled := make(chan error, 1)
	go func() {
		_, err := fx.schedules.ChangeOccurrence(ctx, ChangeOccurrenceCommand{
			ActorID: fx.teacher, CircleID: fx.circle, ScheduleID: fx.schedule.ID,
			IdempotencyKey: "cancel-during-start", OriginalLocalDate: fx.date,
			ExpectedSeriesVersion: 1, ExpectedOccurrenceVersion: 0, Cancelled: boolPtr(true),
		})
		cancelled <- err
	}()
	waitForOccurrenceLockWaiter(t, pool, "FROM circle_members")
	close(gate.release)
	startResult := <-started
	if startResult.err != nil || startResult.session.Status != sessions.SessionStatusActive {
		t.Fatalf("start result = %+v, want active session", startResult)
	}
	if err := <-cancelled; !errors.Is(err, ErrOccurrenceStarted) {
		t.Fatalf("cancellation after start = %v, want ErrOccurrenceStarted", err)
	}
	assertOccurrenceMaterialization(t, ctx, pool, fx, startResult.session.ID, "active", false)
}

func TestOccurrenceStart_SeriesEditWaitsAndRetainsStartedPlan(t *testing.T) {
	ctx := context.Background()
	pool := openRepoTestPool(t, ctx)
	fx := newOccurrenceStartFixture(t, ctx, pool)
	gate := newOccurrenceStartGateway()
	starts := fx.occurrenceStartService(gate)

	started := make(chan occurrenceStartResult, 1)
	go func() {
		session, _, err := starts.StartOccurrence(ctx, occurrenceStartCommand(fx, "start-series-edit"))
		started <- occurrenceStartResult{session: session, err: err}
	}()
	awaitOccurrenceStartGate(t, pool, gate, started)

	edited := make(chan error, 1)
	go func() {
		plan := intervalRecord(fx.date, 1, IntervalUnitDay)
		plan.Title = "Changed Series"
		_, err := fx.schedules.Change(ctx, ChangeScheduleCommand{
			ActorID: fx.teacher, CircleID: fx.circle, ScheduleID: fx.schedule.ID,
			IdempotencyKey: "edit-during-start", ExpectedVersion: 1,
			EffectiveLocalDate: fx.date, Plan: plan,
		})
		edited <- err
	}()
	waitForOccurrenceLockWaiter(t, pool, "FROM circle_members")
	close(gate.release)
	startResult := <-started
	if startResult.err != nil || startResult.session.Status != sessions.SessionStatusActive {
		t.Fatalf("start result = %+v, want active session", startResult)
	}
	if err := <-edited; err != nil {
		t.Fatalf("series edit after start: %v", err)
	}
	assertOccurrenceMaterialization(t, ctx, pool, fx, startResult.session.ID, "active", false)
	var currentVersion int
	var title string
	if err := pool.QueryRow(ctx, `SELECT current_version FROM schedules WHERE id=$1::uuid`, fx.schedule.ID).Scan(&currentVersion); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `SELECT title FROM planned_session_details WHERE session_id=$1::uuid`, startResult.session.ID).Scan(&title); err != nil {
		t.Fatal(err)
	}
	if currentVersion != 2 || title != "Interval Circle" {
		t.Fatalf("series/detail after start = version %d, title %q; want revision 2 and retained started title", currentVersion, title)
	}
}

func TestOccurrenceStart_RetainsEarlierExceptionAfterLaterSeriesEdit(t *testing.T) {
	ctx := context.Background()
	pool := openRepoTestPool(t, ctx)
	fx := newOccurrenceStartFixture(t, ctx, pool)
	cancelled := fx.date
	movedOriginal := fx.date.AddDate(0, 0, 1)
	movedDate := fx.date.AddDate(0, 0, 2)
	cancelledAt := time.Now().UTC()
	if _, err := fx.repo.UpsertException(ctx, OccurrenceException{ScheduleID: fx.schedule.ID, OriginalLocalDate: cancelled, CancelledAt: &cancelledAt, UpdatedBy: fx.teacher}); err != nil {
		t.Fatalf("cancel earlier occurrence: %v", err)
	}
	if _, err := fx.repo.UpsertException(ctx, OccurrenceException{ScheduleID: fx.schedule.ID, OriginalLocalDate: movedOriginal, ReplacementLocalDate: &movedDate, UpdatedBy: fx.teacher}); err != nil {
		t.Fatalf("move earlier occurrence: %v", err)
	}
	newPlan := intervalRecord(fx.date, 1, IntervalUnitDay)
	newPlan.EffectiveLocalDate = fx.date.AddDate(0, 0, 7)
	if _, err := fx.repo.AppendRevision(ctx, fx.schedule.ID, 1, newPlan); err != nil {
		t.Fatalf("edit later series: %v", err)
	}
	schedule, err := fx.repo.GetSchedule(ctx, fx.schedule.ID)
	if err != nil {
		t.Fatal(err)
	}
	start := &OccurrenceStartService{}
	cmd := occurrenceStartCommand(fx, "retained-cancellation")
	if _, err := start.resolveOccurrenceStartPlan(ctx, pool, cmd, schedule); !errors.Is(err, ErrOccurrenceNotFound) {
		t.Fatalf("retained cancellation resolves with error %v, want occurrence not found", err)
	}
	cmd.OriginalLocalDate = movedOriginal
	plan, err := start.resolveOccurrenceStartPlan(ctx, pool, cmd, schedule)
	if err != nil || !plan.effectiveDate.Equal(movedDate) {
		t.Fatalf("retained move resolves to %v, err %v; want %v", plan.effectiveDate, err, movedDate)
	}
}

func TestOccurrenceStart_StopUsesMovedEffectiveDate(t *testing.T) {
	ctx := context.Background()
	pool := openRepoTestPool(t, ctx)
	fx := newOccurrenceStartFixture(t, ctx, pool)
	original := fx.date.AddDate(0, 0, 5)
	moved := fx.date.AddDate(0, 0, 1)
	boundary := fx.date.AddDate(0, 0, 3)
	if _, err := fx.repo.UpsertException(ctx, OccurrenceException{ScheduleID: fx.schedule.ID, OriginalLocalDate: original, ReplacementLocalDate: &moved, UpdatedBy: fx.teacher}); err != nil {
		t.Fatalf("move occurrence before stop boundary: %v", err)
	}
	schedule, err := fx.repo.StopSchedule(ctx, fx.schedule.ID, 1, boundary)
	if err != nil {
		t.Fatalf("stop schedule: %v", err)
	}
	cmd := occurrenceStartCommand(fx, "moved-before-stop")
	cmd.OriginalLocalDate = original
	plan, err := (&OccurrenceStartService{}).resolveOccurrenceStartPlan(ctx, pool, cmd, schedule)
	if err != nil || !plan.effectiveDate.Equal(moved) {
		t.Fatalf("occurrence moved before stop resolves to %v, err %v; want %v", plan.effectiveDate, err, moved)
	}
}

func TestOccurrenceStart_OneOffStartWinsConcurrentCancellation(t *testing.T) {
	ctx := context.Background()
	pool := openRepoTestPool(t, ctx)
	fx := newOccurrenceStartFixture(t, ctx, pool)
	gate := newOccurrenceStartGateway()
	sessionService := fx.sessionService(gate)
	oneOffs := NewPlannedSessionService(fx.repo)
	plan := plannedSessionTestPlan(fx.date, 60, "One-off race")
	oneOff, err := oneOffs.Create(ctx, CreatePlannedSessionCommand{
		ActorID: fx.teacher, CircleID: fx.circle, IdempotencyKey: "create-one-off-race", Plan: plan,
	})
	if err != nil {
		t.Fatalf("create one-off: %v", err)
	}

	started := make(chan occurrenceStartResult, 1)
	go func() {
		session, _, err := sessionService.StartSession(ctx, fx.teacher, oneOff.ID)
		started <- occurrenceStartResult{session: session, err: err}
	}()
	awaitOccurrenceStartGate(t, pool, gate, started)
	cancelled := make(chan error, 1)
	go func() {
		_, err := oneOffs.Change(ctx, ChangePlannedSessionCommand{
			ActorID: fx.teacher, SessionID: oneOff.ID, IdempotencyKey: "cancel-during-one-off-start",
			ExpectedVersion: oneOff.Version, Cancelled: boolPtr(true),
		})
		cancelled <- err
	}()
	waitForOccurrenceLockWaiter(t, pool, "pg_advisory_xact_lock")
	close(gate.release)
	startResult := <-started
	if startResult.err != nil || startResult.session.Status != sessions.SessionStatusActive {
		t.Fatalf("one-off start = %+v, want active session", startResult)
	}
	if err := <-cancelled; !errors.Is(err, ErrOccurrenceStarted) {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) {
			t.Logf("one-off cancellation PostgreSQL error code=%q detail=%q hint=%q where=%q", pgErr.Code, pgErr.Detail, pgErr.Hint, pgErr.Where)
		}
		t.Fatalf("one-off cancellation after start = %v, want ErrOccurrenceStarted", err)
	}
	assertOneOffStartState(t, ctx, pool, oneOff.ID, "active", false)
}

func TestOccurrenceStart_DuplicateRecurringStartsConvergeOnOneSession(t *testing.T) {
	ctx := context.Background()
	pool := openRepoTestPool(t, ctx)
	fx := newOccurrenceStartFixture(t, ctx, pool)
	gate := newOccurrenceStartGateway()
	starts := fx.occurrenceStartService(gate)

	results := make(chan occurrenceStartResult, 2)
	for i := 0; i < 2; i++ {
		go func(i int) {
			session, _, err := starts.StartOccurrence(ctx, occurrenceStartCommand(fx, fmt.Sprintf("duplicate-start-%d", i)))
			results <- occurrenceStartResult{session: session, err: err}
		}(i)
	}
	awaitOccurrenceStartGate(t, pool, gate, results)
	waitForOccurrenceLockWaiter(t, pool, "FROM circle_members")
	close(gate.release)
	first, second := <-results, <-results
	for _, result := range []occurrenceStartResult{first, second} {
		if result.err != nil || result.session.Status != sessions.SessionStatusActive {
			t.Fatalf("duplicate start result = %+v, want active session", result)
		}
	}
	if first.session.ID != second.session.ID {
		t.Fatalf("duplicate starts returned sessions %q and %q", first.session.ID, second.session.ID)
	}
	assertOccurrenceMaterialization(t, ctx, pool, fx, first.session.ID, "active", false)
	if got := gate.ensureCalls.Load(); got != 1 {
		t.Fatalf("EnsureRoom calls = %d, want one for the shared occurrence", got)
	}
}

type occurrenceStartFixture struct {
	pool      *pgxpool.Pool
	repo      *Repository
	schedules *ScheduleService
	teacher   string
	circle    string
	schedule  Schedule
	date      time.Time
}

type occurrenceStartResult struct {
	session sessions.Session
	err     error
}

func newOccurrenceStartFixture(t *testing.T, ctx context.Context, pool *pgxpool.Pool) occurrenceStartFixture {
	t.Helper()
	teacher := "11111111-1111-1111-1111-111111111111"
	circle := "22222222-2222-2222-2222-222222222222"
	seedRepoUser(t, ctx, pool, teacher)
	seedRepoCircle(t, ctx, pool, circle, teacher, "HLQ-OCC1")
	if _, err := pool.Exec(ctx, `INSERT INTO circle_members(circle_id,user_id,role) VALUES($1::uuid,$2::uuid,'teacher')`, circle, teacher); err != nil {
		t.Fatalf("seed teacher membership: %v", err)
	}
	repo := NewScheduleRepository(pool)
	schedules := NewScheduleService(repo)
	date := civilDate(2030, 1, 3)
	schedules.now = func() time.Time { return civilDate(2030, 1, 1) }
	schedule, err := schedules.Create(ctx, CreateScheduleCommand{
		ActorID: teacher, CircleID: circle, IdempotencyKey: "create-recurring-race",
		Plan: intervalRecord(date, 1, IntervalUnitDay),
	})
	if err != nil {
		t.Fatalf("create recurring schedule: %v", err)
	}
	return occurrenceStartFixture{pool: pool, repo: repo, schedules: schedules, teacher: teacher, circle: circle, schedule: schedule, date: date}
}

func (f occurrenceStartFixture) occurrenceStartService(gateway *occurrenceStartGateway) *OccurrenceStartService {
	return NewOccurrenceStartService(f.repo, f.sessionService(gateway))
}

func (f occurrenceStartFixture) sessionService(gateway *occurrenceStartGateway) *sessions.Service {
	sessionRepo := sessions.NewSessionRepository(f.pool)
	service, err := sessions.NewServiceWithRoomKey(sessionRepo, gateway, rbac.NewRepository(f.pool), []byte("integration-test-room-key"))
	if err != nil {
		panic(err)
	}
	return service
}

func occurrenceStartCommand(f occurrenceStartFixture, key string) StartOccurrenceCommand {
	return StartOccurrenceCommand{
		ActorID: f.teacher, CircleID: f.circle, ScheduleID: f.schedule.ID,
		OriginalLocalDate: f.date, IdempotencyKey: key,
	}
}

func assertOccurrenceMaterialization(t *testing.T, ctx context.Context, pool *pgxpool.Pool, f occurrenceStartFixture, sessionID, wantStatus string, wantCancelled bool) {
	t.Helper()
	var status string
	var cancelled bool
	var sessionCount, detailCount int
	if err := pool.QueryRow(ctx, `
		SELECT s.status, d.cancelled_at IS NOT NULL
		FROM sessions s JOIN planned_session_details d ON d.session_id=s.id
		WHERE s.id=$1::uuid`, sessionID).Scan(&status, &cancelled); err != nil {
		t.Fatalf("read materialized occurrence: %v", err)
	}
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM sessions WHERE circle_id=$1::uuid AND created_by=$2::uuid`, f.circle, f.teacher).Scan(&sessionCount); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM planned_session_details WHERE schedule_id=$1::uuid AND original_local_date=$2`, f.schedule.ID, f.date).Scan(&detailCount); err != nil {
		t.Fatal(err)
	}
	if status != wantStatus || cancelled != wantCancelled || sessionCount != 1 || detailCount != 1 {
		t.Fatalf("occurrence rows = status %q cancelled %t, %d sessions, %d details; want %q/%t and exactly one", status, cancelled, sessionCount, detailCount, wantStatus, wantCancelled)
	}
}

func assertOneOffStartState(t *testing.T, ctx context.Context, pool *pgxpool.Pool, sessionID, wantStatus string, wantCancelled bool) {
	t.Helper()
	var status string
	var cancelled bool
	var sessionCount, detailCount int
	if err := pool.QueryRow(ctx, `
		SELECT s.status, d.cancelled_at IS NOT NULL
		FROM sessions s JOIN planned_session_details d ON d.session_id=s.id
		WHERE s.id=$1::uuid`, sessionID).Scan(&status, &cancelled); err != nil {
		t.Fatalf("read one-off state: %v", err)
	}
	if status != wantStatus || cancelled != wantCancelled {
		t.Fatalf("one-off state = %q cancelled=%t, want %q/%t", status, cancelled, wantStatus, wantCancelled)
	}
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM sessions WHERE id=$1::uuid`, sessionID).Scan(&sessionCount); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM planned_session_details WHERE session_id=$1::uuid`, sessionID).Scan(&detailCount); err != nil {
		t.Fatal(err)
	}
	if sessionCount != 1 || detailCount != 1 {
		t.Fatalf("one-off rows = %d sessions, %d details; want one each", sessionCount, detailCount)
	}
}

type occurrenceStartGateway struct {
	entered     chan struct{}
	release     chan struct{}
	gateOnce    sync.Once
	ensureCalls atomic.Int32
}

func newOccurrenceStartGateway() *occurrenceStartGateway {
	return &occurrenceStartGateway{entered: make(chan struct{}), release: make(chan struct{})}
}

func (g *occurrenceStartGateway) EnsureRoom(ctx context.Context, _ sessions.MediaRoomRef, _ sessions.MediaMode) error {
	g.ensureCalls.Add(1)
	g.gateOnce.Do(func() { close(g.entered) })
	select {
	case <-g.release:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (*occurrenceStartGateway) CloseRoom(context.Context, sessions.MediaRoomRef) error { return nil }
func (*occurrenceStartGateway) IssueConnection(_ context.Context, _ sessions.MediaRoomRef, _ string, _ sessions.MediaGrants) (sessions.MediaConnection, error) {
	return sessions.MediaConnection{Endpoint: "wss://media.example", Credential: "test-credential", ExpiresAt: time.Now().Add(time.Hour)}, nil
}
func (*occurrenceStartGateway) MuteParticipant(context.Context, sessions.MediaRoomRef, string) error {
	return nil
}
func (*occurrenceStartGateway) UnmuteParticipant(context.Context, sessions.MediaRoomRef, string) error {
	return nil
}
func (*occurrenceStartGateway) MuteAll(context.Context, sessions.MediaRoomRef) error { return nil }
func (*occurrenceStartGateway) RemoveParticipant(context.Context, sessions.MediaRoomRef, string) error {
	return nil
}

func awaitOccurrenceStartGate(t *testing.T, pool *pgxpool.Pool, gateway *occurrenceStartGateway, results <-chan occurrenceStartResult) {
	t.Helper()
	select {
	case <-gateway.entered:
	case result := <-results:
		t.Fatalf("start returned before EnsureRoom: session=%+v err=%v", result.session, result.err)
	case <-time.After(5 * time.Second):
		logOccurrenceDatabaseActivity(t, pool)
		t.Fatal("start did not reach EnsureRoom")
	}
}

func waitForOccurrenceLockWaiter(t *testing.T, pool *pgxpool.Pool, queryFragment string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	monitorPool, err := pgxpool.New(ctx, os.Getenv("DATABASE_URL"))
	if err != nil {
		t.Fatalf("open lock-wait observer pool: %v", err)
	}
	defer monitorPool.Close()
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	for {
		var waiting bool
		if err := monitorPool.QueryRow(ctx, `
			SELECT EXISTS (
				SELECT 1 FROM pg_stat_activity
				WHERE datname=current_database() AND wait_event_type='Lock'
				  AND lower(query) LIKE '%' || lower($1) || '%'
			)`, queryFragment).Scan(&waiting); err != nil {
			stats := pool.Stat()
			t.Logf("pgxpool stats total=%d acquired=%d idle=%d max=%d", stats.TotalConns(), stats.AcquiredConns(), stats.IdleConns(), stats.MaxConns())
			logOccurrenceDatabaseActivity(t, monitorPool)
			t.Fatalf("observe lock waiter: %v", err)
		}
		if waiting {
			return
		}
		select {
		case <-ctx.Done():
			logOccurrenceDatabaseActivity(t, monitorPool)
			t.Fatalf("no PostgreSQL lock waiter observed for %q", queryFragment)
		case <-ticker.C:
		}
	}
}

func logOccurrenceDatabaseActivity(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	rows, err := pool.Query(ctx, `
		SELECT application_name, state, COALESCE(wait_event_type, ''),
		       COALESCE(wait_event, ''), query
		FROM pg_stat_activity
		WHERE datname = current_database()
		ORDER BY pid`)
	if err != nil {
		t.Logf("pg_stat_activity diagnostic failed: %v", err)
		return
	}
	defer rows.Close()
	for rows.Next() {
		var applicationName, state, waitType, waitEvent, query string
		if err := rows.Scan(&applicationName, &state, &waitType, &waitEvent, &query); err != nil {
			t.Logf("scan pg_stat_activity diagnostic: %v", err)
			return
		}
		t.Logf("pg_stat_activity application_name=%q state=%q wait_event_type=%q wait_event=%q query=%q", applicationName, state, waitType, waitEvent, query)
	}
	if err := rows.Err(); err != nil {
		t.Logf("read pg_stat_activity diagnostic: %v", err)
	}
}
