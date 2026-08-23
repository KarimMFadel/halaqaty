//go:build integration

package integration

import (
	"context"
	"fmt"
	"os"
	"reflect"
	"testing"
	"time"

	"github.com/KarimMFadel/halaqaty/backend/internal/queue"
	"github.com/KarimMFadel/halaqaty/backend/internal/sessions"
	"github.com/jackc/pgx/v5/pgxpool"
)

type roundFlowFixture struct {
	pool       *pgxpool.Pool
	repo       *queue.Repository
	teacherID  string
	supervisor string
	students   []string
	sessionID  string
}

type roundFlowAudioCall struct {
	kind         string
	sessionID    string
	roundID      string
	queueEntryID string
	userID       string
}

type roundFlowAudioControl struct {
	calls []roundFlowAudioCall
}

func (c *roundFlowAudioControl) GrantReciterAudio(_ context.Context, sessionID, roundID, queueEntryID, userID string) error {
	c.calls = append(c.calls, roundFlowAudioCall{
		kind: "grant", sessionID: sessionID, roundID: roundID, queueEntryID: queueEntryID, userID: userID,
	})
	return nil
}

func (c *roundFlowAudioControl) RevokeReciterAudio(_ context.Context, sessionID, roundID, queueEntryID, userID string) error {
	c.calls = append(c.calls, roundFlowAudioCall{
		kind: "revoke", sessionID: sessionID, roundID: roundID, queueEntryID: queueEntryID, userID: userID,
	})
	return nil
}

// TestRecitationQueueRoundFlow covers T022's PostgreSQL acceptance smoke flow
// for both queue population policies. It deliberately uses the service layer
// because REST handler wiring is a separate T024/T029 concern.
func TestRecitationQueueRoundFlow(t *testing.T) {
	for _, policy := range []queue.PopulationPolicy{
		queue.PopulationPolicyPresentAtActivation,
		queue.PopulationPolicyAllActiveStudents,
	} {
		t.Run(string(policy), func(t *testing.T) {
			runRecitationQueueRoundFlow(t, policy)
		})
	}
}

