//go:build integration

package queue

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

var turnServiceMigrations = []string{
	"000010_auth_roles_profile.up.sql",
	"000011_auth_roles_profile_alignment.up.sql",
	"000012_auth_profiles_display_name.up.sql",
	"000013_create_circles.up.sql",
	"000014_circle_members_circle_fk.up.sql",
	"000015_circle_management.up.sql",
	"000016_live_sessions.up.sql",
	"000017_recitation_queue_system.up.sql",
}

func newTurnServiceRepository(t *testing.T) *Repository {
	t.Helper()
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		t.Fatal("DATABASE_URL is required for queue integration tests")
	}

	ctx := context.Background()
	schema := fmt.Sprintf("test_turn_service_%d", time.Now().UnixNano())
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
	for _, name := range turnServiceMigrations {
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

func seedTurnServiceUser(t *testing.T, repo *Repository, label string) string {
	t.Helper()
	var id string
	if err := repo.pool.QueryRow(context.Background(), `
		INSERT INTO users (firebase_uid, email)
		VALUES ($1, $2)
		RETURNING id::text
	`, "firebase-turn-"+label, label+"@example.com").Scan(&id); err != nil {
		t.Fatalf("seed user %s: %v", label, err)
	}
	return id
}

func seedTurnServiceCircle(t *testing.T, repo *Repository, teacherID string) string {
	t.Helper()
	var id string
	if err := repo.pool.QueryRow(context.Background(), `
		INSERT INTO circles (name, teacher_id, invite_code)
		VALUES ('Turn Service Circle', $1::uuid, 'HLQ-TURN01')
		RETURNING id::text
	`, teacherID).Scan(&id); err != nil {
		t.Fatalf("seed circle: %v", err)
	}
	return id
}

func seedTurnServiceMember(t *testing.T, repo *Repository, circleID, userID, role string) {
	t.Helper()
	if _, err := repo.pool.Exec(context.Background(), `
		INSERT INTO circle_members (circle_id, user_id, role, joined_at)
		VALUES ($1::uuid, $2::uuid, $3, NOW())
	`, circleID, userID, role); err != nil {
		t.Fatalf("seed member %s (%s): %v", userID, role, err)
	}
}

func insertTurnServiceSession(t *testing.T, repo *Repository, circleID, creatorID string) string {
	t.Helper()
	var id string
	if err := repo.pool.QueryRow(context.Background(), `
		INSERT INTO sessions (circle_id, created_by)
		VALUES ($1::uuid, $2::uuid)
		RETURNING id::text
	`, circleID, creatorID).Scan(&id); err != nil {
		t.Fatalf("seed session: %v", err)
	}
	return id
}

func createTurnServiceRound(t *testing.T, repo *Repository, sessionID, createdBy string, studentIDs []string) Round {
	t.Helper()
	ctx := context.Background()
	var round Round
	err := repo.WithTx(ctx, func(tx *Tx) error {
		if err := tx.LockRoundAllocation(ctx, sessionID); err != nil {
			return err
		}
		number, err := tx.NextRoundNumber(ctx, sessionID)
		if err != nil {
			return err
		}
		round, err = tx.CreateRound(ctx, NewRound{
			SessionID:       sessionID,
			RoundNumber:     number,
			Type:            RoundTypeRevision,
			SurahID:         1,
			FromAyah:        1,
			ToAyah:          7,
			GradingRequired: true,
			Lifecycle:       RoundLifecycleActive,
			CreatedBy:       createdBy,
		})
		if err != nil {
			return err
		}
		for i, studentID := range studentIDs {
			if _, err := tx.InsertQueueEntry(ctx, round.ID, studentID, i+1); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("create round: %v", err)
	}
	return round
}

func roundState(t *testing.T, repo *Repository, roundID string) Round {
	t.Helper()
	var round Round
	err := repo.WithTx(context.Background(), func(tx *Tx) error {
		var err error
		round, err = tx.LockRound(context.Background(), roundID)
		return err
	})
	if err != nil {
		t.Fatalf("load round state: %v", err)
	}
	return round
}

func queueStateForManager(t *testing.T, repo *Repository, roundID, managerID string) QueueState {
	t.Helper()
	state, err := repo.LoadQueueState(context.Background(), roundID, Viewer{UserID: managerID, IsManager: true})
	if err != nil {
		t.Fatalf("load queue state: %v", err)
	}
	return state
}

func outboxCount(t *testing.T, repo *Repository, roundID string) int {
	t.Helper()
	var count int
	if err := repo.pool.QueryRow(context.Background(), `
		SELECT COUNT(*) FROM queue_event_outbox WHERE round_id = $1::uuid
	`, roundID).Scan(&count); err != nil {
		t.Fatalf("count outbox rows: %v", err)
	}
	return count
}

func insertSelectedEntry(t *testing.T, repo *Repository, roundID, entryID string, expectedVersion int64) Round {
	t.Helper()
	var round Round
	err := repo.WithTx(context.Background(), func(tx *Tx) error {
		var err error
		round, err = tx.SetRoundSelection(context.Background(), roundID, &entryID, expectedVersion)
		return err
	})
	if err != nil {
		t.Fatalf("set selection: %v", err)
	}
	return round
}

func transitionEntryStatus(t *testing.T, repo *Repository, entryID string, from, to EntryStatus) QueueEntry {
	t.Helper()
	var entry QueueEntry
	err := repo.WithTx(context.Background(), func(tx *Tx) error {
		current, err := tx.LockEntry(context.Background(), entryID)
		if err != nil {
			return err
		}
		if current.Status != from {
			return fmt.Errorf("entry %s status = %s, want %s", entryID, current.Status, from)
		}
		entry, err = tx.TransitionEntry(context.Background(), entryID, from, current.Version, to, nil, nil, nil)
		return err
	})
	if err != nil {
		t.Fatalf("transition entry %s %s->%s: %v", entryID, from, to, err)
	}
	return entry
}

func createTurnServiceFixture(t *testing.T, studentCount int) (*Repository, string, string, []string, Round, QueueState) {
	t.Helper()
	repo := newTurnServiceRepository(t)
	teacherID := seedTurnServiceUser(t, repo, "teacher")
	circleID := seedTurnServiceCircle(t, repo, teacherID)
	seedTurnServiceMember(t, repo, circleID, teacherID, "teacher")
	sessionID := insertTurnServiceSession(t, repo, circleID, teacherID)

	students := make([]string, 0, studentCount)
	for i := 0; i < studentCount; i++ {
		studentID := seedTurnServiceUser(t, repo, fmt.Sprintf("student-%d", i+1))
		seedTurnServiceMember(t, repo, circleID, studentID, "student")
		students = append(students, studentID)
	}

	round := createTurnServiceRound(t, repo, sessionID, teacherID, students)
	state := queueStateForManager(t, repo, round.ID, teacherID)
	return repo, teacherID, sessionID, students, round, state
}

func findEntry(t *testing.T, state QueueState, studentID string) QueueEntry {
	t.Helper()
	for _, entry := range state.Entries {
		if entry.StudentID == studentID {
			return entry
		}
	}
	t.Fatalf("entry for student %s not found", studentID)
	return QueueEntry{}
}

func recitingEntries(state QueueState) []QueueEntry {
	var entries []QueueEntry
	for _, entry := range state.Entries {
		if entry.Status == EntryStatusReciting {
			entries = append(entries, entry)
		}
	}
	return entries
}

func TestTurnServiceAdvanceReplacesSelectionWithoutDuplicate(t *testing.T) {
	repo, teacherID, _, students, round, state := createTurnServiceFixture(t, 2)
	service := NewTurnService(repo)

	firstRound, err := service.Advance(context.Background(), round.ID, round.Version)
	if err != nil {
		t.Fatalf("first advance: %v", err)
	}
	if firstRound.SelectedEntryID == nil {
		t.Fatal("first advance left selection nil")
	}
	firstEntry := findEntry(t, state, students[0])
	secondEntry := findEntry(t, state, students[1])
	if *firstRound.SelectedEntryID != firstEntry.ID {
		t.Fatalf("first selection = %s, want %s", *firstRound.SelectedEntryID, firstEntry.ID)
	}

	secondRound, err := service.Advance(context.Background(), round.ID, firstRound.Version)
	if err != nil {
		t.Fatalf("second advance: %v", err)
	}
	if secondRound.SelectedEntryID == nil {
		t.Fatal("second advance left selection nil")
	}
	if *secondRound.SelectedEntryID != secondEntry.ID {
		t.Fatalf("second selection = %s, want replacement %s", *secondRound.SelectedEntryID, secondEntry.ID)
	}

	reloaded := queueStateForManager(t, repo, round.ID, teacherID)
	if len(reloaded.Entries) != 2 {
		t.Fatalf("entries after replacement = %d, want 2", len(reloaded.Entries))
	}
}

func TestTurnServiceAdvanceWithNoWaitingEntryDoesNotMutate(t *testing.T) {
	repo, teacherID, _, _, round, state := createTurnServiceFixture(t, 0)
	service := NewTurnService(repo)

	_, err := service.Advance(context.Background(), round.ID, round.Version)
	if err == nil {
		t.Fatal("advance with no waiting entry succeeded")
	}
	var qerr *QueueError
	if !errors.As(err, &qerr) || qerr.Code != QueueErrorCodeNoWaitingEntry {
		t.Fatalf("advance with no waiting entry error = %v, want no_waiting_entry", err)
	}

	reloaded := roundState(t, repo, round.ID)
	if reloaded.Version != round.Version {
		t.Fatalf("round version = %d, want unchanged %d", reloaded.Version, round.Version)
	}
	if reloaded.SelectedEntryID != nil {
		t.Fatalf("selection = %v, want nil", reloaded.SelectedEntryID)
	}
	if outboxCount(t, repo, round.ID) != 0 {
		t.Fatalf("outbox count = %d, want 0", outboxCount(t, repo, round.ID))
	}
	if len(state.Entries) != 0 {
		t.Fatalf("initial entries = %d, want 0", len(state.Entries))
	}
	_ = teacherID
}

func TestTurnServiceAdvanceRejectsWhileReciting(t *testing.T) {
	repo, teacherID, _, students, round, state := createTurnServiceFixture(t, 2)
	service := NewTurnService(repo)

	reciting := transitionEntryStatus(t, repo, findEntry(t, state, students[0]).ID, EntryStatusWaiting, EntryStatusReciting)
	_, err := service.Advance(context.Background(), round.ID, round.Version)
	if err == nil {
		t.Fatal("advance while reciting succeeded")
	}
	var qerr *QueueError
	if !errors.As(err, &qerr) || qerr.Code != QueueErrorCodeEntryReciting {
		t.Fatalf("advance while reciting error = %v, want entry_reciting", err)
	}

	reloaded := roundState(t, repo, round.ID)
	if reloaded.SelectedEntryID != nil {
		t.Fatalf("selection = %v, want nil", reloaded.SelectedEntryID)
	}
	state = queueStateForManager(t, repo, round.ID, teacherID)
	if got := recitingEntries(state); len(got) != 1 || got[0].ID != reciting.ID {
		t.Fatalf("reciting entries = %+v, want [%s]", got, reciting.ID)
	}
}

func TestTurnServiceStartRequiresSelectedWaitingEntry(t *testing.T) {
	repo, teacherID, _, students, round, state := createTurnServiceFixture(t, 2)
	service := NewTurnService(repo)

	firstEntry := findEntry(t, state, students[0])
	secondEntry := findEntry(t, state, students[1])

	if _, err := service.Start(context.Background(), firstEntry.ID, firstEntry.Version); err == nil {
		t.Fatal("start without selection succeeded")
	}

	insertSelectedEntry(t, repo, round.ID, secondEntry.ID, round.Version)
	if _, err := service.Start(context.Background(), firstEntry.ID, firstEntry.Version); err == nil {
		t.Fatal("start for unselected entry succeeded")
	}

	if _, err := service.Start(context.Background(), secondEntry.ID, secondEntry.Version); err != nil {
		t.Fatalf("start selected entry: %v", err)
	}

	state = queueStateForManager(t, repo, round.ID, teacherID)
	entry := findEntry(t, state, students[1])
	if entry.Status != EntryStatusReciting {
		t.Fatalf("selected entry status = %s, want reciting", entry.Status)
	}
}

func TestTurnServiceRecordsTurnsWithoutMediaControl(t *testing.T) {
	repo, teacherID, _, students, round, state := createTurnServiceFixture(t, 1)
	entry := findEntry(t, state, students[0])
	insertSelectedEntry(t, repo, round.ID, entry.ID, round.Version)

	service := NewTurnService(repo)
	started, err := service.Start(context.Background(), entry.ID, entry.Version)
	if err != nil {
		t.Fatalf("start without media control: %v", err)
	}
	if started.Status != EntryStatusReciting {
		t.Fatalf("started status = %s, want reciting", started.Status)
	}

	state = queueStateForManager(t, repo, round.ID, teacherID)
	if got := findEntry(t, state, students[0]).Status; got != EntryStatusReciting {
		t.Fatalf("persisted status = %s, want reciting", got)
	}
}

func TestTurnServiceStartUsesOneReciterDatabaseGuard(t *testing.T) {
	repo, teacherID, _, students, round, state := createTurnServiceFixture(t, 2)
	service := NewTurnService(repo)

	firstEntry := findEntry(t, state, students[0])
	secondEntry := findEntry(t, state, students[1])
	reciting := transitionEntryStatus(t, repo, firstEntry.ID, EntryStatusWaiting, EntryStatusReciting)
	insertSelectedEntry(t, repo, round.ID, secondEntry.ID, round.Version)

	_, err := service.Start(context.Background(), secondEntry.ID, secondEntry.Version)
	if err == nil {
		t.Fatal("start with existing reciter succeeded")
	}
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) {
		t.Fatalf("start with existing reciter error = %T %v, want pg constraint error", err, err)
	}

	state = queueStateForManager(t, repo, round.ID, teacherID)
	recitingNow := recitingEntries(state)
	if len(recitingNow) != 1 || recitingNow[0].ID != reciting.ID {
		t.Fatalf("reciting entries = %+v, want only %s", recitingNow, reciting.ID)
	}
}

func TestTurnServiceSkipHandlesWaitingAndRecitingEntries(t *testing.T) {
	repo, teacherID, _, students, round, state := createTurnServiceFixture(t, 2)
	waitingEntry := findEntry(t, state, students[0])
	recitingEntry := findEntry(t, state, students[1])

	recitingEntry = transitionEntryStatus(t, repo, recitingEntry.ID, EntryStatusWaiting, EntryStatusReciting)
	service := NewTurnService(repo)

	waitingSkipped, err := service.Skip(context.Background(), waitingEntry.ID, waitingEntry.Version, teacherID)
	if err != nil {
		t.Fatalf("skip waiting entry: %v", err)
	}
	if waitingSkipped.Status != EntryStatusSkipped {
		t.Fatalf("waiting skip status = %s, want skipped", waitingSkipped.Status)
	}
	recitingSkipped, err := service.Skip(context.Background(), recitingEntry.ID, recitingEntry.Version, teacherID)
	if err != nil {
		t.Fatalf("skip reciting entry: %v", err)
	}
	if recitingSkipped.Status != EntryStatusSkipped {
		t.Fatalf("reciting skip status = %s, want skipped", recitingSkipped.Status)
	}
	state = queueStateForManager(t, repo, round.ID, teacherID)
	for _, entry := range state.Entries {
		if entry.Status != EntryStatusSkipped {
			t.Fatalf("entry %s status = %s, want skipped", entry.ID, entry.Status)
		}
	}
}

func TestTurnServiceStartWithExistingReciterLeavesCommittedTruth(t *testing.T) {
	repo, teacherID, _, students, round, state := createTurnServiceFixture(t, 2)
	firstEntry := findEntry(t, state, students[0])
	secondEntry := findEntry(t, state, students[1])

	_ = transitionEntryStatus(t, repo, firstEntry.ID, EntryStatusWaiting, EntryStatusReciting)
	insertSelectedEntry(t, repo, round.ID, secondEntry.ID, round.Version)
	service := NewTurnService(repo)

	_, err := service.Start(context.Background(), secondEntry.ID, secondEntry.Version)
	if err == nil {
		t.Fatal("start with existing reciter succeeded")
	}
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) {
		t.Fatalf("start with existing reciter error = %T %v, want pg constraint error", err, err)
	}

	committed := queueStateForManager(t, repo, round.ID, teacherID)
	if got := recitingEntries(committed); len(got) != 1 || got[0].StudentID != students[0] {
		t.Fatalf("reciting entries = %+v, want only first student", got)
	}
}

func TestTurnServiceStartReportsStaleEntryVersion(t *testing.T) {
	repo, _, _, students, round, state := createTurnServiceFixture(t, 1)
	entry := findEntry(t, state, students[0])
	insertSelectedEntry(t, repo, round.ID, entry.ID, round.Version)

	// Bump the entry's own optimistic-lock version out from under the caller;
	// the version guard fires before the selection and status checks.
	bumped := transitionEntryStatus(t, repo, entry.ID, EntryStatusWaiting, EntryStatusReciting)
	service := NewTurnService(repo)

	_, err := service.Start(context.Background(), entry.ID, entry.Version)
	if err == nil {
		t.Fatal("start with stale entry version succeeded")
	}
	var qerr *QueueError
	if !errors.As(err, &qerr) || qerr.Code != QueueErrorCodeStaleVersion {
		t.Fatalf("stale start error = %v, want stale_version", err)
	}
	if bumped.Version == entry.Version {
		t.Fatalf("bumped entry version = %d, want > %d", bumped.Version, entry.Version)
	}
}

func TestTurnServiceSkipReportsStaleEntryVersion(t *testing.T) {
	repo, teacherID, _, students, round, state := createTurnServiceFixture(t, 1)
	entry := findEntry(t, state, students[0])
	insertSelectedEntry(t, repo, round.ID, entry.ID, round.Version)

	// Skipping a reciting entry with the current version is legal, so only
	// the stale token can explain the first rejection below.
	bumped := transitionEntryStatus(t, repo, entry.ID, EntryStatusWaiting, EntryStatusReciting)
	service := NewTurnService(repo)

	if _, err := service.Skip(context.Background(), entry.ID, entry.Version, teacherID); err == nil {
		t.Fatal("skip with stale entry version succeeded")
	}
	if _, err := service.Skip(context.Background(), entry.ID, bumped.Version, teacherID); err != nil {
		t.Fatalf("skip reciting entry with current version: %v", err)
	}
}

func createTurnServiceActiveRound(t *testing.T, repo *Repository, sessionID, createdBy, studentID string, gradingRequired bool) (Round, QueueEntry) {
	t.Helper()
	ctx := context.Background()
	var round Round
	var entry QueueEntry
	err := repo.WithTx(ctx, func(tx *Tx) error {
		if err := tx.LockRoundAllocation(ctx, sessionID); err != nil {
			return err
		}
		// Finalize any existing active round so this test owns exactly one active round.
		existing, err := tx.LockActiveRound(ctx, sessionID)
		if err == nil {
			policy, err := tx.LockSessionPolicy(ctx, sessionID)
			if err != nil {
				return err
			}
			if _, err := tx.FinalizeRound(ctx, existing.ID, existing.Version, policy.Policy.Finalization, createdBy); err != nil {
				return err
			}
		} else if !errors.Is(err, pgx.ErrNoRows) {
			return err
		}

		number, err := tx.NextRoundNumber(ctx, sessionID)
		if err != nil {
			return err
		}
		round, err = tx.CreateRound(ctx, NewRound{
			SessionID:       sessionID,
			RoundNumber:     number,
			Type:            RoundTypeRevision,
			SurahID:         1,
			FromAyah:        1,
			ToAyah:          7,
			GradingRequired: gradingRequired,
			Lifecycle:       RoundLifecyclePrepared,
			CreatedBy:       createdBy,
		})
		if err != nil {
			return err
		}
		entry, err = tx.InsertQueueEntry(ctx, round.ID, studentID, 1)
		if err != nil {
			return err
		}
		round, err = tx.ActivateRound(ctx, round.ID)
		return err
	})
	if err != nil {
		t.Fatalf("create active round: %v", err)
	}
	insertSelectedEntry(t, repo, round.ID, entry.ID, round.Version)
	transitionEntryStatus(t, repo, entry.ID, EntryStatusWaiting, EntryStatusReciting)
	state := queueStateForManager(t, repo, round.ID, createdBy)
	return state.Round, findEntry(t, state, studentID)
}

func setGradeCorrectionPolicy(t *testing.T, repo *Repository, sessionID string, correction GradeCorrection) {
	t.Helper()
	if _, err := repo.pool.Exec(context.Background(), `
		UPDATE sessions
		SET queue_grade_correction = $1, queue_policy_version = queue_policy_version + 1
		WHERE id = $2::uuid
	`, correction, sessionID); err != nil {
		t.Fatalf("set grade correction policy: %v", err)
	}
}

func progressForEntry(t *testing.T, repo *Repository, entryID string) *ProgressRecord {
	t.Helper()
	var row ProgressRecord
	var grade, notes *string
	err := repo.pool.QueryRow(context.Background(), `
		SELECT id::text, student_id::text, circle_id::text, session_id::text, queue_entry_id::text,
		       surah_id, surah_name, from_ayah, to_ayah, type, grade, notes, date, created_at, updated_at
		FROM memorization_progress
		WHERE queue_entry_id = $1::uuid
	`, entryID).Scan(&row.ID, &row.StudentID, &row.CircleID, &row.SessionID, &row.QueueEntryID,
		&row.SurahID, &row.SurahName, &row.FromAyah, &row.ToAyah, &row.Type, &grade, &notes, &row.Date, &row.CreatedAt, &row.UpdatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}
		t.Fatalf("load progress for entry %s: %v", entryID, err)
	}
	if grade != nil {
		g := Grade(*grade)
		row.Grade = &g
	}
	row.Notes = notes
	return &row
}

