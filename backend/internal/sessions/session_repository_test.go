//go:build integration

package sessions

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

var sessionRepoMigrations = []string{
	"000010_auth_roles_profile.up.sql",
	"000011_auth_roles_profile_alignment.up.sql",
	"000012_auth_profiles_display_name.up.sql",
	"000013_create_circles.up.sql",
	"000014_circle_members_circle_fk.up.sql",
	"000015_circle_management.up.sql",
	"000016_live_sessions.up.sql",
}

// newSessionRepository opens an isolated schema with the full migration chain
// applied and returns a repository backed by a pool bound to that schema.
func newSessionRepository(t *testing.T) *Repository {
	t.Helper()
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		t.Skip("DATABASE_URL is not set")
	}
	ctx := context.Background()
	schema := fmt.Sprintf("test_sessions_repo_%d", time.Now().UnixNano())
	sep := "?"
	if strings.Contains(dbURL, sep) {
		sep = "&"
	}
	pool, err := pgxpool.New(ctx, dbURL+sep+"search_path="+schema)
	if err != nil {
		t.Fatalf("open pool: %v", err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), "DROP SCHEMA IF EXISTS "+schema+" CASCADE")
		pool.Close()
	})
	if _, err := pool.Exec(ctx, "CREATE SCHEMA "+schema); err != nil {
		t.Fatalf("create schema: %v", err)
	}

	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("could not determine test file path")
	}
	migrationsDir := filepath.Join(filepath.Dir(thisFile), "..", "..", "migrations")
	for _, name := range sessionRepoMigrations {
		sql, err := os.ReadFile(filepath.Join(migrationsDir, name))
		if err != nil {
			t.Fatalf("read migration %s: %v", name, err)
		}
		if _, err := pool.Exec(ctx, string(sql)); err != nil {
			t.Fatalf("apply migration %s: %v", name, err)
		}
	}
	return NewSessionRepository(pool)
}

func seedRepoUser(t *testing.T, repo *Repository, label string) string {
	t.Helper()
	var id string
	if err := repo.pool.QueryRow(context.Background(), `
		INSERT INTO users (firebase_uid, email)
		VALUES ($1, $2)
		RETURNING id::text
	`, "firebase-repo-"+label, label+"@example.com").Scan(&id); err != nil {
		t.Fatalf("seed user %s: %v", label, err)
	}
	return id
}

func seedRepoCircle(t *testing.T, repo *Repository, teacherID string) string {
	t.Helper()
	var id string
	if err := repo.pool.QueryRow(context.Background(), `
		INSERT INTO circles (name, teacher_id, invite_code)
		VALUES ('Repo Circle', $1::uuid, 'HLQ-REPO01')
		RETURNING id::text
	`, teacherID).Scan(&id); err != nil {
		t.Fatalf("seed circle: %v", err)
	}
	return id
}

// startRepoSession creates, starts, and returns one active ad-hoc session.
func startRepoSession(t *testing.T, repo *Repository, circleID, teacherID string) Session {
	t.Helper()
	created, err := repo.CreateAdHocSession(context.Background(), circleID, teacherID)
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	started, err := repo.StartSession(context.Background(), created.ID, MediaRoomRef("room-"+created.ID))
	if err != nil {
		t.Fatalf("start session: %v", err)
	}
	return started
}

func TestSessionRepository_CreateAdHocSessionDefaults(t *testing.T) {
	repo := newSessionRepository(t)
	ctx := context.Background()
	teacher := seedRepoUser(t, repo, "creator")
	circle := seedRepoCircle(t, repo, teacher)

	created, err := repo.CreateAdHocSession(ctx, circle, teacher)
	if err != nil {
		t.Fatalf("create ad-hoc session: %v", err)
	}
	if created.Status != SessionStatusScheduled {
		t.Fatalf("status: got %q, want scheduled", created.Status)
	}
	if created.ScheduledAt != nil {
		t.Fatalf("ad-hoc session must have NULL scheduled_at, got %v", created.ScheduledAt)
	}
	if created.MediaMode != MediaModeAudioOnly {
		t.Fatalf("media mode: got %q, want audio_only", created.MediaMode)
	}
	if created.ParticipantCount != 0 || created.IsLocked {
		t.Fatalf("unexpected initial state: count=%d locked=%v", created.ParticipantCount, created.IsLocked)
	}
	if created.MediaRoomRef != "" || created.ActualStart != nil || created.ActualEnd != nil || created.EndReason != "" {
		t.Fatalf("new session must not carry activation or end facts: %+v", created)
	}
}