func runRecitationQueueRoundFlow(t *testing.T, population queue.PopulationPolicy) {
	t.Helper()
	ctx := context.Background()
	fixture := newRoundFlowFixture(t, population)
	service := queue.NewRoundService(fixture.repo, nil)

	assertRoundFlowAuthorizationContext(t, fixture)

	first, err := service.Prepare(ctx, queue.PrepareRoundInput{
		SessionID: fixture.sessionID, Type: queue.RoundTypeRevision, SurahID: 1,
		FromAyah: 1, ToAyah: 7, SurahAyahCount: 7, GradingRequired: true,
		CreatedBy: fixture.teacherID, Preorder: []string{fixture.students[2], fixture.students[3], fixture.students[1]},
	})
	if err != nil {
		t.Fatalf("prepare first round: %v", err)
	}
	second, err := service.Prepare(ctx, queue.PrepareRoundInput{
		SessionID: fixture.sessionID, Type: queue.RoundTypeTest, SurahID: 2,
		FromAyah: 1, ToAyah: 3, SurahAyahCount: 286, GradingRequired: false,
		CreatedBy: fixture.teacherID, Preorder: []string{fixture.students[0], fixture.students[2], fixture.students[3]},
	})
	if err != nil {
		t.Fatalf("prepare second round: %v", err)
	}

	assertRoundLifecycle(t, fixture.repo, first.ID, queue.RoundLifecyclePrepared, 1)
	assertRoundLifecycle(t, fixture.repo, second.ID, queue.RoundLifecyclePrepared, 2)
	assertQueueState(t, fixture.repo, first.ID, nil, []string{fixture.students[2], fixture.students[3], fixture.students[1]})
	assertQueueState(t, fixture.repo, second.ID, nil, []string{fixture.students[0], fixture.students[2], fixture.students[3]})

	// Reordering is manager-only and durable while the round is still prepared.
	reordered, err := service.Reorder(ctx, second.ID, fixture.teacherID, second.Version,
		[]string{fixture.students[3], fixture.students[0], fixture.students[2]})
	if err != nil {
		t.Fatalf("reorder prepared round: %v", err)
	}
	if reordered.Version != second.Version+1 {
		t.Fatalf("reordered round version = %d, want %d", reordered.Version, second.Version+1)
	}
	assertQueueState(t, fixture.repo, second.ID, nil,
		[]string{fixture.students[3], fixture.students[0], fixture.students[2]})

	// This update models the committed F-005 session-start fact. Queue
	// activation is then restored through the provider-neutral queue hook.
	setRoundFlowSessionActive(t, fixture)
	if err := service.ActivateIfNeeded(ctx, fixture.sessionID); err != nil {
		t.Fatalf("activate lowest prepared round at session start: %v", err)
	}

	firstState := roundFlowState(t, fixture.repo, first.ID)
	if firstState.Round.Lifecycle != queue.RoundLifecycleActive || firstState.Round.ActivatedAt == nil {
		t.Fatalf("first round after session start = %+v, want active with activation time", firstState.Round)
	}
	if secondState := roundFlowState(t, fixture.repo, second.ID); secondState.Round.Lifecycle != queue.RoundLifecyclePrepared {
		t.Fatalf("second round after session start = %s, want prepared", secondState.Round.Lifecycle)
	}

	wantFirstPopulation := []string{fixture.students[2], fixture.students[3], fixture.students[0]}
	if population == queue.PopulationPolicyAllActiveStudents {
		wantFirstPopulation = []string{fixture.students[2], fixture.students[3], fixture.students[1], fixture.students[0]}
	}
	assertQueueState(t, fixture.repo, first.ID, wantFirstPopulation, []string{fixture.students[2], fixture.students[3], fixture.students[1]})
	firstState = roundFlowState(t, fixture.repo, first.ID)
	assertNoDuplicateQueuePositionsOrStudents(t, firstState)

	entriesByStudent := make(map[string]queue.QueueEntry, len(firstState.Entries))
	for _, entry := range firstState.Entries {
		entriesByStudent[entry.StudentID] = entry
	}
	firstEntry := entriesByStudent[fixture.students[2]]
	movedEntry := entriesByStudent[fixture.students[0]]
	if firstEntry.ID == "" || movedEntry.ID == "" {
		t.Fatalf("activated entries missing first=%+v moved=%+v", firstEntry, movedEntry)
	}

	audio := &roundFlowAudioControl{}
	turns := queue.NewTurnService(fixture.repo, audio)
	selected, err := turns.Advance(ctx, first.ID, firstState.Round.Version)
	if err != nil {
		t.Fatalf("advance first turn: %v", err)
	}
	if selected.SelectedEntryID == nil || *selected.SelectedEntryID != firstEntry.ID {
		t.Fatalf("advance selected entry = %v, want %s", selected.SelectedEntryID, firstEntry.ID)
	}
	selectedState := roundFlowState(t, fixture.repo, first.ID)
	if selectedState.Round.SelectedEntryID == nil || *selectedState.Round.SelectedEntryID != firstEntry.ID {
		t.Fatalf("durable selection = %v, want %s", selectedState.Round.SelectedEntryID, firstEntry.ID)
	}
	assertStatusCount(t, selectedState, queue.EntryStatusWaiting, len(firstState.Entries))
	if len(audio.calls) != 0 {
		t.Fatalf("audio calls after advance = %+v, want none", audio.calls)
	}

	started, err := turns.Start(ctx, firstEntry.ID, selected.Version)
	if err != nil {
		t.Fatalf("start selected turn: %v", err)
	}
	if started.Status != queue.EntryStatusReciting {
		t.Fatalf("started status = %s, want reciting", started.Status)
	}
	startedState := roundFlowState(t, fixture.repo, first.ID)
	assertStatusCount(t, startedState, queue.EntryStatusReciting, 1)
	if len(audio.calls) != 1 || audio.calls[0].kind != "grant" || audio.calls[0].queueEntryID != firstEntry.ID || audio.calls[0].userID != firstEntry.StudentID {
		t.Fatalf("audio calls after start = %+v, want one grant for %s", audio.calls, firstEntry.ID)
	}

	moved, err := service.Move(ctx, movedEntry.ID, startedState.Round.Version, 2)
	if err != nil {
		t.Fatalf("move waiting entry while another recites: %v", err)
	}
	if moved.Position != 2 {
		t.Fatalf("moved entry position = %d, want 2", moved.Position)
	}
	movedState := roundFlowState(t, fixture.repo, first.ID)
	assertNoDuplicateQueuePositionsOrStudents(t, movedState)
	if movedState.Entries[1].ID != movedEntry.ID {
		t.Fatalf("durable moved order = %v, want %s at position 2", queueFlowStudentOrder(movedState), movedEntry.ID)
	}
	if got := roundFlowState(t, fixture.repo, first.ID).Entries[0].Status; got != queue.EntryStatusReciting {
		t.Fatalf("reciting entry after move = %s, want reciting", got)
	}

	skipped, err := turns.Skip(ctx, firstEntry.ID, movedState.Round.Version, fixture.teacherID)
	if err != nil {
		t.Fatalf("skip first reciting turn: %v", err)
	}
	if skipped.Status != queue.EntryStatusSkipped {
		t.Fatalf("skipped first status = %s, want skipped", skipped.Status)
	}
	if len(audio.calls) != 2 || audio.calls[1].kind != "revoke" || audio.calls[1].queueEntryID != firstEntry.ID {
		t.Fatalf("audio calls after first skip = %+v, want revoke for %s", audio.calls, firstEntry.ID)
	}
	afterFirstSkip := roundFlowState(t, fixture.repo, first.ID)
	if afterFirstSkip.Round.SelectedEntryID != nil {
		t.Fatalf("durable selection after terminal skip = %v, want cleared", afterFirstSkip.Round.SelectedEntryID)
	}

	nextSelected, err := turns.Advance(ctx, first.ID, afterFirstSkip.Round.Version)
	if err != nil {
		t.Fatalf("advance after first skip: %v", err)
	}
	if nextSelected.SelectedEntryID == nil || *nextSelected.SelectedEntryID != movedEntry.ID {
		t.Fatalf("next selected entry = %v, want moved entry %s", nextSelected.SelectedEntryID, movedEntry.ID)
	}
	if _, err := turns.Start(ctx, movedEntry.ID, nextSelected.Version); err != nil {
		t.Fatalf("start moved entry: %v", err)
	}
	startedMoved := roundFlowState(t, fixture.repo, first.ID)
	if got := len(queueFlowRecitingEntries(startedMoved)); got != 1 || queueFlowRecitingEntries(startedMoved)[0].ID != movedEntry.ID {
		t.Fatalf("reciting entries after second start = %+v, want only %s", queueFlowRecitingEntries(startedMoved), movedEntry.ID)
	}
	if _, err := turns.Skip(ctx, movedEntry.ID, startedMoved.Round.Version, fixture.teacherID); err != nil {
		t.Fatalf("skip moved turn: %v", err)
	}
	if len(audio.calls) != 4 || audio.calls[2].kind != "grant" || audio.calls[2].queueEntryID != movedEntry.ID || audio.calls[3].kind != "revoke" || audio.calls[3].queueEntryID != movedEntry.ID {
		t.Fatalf("audio calls after second skip = %+v, want grant/revoke for %s", audio.calls, movedEntry.ID)
	}

	beforeReset := roundFlowState(t, fixture.repo, first.ID)
	historyIDs := queueFlowEntryIDs(beforeReset)
	resetRound, err := service.Reset(ctx, queue.PrepareRoundInput{
		SessionID: fixture.sessionID, Type: queue.RoundTypeOldRevision, SurahID: 2,
		FromAyah: 4, ToAyah: 8, SurahAyahCount: 286, GradingRequired: false,
		CreatedBy: fixture.teacherID,
	})
	if err != nil {
		t.Fatalf("reset active round: %v", err)
	}
	if resetRound.RoundNumber != 3 || resetRound.Lifecycle != queue.RoundLifecyclePrepared {
		t.Fatalf("reset-created round = %+v, want round 3 prepared", resetRound)
	}

	finalHistory := roundFlowState(t, fixture.repo, first.ID)
	if finalHistory.Round.Lifecycle != queue.RoundLifecycleFinalized || finalHistory.Round.SelectedEntryID != nil || finalHistory.Round.FinalizedAt == nil {
		t.Fatalf("finalized history round = %+v, want retained finalized non-actionable round", finalHistory.Round)
	}
	if !reflect.DeepEqual(historyIDs, queueFlowEntryIDs(finalHistory)) {
		t.Fatalf("history entry IDs changed on reset: before=%v after=%v", historyIDs, queueFlowEntryIDs(finalHistory))
	}
	for _, entry := range finalHistory.Entries {
		if entry.Status == queue.EntryStatusWaiting || entry.Status == queue.EntryStatusReciting {
			t.Fatalf("finalized history entry %s remains actionable with status %s", entry.ID, entry.Status)
		}
		if entry.ResolvedBy == nil || *entry.ResolvedBy != fixture.teacherID {
			t.Fatalf("finalized history entry %s resolved_by = %v, want authorized teacher %s", entry.ID, entry.ResolvedBy, fixture.teacherID)
		}
	}

	secondState := roundFlowState(t, fixture.repo, second.ID)
	if secondState.Round.Lifecycle != queue.RoundLifecycleActive || secondState.Round.ActivatedAt == nil {
		t.Fatalf("stacked second round after reset = %+v, want active", secondState.Round)
	}
	wantSecondPopulation := []string{fixture.students[3], fixture.students[0], fixture.students[2]}
	if population == queue.PopulationPolicyAllActiveStudents {
		wantSecondPopulation = []string{fixture.students[3], fixture.students[0], fixture.students[2], fixture.students[1]}
	}
	assertQueueState(t, fixture.repo, second.ID, wantSecondPopulation,
		[]string{fixture.students[3], fixture.students[0], fixture.students[2]})
	assertNoDuplicateQueuePositionsOrStudents(t, secondState)

	// A later round receives new entry identities; the finalized round remains
	// readable and unchanged after the activation chain advances.
	if overlap := intersectStrings(queueFlowEntryIDs(finalHistory), queueFlowEntryIDs(secondState)); len(overlap) != 0 {
		t.Fatalf("reset reused historical entry IDs: %v", overlap)
	}
	if reread := roundFlowState(t, fixture.repo, first.ID); !reflect.DeepEqual(finalHistory, reread) {
		t.Fatalf("finalized history changed after next-round activation: before=%+v after=%+v", finalHistory, reread)
	}
}

