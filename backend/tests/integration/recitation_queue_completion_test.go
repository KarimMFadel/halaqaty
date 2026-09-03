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

	"github.com/KarimMFadel/halaqaty/backend/internal/queue"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// T050 — exactly one memorization_progress row per completed entry under
// retries and concurrent completion; zero rows for skipped/opted-out entries.

func TestRecitationQueueCompletion_OneProgressRowPerCompletedEntry(t *testing.T) {
	f := newRecitationQueueCompletionFixture(t, true)
	ctx := context.Background()
	student := f.students[0]
	round, entry := f.activeRecitingEntry(t, student, true)
	turns := queue.NewTurnService(f.repo)

	grade := queue.GradeGood
	note := "retry-safe completion"
	completed, err := turns.Complete(ctx, entry.ID, entry.Version, f.teacher, &grade, &note)
	if err != nil {
		t.Fatalf("complete entry: %v", err)
	}
	if completed.Status != queue.EntryStatusCompleted {
		t.Fatalf("entry status = %s, want completed", completed.Status)
	}

	progress := f.progressForEntry(t, entry.ID)
	if progress == nil {
		t.Fatal("no progress row after completion")
	}
	if progress.Grade == nil || *progress.Grade != grade {
		t.Fatalf("progress grade = %v, want %s", progress.Grade, grade)
	}
	if progress.Notes == nil || *progress.Notes != note {
		t.Fatalf("progress notes = %v, want %q", progress.Notes, note)
	}
	if f.countProgressRows(t) != 1 {
		t.Fatalf("progress rows = %d, want 1", f.countProgressRows(t))
	}

	// Retry the same completion call with the original entry version; it
	// should be rejected as stale and must not create another progress row.
	_, err = turns.Complete(ctx, entry.ID, entry.Version, f.teacher, &grade, &note)
	var qerr *queue.QueueError
	if !errors.As(err, &qerr) || qerr.Code != queue.QueueErrorCodeStaleVersion {
		t.Fatalf("retry completion error = %v, want stale_version", err)
	}
	if f.countProgressRows(t) != 1 {
		t.Fatalf("retry created extra progress rows: got %d", f.countProgressRows(t))
	}
	_ = round
}

func TestRecitationQueueCompletion_ConcurrentCompletionConvergesToOneRow(t *testing.T) {
	f := newRecitationQueueCompletionFixture(t, true)
	ctx := context.Background()
	student := f.students[0]
	_, entry := f.activeRecitingEntry(t, student, true)
	turns := queue.NewTurnService(f.repo)

	grade := queue.GradeExcellent
	note := "concurrent completion"
	var wg sync.WaitGroup
	results := make(chan error, 5)
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := turns.Complete(ctx, entry.ID, entry.Version, f.teacher, &grade, &note)
			results <- err
		}()
	}
	wg.Wait()
	close(results)

	successes := 0
	for err := range results {
		if err == nil {
			successes++
			continue
		}
		var qerr *queue.QueueError
		if !errors.As(err, &qerr) || (qerr.Code != queue.QueueErrorCodeStaleVersion && !errors.Is(err, pgx.ErrTxCommitRollback)) {
			t.Fatalf("unexpected concurrent completion error: %v", err)
		}
	}
	if successes != 1 {
		t.Fatalf("concurrent completion successes = %d, want 1", successes)
	}
	if f.countProgressRows(t) != 1 {
		t.Fatalf("progress rows after concurrent completion = %d, want 1", f.countProgressRows(t))
	}
	progress := f.progressForEntry(t, entry.ID)
	if progress == nil || progress.Grade == nil || *progress.Grade != grade {
		t.Fatalf("progress after race = %+v, want %s", progress, grade)
	}
}

func TestRecitationQueueCompletion_GradingOptionalCreatesProgressWithoutGrade(t *testing.T) {
	f := newRecitationQueueCompletionFixture(t, false)
	ctx := context.Background()
	student := f.students[0]
	_, entry := f.activeRecitingEntry(t, student, false)
	turns := queue.NewTurnService(f.repo)

	completed, err := turns.Complete(ctx, entry.ID, entry.Version, f.teacher, nil, nil)
	if err != nil {
		t.Fatalf("grading-optional complete: %v", err)
	}
	if completed.Grade != nil || completed.TeacherNotes != nil {
		t.Fatalf("optional completion carries grade/note: %+v", completed)
	}
	progress := f.progressForEntry(t, entry.ID)
	if progress == nil {
		t.Fatal("grading-optional completion created no progress row")
	}
	if progress.Grade != nil || progress.Notes != nil {
		t.Fatalf("optional progress carries grade/note: %+v", progress)
	}
}