func TestSessionRepository_LifecycleCAS(t *testing.T) {
	repo := newSessionRepository(t)
	ctx := context.Background()
	teacher := seedRepoUser(t, repo, "lifecycle")
	circle := seedRepoCircle(t, repo, teacher)
	created, err := repo.CreateAdHocSession(ctx, circle, teacher)
	if err != nil {
		t.Fatalf("create session: %v", err)
	}

	started, err := repo.StartSession(ctx, created.ID, MediaRoomRef("room-lifecycle"))
	if err != nil {
		t.Fatalf("start session: %v", err)
	}
	if started.Status != SessionStatusActive || started.ActualStart == nil || started.MediaRoomRef != "room-lifecycle" {
		t.Fatalf("unexpected started state: %+v", started)
	}

	if _, err := repo.StartSession(ctx, created.ID, MediaRoomRef("room-second")); !errors.Is(err, ErrSessionAlreadyActive) {
		t.Fatalf("double start: got %v, want ErrSessionAlreadyActive", err)
	}

	ended, err := repo.EndSession(ctx, created.ID, EndReasonManual)
	if err != nil {
		t.Fatalf("end session: %v", err)
	}
	if ended.Status != SessionStatusEnded || ended.ActualEnd == nil || ended.EndReason != EndReasonManual {
		t.Fatalf("unexpected ended state: %+v", ended)
	}

	if _, err := repo.EndSession(ctx, created.ID, EndReasonManual); !errors.Is(err, ErrSessionAlreadyEnded) {
		t.Fatalf("double end: got %v, want ErrSessionAlreadyEnded", err)
	}
	if _, err := repo.StartSession(ctx, created.ID, MediaRoomRef("room-third")); !errors.Is(err, ErrSessionAlreadyEnded) {
		t.Fatalf("start after end: got %v, want ErrSessionAlreadyEnded", err)
	}
}

func TestSessionRepository_MissingSessionErrors(t *testing.T) {
	repo := newSessionRepository(t)
	ctx := context.Background()
	missing := "99999999-9999-4999-8999-999999999999"

	if _, err := repo.GetSession(ctx, missing); !errors.Is(err, ErrSessionNotFound) {
		t.Fatalf("get missing: got %v, want ErrSessionNotFound", err)
	}
	if _, err := repo.StartSession(ctx, missing, MediaRoomRef("room-x")); !errors.Is(err, ErrSessionNotFound) {
		t.Fatalf("start missing: got %v, want ErrSessionNotFound", err)
	}
	if _, err := repo.EndSession(ctx, missing, EndReasonManual); !errors.Is(err, ErrSessionNotFound) {
		t.Fatalf("end missing: got %v, want ErrSessionNotFound", err)
	}
	if _, err := repo.SetLock(ctx, missing, true); !errors.Is(err, ErrSessionNotFound) {
		t.Fatalf("lock missing: got %v, want ErrSessionNotFound", err)
	}
	if _, err := repo.JoinSession(ctx, missing, missing); !errors.Is(err, ErrSessionNotFound) {
		t.Fatalf("join missing: got %v, want ErrSessionNotFound", err)
	}
}

