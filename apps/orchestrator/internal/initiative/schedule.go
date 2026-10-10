package initiative

import (
	"sort"
	"time"
)

type Schedule struct {
	ID              string    `json:"schedule_id"`
	SourceID        string    `json:"source_id"`
	TimeZone        string    `json:"time_zone"`
	Kind            string    `json:"kind"`
	Hour            int       `json:"hour"`
	Minute          int       `json:"minute"`
	IntervalSeconds int       `json:"interval_seconds"`
	MissedPolicy    string    `json:"missed_policy"`
	GraceSeconds    int       `json:"grace_seconds"`
	NextAt          time.Time `json:"next_at"`
	Enabled         bool      `json:"enabled"`
	Revision        int64     `json:"revision"`
}

func (s Schedule) Validate() error {
	if !identifier.MatchString(s.ID) || !identifier.MatchString(s.SourceID) {
		return ErrInvalid
	}
	if _, err := time.LoadLocation(s.TimeZone); err != nil || s.TimeZone == "" || s.TimeZone == "Local" {
		return ErrInvalid
	}
	if s.MissedPolicy != "skip" && s.MissedPolicy != "coalesce" {
		return ErrInvalid
	}
	if s.GraceSeconds < 0 || s.GraceSeconds > 3600 {
		return ErrInvalid
	}
	switch s.Kind {
	case "daily":
		if s.Hour < 0 || s.Hour > 23 || s.Minute < 0 || s.Minute > 59 || s.IntervalSeconds != 0 {
			return ErrInvalid
		}
	case "interval":
		if s.IntervalSeconds < 60 || s.IntervalSeconds > 7*24*3600 {
			return ErrInvalid
		}
	default:
		return ErrInvalid
	}
	return nil
}

// A missing local minute is skipped. An ambiguous minute uses its first actual
// occurrence, with one calendar-date key, preventing a duplicate autumn tick.
func localInstants(year int, month time.Month, day, hour, minute int, loc *time.Location) []time.Time {
	nominal := time.Date(year, month, day, hour, minute, 0, 0, time.UTC)
	offsets := map[int]bool{}
	for _, delta := range []time.Duration{-48 * time.Hour, -24 * time.Hour, 0, 24 * time.Hour, 48 * time.Hour} {
		_, offset := nominal.Add(delta).In(loc).Zone()
		offsets[offset] = true
	}
	result := []time.Time{}
	for offset := range offsets {
		candidate := nominal.Add(-time.Duration(offset) * time.Second)
		actual := candidate.In(loc)
		if actual.Year() == year && actual.Month() == month && actual.Day() == day && actual.Hour() == hour && actual.Minute() == minute {
			result = append(result, candidate)
		}
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Before(result[j]) })
	return result
}

func (s Schedule) NextAfter(at time.Time) time.Time {
	if s.Validate() != nil || at.IsZero() {
		return time.Time{}
	}
	if s.Kind == "interval" {
		return at.Add(time.Duration(s.IntervalSeconds) * time.Second).UTC()
	}
	loc, _ := time.LoadLocation(s.TimeZone)
	local := at.In(loc)
	for day := 0; day < 8; day++ {
		date := local.AddDate(0, 0, day)
		instants := localInstants(date.Year(), date.Month(), date.Day(), s.Hour, s.Minute, loc)
		if len(instants) > 0 && instants[0].After(at) {
			return instants[0]
		}
	}
	return time.Time{}
}

type Tick struct {
	SlotKey       string    `json:"slot_key"`
	ScheduledAt   time.Time `json:"scheduled_at"`
	NextAt        time.Time `json:"next_at"`
	MissedCount   int64     `json:"missed_count"`
	ShouldObserve bool      `json:"should_observe"`
}

// Due coalesces downtime into at most one observation, advancing past now in the
// same durable transaction. It never queues one task per historical tick.
func (s Schedule) Due(now time.Time) (Tick, bool) {
	if s.Validate() != nil || now.IsZero() || !s.Enabled || s.NextAt.IsZero() || s.NextAt.After(now) {
		return Tick{}, false
	}
	latest := s.NextAt.UTC()
	missed := int64(0)
	if s.Kind == "interval" {
		interval := time.Duration(s.IntervalSeconds) * time.Second
		missed = int64(now.Sub(latest) / interval)
		latest = latest.Add(time.Duration(missed) * interval)
	} else {
		loc, _ := time.LoadLocation(s.TimeZone)
		local := now.In(loc)
		for day := 0; day < 8; day++ {
			date := local.AddDate(0, 0, -day)
			candidates := localInstants(date.Year(), date.Month(), date.Day(), s.Hour, s.Minute, loc)
			if len(candidates) > 0 && !candidates[0].After(now) && !candidates[0].Before(s.NextAt) {
				latest = candidates[0]
				first := s.NextAt.In(loc)
				firstDate := time.Date(first.Year(), first.Month(), first.Day(), 0, 0, 0, 0, time.UTC)
				lastDate := time.Date(date.Year(), date.Month(), date.Day(), 0, 0, 0, 0, time.UTC)
				missed = int64(lastDate.Sub(firstDate) / (24 * time.Hour))
				break
			}
		}
	}
	slot := latest.Format(time.RFC3339)
	if s.Kind == "daily" {
		loc, _ := time.LoadLocation(s.TimeZone)
		slot = latest.In(loc).Format("2006-01-02")
	}
	observe := s.MissedPolicy == "coalesce" || now.Sub(latest) <= time.Duration(s.GraceSeconds)*time.Second
	next := s.NextAfter(now)
	if s.Kind == "interval" {
		next = latest.Add(time.Duration(s.IntervalSeconds) * time.Second)
	}
	return Tick{SlotKey: slot, ScheduledAt: latest, NextAt: next, MissedCount: missed, ShouldObserve: observe}, true
}
