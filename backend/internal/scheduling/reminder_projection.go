package scheduling

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"time"

	"github.com/KarimMFadel/halaqaty/backend/internal/notifications"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// reminderProjection implements F-006 reminder scheduling for F-008. F-006
// owns reminder eligibility and candidate enumeration (reminder_eligibility.go,
// Spec-Kit T058/T059); this file projects the eligible reminders into the
// notifications package exactly the way other source features do — it owns no
// preference, quiet-hours, or dispatch logic (ADR-027 §3).

// reminderSourceEventPrefix is the self-describing prefix of every F-006
// reminder source event identifier.
const reminderSourceEventPrefix = "session.reminder"

// ReminderOffsets returns the approved notification-matrix reminder offsets in
// ascending order: 1 hour, 30 minutes, 15 minutes and 5 minutes before start.
func ReminderOffsets() []time.Duration {
	return []time.Duration{
		time.Hour,
		30 * time.Minute,
		15 * time.Minute,
		5 * time.Minute,
	}
}

// ReminderExpiries maps one EligibleReminders result (ascending ReminderOffsets
// order) onto the approved pending-reminder expiry chain: reminder k expires
// when the next closer configured offset becomes due, and the final reminder
// expires at occurrence start (2026-10-17 clarification of FR-008). The input
// must come from the ascending-offset derivation, which orders NotifyAt
// ascending by construction.
func ReminderExpiries(reminders []ReminderEligibility) []time.Time {
	if len(reminders) == 0 {
		return nil
	}
	expiries := make([]time.Time, len(reminders))
	for i := range reminders {
		expiries[i] = reminders[i].StartsAt
		if i+1 < len(reminders) {
			expiries[i] = reminders[i+1].NotifyAt
		}
	}
	return expiries
}

// ReminderDue reports whether one reminder is DUE-ISH at now: its NotifyAt
// arrived within the enqueue lead (a late tick after operator downtime still
// qualifies) and its approved expiry has not passed, so expired reminders are
// never created by any tick.
func ReminderDue(now time.Time, reminder ReminderEligibility, expiry time.Time, lead time.Duration) bool {
	return !now.Add(lead).Before(reminder.NotifyAt) && expiry.After(now)
}

// ReminderSourceEventID composes the stable, self-describing source event
// identifier of one reminder: `session.reminder:<scheduleID>:<YYYY-MM-DD>:<version>:<offsetMinutes>`.
// It is the single source of truth for the format; ParseReminderSourceEventID
// is its inverse.
func ReminderSourceEventID(scheduleID string, originalLocalDate time.Time, version int, offset time.Duration) string {
	return fmt.Sprintf("%s:%s:%s:%d:%d",
		reminderSourceEventPrefix, scheduleID, civilOf(originalLocalDate).Format(time.DateOnly), version, int(offset/time.Minute))
}

// ParseReminderSourceEventID decomposes one ReminderSourceEventID back into
// its identity, validating every part so malformed values fail closed.
func ParseReminderSourceEventID(sourceEventID string) (scheduleID string, originalLocalDate time.Time, version int, offset time.Duration, err error) {
	parts := strings.SplitN(sourceEventID, ":", 5)
	if len(parts) != 5 || parts[0] != reminderSourceEventPrefix {
		return "", time.Time{}, 0, 0, fmt.Errorf("reminder source_event_id %q: want %s:<scheduleID>:<date>:<version>:<offsetMinutes>", sourceEventID, reminderSourceEventPrefix)
	}
	scheduleID = parts[1]
	if _, err = uuid.Parse(scheduleID); err != nil {
		return "", time.Time{}, 0, 0, fmt.Errorf("reminder source_event_id %q schedule: %w", sourceEventID, err)
	}
	date, err := time.Parse(time.DateOnly, parts[2])
	if err != nil {
		return "", time.Time{}, 0, 0, fmt.Errorf("reminder source_event_id %q date: %w", sourceEventID, err)
	}
	version, err = strconv.Atoi(parts[3])
	if err != nil {
		return "", time.Time{}, 0, 0, fmt.Errorf("reminder source_event_id %q version: %w", sourceEventID, err)
	}
	if version <= 0 {
		return "", time.Time{}, 0, 0, fmt.Errorf("reminder source_event_id %q version %d is not positive", sourceEventID, version)
	}
	minutes, err := strconv.Atoi(parts[4])
	if err != nil {
		return "", time.Time{}, 0, 0, fmt.Errorf("reminder source_event_id %q offset: %w", sourceEventID, err)
	}
	if minutes <= 0 {
		return "", time.Time{}, 0, 0, fmt.Errorf("reminder source_event_id %q offset %d minutes is not positive", sourceEventID, minutes)
	}
	return scheduleID, date, version, time.Duration(minutes) * time.Minute, nil
}

