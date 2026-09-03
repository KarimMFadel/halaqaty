//go:build integration

package queue

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var roundServiceMigrations = []string{
	"000010_auth_roles_profile.up.sql",
	"000011_auth_roles_profile_alignment.up.sql",
	"000012_auth_profiles_display_name.up.sql",
	"000013_create_circles.up.sql",
	"000014_circle_members_circle_fk.up.sql",
	"000015_circle_management.up.sql",
	"000016_live_sessions.up.sql",
	"000017_recitation_queue_system.up.sql",
}

func newRoundServiceRepository(t *testing.T) *Repository {
	t.Helper()
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		t.Fatal("DATABASE_URL is required for queue integration tests")
	}
	ctx := context.Background()
	schema := fmt.Sprintf("test_round_service_%d", time.Now().UnixNano())
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
	for _, name := range roundServiceMigrations {
		sql, err := os.ReadFile(filepath.Join(migrationsDir, name))
		if err != nil {
			t.Fatalf("read migration %s: %v", name, err)
		}
		if _, err := pool.Exec(ctx, string(sql)); err != nil {
			t.Fatalf("apply migration %s: %v", name, err)
		}
	}
	return NewQueueRepository(pool)
}

func rsSeedUser(t *testing.T, repo *Repository, label string) string {
	t.Helper()
	var id string
	if err := repo.pool.QueryRow(context.Background(), `
		INSERT INTO users (firebase_uid, email)
		VALUES ($1, $2)
		RETURNING id::text
	`, "firebase-round-"+label, label+"@example.com").Scan(&id); err != nil {
		t.Fatalf("seed user %s: %v", label, err)
	}
	return id
}

func rsSeedCircle(t *testing.T, repo *Repository, teacherID string) string {
	t.Helper()
	var id string
	if err := repo.pool.QueryRow(context.Background(), `
		INSERT INTO circles (name, teacher_id, invite_code)
		VALUES ('Round Service Circle', $1::uuid, 'HLQ-RNDSVC')
		RETURNING id::text
	`, teacherID).Scan(&id); err != nil {
		t.Fatalf("seed circle: %v", err)
	}
	return id
}

func rsSeedMember(t *testing.T, repo *Repository, circleID, userID, role string, joinedAt time.Time) {
	t.Helper()
	if _, err := repo.pool.Exec(context.Background(), `
		INSERT INTO circle_members (circle_id, user_id, role, joined_at)
		VALUES ($1::uuid, $2::uuid, $3, $4)
	`, circleID, userID, role, joinedAt); err != nil {
		t.Fatalf("seed member %s (%s): %v", userID, role, err)
	}
}

func rsInsertSession(t *testing.T, repo *Repository, circleID, creatorID string) string {
	t.Helper()
	var id string
	if err := repo.pool.QueryRow(context.Background(), `
		INSERT INTO sessions (circle_id, created_by)
		VALUES ($1::uuid, $2::uuid)
		RETURNING id::text
	`, circleID, creatorID).Scan(&id); err != nil {
		t.Fatalf("insert session: %v", err)
	}
	return id
}

func rsInsertPresence(t *testing.T, repo *Repository, sessionID, userID string, firstJoinedAt time.Time, present bool) {
	t.Helper()
	if _, err := repo.pool.Exec(context.Background(), `
		INSERT INTO session_participant_presence (session_id, user_id, first_joined_at, last_joined_at, is_currently_present)
		VALUES ($1::uuid, $2::uuid, $3, $3, $4)
	`, sessionID, userID, firstJoinedAt, present); err != nil {
		t.Fatalf("insert presence for %s: %v", userID, err)
	}
}

func rsSetSessionStatus(t *testing.T, repo *Repository, sessionID, status string) {
	t.Helper()
	if _, err := repo.pool.Exec(context.Background(), `
		UPDATE sessions
		SET status = $2::varchar,
		    actual_start = CASE WHEN $2::varchar = 'active' THEN COALESCE(actual_start, NOW()) ELSE actual_start END,
		    actual_end = CASE WHEN $2::varchar = 'ended' THEN COALESCE(actual_end, NOW()) ELSE actual_end END
		WHERE id = $1::uuid
	`, sessionID, status); err != nil {
		t.Fatalf("set session status %s: %v", status, err)
	}
}

func rsSetPopulationPolicy(t *testing.T, repo *Repository, sessionID string, policy PopulationPolicy) {
	t.Helper()
	if _, err := repo.pool.Exec(context.Background(), `
		UPDATE sessions
		SET queue_population_policy = $2
		WHERE id = $1::uuid
	`, sessionID, policy); err != nil {
		t.Fatalf("set population policy %s: %v", policy, err)
	}
}

func rsMustRound(t *testing.T, repo *Repository, roundID string) Round {
	t.Helper()
	round, err := scanRound(repo.pool.QueryRow(context.Background(), findQueueRoundQuery, roundID))
	if err != nil {
		t.Fatalf("load round %s: %v", roundID, err)
	}
	return round
}

func rsQueueState(t *testing.T, repo *Repository, roundID string) QueueState {
	t.Helper()
	state, err := repo.LoadQueueState(context.Background(), roundID, Viewer{UserID: "manager", IsManager: true})
	if err != nil {
		t.Fatalf("load queue state %s: %v", roundID, err)
	}
	return state
}

