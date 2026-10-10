package initiative

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
)

const candidateColumns = `initiative_id,source_id,source_revision,owner_id,goal_id,mandate_id,fingerprint,status,reason,occurrences,confidence,importance,expected_model_calls,attention_cost,not_before,last_observed_at,COALESCE(task_id,''),COALESCE(input_id,''),observation_ref`

func scanCandidate(row sourceRow) (Candidate, error) {
	var c Candidate
	err := row.Scan(&c.ID, &c.SourceID, &c.SourceRevision, &c.OwnerID, &c.GoalID, &c.MandateID, &c.Fingerprint, &c.Status, &c.Reason, &c.Occurrences, &c.Confidence, &c.Importance, &c.ExpectedModelCalls, &c.AttentionCost, &c.NotBefore, &c.LastObservedAt, &c.TaskID, &c.InputID, &c.ObservationRef)
	if errors.Is(err, pgx.ErrNoRows) {
		return c, ErrNotFound
	}
	return c, err
}

// Observe is called only by an authenticated source adapter, never a model tool.
// Confidence, origin and normalized condition identities belong to that adapter.
// The event remains data and cannot assign execution authority to a candidate.
func (s *Store) Observe(ctx context.Context, ownerID string, event Event, expectedSourceRevision int64, at time.Time) (Candidate, bool, error) {
	return s.observe(ctx, ownerID, event, expectedSourceRevision, at, nil)
}

func (s *Store) ObserveTick(ctx context.Context, ownerID string, event Event, tick ClaimedTick, at time.Time) (Candidate, bool, error) {
	if event.SourceID != tick.SourceID {
		return Candidate{}, false, ErrInvalid
	}
	return s.observe(ctx, ownerID, event, tick.SourceRevision, at, &tick)
}

