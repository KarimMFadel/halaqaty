//go:build integration

package integration

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/KarimMFadel/halaqaty/backend/internal/queue"
	"github.com/jackc/pgx/v5/pgxpool"
)

type recitationQueueConcurrencyFixture struct {
	pool    *pgxpool.Pool
	repo    *queue.Repository
	teacher string
	circle  string
	session string
}

func newRecitationQueueConcurrencyFixture(t *testing.T) *recitationQueueConcurrencyFixture {
	t.Helper()
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		t.Skip("DATABASE_URL is not set")
	}

	ctx := context.Background()
	schema := fmt.Sprintf("test_recitation_queue_concurrency_%d", time.Now().UnixNano())
	separator := "?"
	if strings.Contains(dbURL, "?") {
		separator = "&"
	}
	pool, err := pgxpool.New(ctx, dbURL+separator+"search_path="+schema)
	if err != nil {
		t.Fatalf("open concurrency-test pool: %v", err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), "DROP SCHEMA IF EXISTS "+schema+" CASCADE")
		pool.Close()
	})
	if _, err := pool.Exec(ctx, "CREATE SCHEMA "+schema); err != nil {
		t.Fatalf("create concurrency-test schema: %v", err)
	}

	conn := acquireConn(t, pool, ctx)
	defer conn.Release()
	for _, migration := range recitationQueueHeadMigrations {
		runMigrationFile(t, conn, ctx, migration)
	}

	teacher := cqSeedUser(t, pool, "teacher")
	circle := cqSeedCircle(t, pool, teacher)
	session := cqSeedSession(t, pool, circle, teacher)
	cqSeedMember(t, pool, circle, teacher, "teacher", time.Now().UTC())
	if _, err := pool.Exec(ctx, `
		UPDATE sessions
		SET status = 'active', actual_start = NOW()
		WHERE id = $1::uuid
	`, session); err != nil {
		t.Fatalf("activate concurrency-test session: %v", err)
	}

	return &recitationQueueConcurrencyFixture{
		pool:    pool,
		repo:    queue.NewQueueRepository(pool),
		teacher: teacher,
		circle:  circle,
		session: session,
	}
}

func (f *recitationQueueConcurrencyFixture) addStudent(t *testing.T, label string, joinedAt time.Time) string {
	t.Helper()
	student := cqSeedUser(t, f.pool, label)
	cqSeedMember(t, f.pool, f.circle, student, "student", joinedAt)
	return student
}

func cqSeedUser(t *testing.T, pool *pgxpool.Pool, label string) string {
	t.Helper()
	var id string
	if err := pool.QueryRow(context.Background(), `
		INSERT INTO users (firebase_uid, email)
		VALUES ($1, $2)
		RETURNING id::text
	`, "firebase-concurrency-"+label, label+"@example.com").Scan(&id); err != nil {
		t.Fatalf("seed user %s: %v", label, err)
	}
	return id
}

func cqSeedCircle(t *testing.T, pool *pgxpool.Pool, teacherID string) string {
	t.Helper()
	var id string
	if err := pool.QueryRow(context.Background(), `
		INSERT INTO circles (name, teacher_id, invite_code)
		VALUES ('Concurrency Circle', $1::uuid, 'HLQ-CONCUR')
		RETURNING id::text
	`, teacherID).Scan(&id); err != nil {
		t.Fatalf("seed concurrency circle: %v", err)
	}
	return id
}

func cqSeedSession(t *testing.T, pool *pgxpool.Pool, circleID, teacherID string) string {
	t.Helper()
	var id string
	if err := pool.QueryRow(context.Background(), `
		INSERT INTO sessions (circle_id, created_by)
		VALUES ($1::uuid, $2::uuid)
		RETURNING id::text
	`, circleID, teacherID).Scan(&id); err != nil {
		t.Fatalf("seed concurrency session: %v", err)
	}
	return id
}

func cqSeedMember(t *testing.T, pool *pgxpool.Pool, circleID, userID, role string, joinedAt time.Time) {
	t.Helper()
	if _, err := pool.Exec(context.Background(), `
		INSERT INTO circle_members (circle_id, user_id, role, joined_at)
		VALUES ($1::uuid, $2::uuid, $3, $4)
	`, circleID, userID, role, joinedAt); err != nil {
		t.Fatalf("seed concurrency member %s: %v", userID, err)
	}
}

