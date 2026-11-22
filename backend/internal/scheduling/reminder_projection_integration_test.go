//go:build integration

package scheduling

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/KarimMFadel/halaqaty/backend/internal/auth"
	"github.com/KarimMFadel/halaqaty/backend/internal/notifications"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// openReminderProjectionTestPool extends the F-006 migration harness with the
// F-008 notification schema so the reminder scanner, the recheck helper and
// the dispatcher recheck seam run against real tables, checks and unique keys.
func openReminderProjectionTestPool(t *testing.T, ctx context.Context) *pgxpool.Pool {
	t.Helper()
	pool := openRepoTestPool(t, ctx)
	_, currentFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("locate reminder projection integration test")
	}
	migration, err := os.ReadFile(filepath.Join(filepath.Dir(currentFile), "..", "..", "migrations", "000021_notification_system.up.sql"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, string(migration)); err != nil {
		t.Fatalf("apply F-008 migration: %v", err)
	}
	return pool
}

type reminderFixture struct {
	pool     *pgxpool.Pool
	sched    *Repository
	scanner  *ReminderScanner
	notices  *notifications.Repository
	teacher  uuid.UUID
	student  uuid.UUID
	dropped  uuid.UUID
	circleID uuid.UUID
	// startAt is the seeded occurrence's planned start; reminder offsets all
	// derive from it.
	startAt time.Time
	// scanNow is the injected scan instant; the fixture story is expressed
	// relative to these two values, so wall-time drift during the test cannot
	// change expectations.
	scanNow time.Time
	// startIn/scanLead document the construction (startIn + scanLead after
	// the wall clock / before the occurrence start).
	startIn, scanLead time.Duration
}

// seedReminderFixture builds one UTC circle with a teacher, a student and a
// deleted (former) member, plus one daily-recurring schedule whose next
// occurrence starts scanLead + startIn after the injected scan instant.
func seedReminderFixtureWithLead(t *testing.T, ctx context.Context, scanLead, startIn time.Duration) *reminderFixture {
	t.Helper()
	pool := openReminderProjectionTestPool(t, ctx)
	f := &reminderFixture{pool: pool, scanLead: scanLead, startIn: startIn}
	f.teacher = uuid.New()
	f.student = uuid.New()
	f.dropped = uuid.New()
	f.circleID = uuid.New()
	seedRepoUser(t, ctx, pool, f.teacher.String())
	seedRepoUser(t, ctx, pool, f.student.String())
	seedRepoUser(t, ctx, pool, f.dropped.String())
	seedRepoCircle(t, ctx, pool, f.circleID.String(), f.teacher.String(), "HLQ-"+f.circleID.String()[:8])
	for _, member := range []struct {
		id   uuid.UUID
		role string
	}{{f.teacher, "teacher"}, {f.student, "student"}, {f.dropped, "student"}} {
		if _, err := pool.Exec(ctx, `INSERT INTO circle_members (circle_id, user_id, role) VALUES ($1::uuid, $2::uuid, $3)`, f.circleID, member.id, member.role); err != nil {
			t.Fatal(err)
		}
	}
	// The former member no longer receives anything (recipient gate).
	if _, err := pool.Exec(ctx, `UPDATE users SET deleted_at = NOW() WHERE id = $1::uuid`, f.dropped); err != nil {
		t.Fatal(err)
	}

	f.startAt = time.Now().UTC().Truncate(time.Minute).Add(startIn).Add(scanLead)
	f.scanNow = f.startAt.Add(-scanLead)
	f.sched = NewScheduleRepository(pool)
	f.scanner = NewReminderScanner(f.sched, func() time.Time { return f.scanNow })
	f.notices = notifications.NewRepository(pool)

	f.seedSchedule(t, ctx, civilOf(f.startAt), f.startAt.Hour(), f.startAt.Minute())
	return f
}

func seedReminderFixture(t *testing.T, ctx context.Context) *reminderFixture {
	t.Helper()
	return seedReminderFixtureWithLead(t, ctx, 16*time.Minute, 0)
}

