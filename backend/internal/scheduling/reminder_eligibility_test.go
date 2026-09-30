package scheduling

import (
	"testing"
	"time"
)

func TestEligibleReminders_OffsetsAndCancelledOrSupersededOccurrences(t *testing.T) {
	date := time.Date(2026, time.October, 5, 0, 0, 0, 0, time.UTC)
	first := RevisionRecord{Revision: Revision{ScheduleID: "schedule-1", Version: 1, EffectiveLocalDate: date, AnchorLocalDate: date, Mode: ModeInterval, IntervalCount: 1, IntervalUnit: IntervalUnitDay}, StartLocalTime: LocalClock{Hour: 12}, DurationMinutes: 60, Timezone: "UTC"}
	second := first
	second.Version = 2
	second.EffectiveLocalDate = date.AddDate(0, 0, 2)
	otherSchedule := first
	otherSchedule.ScheduleID = "schedule-2"
	otherSchedule.Version = 3
	startsAt := time.Date(2026, time.October, 5, 12, 0, 0, 0, time.UTC)
	offsets := []time.Duration{time.Hour, 30 * time.Minute, 15 * time.Minute, 5 * time.Minute}
	got, err := EligibleReminders([]RevisionRecord{first, second, otherSchedule}, Occurrence{ScheduleID: first.ScheduleID, Version: 1, OriginalLocalDate: date}, false, offsets)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != len(offsets) {
		t.Fatalf("got %d reminder times, want %d", len(got), len(offsets))
	}
	for i, offset := range offsets {
		if got[i].Offset != offset || !got[i].NotifyAt.Equal(startsAt.Add(-offset)) {
			t.Errorf("reminder %d = (%s, %s), want (%s, %s)", i, got[i].Offset, got[i].NotifyAt, offset, startsAt.Add(-offset))
		}
		if got[i].ScheduleID != first.ScheduleID || got[i].Version != first.Version || !got[i].OriginalLocalDate.Equal(date) || !got[i].StartsAt.Equal(startsAt) {
			t.Errorf("reminder %d lost stable occurrence or start identity: %+v", i, got[i])
		}
	}

	for name, scenario := range map[string]struct {
		occurrence Occurrence
		cancelled  bool
	}{
		"cancelled":  {Occurrence{ScheduleID: first.ScheduleID, Version: 1, OriginalLocalDate: date}, true},
		"superseded": {Occurrence{ScheduleID: first.ScheduleID, Version: 1, OriginalLocalDate: date.AddDate(0, 0, 2)}, false},
	} {
		t.Run(name, func(t *testing.T) {
			reminders, err := EligibleReminders([]RevisionRecord{first, second}, scenario.occurrence, scenario.cancelled, offsets)
			if err != nil {
				t.Fatal(err)
			}
			if len(reminders) != 0 {
				t.Fatalf("got reminders for ineligible occurrence: %+v", reminders)
			}
		})
	}
}
