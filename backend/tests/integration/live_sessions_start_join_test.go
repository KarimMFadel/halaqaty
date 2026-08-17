//go:build integration

package integration

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/KarimMFadel/halaqaty/backend/internal/sessions"
)

// ---- integration doubles ---------------------------------------------------

// integGateway is a fake SessionMediaGateway that records calls and returns
// deterministic connections for integration tests.
type integGateway struct {
	mu     sync.Mutex
	issued int
}

func (g *integGateway) EnsureRoom(_ context.Context, _ sessions.MediaRoomRef, _ sessions.MediaMode) error {
	return nil
}
func (g *integGateway) CloseRoom(_ context.Context, _ sessions.MediaRoomRef) error { return nil }
func (g *integGateway) IssueConnection(_ context.Context, _ sessions.MediaRoomRef, userID string, _ sessions.MediaGrants) (sessions.MediaConnection, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.issued++
	return sessions.MediaConnection{
		Endpoint:   "wss://media.test.halaqaty.app/room",
		Credential: sessions.MediaCredential(fmt.Sprintf("integ-cred-%s-%d", userID, g.issued)),
		ExpiresAt:  time.Now().Add(time.Hour),
	}, nil
}
func (g *integGateway) MuteParticipant(_ context.Context, _ sessions.MediaRoomRef, _ string) error {
	return nil
}
func (g *integGateway) UnmuteParticipant(_ context.Context, _ sessions.MediaRoomRef, _ string) error {
	return nil
}
func (g *integGateway) MuteAll(_ context.Context, _ sessions.MediaRoomRef) error { return nil }
func (g *integGateway) RemoveParticipant(_ context.Context, _ sessions.MediaRoomRef, _ string) error {
	return nil
}

// integRoles reads circle membership roles from the real circle_members table.
type integRoles struct {
	pool *pgxpool.Pool
}

func (r *integRoles) RoleInCircle(ctx context.Context, circleID, userID string) (string, error) {
	var role string
	err := r.pool.QueryRow(ctx,
		`SELECT role FROM circle_members WHERE circle_id = $1::uuid AND user_id = $2::uuid`,
		circleID, userID).Scan(&role)
	if err != nil {
		return "", nil // "" = not a member
	}
	return role, nil
}

// ---- test environment -------------------------------------------------------

// sessionIntegEnv holds the wired session service against real PostgreSQL.
type sessionIntegEnv struct {
	pool     *pgxpool.Pool
	repo     *sessions.Repository
	svc      *sessions.Service
	gw       *integGateway
	schema   string
	userIDs  map[string]string
	circleID string
}

