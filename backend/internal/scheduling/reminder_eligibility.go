package scheduling

import "time"

// ReminderEligibility is the stable occurrence and timing data F-008 can use
// to decide whether to deliver a configured reminder.
type ReminderEligibility struct {
	ScheduleID        string
	OriginalLocalDate time.Time
	Version           int
	StartsAt          time.Time
	Offset            time.Duration
	NotifyAt          time.Time
}

// EligibleReminders derives reminder times for a current, uncancelled
// occurrence. Delivery, preferences, and reminder history belong to F-008.
func EligibleReminders(revisions []RevisionRecord, occurrence Occurrence, cancelled bool, offsets []time.Duration) ([]ReminderEligibility, error) {
	if cancelled {
		return nil, nil
	}
	date := civilOf(occurrence.OriginalLocalDate)
	revision, ok := reminderRevision(revisions, occurrence.ScheduleID, occurrence.Version, date)
	if !ok {
		return nil, nil
	}
	zone, err := LoadLocation(revision.Timezone)
	if err != nil {
		return nil, err
	}
	startsAt, _, err := ResolveIntervalUTC(zone, date, revision.StartLocalTime, revision.DurationMinutes)
	if err != nil {
		return nil, err
	}

	result := make([]ReminderEligibility, 0, len(offsets))
	for _, offset := range offsets {
		if offset <= 0 {
			continue
		}
		result = append(result, ReminderEligibility{
			ScheduleID:        occurrence.ScheduleID,
			OriginalLocalDate: date,
			Version:           occurrence.Version,
			StartsAt:          startsAt,
			Offset:            offset,
			NotifyAt:          startsAt.Add(-offset),
		})
	}
	return result, nil
}

func reminderRevision(revisions []RevisionRecord, scheduleID string, version int, date time.Time) (RevisionRecord, bool) {
	var selectedRecords []RevisionRecord
	for _, revision := range revisions {
		if revision.ScheduleID == scheduleID {
			selectedRecords = append(selectedRecords, revision)
		}
	}
	values := revisionValues(selectedRecords)
	current, ok := SelectRevision(values, date)
	if !ok || current.Version != version || len(current.OccurrenceDates(date, date)) == 0 {
		return RevisionRecord{}, false
	}
	return revisionFor(selectedRecords, version)
}
