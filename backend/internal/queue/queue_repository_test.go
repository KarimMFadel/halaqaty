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
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

var queueRepoMigrations = []string{
	"000010_auth_roles_profile.up.sql",
	"000011_auth_roles_profile_alignment.up.sql",
	"000012_auth_profiles_display_name.up.sql",
	"000013_create_circles.up.sql",
	"000014_circle_members_circle_fk.up.sql",
	"000015_circle_management.up.sql",
	"000016_live_sessions.up.sql",
	"000017_recitation_queue_system.up.sql",
}

// newQueueRepository opens an isolated schema with the full migration chain
// applied and returns a queue repository bound to that schema, mirroring the
// sessions internal-test pattern.
func newQueueRepository(t *testing.T) *Repository {
	t.Helper()
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		t.Skip("DATABASE_URL is not set")
	}
	ctx := context.Background()
	schema := fmt.Sprintf("test_queue_repo_%d", time.Now().UnixNano())
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
	for _, name := range queueRepoMigrations {
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

func qSeedUser(t *testing.T, repo *Repository, label string) string {
	t.Helper()
	var id string
	if err := repo.pool.QueryRow(context.Background(), `
		INSERT INTO users (firebase_uid, email)
		VALUES ($1, $2)
		RETURNING id::text
	`, "firebase-queue-"+label, label+"@example.com").Scan(&id); err != nil {
		t.Fatalf("seed user %s: %v", label, err)
	}
	return id
}

// qSeedUserWithDisplayName also seeds a profile row whose display name must
// never leak into receipts or outbox metadata.
func qSeedUserWithDisplayName(t *testing.T, repo *Repository, label, displayName string) string {
	t.Helper()
	id := qSeedUser(t, repo, label)
	if _, err := repo.pool.Exec(context.Background(), `
		INSERT INTO profiles (user_id, display_name) VALUES ($1::uuid, $2)
	`, id, displayName); err != nil {
		t.Fatalf("seed profile %s: %v", label, err)
	}
	return id
}

func qSeedCircle(t *testing.T, repo *Repository, teacherID string) string {
	t.Helper()
	var id string
	if err := repo.pool.QueryRow(context.Background(), `
		INSERT INTO circles (name, teacher_id, invite_code)
		VALUES ('Queue Repo Circle', $1::uuid, 'HLQ-QREPO1')
		RETURNING id::text
	`, teacherID).Scan(&id); err != nil {
		t.Fatalf("seed circle: %v", err)
	}
	return id
}

func qSeedMember(t *testing.T, repo *Repository, circleID, userID, role string, joinedAt time.Time) {
	t.Helper()
	if _, err := repo.pool.Exec(context.Background(), `
		INSERT INTO circle_members (circle_id, user_id, role, joined_at)
		VALUES ($1::uuid, $2::uuid, $3, $4)
	`, circleID, userID, role, joinedAt); err != nil {
		t.Fatalf("seed member %s (%s): %v", userID, role, err)
	}
}

func qInsertSession(t *testing.T, repo *Repository, circleID, creatorID, gradeVisibility string) string {
	t.Helper()
	var id string
	if err := repo.pool.QueryRow(context.Background(), `
		INSERT INTO sessions (circle_id, created_by, queue_grade_visibility)
		VALUES ($1::uuid, $2::uuid, $3)
		RETURNING id::text
	`, circleID, creatorID, gradeVisibility).Scan(&id); err != nil {
		t.Fatalf("seed session: %v", err)
	}
	return id
}

func qInsertPresence(t *testing.T, repo *Repository, sessionID, userID string, firstJoinedAt time.Time, present bool) {
	t.Helper()
	if _, err := repo.pool.Exec(context.Background(), `
		INSERT INTO session_participant_presence (session_id, user_id, first_joined_at, last_joined_at, is_currently_present)
		VALUES ($1::uuid, $2::uuid, $3, $3, $4)
	`, sessionID, userID, firstJoinedAt, present); err != nil {
		t.Fatalf("seed presence %s: %v", userID, err)
	}
}

func qInsertPreorder(t *testing.T, repo *Repository, queueID, studentID, addedBy string, position int) {
	t.Helper()
	if _, err := repo.pool.Exec(context.Background(), `
		INSERT INTO recitation_queue_preorder (queue_id, student_id, position, added_by)
		VALUES ($1::uuid, $2::uuid, $3, $4::uuid)
	`, queueID, studentID, position, addedBy); err != nil {
		t.Fatalf("seed preorder %d: %v", position, err)
	}
}

// qCreateRound creates one round in the requested lifecycle with optional
// student entries materialized in the given order, all inside a single
// transaction that takes the per-session round allocation lock.
func qCreateRound(t *testing.T, repo *Repository, sessionID, createdBy, lifecycle string, studentIDs []string) Round {
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
			Lifecycle:       RoundLifecycle(lifecycle),
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

// qCompleteEntry runs the advance→start→complete chain for the next waiting
// entry under one transaction: it locks the round, selects the next waiting
// entry, replaces the selection, transitions the entry through reciting to
// completed with the given grade and note, bumps the round version, and
// upserts the progress row. It returns the completed entry and the round
// version after the completion bump.
func qCompleteEntry(t *testing.T, repo *Repository, round Round, teacherID string, grade Grade, note string) (QueueEntry, int64) {
	t.Helper()
	ctx := context.Background()
	var entry QueueEntry
	var roundVersion int64
	err := repo.WithTx(ctx, func(tx *Tx) error {
		locked, err := tx.LockRound(ctx, round.ID)
		if err != nil {
			return err
		}
		next, err := tx.SelectNextWaitingEntry(ctx, round.ID)
		if err != nil {
			return err
		}
		if next == nil {
			return errors.New("no waiting entry left")
		}
		if _, err := tx.SetRoundSelection(ctx, round.ID, &next.ID, locked.Version); err != nil {
			return err
		}
		reciting, err := tx.TransitionEntry(ctx, next.ID, EntryStatusWaiting, next.Version, EntryStatusReciting, nil, nil, nil)
		if err != nil {
			return err
		}
		notes := note
		completed, err := tx.TransitionEntry(ctx, reciting.ID, EntryStatusReciting, reciting.Version, EntryStatusCompleted, &grade, &notes, &teacherID)
		if err != nil {
			return err
		}
		if err := tx.BumpRoundVersion(ctx, round.ID); err != nil {
			return err
		}
		if err := tx.UpsertProgress(ctx, NewProgress{
			StudentID:    completed.StudentID,
			CircleID:     "",
			SessionID:    round.SessionID,
			QueueEntryID: completed.ID,
			SurahID:      1,
			SurahName:    "Al-Fatihah",
			FromAyah:     1,
			ToAyah:       7,
			Type:         RoundTypeRevision,
			Grade:        &grade,
			Notes:        &notes,
			Date:         time.Now().UTC(),
		}); err != nil {
			return err
		}
		entry = completed
		roundVersion = locked.Version + 2 // selection bump + completion bump
		return nil
	})
	if err != nil {
		t.Fatalf("complete entry: %v", err)
	}
	return entry, roundVersion
}

func qCountRounds(t *testing.T, repo *Repository, sessionID string) int {
	t.Helper()
	var count int
	if err := repo.pool.QueryRow(context.Background(),
		`SELECT COUNT(*) FROM recitation_queue WHERE session_id = $1::uuid`, sessionID).Scan(&count); err != nil {
		t.Fatalf("count rounds: %v", err)
	}
	return count
}

// Matrix 1 + 5: a failed mutation inside a repository transaction leaves
// zero rows — round, receipt, and outbox are all absent, proving the outbox
// row lives in the same transaction as the round mutation.
func TestQueueRepository_TransactionRollbackLeavesZeroRows(t *testing.T) {
	repo := newQueueRepository(t)
	ctx := context.Background()
	teacher := qSeedUser(t, repo, "rollback-teacher")
	circle := qSeedCircle(t, repo, teacher)
	session := qInsertSession(t, repo, circle, teacher, "managers_and_student")

	sentinel := errors.New("boom")
	err := repo.WithTx(ctx, func(tx *Tx) error {
		if err := tx.LockRoundAllocation(ctx, session); err != nil {
			return err
		}
		number, err := tx.NextRoundNumber(ctx, session)
		if err != nil {
			return err
		}
		round, err := tx.CreateRound(ctx, NewRound{
			SessionID:       session,
			RoundNumber:     number,
			Type:            RoundTypeRevision,
			SurahID:         1,
			FromAyah:        1,
			ToAyah:          7,
			GradingRequired: true,
			Lifecycle:       RoundLifecycleActive,
			CreatedBy:       teacher,
		})
		if err != nil {
			return err
		}
		if _, _, err := tx.ReserveCommandReceipt(ctx, session, teacher, "create-round-1", "queue.create_round"); err != nil {
			return err
		}
		if err := tx.InsertOutboxEvent(ctx, OutboxEvent{
			EventID:       "22222222-2222-4222-8222-222222222222",
			SessionID:     session,
			RoundID:       round.ID,
			EventType:     "queue.round_started",
			RoundVersion:  1,
			EventMetadata: []byte(`{"lifecycle":"active"}`),
			AttemptCount:  0,
		}); err != nil {
			return err
		}
		return sentinel
	})
	if !errors.Is(err, sentinel) {
		t.Fatalf("rollback tx: got %v, want sentinel", err)
	}
	if got := qCountRounds(t, repo, session); got != 0 {
		t.Fatalf("rounds after rollback: got %d, want 0", got)
	}
	var receipts, outbox int
	if err := repo.pool.QueryRow(ctx,
		`SELECT (SELECT COUNT(*) FROM queue_command_receipts), (SELECT COUNT(*) FROM queue_event_outbox)`).Scan(&receipts, &outbox); err != nil {
		t.Fatalf("count receipts/outbox: %v", err)
	}
	if receipts != 0 || outbox != 0 {
		t.Fatalf("after rollback: receipts=%d outbox=%d, want 0/0", receipts, outbox)
	}
}

// Matrix 2: SELECT ... FOR UPDATE on a round serializes concurrent
// transactions — a second lock on the same round blocks until the holder
// commits (channel-orchestrated two-connection proof).
func TestQueueRepository_RoundLockSerializesAccessors(t *testing.T) {
	repo := newQueueRepository(t)
	ctx := context.Background()
	teacher := qSeedUser(t, repo, "lock-teacher")
	circle := qSeedCircle(t, repo, teacher)
	session := qInsertSession(t, repo, circle, teacher, "managers_and_student")
	round := qCreateRound(t, repo, session, teacher, "active", nil)

	acquired := make(chan struct{})
	release := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		err := repo.WithTx(ctx, func(tx *Tx) error {
			if _, err := tx.LockRound(ctx, round.ID); err != nil {
				return err
			}
			acquired <- struct{}{}
			<-release
			return nil
		})
		if err != nil {
			t.Errorf("holder transaction: %v", err)
		}
	}()

	<-acquired
	blockedCtx, cancel := context.WithTimeout(ctx, 200*time.Millisecond)
	defer cancel()
	start := time.Now()
	err := repo.WithTx(blockedCtx, func(tx *Tx) error {
		_, err := tx.LockRound(blockedCtx, round.ID)
		return err
	})
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("second lock: got %v, want context.DeadlineExceeded", err)
	}
	if elapsed := time.Since(start); elapsed < 150*time.Millisecond {
		t.Fatalf("second lock returned after %v, want it to have blocked", elapsed)
	}

	close(release)
	wg.Wait()
	err = repo.WithTx(ctx, func(tx *Tx) error {
		_, err := tx.LockRound(ctx, round.ID)
		return err
	})
	if err != nil {
		t.Fatalf("lock after holder committed: %v", err)
	}
}