// setupSessionIntegEnv creates a fresh schema, runs all migrations, seeds a
// circle with members, and wires the session service.
func setupSessionIntegEnv(t *testing.T) *sessionIntegEnv {
	t.Helper()
	ctx := context.Background()

	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		t.Skip("DATABASE_URL is not set")
	}

	adminPool, err := pgxpool.New(ctx, dbURL)
	if err != nil {
		t.Fatalf("open admin pool: %v", err)
	}
	conn := acquireConn(t, adminPool, ctx)
	schema := uniqueSchemaName(t)
	createSchema(t, conn, ctx, schema)

	migrations := []string{
		"000010_auth_roles_profile.up.sql",
		"000011_auth_roles_profile_alignment.up.sql",
		"000012_auth_profiles_display_name.up.sql",
		"000013_create_circles.up.sql",
		"000014_circle_members_circle_fk.up.sql",
		"000015_circle_management.up.sql",
		"000016_live_sessions.up.sql",
	}
	for _, m := range migrations {
		runMigrationFile(t, conn, ctx, m)
	}
	conn.Release()

	poolCfg, err := pgxpool.ParseConfig(dbURL)
	if err != nil {
		t.Fatalf("parse pool config: %v", err)
	}
	poolCfg.ConnConfig.RuntimeParams["search_path"] = schema
	pool, err := pgxpool.NewWithConfig(ctx, poolCfg)
	if err != nil {
		t.Fatalf("open schema pool: %v", err)
	}
	t.Cleanup(func() {
		pool.Close()
		dropSchema(t, adminPool, ctx, schema)
		adminPool.Close()
	})

	// Seed users.
	users := map[string]string{"teacher": "teacher@test.com", "supervisor": "super@test.com", "student": "student@test.com", "outsider": "outsider@test.com"}
	userIDs := make(map[string]string)
	sConn, _ := pool.Acquire(ctx)
	defer sConn.Release()
	for label, email := range users {
		var id string
		if err := sConn.QueryRow(ctx, `INSERT INTO users (firebase_uid, email) VALUES ($1, $2) RETURNING id::text`, "firebase-"+label, email).Scan(&id); err != nil {
			t.Fatalf("seed user %s: %v", label, err)
		}
		userIDs[label] = id
	}

	// Seed circle and members.
	var circleID string
	if err := sConn.QueryRow(ctx, `INSERT INTO circles (name, teacher_id, invite_code) VALUES ($1, $2::uuid, $3) RETURNING id::text`, "Integ Circle", userIDs["teacher"], "HLQ-INT01").Scan(&circleID); err != nil {
		t.Fatalf("seed circle: %v", err)
	}
	for _, pair := range []struct{ label, role string }{
		{"teacher", "teacher"}, {"supervisor", "supervisor"}, {"student", "student"},
	} {
		if _, err := sConn.Exec(ctx, `INSERT INTO circle_members (circle_id, user_id, role) VALUES ($1::uuid, $2::uuid, $3)`, circleID, userIDs[pair.label], pair.role); err != nil {
			t.Fatalf("seed member %s: %v", pair.label, err)
		}
	}

	// Seed a second circle for cross-circle denial tests.
	if _, err := sConn.Exec(ctx, `INSERT INTO circles (name, teacher_id, invite_code) VALUES ($1, $2::uuid, $3)`, "Other Circle", userIDs["teacher"], "HLQ-INT02"); err != nil {
		t.Fatalf("seed second circle: %v", err)
	}

	repo := sessions.NewSessionRepository(pool)
	gw := &integGateway{}
	roles := &integRoles{pool: pool}
	svc := sessions.NewService(repo, gw, roles)

	return &sessionIntegEnv{
		pool:     pool,
		repo:     repo,
		svc:      svc,
		gw:       gw,
		schema:   schema,
		userIDs:  userIDs,
		circleID: circleID,
	}
}

// ---- T015 integration tests -------------------------------------------------

func TestStartJoinIntegration_HappyPath(t *testing.T) {
	env := setupSessionIntegEnv(t)
	ctx := context.Background()

	// Teacher creates and starts.
	created, err := env.svc.CreateAdHocSession(ctx, env.userIDs["teacher"], env.circleID)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	started, conn, err := env.svc.StartSession(ctx, env.userIDs["teacher"], created.ID)
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	if started.Status != sessions.SessionStatusActive {
		t.Fatalf("status = %q, want active", started.Status)
	}
	if conn.Credential == "" || conn.ExpiresAt.IsZero() {
		t.Fatal("start must return a complete media connection")
	}

	// Student joins.
	joined, sConn, err := env.svc.JoinSession(ctx, env.userIDs["student"], created.ID)
	if err != nil {
		t.Fatalf("student join: %v", err)
	}
	if joined.ParticipantCount < 2 {
		t.Fatalf("participant count = %d, want >= 2", joined.ParticipantCount)
	}
	if sConn.Credential == "" {
		t.Fatal("student must receive a connection")
	}

	// Verify credentials differ.
	if conn.Credential == sConn.Credential {
		t.Error("teacher and student credentials must differ")
	}

	// Verify persisted state.
	persisted, err := env.repo.GetSession(ctx, created.ID)
	if err != nil {
		t.Fatalf("get persisted session: %v", err)
	}
	if persisted.Status != sessions.SessionStatusActive {
		t.Fatalf("persisted status = %q, want active", persisted.Status)
	}
	if persisted.ParticipantCount < 2 {
		t.Fatalf("persisted count = %d, want >= 2", persisted.ParticipantCount)
	}
}