func countProgressRows(t *testing.T, repo *Repository, sessionID string) int {
	t.Helper()
	var count int
	if err := repo.pool.QueryRow(context.Background(), `
		SELECT COUNT(*) FROM memorization_progress WHERE session_id = $1::uuid
	`, sessionID).Scan(&count); err != nil {
		t.Fatalf("count progress rows: %v", err)
	}
	return count
}

func repeatChar(char string, n int) string {
	var b strings.Builder
	b.Grow(n)
	for i := 0; i < n; i++ {
		b.WriteString(char)
	}
	return b.String()
}

func TestTurnServiceCompleteAllGrades(t *testing.T) {
	repo, teacherID, sessionID, students, _, _ := createTurnServiceFixture(t, 5)
	service := NewTurnService(repo)
	grades := []Grade{GradeExcellent, GradeGood, GradeAcceptable, GradeNeedsReview, GradeRepeat}
	for i, grade := range grades {
		studentID := students[i]
		round, entry := createTurnServiceActiveRound(t, repo, sessionID, teacherID, studentID, true)
		note := fmt.Sprintf("grade note %d", i)
		completed, err := service.Complete(context.Background(), entry.ID, entry.Version, teacherID, &grade, &note)
		if err != nil {
			t.Fatalf("complete %s: %v", grade, err)
		}
		if completed.Status != EntryStatusCompleted {
			t.Fatalf("completed status = %s, want completed", completed.Status)
		}
		if completed.Grade == nil || *completed.Grade != grade {
			t.Fatalf("completed grade = %v, want %s", completed.Grade, grade)
		}
		if completed.TeacherNotes == nil || *completed.TeacherNotes != note {
			t.Fatalf("completed notes = %v, want %q", completed.TeacherNotes, note)
		}
		progress := progressForEntry(t, repo, entry.ID)
		if progress == nil {
			t.Fatalf("%s: no progress row created", grade)
		}
		if progress.Grade == nil || *progress.Grade != grade {
			t.Fatalf("%s: progress grade = %v, want %s", grade, progress.Grade, grade)
		}
		if progress.Notes == nil || *progress.Notes != note {
			t.Fatalf("%s: progress notes = %v, want %q", grade, progress.Notes, note)
		}
		if progress.SurahID != 1 || progress.FromAyah != 1 || progress.ToAyah != 7 {
			t.Fatalf("%s: progress range = %d-%d in surah %d, want 1-7 in surah 1", grade, progress.FromAyah, progress.ToAyah, progress.SurahID)
		}
		state := queueStateForManager(t, repo, round.ID, teacherID)
		completedEntry := findEntry(t, state, studentID)
		if completedEntry.Status != EntryStatusCompleted {
			t.Fatalf("%s: loaded status = %s, want completed", grade, completedEntry.Status)
		}
		_ = completedEntry
	}
}