// StoredReminderOccurrence carries one reminder's occurrence identity plus the
// source-side state the dispatch-time recheck must consult.
type StoredReminderOccurrence struct {
	Revisions            []RevisionRecord
	ScheduleID           string
	OriginalLocalDate    time.Time
	Version              int
	Offset               time.Duration
	Cancelled            bool
	StoppedFromLocalDate *time.Time
}

// ReminderStillEligible reanswers F-006's authoritative boundary for one
// stored reminder identity at dispatch time. It reuses EligibleReminders —
// no eligibility rule is duplicated — and additionally applies the schedule's
// stop boundary, which the derivation-only boundary does not know. Cancelled
// exceptions, superseded versions, stopped dates and no-longer-generated
// occurrences are ineligible (spec Source Feature Ownership, F-006 row).
func ReminderStillEligible(occurrence StoredReminderOccurrence) bool {
	date := civilOf(occurrence.OriginalLocalDate)
	if dateIsStopped(date, occurrence.StoppedFromLocalDate) {
		return false
	}
	eligible, err := EligibleReminders(
		occurrence.Revisions,
		Occurrence{ScheduleID: occurrence.ScheduleID, Version: occurrence.Version, OriginalLocalDate: date},
		occurrence.Cancelled,
		[]time.Duration{occurrence.Offset},
	)
	return err == nil && len(eligible) == 1
}

const (
	// DefaultReminderScanInterval is the fallback scanner cadence; wiring
	// configures the real one.
	DefaultReminderScanInterval = time.Minute
	// defaultReminderEnqueueLead widens the DUE-ISH comparison so a tick that
	// lands a little after a reminder's NotifyAt still creates it exactly
	// once (late recovery, idempotent by unique key).
	defaultReminderEnqueueLead = 2 * DefaultReminderScanInterval
	// sessionReminderTitle/Body are the generic lock-screen copy required by
	// the notification-content decision ("Notification content — 2026-10-01"):
	// no private recitation content is projected into intent rows.
	sessionReminderTitle = "Session reminder"
	sessionReminderBody  = "Your scheduled session is starting soon."
)

// reminderExceptionKey builds the scanner's per-(schedule, date) cancellation
// lookup identity.
func reminderExceptionKey(scheduleID string, originalLocalDate time.Time) string {
	return scheduleID + "|" + civilOf(originalLocalDate).Format(time.DateOnly)
}

// ReminderScanner projects due F-006 reminders into notifications on an
// interval. It reuses the authoritative eligibility boundary; its only added
// rules are the schedule stop boundary, the recipient gate and the approved
// content projection.
type ReminderScanner struct {
	repo *Repository
	now  func() time.Time
}

// NewReminderScanner constructs a reminder scanner. now is injected for
// deterministic tests; nil selects the wall clock.
func NewReminderScanner(repo *Repository, now func() time.Time) *ReminderScanner {
	if now == nil {
		now = time.Now
	}
	return &ReminderScanner{repo: repo, now: now}
}

