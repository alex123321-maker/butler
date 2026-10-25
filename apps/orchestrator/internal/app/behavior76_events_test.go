package app

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/butler/butler/apps/orchestrator/internal/task"
	"github.com/butler/butler/internal/memory/catalog"
	memoryservice "github.com/butler/butler/internal/memory/service"
	"github.com/butler/butler/internal/security/content"
	"github.com/jackc/pgx/v5"
)

func (f *behavior76Fixture) apply(step behavior76Step) (any, error) {
	e := step.Event
	switch e.Type {
	case "owner_message":
		return map[string]any{"type": e.Type, "intent": e.Intent, "target_task_key": e.TargetTaskKey, "text": e.Text, "natural_language_target_inferred": false}, nil
	case "owner_control":
		r, err := f.tasks.Get(f.ctx, f.lookup("task", e.TaskKey))
		if err != nil {
			return nil, err
		}
		return f.controls.Apply(f.actor("control:"+step.ID), task.ControlRequest{TaskID: r.TaskID, ExpectedRevision: r.Revision, Kind: e.Kind, Instruction: e.Instruction, Goal: e.Goal, Constraints: e.Constraints, ExecutionPolicy: e.ExecutionPolicy})
	case "manual_resource_change":
		if e.Provenance == "owner_report" {
			observation := behavior76Observation{Kind: "owner_report", ResourceKey: e.ResourceKey, Summary: e.NewBody, EvidenceKind: "owner_report"}
			_, err := f.pool.Exec(f.ctx, `INSERT INTO behavior76_observations(observation) VALUES($1)`, behavior76JSON(observation))
			return observation, err
		}
		if e.Provenance != "independently_observed" {
			return nil, fmt.Errorf("unsupported manual-change provenance")
		}
		tx, err := f.pool.Begin(f.ctx)
		if err != nil {
			return nil, err
		}
		defer tx.Rollback(f.ctx)
		before, err := behavior76ObjectQuery(f.ctx, tx, e.ResourceKey, "FOR UPDATE")
		if err != nil {
			return nil, err
		}
		after := before
		after.Body = e.NewBody
		after.Version++
		after.SHA256 = behavior76Hash([]byte(e.NewBody))
		if _, err = tx.Exec(f.ctx, `UPDATE behavior76_objects SET body=$2,version=$3,sha256=$4 WHERE key=$1`, e.ResourceKey, after.Body, after.Version, after.SHA256); err != nil {
			return nil, err
		}
		if _, err = tx.Exec(f.ctx, `INSERT INTO behavior76_object_events(resource_key,kind,before_state,after_state) VALUES($1,'independent_owner_change',$2,$3)`, e.ResourceKey, behavior76JSON(before), behavior76JSON(after)); err != nil {
			return nil, err
		}
		return map[string]any{"kind": "independent_owner_change", "before": before, "after": after}, tx.Commit(f.ctx)
	case "profile_change", "document_change":
		src, err := f.sources.Get(f.actor("change-read:"+step.ID), f.lookup("source", e.SourceKey))
		if err != nil {
			return nil, err
		}
		if e.Type == "profile_change" && (e.Operation == "revoke" || e.Operation == "delete") {
			return f.sources.Delete(f.actor("lifecycle:"+step.ID), src.ID, e.Operation, src.Revision)
		}
		body := e.NewBody
		if e.Type == "profile_change" {
			if e.Operation != "correct" || e.Summary == nil {
				return nil, fmt.Errorf("unsupported profile correction")
			}
			body = *e.Summary
		}
		updated, err := f.sources.Correct(f.actor("correction:"+step.ID), catalog.Correction{SourceID: src.ID, ExpectedRevision: src.Revision, Title: src.Title, Body: body})
		if err != nil {
			return nil, err
		}
		if e.Type == "profile_change" {
			var key, scopeType, scopeID, old string
			if err = f.pool.QueryRow(f.ctx, `SELECT fact_key,scope_type,scope_id FROM memory_claims WHERE claim_id=$1`, f.lookup("claim", e.SourceKey)).Scan(&key, &scopeType, &scopeID); err != nil {
				return nil, err
			}
			value, _ := json.Marshal(body)
			claim, err := f.sources.Propose(f.actor("correction-propose:"+step.ID), catalog.ClaimRequest{SourceID: updated.ID, SourceRevision: updated.Revision, Key: key, Value: value, Summary: body, Confidence: 1})
			if err != nil {
				return nil, err
			}
			if err = f.pool.QueryRow(f.ctx, `SELECT active_claim_id FROM memory_fact_heads WHERE owner_id=$1 AND scope_type=$2 AND scope_id=$3 AND fact_key=$4`, behavior76Owner, scopeType, scopeID, key).Scan(&old); err != nil {
				return nil, err
			}
			accepted, err := f.sources.Decide(f.actor("correction-accept:"+step.ID), catalog.Decision{ClaimID: claim.ID, ExpectedActiveID: old})
			if err != nil {
				return nil, err
			}
			f.alias("claim", e.SourceKey, accepted.ID)
		}
		return updated, nil
	case "service_restart":
		// Owning services and native provider are rebuilt. Only aliases stored in
		// PostgreSQL and immutable stimuli remain; no old task/profile value is kept.
		f.reconstruct()
		return map[string]any{"kind": "service_restart", "durable_store": "postgresql", "provider_reconstructed_on_next_turn": true}, nil
	default:
		return nil, fmt.Errorf("unsupported fixture event")
	}
}

