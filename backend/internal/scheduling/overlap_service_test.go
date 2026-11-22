package scheduling

import (
	"errors"
	"testing"
	"time"
)

func TestOverlapWarningsUseHalfOpenUTCIntervals(t *testing.T) {
	start := time.Date(2030, 1, 2, 10, 0, 0, 0, time.UTC)
	candidate := []OverlapInterval{{OccurrenceKey: "candidate", CircleID: "circle-a", CircleName: "A", StartsAt: start, EndsAt: start.Add(time.Hour)}}
	existing := []OverlapInterval{
		{OccurrenceKey: "touching", CircleID: "circle-b", CircleName: "B", StartsAt: start.Add(time.Hour), EndsAt: start.Add(2 * time.Hour)},
		{OccurrenceKey: "overlap", CircleID: "circle-b", CircleName: "B", StartsAt: start.Add(30 * time.Minute), EndsAt: start.Add(90 * time.Minute)},
	}
	warnings := evaluateOverlapWarnings(candidate, existing)
	if len(warnings) != 1 {
		t.Fatalf("got %d warnings, want 1: %#v", len(warnings), warnings)
	}
	warning := warnings[0]
	if warning.FirstOccurrenceKey != "candidate" || warning.SecondOccurrenceKey != "overlap" || warning.FirstCircleName != "A" || warning.SecondCircleName != "B" {
		t.Fatalf("unexpected warning projection: %#v", warning)
	}
	if warning.OverlapStartsAt != start.Add(30*time.Minute).Format(time.RFC3339Nano) || warning.OverlapEndsAt != start.Add(time.Hour).Format(time.RFC3339Nano) {
		t.Fatalf("unexpected UTC overlap: %#v", warning)
	}
}

func TestPlanPreviewWindowUsesFirst31LocalDaysAndFiniteSelectedDates(t *testing.T) {
	anchor := time.Date(2030, 1, 1, 0, 0, 0, 0, time.UTC)
	record := RevisionRecord{Revision: Revision{Version: 1, Mode: ModeInterval, AnchorLocalDate: anchor, EffectiveLocalDate: anchor, IntervalCount: 1, IntervalUnit: IntervalUnitDay}, StartLocalTime: LocalClock{Hour: 9}, EndLocalTime: LocalClock{Hour: 10}, DurationMinutes: 60, Timezone: "Asia/Riyadh"}
	window, err := previewWindow(record)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := len(window), 31; got != want {
		t.Fatalf("got %d preview occurrences, want %d", got, want)
	}
	if !window[30].OriginalLocalDate.Equal(anchor.AddDate(0, 0, 30)) {
		t.Fatalf("31st local day = %s, want %s", window[30].OriginalLocalDate, anchor.AddDate(0, 0, 30))
	}
	record.Mode, record.SelectedDates, record.IntervalCount, record.IntervalUnit = ModeSelectedDates, []time.Time{anchor, anchor.AddDate(0, 0, 45)}, 0, ""
	window, err = previewWindow(record)
	if err != nil {
		t.Fatal(err)
	}
	if len(window) != 2 || !window[1].OriginalLocalDate.Equal(anchor.AddDate(0, 0, 45)) {
		t.Fatalf("finite selected-date preview = %#v, want both selected dates", window)
	}
}

func TestPlanPreviewWindowRejectsSelectedDatesBeyondLimits(t *testing.T) {
	anchor := time.Date(2030, 1, 1, 0, 0, 0, 0, time.UTC)
	record := RevisionRecord{Revision: Revision{Version: 1, Mode: ModeSelectedDates, AnchorLocalDate: anchor, EffectiveLocalDate: anchor}, StartLocalTime: LocalClock{Hour: 9}, EndLocalTime: LocalClock{Hour: 10}, DurationMinutes: 60, Timezone: "Asia/Riyadh"}

	overCount := record
	overCount.SelectedDates = make([]time.Time, 367)
	for i := range overCount.SelectedDates {
		overCount.SelectedDates[i] = anchor.AddDate(0, 0, i)
	}
	if _, err := previewWindow(overCount); !errors.Is(err, ErrInvalidSelectedDates) {
		t.Fatalf("367 selected dates preview: got %v, want ErrInvalidSelectedDates", err)
	}

	overSpan := record
	overSpan.SelectedDates = []time.Time{anchor, anchor.AddDate(0, 0, 367)}
	if _, err := previewWindow(overSpan); !errors.Is(err, ErrInvalidSelectedDates) {
		t.Fatalf("367-day span preview: got %v, want ErrInvalidSelectedDates", err)
	}
}