func queueErrCode(t *testing.T, err error) QueueErrorCode {
	t.Helper()
	var qerr *QueueError
	if !errors.As(err, &qerr) {
		t.Fatalf("expected *QueueError, got %T: %v", err, err)
	}
	return qerr.Code
}

// Matrix 3: version-guarded mutations reject stale expected versions with
// the stale_version conflict class, for both rounds and entries, and illegal
// transitions map to invalid_transition.
func TestQueueRepository_OptimisticVersionConflicts(t *testing.T) {
	repo := newQueueRepository(t)
	ctx := context.Background()
	teacher := qSeedUser(t, repo, "version-teacher")
	student := qSeedUser(t, repo, "version-student")
	circle := qSeedCircle(t, repo, teacher)
	session := qInsertSession(t, repo, circle, teacher, "managers_and_student")
	round := qCreateRound(t, repo, session, teacher, "active", nil)
	entry, err := func() (QueueEntry, error) {
		var entry QueueEntry
		err := repo.WithTx(ctx, func(tx *Tx) error {
			var err error
			entry, err = tx.InsertQueueEntry(ctx, round.ID, student, 2)
			return err
		})
		return entry, err
	}()
	if err != nil {
		t.Fatalf("insert second entry: %v", err)
	}

	// Round selection: fresh version applies and bumps; stale version conflicts.
	var updated Round
	err = repo.WithTx(ctx, func(tx *Tx) error {
		var err error
		updated, err = tx.SetRoundSelection(ctx, round.ID, nil, round.Version)
		return err
	})
	if err != nil || updated.Version != round.Version+1 {
		t.Fatalf("selection with fresh version: err=%v version=%d want %d", err, updated.Version, round.Version+1)
	}
	staleErr := repo.WithTx(ctx, func(tx *Tx) error {
		_, err := tx.SetRoundSelection(ctx, round.ID, nil, round.Version)
		return err
	})
	if got := queueErrCode(t, staleErr); got != QueueErrorCodeStaleVersion {
		t.Fatalf("stale round selection: got %s, want stale_version", got)
	}

	// Entry transition: fresh version applies; stale version conflicts.
	var reciting QueueEntry
	err = repo.WithTx(ctx, func(tx *Tx) error {
		var err error
		reciting, err = tx.TransitionEntry(ctx, entry.ID, EntryStatusWaiting, entry.Version, EntryStatusReciting, nil, nil, nil)
		return err
	})
	if err != nil || reciting.Version != entry.Version+1 {
		t.Fatalf("entry transition with fresh version: err=%v version=%d", err, reciting.Version)
	}
	staleEntryErr := repo.WithTx(ctx, func(tx *Tx) error {
		_, err := tx.TransitionEntry(ctx, entry.ID, EntryStatusReciting, entry.Version, EntryStatusCompleted, nil, nil, nil)
		return err
	})
	if got := queueErrCode(t, staleEntryErr); got != QueueErrorCodeStaleVersion {
		t.Fatalf("stale entry transition: got %s, want stale_version", got)
	}

	// Illegal transition (terminal → waiting) is rejected before any write.
	illegalErr := repo.WithTx(ctx, func(tx *Tx) error {
		_, err := tx.TransitionEntry(ctx, entry.ID, EntryStatusCompleted, reciting.Version, EntryStatusWaiting, nil, nil, nil)
		return err
	})
	if got := queueErrCode(t, illegalErr); got != QueueErrorCodeInvalidTransition {
		t.Fatalf("illegal transition: got %s, want invalid_transition", got)
	}
}

