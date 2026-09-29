//go:build integration

package attendance

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

	"github.com/KarimMFadel/halaqaty/backend/internal/sessions"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestAttendanceRepository_SnapshotLateJoinRevocationEndAndRetry(t *testing.T) {
	ctx := context.Background()
	pool := newAttendancePool(t)
	teacher := attendanceUser(t, pool, "teacher")
	circle := attendanceCircle(t, pool, teacher)
	addAttendanceMember(t, pool, circle, teacher, "teacher")
	startedStudent := attendanceUser(t, pool, "started")
	laterStudent := attendanceUser(t, pool, "later")
	for _, student := range []string{startedStudent} {
		addAttendanceMember(t, pool, circle, student, "student")
	}
	repo := NewRepository(pool)
	sessionRepo := sessions.NewSessionRepository(pool)
	sessionRepo.SetAttendanceLifecycle(repo)
	session, err := sessionRepo.CreateAdHocSession(ctx, circle, teacher)
	if err != nil {
		t.Fatal(err)
	}
	startAttendanceSession(t, sessionRepo, session.ID, teacher)

	if source := attendanceSource(t, pool, session.ID, startedStudent); source != string(RosterSourceStartSnapshot) {
		t.Fatalf("start roster source = %q", source)
	}
	addAttendanceMember(t, pool, circle, laterStudent, "student")
	if _, _, err := sessionRepo.JoinSessionWithConnection(ctx, session.ID, laterStudent, sessions.MediaGrants{}, testIssue); err != nil {
		t.Fatalf("authorized later student join: %v", err)
	}
	if _, err := pool.Exec(ctx, `DELETE FROM circle_members WHERE circle_id=$1::uuid AND user_id=$2::uuid`, circle, laterStudent); err != nil {
		t.Fatal(err)
	}
	if source := attendanceSource(t, pool, session.ID, laterStudent); source != string(RosterSourceLaterParticipant) {
		t.Fatalf("later roster source = %q", source)
	}
	firstPresenceBefore := attendanceFirstPresence(t, pool, session.ID, laterStudent)

	if _, err := sessionRepo.EndSession(ctx, session.ID, sessions.EndReasonDurationLimit); err != nil {
		t.Fatalf("automatic end: %v", err)
	}
	if err := repo.Finalize(ctx, session.ID); err != nil {
		t.Fatalf("retry finalization: %v", err)
	}
	var rows, finalized int
	if err := pool.QueryRow(ctx, `SELECT count(*), count(finalized_at) FROM session_attendance WHERE session_id=$1::uuid`, session.ID).Scan(&rows, &finalized); err != nil {
		t.Fatal(err)
	}
	if rows != 2 || finalized != 2 {
		t.Fatalf("attendance rows/finalized = %d/%d, want 2/2", rows, finalized)
	}
	var presenceCount int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM session_participant_presence WHERE session_id=$1::uuid AND first_joined_at IS NOT NULL`, session.ID).Scan(&presenceCount); err != nil {
		t.Fatal(err)
	}
	if presenceCount != 2 {
		t.Fatalf("raw presence count = %d, want 2", presenceCount)
	}
	if after := attendanceFirstPresence(t, pool, session.ID, laterStudent); !after.Equal(firstPresenceBefore) {
		t.Fatalf("finalization changed raw first presence from %s to %s", firstPresenceBefore, after)
	}

	manual, err := sessionRepo.CreateAdHocSession(ctx, circle, teacher)
	if err != nil {
		t.Fatal(err)
	}
	startAttendanceSession(t, sessionRepo, manual.ID, teacher)
	if _, err := sessionRepo.EndSession(ctx, manual.ID, sessions.EndReasonManual); err != nil {
		t.Fatalf("manual end: %v", err)
	}
	var manualRows int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM session_attendance WHERE session_id=$1::uuid AND finalized_at IS NOT NULL`, manual.ID).Scan(&manualRows); err != nil || manualRows != 1 {
		t.Fatalf("manual end finalized rows = %d, err=%v, want 1", manualRows, err)
	}
}