func rsRoundNumbersByLifecycle(t *testing.T, repo *Repository, sessionID string) map[RoundLifecycle][]int {
	t.Helper()
	rows, err := repo.pool.Query(context.Background(), `
		SELECT lifecycle, round_number
		FROM recitation_queue
		WHERE session_id = $1::uuid
		ORDER BY round_number
	`, sessionID)
	if err != nil {
		t.Fatalf("list rounds by lifecycle: %v", err)
	}
	defer rows.Close()
	got := map[RoundLifecycle][]int{}
	for rows.Next() {
		var lifecycle RoundLifecycle
		var number int
		if err := rows.Scan(&lifecycle, &number); err != nil {
			t.Fatalf("scan round lifecycle: %v", err)
		}
		got[lifecycle] = append(got[lifecycle], number)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate rounds by lifecycle: %v", err)
	}
	return got
}

func rsEntryIDsByPosition(state QueueState) []string {
	ids := make([]string, 0, len(state.Entries))
	for _, entry := range state.Entries {
		ids = append(ids, entry.StudentID)
	}
	return ids
}

func rsSortedIDs(ids ...string) []string {
	sorted := append([]string(nil), ids...)
	sort.Strings(sorted)
	return sorted
}

func rsRoundEntryByStudent(t *testing.T, state QueueState, studentID string) QueueEntry {
	t.Helper()
	for _, entry := range state.Entries {
		if entry.StudentID == studentID {
			return entry
		}
	}
	t.Fatalf("entry for student %s not found", studentID)
	return QueueEntry{}
}

func rsTransitionEntryStatus(t *testing.T, repo *Repository, entryID string, target EntryStatus) {
	t.Helper()
	ctx := context.Background()
	err := repo.WithTx(ctx, func(tx *Tx) error {
		entry, err := tx.LockEntry(ctx, entryID)
		if err != nil {
			return err
		}
		_, err = tx.TransitionEntry(ctx, entry.ID, entry.Status, entry.Version, target, nil, nil, nil)
		return err
	})
	if err != nil {
		t.Fatalf("transition entry %s to %s: %v", entryID, target, err)
	}
}

func roundServiceQueueErrCode(t *testing.T, err error) QueueErrorCode {
	t.Helper()
	var qerr *QueueError
	if !errors.As(err, &qerr) {
		t.Fatalf("expected *QueueError, got %T: %v", err, err)
	}
	return qerr.Code
}

func rsLockSession(t *testing.T, repo *Repository, sessionID string) pgx.Tx {
	t.Helper()
	tx, err := repo.pool.Begin(context.Background())
	if err != nil {
		t.Fatalf("begin session lock transaction: %v", err)
	}
	t.Cleanup(func() { _ = tx.Rollback(context.Background()) })
	if _, err := tx.Exec(context.Background(), `
		SELECT id FROM sessions WHERE id = $1::uuid FOR UPDATE
	`, sessionID); err != nil {
		t.Fatalf("lock session row: %v", err)
	}
	return tx
}