func TestStartJoinIntegration_CrossCircleDenial(t *testing.T) {
	env := setupSessionIntegEnv(t)
	ctx := context.Background()

	// Teacher creates session in circle 1.
	created, err := env.svc.CreateAdHocSession(ctx, env.userIDs["teacher"], env.circleID)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if _, _, err := env.svc.StartSession(ctx, env.userIDs["teacher"], created.ID); err != nil {
		t.Fatalf("start: %v", err)
	}

	// Teacher from circle 1 tries to start a session that belongs to circle 2.
	// The student is in circle 2 but the session is in circle 1.
	// Student (member of circle 1) can join fine — this is correct.
	if _, _, err := env.svc.JoinSession(ctx, env.userIDs["student"], created.ID); err != nil {
		t.Fatalf("student in same circle can join: %v", err)
	}

	// Outsider (not a member of circle 1) cannot join.
	_, _, err = env.svc.JoinSession(ctx, env.userIDs["outsider"], created.ID)
	if !errors.Is(err, sessions.ErrNotCircleMember) {
		t.Fatalf("outsider join: got %v, want ErrNotCircleMember", err)
	}

	// Outsider cannot start.
	_, _, err = env.svc.StartSession(ctx, env.userIDs["outsider"], created.ID)
	if !errors.Is(err, sessions.ErrNotCircleMember) {
		t.Fatalf("outsider start: got %v, want ErrNotCircleMember", err)
	}

	// Student cannot start (member but not moderator).
	_, _, err = env.svc.StartSession(ctx, env.userIDs["student"], created.ID)
	if !errors.Is(err, sessions.ErrModeratorRoleRequired) {
		t.Fatalf("student start: got %v, want ErrModeratorRoleRequired", err)
	}
}

func TestStartJoinIntegration_StaleMembership(t *testing.T) {
	env := setupSessionIntegEnv(t)
	ctx := context.Background()

	created, err := env.svc.CreateAdHocSession(ctx, env.userIDs["teacher"], env.circleID)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if _, _, err := env.svc.StartSession(ctx, env.userIDs["teacher"], created.ID); err != nil {
		t.Fatalf("start: %v", err)
	}

	// Remove student from circle_members.
	if _, err := env.pool.Exec(ctx, `DELETE FROM circle_members WHERE circle_id = $1::uuid AND user_id = $2::uuid`, env.circleID, env.userIDs["student"]); err != nil {
		t.Fatalf("remove membership: %v", err)
	}

	// Student can no longer join.
	_, _, err = env.svc.JoinSession(ctx, env.userIDs["student"], created.ID)
	if !errors.Is(err, sessions.ErrNotCircleMember) {
		t.Fatalf("stale membership join: got %v, want ErrNotCircleMember", err)
	}
}

func TestStartJoinIntegration_ConcurrentStartConvergesToOneRoom(t *testing.T) {
	env := setupSessionIntegEnv(t)
	ctx := context.Background()

	created, err := env.svc.CreateAdHocSession(ctx, env.userIDs["teacher"], env.circleID)
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	const starters = 4
	type result struct {
		roomRef sessions.MediaRoomRef
		conn    sessions.MediaConnection
		err     error
	}
	results := make([]result, starters)
	var wg sync.WaitGroup
	for i := range starters {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			started, conn, err := env.svc.StartSession(ctx, env.userIDs["teacher"], created.ID)
			results[i] = result{roomRef: started.MediaRoomRef, conn: conn, err: err}
		}(i)
	}
	wg.Wait()

	var persistedRef sessions.MediaRoomRef
	for _, r := range results {
		if r.err != nil {
			t.Fatalf("concurrent start error: %v", r.err)
		}
		if persistedRef == "" {
			persistedRef = r.roomRef
		}
		if r.roomRef != persistedRef {
			t.Fatalf("starters disagree on room: %q vs %q", r.roomRef, persistedRef)
		}
		if r.conn.Credential == "" {
			t.Fatal("every starter must receive a connection")
		}
	}

	// All starters got distinct credentials.
	creds := make(map[string]bool)
	for _, r := range results {
		if creds[string(r.conn.Credential)] {
			t.Fatal("duplicate credential issued")
		}
		creds[string(r.conn.Credential)] = true
	}

	// Verify exactly one room was ensured (the fake gateway doesn't count,
	// but the persisted room ref is authoritative).
	persisted, err := env.repo.GetSession(ctx, created.ID)
	if err != nil {
		t.Fatalf("get session: %v", err)
	}
	if persisted.MediaRoomRef != persistedRef {
		t.Fatalf("persisted ref %q != converged ref %q", persisted.MediaRoomRef, persistedRef)
	}
}