func TestRecitationQueueCompletion_SkippedAndOptedOutCreateZeroProgress(t *testing.T) {
	f := newRecitationQueueCompletionFixture(t, true)
	ctx := context.Background()
	turns := queue.NewTurnService(f.repo)

	round, skippedReciting := f.activeRecitingEntryWithStudents(t, []string{f.students[0], f.students[1]}, true)
	if _, err := turns.Skip(ctx, skippedReciting.ID, skippedReciting.Version, f.teacher); err != nil {
		t.Fatalf("skip reciting entry: %v", err)
	}
	if f.progressForEntry(t, skippedReciting.ID) != nil {
		t.Fatal("skipped reciting entry created a progress row")
	}

	state, err := f.repo.LoadQueueState(ctx, round.ID, queue.Viewer{UserID: f.teacher, IsManager: true})
	if err != nil {
		t.Fatalf("load queue state: %v", err)
	}
	var waitingEntry queue.QueueEntry
	for _, e := range state.Entries {
		if e.Status == queue.EntryStatusWaiting {
			waitingEntry = e
			break
		}
	}
	if waitingEntry.ID == "" {
		t.Fatal("no waiting entry found for skip test")
	}
	if _, err := turns.Skip(ctx, waitingEntry.ID, waitingEntry.Version, f.teacher); err != nil {
		t.Fatalf("skip waiting entry: %v", err)
	}
	if f.progressForEntry(t, waitingEntry.ID) != nil {
		t.Fatal("skipped waiting entry created a progress row")
	}

	// Opted-out entry through direct terminal transition.
	_, optedOutEntry := f.activeRecitingEntry(t, f.students[1], true)
	err = f.repo.WithTx(ctx, func(tx *queue.Tx) error {
		_, err := tx.TransitionEntry(ctx, optedOutEntry.ID, queue.EntryStatusReciting, optedOutEntry.Version, queue.EntryStatusOptedOut, nil, nil, &f.teacher)
		return err
	})
	if err != nil {
		t.Fatalf("opt out entry: %v", err)
	}
	if f.progressForEntry(t, optedOutEntry.ID) != nil {
		t.Fatal("opted-out entry created a progress row")
	}

	if f.countProgressRows(t) != 0 {
		t.Fatalf("progress rows = %d, want 0", f.countProgressRows(t))
	}
}

type completionFixture struct {
	pool     *pgxpool.Pool
	repo     *queue.Repository
	teacher  string
	students []string
	session  string
}