// Matrix 4: idempotency — the same (session, actor, key) with the same
// command replays the committed receipt/resource without a second mutation
// and without moving the round version; the same key with a different
// command is a duplicate-command conflict.
func TestQueueRepository_CommandReceiptIdempotency(t *testing.T) {
	repo := newQueueRepository(t)
	ctx := context.Background()
	teacher := qSeedUser(t, repo, "idem-teacher")
	circle := qSeedCircle(t, repo, teacher)
	session := qInsertSession(t, repo, circle, teacher, "managers_and_student")

	// First execution: reserve the key, create the round, record the result.
	var committed Round
	err := repo.WithTx(ctx, func(tx *Tx) error {
		if err := tx.LockRoundAllocation(ctx, session); err != nil {
			return err
		}
		replay, inserted, err := tx.ReserveCommandReceipt(ctx, session, teacher, "create-1", "queue.create_round")
		if err != nil {
			return err
		}
		if replay != nil || !inserted {
			return errors.New("first reserve must insert")
		}
		number, err := tx.NextRoundNumber(ctx, session)
		if err != nil {
			return err
		}
		committed, err = tx.CreateRound(ctx, NewRound{
			SessionID:       session,
			RoundNumber:     number,
			Type:            RoundTypeRevision,
			SurahID:         1,
			FromAyah:        1,
			ToAyah:          7,
			GradingRequired: true,
			Lifecycle:       RoundLifecyclePrepared,
			CreatedBy:       teacher,
		})
		if err != nil {
			return err
		}
		version := committed.Version
		return tx.UpdateCommandReceiptResult(ctx, session, teacher, "create-1", &committed.ID, &version)
	})
	if err != nil {
		t.Fatalf("first execution: %v", err)
	}

	// Replay: same key + same command returns the committed receipt, applies
	// no second mutation, and leaves the round version unchanged.
	var replay *CommandReceipt
	err = repo.WithTx(ctx, func(tx *Tx) error {
		var err error
		replay, _, err = tx.ReserveCommandReceipt(ctx, session, teacher, "create-1", "queue.create_round")
		return err
	})
	if err != nil {
		t.Fatalf("replay reserve: %v", err)
	}
	if replay == nil || replay.Command != "queue.create_round" {
		t.Fatalf("replay receipt: got %+v, want committed queue.create_round receipt", replay)
	}
	if replay.ResourceID == nil || *replay.ResourceID != committed.ID {
		t.Fatalf("replay resource: got %v, want committed round %s", replay.ResourceID, committed.ID)
	}
	if replay.ResultVersion == nil || *replay.ResultVersion != committed.Version {
		t.Fatalf("replay result version: got %v, want %d", replay.ResultVersion, committed.Version)
	}
	if got := qCountRounds(t, repo, session); got != 1 {
		t.Fatalf("rounds after replay: got %d, want 1", got)
	}
	snapshot, err := repo.LoadQueueState(ctx, committed.ID, Viewer{UserID: teacher, IsManager: true})
	if err != nil {
		t.Fatalf("load state after replay: %v", err)
	}
	if snapshot.Round.Version != committed.Version {
		t.Fatalf("round version after replay: got %d, want unchanged %d", snapshot.Round.Version, committed.Version)
	}

	// Same key with another command: duplicate-command conflict.
	dupErr := repo.WithTx(ctx, func(tx *Tx) error {
		_, _, err := tx.ReserveCommandReceipt(ctx, session, teacher, "create-1", "queue.advance")
		return err
	})
	if got := queueErrCode(t, dupErr); got != QueueErrorCodeDuplicateCommand {
		t.Fatalf("duplicate command: got %s, want duplicate_command", got)
	}
}