func TestPlanPreviewWindowSelectedDatesStayWithinRequestHorizon(t *testing.T) {
	anchor := time.Date(2030, 1, 1, 0, 0, 0, 0, time.UTC)
	last := anchor.AddDate(0, 0, 366)
	record := RevisionRecord{Revision: Revision{Version: 1, Mode: ModeSelectedDates, AnchorLocalDate: anchor, EffectiveLocalDate: anchor, SelectedDates: []time.Time{anchor, last}}, StartLocalTime: LocalClock{Hour: 9}, EndLocalTime: LocalClock{Hour: 10}, DurationMinutes: 60, Timezone: "Asia/Riyadh"}

	window, err := previewWindow(record)
	if err != nil {
		t.Fatal(err)
	}
	if len(window) != 2 || !window[0].OriginalLocalDate.Equal(anchor) || !window[1].OriginalLocalDate.Equal(last) {
		t.Fatalf("selected-date preview = %#v, want exactly the two selected dates %s..%s", window, anchor, last)
	}
	for _, interval := range window {
		if interval.OriginalLocalDate.After(last) {
			t.Fatalf("preview generated %s beyond the selected-date horizon %s", interval.OriginalLocalDate, last)
		}
	}
}

func TestPlanPreviewWindowStartsAtLateEffectiveDate(t *testing.T) {
	anchor := time.Date(2030, 1, 1, 0, 0, 0, 0, time.UTC)
	effective := anchor.AddDate(0, 2, 0)
	record := RevisionRecord{Revision: Revision{Version: 2, Mode: ModeInterval, AnchorLocalDate: anchor, EffectiveLocalDate: effective, IntervalCount: 1, IntervalUnit: IntervalUnitDay}, StartLocalTime: LocalClock{Hour: 9}, EndLocalTime: LocalClock{Hour: 10}, DurationMinutes: 60, Timezone: "Asia/Riyadh"}

	window, err := previewWindow(record)
	if err != nil {
		t.Fatal(err)
	}
	if len(window) != 31 {
		t.Fatalf("late-effective preview has %d occurrences, want 31", len(window))
	}
	if !window[0].OriginalLocalDate.Equal(effective) {
		t.Fatalf("late-effective preview starts %s, want %s", window[0].OriginalLocalDate, effective)
	}
}

func TestOverlapWarningProjectionContainsOnlyAuthorizedCircleNames(t *testing.T) {
	start := time.Date(2030, 1, 2, 10, 0, 0, 0, time.UTC)
	warnings := evaluateOverlapWarnings(
		[]OverlapInterval{{OccurrenceKey: "own", CircleID: "own-circle", CircleName: "Own", StartsAt: start, EndsAt: start.Add(time.Hour)}},
		[]OverlapInterval{{OccurrenceKey: "authorized", CircleID: "member-circle", CircleName: "Member Circle", StartsAt: start, EndsAt: start.Add(time.Hour)}},
	)
	if len(warnings) != 1 {
		t.Fatalf("got %d warnings, want 1", len(warnings))
	}
	warning := warnings[0]
	if warning.FirstCircleName != "Own" && warning.FirstCircleName != "Member Circle" || warning.SecondCircleName != "Own" && warning.SecondCircleName != "Member Circle" {
		t.Fatalf("unexpected authorized circle names: %#v", warning)
	}
	if warning.FirstOccurrenceKey == "private member" || warning.SecondOccurrenceKey == "private member" {
		t.Fatalf("warning disclosed a private member field: %#v", warning)
	}
}