func rsWaitForQueueTransaction(t *testing.T, repo *Repository) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for repo.pool.Stat().AcquiredConns() < 2 {
		if time.Now().After(deadline) {
			t.Fatal("queue mutation transaction did not acquire a connection")
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestRoundServicePrepareScheduledAndLive(t *testing.T) {
	t.Run("scheduled prepare stacks without activation", func(t *testing.T) {
		repo := newRoundServiceRepository(t)
		service := NewRoundService(repo)
		teacher := rsSeedUser(t, repo, "scheduled-teacher")
		circle := rsSeedCircle(t, repo, teacher)
		session := rsInsertSession(t, repo, circle, teacher)

		first, err := service.Prepare(context.Background(), PrepareRoundInput{
			SessionID:       session,
			Type:            RoundTypeRevision,
			SurahID:         2,
			FromAyah:        1,
			ToAyah:          5,
			SurahAyahCount:  286,
			GradingRequired: true,
			CreatedBy:       teacher,
		})
		if err != nil {
			t.Fatalf("prepare first scheduled round: %v", err)
		}
		second, err := service.Prepare(context.Background(), PrepareRoundInput{
			SessionID:       session,
			Type:            RoundTypeOldRevision,
			SurahID:         2,
			FromAyah:        6,
			ToAyah:          10,
			SurahAyahCount:  286,
			GradingRequired: false,
			CreatedBy:       teacher,
		})
		if err != nil {
			t.Fatalf("prepare second scheduled round: %v", err)
		}

		if first.RoundNumber != 1 || second.RoundNumber != 2 {
			t.Fatalf("scheduled round numbers = (%d,%d), want (1,2)", first.RoundNumber, second.RoundNumber)
		}
		rounds := rsRoundNumbersByLifecycle(t, repo, session)
		if got := rounds[RoundLifecyclePrepared]; len(got) != 2 || got[0] != 1 || got[1] != 2 {
			t.Fatalf("prepared rounds = %v, want [1 2]", got)
		}
		if got := rounds[RoundLifecycleActive]; len(got) != 0 {
			t.Fatalf("active rounds = %v, want none for scheduled session", got)
		}
	})

	t.Run("live prepare activates lowest prepared and stacks later rounds", func(t *testing.T) {
		repo := newRoundServiceRepository(t)
		service := NewRoundService(repo)
		base := time.Now().UTC().Add(-time.Hour)
		teacher := rsSeedUser(t, repo, "live-teacher")
		studentA := rsSeedUser(t, repo, "live-student-a")
		studentB := rsSeedUser(t, repo, "live-student-b")
		circle := rsSeedCircle(t, repo, teacher)
		rsSeedMember(t, repo, circle, teacher, "teacher", base)
		rsSeedMember(t, repo, circle, studentA, "student", base.Add(time.Minute))
		rsSeedMember(t, repo, circle, studentB, "student", base.Add(2*time.Minute))
		session := rsInsertSession(t, repo, circle, teacher)
		rsInsertPresence(t, repo, session, studentA, base.Add(10*time.Minute), true)
		rsInsertPresence(t, repo, session, studentB, base.Add(11*time.Minute), true)
		rsSetSessionStatus(t, repo, session, "active")

		first, err := service.Prepare(context.Background(), PrepareRoundInput{
			SessionID:       session,
			Type:            RoundTypeRevision,
			SurahID:         1,
			FromAyah:        1,
			ToAyah:          7,
			SurahAyahCount:  7,
			GradingRequired: true,
			CreatedBy:       teacher,
			Preorder:        []string{studentB},
		})
		if err != nil {
			t.Fatalf("prepare first live round: %v", err)
		}
		second, err := service.Prepare(context.Background(), PrepareRoundInput{
			SessionID:       session,
			Type:            RoundTypeTest,
			SurahID:         2,
			FromAyah:        1,
			ToAyah:          3,
			SurahAyahCount:  286,
			GradingRequired: false,
			CreatedBy:       teacher,
		})
		if err != nil {
			t.Fatalf("prepare second live round: %v", err)
		}

		active := rsMustRound(t, repo, first.ID)
		if active.Lifecycle != RoundLifecycleActive {
			t.Fatalf("first live round lifecycle = %s, want active", active.Lifecycle)
		}
		if active.ActivatedAt == nil {
			t.Fatal("first live round activated_at is nil")
		}
		state := rsQueueState(t, repo, first.ID)
		if got := rsEntryIDsByPosition(state); len(got) != 2 || got[0] != studentB || got[1] != studentA {
			t.Fatalf("active round entries = %v, want [%s %s]", got, studentB, studentA)
		}
		prepared := rsMustRound(t, repo, second.ID)
		if prepared.Lifecycle != RoundLifecyclePrepared {
			t.Fatalf("second live round lifecycle = %s, want prepared", prepared.Lifecycle)
		}
	})
}

func TestRoundServicePrepareRejectsEndedSessionAfterLifecycleRace(t *testing.T) {
	repo := newRoundServiceRepository(t)
	service := NewRoundService(repo)
	teacher := rsSeedUser(t, repo, "prepare-ended-race-teacher")
	circle := rsSeedCircle(t, repo, teacher)
	session := rsInsertSession(t, repo, circle, teacher)
	lock := rsLockSession(t, repo, session)

	result := make(chan error, 1)
	go func() {
		_, err := service.Prepare(context.Background(), PrepareRoundInput{
			SessionID: session, Type: RoundTypeRevision, SurahID: 1, FromAyah: 1,
			ToAyah: 7, SurahAyahCount: 7, GradingRequired: true, CreatedBy: teacher,
		})
		result <- err
	}()
	rsWaitForQueueTransaction(t, repo)
	if _, err := lock.Exec(context.Background(), `UPDATE sessions SET status = 'ended' WHERE id = $1::uuid`, session); err != nil {
		t.Fatalf("end session while prepare waits: %v", err)
	}
	if err := lock.Commit(context.Background()); err != nil {
		t.Fatalf("commit session end: %v", err)
	}

	if err := <-result; roundServiceQueueErrCode(t, err) != QueueErrorCodeRoundFinalized {
		t.Fatalf("prepare ended-session error = %v, want round_finalized", err)
	}
	var roundCount int
	if err := repo.pool.QueryRow(context.Background(), `SELECT COUNT(*) FROM recitation_queue WHERE session_id = $1::uuid`, session).Scan(&roundCount); err != nil {
		t.Fatalf("count rounds after rejected prepare: %v", err)
	}
	if roundCount != 0 {
		t.Fatalf("round count after rejected prepare = %d, want 0", roundCount)
	}
}

func TestRoundServiceResetAndActivationRejectEndedSessionWithoutMutation(t *testing.T) {
	tests := []struct {
		name string
		run  func(*testing.T, *Repository, *RoundService, string, string)
	}{
		{
			name: "reset",
			run: func(t *testing.T, repo *Repository, service *RoundService, session, teacher string) {
				rsSetSessionStatus(t, repo, session, "active")
				if _, err := service.Prepare(context.Background(), PrepareRoundInput{SessionID: session, Type: RoundTypeRevision, SurahID: 1, FromAyah: 1, ToAyah: 7, SurahAyahCount: 7, GradingRequired: true, CreatedBy: teacher}); err != nil {
					t.Fatalf("prepare active round: %v", err)
				}
				lock := rsLockSession(t, repo, session)
				result := make(chan error, 1)
				go func() {
					_, err := service.Reset(context.Background(), PrepareRoundInput{SessionID: session, Type: RoundTypeTest, SurahID: 2, FromAyah: 1, ToAyah: 3, SurahAyahCount: 286, GradingRequired: false, CreatedBy: teacher})
					result <- err
				}()
				rsWaitForQueueTransaction(t, repo)
				if _, err := lock.Exec(context.Background(), `UPDATE sessions SET status = 'ended' WHERE id = $1::uuid`, session); err != nil {
					t.Fatalf("end session while reset waits: %v", err)
				}
				if err := lock.Commit(context.Background()); err != nil {
					t.Fatalf("commit session end: %v", err)
				}
				if err := <-result; roundServiceQueueErrCode(t, err) != QueueErrorCodeRoundFinalized {
					t.Fatalf("reset ended-session error = %v, want round_finalized", err)
				}
				var roundCount int
				if err := repo.pool.QueryRow(context.Background(), `SELECT COUNT(*) FROM recitation_queue WHERE session_id = $1::uuid`, session).Scan(&roundCount); err != nil {
					t.Fatalf("count rounds after rejected reset: %v", err)
				}
				if roundCount != 1 {
					t.Fatalf("round count after rejected reset = %d, want 1", roundCount)
				}
			},
		},
		{
			name: "activation",
			run: func(t *testing.T, repo *Repository, service *RoundService, session, teacher string) {
				round, err := service.Prepare(context.Background(), PrepareRoundInput{SessionID: session, Type: RoundTypeRevision, SurahID: 1, FromAyah: 1, ToAyah: 7, SurahAyahCount: 7, GradingRequired: true, CreatedBy: teacher})
				if err != nil {
					t.Fatalf("prepare scheduled round: %v", err)
				}
				rsSetSessionStatus(t, repo, session, "ended")
				if err := service.ActivateIfNeeded(context.Background(), session); roundServiceQueueErrCode(t, err) != QueueErrorCodeRoundFinalized {
					t.Fatalf("activate ended-session error = %v, want round_finalized", err)
				}
				if got := rsMustRound(t, repo, round.ID).Lifecycle; got != RoundLifecyclePrepared {
					t.Fatalf("round lifecycle after rejected activation = %s, want prepared", got)
				}
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			repo := newRoundServiceRepository(t)
			teacher := rsSeedUser(t, repo, "ended-"+test.name+"-teacher")
			circle := rsSeedCircle(t, repo, teacher)
			session := rsInsertSession(t, repo, circle, teacher)
			test.run(t, repo, NewRoundService(repo), session, teacher)
		})
	}
}

func TestRoundServicePreorderRequiresActiveCircleStudent(t *testing.T) {
	repo := newRoundServiceRepository(t)
	service := NewRoundService(repo)
	teacher := rsSeedUser(t, repo, "preorder-auth-teacher")
	student := rsSeedUser(t, repo, "preorder-auth-student")
	supervisor := rsSeedUser(t, repo, "preorder-auth-supervisor")
	circle := rsSeedCircle(t, repo, teacher)
	now := time.Now().UTC()
	rsSeedMember(t, repo, circle, teacher, "teacher", now)
	rsSeedMember(t, repo, circle, student, "student", now.Add(time.Minute))
	rsSeedMember(t, repo, circle, supervisor, "supervisor", now.Add(2*time.Minute))
	session := rsInsertSession(t, repo, circle, teacher)

	if _, err := service.Prepare(context.Background(), PrepareRoundInput{SessionID: session, Type: RoundTypeRevision, SurahID: 1, FromAyah: 1, ToAyah: 7, SurahAyahCount: 7, GradingRequired: true, CreatedBy: teacher, Preorder: []string{supervisor}}); roundServiceQueueErrCode(t, err) != QueueErrorCodeValidation {
		t.Fatalf("manager preorder error = %v, want validation", err)
	}

	round, err := service.Prepare(context.Background(), PrepareRoundInput{SessionID: session, Type: RoundTypeRevision, SurahID: 1, FromAyah: 1, ToAyah: 7, SurahAyahCount: 7, GradingRequired: true, CreatedBy: teacher, Preorder: []string{student}})
	if err != nil {
		t.Fatalf("prepare valid preorder: %v", err)
	}
	if _, err := service.Reorder(context.Background(), round.ID, teacher, round.Version, []string{supervisor}); roundServiceQueueErrCode(t, err) != QueueErrorCodeValidation {
		t.Fatalf("manager reorder error = %v, want validation", err)
	}
	state := rsQueueState(t, repo, round.ID)
	if len(state.Preorder) != 1 || state.Preorder[0].StudentID != student {
		t.Fatalf("preorder after rejected reorder = %+v, want active student only", state.Preorder)
	}
}

func TestRoundServicePrepareUsesStackedMaxPlusOneNumbering(t *testing.T) {
	repo := newRoundServiceRepository(t)
	service := NewRoundService(repo)
	teacher := rsSeedUser(t, repo, "numbering-teacher")
	circle := rsSeedCircle(t, repo, teacher)
	session := rsInsertSession(t, repo, circle, teacher)

	const parallelRounds = 4
	results := make(chan Round, parallelRounds)
	errs := make(chan error, parallelRounds)
	var wg sync.WaitGroup
	for i := 0; i < parallelRounds; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			round, err := service.Prepare(context.Background(), PrepareRoundInput{
				SessionID:       session,
				Type:            RoundTypeRevision,
				SurahID:         2,
				FromAyah:        i + 1,
				ToAyah:          i + 1,
				SurahAyahCount:  286,
				GradingRequired: true,
				CreatedBy:       teacher,
			})
			if err != nil {
				errs <- err
				return
			}
			results <- round
		}(i)
	}
	wg.Wait()
	close(results)
	close(errs)

	for err := range errs {
		t.Fatalf("concurrent prepare failed: %v", err)
	}
	var numbers []int
	for round := range results {
		numbers = append(numbers, round.RoundNumber)
	}
	sort.Ints(numbers)
	if len(numbers) != parallelRounds {
		t.Fatalf("prepared rounds = %d, want %d", len(numbers), parallelRounds)
	}
	for i, number := range numbers {
		if want := i + 1; number != want {
			t.Fatalf("round numbers = %v, want sequential [1..%d]", numbers, parallelRounds)
		}
	}
}