func TestTurnServiceCompleteNoteBoundary(t *testing.T) {
	repo, teacherID, sessionID, students, _, _ := createTurnServiceFixture(t, 2)
	service := NewTurnService(repo)

	// 500-character note succeeds.
	_, entry500 := createTurnServiceActiveRound(t, repo, sessionID, teacherID, students[0], true)
	note500 := repeatChar("a", 500)
	grade := GradeGood
	completed, err := service.Complete(context.Background(), entry500.ID, entry500.Version, teacherID, &grade, &note500)
	if err != nil {
		t.Fatalf("complete with 500-char note: %v", err)
	}
	if completed.TeacherNotes == nil || *completed.TeacherNotes != note500 {
		t.Fatalf("500-char note not retained")
	}

	// 501-character note is rejected before mutation.
	_, entry501 := createTurnServiceActiveRound(t, repo, sessionID, teacherID, students[1], true)
	note501 := repeatChar("ب", 501)
	_, err = service.Complete(context.Background(), entry501.ID, entry501.Version, teacherID, &grade, &note501)
	if err == nil {
		t.Fatal("complete with 501-char note succeeded")
	}
	var qerr *QueueError
	if !errors.As(err, &qerr) || qerr.Code != QueueErrorCodeValidation {
		t.Fatalf("501-char note error = %v, want validation", err)
	}
	if progressForEntry(t, repo, entry501.ID) != nil {
		t.Fatal("501-char note attempt created a progress row")
	}
}