func TestPreviewWindowIncludesLaterViewedMonthsAndSameCircleCommitments(t *testing.T) {
	anchor := time.Date(2030, 1, 31, 0, 0, 0, 0, time.UTC)
	record := RevisionRecord{Revision: Revision{Version: 1, Mode: ModeInterval, AnchorLocalDate: anchor, EffectiveLocalDate: anchor, IntervalCount: 1, IntervalUnit: IntervalUnitDay}, StartLocalTime: LocalClock{Hour: 9}, EndLocalTime: LocalClock{Hour: 10}, DurationMinutes: 60, Timezone: "UTC"}
	window, err := previewWindow(record)
	if err != nil {
		t.Fatal(err)
	}
	if len(window) != 31 || window[30].OriginalLocalDate.Month() != time.March {
		t.Fatalf("preview should include 31 local dates into a later viewed month: %#v", window)
	}
	window[0].CircleID, window[0].CircleName = "same", "Same circle"
	start := window[0].StartsAt
	warnings := evaluateOverlapWarnings(window[:1], []OverlapInterval{{OccurrenceKey: "existing", CircleID: "same", CircleName: "Same circle", StartsAt: start, EndsAt: start.Add(time.Hour)}})
	if len(warnings) != 1 {
		t.Fatalf("same-circle commitment warning count=%d, want 1", len(warnings))
	}
}

func TestOnlyUncancelledCalendarItemsAreCommitments(t *testing.T) {
	start := time.Date(2030, 1, 2, 10, 0, 0, 0, time.UTC)
	items := []CalendarItem{
		{OccurrenceKey: "scheduled", CircleID: "circle", CircleName: "Circle", StartsAt: start, EndsAt: start.Add(time.Hour), State: "scheduled"},
		{OccurrenceKey: "cancelled", CircleID: "circle", CircleName: "Circle", StartsAt: start, EndsAt: start.Add(time.Hour), State: "cancelled"},
	}
	intervals := eligibleOverlapIntervals(items)
	if len(intervals) != 1 || intervals[0].OccurrenceKey != "scheduled" {
		t.Fatalf("eligible commitments = %#v", intervals)
	}
}

func TestWarningIDIncludesTargetCircleScope(t *testing.T) {
	start := time.Date(2030, 1, 2, 10, 0, 0, 0, time.UTC)
	existing := []OverlapInterval{{OccurrenceKey: "existing", CircleID: "source", CircleName: "Source", StartsAt: start, EndsAt: start.Add(time.Hour)}}
	first := evaluateOverlapWarnings([]OverlapInterval{{OccurrenceKey: "preview:date", CircleID: "target-a", CircleName: "A", StartsAt: start, EndsAt: start.Add(time.Hour)}}, existing)
	second := evaluateOverlapWarnings([]OverlapInterval{{OccurrenceKey: "preview:date", CircleID: "target-b", CircleName: "B", StartsAt: start, EndsAt: start.Add(time.Hour)}}, existing)
	if len(first) != 1 || len(second) != 1 || first[0].WarningID == second[0].WarningID {
		t.Fatalf("target scopes reused warning IDs: first=%#v second=%#v", first, second)
	}
}

func TestReviewedWarningIDsMustMatchCurrentWarnings(t *testing.T) {
	current := []OverlapWarning{{WarningID: "fresh", FirstCircleName: "A", SecondCircleName: "B"}}
	if err := requireOverlapConfirmation(current, false, nil); err == nil {
		t.Fatal("unconfirmed warning should block the write")
	}
	var conflict *OverlapConfirmationError
	if err := requireOverlapConfirmation(current, true, []string{"stale"}); !errors.As(err, &conflict) || len(conflict.Warnings) != 1 || conflict.Warnings[0].WarningID != "fresh" {
		t.Fatalf("stale review should return refreshed warnings, got %v", err)
	}
	if err := requireOverlapConfirmation(current, true, []string{"fresh"}); err != nil {
		t.Fatalf("reviewed current warning should proceed: %v", err)
	}
	if err := requireOverlapConfirmation(nil, false, nil); err != nil {
		t.Fatalf("no-overlap write should proceed: %v", err)
	}
}

func TestScheduleExclusionMatchesSeriesAndSingleOccurrenceScopes(t *testing.T) {
	if !shouldExcludeOverlap("schedule:2030-01-02", "schedule") || !shouldExcludeOverlap("schedule:2030-01-03", "schedule") {
		t.Fatal("series revisions should exclude the existing series")
	}
	if !shouldExcludeOverlap("schedule:2030-01-02", "schedule:2030-01-02") {
		t.Fatal("occurrence edit should exclude its own commitment")
	}
	if shouldExcludeOverlap("schedule:2030-01-03", "schedule:2030-01-02") {
		t.Fatal("occurrence edit must retain sibling commitments")
	}
}