// seedSchedule inserts one daily-recurring revision anchored at date whose
// next occurrence starts at the given UTC clock.
func (f *reminderFixture) seedSchedule(t *testing.T, ctx context.Context, date time.Time, hour, minute int) uuid.UUID {
	t.Helper()
	clock := LocalClock{Hour: hour, Minute: minute}
	record := RevisionRecord{
		Revision: Revision{
			Version:            1,
			EffectiveLocalDate: date,
			AnchorLocalDate:    date,
			Mode:               ModeInterval,
			IntervalCount:      1,
			IntervalUnit:       IntervalUnitDay,
		},
		Title:           "Circle Session",
		StartLocalTime:  clock,
		EndLocalTime:    NominalEndClock(clock, 60),
		DurationMinutes: 60,
		Timezone:        "UTC",
	}
	schedule, err := f.sched.CreateSchedule(ctx, f.circleID.String(), f.teacher.String(), record)
	if err != nil {
		t.Fatalf("seed reminder schedule: %v", err)
	}
	return uuid.MustParse(schedule.ID)
}

// reminderRowCount counts session_reminder rows for one circle, optionally for
// one recipient.
func reminderRowCount(t *testing.T, ctx context.Context, pool *pgxpool.Pool, circleID string, userID *uuid.UUID) int {
	t.Helper()
	var count int
	query := `SELECT COUNT(*) FROM notifications WHERE type = 'session_reminder' AND source_ref_id = $1`
	args := []any{circleID}
	if userID != nil {
		query += ` AND user_id = $2`
		args = append(args, *userID)
	}
	if err := pool.QueryRow(ctx, query, args...).Scan(&count); err != nil {
		t.Fatal(err)
	}
	return count
}