func newRoundFlowFixture(t *testing.T, population queue.PopulationPolicy) roundFlowFixture {
	t.Helper()
	ctx := context.Background()
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		t.Skip("DATABASE_URL is not set; T022 requires PostgreSQL via DATABASE_URL")
	}

	schema := uniqueSchemaName(t) + "_round_flow"
	config, err := pgxpool.ParseConfig(dbURL)
	if err != nil {
		t.Fatalf("parse DATABASE_URL: %v", err)
	}
	if config.ConnConfig.RuntimeParams == nil {
		config.ConnConfig.RuntimeParams = map[string]string{}
	}
	config.ConnConfig.RuntimeParams["search_path"] = schema
	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		t.Fatalf("open PostgreSQL pool for DATABASE_URL: %v", err)
	}
	t.Cleanup(func() {
		cleanupCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_, _ = pool.Exec(cleanupCtx, fmt.Sprintf("DROP SCHEMA IF EXISTS %s CASCADE", schema))
		pool.Close()
	})
	if _, err := pool.Exec(ctx, fmt.Sprintf("CREATE SCHEMA %s", schema)); err != nil {
		t.Fatalf("create isolated PostgreSQL schema: %v", err)
	}
	conn := acquireConn(t, pool, ctx)
	for _, migration := range []string{
		"000010_auth_roles_profile.up.sql",
		"000011_auth_roles_profile_alignment.up.sql",
		"000012_auth_profiles_display_name.up.sql",
		"000013_create_circles.up.sql",
		"000014_circle_members_circle_fk.up.sql",
		"000015_circle_management.up.sql",
		"000016_live_sessions.up.sql",
		"000017_recitation_queue_system.up.sql",
	} {
		runMigrationFile(t, conn, ctx, migration)
	}
	conn.Release()

	teacherID := seedRoundFlowUser(t, pool, "teacher")
	supervisorID := seedRoundFlowUser(t, pool, "supervisor")
	students := []string{
		seedRoundFlowUser(t, pool, "student-a"),
		seedRoundFlowUser(t, pool, "student-b-absent"),
		seedRoundFlowUser(t, pool, "student-c"),
		seedRoundFlowUser(t, pool, "student-d"),
	}
	circleID := seedRoundFlowCircle(t, pool, teacherID)
	base := time.Now().UTC().Add(-time.Hour)
	seedRoundFlowMember(t, pool, circleID, teacherID, "teacher", base)
	seedRoundFlowMember(t, pool, circleID, supervisorID, "supervisor", base.Add(time.Minute))
	for i, studentID := range students {
		seedRoundFlowMember(t, pool, circleID, studentID, "student", base.Add(time.Duration(i+2)*time.Minute))
	}

	var sessionID string
	if err := pool.QueryRow(ctx, `
		INSERT INTO sessions (circle_id, created_by, queue_population_policy)
		VALUES ($1::uuid, $2::uuid, $3)
		RETURNING id::text
	`, circleID, teacherID, population).Scan(&sessionID); err != nil {
		t.Fatalf("insert scheduled session: %v", err)
	}
	seedRoundFlowPresence(t, pool, sessionID, students[0], base.Add(20*time.Minute), true)
	seedRoundFlowPresence(t, pool, sessionID, students[1], base.Add(21*time.Minute), false)
	seedRoundFlowPresence(t, pool, sessionID, students[2], base.Add(22*time.Minute), true)
	seedRoundFlowPresence(t, pool, sessionID, students[3], base.Add(23*time.Minute), true)
	seedRoundFlowPresence(t, pool, sessionID, teacherID, base.Add(24*time.Minute), true)
	seedRoundFlowPresence(t, pool, sessionID, supervisorID, base.Add(25*time.Minute), true)

	return roundFlowFixture{
		pool: pool, repo: queue.NewQueueRepository(pool), teacherID: teacherID,
		supervisor: supervisorID, students: students, sessionID: sessionID,
	}
}