func TestAttendanceRecovery_EndedSessionsRotatePastBoundedPage(t *testing.T) {
	ctx := context.Background()
	pool := newAttendancePool(t)
	teacher := attendanceUser(t, pool, "recovery-teacher")
	circle := attendanceCircle(t, pool, teacher)
	attendanceRepo := NewRepository(pool)
	sessionRepo := sessions.NewSessionRepository(pool)
	createdIDs := make(map[string]struct{}, sessions.RecoveryCandidateLimit+5)
	for range sessions.RecoveryCandidateLimit + 5 {
		session, err := sessionRepo.CreateAdHocSession(ctx, circle, teacher)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := pool.Exec(ctx, `UPDATE sessions SET status='ended', actual_start=NOW()-INTERVAL '1 hour', actual_end=NOW(), updated_at=NOW()-INTERVAL '1 day' WHERE id=$1::uuid`, session.ID); err != nil {
			t.Fatal(err)
		}
		createdIDs[session.ID] = struct{}{}
	}
	firstPage, err := sessionRepo.ListRecoveryCandidates(ctx, sessions.SessionStatusEnded, sessions.RecoveryCandidateLimit)
	if err != nil {
		t.Fatal(err)
	}
	for _, candidate := range firstPage {
		if err := attendanceRepo.Finalize(ctx, candidate.ID); err != nil {
			t.Fatalf("finalize ended session %s: %v", candidate.ID, err)
		}
	}
	secondPage, err := sessionRepo.ListRecoveryCandidates(ctx, sessions.SessionStatusEnded, sessions.RecoveryCandidateLimit)
	if err != nil {
		t.Fatal(err)
	}
	firstIDs := make(map[string]struct{}, len(firstPage))
	for _, candidate := range firstPage {
		firstIDs[candidate.ID] = struct{}{}
	}
	seenUnfinalized := make(map[string]struct{}, 5)
	for _, candidate := range secondPage {
		if _, finalized := firstIDs[candidate.ID]; !finalized {
			seenUnfinalized[candidate.ID] = struct{}{}
		}
	}
	if len(seenUnfinalized) != 5 {
		t.Fatalf("second recovery page includes %d not-yet-finalized sessions, want all 5 beyond the first page", len(seenUnfinalized))
	}
	for sessionID := range createdIDs {
		if _, finalized := firstIDs[sessionID]; finalized {
			continue
		}
		if _, reached := seenUnfinalized[sessionID]; !reached {
			t.Fatalf("session %s beyond the first page was not reached on the next recovery sweep", sessionID)
		}
	}
}

func TestAttendanceRepository_RevocationBeforeJoinCommitDeniesAndPreservesNoRoster(t *testing.T) {
	ctx := context.Background()
	pool := newAttendancePool(t)
	teacher := attendanceUser(t, pool, "race-teacher")
	circle := attendanceCircle(t, pool, teacher)
	addAttendanceMember(t, pool, circle, teacher, "teacher")
	student := attendanceUser(t, pool, "race-student")
	repo := NewRepository(pool)
	sessionRepo := sessions.NewSessionRepository(pool)
	sessionRepo.SetAttendanceLifecycle(repo)
	session, err := sessionRepo.CreateAdHocSession(ctx, circle, teacher)
	if err != nil {
		t.Fatal(err)
	}
	startAttendanceSession(t, sessionRepo, session.ID, teacher)
	addAttendanceMember(t, pool, circle, student, "student")

	revocation, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := revocation.Exec(ctx, `DELETE FROM circle_members WHERE circle_id=$1::uuid AND user_id=$2::uuid`, circle, student); err != nil {
		t.Fatal(err)
	}
	joined := make(chan error, 1)
	go func() {
		_, _, err := sessionRepo.JoinSessionWithConnection(ctx, session.ID, student, sessions.MediaGrants{}, testIssue)
		joined <- err
	}()
	select {
	case err := <-joined:
		_ = revocation.Rollback(ctx)
		t.Fatalf("join passed an uncommitted membership revocation lock: %v", err)
	case <-time.After(50 * time.Millisecond):
	}
	if err := revocation.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	if err := <-joined; !errors.Is(err, ErrStudentMembershipRequired) {
		t.Fatalf("join after committed revocation = %v, want ErrStudentMembershipRequired", err)
	}
	var presence, roster int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM session_participant_presence WHERE session_id=$1::uuid AND user_id=$2::uuid`, session.ID, student).Scan(&presence); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM session_attendance WHERE session_id=$1::uuid AND user_id=$2::uuid`, session.ID, student).Scan(&roster); err != nil {
		t.Fatal(err)
	}
	if presence != 0 || roster != 0 {
		t.Fatalf("revoked join left presence/roster rows = %d/%d", presence, roster)
	}
}

