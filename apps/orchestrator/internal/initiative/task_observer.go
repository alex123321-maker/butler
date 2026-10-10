package initiative

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
)

// TaskObserver reads only Task/Run Service's durable state. It does not execute
// a tool or external request. Project identity/revision is pinned by owner
// configuration; neither provider text nor raw run errors become instructions.
type TaskObserver struct{ Store *Store }

func (o TaskObserver) Check(ctx context.Context, source Source, _ ClaimedTick) (*Event, error) {
	if source.ObserverKind == "" {
		return nil, nil
	}
	if source.Validate() != nil || source.ObserverKind != "task_project" || o.Store == nil {
		return nil, ErrInvalid
	}
	var latest time.Time
	var completed bool
	// Only current task revisions and owner-bound project metadata qualify.
	// A verified newer result clears earlier failures, even before their ticks
	// were consumed. A model final answer cannot supply this healthy state.
	err := o.Store.pool.QueryRow(ctx, `SELECT at,healthy FROM (
	 SELECT r.finished_at AS at,false AS healthy FROM runs r JOIN tasks t ON t.task_id=r.task_id
	 WHERE t.owner_id=$1 AND t.source_json->>'project_ref'=$2 AND t.source_json->>'project_revision'=$3
	 AND r.task_revision=t.revision AND r.status IN ('failed','timed_out') AND r.finished_at IS NOT NULL
	 UNION ALL
	 SELECT t.finished_at,true FROM tasks t JOIN task_results z ON z.result_id=t.completion_result_id AND z.task_id=t.task_id
	 WHERE t.owner_id=$1 AND t.source_json->>'project_ref'=$2 AND t.source_json->>'project_revision'=$3
	 AND t.status='completed' AND z.verdict='verified' AND t.finished_at IS NOT NULL
	) facts ORDER BY at DESC,healthy DESC LIMIT 1`, source.OwnerID, source.ProjectRef, source.ProjectRevision).Scan(&latest, &completed)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if latest.Before(time.Now().UTC().Add(-24 * time.Hour)) {
		return nil, nil
	}
	if completed {
		return &Event{Kind: "project_healthy", ConditionDigest: hash([]string{source.ProjectRef, "attempt_failed"}), ResourceRevision: source.ProjectRevision, EvidenceDigest: hash([]any{"verified_project_result", latest}), Confidence: 100, Importance: 70, ObservedAt: latest}, nil
	}
	var runID string
	var own bool
	var at time.Time
	err = o.Store.pool.QueryRow(ctx, `SELECT r.run_id,COALESCE(t.source_json->>'type'='initiative',false),r.finished_at
	 FROM runs r JOIN tasks t ON t.task_id=r.task_id
	 WHERE t.owner_id=$1 AND t.source_json->>'project_ref'=$2 AND t.source_json->>'project_revision'=$3
	 AND r.task_revision=t.revision AND r.status IN ('failed','timed_out') AND r.finished_at>clock_timestamp()-INTERVAL '24 hours'
	 AND NOT EXISTS(SELECT 1 FROM initiative_events e WHERE e.source_id=$4 AND e.event_json->>'observed_attempt_id'=r.run_id)
	 ORDER BY r.finished_at,r.run_id LIMIT 1`, source.OwnerID, source.ProjectRef, source.ProjectRevision, source.ID).Scan(&runID, &own, &at)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &Event{Kind: "project_error", ConditionDigest: hash([]string{source.ProjectRef, "attempt_failed"}), ResourceRevision: source.ProjectRevision, EvidenceDigest: hash([]string{"project_attempt", runID}), Confidence: 100, Importance: 70, ProducedByButler: own, ObservedAttemptID: runID, ObservedAt: at}, nil
}

func currentTaskCondition(ctx context.Context, tx pgx.Tx, source Source, event Event) (bool, error) {
	if source.ObserverKind == "broker_project" {
		return currentBrokerCondition(ctx, tx, source, event)
	}
	if source.ObserverKind != "task_project" {
		return true, nil
	}
	if event.ObservedAttemptID == "" || event.ResourceRevision != source.ProjectRevision {
		return false, nil
	}
	var current bool
	err := tx.QueryRow(ctx, `SELECT EXISTS(
	 SELECT 1 FROM runs r JOIN tasks t ON t.task_id=r.task_id
	 WHERE r.run_id=$1 AND r.task_revision=t.revision AND r.status IN ('failed','timed_out')
	 AND t.owner_id=$2 AND t.source_json->>'project_ref'=$3 AND t.source_json->>'project_revision'=$4
	 AND r.finished_at>clock_timestamp()-INTERVAL '24 hours'
	 AND NOT EXISTS(SELECT 1 FROM tasks h JOIN task_results z ON z.result_id=h.completion_result_id AND z.task_id=h.task_id
	 WHERE h.owner_id=$2 AND h.source_json->>'project_ref'=$3 AND h.source_json->>'project_revision'=$4
	 AND h.status='completed' AND z.verdict='verified' AND h.finished_at>=r.finished_at))`, event.ObservedAttemptID, source.OwnerID, source.ProjectRef, source.ProjectRevision).Scan(&current)
	return current, err
}
