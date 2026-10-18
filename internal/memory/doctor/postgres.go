package doctor

import (
	"context"
	"encoding/json"
	"time"

	"github.com/butler/butler/internal/memory/diagnostics"
	"github.com/jackc/pgx/v5/pgxpool"
)

type postgresReader struct{ pool *pgxpool.Pool }
type JobObservation struct {
	Pending                 int64            `json:"pending"`
	Running                 int64            `json:"running"`
	Completed               int64            `json:"completed"`
	Failed                  int64            `json:"failed"`
	Cancelled               int64            `json:"cancelled"`
	Retries                 int64            `json:"retries"`
	OldestPendingSeconds    *float64         `json:"oldest_pending_seconds"`
	Reasons                 map[string]int64 `json:"reasons"`
	ModelControl            string           `json:"model_control"`
	ModelAttempts           int64            `json:"model_attempts"`
	ModelUnknownUsage       int64            `json:"model_unknown_usage"`
	ModelReservedTokens     int64            `json:"model_reserved_tokens"`
	ModelReservedCostMicros int64            `json:"model_reserved_cost_micros"`
}
type IndexObservation struct {
	ID         string          `json:"generation_id"`
	ScopeType  string          `json:"scope_type"`
	ScopeID    string          `json:"scope_id"`
	SpaceID    string          `json:"space_id"`
	State      string          `json:"state"`
	Active     bool            `json:"active"`
	VerifiedAt *time.Time      `json:"verified_at"`
	Proof      json.RawMessage `json:"generation_proof,omitempty"`
}

func (p *postgresReader) Stores(ctx context.Context) map[string]diagnostics.Component {
	result := map[string]diagnostics.Component{}
	for name, table := range map[string]string{"transcript": "messages", "working": "tasks", "legacy_working": "memory_working", "profile": "memory_profile", "episodic": "memory_episodes", "chunks": "memory_chunks"} {
		c := diagnostics.Component{Configured: true, Effective: true, Required: true, Status: "healthy", Reason: "sql_read_checked", CheckedAt: time.Now().UTC()}
		if ctx.Err() != nil {
			c.Status = "unknown"
			c.Reason = "not_observed"
			result[name] = c
			continue
		}
		probe, cancel := context.WithTimeout(ctx, time.Second)
		rows, err := p.pool.Query(probe, "SELECT 1 FROM "+table+" LIMIT 0")
		if err == nil {
			rows.Close()
			err = rows.Err()
		}
		cancel()
		if err != nil {
			c.Status = "failed"
			c.Reason = "backend"
		}
		result[name] = c
	}
	probe, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()
	var enabled bool
	err := p.pool.QueryRow(probe, `SELECT EXISTS(SELECT 1 FROM pg_extension WHERE extname='vector')`).Scan(&enabled)
	c := diagnostics.Component{Configured: true, Effective: enabled, Status: "healthy", Reason: "extension_checked", CheckedAt: time.Now().UTC()}
	if err != nil {
		c.Status = "failed"
		c.Reason = "backend"
	} else if !enabled {
		c.Status = "disabled"
		c.Reason = "extension_missing"
	}
	result["pgvector"] = c
	return result
}
func (p *postgresReader) Jobs(ctx context.Context, owner string) (JobObservation, error) {
	j := JobObservation{Reasons: map[string]int64{}}
	err := p.pool.QueryRow(ctx, `SELECT count(*) FILTER(WHERE status='pending'),count(*) FILTER(WHERE status='running'),count(*) FILTER(WHERE status='done'),count(*) FILTER(WHERE status='failed'),count(*) FILTER(WHERE status='cancelled'),coalesce(sum(greatest(attempts-1,0)),0),extract(epoch FROM clock_timestamp()-min(created_at) FILTER(WHERE status='pending')) FROM memory_aux_jobs WHERE ($1='' OR owner_id=$1)`, owner).Scan(&j.Pending, &j.Running, &j.Completed, &j.Failed, &j.Cancelled, &j.Retries, &j.OldestPendingSeconds)
	if err != nil {
		return j, err
	}
	err = p.pool.QueryRow(ctx, `SELECT (SELECT state FROM memory_model_control WHERE singleton),count(*),count(*) FILTER(WHERE a.usage IS NULL),COALESCE(sum((a.quote->>'tokens')::bigint),0),COALESCE(sum((a.quote->>'cost_micros')::bigint),0) FROM memory_model_attempts a JOIN memory_aux_jobs j USING(job_id) WHERE ($1='' OR j.owner_id=$1)`, owner).Scan(&j.ModelControl, &j.ModelAttempts, &j.ModelUnknownUsage, &j.ModelReservedTokens, &j.ModelReservedCostMicros)
	if err != nil {
		return j, err
	}
	rows, err := p.pool.Query(ctx, `SELECT CASE WHEN error_code IN ('claim_expired','index_rejected','local_model_unavailable','source_stale','budget_exhausted','cost_unknown','authority_or_source_denied','provider_failed','provider_timeout','cancelled','owner_cancelled','policy_revoked','claim_or_deadline_expired','admission_wait','interactive_priority','worker_disabled','worker_paused','deadline','local_backend_unavailable','storage_unavailable','worker_stopped') THEN error_code WHEN error_code='' THEN 'none' ELSE 'other' END,count(*) FROM memory_aux_jobs WHERE ($1='' OR owner_id=$1) GROUP BY 1`, owner)
	if err != nil {
		return j, err
	}
	defer rows.Close()
	for rows.Next() {
		var reason string
		var n int64
		if err = rows.Scan(&reason, &n); err != nil {
			return j, err
		}
		j.Reasons[reason] = n
	}
	return j, rows.Err()
}
func (p *postgresReader) Indexes(ctx context.Context, owner string) ([]IndexObservation, bool, error) {
	rows, err := p.pool.Query(ctx, `SELECT g.id,g.scope_type,g.scope_id,g.space_id,CASE WHEN g.retired_at IS NOT NULL THEN 'retired' ELSE g.state END,coalesce(a.generation_id=g.id,false),g.verified_at,g.quality_proof FROM semantic_index_generations g LEFT JOIN semantic_index_active a USING(scope_type,scope_id) WHERE (g.scope_type='user' AND g.scope_id=$1) OR (g.scope_type='session' AND EXISTS(SELECT 1 FROM sessions s WHERE s.session_key=g.scope_id AND s.user_id=$1)) OR (g.scope_type='task' AND EXISTS(SELECT 1 FROM tasks t WHERE t.task_id=g.scope_id AND t.owner_id=$1)) OR EXISTS(SELECT 1 FROM memory_index_jobs i JOIN memory_aux_jobs j USING(job_id) WHERE i.generation_id=g.id AND j.owner_id=$1) ORDER BY g.created_at DESC,g.id LIMIT 51`, owner)
	if err != nil {
		return nil, false, err
	}
	defer rows.Close()
	out := []IndexObservation{}
	for rows.Next() {
		var g IndexObservation
		if err = rows.Scan(&g.ID, &g.ScopeType, &g.ScopeID, &g.SpaceID, &g.State, &g.Active, &g.VerifiedAt, &g.Proof); err != nil {
			return nil, false, err
		}
		out = append(out, g)
	}
	truncated := len(out) > 50
	if truncated {
		out = out[:50]
	}
	return out, truncated, rows.Err()
}