func TestAttendanceRepository_ReadScopesAndArchivedHistory(t *testing.T) {
	ctx := context.Background()
	pool := newAttendancePool(t)
	teacher := attendanceUser(t, pool, "scope-teacher")
	circle := attendanceCircle(t, pool, teacher)
	supervisor := attendanceUser(t, pool, "scope-supervisor")
	studentOne := attendanceUser(t, pool, "scope-student-one")
	studentTwo := attendanceUser(t, pool, "scope-student-two")
	outsider := attendanceUser(t, pool, "scope-outsider")
	addAttendanceMember(t, pool, circle, teacher, "teacher")
	addAttendanceMember(t, pool, circle, supervisor, "supervisor")
	addAttendanceMember(t, pool, circle, studentOne, "student")
	addAttendanceMember(t, pool, circle, studentTwo, "student")
	repo := NewRepository(pool)
	sessionRepo := sessions.NewSessionRepository(pool)
	sessionRepo.SetAttendanceLifecycle(repo)
	session, err := sessionRepo.CreateAdHocSession(ctx, circle, teacher)
	if err != nil {
		t.Fatal(err)
	}
	startAttendanceSession(t, sessionRepo, session.ID, teacher)
	if _, _, err := sessionRepo.JoinSessionWithConnection(ctx, session.ID, studentOne, sessions.MediaGrants{}, testIssue); err != nil {
		t.Fatal(err)
	}
	if _, err := sessionRepo.EndSession(ctx, session.ID, sessions.EndReasonManual); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		actor string
		count int
	}{{teacher, 2}, {supervisor, 2}, {studentOne, 1}, {studentTwo, 1}} {
		records, err := repo.List(ctx, session.ID, tc.actor)
		if err != nil || len(records) != tc.count {
			t.Fatalf("attendance for %s = %d, %v; want %d", tc.actor, len(records), err, tc.count)
		}
	}
	if _, err := repo.List(ctx, session.ID, outsider); !errors.Is(err, ErrAttendanceNotFound) {
		t.Fatalf("non-member attendance error = %v, want concealed not-found", err)
	}
	if _, err := pool.Exec(ctx, `DELETE FROM circle_members WHERE circle_id=$1::uuid AND user_id=$2::uuid`, circle, studentOne); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.List(ctx, session.ID, studentOne); !errors.Is(err, ErrAttendanceNotFound) {
		t.Fatalf("revoked member attendance error = %v, want concealed not-found", err)
	}
	if _, err := pool.Exec(ctx, `UPDATE circles SET is_archived=TRUE WHERE id=$1::uuid`, circle); err != nil {
		t.Fatal(err)
	}
	if records, err := repo.List(ctx, session.ID, teacher); err != nil || len(records) != 2 {
		t.Fatalf("archived history read = %d, %v; want 2 records", len(records), err)
	}
	_, err = NewCorrectionService(repo).Correct(ctx, CorrectionCommand{
		ActorID: teacher, SessionID: session.ID, UserID: studentTwo,
		IdempotencyKey: "archived-correction", Status: StatusExcused, Reason: "Excused",
	})
	if !errors.Is(err, ErrAttendanceArchived) {
		t.Fatalf("archived correction error = %v, want read-only denial", err)
	}
}