func cqCreateActiveRound(t *testing.T, f *recitationQueueConcurrencyFixture, students ...string) queue.Round {
	t.Helper()
	ctx := context.Background()
	var round queue.Round
	err := f.repo.WithTx(ctx, func(tx *queue.Tx) error {
		if err := tx.LockRoundAllocation(ctx, f.session); err != nil {
			return err
		}
		number, err := tx.NextRoundNumber(ctx, f.session)
		if err != nil {
			return err
		}
		round, err = tx.CreateRound(ctx, queue.NewRound{
			SessionID:       f.session,
			RoundNumber:     number,
			Type:            queue.RoundTypeRevision,
			SurahID:         1,
			FromAyah:        1,
			ToAyah:          7,
			GradingRequired: true,
			Lifecycle:       queue.RoundLifecycleActive,
			CreatedBy:       f.teacher,
		})
		if err != nil {
			return err
		}
		for position, student := range students {
			if _, err := tx.InsertQueueEntry(ctx, round.ID, student, position+1); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("create active concurrency round: %v", err)
	}
	return round
}

func cqSelectEntry(t *testing.T, f *recitationQueueConcurrencyFixture, roundID, entryID string, expectedVersion int64) queue.Round {
	t.Helper()
	var selected queue.Round
	err := f.repo.WithTx(context.Background(), func(tx *queue.Tx) error {
		var err error
		selected, err = tx.SetRoundSelection(context.Background(), roundID, &entryID, expectedVersion)
		return err
	})
	if err != nil {
		t.Fatalf("select concurrency entry: %v", err)
	}
	return selected
}

func cqState(t *testing.T, f *recitationQueueConcurrencyFixture, roundID string) queue.QueueState {
	t.Helper()
	state, err := f.repo.LoadQueueState(context.Background(), roundID, queue.Viewer{UserID: f.teacher, IsManager: true})
	if err != nil {
		t.Fatalf("load durable concurrency state: %v", err)
	}
	return state
}

func cqEntry(t *testing.T, state queue.QueueState, studentID string) queue.QueueEntry {
	t.Helper()
	for _, entry := range state.Entries {
		if entry.StudentID == studentID {
			return entry
		}
	}
	t.Fatalf("durable queue entry for student %s not found", studentID)
	return queue.QueueEntry{}
}

func cqEntryByID(t *testing.T, state queue.QueueState, entryID string) queue.QueueEntry {
	t.Helper()
	for _, entry := range state.Entries {
		if entry.ID == entryID {
			return entry
		}
	}
	t.Fatalf("durable queue entry %s not found", entryID)
	return queue.QueueEntry{}
}

func cqHasStudent(state queue.QueueState, studentID string) bool {
	for _, entry := range state.Entries {
		if entry.StudentID == studentID {
			return true
		}
	}
	return false
}

type cqRaceResult struct {
	operation int
	err       error
}

func cqRace(t *testing.T, operations ...func(context.Context) error) []cqRaceResult {
	t.Helper()
	start := make(chan struct{})
	results := make(chan cqRaceResult, len(operations))
	var wg sync.WaitGroup
	for operationIndex, operation := range operations {
		operationIndex := operationIndex
		operation := operation
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			results <- cqRaceResult{operation: operationIndex, err: operation(ctx)}
		}()
	}
	close(start)
	wg.Wait()
	close(results)

	collected := make([]cqRaceResult, 0, len(operations))
	for result := range results {
		collected = append(collected, result)
	}
	return collected
}

func cqQueueErrorCode(err error) (queue.QueueErrorCode, bool) {
	var queueErr *queue.QueueError
	if !errors.As(err, &queueErr) {
		return "", false
	}
	return queueErr.Code, true
}

func cqCountReciters(state queue.QueueState) int {
	count := 0
	for _, entry := range state.Entries {
		if entry.Status == queue.EntryStatusReciting {
			count++
		}
	}
	return count
}

func cqAssertContiguousPositions(t *testing.T, state queue.QueueState) {
	t.Helper()
	seen := make(map[int]struct{}, len(state.Entries))
	for _, entry := range state.Entries {
		if entry.Position < 1 {
			t.Fatalf("entry %s has non-positive position %d", entry.ID, entry.Position)
		}
		if _, exists := seen[entry.Position]; exists {
			t.Fatalf("duplicate durable position %d in %+v", entry.Position, state.Entries)
		}
		seen[entry.Position] = struct{}{}
	}
	for position := 1; position <= len(state.Entries); position++ {
		if _, exists := seen[position]; !exists {
			t.Fatalf("durable positions are not contiguous: %+v", state.Entries)
		}
	}
}

func cqCompleteRecitingEntry(ctx context.Context, f *recitationQueueConcurrencyFixture, roundID, entryID string, grade queue.Grade) error {
	return f.repo.WithTx(ctx, func(tx *queue.Tx) error {
		round, err := tx.LockRound(ctx, roundID)
		if err != nil {
			return err
		}
		if round.Lifecycle != queue.RoundLifecycleActive {
			return &queue.QueueError{Code: queue.QueueErrorCodeRoundFinalized, Message: "queue round is not active"}
		}
		entry, err := tx.LockEntry(ctx, entryID)
		if err != nil {
			return err
		}
		if entry.Status != queue.EntryStatusReciting {
			return &queue.QueueError{Code: queue.QueueErrorCodeInvalidTransition, Message: "entry is not reciting"}
		}
		note := "concurrency completion"
		if _, err := tx.TransitionEntry(ctx, entry.ID, queue.EntryStatusReciting, entry.Version, queue.EntryStatusCompleted, &grade, &note, &f.teacher); err != nil {
			return err
		}
		if err := tx.BumpRoundVersion(ctx, round.ID); err != nil {
			return err
		}
		return tx.UpsertProgress(ctx, queue.NewProgress{
			StudentID:    entry.StudentID,
			CircleID:     f.circle,
			SessionID:    f.session,
			QueueEntryID: entry.ID,
			SurahID:      1,
			SurahName:    "Al-Fatihah",
			FromAyah:     1,
			ToAyah:       7,
			Type:         queue.RoundTypeRevision,
			Grade:        &grade,
			Notes:        &note,
			Date:         time.Now().UTC(),
		})
	})
}

func cqOptOutEntry(ctx context.Context, f *recitationQueueConcurrencyFixture, roundID, entryID, studentID string, expectedEntryVersion int64) error {
	return f.repo.WithTx(ctx, func(tx *queue.Tx) error {
		round, err := tx.LockRound(ctx, roundID)
		if err != nil {
			return err
		}
		if round.Lifecycle != queue.RoundLifecycleActive {
			return &queue.QueueError{Code: queue.QueueErrorCodeRoundFinalized, Message: "queue round is not active"}
		}
		entry, err := tx.LockEntry(ctx, entryID)
		if err != nil {
			return err
		}
		// Entry-scoped guard mirroring EntryStatusRequest.expected_entry_version.
		if entry.Version != expectedEntryVersion {
			return &queue.QueueError{Code: queue.QueueErrorCodeStaleVersion, Message: "queue entry changed"}
		}
		if !queue.CanTransitionEntry(entry.Status, queue.EntryStatusOptedOut) {
			return &queue.QueueError{Code: queue.QueueErrorCodeEntryTerminal, Message: "entry is terminal"}
		}
		if _, err := tx.TransitionEntry(ctx, entry.ID, entry.Status, entry.Version, queue.EntryStatusOptedOut, nil, nil, &studentID); err != nil {
			return err
		}
		return tx.BumpRoundVersion(ctx, round.ID)
	})
}

func cqAppendLateJoin(ctx context.Context, f *recitationQueueConcurrencyFixture, roundID, studentID string, expectedVersion int64) error {
	tx, err := f.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin late-join append: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var lifecycle queue.RoundLifecycle
	var version int64
	if err := tx.QueryRow(ctx, `
		SELECT lifecycle, version
		FROM recitation_queue
		WHERE id = $1::uuid
		FOR UPDATE
	`, roundID).Scan(&lifecycle, &version); err != nil {
		return fmt.Errorf("lock late-join round: %w", err)
	}
	if lifecycle != queue.RoundLifecycleActive {
		return &queue.QueueError{Code: queue.QueueErrorCodeInvalidTransition, Message: "queue round is not active"}
	}
	if version != expectedVersion {
		return &queue.QueueError{Code: queue.QueueErrorCodeStaleVersion, Message: "queue state changed"}
	}
	var position int
	if err := tx.QueryRow(ctx, `
		SELECT COALESCE(MAX(position), 0) + 1
		FROM recitation_queue_entries
		WHERE queue_id = $1::uuid
	`, roundID).Scan(&position); err != nil {
		return fmt.Errorf("allocate late-join position: %w", err)
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO recitation_queue_entries (queue_id, student_id, position, status)
		VALUES ($1::uuid, $2::uuid, $3, 'waiting')
	`, roundID, studentID, position); err != nil {
		return fmt.Errorf("append late-join entry: %w", err)
	}
	if _, err := tx.Exec(ctx, `
		UPDATE recitation_queue
		SET version = version + 1
		WHERE id = $1::uuid AND version = $2
	`, roundID, expectedVersion); err != nil {
		return fmt.Errorf("bump late-join round version: %w", err)
	}
	return tx.Commit(ctx)
}

func TestRecitationQueueConcurrency_AdvanceAndStart(t *testing.T) {
	f := newRecitationQueueConcurrencyFixture(t)
	first := f.addStudent(t, "advance-start-first", time.Now().UTC())
	second := f.addStudent(t, "advance-start-second", time.Now().UTC().Add(time.Second))
	round := cqCreateActiveRound(t, f, first, second)
	state := cqState(t, f, round.ID)
	firstEntry := cqEntry(t, state, first)
	selected := cqSelectEntry(t, f, round.ID, firstEntry.ID, round.Version)
	turns := queue.NewTurnService(f.repo)

	results := cqRace(t,
		func(ctx context.Context) error {
			_, err := turns.Advance(ctx, round.ID, selected.Version)
			return err
		},
		func(ctx context.Context) error {
			_, err := turns.Start(ctx, firstEntry.ID, firstEntry.Version)
			return err
		},
	)

	// Advance (round token) and Start (entry token) guard different rows, so
	// both may legitimately commit when they target the same selected entry
	// (select-then-start is a consistent pair). The CHK032 invariants below
	// are what must hold: exactly one displayed reciter and a durable
	// selection that matches it.
	successes := 0
	for _, result := range results {
		err := result.err
		if err == nil {
			successes++
			continue
		}
		if code, ok := cqQueueErrorCode(err); !ok || (code != queue.QueueErrorCodeStaleVersion && code != queue.QueueErrorCodeEntryReciting && code != queue.QueueErrorCodeInvalidTransition) {
			t.Fatalf("advance/start race returned unexpected error: %v", err)
		}
	}
	if successes < 1 || successes > 2 {
		t.Fatalf("advance/start race successes = %d, want 1 or 2: %v", successes, results)
	}

	final := cqState(t, f, round.ID)
	if cqCountReciters(final) > 1 {
		t.Fatalf("advance/start race created multiple reciters: %+v", final.Entries)
	}
	if final.Round.Version <= selected.Version {
		t.Fatalf("advance/start final version = %d, want > %d", final.Round.Version, selected.Version)
	}
	if final.Round.SelectedEntryID == nil {
		t.Fatal("advance/start race lost the durable selection")
	}
	if firstFinal := cqEntryByID(t, final, firstEntry.ID); firstFinal.Status == queue.EntryStatusReciting && *final.Round.SelectedEntryID != firstEntry.ID {
		t.Fatalf("reciting entry is not the selected entry: round=%+v entry=%+v", final.Round, firstFinal)
	}
}

func TestRecitationQueueConcurrency_ResetAndComplete(t *testing.T) {
	f := newRecitationQueueConcurrencyFixture(t)
	student := f.addStudent(t, "reset-complete", time.Now().UTC())
	round := cqCreateActiveRound(t, f, student)
	entry := cqEntry(t, cqState(t, f, round.ID), student)
	turns := queue.NewTurnService(f.repo)
	if _, err := turns.Advance(context.Background(), round.ID, round.Version); err != nil {
		t.Fatalf("prepare reset/complete race selection: %v", err)
	}
	if _, err := turns.Start(context.Background(), entry.ID, entry.Version); err != nil {
		t.Fatalf("prepare reset/complete race start: %v", err)
	}
	before := cqState(t, f, round.ID)
	resetInput := queue.PrepareRoundInput{
		SessionID:       f.session,
		Type:            queue.RoundTypeRevision,
		SurahID:         1,
		FromAyah:        1,
		ToAyah:          7,
		SurahAyahCount:  7,
		GradingRequired: true,
		CreatedBy:       f.teacher,
	}
	rounds := queue.NewRoundService(f.repo)
	results := cqRace(t,
		func(ctx context.Context) error {
			_, err := rounds.Reset(ctx, resetInput)
			return err
		},
		func(ctx context.Context) error {
			return cqCompleteRecitingEntry(ctx, f, round.ID, entry.ID, queue.GradeGood)
		},
	)

	resetSuccesses := 0
	completeSuccesses := 0
	for _, result := range results {
		raceErr := result.err
		if raceErr == nil {
			if result.operation == 0 {
				resetSuccesses++
			} else {
				completeSuccesses++
			}
			continue
		}
		if code, ok := cqQueueErrorCode(raceErr); !ok || (code != queue.QueueErrorCodeRoundFinalized && code != queue.QueueErrorCodeInvalidTransition) {
			t.Fatalf("reset/complete race returned unexpected error: %v", raceErr)
		}
	}
	if resetSuccesses != 1 {
		t.Fatalf("reset/complete race reset successes = %d, want 1: %v", resetSuccesses, results)
	}
	_ = completeSuccesses

	current := cqState(t, f, round.ID)
	if current.Round.Lifecycle != queue.RoundLifecycleFinalized {
		t.Fatalf("reset/complete current lifecycle = %s, want finalized", current.Round.Lifecycle)
	}
	if currentEntry := cqEntryByID(t, current, entry.ID); currentEntry.Status != queue.EntryStatusCompleted && currentEntry.Status != queue.EntryStatusSkipped {
		t.Fatalf("reset/complete entry status = %s, want completed or skipped", currentEntry.Status)
	}
	if cqCountReciters(current) != 0 {
		t.Fatalf("reset/complete left a reciter in finalized round: %+v", current.Entries)
	}
	var activeRounds, progressRows int
	if err := f.pool.QueryRow(context.Background(), `
		SELECT COUNT(*) FILTER (WHERE lifecycle = 'active'),
		       (SELECT COUNT(*) FROM memorization_progress WHERE queue_entry_id = $1::uuid)
		FROM recitation_queue
		WHERE session_id = $2::uuid
	`, entry.ID, f.session).Scan(&activeRounds, &progressRows); err != nil {
		t.Fatalf("read reset/complete durable summary: %v", err)
	}
	if activeRounds != 1 {
		t.Fatalf("reset/complete active rounds = %d, want exactly 1", activeRounds)
	}
	if currentEntry := cqEntryByID(t, current, entry.ID); currentEntry.Status == queue.EntryStatusCompleted && progressRows != 1 {
		t.Fatalf("completed reset/complete entry progress rows = %d, want 1", progressRows)
	}
	if currentEntry := cqEntryByID(t, current, entry.ID); currentEntry.Status == queue.EntryStatusSkipped && progressRows != 0 {
		t.Fatalf("skipped reset/complete entry progress rows = %d, want 0", progressRows)
	}
	if before.Round.Version >= current.Round.Version {
		t.Fatalf("reset/complete round version moved backwards: before=%d current=%d", before.Round.Version, current.Round.Version)
	}
}

func TestRecitationQueueConcurrency_MoveAndLateJoinAppend(t *testing.T) {
	f := newRecitationQueueConcurrencyFixture(t)
	first := f.addStudent(t, "move-late-first", time.Now().UTC())
	second := f.addStudent(t, "move-late-second", time.Now().UTC().Add(time.Second))
	third := f.addStudent(t, "move-late-third", time.Now().UTC().Add(2*time.Second))
	late := f.addStudent(t, "move-late-joiner", time.Now().UTC().Add(3*time.Second))
	round := cqCreateActiveRound(t, f, first, second, third)
	initial := cqState(t, f, round.ID)
	moving := cqEntry(t, initial, first)
	secondEntry := cqEntry(t, initial, second)
	thirdEntry := cqEntry(t, initial, third)
	rounds := queue.NewRoundService(f.repo)
	results := cqRace(t,
		func(ctx context.Context) error {
			_, err := rounds.Move(ctx, moving.ID, initial.Round.Version, 3)
			return err
		},
		func(ctx context.Context) error {
			return cqAppendLateJoin(ctx, f, round.ID, late, initial.Round.Version)
		},
	)

	winner := -1
	successes := 0
	for _, result := range results {
		err := result.err
		if err == nil {
			successes++
			winner = result.operation
			continue
		}
		if code, ok := cqQueueErrorCode(err); !ok || code != queue.QueueErrorCodeStaleVersion {
			t.Fatalf("move/late-join race returned unexpected error: %v", err)
		}
	}
	if successes != 1 {
		t.Fatalf("move/late-join race successes = %d, want exactly 1: %v", successes, results)
	}
	if winner != 0 && winner != 1 {
		t.Fatalf("move/late-join race winner = %d, want move (0) or late join (1)", winner)
	}

	final := cqState(t, f, round.ID)
	cqAssertContiguousPositions(t, final)
	if final.Round.Version != initial.Round.Version+1 {
		t.Fatalf("move/late-join final version = %d, want %d", final.Round.Version, initial.Round.Version+1)
	}
	if cqCountReciters(final) != 0 {
		t.Fatalf("move/late-join race created a reciter: %+v", final.Entries)
	}
	if winner == 0 {
		if len(final.Entries) != 3 {
			t.Fatalf("move winner durable entry count = %d, want 3", len(final.Entries))
		}
		if got := []string{final.Entries[0].StudentID, final.Entries[1].StudentID, final.Entries[2].StudentID}; got[0] != second || got[1] != third || got[2] != first {
			t.Fatalf("move winner order = %v, want [%s %s %s]", got, second, third, first)
		}
		if got := []string{final.Entries[0].ID, final.Entries[1].ID, final.Entries[2].ID}; got[0] != secondEntry.ID || got[1] != thirdEntry.ID || got[2] != moving.ID {
			t.Fatalf("move winner entry IDs = %v, want [%s %s %s]", got, secondEntry.ID, thirdEntry.ID, moving.ID)
		}
		if cqHasStudent(final, late) {
			t.Fatalf("move winner unexpectedly retained late student %s", late)
		}
	} else {
		if len(final.Entries) != 4 {
			t.Fatalf("late-join winner durable entry count = %d, want 4", len(final.Entries))
		}
		if got := []string{final.Entries[0].StudentID, final.Entries[1].StudentID, final.Entries[2].StudentID, final.Entries[3].StudentID}; got[0] != first || got[1] != second || got[2] != third || got[3] != late {
			t.Fatalf("late-join winner order = %v, want [%s %s %s %s]", got, first, second, third, late)
		}
		lateEntry := cqEntry(t, final, late)
		if got := []string{final.Entries[0].ID, final.Entries[1].ID, final.Entries[2].ID, final.Entries[3].ID}; got[0] != moving.ID || got[1] != secondEntry.ID || got[2] != thirdEntry.ID || got[3] != lateEntry.ID {
			t.Fatalf("late-join winner entry IDs = %v, want [%s %s %s %s]", got, moving.ID, secondEntry.ID, thirdEntry.ID, lateEntry.ID)
		}
		if final.Entries[3].Position != 4 {
			t.Fatalf("late-join position = %d, want 4", final.Entries[3].Position)
		}
		if !cqHasStudent(final, late) {
			t.Fatalf("late-join winner omitted late student %s", late)
		}
	}
}

func TestRecitationQueueConcurrency_OptOutAndSkipFirstTerminalWins(t *testing.T) {
	f := newRecitationQueueConcurrencyFixture(t)
	student := f.addStudent(t, "optout-skip", time.Now().UTC())
	round := cqCreateActiveRound(t, f, student)
	entry := cqEntry(t, cqState(t, f, round.ID), student)
	turns := queue.NewTurnService(f.repo)

	results := cqRace(t,
		func(ctx context.Context) error {
			_, err := turns.Skip(ctx, entry.ID, entry.Version, f.teacher)
			return err
		},
		func(ctx context.Context) error {
			return cqOptOutEntry(ctx, f, round.ID, entry.ID, student, entry.Version)
		},
	)

	successes := 0
	for _, result := range results {
		err := result.err
		if err == nil {
			successes++
			continue
		}
		if code, ok := cqQueueErrorCode(err); !ok || code != queue.QueueErrorCodeStaleVersion {
			t.Fatalf("opt-out/skip race returned unexpected error: %v", err)
		}
	}
	if successes != 1 {
		t.Fatalf("opt-out/skip race successes = %d, want exactly 1: %v", successes, results)
	}

	final := cqState(t, f, round.ID)
	finalEntry := cqEntryByID(t, final, entry.ID)
	if finalEntry.Status != queue.EntryStatusSkipped && finalEntry.Status != queue.EntryStatusOptedOut {
		t.Fatalf("opt-out/skip final status = %s, want skipped or opted_out", finalEntry.Status)
	}
	if cqCountReciters(final) != 0 {
		t.Fatalf("opt-out/skip race left a reciter: %+v", final.Entries)
	}
	if final.Round.Version != round.Version+1 {
		t.Fatalf("opt-out/skip final version = %d, want %d", final.Round.Version, round.Version+1)
	}
	var terminalRows int
	if err := f.pool.QueryRow(context.Background(), `
		SELECT COUNT(*)
		FROM recitation_queue_entries
		WHERE id = $1::uuid AND status IN ('completed', 'skipped', 'opted_out')
	`, entry.ID).Scan(&terminalRows); err != nil {
		t.Fatalf("read opt-out/skip terminal state: %v", err)
	}
	if terminalRows != 1 {
		t.Fatalf("opt-out/skip terminal row count = %d, want 1", terminalRows)
	}
}

func TestRecitationQueueConcurrency_StaleVersionReplaysConverge(t *testing.T) {
	f := newRecitationQueueConcurrencyFixture(t)
	first := f.addStudent(t, "stale-replay-first", time.Now().UTC())
	second := f.addStudent(t, "stale-replay-second", time.Now().UTC().Add(time.Second))
	round := cqCreateActiveRound(t, f, first, second)
	initial := cqState(t, f, round.ID)
	turns := queue.NewTurnService(f.repo)

	results := cqRace(t,
		func(ctx context.Context) error {
			_, err := turns.Advance(ctx, round.ID, initial.Round.Version)
			return err
		},
		func(ctx context.Context) error {
			_, err := turns.Advance(ctx, round.ID, initial.Round.Version)
			return err
		},
		func(ctx context.Context) error {
			_, err := turns.Advance(ctx, round.ID, initial.Round.Version)
			return err
		},
	)

	successes := 0
	stale := 0
	for _, result := range results {
		err := result.err
		if err == nil {
			successes++
			continue
		}
		if code, ok := cqQueueErrorCode(err); !ok || code != queue.QueueErrorCodeStaleVersion {
			t.Fatalf("stale-version race returned unexpected error: %v", err)
		}
		stale++
	}
	if successes != 1 || stale != 2 {
		t.Fatalf("stale-version race outcomes = successes:%d stale:%d, want 1:2: %v", successes, stale, results)
	}

	committed := cqState(t, f, round.ID)
	if committed.Round.Version != initial.Round.Version+1 {
		t.Fatalf("stale-version committed version = %d, want %d", committed.Round.Version, initial.Round.Version+1)
	}
	if committed.Round.SelectedEntryID == nil {
		t.Fatal("stale-version replay left no durable selection")
	}
	if len(committed.Entries) != 2 || cqCountReciters(committed) != 0 {
		t.Fatalf("stale-version replay changed entry cardinality or reciter invariant: %+v", committed.Entries)
	}
	for _, entry := range committed.Entries {
		if entry.Status != queue.EntryStatusWaiting {
			t.Fatalf("stale-version replay changed entry %s to %s", entry.ID, entry.Status)
		}
	}

	for replay := 0; replay < 2; replay++ {
		if _, err := turns.Advance(context.Background(), round.ID, initial.Round.Version); err == nil {
			t.Fatalf("stale-version replay %d unexpectedly succeeded", replay+1)
		} else if code, ok := cqQueueErrorCode(err); !ok || code != queue.QueueErrorCodeStaleVersion {
			t.Fatalf("stale-version replay %d error = %v, want stale_version", replay+1, err)
		}
		afterReplay := cqState(t, f, round.ID)
		if afterReplay.Round.Version != committed.Round.Version || afterReplay.Round.SelectedEntryID == nil || *afterReplay.Round.SelectedEntryID != *committed.Round.SelectedEntryID {
			t.Fatalf("stale-version replay %d changed durable outcome: before=%+v after=%+v", replay+1, committed.Round, afterReplay.Round)
		}
	}
}