// Matrix 6: redaction — receipt rows and outbox event_metadata contain no
// grade, note, or student-name material in their persisted bytes, even though
// the completed entry itself carries a grade and note; metadata carrying a
// banned key is rejected at the insert boundary.
func TestQueueRepository_ReceiptAndOutboxRedaction(t *testing.T) {
	repo := newQueueRepository(t)
	ctx := context.Background()
	teacher := qSeedUser(t, repo, "redact-teacher")
	displayName := "أحمد القارئ المتميز"
	student := qSeedUserWithDisplayName(t, repo, "redact-student", displayName)
	circle := qSeedCircle(t, repo, teacher)
	session := qInsertSession(t, repo, circle, teacher, "managers_and_student")
	round := qCreateRound(t, repo, session, teacher, "active", []string{student})

	grade := GradeExcellent
	note := "مراجعة المدود مرة أخرى"
	entry, roundVersion := qCompleteEntry(t, repo, round, teacher, grade, note)

	err := repo.WithTx(ctx, func(tx *Tx) error {
		if _, _, err := tx.ReserveCommandReceipt(ctx, session, teacher, "complete-1", "queue.complete_entry"); err != nil {
			return err
		}
		resource := entry.ID
		version := roundVersion
		if err := tx.UpdateCommandReceiptResult(ctx, session, teacher, "complete-1", &resource, &version); err != nil {
			return err
		}
		return tx.InsertOutboxEvent(ctx, OutboxEvent{
			EventID:       "33333333-3333-4333-8333-333333333333",
			SessionID:     session,
			RoundID:       round.ID,
			EventType:     "queue.entry_completed",
			ResourceID:    &entry.ID,
			RoundVersion:  roundVersion,
			EventMetadata: []byte(`{"entry_id":"` + entry.ID + `","from":"reciting","to":"completed","position":1}`),
			AttemptCount:  0,
		})
	})
	if err != nil {
		t.Fatalf("write receipt and outbox: %v", err)
	}

	// The grade and note really are persisted on the entry — the assertions
	// below are not vacuous.
	var entryJSON string
	if err := repo.pool.QueryRow(ctx,
		`SELECT row_to_json(e)::text FROM recitation_queue_entries e WHERE e.id = $1::uuid`, entry.ID).Scan(&entryJSON); err != nil {
		t.Fatalf("read entry row: %v", err)
	}
	if !strings.Contains(entryJSON, string(grade)) || !strings.Contains(entryJSON, note) {
		t.Fatalf("entry row must carry the grade and note: %s", entryJSON)
	}

	var receiptJSON, metadataJSON string
	if err := repo.pool.QueryRow(ctx,
		`SELECT row_to_json(r)::text FROM queue_command_receipts r WHERE r.idempotency_key = 'complete-1'`).Scan(&receiptJSON); err != nil {
		t.Fatalf("read receipt row: %v", err)
	}
	if err := repo.pool.QueryRow(ctx,
		`SELECT event_metadata::text FROM queue_event_outbox WHERE event_id = $1::uuid`,
		"33333333-3333-4333-8333-333333333333").Scan(&metadataJSON); err != nil {
		t.Fatalf("read outbox metadata: %v", err)
	}
	for name, blob := range map[string]string{"receipt": receiptJSON, "outbox metadata": metadataJSON} {
		for _, secret := range []string{string(grade), note, displayName} {
			if strings.Contains(blob, secret) {
				t.Fatalf("%s leaked sensitive material %q: %s", name, secret, blob)
			}
		}
	}

	// A metadata payload carrying a banned key is rejected at the boundary.
	bannedErr := repo.WithTx(ctx, func(tx *Tx) error {
		return tx.InsertOutboxEvent(ctx, OutboxEvent{
			EventID:       "44444444-4444-4444-8444-444444444444",
			SessionID:     session,
			RoundID:       round.ID,
			EventType:     "queue.entry_completed",
			RoundVersion:  roundVersion,
			EventMetadata: []byte(`{"grade":"excellent"}`),
			AttemptCount:  0,
		})
	})
	if got := queueErrCode(t, bannedErr); got != QueueErrorCodeValidation {
		t.Fatalf("banned metadata key: got %s, want validation", got)
	}
	var bannedCount int
	if err := repo.pool.QueryRow(ctx, `SELECT COUNT(*) FROM queue_event_outbox`).Scan(&bannedCount); err != nil {
		t.Fatalf("count outbox after banned insert: %v", err)
	}
	if bannedCount != 1 {
		t.Fatalf("outbox rows after banned insert: got %d, want 1 (banned row rolled back)", bannedCount)
	}
}