func TestSessionRepository_JoinIdempotencyAndRejoin(t *testing.T) {
	repo := newSessionRepository(t)
	ctx := context.Background()
	teacher := seedRepoUser(t, repo, "join")
	circle := seedRepoCircle(t, repo, teacher)
	member := seedRepoUser(t, repo, "member")
	session := startRepoSession(t, repo, circle, teacher)

	joined, err := repo.JoinSession(ctx, session.ID, member)
	if err != nil {
		t.Fatalf("join: %v", err)
	}
	if joined.ParticipantCount != 1 {
		t.Fatalf("count after join: got %d, want 1", joined.ParticipantCount)
	}
	rejoined, err := repo.JoinSession(ctx, session.ID, member)
	if err != nil {
		t.Fatalf("duplicate join: %v", err)
	}
	if rejoined.ParticipantCount != 1 {
		t.Fatalf("count after duplicate join: got %d, want 1", rejoined.ParticipantCount)
	}
	rows := repoPresenceRows(t, repo, session.ID)
	if len(rows) != 1 {
		t.Fatalf("presence rows after duplicate join: got %d, want 1", len(rows))
	}
	if !rows[0].IsCurrentlyPresent || rows[0].ReconnectCount != 0 || rows[0].FirstJoinedAt == nil || rows[0].LastLeftAt != nil {
		t.Fatalf("unexpected presence after duplicate join: %+v", rows[0])
	}

	left, err := repo.LeaveSession(ctx, session.ID, member)
	if err != nil {
		t.Fatalf("leave: %v", err)
	}
	if left.ParticipantCount != 0 {
		t.Fatalf("count after leave: got %d, want 0", left.ParticipantCount)
	}
	leftAgain, err := repo.LeaveSession(ctx, session.ID, member)
	if err != nil {
		t.Fatalf("duplicate leave: %v", err)
	}
	if leftAgain.ParticipantCount != 0 {
		t.Fatalf("count after duplicate leave: got %d, want 0", leftAgain.ParticipantCount)
	}
	rows = repoPresenceRows(t, repo, session.ID)
	if len(rows) != 1 || rows[0].IsCurrentlyPresent || rows[0].LastLeftAt == nil {
		t.Fatalf("unexpected presence after leave: %+v", rows)
	}

	again, err := repo.JoinSession(ctx, session.ID, member)
	if err != nil {
		t.Fatalf("rejoin: %v", err)
	}
	if again.ParticipantCount != 1 {
		t.Fatalf("count after rejoin: got %d, want 1", again.ParticipantCount)
	}
	rows = repoPresenceRows(t, repo, session.ID)
	if len(rows) != 1 || rows[0].ReconnectCount != 1 || !rows[0].IsCurrentlyPresent {
		t.Fatalf("unexpected presence after rejoin: %+v", rows[0])
	}
}

func TestSessionRepository_JoinStateValidation(t *testing.T) {
	repo := newSessionRepository(t)
	ctx := context.Background()
	teacher := seedRepoUser(t, repo, "validate")
	circle := seedRepoCircle(t, repo, teacher)
	member := seedRepoUser(t, repo, "validate-member")

	created, err := repo.CreateAdHocSession(ctx, circle, teacher)
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	if _, err := repo.JoinSession(ctx, created.ID, member); !errors.Is(err, ErrSessionNotStartable) {
		t.Fatalf("join scheduled: got %v, want ErrSessionNotStartable", err)
	}

	started := startRepoSession(t, repo, circle, teacher)
	if _, err := repo.JoinSession(ctx, started.ID, member); err != nil {
		t.Fatalf("join active: %v", err)
	}
	if _, err := repo.EndSession(ctx, started.ID, EndReasonManual); err != nil {
		t.Fatalf("end session: %v", err)
	}
	if _, err := repo.JoinSession(ctx, started.ID, member); !errors.Is(err, ErrSessionAlreadyEnded) {
		t.Fatalf("join ended: %v", err)
	}

	full := startRepoSession(t, repo, circle, teacher)
	if _, err := repo.pool.Exec(ctx, `UPDATE sessions SET participant_count = 50 WHERE id = $1::uuid`, full.ID); err != nil {
		t.Fatalf("fill session: %v", err)
	}
	if _, err := repo.JoinSession(ctx, full.ID, member); !errors.Is(err, ErrSessionFull) {
		t.Fatalf("join full: got %v, want ErrSessionFull", err)
	}

	locked := startRepoSession(t, repo, circle, teacher)
	if _, err := repo.SetLock(ctx, locked.ID, true); err != nil {
		t.Fatalf("lock session: %v", err)
	}
	if _, err := repo.JoinSession(ctx, locked.ID, member); !errors.Is(err, ErrSessionLocked) {
		t.Fatalf("join locked: got %v, want ErrSessionLocked", err)
	}

	removed := startRepoSession(t, repo, circle, teacher)
	if _, err := repo.JoinSession(ctx, removed.ID, member); err != nil {
		t.Fatalf("join before removal: %v", err)
	}
	if _, err := repo.RemoveParticipant(ctx, removed.ID, member); err != nil {
		t.Fatalf("remove participant: %v", err)
	}
	if _, err := repo.JoinSession(ctx, removed.ID, member); !errors.Is(err, ErrParticipantRemoved) {
		t.Fatalf("join removed: got %v, want ErrParticipantRemoved", err)
	}
}

