package initiative

import (
	"encoding/json"
	"testing"
	"time"
)

func instant(t *testing.T, raw string) time.Time {
	t.Helper()
	at, err := time.Parse(time.RFC3339, raw)
	if err != nil {
		t.Fatal(err)
	}
	return at
}

func daily(zone string, hour, minute int) Schedule {
	return Schedule{ID: "schedule", SourceID: "project", TimeZone: zone, Kind: "daily", Hour: hour, Minute: minute, MissedPolicy: "coalesce", Enabled: true}
}

func TestScheduleInitiativeWireFormatRetainsOriginalKinds(t *testing.T) {
	s := daily("UTC", 9, 0)
	s.Kind = "once"
	if s.Validate() == nil {
		t.Fatal("agent one-shot kind widened owner initiative API")
	}
	raw, err := json.Marshal(daily("UTC", 9, 0))
	if err != nil {
		t.Fatal(err)
	}
	var fields map[string]json.RawMessage
	if json.Unmarshal(raw, &fields) != nil {
		t.Fatal("schedule wire format")
	}
	for _, k := range []string{"at", "weekdays"} {
		if _, exists := fields[k]; exists {
			t.Fatal("owner schedule fingerprint changed", k)
		}
	}
}

func TestCalendarHandlesMissingAndRepeatedLocalMinutes(t *testing.T) {
	cases := []struct {
		name, zone, after, want string
		hour, minute            int
	}{
		{"ordinary", "UTC", "2026-10-10T08:59:59Z", "2026-10-10T09:00:00Z", 9, 0},
		{"spring_gap", "Europe/Berlin", "2026-03-28T23:00:00Z", "2026-03-30T00:30:00Z", 2, 30},
		{"autumn_first_occurrence", "Europe/Berlin", "2026-10-24T23:00:00Z", "2026-10-25T00:30:00Z", 2, 30},
		{"autumn_second_is_not_another_tick", "Europe/Berlin", "2026-10-25T00:45:00Z", "2026-10-26T01:30:00Z", 2, 30},
		{"half_hour_spring_gap", "Australia/Lord_Howe", "2026-10-03T12:00:00Z", "2026-10-04T15:15:00Z", 2, 15},
		{"new_york_spring_gap", "America/New_York", "2026-03-08T05:00:00Z", "2026-03-09T06:30:00Z", 2, 30},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := daily(tc.zone, tc.hour, tc.minute).NextAfter(instant(t, tc.after))
			if !got.Equal(instant(t, tc.want)) {
				t.Fatalf("next=%s want=%s", got, tc.want)
			}
		})
	}
}

func TestDowntimeCoalescesOnceAndKeepsIntervalPhase(t *testing.T) {
	s := Schedule{ID: "hourly", SourceID: "project", TimeZone: "UTC", Kind: "interval", IntervalSeconds: 3600, MissedPolicy: "coalesce", NextAt: instant(t, "2026-10-09T12:00:00Z"), Enabled: true}
	// Durable JSON round-trip represents an executor restart.
	encoded, _ := json.Marshal(s)
	var restored Schedule
	if json.Unmarshal(encoded, &restored) != nil {
		t.Fatal("saved schedule failed")
	}
	now := instant(t, "2026-10-10T12:17:00Z")
	tick, ok := restored.Due(now)
	if !ok || !tick.ShouldObserve || tick.MissedCount != 24 || !tick.ScheduledAt.Equal(instant(t, "2026-10-10T12:00:00Z")) || !tick.NextAt.Equal(instant(t, "2026-10-10T13:00:00Z")) {
		t.Fatalf("downtime tick=%+v, due=%v", tick, ok)
	}
	restored.NextAt = tick.NextAt
	if _, ok := restored.Due(now); ok {
		t.Fatal("restart queued a historical avalanche")
	}
	s.MissedPolicy = "skip"
	if skipped, ok := s.Due(now); !ok || skipped.ShouldObserve {
		t.Fatal("missed skip policy requested observation")
	}
}

func TestAutumnDowntimeDoesNotReplaySecondWallClockOccurrence(t *testing.T) {
	s := daily("Europe/Berlin", 2, 30)
	s.NextAt = instant(t, "2026-10-25T00:30:00Z")
	tick, ok := s.Due(instant(t, "2026-10-25T01:45:00Z"))
	if !ok || tick.SlotKey != "2026-10-25" || !tick.ScheduledAt.Equal(s.NextAt) || !tick.NextAt.Equal(instant(t, "2026-10-26T01:30:00Z")) {
		t.Fatalf("ambiguous wall minute=%+v", tick)
	}
}

func TestInvalidTimezoneAndHostLocalNeverSchedule(t *testing.T) {
	for _, zone := range []string{"", "Local", "not/a/timezone"} {
		s := daily(zone, 2, 30)
		if s.Validate() == nil || !s.NextAfter(time.Now()).IsZero() {
			t.Fatalf("accepted ambiguous timezone %q", zone)
		}
		if _, ok := s.Due(time.Now()); ok {
			t.Fatal("invalid schedule is due")
		}
	}
}
