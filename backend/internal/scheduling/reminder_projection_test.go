package scheduling

import (
	"testing"
	"time"
)

// reminderTestRevisions builds one interval revision generating a daily
// occurrence at the given local clock in UTC.
func reminderTestRevisions(scheduleID string, anchor time.Time, start LocalClock) []RevisionRecord {
	record := RevisionRecord{
		Revision: Revision{
			ScheduleID:         scheduleID,
			Version:            1,
			EffectiveLocalDate: anchor,
			AnchorLocalDate:    anchor,
			Mode:               ModeInterval,
			IntervalCount:      1,
			IntervalUnit:       IntervalUnitDay,
		},
		Title:           "Circle Session",
		StartLocalTime:  start,
		EndLocalTime:    LocalClock{Hour: start.Hour + 1, Minute: start.Minute},
		DurationMinutes: 60,
		Timezone:        "UTC",
	}
	if record.EndLocalTime.Hour > 23 {
		record.EndLocalTime = LocalClock{Hour: 23, Minute: 59}
		record.DurationMinutes = 60
	}
	return []RevisionRecord{record}
}

// The approved expiry chain: reminder k expires when the next closer offset
// becomes due; the final reminder expires at occurrence start.
func TestReminderExpiriesFollowNextCloserOffset(t *testing.T) {
	startsAt := time.Date(2026, 10, 24, 12, 0, 0, 0, time.UTC)
	date := civilOf(startsAt)
	reminders, err := EligibleReminders(reminderTestRevisions("sched-1", date, LocalClock{Hour: 12}),
		Occurrence{ScheduleID: "sched-1", Version: 1, OriginalLocalDate: date}, false, ReminderOffsets())
	if err != nil {
		t.Fatal(err)
	}
	if len(reminders) != 4 {
		t.Fatalf("eligible reminders=%d, want 4", len(reminders))
	}
	expiries := ReminderExpiries(reminders)
	want := []time.Time{
		startsAt.Add(-30 * time.Minute),
		startsAt.Add(-15 * time.Minute),
		startsAt.Add(-5 * time.Minute),
		startsAt,
	}
	for i, expiry := range want {
		if !expiries[i].Equal(expiry) {
			t.Errorf("reminder %d expiry=%s, want %s (next closer offset or occurrence start)", i, expiries[i], expiry)
		}
	}
}

// A single-offset evaluation (dispatch recheck path) must report the
// occurrence start as its own expiry.
func TestReminderExpiriesSingletonUsesOccurrenceStart(t *testing.T) {
	startsAt := time.Date(2026, 10, 24, 12, 0, 0, 0, time.UTC)
	reminders := []ReminderEligibility{{
		ScheduleID: "sched-1", Version: 1, StartsAt: startsAt,
		Offset: 5 * time.Minute, NotifyAt: startsAt.Add(-5 * time.Minute),
	}}
	expiries := ReminderExpiries(reminders)
	if len(expiries) != 1 || !expiries[0].Equal(startsAt) {
		t.Fatalf("expiries=%v, want the occurrence start %s", expiries, startsAt)
	}
}

func TestReminderOffsetsMatchApprovedMatrix(t *testing.T) {
	offsets := ReminderOffsets()
	want := []time.Duration{time.Hour, 30 * time.Minute, 15 * time.Minute, 5 * time.Minute}
	if len(offsets) != len(want) {
		t.Fatalf("offsets=%v, want %v", offsets, want)
	}
	for i, offset := range want {
		if offsets[i] != offset {
			t.Errorf("offset %d=%s, want %s", i, offsets[i], offset)
		}
	}
}

// DUE-ISH creation rule: NotifyAt may be slightly early (bounded by the lead)
// and late ticks are allowed while the expiry has not passed, but a reminder
// past its chain expiry is never created.
func TestReminderDueRequiresNotExpired(t *testing.T) {
	startsAt := time.Date(2026, 10, 24, 12, 0, 0, 0, time.UTC)
	reminder := ReminderEligibility{Offset: time.Hour, NotifyAt: startsAt.Add(-time.Hour), StartsAt: startsAt}
	const lead = 2 * time.Minute

	t.Run("before notify and lead", func(t *testing.T) {
		now := startsAt.Add(-time.Hour).Add(-lead).Add(-time.Minute)
		if ReminderDue(now, reminder, startsAt.Add(-30*time.Minute), lead) {
			t.Fatal("reminder created more than a lead before NotifyAt")
		}
	})
	t.Run("when NotifyAt passed and expiry still future", func(t *testing.T) {
		now := startsAt.Add(-time.Hour).Add(time.Minute) // late tick after downtime
		if !ReminderDue(now, reminder, startsAt.Add(-30*time.Minute), lead) {
			t.Fatal("late tick within expiry window must still enqueue")
		}
	})
	t.Run("past expiry", func(t *testing.T) {
		now := startsAt.Add(-30 * time.Minute)
		if ReminderDue(now, reminder, startsAt.Add(-30*time.Minute), lead) {
			t.Fatal("reminder whose expiry has passed must never be created")
		}
	})
	t.Run("within lead before NotifyAt", func(t *testing.T) {
		now := startsAt.Add(-time.Hour).Add(-lead).Add(time.Second)
		if !ReminderDue(now, reminder, startsAt.Add(-30*time.Minute), lead) {
			t.Fatal("dying tick just inside the lead should enqueue the reminder")
		}
	})
}

