package initiative

import (
	"time"

	"github.com/butler/butler/internal/scheduling"
)

// Schedule retains the owner initiative wire format. The clock calculation is
// shared with scheduled agent jobs; source admission remains initiative-owned.
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

func (s Schedule) clock() scheduling.Schedule {
	return scheduling.Schedule{ID: s.ID, SourceID: s.SourceID, TimeZone: s.TimeZone, Kind: s.Kind, Hour: s.Hour, Minute: s.Minute, IntervalSeconds: s.IntervalSeconds, MissedPolicy: s.MissedPolicy, GraceSeconds: s.GraceSeconds, NextAt: s.NextAt, Enabled: s.Enabled, Revision: s.Revision}
}
func (s Schedule) Validate() error {
	if s.clock().Validate() != nil {
		return ErrInvalid
	}
	return nil
}
func (s Schedule) NextAfter(at time.Time) time.Time { return s.clock().NextAfter(at) }
func (s Schedule) Due(now time.Time) (Tick, bool)   { return s.clock().Due(now) }

type Tick = scheduling.Tick

func localInstants(year int, month time.Month, day, hour, minute int, loc *time.Location) []time.Time {
	return scheduling.LocalInstants(year, month, day, hour, minute, loc)
}
