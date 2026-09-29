package scheduling

import (
	"testing"
	"time"
)

func TestCalendarWarningsCompareOnlyTheViewersEligibleItems(t *testing.T) {
	start := time.Date(2030, 3, 4, 10, 0, 0, 0, time.UTC)
	items := []CalendarItem{
		{OccurrenceKey: "student-own", CircleID: "a", CircleName: "A", StartsAt: start, EndsAt: start.Add(time.Hour), State: "scheduled"},
		{OccurrenceKey: "student-cross", CircleID: "b", CircleName: "B", StartsAt: start.Add(30 * time.Minute), EndsAt: start.Add(90 * time.Minute), State: "scheduled"},
		{OccurrenceKey: "cancelled", CircleID: "c", CircleName: "C", StartsAt: start, EndsAt: start.Add(time.Hour), State: "cancelled"},
	}
	warnings := calendarWarnings(items)
	if len(warnings) != 1 || warnings[0].FirstCircleName == warnings[0].SecondCircleName {
		t.Fatalf("viewer calendar warnings = %#v, want the eligible cross-circle conflict", warnings)
	}
	if warnings[0].WarningID == "" || warnings[0].OverlapStartsAt != start.Add(30*time.Minute).Format(time.RFC3339Nano) {
		t.Fatalf("warning not actionable: %#v", warnings[0])
	}
	if len(items) != 3 || items[0].State != "scheduled" {
		t.Fatalf("warnings must not alter calendar participation or item state: %#v", items)
	}
}
