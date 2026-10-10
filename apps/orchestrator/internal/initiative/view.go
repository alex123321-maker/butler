package initiative

import (
	"context"
	"time"
)

type AuditEvent struct {
	SourceID    string    `json:"source_id"`
	EventID     string    `json:"event_id"`
	CandidateID string    `json:"initiative_id,omitempty"`
	Verdict     string    `json:"verdict"`
	ObservedAt  time.Time `json:"observed_at"`
}
type View struct {
	Sources        []Source     `json:"sources"`
	Schedules      []Schedule   `json:"schedules"`
	Candidates     []Candidate  `json:"initiatives"`
	Events         []AuditEvent `json:"events"`
	AttentionUsed  int          `json:"attention_used_today"`
	AttentionLimit int          `json:"attention_daily_limit"`
}

// View is a quiet owner pull/digest. It never queues notifications for empty
// checks, proposed low-confidence work, duplicates or scheduler bookkeeping.
func (s *Store) View(ctx context.Context, owner string) (View, error) {
	v := View{Sources: []Source{}, Schedules: []Schedule{}, Events: []AuditEvent{}, AttentionLimit: 20}
	rows, err := s.pool.Query(ctx, `SELECT source_json,revision,enabled FROM initiative_sources WHERE owner_id=$1 ORDER BY source_id LIMIT 200`, owner)
	if err != nil {
		return v, err
	}
	for rows.Next() {
		source, err := scanSource(rows)
		if err != nil {
			rows.Close()
			return v, err
		}
		v.Sources = append(v.Sources, source)
	}
	rows.Close()
	if err = rows.Err(); err != nil {
		return v, err
	}
	rows, err = s.pool.Query(ctx, `SELECT q.schedule_json,q.next_at,q.revision,q.enabled FROM initiative_schedules q JOIN initiative_sources s ON s.source_id=q.source_id WHERE s.owner_id=$1 ORDER BY q.schedule_id LIMIT 200`, owner)
	if err != nil {
		return v, err
	}
	for rows.Next() {
		q, err := scanSchedule(rows)
		if err != nil {
			rows.Close()
			return v, err
		}
		v.Schedules = append(v.Schedules, q)
	}
	rows.Close()
	if err = rows.Err(); err != nil {
		return v, err
	}
	if v.Candidates, err = s.List(ctx, owner, 100); err != nil {
		return v, err
	}
	rows, err = s.pool.Query(ctx, `SELECT e.source_id,e.event_id,COALESCE(e.initiative_id,''),e.verdict,e.observed_at FROM initiative_events e JOIN initiative_sources s ON s.source_id=e.source_id WHERE s.owner_id=$1 ORDER BY e.recorded_at DESC,e.event_id LIMIT 100`, owner)
	if err != nil {
		return v, err
	}
	for rows.Next() {
		var a AuditEvent
		if err = rows.Scan(&a.SourceID, &a.EventID, &a.CandidateID, &a.Verdict, &a.ObservedAt); err != nil {
			rows.Close()
			return v, err
		}
		v.Events = append(v.Events, a)
	}
	rows.Close()
	if err = rows.Err(); err != nil {
		return v, err
	}
	err = s.pool.QueryRow(ctx, `SELECT COALESCE((SELECT admitted_tasks FROM initiative_attention_days WHERE owner_id=$1 AND day=(clock_timestamp() AT TIME ZONE 'UTC')::date),0)`, owner).Scan(&v.AttentionUsed)
	return v, err
}