func TestTurnServiceCompleteGradingRequiredRejectsMissingGrade(t *testing.T) {
	repo, teacherID, sessionID, students, _, _ := createTurnServiceFixture(t, 1)
	service := NewTurnService(repo)
	_, entry := createTurnServiceActiveRound(t, repo, sessionID, teacherID, students[0], true)
	_, err := service.Complete(context.Background(), entry.ID, entry.Version, teacherID, nil, nil)
	if err == nil {
		t.Fatal("grading-required completion without grade succeeded")
	}
	var qerr *QueueError
	if !errors.As(err, &qerr) || qerr.Code != QueueErrorCodeValidation {
		t.Fatalf("missing grade error = %v, want validation", err)
	}
	if progressForEntry(t, repo, entry.ID) != nil {
		t.Fatal("missing grade attempt created a progress row")
	}
}

func TestTurnServiceCompleteGradingOptionalRejectsGradeAndNote(t *testing.T) {
	repo, teacherID, sessionID, students, _, _ := createTurnServiceFixture(t, 2)
	service := NewTurnService(repo)
	grade := GradeGood
	note := "optional note"

	_, entryWithGrade := createTurnServiceActiveRound(t, repo, sessionID, teacherID, students[0], false)
	_, err := service.Complete(context.Background(), entryWithGrade.ID, entryWithGrade.Version, teacherID, &grade, nil)
	if err == nil {
		t.Fatal("grading-optional completion with grade succeeded")
	}
	var qerr *QueueError
	if !errors.As(err, &qerr) || qerr.Code != QueueErrorCodeValidation {
		t.Fatalf("grade on optional error = %v, want validation", err)
	}

	_, entryWithNote := createTurnServiceActiveRound(t, repo, sessionID, teacherID, students[1], false)
	_, err = service.Complete(context.Background(), entryWithNote.ID, entryWithNote.Version, teacherID, nil, &note)
	if err == nil {
		t.Fatal("grading-optional completion with note succeeded")
	}
	if !errors.As(err, &qerr) || qerr.Code != QueueErrorCodeValidation {
		t.Fatalf("note on optional error = %v, want validation", err)
	}
}