func TestSessionRepository_ReconnectLockRules(t *testing.T) {
	repo := newSessionRepository(t)
	ctx := context.Background()
	teacher := seedRepoUser(t, repo, "reconnect")
	circle := seedRepoCircle(t, repo, teacher)
	preLock := seedRepoUser(t, repo, "pre-lock")
	outsider := seedRepoUser(t, repo, "outsider")
	session := startRepoSession(t, repo, circle, teacher)

	if _, err := repo.JoinSession(ctx, session.ID, preLock); err != nil {
		t.Fatalf("pre-lock join: %v", err)
	}
	if _, err := repo.LeaveSession(ctx, session.ID, preLock); err != nil {
		t.Fatalf("pre-lock leave: %v", err)
	}
	if _, err := repo.SetLock(ctx, session.ID, true); err != nil {
		t.Fatalf("lock session: %v", err)
	}

	reconnected, err := repo.ReconnectPresence(ctx, session.ID, preLock)
	if err != nil {
		t.Fatalf("locked reconnect of pre-lock participant: %v", err)
	}
	if reconnected.ParticipantCount != 1 {
		t.Fatalf("count after locked reconnect: got %d, want 1", reconnected.ParticipantCount)
	}
	if _, err := repo.JoinSession(ctx, session.ID, outsider); !errors.Is(err, ErrSessionLocked) {
		t.Fatalf("locked new join: got %v, want ErrSessionLocked", err)
	}
	if _, err := repo.ReconnectPresence(ctx, session.ID, outsider); !errors.Is(err, ErrSessionLocked) {
		t.Fatalf("locked reconnect without presence row: got %v, want ErrSessionLocked", err)
	}

	if _, err := repo.RemoveParticipant(ctx, session.ID, preLock); err != nil {
		t.Fatalf("remove locked participant: %v", err)
	}
	if _, err := repo.ReconnectPresence(ctx, session.ID, preLock); !errors.Is(err, ErrParticipantRemoved) {
		t.Fatalf("reconnect removed: got %v, want ErrParticipantRemoved", err)
	}

	if _, err := repo.SetLock(ctx, session.ID, false); err != nil {
		t.Fatalf("unlock session: %v", err)
	}
	if _, err := repo.JoinSession(ctx, session.ID, outsider); err != nil {
		t.Fatalf("join after unlock: %v", err)
	}

	ended := startRepoSession(t, repo, circle, teacher)
	if _, err := repo.JoinSession(ctx, ended.ID, preLock); err != nil {
		t.Fatalf("join for ended-case: %v", err)
	}
	if _, err := repo.EndSession(ctx, ended.ID, EndReasonIdleTimeout); err != nil {
		t.Fatalf("end session: %v", err)
	}
	if _, err := repo.ReconnectPresence(ctx, ended.ID, preLock); !errors.Is(err, ErrSessionAlreadyEnded) {
		t.Fatalf("reconnect ended: got %v, want ErrSessionAlreadyEnded", err)
	}

	scheduled, err := repo.CreateAdHocSession(ctx, circle, teacher)
	if err != nil {
		t.Fatalf("create scheduled session: %v", err)
	}
	if _, err := repo.ReconnectPresence(ctx, scheduled.ID, preLock); !errors.Is(err, ErrSessionNotStartable) {
		t.Fatalf("reconnect scheduled: got %v, want ErrSessionNotStartable", err)
	}
	if _, err := repo.ReconnectPresence(ctx, "99999999-9999-4999-8999-999999999999", preLock); !errors.Is(err, ErrSessionNotFound) {
		t.Fatalf("reconnect missing: got %v, want ErrSessionNotFound", err)
	}
}