// The scanner enqueues recipient-scoped reminders only for DUE-ISH offsets
// whose expiry chain is unexpired, stamps the reminder's NotifyAt as
// created_at and the approved next-closer expiry as push_expires_at, and stays
// idempotent across rescans (unique key). The deleted former member never
// receives anything.
func TestReminderScannerEnqueuesDueIdempotentRecipientReminders(t *testing.T) {
	ctx := context.Background()
	f := seedReminderFixture(t, ctx)

	enqueued, err := f.scanner.EnqueueDueReminders(ctx, f.scanNow)
	if err != nil {
		t.Fatal(err)
	}
	if enqueued != 4 { // 30m + 15m for both active members
		t.Fatalf("enqueued=%d, want 4 (30m and 15m for both active members)", enqueued)
	}
	for _, member := range []uuid.UUID{f.teacher, f.student} {
		if count := reminderRowCount(t, ctx, f.pool, f.circleID.String(), &member); count != 2 {
			t.Fatalf("member %s reminders=%d, want 2", member, count)
		}
	}
	if count := reminderRowCount(t, ctx, f.pool, f.circleID.String(), &f.dropped); count != 0 {
		t.Fatalf("deleted member received %d reminders, want 0", count)
	}

	// Content and expiry-chain projection of the two created offsets, keyed
	// by created_at (the reminder's NotifyAt).
	rows, err := f.pool.Query(ctx, `
SELECT source_event_id, source_ref_type, created_at, push_expires_at,
       title, body, delivery_state, is_read
FROM notifications
WHERE type = 'session_reminder' AND user_id = $1 AND source_ref_id = $2
ORDER BY created_at`, f.student, f.circleID)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	type projection struct {
		sourceEventID, sourceRefType, title, body, deliveryState string
		createdAt, pushExpiresAt                                 time.Time
		isRead                                                   bool
	}
	var got []projection
	for rows.Next() {
		var r projection
		if err := rows.Scan(&r.sourceEventID, &r.sourceRefType, &r.createdAt, &r.pushExpiresAt, &r.title, &r.body, &r.deliveryState, &r.isRead); err != nil {
			t.Fatal(err)
		}
		got = append(got, r)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	wantExpiry := map[time.Duration]time.Time{ // offset -> push_expires_at
		30 * time.Minute: f.startAt.Add(-15 * time.Minute),
		15 * time.Minute: f.startAt.Add(-5 * time.Minute),
	}
	if len(got) != 2 {
		t.Fatalf("student reminders=%d, want the 30m and 15m offsets", len(got))
	}
	for _, r := range got {
		if r.sourceRefType != "circle" {
			t.Fatalf("source_ref_type=%q, want circle (planned occurrence's authorized shared resource)", r.sourceRefType)
		}
		if r.title != "Session reminder" || r.body != "Your scheduled session is starting soon." {
			t.Fatalf("projection=(%q, %q), want generic lock-screen copy", r.title, r.body)
		}
		if r.deliveryState != "pending_no_device" || r.isRead {
			t.Fatalf("state=(%q, read=%t), want a fresh pending unread intent", r.deliveryState, r.isRead)
		}
		_, date, version, offset, err := ParseReminderSourceEventID(r.sourceEventID)
		if err != nil {
			t.Fatalf("source_event_id %q: %v", r.sourceEventID, err)
		}
		if !date.Equal(civilOf(f.startAt)) || version != 1 {
			t.Fatalf("source identity=(%s, v%d), want (occurrence date, v1)", date, version)
		}
		if expiry := wantExpiry[offset]; !r.pushExpiresAt.Equal(expiry) {
			t.Fatalf("reminder %s expires at %s, want the next closer offset %s", offset, r.pushExpiresAt, expiry)
		}
		if want := f.startAt.Add(-offset); !r.createdAt.Equal(want) {
			t.Fatalf("created_at=%s, want the reminder NotifyAt %s", r.createdAt, want)
		}
	}
}

// Late ticks after downtime still enqueue reminders whose expiry has not
// passed (the final 5m one), while already-expired offsets are never created
// and the unique key keeps rescans idempotent.
func TestReminderScannerLateTickAndIdempotentRescan(t *testing.T) {
	ctx := context.Background()
	f := seedReminderFixture(t, ctx)

	if _, err := f.scanner.EnqueueDueReminders(ctx, f.scanNow); err != nil {
		t.Fatal(err)
	}
	if before := reminderRowCount(t, ctx, f.pool, f.circleID.String(), nil); before != 4 {
		t.Fatalf("first scan rows=%d, want 4", before)
	}
	// Late tick 4 minutes later: the 30m reminder again enters the DUE-ISH
	// window but its expiry has passed and it already exists; the 15m one is
	// idempotent; the 5m one is still early — nothing new appears.
	if _, err := f.scanner.EnqueueDueReminders(ctx, f.scanNow.Add(4*time.Minute)); err != nil {
		t.Fatal(err)
	}
	if after := reminderRowCount(t, ctx, f.pool, f.circleID.String(), nil); after != 4 {
		t.Fatalf("late-tick rows=%d, want exactly 4 (idempotent)", after)
	}
	// Close to occurrence start the final 5m reminder becomes due while its
	// occurrence-start expiry is still future.
	if _, err := f.scanner.EnqueueDueReminders(ctx, f.startAt.Add(-6*time.Minute)); err != nil {
		t.Fatal(err)
	}
	if after := reminderRowCount(t, ctx, f.pool, f.circleID.String(), nil); after != 6 {
		t.Fatalf("rows after final-offset scan=%d, want 6 (5m reminder per member)", after)
	}
}

// Cancelled and stopped occurrences produce no reminders at any scan time,
// and reviving a cancellation restores generation.
func TestReminderScannerSkipsCancelledAndStoppedOccurrences(t *testing.T) {
	ctx := context.Background()
	f := seedReminderFixture(t, ctx)
	actor := f.teacher.String()
	scheduleID, err := f.sched.ListSchedulesForReminders(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(scheduleID) != 1 {
		t.Fatalf("fixture schedules=%d, want 1", len(scheduleID))
	}

	// Cancel the one occurrence that starts inside the scan window.
	if _, err := f.sched.UpsertException(ctx, OccurrenceException{
		ScheduleID:        scheduleID[0].ID,
		OriginalLocalDate: civilOf(f.startAt),
		CancelledAt:       &f.scanNow,
		UpdatedBy:         actor,
	}); err != nil {
		t.Fatalf("cancel occurrence: %v", err)
	}
	for _, scan := range []time.Time{f.scanNow, f.startAt.Add(-6 * time.Minute)} {
		if _, err := f.scanner.EnqueueDueReminders(ctx, scan); err != nil {
			t.Fatal(err)
		}
	}
	if count := reminderRowCount(t, ctx, f.pool, f.circleID.String(), nil); count != 0 {
		t.Fatalf("cancelled occurrence produced %d reminders, want 0", count)
	}

	// Reviving the cancellation restores generation (late tick: 15m and 5m
	// reminders remain inside their unexpired windows).
	if _, err := f.sched.UpsertException(ctx, OccurrenceException{
		ScheduleID:        scheduleID[0].ID,
		OriginalLocalDate: civilOf(f.startAt),
		UpdatedBy:         actor,
	}); err != nil {
		t.Fatalf("revive occurrence: %v", err)
	}
	if _, err := f.scanner.EnqueueDueReminders(ctx, f.startAt.Add(-6*time.Minute)); err != nil {
		t.Fatal(err)
	}
	if count := reminderRowCount(t, ctx, f.pool, f.circleID.String(), nil); count != 4 {
		t.Fatalf("revived occurrence produced %d reminders, want 4 (15m and 5m per member)", count)
	}

	// A schedule stopped from the occurrence's own local date no longer
	// generates anything, even at the final-offset scan.
	stoppedCircle := uuid.New()
	stoppedTeacher := uuid.New()
	stoppedStudent := uuid.New()
	seedRepoUser(t, ctx, f.pool, stoppedTeacher.String())
	seedRepoUser(t, ctx, f.pool, stoppedStudent.String())
	seedRepoCircle(t, ctx, f.pool, stoppedCircle.String(), stoppedTeacher.String(), "HLQ-"+stoppedCircle.String()[:8])
	for _, member := range []struct {
		id   uuid.UUID
		role string
	}{{stoppedTeacher, "teacher"}, {stoppedStudent, "student"}} {
		if _, err := f.pool.Exec(ctx, `INSERT INTO circle_members (circle_id, user_id, role) VALUES ($1::uuid, $2::uuid, $3)`, stoppedCircle, member.id, member.role); err != nil {
			t.Fatal(err)
		}
	}
	stoppedFixture := &reminderFixture{pool: f.pool, sched: f.sched, scanner: f.scanner, notices: f.notices, teacher: stoppedTeacher, student: stoppedStudent, circleID: stoppedCircle, startAt: f.startAt}
	stoppedFixture.seedSchedule(t, ctx, civilOf(f.startAt), f.startAt.Hour(), f.startAt.Minute())
	stopped, err := f.sched.ListSchedulesForReminders(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, schedule := range stopped {
		if schedule.CircleID != stoppedCircle.String() {
			continue
		}
		if _, err := f.sched.StopSchedule(ctx, schedule.ID, schedule.CurrentVersion, civilOf(f.startAt)); err != nil {
			t.Fatalf("stop schedule: %v", err)
		}
	}
	if _, err := f.scanner.EnqueueDueReminders(ctx, f.startAt.Add(-6*time.Minute)); err != nil {
		t.Fatal(err)
	}
	if count := reminderRowCount(t, ctx, f.pool, stoppedCircle.String(), nil); count != 0 {
		t.Fatalf("stopped schedule produced %d reminders, want 0", count)
	}
}

// The DB-backed recheck helper tracks current eligibility: a live reminder
// stays eligible until its occurrence is cancelled, superseded or stopped.
func TestReminderRecheckTracksCurrentEligibility(t *testing.T) {
	ctx := context.Background()
	f := seedReminderFixture(t, ctx)
	schedules, err := f.sched.ListSchedulesForReminders(ctx)
	if err != nil {
		t.Fatal(err)
	}
	scheduleID := schedules[0].ID
	date := civilOf(f.startAt)

	eligible, err := f.sched.RecheckReminder(ctx, scheduleID, date, 1, 5*time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if !eligible {
		t.Fatal("live v1 reminder reported ineligible")
	}

	// Cancelled occurrence -> ineligible; revived -> eligible again.
	if _, err := f.sched.UpsertException(ctx, OccurrenceException{ScheduleID: scheduleID, OriginalLocalDate: date, CancelledAt: &f.scanNow, UpdatedBy: f.teacher.String()}); err != nil {
		t.Fatal(err)
	}
	eligible, err = f.sched.RecheckReminder(ctx, scheduleID, date, 1, 5*time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if eligible {
		t.Fatal("cancelled occurrence reminder reported eligible")
	}
	if _, err := f.sched.UpsertException(ctx, OccurrenceException{ScheduleID: scheduleID, OriginalLocalDate: date, UpdatedBy: f.teacher.String()}); err != nil {
		t.Fatal(err)
	}
	eligible, err = f.sched.RecheckReminder(ctx, scheduleID, date, 1, 5*time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if !eligible {
		t.Fatal("revived occurrence reminder reported ineligible")
	}

	// Superseded revision: v2 governs now, so the stored v1 reminder is
	// ineligible while the v2 reminder is eligible.
	record := RevisionRecord{
		Revision: Revision{
			Version:            2,
			EffectiveLocalDate: date,
			AnchorLocalDate:    date,
			Mode:               ModeInterval,
			IntervalCount:      1,
			IntervalUnit:       IntervalUnitDay,
		},
		Title:           "Circle Session",
		StartLocalTime:  LocalClock{Hour: f.startAt.Hour(), Minute: f.startAt.Minute()},
		EndLocalTime:    NominalEndClock(LocalClock{Hour: f.startAt.Hour(), Minute: f.startAt.Minute()}, 60),
		DurationMinutes: 60,
		Timezone:        "UTC",
	}
	if _, err := f.sched.AppendRevision(ctx, scheduleID, schedules[0].CurrentVersion, record); err != nil {
		t.Fatalf("append v2 revision: %v", err)
	}
	for _, tc := range []struct {
		version int
		want    bool
	}{
		{1, false},
		{2, true},
		{9, false}, // unknown version fails closed
	} {
		eligible, err := f.sched.RecheckReminder(ctx, scheduleID, date, tc.version, 5*time.Minute)
		if err != nil {
			t.Fatalf("recheck v%d: %v", tc.version, err)
		}
		if eligible != tc.want {
			t.Fatalf("recheck v%d=%t, want %t", tc.version, eligible, tc.want)
		}
	}

	// Stopped schedule -> ineligible.
	if _, err := f.sched.StopSchedule(ctx, scheduleID, schedules[0].CurrentVersion+1, date); err != nil {
		t.Fatalf("stop schedule: %v", err)
	}
	eligible, err = f.sched.RecheckReminder(ctx, scheduleID, date, 2, 5*time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if eligible {
		t.Fatal("stopped schedule reminder reported eligible")
	}
}

// reminderRecordingSender records dispatch outcomes without an FCM seam.
type reminderRecordingSender struct {
	sent []notifications.PushMessage
}

func (s *reminderRecordingSender) Send(_ context.Context, message notifications.PushMessage) error {
	s.sent = append(s.sent, message)
	return nil
}

// reminderRecheckClosure is the exported cmd/api wiring constructor exercised
// in-package so the wiring contract cannot drift from the dispatcher gate.
func reminderRecheckClosure(repo *Repository) func(context.Context, notifications.DispatchIntent) (bool, error) {
	return ReminderEligibilityRecheck(repo)
}

// End to end against the notification schema: an eligible enqueued reminder
// dispatches push attempts, while a reminder whose occurrence is cancelled
// after enqueue is suppressed with DeliveryExpired before any provider send,
// leaving its history row retained and unread.
func TestReminderDispatchRecheckSuppressionAgainstDatabase(t *testing.T) {
	ctx := context.Background()

	// Eligible path: enqueued intents reach the provider.
	f := seedReminderFixtureWithLead(t, ctx, 21*time.Minute, 0)
	sender := &reminderRecordingSender{}
	authRepo := auth.NewSessionRepository(f.pool)
	metrics := &notifications.NotificationMetrics{}
	dispatcher := notifications.NewDispatcher(f.notices, sender, authRepo, metrics, time.Now, nil)
	dispatcher.EligibilityRecheck = reminderRecheckClosure(f.sched)

	if enqueued, err := f.scanner.EnqueueDueReminders(ctx, f.scanNow); err != nil || enqueued != 2 {
		t.Fatalf("enqueued=%d err=%v, want the 30m reminder for both members", enqueued, err)
	}
	for _, member := range []uuid.UUID{f.teacher, f.student} {
		if err := f.notices.UpdatePreferences(ctx, member.String(), nil,
			&notifications.QuietHoursUpdate{Enabled: false, StartLocal: "22:00", EndLocal: "07:00"}); err != nil {
			t.Fatal(err)
		}
		if err := authRepo.UpsertDeviceToken(ctx, member.String(), "dev-"+member.String(), "android", nil); err != nil {
			t.Fatal(err)
		}
	}
	if err := dispatcher.DispatchDue(ctx, 10); err != nil {
		t.Fatal(err)
	}
	if got := len(sender.sent); got != 2 {
		t.Fatalf("provider sends=%d, want one per member", got)
	}
	if summary := metrics.Summary().Outcomes; summary[notifications.DispatchAccepted] != 2 {
		t.Fatalf("accepted metric=%d, want 2", summary[notifications.DispatchAccepted])
	}

	// Suppression path: cancel between enqueue and dispatch.
	f2 := seedReminderFixtureWithLead(t, ctx, 21*time.Minute, 0)
	sender2 := &reminderRecordingSender{}
	authRepo2 := auth.NewSessionRepository(f2.pool)
	metrics2 := &notifications.NotificationMetrics{}
	dispatcher2 := notifications.NewDispatcher(f2.notices, sender2, authRepo2, metrics2, time.Now, nil)
	dispatcher2.EligibilityRecheck = reminderRecheckClosure(f2.sched)

	if enqueued, err := f2.scanner.EnqueueDueReminders(ctx, f2.scanNow); err != nil || enqueued != 2 {
		t.Fatalf("enqueued=%d err=%v, want the 30m reminder for both members", enqueued, err)
	}
	schedules, err := f2.sched.ListSchedulesForReminders(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f2.sched.UpsertException(ctx, OccurrenceException{
		ScheduleID:        schedules[0].ID,
		OriginalLocalDate: civilOf(f2.startAt),
		CancelledAt:       &f2.scanNow,
		UpdatedBy:         f2.teacher.String(),
	}); err != nil {
		t.Fatal(err)
	}
	for _, member := range []uuid.UUID{f2.teacher, f2.student} {
		if err := f2.notices.UpdatePreferences(ctx, member.String(), nil,
			&notifications.QuietHoursUpdate{Enabled: false, StartLocal: "22:00", EndLocal: "07:00"}); err != nil {
			t.Fatal(err)
		}
		if err := authRepo2.UpsertDeviceToken(ctx, member.String(), "dev-"+member.String(), "android", nil); err != nil {
			t.Fatal(err)
		}
	}
	if err := dispatcher2.DispatchDue(ctx, 10); err != nil {
		t.Fatal(err)
	}
	if got := len(sender2.sent); got != 0 {
		t.Fatalf("provider sends after cancellation=%d, want 0", got)
	}
	if summary := metrics2.Summary().Outcomes; summary[notifications.DispatchExpired] != 2 || summary[notifications.DispatchAccepted] != 0 {
		t.Fatalf("expired=%d accepted=%d, want 2 and 0",
			summary[notifications.DispatchExpired], summary[notifications.DispatchAccepted])
	}
	var expired, unread int
	if err := f2.pool.QueryRow(ctx, `
SELECT COUNT(*) FILTER (WHERE delivery_state = 'expired'),
       COUNT(*) FILTER (WHERE is_read = FALSE)
FROM notifications
WHERE type = 'session_reminder' AND source_ref_id = $1`, f2.circleID).Scan(&expired, &unread); err != nil {
		t.Fatal(err)
	}
	if expired != 2 || unread != 2 {
		t.Fatalf("expired=%d unread=%d, want history rows retained unread and suppressed", expired, unread)
	}
}