func TestTurnServiceCompleteGradingOptionalCreatesProgressWithoutGrade(t *testing.T) {
	repo, teacherID, sessionID, students, _, _ := createTurnServiceFixture(t, 1)
	service := NewTurnService(repo)
	_, entry := createTurnServiceActiveRound(t, repo, sessionID, teacherID, students[0], false)
	completed, err := service.Complete(context.Background(), entry.ID, entry.Version, teacherID, nil, nil)
	if err != nil {
		t.Fatalf("grading-optional completion: %v", err)
	}
	if completed.Status != EntryStatusCompleted || completed.Grade != nil || completed.TeacherNotes != nil {
		t.Fatalf("optional completion = %+v, want completed with nil grade/note", completed)
	}
	progress := progressForEntry(t, repo, entry.ID)
	if progress == nil {
		t.Fatal("grading-optional completion created no progress row")
	}
	if progress.Grade != nil || progress.Notes != nil {
		t.Fatalf("optional progress = %+v, want nil grade/note", progress)
	}
}

func TestTurnServiceCompleteSkipsOptOutsCreateZeroProgress(t *testing.T) {
	repo, teacherID, _, students, round, state := createTurnServiceFixture(t, 2)
	service := NewTurnService(repo)

	// Start then skip the first selected entry.
	firstEntry := findEntry(t, state, students[0])
	insertSelectedEntry(t, repo, round.ID, firstEntry.ID, round.Version)
	started := transitionEntryStatus(t, repo, firstEntry.ID, EntryStatusWaiting, EntryStatusReciting)
	if _, err := service.Skip(context.Background(), started.ID, started.Version, teacherID); err != nil {
		t.Fatalf("skip reciting entry: %v", err)
	}
	if progressForEntry(t, repo, firstEntry.ID) != nil {
		t.Fatal("skipped reciting entry created a progress row")
	}

	// Skip a still-waiting entry.
	state = queueStateForManager(t, repo, round.ID, teacherID)
	waitingEntry := findEntry(t, state, students[1])
	if _, err := service.Skip(context.Background(), waitingEntry.ID, waitingEntry.Version, teacherID); err != nil {
		t.Fatalf("skip waiting entry: %v", err)
	}
	if progressForEntry(t, repo, waitingEntry.ID) != nil {
		t.Fatal("skipped waiting entry created a progress row")
	}

	// Simulate an opted-out entry in a fresh active round.
	_, optedOutEntry := createTurnServiceActiveRound(t, repo, round.SessionID, teacherID, students[0], true)
	transitionEntryStatus(t, repo, optedOutEntry.ID, EntryStatusReciting, EntryStatusOptedOut)
	if progressForEntry(t, repo, optedOutEntry.ID) != nil {
		t.Fatal("opted-out entry created a progress row")
	}
}