func TestSessionRepository_SetLockCAS(t *testing.T) {
	repo := newSessionRepository(t)
	ctx := context.Background()
	teacher := seedRepoUser(t, repo, "lock")
	circle := seedRepoCircle(t, repo, teacher)
	member := seedRepoUser(t, repo, "lock-member")

	created, err := repo.CreateAdHocSession(ctx, circle, teacher)
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	if _, err := repo.SetLock(ctx, created.ID, true); !errors.Is(err, ErrSessionNotStartable) {
		t.Fatalf("lock scheduled: got %v, want ErrSessionNotStartable", err)
	}

	session := startRepoSession(t, repo, circle, teacher)
	locked, err := repo.SetLock(ctx, session.ID, true)
	if err != nil || !locked.IsLocked {
		t.Fatalf("lock active: err=%v locked=%v", err, locked.IsLocked)
	}
	replayed, err := repo.SetLock(ctx, session.ID, true)
	if err != nil || !replayed.IsLocked {
		t.Fatalf("replay lock: err=%v locked=%v", err, replayed.IsLocked)
	}
	if _, err := repo.SetLock(ctx, session.ID, false); err != nil {
		t.Fatalf("unlock: %v", err)
	}
	if _, err := repo.JoinSession(ctx, session.ID, member); err != nil {
		t.Fatalf("join after unlock: %v", err)
	}

	if _, err := repo.EndSession(ctx, session.ID, EndReasonManual); err != nil {
		t.Fatalf("end session: %v", err)
	}
	if _, err := repo.SetLock(ctx, session.ID, true); !errors.Is(err, ErrSessionAlreadyEnded) {
		t.Fatalf("lock ended: got %v, want ErrSessionAlreadyEnded", err)
	}
}

func TestSessionRepository_RemoveParticipant(t *testing.T) {
	repo := newSessionRepository(t)
	ctx := context.Background()
	teacher := seedRepoUser(t, repo, "remove")
	circle := seedRepoCircle(t, repo, teacher)
	member := seedRepoUser(t, repo, "remove-member")
	stranger := seedRepoUser(t, repo, "remove-stranger")
	session := startRepoSession(t, repo, circle, teacher)

	if _, err := repo.JoinSession(ctx, session.ID, member); err != nil {
		t.Fatalf("join: %v", err)
	}
	if err := repo.SetHandRaised(ctx, session.ID, member); err != nil {
		t.Fatalf("raise hand: %v", err)
	}

	after, err := repo.RemoveParticipant(ctx, session.ID, member)
	if err != nil {
		t.Fatalf("remove participant: %v", err)
	}
	if after.ParticipantCount != 0 {
		t.Fatalf("count after removal: got %d, want 0", after.ParticipantCount)
	}
	rows := repoPresenceRows(t, repo, session.ID)
	if len(rows) != 1 || rows[0].RemovedAt == nil || rows[0].IsCurrentlyPresent || rows[0].HandRaisedAt != nil || rows[0].LastLeftAt == nil {
		t.Fatalf("unexpected presence after removal: %+v", rows[0])
	}

	replay, err := repo.RemoveParticipant(ctx, session.ID, member)
	if err != nil {
		t.Fatalf("duplicate removal must converge: %v", err)
	}
	if replay.ParticipantCount != 0 {
		t.Fatalf("count after duplicate removal: got %d, want 0", replay.ParticipantCount)
	}
	if _, err := repo.RemoveParticipant(ctx, session.ID, stranger); !errors.Is(err, ErrParticipantRemoved) {
		t.Fatalf("remove stranger: got %v, want ErrParticipantRemoved", err)
	}
}