// The stable, self-describing reminder identity: the builder, parser, and the
// exact approved format.
func TestReminderSourceEventIDRoundTrip(t *testing.T) {
	buildDate := time.Date(2026, 10, 24, 0, 0, 0, 0, time.UTC)
	id := ReminderSourceEventID("1b290a97-8f20-4376-a0e2-d7ca4f05b29a", buildDate, 3, time.Hour)
	if id != "session.reminder:1b290a97-8f20-4376-a0e2-d7ca4f05b29a:2026-10-24:3:60" {
		t.Fatalf("source_event_id=%q, want the documented format", id)
	}
	scheduleID, parsedDate, version, offset, err := ParseReminderSourceEventID(id)
	if err != nil {
		t.Fatal(err)
	}
	if scheduleID != "1b290a97-8f20-4376-a0e2-d7ca4f05b29a" || !parsedDate.Equal(buildDate) ||
		version != 3 || offset != time.Hour {
		t.Fatalf("parsed=(%s, %s, %d, %s), want the built identity", scheduleID, parsedDate, version, offset)
	}
	// 30 minutes composes as exactly 30.
	if _, _, _, offset, err := ParseReminderSourceEventID(
		ReminderSourceEventID("1b290a97-8f20-4376-a0e2-d7ca4f05b29a", time.Date(2026, 10, 24, 0, 0, 0, 0, time.UTC), 1, 30*time.Minute)); err != nil || offset != 30*time.Minute {
		t.Fatalf("30-minute offset parse=(%s, %v)", offset, err)
	}
}

func TestReminderSourceEventIDRejectsMalformedValues(t *testing.T) {
	for name, id := range map[string]string{
		"empty":                  "",
		"wrong prefix":           "session.other:1b290a97-8f20-4376-a0e2-d7ca4f05b29a:2026-10-24:1:60",
		"not a schedule":         "session.reminder:not-a-uuid:2026-10-24:1:60",
		"bad date":               "session.reminder:1b290a97-8f20-4376-a0e2-d7ca4f05b29a:2026-10-2:1:60",
		"bad version":            "session.reminder:1b290a97-8f20-4376-a0e2-d7ca4f05b29a:2026-10-24:x:60",
		"non-positive":           "session.reminder:1b290a97-8f20-4376-a0e2-d7ca4f05b29a:2026-10-24:1:0",
		"missing offset":         "session.reminder:1b290a97-8f20-4376-a0e2-d7ca4f05b29a:2026-10-24:1",
		"date is a full instant": "session.reminder:1b290a97-8f20-4376-a0e2-d7ca4f05b29a:2026-10-24T10:00Z:1:60",
	} {
		_, _, _, _, err := ParseReminderSourceEventID(id)
		if err == nil {
			t.Errorf("%s: malformed source_event_id %q parsed without error", name, id)
		}
	}
}

// The dispatch-time recheck decision: cancelled exceptions, superseded
// versions, stopped dates and no-longer-generated occurrences are ineligible;
// the stored identity still matches the current generation.
func TestReminderStillEligibleReturnsFalseForStaleSources(t *testing.T) {
	anchor := time.Date(2026, 10, 24, 0, 0, 0, 0, time.UTC)
	revisions := reminderTestRevisions("sched-1", anchor, LocalClock{Hour: 12})
	current := time.Date(2026, 10, 25, 0, 0, 0, 0, time.UTC)
	superseding := revisions[0]
	superseding.Version = 2
	superseding.EffectiveLocalDate = current
	stoppedFrom := current // future stop boundary keeps earlier occurrences
	tryReminder := func(revisions []RevisionRecord, cancelled bool, stoppedFrom *time.Time, date time.Time, version int) bool {
		return ReminderStillEligible(StoredReminderOccurrence{
			Revisions:            revisions,
			ScheduleID:           "sched-1",
			OriginalLocalDate:    date,
			Version:              version,
			Offset:               5 * time.Minute,
			Cancelled:            cancelled,
			StoppedFromLocalDate: stoppedFrom,
		})
	}

	for name, tc := range map[string]struct {
		revisions   []RevisionRecord
		cancelled   bool
		stoppedFrom *time.Time
		date        time.Time
		version     int
		want        bool
	}{
		"current occurrence eligible":         {revisions, false, nil, anchor, 1, true},
		"cancelled exception ineligible":      {revisions, true, nil, anchor, 1, false},
		"superseded version ineligible":       {[]RevisionRecord{revisions[0], superseding}, false, nil, current, 1, false},
		"unknown version ineligible":          {revisions, false, nil, anchor, 9, false},
		"stopped date ineligible":             {revisions, false, &stoppedFrom, anchor.AddDate(0, 0, 10), 1, false},
		"before a future stop still eligible": {revisions, false, &stoppedFrom, anchor, 1, true},
		"not generated date ineligible":       {revisions, false, nil, anchor.AddDate(0, 0, -10), 1, false},
	} {
		if got := tryReminder(tc.revisions, tc.cancelled, tc.stoppedFrom, tc.date, tc.version); got != tc.want {
			t.Errorf("%s: eligible=%t, want %t", name, got, tc.want)
		}
	}
}