func TestRoundServiceActivationInvariantAndPopulationPolicies(t *testing.T) {
	t.Run("present_at_activation orders preset present first then present join order with uuid tie break", func(t *testing.T) {
		repo := newRoundServiceRepository(t)
		service := NewRoundService(repo)
		base := time.Now().UTC().Add(-2 * time.Hour)
		teacher := rsSeedUser(t, repo, "present-teacher")
		supervisor := rsSeedUser(t, repo, "present-supervisor")
		s1 := rsSeedUser(t, repo, "present-s1")
		s2 := rsSeedUser(t, repo, "present-s2")
		s3 := rsSeedUser(t, repo, "present-s3")
		absentPreset := rsSeedUser(t, repo, "present-absent")
		circle := rsSeedCircle(t, repo, teacher)
		rsSeedMember(t, repo, circle, teacher, "teacher", base)
		rsSeedMember(t, repo, circle, supervisor, "supervisor", base.Add(time.Minute))
		rsSeedMember(t, repo, circle, s1, "student", base.Add(2*time.Minute))
		rsSeedMember(t, repo, circle, s2, "student", base.Add(3*time.Minute))
		rsSeedMember(t, repo, circle, s3, "student", base.Add(4*time.Minute))
		rsSeedMember(t, repo, circle, absentPreset, "student", base.Add(5*time.Minute))
		session := rsInsertSession(t, repo, circle, teacher)
		sharedJoin := base.Add(30 * time.Minute)
		rsInsertPresence(t, repo, session, teacher, base.Add(20*time.Minute), true)
		rsInsertPresence(t, repo, session, supervisor, base.Add(21*time.Minute), true)
		rsInsertPresence(t, repo, session, s1, sharedJoin, true)
		rsInsertPresence(t, repo, session, s2, sharedJoin, true)
		rsInsertPresence(t, repo, session, s3, base.Add(29*time.Minute), true)

		round, err := service.Prepare(context.Background(), PrepareRoundInput{
			SessionID:       session,
			Type:            RoundTypeRevision,
			SurahID:         1,
			FromAyah:        1,
			ToAyah:          7,
			SurahAyahCount:  7,
			GradingRequired: true,
			CreatedBy:       teacher,
			Preorder:        []string{s3, absentPreset},
		})
		if err != nil {
			t.Fatalf("prepare present_at_activation round: %v", err)
		}
		rsSetSessionStatus(t, repo, session, "active")
		if err := service.ActivateIfNeeded(context.Background(), session); err != nil {
			t.Fatalf("activate present_at_activation round: %v", err)
		}

		activated := rsMustRound(t, repo, round.ID)
		if activated.Lifecycle != RoundLifecycleActive {
			t.Fatalf("activated lifecycle = %s, want active", activated.Lifecycle)
		}
		state := rsQueueState(t, repo, round.ID)
		want := append([]string{s3}, rsSortedIDs(s1, s2)...)
		if got := rsEntryIDsByPosition(state); len(got) != len(want) || strings.Join(got, ",") != strings.Join(want, ",") {
			t.Fatalf("present_at_activation entries = %v, want %v", got, want)
		}
		for _, forbidden := range []string{teacher, supervisor, absentPreset} {
			for _, got := range rsEntryIDsByPosition(state) {
				if got == forbidden {
					t.Fatalf("forbidden user %s materialized in active queue %v", forbidden, rsEntryIDsByPosition(state))
				}
			}
		}
	})

	t.Run("all_active_students orders preset first then circle join order with uuid tie break", func(t *testing.T) {
		repo := newRoundServiceRepository(t)
		service := NewRoundService(repo)
		base := time.Now().UTC().Add(-2 * time.Hour)
		teacher := rsSeedUser(t, repo, "all-teacher")
		supervisor := rsSeedUser(t, repo, "all-supervisor")
		s1 := rsSeedUser(t, repo, "all-s1")
		s2 := rsSeedUser(t, repo, "all-s2")
		s3 := rsSeedUser(t, repo, "all-s3")
		s4 := rsSeedUser(t, repo, "all-s4")
		circle := rsSeedCircle(t, repo, teacher)
		rsSeedMember(t, repo, circle, teacher, "teacher", base)
		rsSeedMember(t, repo, circle, supervisor, "supervisor", base.Add(time.Minute))
		sharedJoined := base.Add(3 * time.Minute)
		rsSeedMember(t, repo, circle, s1, "student", sharedJoined)
		rsSeedMember(t, repo, circle, s2, "student", sharedJoined)
		rsSeedMember(t, repo, circle, s3, "student", base.Add(2*time.Minute))
		rsSeedMember(t, repo, circle, s4, "student", base.Add(4*time.Minute))
		session := rsInsertSession(t, repo, circle, teacher)
		rsSetPopulationPolicy(t, repo, session, PopulationPolicyAllActiveStudents)

		round, err := service.Prepare(context.Background(), PrepareRoundInput{
			SessionID:       session,
			Type:            RoundTypeRevision,
			SurahID:         1,
			FromAyah:        1,
			ToAyah:          7,
			SurahAyahCount:  7,
			GradingRequired: true,
			CreatedBy:       teacher,
			Preorder:        []string{s3},
		})
		if err != nil {
			t.Fatalf("prepare all_active_students round: %v", err)
		}
		rsSetSessionStatus(t, repo, session, "active")
		if err := service.ActivateIfNeeded(context.Background(), session); err != nil {
			t.Fatalf("activate all_active_students round: %v", err)
		}

		state := rsQueueState(t, repo, round.ID)
		want := append([]string{s3}, append(rsSortedIDs(s1, s2), s4)...)
		if got := rsEntryIDsByPosition(state); len(got) != len(want) || strings.Join(got, ",") != strings.Join(want, ",") {
			t.Fatalf("all_active_students entries = %v, want %v", got, want)
		}
		for _, got := range rsEntryIDsByPosition(state) {
			if got == teacher || got == supervisor {
				t.Fatalf("manager user %s materialized in all_active_students queue %v", got, rsEntryIDsByPosition(state))
			}
		}
	})

	t.Run("empty student population still activates round", func(t *testing.T) {
		repo := newRoundServiceRepository(t)
		service := NewRoundService(repo)
		base := time.Now().UTC().Add(-time.Hour)
		teacher := rsSeedUser(t, repo, "empty-teacher")
		supervisor := rsSeedUser(t, repo, "empty-supervisor")
		circle := rsSeedCircle(t, repo, teacher)
		rsSeedMember(t, repo, circle, teacher, "teacher", base)
		rsSeedMember(t, repo, circle, supervisor, "supervisor", base.Add(time.Minute))
		session := rsInsertSession(t, repo, circle, teacher)
		rsInsertPresence(t, repo, session, teacher, base.Add(2*time.Minute), true)
		rsInsertPresence(t, repo, session, supervisor, base.Add(3*time.Minute), true)

		round, err := service.Prepare(context.Background(), PrepareRoundInput{
			SessionID:       session,
			Type:            RoundTypeRevision,
			SurahID:         1,
			FromAyah:        1,
			ToAyah:          7,
			SurahAyahCount:  7,
			GradingRequired: true,
			CreatedBy:       teacher,
		})
		if err != nil {
			t.Fatalf("prepare empty population round: %v", err)
		}
		rsSetSessionStatus(t, repo, session, "active")
		if err := service.ActivateIfNeeded(context.Background(), session); err != nil {
			t.Fatalf("activate empty population round: %v", err)
		}

		activated := rsMustRound(t, repo, round.ID)
		if activated.Lifecycle != RoundLifecycleActive {
			t.Fatalf("empty population lifecycle = %s, want active", activated.Lifecycle)
		}
		state := rsQueueState(t, repo, round.ID)
		if len(state.Entries) != 0 {
			t.Fatalf("empty population entries = %v, want none", rsEntryIDsByPosition(state))
		}
	})
}