func TestSessionRepository_HandState(t *testing.T) {
	repo := newSessionRepository(t)
	ctx := context.Background()
	teacher := seedRepoUser(t, repo, "hand")
	circle := seedRepoCircle(t, repo, teacher)
	member := seedRepoUser(t, repo, "hand-member")
	stranger := seedRepoUser(t, repo, "hand-stranger")
	session := startRepoSession(t, repo, circle, teacher)

	if err := repo.SetHandRaised(ctx, session.ID, member); !errors.Is(err, ErrParticipantRemoved) {
		t.Fatalf("raise without join: got %v, want ErrParticipantRemoved", err)
	}

	if _, err := repo.JoinSession(ctx, session.ID, member); err != nil {
		t.Fatalf("join: %v", err)
	}
	if err := repo.SetHandRaised(ctx, session.ID, member); err != nil {
		t.Fatalf("raise hand: %v", err)
	}
	if err := repo.SetHandRaised(ctx, session.ID, member); err != nil {
		t.Fatalf("duplicate raise must converge: %v", err)
	}
	rows := repoPresenceRows(t, repo, session.ID)
	if len(rows) != 1 || rows[0].HandRaisedAt == nil {
		t.Fatalf("hand not raised after duplicate delivery: %+v", rows[0])
	}

	if err := repo.SetHandLowered(ctx, session.ID, member); err != nil {
		t.Fatalf("lower hand: %v", err)
	}
	if err := repo.SetHandLowered(ctx, session.ID, member); err != nil {
		t.Fatalf("duplicate lower must converge: %v", err)
	}
	rows = repoPresenceRows(t, repo, session.ID)
	if len(rows) != 1 || rows[0].HandRaisedAt != nil {
		t.Fatalf("hand not lowered after duplicate delivery: %+v", rows[0])
	}

	if err := repo.SetHandRaised(ctx, session.ID, stranger); !errors.Is(err, ErrParticipantRemoved) {
		t.Fatalf("raise as stranger: got %v, want ErrParticipantRemoved", err)
	}
	if _, err := repo.LeaveSession(ctx, session.ID, member); err != nil {
		t.Fatalf("leave: %v", err)
	}
	if err := repo.SetHandRaised(ctx, session.ID, member); !errors.Is(err, ErrParticipantRemoved) {
		t.Fatalf("raise after leave: got %v, want ErrParticipantRemoved", err)
	}
}

func TestSessionRepository_CapacityBoundary(t *testing.T) {
	repo := newSessionRepository(t)
	ctx := context.Background()
	teacher := seedRepoUser(t, repo, "capacity")
	circle := seedRepoCircle(t, repo, teacher)
	session := startRepoSession(t, repo, circle, teacher)

	for i := 0; i < maxParticipants; i++ {
		member := seedRepoUser(t, repo, fmt.Sprintf("capacity-%02d", i))
		if _, err := repo.JoinSession(ctx, session.ID, member); err != nil {
			t.Fatalf("join %d: %v", i, err)
		}
	}
	current, err := repo.GetSession(ctx, session.ID)
	if err != nil {
		t.Fatalf("get session: %v", err)
	}
	if current.ParticipantCount != maxParticipants {
		t.Fatalf("count at capacity: got %d, want %d", current.ParticipantCount, maxParticipants)
	}

	overflow := seedRepoUser(t, repo, "capacity-overflow")
	if _, err := repo.JoinSession(ctx, session.ID, overflow); !errors.Is(err, ErrSessionFull) {
		t.Fatalf("join over capacity: got %v, want ErrSessionFull", err)
	}
	if _, err := repo.ReconnectPresence(ctx, session.ID, overflow); !errors.Is(err, ErrSessionFull) {
		t.Fatalf("reconnect over capacity: got %v, want ErrSessionFull", err)
	}
}