// Run enqueues due reminders on the given interval until the context is
// cancelled, tolerating per-cycle failures like the notification dispatcher.
func (s *ReminderScanner) Run(ctx context.Context, interval time.Duration) {
	if interval <= 0 {
		interval = DefaultReminderScanInterval
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		if _, err := s.EnqueueDueReminders(ctx, s.now()); err != nil && ctx.Err() == nil {
			slog.WarnContext(ctx, "reminder enqueue cycle failed", "error", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

// EnqueueDueReminders enumerates occurrences of every schedule inside a
// bounded civil-date window around now, derives F-006's eligible reminders and
// persists one recipient-scoped intent per due reminder inside a notification
// transaction. Expired reminders are never created, late ticks recover
// unexpired ones, and the notifications unique key keeps rescans idempotent,
// so the return count can never underestimate created rows. It evaluates
// history-free derivation rules (cancelled occurrences, superseded versions,
// no-longer-generated dates, stopped boundaries) directly against the current
// database state — moved-exception occurrences (replacement_local_date) are a
// documented gap, like the early-start edge (T022 gap notes).
func (s *ReminderScanner) EnqueueDueReminders(ctx context.Context, now time.Time) (int, error) {
	schedules, err := s.repo.ListSchedulesForReminders(ctx)
	if err != nil {
		return 0, fmt.Errorf("list reminder schedules: %w", err)
	}
	if len(schedules) == 0 {
		return 0, nil
	}
	scheduleIDs := make([]string, len(schedules))
	for i, schedule := range schedules {
		scheduleIDs[i] = schedule.ID
	}
	revisionsByID, err := s.repo.LoadRevisions(ctx, scheduleIDs)
	if err != nil {
		return 0, fmt.Errorf("load reminder revisions: %w", err)
	}

	// occurrences generated from civil dates whose session can fall inside
	// [now, now+offsets+lead]; ±26h covers the union of timezone offsets.
	maxSpan := ReminderOffsets()[0] + defaultReminderEnqueueLead
	fromDate := civilOf(now.Add(-26 * time.Hour))
	toDate := civilOf(now.Add(maxSpan + 26*time.Hour))
	cancelled, err := s.repo.listCancelledReminderExceptions(ctx, scheduleIDs, fromDate, toDate)
	if err != nil {
		return 0, err
	}

	enqueued := 0
	for _, schedule := range schedules {
		records, ok := revisionsByID[schedule.ID]
		if !ok {
			continue
		}
		revisions := revisionValues(records)
		for _, occurrence := range OccurrenceWindow(revisions, fromDate, toDate) {
			if dateIsStopped(occurrence.OriginalLocalDate, schedule.StoppedFromLocalDate) ||
				cancelled[reminderExceptionKey(occurrence.ScheduleID, occurrence.OriginalLocalDate)] {
				continue
			}
			eligible, err := EligibleReminders(records, occurrence, false, ReminderOffsets())
			if err != nil {
				return enqueued, fmt.Errorf("derive reminders for %s at %s: %w",
					occurrence.ScheduleID, civilOf(occurrence.OriginalLocalDate), err)
			}
			expiries := ReminderExpiries(eligible)
			due := make([]dueReminder, 0, len(eligible))
			for k, reminder := range eligible {
				if ReminderDue(now, reminder, expiries[k], defaultReminderEnqueueLead) {
					due = append(due, dueReminder{reminder: reminder, expiry: expiries[k]})
				}
			}
			if len(due) == 0 {
				continue
			}
			recipients, err := s.repo.listReminderRecipients(ctx, schedule.CircleID)
			if err != nil {
				return enqueued, err
			}
			if len(recipients) == 0 {
				continue
			}
			inserted, err := s.repo.createReminderNotifications(ctx, schedule.CircleID, due, recipients)
			if err != nil {
				return enqueued, err
			}
			enqueued += inserted
		}
	}
	return enqueued, nil
}

// dueReminder pairs one DUE-ISH reminder with its approved chain expiry.
type dueReminder struct {
	reminder ReminderEligibility
	expiry   time.Time
}

// ListSchedulesForReminders reads every schedule for projection enumeration.
func (r *Repository) ListSchedulesForReminders(ctx context.Context) ([]Schedule, error) {
	rows, err := r.querier().Query(ctx, listSchedulesForRemindersQuery)
	if err != nil {
		return nil, fmt.Errorf("list reminder schedules: %w", err)
	}
	defer rows.Close()
	var schedules []Schedule
	for rows.Next() {
		schedule, err := scanSchedule(rows)
		if err != nil {
			return nil, fmt.Errorf("scan reminder schedule: %w", err)
		}
		schedules = append(schedules, schedule)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list reminder schedules: %w", err)
	}
	return schedules, nil
}

// listCancelledReminderExceptions returns the cancelled (schedule, date) keys
// inside the candidate window, batched over all schedules.
func (r *Repository) listCancelledReminderExceptions(ctx context.Context, scheduleIDs []string, fromDate, toDate time.Time) (map[string]bool, error) {
	rows, err := r.querier().Query(ctx, listCancelledReminderExceptionsQuery, scheduleIDs, fromDate, toDate)
	if err != nil {
		return nil, fmt.Errorf("list reminder cancellations: %w", err)
	}
	defer rows.Close()
	cancelled := make(map[string]bool)
	for rows.Next() {
		var scheduleID string
		var date time.Time
		if err := rows.Scan(&scheduleID, &date); err != nil {
			return nil, fmt.Errorf("scan reminder cancellation: %w", err)
		}
		cancelled[reminderExceptionKey(scheduleID, date)] = true
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list reminder cancellations: %w", err)
	}
	return cancelled, nil
}

// listReminderRecipients returns the circle's active member ids.
func (r *Repository) listReminderRecipients(ctx context.Context, circleID string) ([]uuid.UUID, error) {
	rows, err := r.querier().Query(ctx, listReminderRecipientsQuery, circleID)
	if err != nil {
		return nil, fmt.Errorf("list reminder recipients: %w", err)
	}
	defer rows.Close()
	var recipients []uuid.UUID
	for rows.Next() {
		var recipient uuid.UUID
		if err := rows.Scan(&recipient); err != nil {
			return nil, fmt.Errorf("scan reminder recipient: %w", err)
		}
		recipients = append(recipients, recipient)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list reminder recipients: %w", err)
	}
	return recipients, nil
}

// createReminderNotifications persists one intent per (due reminder,
// recipient) inside one notification transaction, replaying NotifyAt as the
// history created_at and the approved expiry chain as push_expires_at.
func (r *Repository) createReminderNotifications(ctx context.Context, circleID string, due []dueReminder, recipients []uuid.UUID) (int, error) {
	circleUUID, err := uuid.Parse(circleID)
	if err != nil {
		return 0, fmt.Errorf("parse reminder circle %s: %w", circleID, err)
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return 0, fmt.Errorf("begin reminder notification tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	inserted := 0
	for _, item := range due {
		reminder, expiry := item.reminder, item.expiry
		for _, recipient := range recipients {
			notifyAt := reminder.NotifyAt
			if _, ins, err := notifications.CreateInTx(ctx, tx, notifications.CreateInput{
				RecipientID:   recipient,
				Type:          notifications.EventSessionReminder,
				SourceEventID: ReminderSourceEventID(reminder.ScheduleID, reminder.OriginalLocalDate, reminder.Version, reminder.Offset),
				SourceRef:     notifications.SourceRef{Type: notifications.SourceCircle, ID: circleUUID},
				Title:         sessionReminderTitle,
				Body:          sessionReminderBody,
				CreatedAt:     &notifyAt,
				PushExpiresAt: expiry,
			}); err != nil {
				return inserted, fmt.Errorf("create reminder notification: %w", err)
			} else if ins {
				inserted++
			}
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return 0, fmt.Errorf("commit reminder notifications: %w", err)
	}
	return inserted, nil
}

// ReminderEligibilityRecheck builds the dispatcher's optional
// EligibilityRecheck seam for F-006 session reminders: it parses the
// reminder's stable source event identity and reanswers current occurrence
// eligibility against the database. A malformed identity can never become
// eligible, so a parse failure is terminal like expiry (it suppresses the
// intent and logs a warning) rather than a transient dispatch-cycle abort;
// only store/read errors remain transient to the caller.
func ReminderEligibilityRecheck(repo *Repository) func(ctx context.Context, intent notifications.DispatchIntent) (bool, error) {
	return func(ctx context.Context, intent notifications.DispatchIntent) (bool, error) {
		scheduleID, originalLocalDate, version, offset, err := ParseReminderSourceEventID(intent.SourceEventID)
		if err != nil {
			slog.WarnContext(ctx, "malformed reminder source identity; suppressing terminally",
				"notification_id", intent.ID, "error", err)
			return false, nil
		}
		return repo.RecheckReminder(ctx, scheduleID, originalLocalDate, version, offset)
	}
}

// RecheckReminder reanswers the dispatch-time eligibility of one stored
// reminder against current database state: the schedule's stop boundary, its
// retained revisions and the occurrence's cancellation timestamp, fed into
// ReminderStillEligible (which reuses EligibleReminders on top). A missing
// schedule is ineligible, not an error, so a deleted circle cannot keep stale
// reminders alive.
func (r *Repository) RecheckReminder(ctx context.Context, scheduleID string, originalLocalDate time.Time, version int, offset time.Duration) (bool, error) {
	date := civilOf(originalLocalDate)
	schedule, err := r.GetSchedule(ctx, scheduleID)
	if errors.Is(err, ErrScheduleNotFound) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("recheck reminder schedule: %w", err)
	}
	var cancelledAt *time.Time
	err = r.querier().QueryRow(ctx, reminderCancellationQuery, scheduleID, date).Scan(&cancelledAt)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return false, fmt.Errorf("recheck reminder cancellation: %w", err)
	}
	revisionsByID, err := r.LoadRevisions(ctx, []string{scheduleID})
	if err != nil {
		return false, fmt.Errorf("recheck reminder revisions: %w", err)
	}
	return ReminderStillEligible(StoredReminderOccurrence{
		Revisions:            revisionsByID[scheduleID],
		ScheduleID:           scheduleID,
		OriginalLocalDate:    date,
		Version:              version,
		Offset:               offset,
		Cancelled:            cancelledAt != nil,
		StoppedFromLocalDate: schedule.StoppedFromLocalDate,
	}), nil
}