func seedRoundFlowUser(t *testing.T, pool *pgxpool.Pool, label string) string {
	t.Helper()
	var id string
	if err := pool.QueryRow(context.Background(), `
		INSERT INTO users (firebase_uid, email)
		VALUES ($1, $2)
		RETURNING id::text
	`, "firebase-round-flow-"+label, label+"@example.com").Scan(&id); err != nil {
		t.Fatalf("seed round-flow user %s: %v", label, err)
	}
	return id
}

func seedRoundFlowCircle(t *testing.T, pool *pgxpool.Pool, teacherID string) string {
	t.Helper()
	var id string
	if err := pool.QueryRow(context.Background(), `
		INSERT INTO circles (name, teacher_id, invite_code)
		VALUES ('T022 Round Flow Circle', $1::uuid, 'HLQ-T022FLOW')
		RETURNING id::text
	`, teacherID).Scan(&id); err != nil {
		t.Fatalf("seed round-flow circle: %v", err)
	}
	return id
}

func seedRoundFlowMember(t *testing.T, pool *pgxpool.Pool, circleID, userID, role string, joinedAt time.Time) {
	t.Helper()
	if _, err := pool.Exec(context.Background(), `
		INSERT INTO circle_members (circle_id, user_id, role, joined_at)
		VALUES ($1::uuid, $2::uuid, $3, $4)
	`, circleID, userID, role, joinedAt); err != nil {
		t.Fatalf("seed round-flow member %s (%s): %v", userID, role, err)
	}
}