func TestSessionRepository_DuplicateJoinAtCapacityIsNoOp(t *testing.T) {
	repo := newSessionRepository(t)
	ctx := context.Background()
	teacher := seedRepoUser(t, repo, "dup-join")
	circle := seedRepoCircle(t, repo, teacher)
	member := seedRepoUser(t, repo, "dup-join-member")
	session := startRepoSession(t, repo, circle, teacher)

	if _, err := repo.JoinSession(ctx, session.ID, member); err != nil {
		t.Fatalf("join: %v", err)
	}
	if _, err := repo.pool.Exec(ctx, `UPDATE sessions SET participant_count = 50 WHERE id = $1::uuid`, session.ID); err != nil {
		t.Fatalf("fill session: %v", err)
	}

	rejoined, err := repo.JoinSession(ctx, session.ID, member)
	if err != nil {
		t.Fatalf("duplicate join by already-present participant at capacity: got %v, want no-op", err)
	}
	if rejoined.ParticipantCount != maxParticipants {
		t.Fatalf("count after duplicate join at capacity: got %d, want %d", rejoined.ParticipantCount, maxParticipants)
	}
}

func TestSessionRepository_ReconnectPresentParticipantAtCapacitySucceeds(t *testing.T) {
	repo := newSessionRepository(t)
	ctx := context.Background()
	teacher := seedRepoUser(t, repo, "dup-reconnect")
	circle := seedRepoCircle(t, repo, teacher)
	member := seedRepoUser(t, repo, "dup-reconnect-member")
	session := startRepoSession(t, repo, circle, teacher)

	if _, err := repo.JoinSession(ctx, session.ID, member); err != nil {
		t.Fatalf("join: %v", err)
	}
	if _, err := repo.pool.Exec(ctx, `UPDATE sessions SET participant_count = 50 WHERE id = $1::uuid`, session.ID); err != nil {
		t.Fatalf("fill session: %v", err)
	}

	reconnected, err := repo.ReconnectPresence(ctx, session.ID, member)
	if err != nil {
		t.Fatalf("reconnect of currently-present participant at capacity: got %v, want success", err)
	}
	if reconnected.ParticipantCount != maxParticipants {
		t.Fatalf("count after reconnect at capacity: got %d, want %d", reconnected.ParticipantCount, maxParticipants)
	}
}

func TestSessionRepository_ConcurrentJoinAdmitsExactly50(t *testing.T) {
	repo := newSessionRepository(t)
	ctx := context.Background()
	teacher := seedRepoUser(t, repo, "race")
	circle := seedRepoCircle(t, repo, teacher)
	session := startRepoSession(t, repo, circle, teacher)

	const attempts = maxParticipants + 1
	userIDs := make([]string, attempts)
	for i := range attempts {
		userIDs[i] = seedRepoUser(t, repo, fmt.Sprintf("race-%02d", i))
	}

	results := make(chan error, attempts)
	var wg sync.WaitGroup
	for _, id := range userIDs {
		wg.Add(1)
		go func(userID string) {
			defer wg.Done()
			_, err := repo.JoinSession(ctx, session.ID, userID)
			results <- err
		}(id)
	}
	wg.Wait()
	close(results)

	admitted, rejected := 0, 0
	for err := range results {
		switch {
		case err == nil:
			admitted++
		case errors.Is(err, ErrSessionFull):
			rejected++
		default:
			t.Errorf("unexpected concurrent join error: %v", err)
		}
	}
	if admitted != maxParticipants || rejected != 1 {
		t.Fatalf("concurrent joins: admitted=%d rejected=%d, want %d admitted and 1 rejected", admitted, rejected, maxParticipants)
	}
	current, err := repo.GetSession(ctx, session.ID)
	if err != nil {
		t.Fatalf("get session: %v", err)
	}
	if current.ParticipantCount != maxParticipants {
		t.Fatalf("final count: got %d, want %d", current.ParticipantCount, maxParticipants)
	}
	if rows := repoPresenceRows(t, repo, session.ID); len(rows) != maxParticipants {
		t.Fatalf("presence rows: got %d, want %d", len(rows), maxParticipants)
	}
}