// Matrix 7: QueueState visibility — a student viewer loses other students'
// grade/note under managers_and_student, loses every grade/note under
// managers_only, and sees all under all_participants; managers always see
// grades; preorder is a manager-only projection (CHK008); and the optimistic
// policy patch bumps the version only on a matching expected version.
func TestQueueRepository_QueueStateVisibilityMatrix(t *testing.T) {
	for _, tc := range []struct {
		policy           GradeVisibility
		studentSeesOwn   bool
		studentSeesOther bool
	}{
		{GradeVisibilityManagersAndStudent, true, false},
		{GradeVisibilityManagersOnly, false, false},
		{GradeVisibilityAllParticipants, true, true},
	} {
		t.Run(string(tc.policy), func(t *testing.T) {
			repo := newQueueRepository(t)
			ctx := context.Background()
			teacher := qSeedUser(t, repo, "vis-teacher")
			viewer := qSeedUser(t, repo, "vis-viewer")
			other := qSeedUser(t, repo, "vis-other")
			circle := qSeedCircle(t, repo, teacher)
			session := qInsertSession(t, repo, circle, teacher, string(tc.policy))
			round := qCreateRound(t, repo, session, teacher, "active", []string{viewer, other})
			qInsertPreorder(t, repo, round.ID, viewer, teacher, 1)

			qCompleteEntry(t, repo, round, teacher, GradeGood, "well done")
			qCompleteEntry(t, repo, round, teacher, GradeRepeat, "repeat this range")

			studentView, err := repo.LoadQueueState(ctx, round.ID, Viewer{UserID: viewer, IsManager: false})
			if err != nil {
				t.Fatalf("student view: %v", err)
			}
			if len(studentView.Entries) != 2 {
				t.Fatalf("student view entries: got %d, want 2", len(studentView.Entries))
			}
			for _, entry := range studentView.Entries {
				own := entry.StudentID == viewer
				wantGrade := own && tc.studentSeesOwn || !own && tc.studentSeesOther
				if (entry.Grade != nil) != wantGrade {
					t.Fatalf("student view entry %s grade: got %v, want present=%v", entry.StudentID, entry.Grade, wantGrade)
				}
				if (entry.TeacherNotes != nil) != wantGrade {
					t.Fatalf("student view entry %s notes: got %v, want present=%v", entry.StudentID, entry.TeacherNotes, wantGrade)
				}
			}
			if len(studentView.Preorder) != 0 {
				t.Fatalf("student view preorder: got %d, want 0 (CHK008)", len(studentView.Preorder))
			}

			managerView, err := repo.LoadQueueState(ctx, round.ID, Viewer{UserID: teacher, IsManager: true})
			if err != nil {
				t.Fatalf("manager view: %v", err)
			}
			for _, entry := range managerView.Entries {
				if entry.Grade == nil || entry.TeacherNotes == nil {
					t.Fatalf("manager view must see grade and notes of entry %s: %+v", entry.StudentID, entry)
				}
			}
			if len(managerView.Preorder) != 1 || managerView.Preorder[0].StudentID != viewer {
				t.Fatalf("manager view preorder: got %+v, want one candidate for the viewer", managerView.Preorder)
			}

			// Optimistic policy patch: fresh expected version bumps, stale
			// expected version conflicts.
			policyCtx, err := repo.SessionPolicy(ctx, session)
			if err != nil {
				t.Fatalf("read policy: %v", err)
			}
			next := policyCtx.Policy
			next.GradeVisibility = GradeVisibilityManagersOnly
			if next.GradeVisibility == policyCtx.Policy.GradeVisibility {
				next.GradeVisibility = GradeVisibilityAllParticipants
			}
			updated, err := repo.UpdateSessionPolicy(ctx, session, policyCtx.Policy.Version, next)
			if err != nil || updated.Version != policyCtx.Policy.Version+1 {
				t.Fatalf("policy update: err=%v version=%d want %d", err, updated.Version, policyCtx.Policy.Version+1)
			}
			if updated.GradeVisibility != next.GradeVisibility {
				t.Fatalf("policy update visibility: got %s", updated.GradeVisibility)
			}
			staleErr := func() error {
				_, err := repo.UpdateSessionPolicy(ctx, session, policyCtx.Policy.Version, next)
				return err
			}()
			if got := queueErrCode(t, staleErr); got != QueueErrorCodeStaleVersion {
				t.Fatalf("stale policy update: got %s, want stale_version", got)
			}
		})
	}
}