func seedRoundFlowPresence(t *testing.T, pool *pgxpool.Pool, sessionID, userID string, joinedAt time.Time, present bool) {
	t.Helper()
	if _, err := pool.Exec(context.Background(), `
		INSERT INTO session_participant_presence (session_id, user_id, first_joined_at, last_joined_at, is_currently_present)
		VALUES ($1::uuid, $2::uuid, $3, $3, $4)
	`, sessionID, userID, joinedAt, present); err != nil {
		t.Fatalf("seed round-flow presence %s: %v", userID, err)
	}
}

func setRoundFlowSessionActive(t *testing.T, fixture roundFlowFixture) {
	t.Helper()
	started, err := sessions.NewSessionRepository(fixture.pool).StartSession(
		context.Background(), fixture.sessionID, sessions.MediaRoomRef("t022-integration-room"),
	)
	if err != nil {
		t.Fatalf("commit session-start state: %v", err)
	}
	if started.Status != sessions.SessionStatusActive {
		t.Fatalf("session-start status = %q, want active", started.Status)
	}
}

func assertRoundFlowAuthorizationContext(t *testing.T, fixture roundFlowFixture) {
	t.Helper()
	ctx := context.Background()
	role, err := fixture.repo.SessionRole(ctx, fixture.sessionID, fixture.teacherID)
	if err != nil {
		t.Fatalf("load teacher session role: %v", err)
	}
	if role != "teacher" {
		t.Fatalf("queue manager role = %q, want teacher", role)
	}
	supervisorRole, err := fixture.repo.SessionRole(ctx, fixture.sessionID, fixture.supervisor)
	if err != nil {
		t.Fatalf("load supervisor session role: %v", err)
	}
	if supervisorRole != "supervisor" {
		t.Fatalf("supervisor role = %q, want supervisor", supervisorRole)
	}
	studentRole, err := fixture.repo.SessionRole(ctx, fixture.sessionID, fixture.students[0])
	if err != nil {
		t.Fatalf("load student session role: %v", err)
	}
	if studentRole != "student" {
		t.Fatalf("student role = %q, want student", studentRole)
	}
}