func createTurnServiceActiveRoundWithType(t *testing.T, repo *Repository, sessionID, createdBy, studentID string, roundType RoundType, surahID, fromAyah, toAyah int, gradingRequired bool) (Round, QueueEntry) {
	t.Helper()
	ctx := context.Background()
	var round Round
	var entry QueueEntry
	err := repo.WithTx(ctx, func(tx *Tx) error {
		if err := tx.LockRoundAllocation(ctx, sessionID); err != nil {
			return err
		}
		existing, err := tx.LockActiveRound(ctx, sessionID)
		if err == nil {
			policy, err := tx.LockSessionPolicy(ctx, sessionID)
			if err != nil {
				return err
			}
			if _, err := tx.FinalizeRound(ctx, existing.ID, existing.Version, policy.Policy.Finalization, createdBy); err != nil {
				return err
			}
		} else if !errors.Is(err, pgx.ErrNoRows) {
			return err
		}
		number, err := tx.NextRoundNumber(ctx, sessionID)
		if err != nil {
			return err
		}
		round, err = tx.CreateRound(ctx, NewRound{
			SessionID: sessionID, RoundNumber: number, Type: roundType, SurahID: surahID,
			FromAyah: fromAyah, ToAyah: toAyah, GradingRequired: gradingRequired, Lifecycle: RoundLifecyclePrepared, CreatedBy: createdBy,
		})
		if err != nil {
			return err
		}
		entry, err = tx.InsertQueueEntry(ctx, round.ID, studentID, 1)
		if err != nil {
			return err
		}
		round, err = tx.ActivateRound(ctx, round.ID)
		return err
	})
	if err != nil {
		t.Fatalf("create active round with type: %v", err)
	}
	insertSelectedEntry(t, repo, round.ID, entry.ID, round.Version)
	transitionEntryStatus(t, repo, entry.ID, EntryStatusWaiting, EntryStatusReciting)
	state := queueStateForManager(t, repo, round.ID, createdBy)
	return state.Round, findEntry(t, state, studentID)
}