func TestRoundServiceReorderAndMoveConstraints(t *testing.T) {
	repo := newRoundServiceRepository(t)
	service := NewRoundService(repo)
	base := time.Now().UTC().Add(-time.Hour)
	teacher := rsSeedUser(t, repo, "move-teacher")
	s1 := rsSeedUser(t, repo, "move-s1")
	s2 := rsSeedUser(t, repo, "move-s2")
	s3 := rsSeedUser(t, repo, "move-s3")
	circle := rsSeedCircle(t, repo, teacher)
	rsSeedMember(t, repo, circle, teacher, "teacher", base)
	rsSeedMember(t, repo, circle, s1, "student", base.Add(time.Minute))
	rsSeedMember(t, repo, circle, s2, "student", base.Add(2*time.Minute))
	rsSeedMember(t, repo, circle, s3, "student", base.Add(3*time.Minute))
	session := rsInsertSession(t, repo, circle, teacher)
	rsInsertPresence(t, repo, session, s1, base.Add(10*time.Minute), true)
	rsInsertPresence(t, repo, session, s2, base.Add(11*time.Minute), true)
	rsInsertPresence(t, repo, session, s3, base.Add(12*time.Minute), true)

	round, err := service.Prepare(context.Background(), PrepareRoundInput{
		SessionID:       session,
		Type:            RoundTypeRevision,
		SurahID:         1,
		FromAyah:        1,
		ToAyah:          7,
		SurahAyahCount:  7,
		GradingRequired: true,
		CreatedBy:       teacher,
		Preorder:        []string{s1, s2, s3},
	})
	if err != nil {
		t.Fatalf("prepare reorder round: %v", err)
	}

	reordered, err := service.Reorder(context.Background(), round.ID, teacher, round.Version, []string{s3, s1, s2})
	if err != nil {
		t.Fatalf("reorder prepared round: %v", err)
	}
	if reordered.Version != round.Version+1 {
		t.Fatalf("reordered version = %d, want %d", reordered.Version, round.Version+1)
	}
	preparedState := rsQueueState(t, repo, round.ID)
	if len(preparedState.Preorder) != 3 || preparedState.Preorder[0].StudentID != s3 || preparedState.Preorder[1].StudentID != s1 || preparedState.Preorder[2].StudentID != s2 {
		t.Fatalf("prepared preorder = %+v, want [%s %s %s]", preparedState.Preorder, s3, s1, s2)
	}

	rsSetSessionStatus(t, repo, session, "active")
	if err := service.ActivateIfNeeded(context.Background(), session); err != nil {
		t.Fatalf("activate reordered round: %v", err)
	}
	activeRound := rsMustRound(t, repo, round.ID)
	activeState := rsQueueState(t, repo, round.ID)
	first := rsRoundEntryByStudent(t, activeState, s3)
	second := rsRoundEntryByStudent(t, activeState, s1)
	third := rsRoundEntryByStudent(t, activeState, s2)
	rsTransitionEntryStatus(t, repo, first.ID, EntryStatusReciting)

	moved, err := service.Move(context.Background(), third.ID, activeRound.Version, 2)
	if err != nil {
		t.Fatalf("move waiting entry while another recites: %v", err)
	}
	if moved.Position != 2 {
		t.Fatalf("moved entry position = %d, want 2", moved.Position)
	}
	movedState := rsQueueState(t, repo, round.ID)
	if got := rsEntryIDsByPosition(movedState); len(got) != 3 || got[0] != s3 || got[1] != s2 || got[2] != s1 {
		t.Fatalf("entries after move = %v, want [%s %s %s]", got, s3, s2, s1)
	}

	currentRound := rsMustRound(t, repo, round.ID)
	if _, err := service.Move(context.Background(), first.ID, currentRound.Version, 2); roundServiceQueueErrCode(t, err) != QueueErrorCodeInvalidTransition {
		t.Fatalf("move reciting entry error = %v, want invalid_transition", err)
	}

	rsTransitionEntryStatus(t, repo, second.ID, EntryStatusSkipped)
	currentRound = rsMustRound(t, repo, round.ID)
	if _, err := service.Move(context.Background(), second.ID, currentRound.Version, 1); roundServiceQueueErrCode(t, err) != QueueErrorCodeInvalidTransition {
		t.Fatalf("move terminal entry error = %v, want invalid_transition", err)
	}
}