func roundFlowState(t *testing.T, repo *queue.Repository, roundID string) queue.QueueState {
	t.Helper()
	state, err := repo.LoadQueueState(context.Background(), roundID, queue.Viewer{UserID: "t022-manager", IsManager: true})
	if err != nil {
		t.Fatalf("load queue state %s: %v", roundID, err)
	}
	return state
}

func assertRoundLifecycle(t *testing.T, repo *queue.Repository, roundID string, wantLifecycle queue.RoundLifecycle, wantNumber int) {
	t.Helper()
	state := roundFlowState(t, repo, roundID)
	if state.Round.Lifecycle != wantLifecycle || state.Round.RoundNumber != wantNumber {
		t.Fatalf("round %s = lifecycle=%s number=%d, want lifecycle=%s number=%d", roundID, state.Round.Lifecycle, state.Round.RoundNumber, wantLifecycle, wantNumber)
	}
}

func assertQueueState(t *testing.T, repo *queue.Repository, roundID string, wantEntries, wantPreorder []string) {
	t.Helper()
	state := roundFlowState(t, repo, roundID)
	if wantEntries == nil {
		if len(state.Entries) != 0 {
			t.Fatalf("round %s entries = %v, want none", roundID, queueFlowStudentOrder(state))
		}
	} else if got := queueFlowStudentOrder(state); !reflect.DeepEqual(got, wantEntries) {
		t.Fatalf("round %s durable student order = %v, want %v", roundID, got, wantEntries)
	}
	gotPreorder := make([]string, 0, len(state.Preorder))
	for _, candidate := range state.Preorder {
		gotPreorder = append(gotPreorder, candidate.StudentID)
	}
	if wantPreorder == nil {
		if len(gotPreorder) != 0 {
			t.Fatalf("round %s preorder = %v, want none", roundID, gotPreorder)
		}
	} else if !reflect.DeepEqual(gotPreorder, wantPreorder) {
		t.Fatalf("round %s durable preorder = %v, want %v", roundID, gotPreorder, wantPreorder)
	}
}

func assertNoDuplicateQueuePositionsOrStudents(t *testing.T, state queue.QueueState) {
	t.Helper()
	positions := make(map[int]struct{}, len(state.Entries))
	students := make(map[string]struct{}, len(state.Entries))
	for _, entry := range state.Entries {
		if _, exists := positions[entry.Position]; exists {
			t.Fatalf("duplicate durable queue position %d in %+v", entry.Position, state.Entries)
		}
		if _, exists := students[entry.StudentID]; exists {
			t.Fatalf("duplicate durable queue student %s in %+v", entry.StudentID, state.Entries)
		}
		positions[entry.Position] = struct{}{}
		students[entry.StudentID] = struct{}{}
	}
}

func assertStatusCount(t *testing.T, state queue.QueueState, status queue.EntryStatus, want int) {
	t.Helper()
	got := 0
	for _, entry := range state.Entries {
		if entry.Status == status {
			got++
		}
	}
	if got != want {
		t.Fatalf("status %s count = %d, want %d in %+v", status, got, want, state.Entries)
	}
}

func queueFlowStudentOrder(state queue.QueueState) []string {
	order := make([]string, 0, len(state.Entries))
	for _, entry := range state.Entries {
		order = append(order, entry.StudentID)
	}
	return order
}

func queueFlowRecitingEntries(state queue.QueueState) []queue.QueueEntry {
	entries := make([]queue.QueueEntry, 0, 1)
	for _, entry := range state.Entries {
		if entry.Status == queue.EntryStatusReciting {
			entries = append(entries, entry)
		}
	}
	return entries
}

func queueFlowEntryIDs(state queue.QueueState) []string {
	ids := make([]string, 0, len(state.Entries))
	for _, entry := range state.Entries {
		ids = append(ids, entry.ID)
	}
	return ids
}

func intersectStrings(left, right []string) []string {
	rightSet := make(map[string]struct{}, len(right))
	for _, value := range right {
		rightSet[value] = struct{}{}
	}
	var intersection []string
	for _, value := range left {
		if _, ok := rightSet[value]; ok {
			intersection = append(intersection, value)
		}
	}
	return intersection
}