func newAttendancePool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		t.Skip("DATABASE_URL is not set")
	}
	schema := fmt.Sprintf("test_attendance_%d", time.Now().UnixNano())
	sep := "?"
	if strings.Contains(dbURL, sep) {
		sep = "&"
	}
	pool, err := pgxpool.New(context.Background(), dbURL+sep+"search_path="+schema)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(context.Background(), "CREATE SCHEMA "+schema); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), "DROP SCHEMA IF EXISTS "+schema+" CASCADE")
		pool.Close()
	})
	_, path, _, _ := runtime.Caller(0)
	migrations := filepath.Join(filepath.Dir(path), "..", "..", "migrations")
	for _, name := range []string{"000010_auth_roles_profile.up.sql", "000011_auth_roles_profile_alignment.up.sql", "000012_auth_profiles_display_name.up.sql", "000013_create_circles.up.sql", "000014_circle_members_circle_fk.up.sql", "000015_circle_management.up.sql", "000016_live_sessions.up.sql", "000019_account_deletion_tombstone.up.sql", "000020_schedule_calendar_attendance.up.sql"} {
		sql, err := os.ReadFile(filepath.Join(migrations, name))
		if err != nil {
			t.Fatal(err)
		}
		if _, err := pool.Exec(context.Background(), string(sql)); err != nil {
			t.Fatalf("apply %s: %v", name, err)
		}
	}
	return pool
}

func attendanceUser(t *testing.T, pool *pgxpool.Pool, label string) string {
	t.Helper()
	var id string
	if err := pool.QueryRow(context.Background(), `INSERT INTO users (firebase_uid,email) VALUES ($1,$2) RETURNING id::text`, "attendance-"+label, label+"@example.com").Scan(&id); err != nil {
		t.Fatal(err)
	}
	return id
}

func attendanceCircle(t *testing.T, pool *pgxpool.Pool, teacher string) string {
	t.Helper()
	var id string
	if err := pool.QueryRow(context.Background(), `INSERT INTO circles (name,teacher_id,invite_code) VALUES ('Attendance',$1::uuid,'HLQ-ATT001') RETURNING id::text`, teacher).Scan(&id); err != nil {
		t.Fatal(err)
	}
	return id
}

func addAttendanceMember(t *testing.T, pool *pgxpool.Pool, circle, user, role string) {
	t.Helper()
	if _, err := pool.Exec(context.Background(), `INSERT INTO circle_members(circle_id,user_id,role) VALUES ($1::uuid,$2::uuid,$3)`, circle, user, role); err != nil {
		t.Fatal(err)
	}
}

func startAttendanceSession(t *testing.T, repo *sessions.Repository, sessionID, teacher string) {
	t.Helper()
	if _, _, err := repo.StartSessionWithConnection(context.Background(), sessionID, teacher, sessions.MediaRoomRef("attendance-room-"+sessionID), sessions.MediaGrants{}, func(context.Context, sessions.MediaRoomRef, sessions.MediaMode) error { return nil }, testIssue); err != nil {
		t.Fatal(err)
	}
}

func testIssue(context.Context, sessions.MediaRoomRef, sessions.MediaGrants) (sessions.MediaConnection, error) {
	return sessions.MediaConnection{Endpoint: "wss://example.test", Credential: "test", ExpiresAt: time.Now().Add(time.Hour)}, nil
}

func attendanceSource(t *testing.T, pool *pgxpool.Pool, sessionID, userID string) string {
	t.Helper()
	var source string
	if err := pool.QueryRow(context.Background(), `SELECT roster_source FROM session_attendance WHERE session_id=$1::uuid AND user_id=$2::uuid`, sessionID, userID).Scan(&source); err != nil {
		t.Fatal(err)
	}
	return source
}

func attendanceFirstPresence(t *testing.T, pool *pgxpool.Pool, sessionID, userID string) time.Time {
	t.Helper()
	var first time.Time
	if err := pool.QueryRow(context.Background(), `SELECT first_joined_at FROM session_participant_presence WHERE session_id=$1::uuid AND user_id=$2::uuid`, sessionID, userID).Scan(&first); err != nil {
		t.Fatal(err)
	}
	return first
}
