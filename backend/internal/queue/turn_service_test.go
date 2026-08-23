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

type audioCall struct {
	sessionID    string
	roundID      string
	queueEntryID string
	userID       string
}

type fakeReciterAudioControl struct {
	grantErr    error
	revokeErr   error
	grantCalls  []audioCall
	revokeCalls []audioCall
	onGrant     func(audioCall) error
	onRevoke    func(audioCall) error
}

func (f *fakeReciterAudioControl) GrantReciterAudio(_ context.Context, sessionID, roundID, queueEntryID, userID string) error {
	call := audioCall{sessionID: sessionID, roundID: roundID, queueEntryID: queueEntryID, userID: userID}
	f.grantCalls = append(f.grantCalls, call)
	if f.onGrant != nil {
		if err := f.onGrant(call); err != nil {
			return err
		}
	}
	return f.grantErr
}

func (f *fakeReciterAudioControl) RevokeReciterAudio(_ context.Context, sessionID, roundID, queueEntryID, userID string) error {
	call := audioCall{sessionID: sessionID, roundID: roundID, queueEntryID: queueEntryID, userID: userID}
	f.revokeCalls = append(f.revokeCalls, call)
	if f.onRevoke != nil {
		if err := f.onRevoke(call); err != nil {
			return err
		}
	}
	return f.revokeErr
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

func TestTurnServiceAudioConvergenceClassification(t *testing.T) {
	if IsAudioConvergencePending(nil) {
		t.Fatal("nil error classified as audio convergence pending")
	}
	if !IsAudioConvergencePending(&QueueError{Code: QueueErrorCodeAudioConvergencePending}) {
		t.Fatal("audio convergence error was not classified")
	}
	if IsAudioConvergencePending(&QueueError{Code: QueueErrorCodeStaleVersion}) {
		t.Fatal("stale version classified as audio convergence pending")
	}
}

func TestTurnServiceAdvanceReplacesSelectionWithoutDuplicate(t *testing.T) {
	repo, teacherID, _, students, round, state := createTurnServiceFixture(t, 2)
	service := NewTurnService(repo, nil)

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
	service := NewTurnService(repo, nil)

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
	service := NewTurnService(repo, nil)

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
	service := NewTurnService(repo, nil)

	firstEntry := findEntry(t, state, students[0])
	secondEntry := findEntry(t, state, students[1])

	if _, err := service.Start(context.Background(), firstEntry.ID, round.Version); err == nil {
		t.Fatal("start without selection succeeded")
	}

	selectedRound := insertSelectedEntry(t, repo, round.ID, secondEntry.ID, round.Version)
	if _, err := service.Start(context.Background(), firstEntry.ID, selectedRound.Version); err == nil {
		t.Fatal("start for unselected entry succeeded")
	}

	if _, err := service.Start(context.Background(), secondEntry.ID, selectedRound.Version); err != nil {
		t.Fatalf("start selected entry: %v", err)
	}

	state = queueStateForManager(t, repo, round.ID, teacherID)
	entry := findEntry(t, state, students[1])
	if entry.Status != EntryStatusReciting {
		t.Fatalf("selected entry status = %s, want reciting", entry.Status)
	}
}

func TestTurnServiceStartUsesOneReciterDatabaseGuard(t *testing.T) {
	repo, teacherID, _, students, round, state := createTurnServiceFixture(t, 2)
	service := NewTurnService(repo, nil)

	firstEntry := findEntry(t, state, students[0])
	secondEntry := findEntry(t, state, students[1])
	reciting := transitionEntryStatus(t, repo, firstEntry.ID, EntryStatusWaiting, EntryStatusReciting)
	selectedRound := insertSelectedEntry(t, repo, round.ID, secondEntry.ID, round.Version)

	_, err := service.Start(context.Background(), secondEntry.ID, selectedRound.Version)
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
	control := &fakeReciterAudioControl{}
	service := NewTurnService(repo, control)

	waitingSkipped, err := service.Skip(context.Background(), waitingEntry.ID, round.Version, teacherID)
	if err != nil {
		t.Fatalf("skip waiting entry: %v", err)
	}
	if waitingSkipped.Status != EntryStatusSkipped {
		t.Fatalf("waiting skip status = %s, want skipped", waitingSkipped.Status)
	}
	if len(control.revokeCalls) != 0 {
		t.Fatalf("waiting skip revoke calls = %d, want 0", len(control.revokeCalls))
	}

	afterWaiting := roundState(t, repo, round.ID)
	recitingSkipped, err := service.Skip(context.Background(), recitingEntry.ID, afterWaiting.Version, teacherID)
	if err != nil {
		t.Fatalf("skip reciting entry: %v", err)
	}
	if recitingSkipped.Status != EntryStatusSkipped {
		t.Fatalf("reciting skip status = %s, want skipped", recitingSkipped.Status)
	}
	if len(control.revokeCalls) != 1 || control.revokeCalls[0].queueEntryID != recitingEntry.ID {
		t.Fatalf("revoke calls = %+v, want one revoke for %s", control.revokeCalls, recitingEntry.ID)
	}

	state = queueStateForManager(t, repo, round.ID, teacherID)
	for _, entry := range state.Entries {
		if entry.Status != EntryStatusSkipped {
			t.Fatalf("entry %s status = %s, want skipped", entry.ID, entry.Status)
		}
	}
}

func TestTurnServiceStartGrantsAudioAfterCommit(t *testing.T) {
	repo, teacherID, _, students, round, state := createTurnServiceFixture(t, 1)
	entry := findEntry(t, state, students[0])
	selectedRound := insertSelectedEntry(t, repo, round.ID, entry.ID, round.Version)

	control := &fakeReciterAudioControl{}
	control.onGrant = func(call audioCall) error {
		committedRound := roundState(t, repo, call.roundID)
		if committedRound.Version != selectedRound.Version+1 {
			return fmt.Errorf("round version at grant = %d, want %d", committedRound.Version, selectedRound.Version+1)
		}
		committed := queueStateForManager(t, repo, call.roundID, teacherID)
		started := findEntry(t, committed, students[0])
		if started.Status != EntryStatusReciting {
			return fmt.Errorf("entry status at grant = %s, want reciting", started.Status)
		}
		if outboxCount(t, repo, call.roundID) != 1 {
			return fmt.Errorf("outbox count at grant = %d, want 1", outboxCount(t, repo, call.roundID))
		}
		return nil
	}
	service := NewTurnService(repo, control)

	started, err := service.Start(context.Background(), entry.ID, selectedRound.Version)
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	if started.Status != EntryStatusReciting {
		t.Fatalf("started status = %s, want reciting", started.Status)
	}
	if len(control.grantCalls) != 1 || control.grantCalls[0].queueEntryID != entry.ID {
		t.Fatalf("grant calls = %+v, want one call for %s", control.grantCalls, entry.ID)
	}
}

func TestTurnServiceSkipRevokesAudioBeforeNextReciterStarts(t *testing.T) {
	repo, teacherID, _, students, round, state := createTurnServiceFixture(t, 2)
	firstEntry := findEntry(t, state, students[0])
	secondEntry := findEntry(t, state, students[1])

	var callOrder []string
	control := &fakeReciterAudioControl{}
	control.onGrant = func(call audioCall) error {
		callOrder = append(callOrder, "grant:"+call.queueEntryID)
		return nil
	}
	control.onRevoke = func(call audioCall) error {
		callOrder = append(callOrder, "revoke:"+call.queueEntryID)
		committed := queueStateForManager(t, repo, call.roundID, teacherID)
		if got := recitingEntries(committed); len(got) != 0 {
			return fmt.Errorf("reciting entries at revoke = %d, want 0", len(got))
		}
		return nil
	}
	service := NewTurnService(repo, control)

	selectedRound := insertSelectedEntry(t, repo, round.ID, firstEntry.ID, round.Version)
	if _, err := service.Start(context.Background(), firstEntry.ID, selectedRound.Version); err != nil {
		t.Fatalf("start first entry: %v", err)
	}

	afterStart := roundState(t, repo, round.ID)
	if _, err := service.Skip(context.Background(), firstEntry.ID, afterStart.Version, teacherID); err != nil {
		t.Fatalf("skip first entry: %v", err)
	}

	afterSkip := roundState(t, repo, round.ID)
	if afterSkip.SelectedEntryID != nil {
		t.Fatalf("selection after skip = %v, want cleared", afterSkip.SelectedEntryID)
	}
	nextSelected, err := service.Advance(context.Background(), round.ID, afterSkip.Version)
	if err != nil {
		t.Fatalf("advance next waiting entry: %v", err)
	}
	if nextSelected.SelectedEntryID == nil || *nextSelected.SelectedEntryID != secondEntry.ID {
		t.Fatalf("next selection = %v, want %s", nextSelected.SelectedEntryID, secondEntry.ID)
	}
	if _, err := service.Start(context.Background(), secondEntry.ID, nextSelected.Version); err != nil {
		t.Fatalf("start second entry: %v", err)
	}

	wantOrder := []string{
		"grant:" + firstEntry.ID,
		"revoke:" + firstEntry.ID,
		"grant:" + secondEntry.ID,
	}
	if len(callOrder) != len(wantOrder) {
		t.Fatalf("call order = %v, want %v", callOrder, wantOrder)
	}
	for i := range wantOrder {
		if callOrder[i] != wantOrder[i] {
			t.Fatalf("call order = %v, want %v", callOrder, wantOrder)
		}
	}
}

func TestTurnServiceStartReturnsCommittedTruthOnAudioFailure(t *testing.T) {
	repo, teacherID, _, students, round, state := createTurnServiceFixture(t, 1)
	entry := findEntry(t, state, students[0])
	selectedRound := insertSelectedEntry(t, repo, round.ID, entry.ID, round.Version)

	control := &fakeReciterAudioControl{grantErr: context.DeadlineExceeded}
	service := NewTurnService(repo, control)

	started, err := service.Start(context.Background(), entry.ID, selectedRound.Version)
	if err == nil {
		t.Fatal("start with audio failure succeeded")
	}
	if !IsAudioConvergencePending(err) {
		t.Fatalf("start audio failure error = %v, want audio convergence pending", err)
	}
	if started.Status != EntryStatusReciting {
		t.Fatalf("returned entry status = %s, want reciting", started.Status)
	}

	committed := queueStateForManager(t, repo, round.ID, teacherID)
	persisted := findEntry(t, committed, students[0])
	if persisted.Status != EntryStatusReciting {
		t.Fatalf("persisted status = %s, want reciting", persisted.Status)
	}
	if outboxCount(t, repo, round.ID) != 1 {
		t.Fatalf("outbox count = %d, want 1", outboxCount(t, repo, round.ID))
	}
}

func TestTurnServiceSkipReturnsCommittedTruthOnAudioFailure(t *testing.T) {
	repo, teacherID, _, students, round, state := createTurnServiceFixture(t, 1)
	entry := findEntry(t, state, students[0])
	entry = transitionEntryStatus(t, repo, entry.ID, EntryStatusWaiting, EntryStatusReciting)

	control := &fakeReciterAudioControl{revokeErr: context.DeadlineExceeded}
	service := NewTurnService(repo, control)

	skipped, err := service.Skip(context.Background(), entry.ID, round.Version, teacherID)
	if err == nil {
		t.Fatal("skip with audio failure succeeded")
	}
	if !IsAudioConvergencePending(err) {
		t.Fatalf("skip audio failure error = %v, want audio convergence pending", err)
	}
	if skipped.Status != EntryStatusSkipped {
		t.Fatalf("returned entry status = %s, want skipped", skipped.Status)
	}

	committed := queueStateForManager(t, repo, round.ID, teacherID)
	persisted := findEntry(t, committed, students[0])
	if persisted.Status != EntryStatusSkipped {
		t.Fatalf("persisted status = %s, want skipped", persisted.Status)
	}
	if len(control.revokeCalls) != 1 {
		t.Fatalf("revoke calls = %d, want 1", len(control.revokeCalls))
	}
}

func TestTurnServiceStartWithExistingReciterLeavesCommittedTruth(t *testing.T) {
	repo, teacherID, _, students, round, state := createTurnServiceFixture(t, 2)
	firstEntry := findEntry(t, state, students[0])
	secondEntry := findEntry(t, state, students[1])

	_ = transitionEntryStatus(t, repo, firstEntry.ID, EntryStatusWaiting, EntryStatusReciting)
	selectedRound := insertSelectedEntry(t, repo, round.ID, secondEntry.ID, round.Version)
	service := NewTurnService(repo, nil)

	_, err := service.Start(context.Background(), secondEntry.ID, selectedRound.Version)
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

func TestTurnServiceStartReportsStaleVersionWhenRoundChanged(t *testing.T) {
	repo, _, _, students, round, state := createTurnServiceFixture(t, 1)
	entry := findEntry(t, state, students[0])
	selectedRound := insertSelectedEntry(t, repo, round.ID, entry.ID, round.Version)
	service := NewTurnService(repo, nil)

	if _, err := service.Start(context.Background(), entry.ID, round.Version); err == nil {
		t.Fatal("start with stale round version succeeded")
	}
	if selectedRound.Version == round.Version {
		t.Fatalf("selected round version = %d, want > %d", selectedRound.Version, round.Version)
	}
}

func TestTurnServiceSkipReportsStaleVersionWhenRoundChanged(t *testing.T) {
	repo, teacherID, _, students, round, state := createTurnServiceFixture(t, 1)
	entry := findEntry(t, state, students[0])
	service := NewTurnService(repo, nil)

	selectedRound := insertSelectedEntry(t, repo, round.ID, entry.ID, round.Version)
	if _, err := service.Skip(context.Background(), entry.ID, round.Version, teacherID); err == nil {
		t.Fatal("skip with stale round version succeeded")
	}
	if selectedRound.Version == round.Version {
		t.Fatalf("selected round version = %d, want > %d", selectedRound.Version, round.Version)
	}
}
