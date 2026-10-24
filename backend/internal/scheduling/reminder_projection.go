package scheduling

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
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
	if err != nil || version <= 0 {
		return "", time.Time{}, 0, 0, fmt.Errorf("reminder source_event_id %q version: %w", sourceEventID, err)
	}
	minutes, err := strconv.Atoi(parts[4])
	if err != nil || minutes <= 0 {
		return "", time.Time{}, 0, 0, fmt.Errorf("reminder source_event_id %q offset: %w", sourceEventID, err)
	}
	return scheduleID, date, version, time.Duration(minutes) * time.Minute, nil
}

// ReminderStillEligible reanswers F-006's authoritative boundary for one
// stored reminder identity at dispatch time. It reuses EligibleReminders —
// no eligibility rule is duplicated — and additionally applies the schedule's
// stop boundary, which the derivation-only boundary does not know. Cancelled
// exceptions, superseded versions, stopped dates and no-longer-generated
// occurrences are ineligible (spec Source Feature Ownership, F-006 row).
func ReminderStillEligible(revisions []RevisionRecord, cancelled bool, stoppedFrom *time.Time, scheduleID string, originalLocalDate time.Time, version int, offset time.Duration) bool {
	date := civilOf(originalLocalDate)
	if dateIsStopped(date, stoppedFrom) {
		return false
	}
	eligible, err := EligibleReminders(revisions, Occurrence{ScheduleID: scheduleID, Version: version, OriginalLocalDate: date}, cancelled, []time.Duration{offset})
	return err == nil && len(eligible) == 1
}