func TestSessionRepository_EndClearsTransientPresence(t *testing.T) {
	repo := newSessionRepository(t)
	ctx := context.Background()
	teacher := seedRepoUser(t, repo, "endclear")
	circle := seedRepoCircle(t, repo, teacher)
	member := seedRepoUser(t, repo, "endclear-member")
	session := startRepoSession(t, repo, circle, teacher)

	if _, err := repo.JoinSession(ctx, session.ID, member); err != nil {
		t.Fatalf("join: %v", err)
	}
	if err := repo.SetHandRaised(ctx, session.ID, member); err != nil {
		t.Fatalf("raise hand: %v", err)
	}
	if _, err := repo.LeaveSession(ctx, session.ID, member); err != nil {
		t.Fatalf("leave: %v", err)
	}
	if _, err := repo.JoinSession(ctx, session.ID, member); err != nil {
		t.Fatalf("rejoin: %v", err)
	}

	if _, err := repo.EndSession(ctx, session.ID, EndReasonDurationLimit); err != nil {
		t.Fatalf("end session: %v", err)
	}
	rows := repoPresenceRows(t, repo, session.ID)
	if len(rows) != 1 {
		t.Fatalf("presence rows after end: got %d, want 1", len(rows))
	}
	row := rows[0]
	if row.IsCurrentlyPresent || row.HandRaisedAt != nil {
		t.Fatalf("end must clear transient presence/hand state: %+v", row)
	}
	if row.FirstJoinedAt == nil || row.LastJoinedAt == nil || row.ReconnectCount != 1 {
		t.Fatalf("end must retain durable history: %+v", row)
	}
}

func TestSessionRepository_ListSessionParticipantsSnapshot(t *testing.T) {
	repo := newSessionRepository(t)
	ctx := context.Background()
	teacher := seedRepoUser(t, repo, "snapshot")
	circle := seedRepoCircle(t, repo, teacher)
	first := seedRepoUser(t, repo, "snapshot-a")
	second := seedRepoUser(t, repo, "snapshot-b")
	session := startRepoSession(t, repo, circle, teacher)

	for _, id := range []string{first, second} {
		if _, err := repo.JoinSession(ctx, session.ID, id); err != nil {
			t.Fatalf("join: %v", err)
		}
	}
	if err := repo.SetHandRaised(ctx, session.ID, second); err != nil {
		t.Fatalf("raise hand: %v", err)
	}

	participants, err := repo.ListSessionParticipants(ctx, session.ID)
	if err != nil {
		t.Fatalf("list participants: %v", err)
	}
	if len(participants) != 2 {
		t.Fatalf("participants: got %d, want 2", len(participants))
	}
	if participants[0].UserID != first || participants[1].UserID != second {
		t.Fatalf("participants must keep join order: got [%s %s]", participants[0].UserID, participants[1].UserID)
	}
	if !participants[0].IsCurrentlyPresent || participants[0].HandRaisedAt != nil {
		t.Fatalf("unexpected first participant projection: %+v", participants[0])
	}
	if !participants[1].IsCurrentlyPresent || participants[1].HandRaisedAt == nil {
		t.Fatalf("unexpected second participant projection: %+v", participants[1])
	}
	for _, p := range participants {
		if p.SessionID != session.ID || p.DisplayName != "Member" {
			t.Fatalf("unexpected participant projection: %+v", p)
		}
	}
}

// repoPresenceRows reads the durable presence rows for assertions.
func repoPresenceRows(t *testing.T, repo *Repository, sessionID string) []ParticipantPresence {
	t.Helper()
	rows, err := repo.ListSessionParticipants(context.Background(), sessionID)
	if err != nil {
		t.Fatalf("list presence rows: %v", err)
	}
	return rows
}