func TestTurnServiceCompleteTestTypeRetained(t *testing.T) {
	repo, teacherID, sessionID, students, _, _ := createTurnServiceFixture(t, 1)
	service := NewTurnService(repo)
	_, entry := createTurnServiceActiveRoundWithType(t, repo, sessionID, teacherID, students[0], RoundTypeTest, 2, 1, 3, true)

	grade := GradeAcceptable
	completed, err := service.Complete(context.Background(), entry.ID, entry.Version, teacherID, &grade, nil)
	if err != nil {
		t.Fatalf("complete test entry: %v", err)
	}
	if completed.Status != EntryStatusCompleted {
		t.Fatalf("test entry status = %s, want completed", completed.Status)
	}
	progress := progressForEntry(t, repo, entry.ID)
	if progress == nil {
		t.Fatal("test entry created no progress row")
	}
	if progress.Type != RoundTypeTest {
		t.Fatalf("progress type = %s, want test", progress.Type)
	}
	if progress.SurahID != 2 || progress.FromAyah != 1 || progress.ToAyah != 3 {
		t.Fatalf("progress range = %d-%d in surah %d, want 1-3 in surah 2", progress.FromAyah, progress.ToAyah, progress.SurahID)
	}
}

func TestTurnServiceCorrectBeforeRoundFinalization(t *testing.T) {
	repo, teacherID, sessionID, students, _, _ := createTurnServiceFixture(t, 1)
	setGradeCorrectionPolicy(t, repo, sessionID, GradeCorrectionBeforeRoundFinalization)
	service := NewTurnService(repo)
	_, entry := createTurnServiceActiveRound(t, repo, sessionID, teacherID, students[0], true)
	grade := GradeGood
	note := "first note"
	completed, err := service.Complete(context.Background(), entry.ID, entry.Version, teacherID, &grade, &note)
	if err != nil {
		t.Fatalf("complete: %v", err)
	}

	newGrade := GradeExcellent
	newNote := "corrected note"
	corrected, err := service.Correct(context.Background(), entry.ID, completed.Version, teacherID, &newGrade, &newNote)
	if err != nil {
		t.Fatalf("correct before finalization: %v", err)
	}
	if corrected.Grade == nil || *corrected.Grade != newGrade {
		t.Fatalf("corrected grade = %v, want %s", corrected.Grade, newGrade)
	}
	if corrected.TeacherNotes == nil || *corrected.TeacherNotes != newNote {
		t.Fatalf("corrected notes = %v, want %q", corrected.TeacherNotes, newNote)
	}
	progress := progressForEntry(t, repo, entry.ID)
	if progress == nil || progress.Grade == nil || *progress.Grade != newGrade {
		t.Fatalf("progress after correction = %+v, want %s", progress, newGrade)
	}
	if countProgressRows(t, repo, sessionID) != 1 {
		t.Fatalf("progress rows = %d, want 1", countProgressRows(t, repo, sessionID))
	}
}

func TestTurnServiceCorrectImmutableRejected(t *testing.T) {
	repo, teacherID, sessionID, students, _, _ := createTurnServiceFixture(t, 1)
	setGradeCorrectionPolicy(t, repo, sessionID, GradeCorrectionImmutable)
	service := NewTurnService(repo)
	_, entry := createTurnServiceActiveRound(t, repo, sessionID, teacherID, students[0], true)
	grade := GradeGood
	completed, err := service.Complete(context.Background(), entry.ID, entry.Version, teacherID, &grade, nil)
	if err != nil {
		t.Fatalf("complete: %v", err)
	}

	newGrade := GradeExcellent
	_, err = service.Correct(context.Background(), entry.ID, completed.Version, teacherID, &newGrade, nil)
	if err == nil {
		t.Fatal("immutable correction succeeded")
	}
	var qerr *QueueError
	if !errors.As(err, &qerr) || qerr.Code != QueueErrorCodeInvalidTransition {
		t.Fatalf("immutable correction error = %v, want invalid_transition", err)
	}
	progress := progressForEntry(t, repo, entry.ID)
	if progress == nil || progress.Grade == nil || *progress.Grade != grade {
		t.Fatalf("immutable correction changed progress to %+v", progress)
	}
}

func TestTurnServiceCorrectAuditedAnyTimeAfterFinalization(t *testing.T) {
	repo, teacherID, sessionID, students, _, _ := createTurnServiceFixture(t, 1)
	setGradeCorrectionPolicy(t, repo, sessionID, GradeCorrectionAuditedAnyTime)
	service := NewTurnService(repo)
	_, entry := createTurnServiceActiveRound(t, repo, sessionID, teacherID, students[0], true)
	grade := GradeGood
	completed, err := service.Complete(context.Background(), entry.ID, entry.Version, teacherID, &grade, nil)
	if err != nil {
		t.Fatalf("complete: %v", err)
	}

	// Finalize the round directly.
	err = repo.WithTx(context.Background(), func(tx *Tx) error {
		round, err := tx.LockRound(context.Background(), entry.QueueID)
		if err != nil {
			return err
		}
		_, err = tx.FinalizeRound(context.Background(), round.ID, round.Version, FinalizationPolicyMarkUnfinishedSkipped, teacherID)
		return err
	})
	if err != nil {
		t.Fatalf("finalize round: %v", err)
	}

	newGrade := GradeExcellent
	corrected, err := service.Correct(context.Background(), entry.ID, completed.Version, teacherID, &newGrade, nil)
	if err != nil {
		t.Fatalf("correct after finalization under audited_any_time: %v", err)
	}
	if corrected.Grade == nil || *corrected.Grade != newGrade {
		t.Fatalf("corrected grade = %v, want %s", corrected.Grade, newGrade)
	}
}