// Outbox lifecycle: claim returns only due rows, delivery completes a row,
// retry counts the attempt and reschedules, park excludes a row from claims,
// and the operational counts reflect pending vs parked backlog.
func TestQueueRepository_OutboxClaimRetryParkLifecycle(t *testing.T) {
	repo := newQueueRepository(t)
	ctx := context.Background()
	teacher := qSeedUser(t, repo, "outbox-teacher")
	circle := qSeedCircle(t, repo, teacher)
	session := qInsertSession(t, repo, circle, teacher, "managers_and_student")
	round := qCreateRound(t, repo, session, teacher, "prepared", nil)

	const dueA, dueB, futureC = "55555555-5555-4555-8555-555555555555", "66666666-6666-4666-8666-666666666666", "77777777-7777-4777-8777-777777777777"
	for _, tc := range []struct {
		eventID     string
		availableAt time.Time
	}{
		{dueA, time.Now().UTC().Add(-time.Minute)},
		{dueB, time.Now().UTC()},
		{futureC, time.Now().UTC().Add(time.Hour)},
	} {
		err := repo.WithTx(ctx, func(tx *Tx) error {
			return tx.InsertOutboxEvent(ctx, OutboxEvent{
				EventID:       tc.eventID,
				SessionID:     session,
				RoundID:       round.ID,
				EventType:     "queue.round_started",
				RoundVersion:  1,
				EventMetadata: []byte(`{"lifecycle":"prepared"}`),
				AvailableAt:   tc.availableAt,
				AttemptCount:  0,
			})
		})
		if err != nil {
			t.Fatalf("insert outbox event %s: %v", tc.eventID, err)
		}
	}

	claimed, err := repo.ClaimDueOutboxEvents(ctx, 10)
	if err != nil {
		t.Fatalf("claim: %v", err)
	}
	claimedIDs := map[string]bool{}
	for _, event := range claimed {
		claimedIDs[event.EventID] = true
	}
	if len(claimed) != 2 || !claimedIDs[dueA] || !claimedIDs[dueB] || claimedIDs[futureC] {
		t.Fatalf("claim: got %v, want exactly the two due events", claimedIDs)
	}

	if err := repo.MarkOutboxDelivered(ctx, dueA); err != nil {
		t.Fatalf("mark delivered: %v", err)
	}
	if err := repo.RetryOutboxEvent(ctx, dueB, time.Now().UTC().Add(time.Hour)); err != nil {
		t.Fatalf("retry: %v", err)
	}
	var attempts int
	if err := repo.pool.QueryRow(ctx,
		`SELECT attempt_count FROM queue_event_outbox WHERE event_id = $1::uuid`, dueB).Scan(&attempts); err != nil {
		t.Fatalf("read attempts: %v", err)
	}
	if attempts != 1 {
		t.Fatalf("attempts after retry: got %d, want 1", attempts)
	}
	if err := repo.ParkOutboxEvent(ctx, dueB); err != nil {
		t.Fatalf("park: %v", err)
	}

	claimed, err = repo.ClaimDueOutboxEvents(ctx, 10)
	if err != nil {
		t.Fatalf("claim after deliver/park: %v", err)
	}
	if len(claimed) != 0 {
		t.Fatalf("claim after deliver/park: got %d events, want 0", len(claimed))
	}

	pending, parked, err := repo.OutboxCounts(ctx, session)
	if err != nil {
		t.Fatalf("counts: %v", err)
	}
	if pending != 1 || parked != 1 {
		t.Fatalf("counts: pending=%d parked=%d, want 1/1", pending, parked)
	}
}

