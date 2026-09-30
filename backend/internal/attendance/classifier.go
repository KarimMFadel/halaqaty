// Package attendance derives attendance status from the durable session roster and presence facts.
package attendance

import "time"

// Status is an effective attendance classification.
type Status string

const (
	StatusPresent Status = "present"
	StatusLate    Status = "late"
	StatusAbsent  Status = "absent"
	StatusExcused Status = "excused"
)

// RosterEntry is an already-authorized attendee retained in the session roster.
type RosterEntry struct {
	UserID          string
	FirstPresenceAt *time.Time
	Source          RosterSource
}

// RosterSource records why the attendee was included in the durable roster.
type RosterSource string

const (
	RosterSourceStartSnapshot    RosterSource = "start_snapshot"
	RosterSourceLaterParticipant RosterSource = "later_participant"
)

// FinalizationInput contains immutable session facts needed to classify its roster.
type FinalizationInput struct {
	ActualStart *time.Time
	Ended       bool
	Cancelled   bool
	Roster      []RosterEntry
}

// ClassifyEndedSession returns statuses only for a started, ended, non-cancelled session.
// The roster is expected to contain only durable eligible participants.
func ClassifyEndedSession(input FinalizationInput) map[string]Status {
	if input.ActualStart == nil || !input.Ended || input.Cancelled {
		return nil
	}

	statuses := make(map[string]Status, len(input.Roster))
	cutoff := input.ActualStart.Add(10 * time.Minute)
	for _, attendee := range input.Roster {
		status := StatusAbsent
		if attendee.FirstPresenceAt != nil {
			status = StatusLate
			if !attendee.FirstPresenceAt.After(cutoff) {
				status = StatusPresent
			}
		}
		statuses[attendee.UserID] = status
	}
	return statuses
}