func newRecitationQueueCompletionFixture(t *testing.T, gradingRequired bool) *completionFixture {
	t.Helper()
	ctx := context.Background()
	_ = requireDatabaseURL(t)
	schema := uniqueSchemaName(t) + "_completion"
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
	students := []string{
		seedCompletionUser(t, pool, "student-a"),
		seedCompletionUser(t, pool, "student-b"),
	}
	circleID := seedCompletionCircle(t, pool, teacher)
	seedCompletionMember(t, pool, circleID, teacher, "teacher", time.Now().UTC())
	for i, studentID := range students {
		seedCompletionMember(t, pool, circleID, studentID, "student", time.Now().UTC().Add(time.Duration(i)*time.Second))
	}

	var sessionID string
	if err := pool.QueryRow(ctx, `
		INSERT INTO sessions (circle_id, created_by, queue_grade_visibility)
		VALUES ($1::uuid, $2::uuid, 'managers_only')
		RETURNING id::text
	`, circleID, teacher).Scan(&sessionID); err != nil {
		t.Fatalf("insert session: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		UPDATE sessions SET status = 'active', actual_start = NOW() WHERE id = $1::uuid
	`, sessionID); err != nil {
		t.Fatalf("activate session: %v", err)
	}

	repo := queue.NewQueueRepository(pool)
	return &completionFixture{pool: pool, repo: repo, teacher: teacher, students: students, session: sessionID}
}

func (f *completionFixture) activeRecitingEntry(t *testing.T, studentID string, gradingRequired bool) (queue.Round, queue.QueueEntry) {
	t.Helper()
	return f.activeRecitingEntryWithStudents(t, []string{studentID}, gradingRequired)
}

func (f *completionFixture) activeRecitingEntryWithStudents(t *testing.T, studentIDs []string, gradingRequired bool) (queue.Round, queue.QueueEntry) {
	t.Helper()
	if len(studentIDs) == 0 {
		t.Fatal("activeRecitingEntryWithStudents needs at least one student")
	}
	ctx := context.Background()
	var round queue.Round
	var recitingEntry queue.QueueEntry
	err := f.repo.WithTx(ctx, func(tx *queue.Tx) error {
		if err := tx.LockRoundAllocation(ctx, f.session); err != nil {
			return err
		}
		existing, err := tx.LockActiveRound(ctx, f.session)
		if err == nil {
			policy, err := tx.LockSessionPolicy(ctx, f.session)
			if err != nil {
				return err
			}
			if _, err := tx.FinalizeRound(ctx, existing.ID, existing.Version, policy.Policy.Finalization, f.teacher); err != nil {
				return err
			}
		} else if !errors.Is(err, pgx.ErrNoRows) {
			return err
		}
		number, err := tx.NextRoundNumber(ctx, f.session)
		if err != nil {
			return err
		}
		round, err = tx.CreateRound(ctx, queue.NewRound{
			SessionID: f.session, RoundNumber: number, Type: queue.RoundTypeRevision, SurahID: 1,
			FromAyah: 1, ToAyah: 7, GradingRequired: gradingRequired, Lifecycle: queue.RoundLifecyclePrepared, CreatedBy: f.teacher,
		})
		if err != nil {
			return err
		}
		for i, studentID := range studentIDs {
			entry, err := tx.InsertQueueEntry(ctx, round.ID, studentID, i+1)
			if err != nil {
				return err
			}
			if i == 0 {
				recitingEntry = entry
			}
		}
		round, err = tx.ActivateRound(ctx, round.ID)
		return err
	})
	if err != nil {
		t.Fatalf("create active reciting entry: %v", err)
	}

	var updated queue.QueueEntry
	err = f.repo.WithTx(ctx, func(tx *queue.Tx) error {
		if _, err := tx.SetRoundSelection(ctx, round.ID, &recitingEntry.ID, round.Version); err != nil {
			return err
		}
		updated, err = tx.TransitionEntry(ctx, recitingEntry.ID, queue.EntryStatusWaiting, recitingEntry.Version, queue.EntryStatusReciting, nil, nil, nil)
		return err
	})
	if err != nil {
		t.Fatalf("start entry: %v", err)
	}
	return round, updated
}

func (f *completionFixture) progressForEntry(t *testing.T, entryID string) *queue.ProgressRecord {
	t.Helper()
	var row queue.ProgressRecord
	var grade, notes *string
	err := f.pool.QueryRow(context.Background(), `
		SELECT id::text, student_id::text, circle_id::text, session_id::text, queue_entry_id::text,
		       surah_id, surah_name, from_ayah, to_ayah, type, grade, notes, date
		FROM memorization_progress
		WHERE queue_entry_id = $1::uuid
	`, entryID).Scan(&row.ID, &row.StudentID, &row.CircleID, &row.SessionID, &row.QueueEntryID,
		&row.SurahID, &row.SurahName, &row.FromAyah, &row.ToAyah, &row.Type, &grade, &notes, &row.Date)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}
		t.Fatalf("load progress for entry %s: %v", entryID, err)
	}
	if grade != nil {
		g := queue.Grade(*grade)
		row.Grade = &g
	}
	row.Notes = notes
	return &row
}

func (f *completionFixture) countProgressRows(t *testing.T) int {
	t.Helper()
	var count int
	if err := f.pool.QueryRow(context.Background(), `
		SELECT COUNT(*) FROM memorization_progress WHERE session_id = $1::uuid
	`, f.session).Scan(&count); err != nil {
		t.Fatalf("count progress rows: %v", err)
	}
	return count
}

func seedCompletionUser(t *testing.T, pool *pgxpool.Pool, label string) string {
	t.Helper()
	var id string
	if err := pool.QueryRow(context.Background(), `
		INSERT INTO users (firebase_uid, email)
		VALUES ($1, $2)
		RETURNING id::text
	`, "firebase-completion-"+label, label+"@example.com").Scan(&id); err != nil {
		t.Fatalf("seed user %s: %v", label, err)
	}
	return id
}

func seedCompletionCircle(t *testing.T, pool *pgxpool.Pool, teacherID string) string {
	t.Helper()
	var id string
	if err := pool.QueryRow(context.Background(), `
		INSERT INTO circles (name, teacher_id, invite_code)
		VALUES ('T050 Completion Circle', $1::uuid, 'HLQ-T050')
		RETURNING id::text
	`, teacherID).Scan(&id); err != nil {
		t.Fatalf("seed circle: %v", err)
	}
	return id
}

func seedCompletionMember(t *testing.T, pool *pgxpool.Pool, circleID, userID, role string, joinedAt time.Time) {
	t.Helper()
	if _, err := pool.Exec(context.Background(), `
		INSERT INTO circle_members (circle_id, user_id, role, joined_at)
		VALUES ($1::uuid, $2::uuid, $3, $4)
	`, circleID, userID, role, joinedAt); err != nil {
		t.Fatalf("seed member %s (%s): %v", userID, role, err)
	}
}

func requireDatabaseURL(t *testing.T) string {
	t.Helper()
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		t.Skip("DATABASE_URL is not set; T050 requires PostgreSQL via DATABASE_URL")
	}
	return dbURL
}