// Population ordering under both policies: present_at_activation keeps
// present preorder students in relative order first, then present student
// members without preorder by first_joined_at (UUID tie-break); an absent
// preorder student and a present non-member are excluded.
// all_active_students keeps preorder first, then every remaining student
// member by joined_at (UUID tie-break), with no presence gate.
func TestQueueRepository_PopulationOrderPolicies(t *testing.T) {
	repo := newQueueRepository(t)
	ctx := context.Background()
	teacher := qSeedUser(t, repo, "pop-teacher")
	s1 := qSeedUser(t, repo, "pop-s1")
	s2 := qSeedUser(t, repo, "pop-s2")
	s3 := qSeedUser(t, repo, "pop-s3")
	s4 := qSeedUser(t, repo, "pop-s4")
	s6 := qSeedUser(t, repo, "pop-s6")
	outsider := qSeedUser(t, repo, "pop-outsider")
	circle := qSeedCircle(t, repo, teacher)
	base := time.Now().UTC().Add(-time.Hour)
	for _, m := range []struct {
		user     string
		role     string
		joinedAt time.Time
	}{
		{teacher, "teacher", base},
		{s1, "student", base.Add(3 * time.Minute)},
		{s2, "student", base.Add(time.Minute)},
		{s3, "student", base.Add(2 * time.Minute)},
		{s4, "student", base.Add(90 * time.Second)},
	} {
		qSeedMember(t, repo, circle, m.user, m.role, m.joinedAt)
	}
	session := qInsertSession(t, repo, circle, teacher, "managers_and_student")

	t0 := base.Add(10 * time.Minute)
	qInsertPresence(t, repo, session, s1, t0, true)
	qInsertPresence(t, repo, session, s2, t0.Add(time.Minute), true)
	qInsertPresence(t, repo, session, s3, t0.Add(2*time.Minute), true)
	qInsertPresence(t, repo, session, outsider, t0.Add(3*time.Minute), true)

	assertOrder := func(policy PopulationPolicy, queueID string, want []string) {
		t.Helper()
		var got []string
		err := repo.WithTx(ctx, func(tx *Tx) error {
			var err error
			got, err = tx.PopulationOrder(ctx, session, queueID, policy)
			return err
		})
		if err != nil {
			t.Fatalf("population order (%s): %v", policy, err)
		}
		if len(got) != len(want) {
			t.Fatalf("population order (%s): got %v, want %v", policy, got, want)
		}
		for i := range want {
			if got[i] != want[i] {
				t.Fatalf("population order (%s): got %v, want %v", policy, got, want)
			}
		}
	}

	presentRound := qCreateRound(t, repo, session, teacher, "prepared", nil)
	qInsertPreorder(t, repo, presentRound.ID, s3, teacher, 1)
	qInsertPreorder(t, repo, presentRound.ID, s1, teacher, 2)
	qInsertPreorder(t, repo, presentRound.ID, s6, teacher, 3) // absent: excluded
	assertOrder(PopulationPolicyPresentAtActivation, presentRound.ID, []string{s3, s1, s2})

	allRound := qCreateRound(t, repo, session, teacher, "prepared", nil)
	qInsertPreorder(t, repo, allRound.ID, s3, teacher, 1)
	assertOrder(PopulationPolicyAllActiveStudents, allRound.ID, []string{s3, s2, s4, s1})
}

// Finalization: mark_unfinished_skipped converts waiting/reciting entries to
// skipped with attribution while preserve_last_state keeps their last state;
// both finalize the round, clear the selection, and bump the version, and a
// replayed finalize with the old version conflicts.
func TestQueueRepository_FinalizeRoundPolicies(t *testing.T) {
	for _, tc := range []struct {
		policy          FinalizationPolicy
		wantEntryStatus EntryStatus
	}{
		{FinalizationPolicyMarkUnfinishedSkipped, EntryStatusSkipped},
		{FinalizationPolicyPreserveLastState, EntryStatusWaiting},
	} {
		t.Run(string(tc.policy), func(t *testing.T) {
			repo := newQueueRepository(t)
			ctx := context.Background()
			teacher := qSeedUser(t, repo, "fin-teacher")
			student := qSeedUser(t, repo, "fin-student")
			circle := qSeedCircle(t, repo, teacher)
			session := qInsertSession(t, repo, circle, teacher, "managers_and_student")
			round := qCreateRound(t, repo, session, teacher, "active", []string{student})

			var finalized Round
			err := repo.WithTx(ctx, func(tx *Tx) error {
				var err error
				finalized, err = tx.FinalizeRound(ctx, round.ID, round.Version, tc.policy, teacher)
				return err
			})
			if err != nil {
				t.Fatalf("finalize (%s): %v", tc.policy, err)
			}
			if finalized.Lifecycle != RoundLifecycleFinalized || finalized.SelectedEntryID != nil || finalized.FinalizedAt == nil {
				t.Fatalf("finalized round state: %+v", finalized)
			}
			if finalized.Version != round.Version+1 {
				t.Fatalf("finalized version: got %d, want %d", finalized.Version, round.Version+1)
			}

			snapshot, err := repo.LoadQueueState(ctx, round.ID, Viewer{UserID: teacher, IsManager: true})
			if err != nil {
				t.Fatalf("load finalized state: %v", err)
			}
			if len(snapshot.Entries) != 1 || snapshot.Entries[0].Status != tc.wantEntryStatus {
				t.Fatalf("entries after finalize (%s): %+v, want one %s", tc.policy, snapshot.Entries, tc.wantEntryStatus)
			}
			if tc.policy == FinalizationPolicyMarkUnfinishedSkipped {
				if snapshot.Entries[0].ResolvedBy == nil || *snapshot.Entries[0].ResolvedBy != teacher {
					t.Fatalf("skipped entry attribution: %+v", snapshot.Entries[0])
				}
			}

			staleErr := repo.WithTx(ctx, func(tx *Tx) error {
				_, err := tx.FinalizeRound(ctx, round.ID, round.Version, tc.policy, teacher)
				return err
			})
			if got := queueErrCode(t, staleErr); got != QueueErrorCodeStaleVersion {
				t.Fatalf("replayed finalize: got %s, want stale_version", got)
			}
		})
	}
}

