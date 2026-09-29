//go:build integration

package attendance

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/KarimMFadel/halaqaty/backend/internal/sessions"
)

func TestCorrectionService_TeacherAuditRetryAndRecalculationPrecedence(t *testing.T) {
	ctx := context.Background()
	pool := newAttendancePool(t)
	teacher := attendanceUser(t, pool, "corrector")
	circle := attendanceCircle(t, pool, teacher)
	addAttendanceMember(t, pool, circle, teacher, "teacher")
	student := attendanceUser(t, pool, "corrected-student")
	addAttendanceMember(t, pool, circle, student, "student")
	repo := NewRepository(pool)
	sessionRepo := sessions.NewSessionRepository(pool)
	sessionRepo.SetAttendanceLifecycle(repo)
	session, err := sessionRepo.CreateAdHocSession(ctx, circle, teacher)
	if err != nil {
		t.Fatal(err)
	}
	startAttendanceSession(t, sessionRepo, session.ID, teacher)
	if _, err := sessionRepo.EndSession(ctx, session.ID, sessions.EndReasonManual); err != nil {
		t.Fatal(err)
	}
	service := NewCorrectionService(repo)
	command := CorrectionCommand{ActorID: teacher, SessionID: session.ID, UserID: student, IdempotencyKey: "attendance-correction-1", Status: StatusExcused, Reason: "Teacher approved the absence"}
	first, err := service.Correct(ctx, command)
	if err != nil {
		t.Fatalf("teacher correction: %v", err)
	}
	second, err := service.Correct(ctx, command)
	if err != nil || first.CorrectionID == nil || second.CorrectionID == nil || *first.CorrectionID != *second.CorrectionID {
		t.Fatalf("retry correction = %+v, %v; want same audit record as %+v", second, err, first)
	}
	if _, err := service.Correct(ctx, CorrectionCommand{ActorID: student, SessionID: session.ID, UserID: student, IdempotencyKey: "student-correction", Status: StatusPresent, Reason: "attempt"}); !errors.Is(err, ErrAttendanceForbidden) {
		t.Fatalf("student correction error = %v, want forbidden", err)
	}
	if err := repo.Finalize(ctx, session.ID); err != nil {
		t.Fatalf("recalculation retry: %v", err)
	}
	var base, effective, actor, reason string
	var corrections int
	if err := pool.QueryRow(ctx, `SELECT a.base_status,a.effective_status,c.actor_id::text,c.reason,(SELECT count(*) FROM attendance_corrections WHERE session_id=a.session_id AND user_id=a.user_id) FROM session_attendance a JOIN attendance_corrections c ON c.session_id=a.session_id AND c.user_id=a.user_id WHERE a.session_id=$1::uuid AND a.user_id=$2::uuid`, session.ID, student).Scan(&base, &effective, &actor, &reason, &corrections); err != nil {
		t.Fatal(err)
	}
	if base != string(StatusAbsent) || effective != string(StatusExcused) || actor != teacher || reason != command.Reason || corrections != 1 {
		t.Fatalf("attendance audit after retry: base=%s effective=%s actor=%s reason=%q corrections=%d", base, effective, actor, reason, corrections)
	}
	var at time.Time
	if err := pool.QueryRow(ctx, `SELECT at FROM attendance_corrections WHERE session_id=$1::uuid AND user_id=$2::uuid`, session.ID, student).Scan(&at); err != nil || at.IsZero() {
		t.Fatalf("audit timestamp = %s, err=%v", at, err)
	}
}