type behavior76Documents struct{ f *behavior76Fixture }

func (d behavior76Documents) Search(ctx context.Context, _ []memoryservice.Scope, _ string, limit int) (hits []memoryservice.SemanticHit, err error) {
	defer func() {
		if err != nil && d.f.researchStop != nil {
			d.f.researchStop.Store(true)
		}
	}()
	// Explicit fixture-selected documents, not an embedding-quality claim. The
	// ordinary bundle/formatter still accounts bytes, provenance and exclusions.
	rows, err := d.f.pool.Query(ctx, `SELECT value FROM behavior76_aliases WHERE kind='document' ORDER BY key`)
	if err != nil {
		return nil, err
	}
	var ids []string
	for rows.Next() {
		var id string
		if err = rows.Scan(&id); err != nil {
			rows.Close()
			return nil, err
		}
		ids = append(ids, id)
	}
	rows.Close()
	if err = rows.Err(); err != nil {
		return nil, err
	}
	var out []memoryservice.SemanticHit
	for _, id := range ids {
		if len(out) == limit {
			break
		}
		src, err := d.f.sources.Get(ctx, id)
		if err != nil {
			return nil, err
		}
		out = append(out, memoryservice.SemanticHit{Title: src.Title, Summary: src.Body, SourceRef: src.ID, Revision: fmt.Sprint(src.Revision), Origin: content.Origin{Kind: content.Document, SourceRefs: []string{src.ID}, Sensitivity: content.Private}})
	}
	return out, nil
}

func (f *behavior76Fixture) enrichWorking(key string) (err error) {
	defer func() {
		if err != nil && f.researchStop != nil {
			f.researchStop.Store(true)
		}
	}()
	record, err := f.tasks.Get(f.ctx, f.lookup("task", key))
	if err != nil {
		return err
	}
	working := record.Working
	if working.Scratch == nil {
		working.Scratch = map[string]any{}
	}
	var inventory, observations, resourceRefs []json.RawMessage
	for _, query := range []struct {
		sql    string
		target *[]json.RawMessage
	}{
		{`SELECT jsonb_build_object('task_key',a.key,'task_id',t.task_id,'goal',t.goal,'status',t.status,'revision',t.revision,'constraints_revision',t.constraints_revision) FROM behavior76_aliases a JOIN tasks t ON t.task_id=a.value WHERE a.kind='task' ORDER BY a.key`, &inventory},
		{`SELECT observation FROM behavior76_observations ORDER BY n`, &observations},
		{`SELECT jsonb_build_object('key',key,'task_key',task_key,'url','https://fixture.invalid/objects/'||key) FROM behavior76_objects ORDER BY key`, &resourceRefs},
	} {
		rows, e := f.pool.Query(f.ctx, query.sql)
		if e != nil {
			return e
		}
		for rows.Next() {
			var raw []byte
			if e = rows.Scan(&raw); e != nil {
				rows.Close()
				return e
			}
			*query.target = append(*query.target, json.RawMessage(raw))
		}
		rows.Close()
		if e = rows.Err(); e != nil {
			return e
		}
	}
	working.Scratch["synthetic_fixture_task_inventory"] = inventory
	working.Scratch["synthetic_fixture_observations"] = observations
	working.Scratch["synthetic_fixture_resource_refs"] = resourceRefs
	_, err = f.tasks.SaveWorking(f.ctx, record.TaskID, record.WorkingRevision, working)
	return err
}

func behavior76SafeError(err error) string {
	if err == nil {
		return ""
	}
	if strings.Contains(err.Error(), "task is not active") || strings.Contains(err.Error(), "current preparation task authority") {
		return "inactive_task_denied"
	}
	return "fixture_operation_unavailable"
}

func behavior76Rows(ctx context.Context, q interface {
	Query(context.Context, string, ...any) (pgx.Rows, error)
}, table string) ([]json.RawMessage, error) {
	rows, err := q.Query(ctx, `SELECT to_jsonb(t) FROM `+pgx.Identifier{table}.Sanitize()+` t`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []json.RawMessage{}
	for rows.Next() {
		var raw []byte
		if err = rows.Scan(&raw); err != nil {
			return nil, err
		}
		out = append(out, json.RawMessage(raw))
	}
	return out, rows.Err()
}