// Position primitives: moving a waiting entry to a vacant slot rewrites the
// position and bumps the entry version; rewriting a preorder slot affects
// only that candidate.
func TestQueueRepository_PositionUpdates(t *testing.T) {
	repo := newQueueRepository(t)
	ctx := context.Background()
	teacher := qSeedUser(t, repo, "pos-teacher")
	student := qSeedUser(t, repo, "pos-student")
	circle := qSeedCircle(t, repo, teacher)
	session := qInsertSession(t, repo, circle, teacher, "managers_and_student")
	round := qCreateRound(t, repo, session, teacher, "active", nil)
	var entry QueueEntry
	err := repo.WithTx(ctx, func(tx *Tx) error {
		var err error
		entry, err = tx.InsertQueueEntry(ctx, round.ID, student, 2)
		return err
	})
	if err != nil {
		t.Fatalf("insert entry: %v", err)
	}

	err = repo.WithTx(ctx, func(tx *Tx) error {
		return tx.SetEntryPosition(ctx, entry.ID, 3)
	})
	if err != nil {
		t.Fatalf("move entry: %v", err)
	}
	snapshot, err := repo.LoadQueueState(ctx, round.ID, Viewer{UserID: teacher, IsManager: true})
	if err != nil {
		t.Fatalf("load state: %v", err)
	}
	var moved QueueEntry
	for _, e := range snapshot.Entries {
		if e.ID == entry.ID {
			moved = e
		}
	}
	if moved.Position != 3 || moved.Version != entry.Version+1 {
		t.Fatalf("moved entry: position=%d version=%d, want 3/%d", moved.Position, moved.Version, entry.Version+1)
	}

	preorder := qCreateRound(t, repo, session, teacher, "prepared", nil)
	qInsertPreorder(t, repo, preorder.ID, student, teacher, 1)
	err = repo.WithTx(ctx, func(tx *Tx) error {
		return tx.SetPreorderPosition(ctx, preorder.ID, student, 4)
	})
	if err != nil {
		t.Fatalf("reorder preorder: %v", err)
	}
	var position int
	if err := repo.pool.QueryRow(ctx, `
		SELECT position FROM recitation_queue_preorder WHERE queue_id = $1::uuid AND student_id = $2::uuid
	`, preorder.ID, student).Scan(&position); err != nil {
		t.Fatalf("read preorder position: %v", err)
	}
	if position != 4 {
		t.Fatalf("preorder position: got %d, want 4", position)
	}
}

func TestQueueRepositoryReadsCurrentRoleAndLocksEntry(t *testing.T) {
	repo := newQueueRepository(t)
	ctx := context.Background()
	teacher := qSeedUser(t, repo, "role-teacher")
	student := qSeedUser(t, repo, "role-student")
	circle := qSeedCircle(t, repo, teacher)
	qSeedMember(t, repo, circle, teacher, "teacher", time.Now().UTC())
	qSeedMember(t, repo, circle, student, "student", time.Now().UTC())
	session := qInsertSession(t, repo, circle, teacher, "managers_and_student")
	round := qCreateRound(t, repo, session, teacher, "active", []string{student})

	role, err := repo.SessionRole(ctx, session, student)
	if err != nil || role != "student" {
		t.Fatalf("session role: role=%q err=%v, want student/nil", role, err)
	}
	var entry QueueEntry
	err = repo.WithTx(ctx, func(tx *Tx) error {
		var err error
		entry, err = tx.LockEntry(ctx, "entry-does-not-exist")
		return err
	})
	if err == nil || entry.ID != "" {
		t.Fatalf("missing locked entry: entry=%+v err=%v, want error", entry, err)
	}

	state, err := repo.LoadQueueState(ctx, round.ID, Viewer{UserID: teacher, IsManager: true})
	if err != nil || len(state.Entries) != 1 {
		t.Fatalf("round state: entries=%d err=%v", len(state.Entries), err)
	}
	var locked QueueEntry
	err = repo.WithTx(ctx, func(tx *Tx) error {
		var err error
		locked, err = tx.LockEntry(ctx, state.Entries[0].ID)
		return err
	})
	if err != nil || locked.ID != state.Entries[0].ID {
		t.Fatalf("locked entry: %+v err=%v", locked, err)
	}
}