func (s *Store) observe(ctx context.Context, ownerID string, event Event, expectedSourceRevision int64, at time.Time, tick *ClaimedTick) (Candidate, bool, error) {
	if event.Validate() != nil || !identifier.MatchString(ownerID) || expectedSourceRevision < 1 || at.IsZero() {
		return Candidate{}, false, ErrInvalid
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return Candidate{}, false, err
	}
	defer tx.Rollback(ctx)
	source, err := scanSource(tx.QueryRow(ctx, `SELECT source_json,revision,enabled FROM initiative_sources WHERE source_id=$1 AND owner_id=$2 FOR SHARE`, event.SourceID, ownerID))
	if err != nil {
		return Candidate{}, false, err
	}
	if source.Revision != expectedSourceRevision {
		return Candidate{}, false, ErrConflict
	}
	if tick != nil {
		var revision int64
		var enabled bool
		if err = tx.QueryRow(ctx, `SELECT revision,enabled FROM initiative_schedules WHERE schedule_id=$1 AND source_id=$2 FOR SHARE`, tick.ScheduleID, source.ID).Scan(&revision, &enabled); err != nil {
			return Candidate{}, false, err
		}
		if !enabled || revision != tick.ScheduleRevision {
			return Candidate{}, false, ErrConflict
		}
		var current bool
		if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM initiative_ticks WHERE schedule_id=$1 AND schedule_revision=$2 AND slot_key=$3 AND claim_id=$4 AND status='pending' AND claim_expires_at>clock_timestamp())`, tick.ScheduleID, tick.ScheduleRevision, tick.SlotKey, tick.ClaimID).Scan(&current); err != nil {
			return Candidate{}, false, err
		}
		if !current {
			return Candidate{}, false, ErrConflict
		}
	}
	fingerprint := event.fingerprint(source)
	if _, err = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,0))`, fingerprint); err != nil {
		return Candidate{}, false, err
	}
	encoded, _ := json.Marshal(event)
	var oldEvent []byte
	var previousID, verdict string
	err = tx.QueryRow(ctx, `SELECT event_json,COALESCE(initiative_id,''),verdict FROM initiative_events WHERE source_id=$1 AND event_id=$2`, source.ID, event.ID).Scan(&oldEvent, &previousID, &verdict)
	if err == nil {
		var previous Event
		if json.Unmarshal(oldEvent, &previous) != nil || hash(previous) != hash(event) {
			return Candidate{}, false, ErrConflict
		}
		previousCandidate := Candidate{Status: "suppressed", Reason: verdict}
		if previousID != "" {
			previousCandidate, err = scanCandidate(tx.QueryRow(ctx, `SELECT `+candidateColumns+` FROM initiatives WHERE initiative_id=$1`, previousID))
			if err != nil {
				return Candidate{}, false, err
			}
		}
		return previousCandidate, false, tx.Commit(ctx)
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return Candidate{}, false, err
	}
	status, reason, _ := Decide(source, event, 0, at)
	if event.ObservedAt.After(at.Add(time.Minute)) || event.ObservedAt.Before(at.Add(-24*time.Hour)) {
		status, reason = "rejected", "stale_observation"
	}
	// Own changes and empty checks have an audit record but no task candidate.
	if event.ProducedByButler || event.Kind == "periodic" || event.Kind == "project_healthy" || reason == "stale_observation" || !source.Enabled {
		_, err = tx.Exec(ctx, `INSERT INTO initiative_events(source_id,event_id,event_json,verdict,observed_at) VALUES($1,$2,$3::jsonb,$4,$5)`, source.ID, event.ID, encoded, reason, event.ObservedAt)
		if err != nil {
			return Candidate{}, false, err
		}
		return Candidate{Status: status, Reason: reason}, true, tx.Commit(ctx)
	}
	c, err := scanCandidate(tx.QueryRow(ctx, `SELECT `+candidateColumns+` FROM initiatives WHERE owner_id=$1 AND fingerprint=$2 FOR UPDATE`, ownerID, fingerprint))
	if errors.Is(err, ErrNotFound) {
		id, idErr := newID("initiative_")
		if idErr != nil {
			return Candidate{}, false, idErr
		}
		c = Candidate{ID: id, SourceID: source.ID, SourceRevision: source.Revision, OwnerID: ownerID, GoalID: source.GoalID, MandateID: source.MandateID, Fingerprint: fingerprint}
	} else if err != nil {
		return Candidate{}, false, err
	}
	verdict = "merged_observation"
	if c.SourceID != source.ID || c.SourceRevision != source.Revision {
		verdict = "candidate_source_changed"
	} else if c.TaskID != "" || c.Status == "completed" || c.Status == "rejected" {
		// Delivery changes cannot reopen work for an already admitted revision.
		verdict = "already_admitted"
	} else {
		var repeatedEvidence bool
		if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM initiative_events WHERE initiative_id=$1 AND event_json->>'evidence_digest'=$2)`, c.ID, event.EvidenceDigest).Scan(&repeatedEvidence); err != nil {
			return Candidate{}, false, err
		}
		if !repeatedEvidence {
			if c.Occurrences < source.FailureThreshold {
				c.Occurrences++
			}
			c.Confidence, c.Importance = event.Confidence, event.Importance
			c.LastObservedAt = event.ObservedAt.UTC()
			c.ObservationRef = "observation:" + hash([]string{source.ID, event.ID})
			c.Status, c.Reason, c.NotBefore = Decide(source, event, c.Occurrences, at)
			c.ExpectedModelCalls, c.AttentionCost = 0, 0
			if c.Status == "ready" || c.Status == "deferred" {
				c.ExpectedModelCalls, c.AttentionCost = 2, 1
			}
		}
		// Repeated delivery of one observation cannot change confidence or
		// satisfy the three-distinct-failures threshold.
		if repeatedEvidence {
			verdict = "duplicate_evidence"
		}
		if _, err = tx.Exec(ctx, `INSERT INTO initiatives(initiative_id,owner_id,goal_id,mandate_id,source_id,source_revision,fingerprint,observation_ref,status,reason,occurrences,confidence,importance,expected_model_calls,attention_cost,not_before,last_observed_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17) ON CONFLICT(initiative_id) DO UPDATE SET observation_ref=EXCLUDED.observation_ref,status=EXCLUDED.status,reason=EXCLUDED.reason,occurrences=EXCLUDED.occurrences,confidence=EXCLUDED.confidence,importance=EXCLUDED.importance,expected_model_calls=EXCLUDED.expected_model_calls,attention_cost=EXCLUDED.attention_cost,not_before=EXCLUDED.not_before,last_observed_at=EXCLUDED.last_observed_at,updated_at=clock_timestamp()`, c.ID, c.OwnerID, c.GoalID, c.MandateID, c.SourceID, c.SourceRevision, c.Fingerprint, c.ObservationRef, c.Status, c.Reason, c.Occurrences, c.Confidence, c.Importance, c.ExpectedModelCalls, c.AttentionCost, c.NotBefore, c.LastObservedAt); err != nil {
			return Candidate{}, false, err
		}
	}
	_, err = tx.Exec(ctx, `INSERT INTO initiative_events(source_id,event_id,event_json,initiative_id,verdict,observed_at) VALUES($1,$2,$3::jsonb,$4,$5,$6)`, source.ID, event.ID, encoded, c.ID, verdict, event.ObservedAt)
	if err != nil {
		return Candidate{}, false, err
	}
	return c, true, tx.Commit(ctx)
}

func (s *Store) Get(ctx context.Context, id, ownerID string) (Candidate, error) {
	return scanCandidate(s.pool.QueryRow(ctx, `SELECT `+candidateColumns+` FROM initiatives WHERE initiative_id=$1 AND owner_id=$2`, id, ownerID))
}

func (s *Store) List(ctx context.Context, ownerID string, limit int) ([]Candidate, error) {
	if limit < 1 || limit > 200 {
		limit = 50
	}
	rows, err := s.pool.Query(ctx, `SELECT `+candidateColumns+` FROM initiatives WHERE owner_id=$1 ORDER BY updated_at DESC,initiative_id LIMIT $2`, ownerID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]Candidate, 0)
	for rows.Next() {
		c, err := scanCandidate(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, c)
	}
	return result, rows.Err()
}