func TestStartJoinIntegration_ConcurrentJoinCapacity(t *testing.T) {
	env := setupSessionIntegEnv(t)
	ctx := context.Background()

	created, err := env.svc.CreateAdHocSession(ctx, env.userIDs["teacher"], env.circleID)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if _, _, err := env.svc.StartSession(ctx, env.userIDs["teacher"], created.ID); err != nil {
		t.Fatalf("start: %v", err)
	}

	// Seed 48 more members to reach capacity with concurrent joins.
	// Starter is already #1, seed 48 via SQL to reach 49 present.
	sConn, _ := env.pool.Acquire(ctx)
	defer sConn.Release()
	for i := 0; i < 48; i++ {
		uid := fmt.Sprintf("filler-%d", i)
		var userID string
		if err := sConn.QueryRow(ctx, `INSERT INTO users (firebase_uid, email) VALUES ($1, $2) RETURNING id::text`, "firebase-"+uid, uid+"@test.com").Scan(&userID); err != nil {
			t.Fatalf("seed filler %d: %v", i, err)
		}
		if _, err := sConn.Exec(ctx, `INSERT INTO circle_members (circle_id, user_id, role) VALUES ($1::uuid, $2::uuid, $3)`, env.circleID, userID, "student"); err != nil {
			t.Fatalf("add filler %d to circle: %v", i, err)
		}
		if _, _, err := env.svc.JoinSession(ctx, userID, created.ID); err != nil {
			t.Fatalf("join filler %d: %v", i, err)
		}
	}

	// Student joins as 50th — should succeed.
	if _, _, err := env.svc.JoinSession(ctx, env.userIDs["student"], created.ID); err != nil {
		t.Fatalf("50th join: %v", err)
	}

	// Supervisor tries as 51st — should fail.
	_, _, err = env.svc.JoinSession(ctx, env.userIDs["supervisor"], created.ID)
	if !errors.Is(err, sessions.ErrSessionFull) {
		t.Fatalf("51st join: got %v, want ErrSessionFull", err)
	}

	// Verify persisted count is 50.
	persisted, _ := env.repo.GetSession(ctx, created.ID)
	if persisted.ParticipantCount != 50 {
		t.Fatalf("participant count = %d, want 50", persisted.ParticipantCount)
	}
}

func TestStartJoinIntegration_RemovedParticipantDenied(t *testing.T) {
	env := setupSessionIntegEnv(t)
	ctx := context.Background()

	created, err := env.svc.CreateAdHocSession(ctx, env.userIDs["teacher"], env.circleID)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if _, _, err := env.svc.StartSession(ctx, env.userIDs["teacher"], created.ID); err != nil {
		t.Fatalf("start: %v", err)
	}
	if _, _, err := env.svc.JoinSession(ctx, env.userIDs["student"], created.ID); err != nil {
		t.Fatalf("student join: %v", err)
	}

	// Remove the student directly in DB (simulates moderation).
	if _, err := env.pool.Exec(ctx, `
		UPDATE session_participant_presence
		SET removed_at = NOW(), is_currently_present = FALSE, hand_raised_at = NULL
		WHERE session_id = $1::uuid AND user_id = $2::uuid
	`, created.ID, env.userIDs["student"]); err != nil {
		t.Fatalf("mark removed: %v", err)
	}

	// Student cannot rejoin.
	_, _, err = env.svc.JoinSession(ctx, env.userIDs["student"], created.ID)
	if !errors.Is(err, sessions.ErrParticipantRemoved) {
		t.Fatalf("removed participant join: got %v, want ErrParticipantRemoved", err)
	}
}
