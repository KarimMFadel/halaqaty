package attendance

import (
	"testing"
	"time"
)

func TestClassifyEndedSession(t *testing.T) {
	start := time.Date(2026, 9, 29, 10, 0, 0, 0, time.UTC)
	exactlyTenMinutes := start.Add(10 * time.Minute)
	afterTenMinutes := exactlyTenMinutes.Add(time.Nanosecond)

	tests := []struct {
		name      string
		input     FinalizationInput
		want      map[string]Status
		wantCount int
	}{
		{
			name: "inclusive threshold, later first presence, and no minimum duration",
			input: FinalizationInput{ActualStart: &start, Ended: true, Roster: []RosterEntry{
				{UserID: "on-time", FirstPresenceAt: &exactlyTenMinutes},
				{UserID: "late", FirstPresenceAt: &afterTenMinutes},
			}},
			want: map[string]Status{"on-time": StatusPresent, "late": StatusLate}, wantCount: 2,
		},
		{
			name: "reconnects and devices use the first authorized presence",
			input: FinalizationInput{ActualStart: &start, Ended: true, Roster: []RosterEntry{
				{UserID: "reconnected", FirstPresenceAt: &exactlyTenMinutes},
				{UserID: "different-device", FirstPresenceAt: &afterTenMinutes},
			}},
			want: map[string]Status{"reconnected": StatusPresent, "different-device": StatusLate}, wantCount: 2,
		},
		{
			name:  "eligible absent students are absent while excused remains manual",
			input: FinalizationInput{ActualStart: &start, Ended: true, Roster: []RosterEntry{{UserID: "absent"}}},
			want:  map[string]Status{"absent": StatusAbsent}, wantCount: 1,
		},
		{
			name:  "later-enrolled eligible participant is classified",
			input: FinalizationInput{ActualStart: &start, Ended: true, Roster: []RosterEntry{{UserID: "later", FirstPresenceAt: &afterTenMinutes, Source: RosterSourceLaterParticipant}}},
			want:  map[string]Status{"later": StatusLate}, wantCount: 1,
		},
		{name: "cancelled session has no attendance", input: FinalizationInput{ActualStart: &start, Ended: true, Cancelled: true, Roster: []RosterEntry{{UserID: "student"}}}, wantCount: 0},
		{name: "never-started session has no attendance", input: FinalizationInput{Ended: true, Roster: []RosterEntry{{UserID: "student"}}}, wantCount: 0},
		{name: "not-yet-ended session is not finalized", input: FinalizationInput{ActualStart: &start, Roster: []RosterEntry{{UserID: "student"}}}, wantCount: 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ClassifyEndedSession(tt.input)
			if len(got) != tt.wantCount {
				t.Fatalf("ClassifyEndedSession() returned %d records, want %d: %#v", len(got), tt.wantCount, got)
			}
			for userID, want := range tt.want {
				if gotStatus, ok := got[userID]; !ok || gotStatus != want {
					t.Errorf("ClassifyEndedSession()[%q] = %q, %t; want %q, true", userID, gotStatus, ok, want)
				}
			}
		})
	}

	for userID, status := range ClassifyEndedSession(FinalizationInput{ActualStart: &start, Ended: true, Roster: []RosterEntry{{UserID: "student"}}}) {
		if status == Status("excused") {
			t.Fatalf("excused must remain a manual-only status for %q", userID)
		}
	}
}
