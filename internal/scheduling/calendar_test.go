package scheduling

import (
	"testing"
	"time"
)

func at(raw string) time.Time {
	v, e := time.Parse(time.RFC3339, raw)
	if e != nil {
		panic(e)
	}
	return v
}
func TestOneShotAndSelectedWeekdays(t *testing.T) {
	s := Schedule{ID: "q", SourceID: "s", Kind: "once", TimeZone: "Asia/Tomsk", At: at("2026-10-19T09:00:00+07:00"), Enabled: true, MissedPolicy: "coalesce", NextAt: at("2026-10-19T02:00:00Z")}
	tick, ok := s.Due(at("2026-10-19T02:01:00Z"))
	if !ok || !tick.ShouldObserve || !tick.NextAt.IsZero() {
		t.Fatalf("one-shot must finish: %+v", tick)
	}
	s.Kind = "daily"
	s.At = time.Time{}
	s.Hour = 9
	s.Weekdays = []int{1, 2, 3, 4, 5}
	if next := s.NextAfter(at("2026-10-16T03:00:00Z")); !next.Equal(at("2026-10-19T02:00:00Z")) {
		t.Fatalf("weekday next %s", next)
	}
	s.NextAt = at("2026-10-16T02:00:00Z")
	tick, ok = s.Due(at("2026-10-18T08:00:00Z"))
	if !ok || !tick.ScheduledAt.Equal(s.NextAt) || !tick.NextAt.Equal(at("2026-10-19T02:00:00Z")) {
		t.Fatalf("weekend coalesce %+v", tick)
	}
	s.Weekdays = []int{1, 1}
	if s.Validate() == nil {
		t.Fatal("duplicate weekday accepted")
	}
}

func TestScheduleSelectedWeekdaySkipsMissingDSTWeek(t *testing.T) {
	s := Schedule{ID: "sunday", SourceID: "job", Kind: "daily", TimeZone: "Europe/Berlin", Hour: 2, Minute: 30, Weekdays: []int{0}, Enabled: true, MissedPolicy: "coalesce"}
	if next := s.NextAfter(at("2026-03-28T00:00:00Z")); !next.Equal(at("2026-04-05T00:30:00Z")) {
		t.Fatalf("lost selected weekday across missing DST minute: %s", next)
	}
	s.NextAt = at("2026-03-15T01:30:00Z")
	tick, ok := s.Due(at("2026-04-04T10:00:00Z"))
	if !ok || !tick.ScheduledAt.Equal(at("2026-03-22T01:30:00Z")) {
		t.Fatalf("coalesced to an obsolete selected day: %+v", tick)
	}
}