func TestRoundServiceResetChainActivatesPreparedRoundsInRoundNumberOrder(t *testing.T) {
	repo := newRoundServiceRepository(t)
	service := NewRoundService(repo)
	base := time.Now().UTC().Add(-time.Hour)
	teacher := rsSeedUser(t, repo, "reset-teacher")
	student := rsSeedUser(t, repo, "reset-student")
	circle := rsSeedCircle(t, repo, teacher)
	rsSeedMember(t, repo, circle, teacher, "teacher", base)
	rsSeedMember(t, repo, circle, student, "student", base.Add(time.Minute))
	session := rsInsertSession(t, repo, circle, teacher)
	rsInsertPresence(t, repo, session, student, base.Add(2*time.Minute), true)
	rsSetSessionStatus(t, repo, session, "active")

	first, err := service.Prepare(context.Background(), PrepareRoundInput{
		SessionID:       session,
		Type:            RoundTypeRevision,
		SurahID:         1,
		FromAyah:        1,
		ToAyah:          7,
		SurahAyahCount:  7,
		GradingRequired: true,
		CreatedBy:       teacher,
	})
	if err != nil {
		t.Fatalf("prepare first reset round: %v", err)
	}
	second, err := service.Prepare(context.Background(), PrepareRoundInput{
		SessionID:       session,
		Type:            RoundTypeOldRevision,
		SurahID:         2,
		FromAyah:        1,
		ToAyah:          2,
		SurahAyahCount:  286,
		GradingRequired: false,
		CreatedBy:       teacher,
	})
	if err != nil {
		t.Fatalf("prepare second reset round: %v", err)
	}
	third, err := service.Prepare(context.Background(), PrepareRoundInput{
		SessionID:       session,
		Type:            RoundTypeTest,
		SurahID:         2,
		FromAyah:        3,
		ToAyah:          4,
		SurahAyahCount:  286,
		GradingRequired: false,
		CreatedBy:       teacher,
	})
	if err != nil {
		t.Fatalf("prepare third reset round: %v", err)
	}

	fourth, err := service.Reset(context.Background(), PrepareRoundInput{
		SessionID:       session,
		Type:            RoundTypeRevision,
		SurahID:         3,
		FromAyah:        1,
		ToAyah:          2,
		SurahAyahCount:  200,
		GradingRequired: true,
		CreatedBy:       teacher,
	})
	if err != nil {
		t.Fatalf("first reset: %v", err)
	}
	if fourth.RoundNumber != 4 {
		t.Fatalf("new round after first reset = %d, want 4", fourth.RoundNumber)
	}
	firstSnapshot := rsMustRound(t, repo, first.ID)
	if firstSnapshot.Lifecycle != RoundLifecycleFinalized {
		t.Fatalf("first round lifecycle after reset = %s, want finalized", firstSnapshot.Lifecycle)
	}
	if active := rsMustRound(t, repo, second.ID); active.Lifecycle != RoundLifecycleActive {
		t.Fatalf("second round lifecycle after first reset = %s, want active", active.Lifecycle)
	}
	if prepared := rsMustRound(t, repo, third.ID); prepared.Lifecycle != RoundLifecyclePrepared {
		t.Fatalf("third round lifecycle after first reset = %s, want prepared", prepared.Lifecycle)
	}
	if prepared := rsMustRound(t, repo, fourth.ID); prepared.Lifecycle != RoundLifecyclePrepared {
		t.Fatalf("fourth round lifecycle after first reset = %s, want prepared", prepared.Lifecycle)
	}

	fifth, err := service.Reset(context.Background(), PrepareRoundInput{
		SessionID:       session,
		Type:            RoundTypeRevision,
		SurahID:         4,
		FromAyah:        1,
		ToAyah:          2,
		SurahAyahCount:  176,
		GradingRequired: true,
		CreatedBy:       teacher,
	})
	if err != nil {
		t.Fatalf("second reset: %v", err)
	}
	if fifth.RoundNumber != 5 {
		t.Fatalf("new round after second reset = %d, want 5", fifth.RoundNumber)
	}
	secondSnapshot := rsMustRound(t, repo, second.ID)
	if secondSnapshot.Lifecycle != RoundLifecycleFinalized {
		t.Fatalf("second round lifecycle after second reset = %s, want finalized", secondSnapshot.Lifecycle)
	}
	if active := rsMustRound(t, repo, third.ID); active.Lifecycle != RoundLifecycleActive {
		t.Fatalf("third round lifecycle after second reset = %s, want active", active.Lifecycle)
	}
	if prepared := rsMustRound(t, repo, fourth.ID); prepared.Lifecycle != RoundLifecyclePrepared {
		t.Fatalf("fourth round lifecycle after second reset = %s, want prepared", prepared.Lifecycle)
	}
	if prepared := rsMustRound(t, repo, fifth.ID); prepared.Lifecycle != RoundLifecyclePrepared {
		t.Fatalf("fifth round lifecycle after second reset = %s, want prepared", prepared.Lifecycle)
	}
}

