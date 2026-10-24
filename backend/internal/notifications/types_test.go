package notifications_test

import (
	"testing"
	"time"

	"github.com/KarimMFadel/halaqaty/backend/internal/notifications"
)

func TestEventCatalogCoversActiveMatrix(t *testing.T) {
	want := []notifications.EventType{
		notifications.EventSessionReminder,
		notifications.EventGroupMessage,
		notifications.EventPrivateMessage,
		notifications.EventGradePosted,
		notifications.EventCircleInvitation,
		notifications.EventQueueNextSoon,
		notifications.EventQueueYourTurn,
		notifications.EventStudentJoinedSession,
		notifications.EventStudentJoinedCircle,
		notifications.EventCircleArchived,
		notifications.EventSessionStarted,
		notifications.EventSessionEnded,
	}
	got := notifications.EventPreferences()
	if len(got) != len(want) {
		t.Fatalf("catalog has %d types, want %d", len(got), len(want))
	}
	for i, expected := range want {
		if got[i].Type != expected {
			t.Errorf("catalog[%d]=%q, want %q", i, got[i].Type, expected)
		}
		if !got[i].DefaultEnabled {
			t.Errorf("%q default_enabled=false, want true", expected)
		}
	}
}

func TestAlwaysOnTypesCannotBeConfiguredOff(t *testing.T) {
	for _, preference := range notifications.EventPreferences() {
		if preference.Configurable {
			continue
		}
		if preference.Type != notifications.EventPrivateMessage &&
			preference.Type != notifications.EventCircleInvitation &&
			preference.Type != notifications.EventQueueNextSoon &&
			preference.Type != notifications.EventQueueYourTurn {
			t.Errorf("unexpected always-on type %q", preference.Type)
		}
	}
}

func TestEventPreferencesReturnsCopy(t *testing.T) {
	first := notifications.EventPreferences()
	first[0].Configurable = !first[0].Configurable
	second := notifications.EventPreferences()
	if first[0].Configurable == second[0].Configurable {
		t.Fatal("caller mutation changed the event catalog")
	}
}

func TestIsQuietHoursUsesProfileLocalClockAndHalfOpenInterval(t *testing.T) {
	quiet := notifications.QuietHours{Enabled: true, StartLocal: "22:00", EndLocal: "07:00", Timezone: "Asia/Riyadh"}
	for _, tc := range []struct {
		name string
		now  string
		want bool
	}{
		{name: "start inclusive", now: "2026-10-19T19:00:00Z", want: true},
		{name: "overnight", now: "2026-10-19T02:00:00Z", want: true},
		{name: "end exclusive", now: "2026-10-19T04:00:00Z", want: false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			now, err := time.Parse(time.RFC3339, tc.now)
			if err != nil {
				t.Fatal(err)
			}
			got, err := notifications.IsQuietHours(quiet, now)
			if err != nil {
				t.Fatal(err)
			}
			if got != tc.want {
				t.Fatalf("IsQuietHours(%s)=%t, want %t", tc.now, got, tc.want)
			}
		})
	}
}

func TestIsQuietHoursAcrossDSTTransitions(t *testing.T) {
	quiet := notifications.QuietHours{Enabled: true, StartLocal: "22:00", EndLocal: "07:00", Timezone: "America/New_York"}
	for _, tc := range []struct {
		name string
		now  string
		want bool
	}{
		// Same UTC clock time on consecutive days, resolved through the zone:
		// only the DST offset shift decides membership in the window.
		{name: "spring forward eve 11:30 UTC is 06:30 EST", now: "2026-03-07T11:30:00Z", want: true},
		{name: "spring forward day 11:30 UTC is 07:30 EDT", now: "2026-03-08T11:30:00Z", want: false},
		{name: "spring forward pre-dawn 06:30 UTC is 01:30 EST", now: "2026-03-08T06:30:00Z", want: true},
		{name: "fall back eve 11:30 UTC is 07:30 EDT", now: "2026-10-31T11:30:00Z", want: false},
		{name: "fall back day 11:30 UTC is 06:30 EST", now: "2026-11-01T11:30:00Z", want: true},
		// The repeated fall-back hour stays inside the window on both passes.
		{name: "fall back repeated hour first pass (EDT)", now: "2026-11-01T05:30:00Z", want: true},
		{name: "fall back repeated hour second pass (EST)", now: "2026-11-01T06:30:00Z", want: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			now, err := time.Parse(time.RFC3339, tc.now)
			if err != nil {
				t.Fatal(err)
			}
			got, err := notifications.IsQuietHours(quiet, now)
			if err != nil {
				t.Fatal(err)
			}
			if got != tc.want {
				t.Fatalf("IsQuietHours(%s)=%t, want %t", tc.now, got, tc.want)
			}
		})
	}
}

func TestIsQuietHoursValidatesEnabledIntervalsAndTimezone(t *testing.T) {
	now := time.Date(2026, time.October, 19, 12, 0, 0, 0, time.UTC)
	if got, err := notifications.IsQuietHours(notifications.QuietHours{Enabled: false}, now); err != nil || got {
		t.Fatalf("disabled quiet hours = %t, %v; want false, nil", got, err)
	}
	for _, quiet := range []notifications.QuietHours{
		{Enabled: true, StartLocal: "22:00", EndLocal: "22:00", Timezone: "UTC"},
		{Enabled: true, StartLocal: "22:00", EndLocal: "07:00", Timezone: "Not/AZone"},
		{Enabled: true, StartLocal: "2:00", EndLocal: "07:00", Timezone: "UTC"},
	} {
		if _, err := notifications.IsQuietHours(quiet, now); err == nil {
			t.Fatalf("IsQuietHours(%#v) succeeded, want validation error", quiet)
		}
	}
}