func TestTurnServiceCorrectBeforeFinalizationRejectedAfterFinalize(t *testing.T) {
	repo, teacherID, sessionID, students, _, _ := createTurnServiceFixture(t, 1)
	setGradeCorrectionPolicy(t, repo, sessionID, GradeCorrectionBeforeRoundFinalization)
	service := NewTurnService(repo)
	_, entry := createTurnServiceActiveRound(t, repo, sessionID, teacherID, students[0], true)
	grade := GradeGood
	completed, err := service.Complete(context.Background(), entry.ID, entry.Version, teacherID, &grade, nil)
	if err != nil {
		t.Fatalf("complete: %v", err)
	}

	err = repo.WithTx(context.Background(), func(tx *Tx) error {
		round, err := tx.LockRound(context.Background(), entry.QueueID)
		if err != nil {
			return err
		}
		_, err = tx.FinalizeRound(context.Background(), round.ID, round.Version, FinalizationPolicyMarkUnfinishedSkipped, teacherID)
		return err
	})
	if err != nil {
		t.Fatalf("finalize round: %v", err)
	}

	newGrade := GradeExcellent
	_, err = service.Correct(context.Background(), entry.ID, completed.Version, teacherID, &newGrade, nil)
	if err == nil {
		t.Fatal("correction after finalization succeeded")
	}
	var qerr *QueueError
	if !errors.As(err, &qerr) || qerr.Code != QueueErrorCodeRoundFinalized {
		t.Fatalf("correction after finalization error = %v, want round_finalized", err)
	}
}

func TestTurnServiceCorrectRepeatedIdenticalConverges(t *testing.T) {
	repo, teacherID, sessionID, students, _, _ := createTurnServiceFixture(t, 1)
	service := NewTurnService(repo)
	_, entry := createTurnServiceActiveRound(t, repo, sessionID, teacherID, students[0], true)
	grade := GradeGood
	note := "consistent note"
	completed, err := service.Complete(context.Background(), entry.ID, entry.Version, teacherID, &grade, &note)
	if err != nil {
		t.Fatalf("complete: %v", err)
	}

	for i := 0; i < 3; i++ {
		corrected, err := service.Correct(context.Background(), entry.ID, completed.Version+int64(i), teacherID, &grade, &note)
		if err != nil {
			t.Fatalf("identical correction %d: %v", i+1, err)
		}
		if corrected.Grade == nil || *corrected.Grade != grade {
			t.Fatalf("identical correction %d grade = %v", i+1, corrected.Grade)
		}
		if countProgressRows(t, repo, sessionID) != 1 {
			t.Fatalf("identical correction %d created extra progress rows", i+1)
		}
	}
}

func TestTurnServiceCorrectClearsNoteToNull(t *testing.T) {
	repo, teacherID, sessionID, students, _, _ := createTurnServiceFixture(t, 1)
	service := NewTurnService(repo)
	_, entry := createTurnServiceActiveRound(t, repo, sessionID, teacherID, students[0], true)
	grade := GradeGood
	note := "to be cleared"
	completed, err := service.Complete(context.Background(), entry.ID, entry.Version, teacherID, &grade, &note)
	if err != nil {
		t.Fatalf("complete: %v", err)
	}

	nullNote := ""
	corrected, err := service.Correct(context.Background(), entry.ID, completed.Version, teacherID, &grade, &nullNote)
	if err != nil {
		t.Fatalf("correct clearing note: %v", err)
	}
	if corrected.TeacherNotes != nil {
		t.Fatalf("corrected notes = %v, want nil", corrected.TeacherNotes)
	}
	progress := progressForEntry(t, repo, entry.ID)
	if progress == nil || progress.Notes != nil {
		t.Fatalf("progress notes = %v, want nil", progress)
	}
}

func TestTurnServiceCorrectRejectsNonCompletedEntry(t *testing.T) {
	repo, teacherID, sessionID, students, _, _ := createTurnServiceFixture(t, 1)
	service := NewTurnService(repo)
	round, entry := createTurnServiceActiveRound(t, repo, sessionID, teacherID, students[0], true)
	grade := GradeGood
	_, err := service.Correct(context.Background(), entry.ID, entry.Version, teacherID, &grade, nil)
	if err == nil {
		t.Fatal("correct non-completed entry succeeded")
	}
	var qerr *QueueError
	if !errors.As(err, &qerr) || qerr.Code != QueueErrorCodeInvalidTransition {
		t.Fatalf("correct non-completed error = %v, want invalid_transition", err)
	}
	_ = round
}

func TestTurnServiceCorrectRejectsStaleEntryVersion(t *testing.T) {
	repo, teacherID, sessionID, students, _, _ := createTurnServiceFixture(t, 1)
	service := NewTurnService(repo)
	_, entry := createTurnServiceActiveRound(t, repo, sessionID, teacherID, students[0], true)
	grade := GradeGood
	completed, err := service.Complete(context.Background(), entry.ID, entry.Version, teacherID, &grade, nil)
	if err != nil {
		t.Fatalf("complete: %v", err)
	}

	newGrade := GradeExcellent
	_, err = service.Correct(context.Background(), entry.ID, entry.Version, teacherID, &newGrade, nil)
	if err == nil {
		t.Fatal("correct with stale entry version succeeded")
	}
	var qerr *QueueError
	if !errors.As(err, &qerr) || qerr.Code != QueueErrorCodeStaleVersion {
		t.Fatalf("stale correction error = %v, want stale_version", err)
	}
	_ = completed
}