// TestRoundServiceResetActivatesSuccessorImmediately verifies that a reset
// immediately activates its successor without an audio-control barrier.
func TestRoundServiceResetActivatesSuccessorImmediately(t *testing.T) {
	repo := newRoundServiceRepository(t)
	rounds := NewRoundService(repo)
	turns := NewTurnService(repo)
	base := time.Now().UTC().Add(-time.Hour)
	teacher := rsSeedUser(t, repo, "reset-audio-teacher")
	student := rsSeedUser(t, repo, "reset-audio-student")
	circle := rsSeedCircle(t, repo, teacher)
	rsSeedMember(t, repo, circle, teacher, "teacher", base)
	rsSeedMember(t, repo, circle, student, "student", base.Add(time.Minute))
	session := rsInsertSession(t, repo, circle, teacher)
	rsInsertPresence(t, repo, session, student, base.Add(2*time.Minute), true)
	rsSetSessionStatus(t, repo, session, "active")

	first, err := rounds.Prepare(context.Background(), PrepareRoundInput{
		SessionID: session, Type: RoundTypeRevision, SurahID: 1, FromAyah: 1,
		ToAyah: 7, SurahAyahCount: 7, GradingRequired: true, CreatedBy: teacher,
	})
	if err != nil {
		t.Fatalf("prepare round: %v", err)
	}
	state := rsQueueState(t, repo, first.ID)
	entry := state.Entries[0]
	if _, err := turns.Advance(context.Background(), first.ID, state.Round.Version); err != nil {
		t.Fatalf("advance round: %v", err)
	}
	if _, err := turns.Start(context.Background(), entry.ID, entry.Version); err != nil {
		t.Fatalf("start reciter: %v", err)
	}

	reset, err := rounds.Reset(context.Background(), PrepareRoundInput{
		SessionID: session, Type: RoundTypeTest, SurahID: 2, FromAyah: 1,
		ToAyah: 2, SurahAyahCount: 286, GradingRequired: false, CreatedBy: teacher,
	})
	if err != nil {
		t.Fatalf("reset round: %v", err)
	}
	finalized := rsMustRound(t, repo, first.ID)
	if finalized.Lifecycle != RoundLifecycleFinalized || finalized.SelectedEntryID != nil {
		t.Fatalf("old round after reset = %+v, want finalized and unselected", finalized)
	}
	if current := rsMustRound(t, repo, reset.ID); current.Lifecycle != RoundLifecycleActive || current.ActivatedAt == nil {
		t.Fatalf("reset successor = %+v, want immediately active", current)
	}
	var audioIntentCount int
	if err := repo.pool.QueryRow(context.Background(), `
		SELECT COUNT(*)
		FROM queue_event_outbox
		WHERE session_id = $1::uuid AND event_type = 'queue.audio_revoke_before_activation'
	`, session).Scan(&audioIntentCount); err != nil {
		t.Fatalf("count reset audio intents: %v", err)
	}
	if audioIntentCount != 0 {
		t.Fatalf("reset audio intents = %d, want none", audioIntentCount)
	}
}
