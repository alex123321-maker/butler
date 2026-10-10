package initiative

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/butler/butler/internal/actionauth"
	"github.com/butler/butler/internal/extensions"
	"github.com/jackc/pgx/v5"
)

// ProjectObserver selects an installed PostgreSQL adapter. It never sends HTTP
// requests or calls a model. External observations must already have completed
// through the ordinary Broker grant, dispatch and evidence path.
type ProjectObserver struct {
	Store *Store
	// RunDataGuard applies source retention/release before and after reading.
	// A missing live guard makes this adapter unavailable.
	RunDataGuard func(context.Context, string) error
}

func (o ProjectObserver) Check(ctx context.Context, source Source, tick ClaimedTick) (*Event, error) {
	if source.ObserverKind != "broker_project" {
		return (TaskObserver{Store: o.Store}).Check(ctx, source, tick)
	}
	if o.Store == nil || o.RunDataGuard == nil || source.Validate() != nil {
		return nil, ErrInvalid
	}
	p, err := extensions.NewStore(o.Store.pool, "", nil, nil).ManifestByHash(ctx, source.CheckManifestHash)
	if err != nil {
		return nil, ErrInvalid
	}
	if !projectReadOperation(p, source.CheckToolName) {
		return nil, ErrInvalid
	}
	rows, err := o.Store.pool.Query(ctx, brokerFactsSQL+` ORDER BY c.finished_at DESC,c.tool_call_id DESC LIMIT 100`, source.OwnerID, source.ProjectRef, source.ProjectRevision, source.CheckToolName, source.CheckManifestHash, source.CheckResource, source.ID)
	if err != nil {
		return nil, err
	}
	facts := []brokerFact{}
	for rows.Next() {
		var f brokerFact
		if err = rows.Scan(&f.RunID, &f.CallID, &f.TaskID, &f.TaskRevision, &f.ConstraintsRevision, &f.Own, &f.StartedAt, &f.At, &f.Args, &f.Binding, &f.Result, &f.AlreadyObserved); err != nil {
			rows.Close()
			return nil, err
		}
		facts = append(facts, f)
	}
	rows.Close()
	if err = rows.Err(); err != nil {
		return nil, err
	}
	// Multiple calls in one run are one attempt. Consider its latest actual
	// check only, then consume independent attempts in chronological order.
	seen := map[string]bool{}
	var oldest *Event
	first := true
	for _, f := range facts {
		if seen[f.RunID] {
			continue
		}
		seen[f.RunID] = true
		if o.RunDataGuard(ctx, f.RunID) != nil {
			continue
		}
		e := f.event(source, p)
		if e == nil {
			continue
		}
		if first {
			var resolved bool
			err := o.Store.pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM tasks h JOIN task_results z ON z.result_id=h.completion_result_id AND z.task_id=h.task_id
			 WHERE h.owner_id=$1 AND h.source_json->>'project_ref'=$2 AND h.source_json->>'project_revision'=$3
			 AND h.status='completed' AND z.verdict='verified' AND h.finished_at>=$4)`, source.OwnerID, source.ProjectRef, source.ProjectRevision, f.At).Scan(&resolved)
			if err != nil {
				return nil, err
			}
			if resolved {
				e.Kind = "project_healthy"
				e.ProducedByButler = false
				return e, nil
			}
		}
		if o.RunDataGuard(ctx, f.RunID) != nil {
			continue
		}
		if first && e.Kind == "project_healthy" {
			return e, nil
		}
		first = false
		if e.Kind == "project_healthy" {
			break // older failures before a healthy state cannot be reconsidered
		}
		if !f.AlreadyObserved {
			oldest = e
		}
	}
	return oldest, nil
}

func projectReadOperation(p extensions.Package, name string) bool {
	for _, op := range p.Manifest.Operations {
		if op.Name == name {
			return op.Method == "GET" && op.Effect == "read"
		}
	}
	return false
}

type brokerFact struct {
	RunID, CallID, TaskID             string
	TaskRevision, ConstraintsRevision int64
	Own, AlreadyObserved              bool
	StartedAt, At                     time.Time
	Args, Binding, Result             []byte
}

// Result bodies remain untrusted data. Only hashes, fixed confidence/importance,
// and Task Service origin leave this adapter. No result field grants authority.
func (f brokerFact) event(source Source, p extensions.Package) *Event {
	var b actionauth.ActionBinding
	var args map[string]any
	if json.Unmarshal(f.Binding, &b) != nil || json.Unmarshal(f.Args, &args) != nil || len(args) != 1 || args["url"] != source.CheckResource {
		return nil
	}
	argsHash, err := actionauth.ArgumentsHash(string(f.Args))
	if err != nil {
		return nil
	}
	if b.OwnerID != source.OwnerID || b.RunID != f.RunID || b.ToolCallID != f.CallID || b.TaskID != f.TaskID || b.TaskRevision != f.TaskRevision || b.ConstraintsRevision != f.ConstraintsRevision || b.ToolName != source.CheckToolName || b.Resource != source.CheckResource || b.ArgsHash != argsHash || b.Effect != actionauth.EffectRead || b.ExtensionID != p.Manifest.ID || b.ExtensionManifestHash != p.Digest || b.ExtensionGeneration < 1 || b.ExtensionAccessRevision < 1 || b.ExtensionAccessExpiresAt == nil || !b.ExtensionAccessExpiresAt.After(f.StartedAt) || b.ContractRevision != fmt.Sprintf("%s:%d", p.Digest, b.ExtensionGeneration) || !digest.MatchString(b.ContractHash) {
		return nil
	}
	claimedHash := b.ActionHash
	b.ActionHash = ""
	if hash(b) != claimedHash {
		return nil
	}
	var envelope struct {
		Status int    `json:"status_code"`
		Body   string `json:"body"`
	}
	if json.Unmarshal(f.Result, &envelope) != nil || envelope.Status != 200 || len(envelope.Body) > 64*1024 {
		return nil
	}
	// Reuse the core's bounded unique-key JSON decoder. Ambiguous duplicate
	// members cannot supply a different healthy/failure interpretation.
	if _, err := actionauth.ArgumentsHash(envelope.Body); err != nil {
		return nil
	}
	var check struct {
		Format    string `json:"observation_format"`
		Project   string `json:"project_ref"`
		Revision  string `json:"project_revision"`
		Passed    *bool  `json:"checks_passed"`
		ErrorKind string `json:"error_kind"`
		CheckID   string `json:"check_id"`
	}
	if json.Unmarshal([]byte(envelope.Body), &check) != nil || check.Format != "project_check_v1" || check.Project != source.ProjectRef || check.Revision != source.ProjectRevision || check.Passed == nil || !digest.MatchString(check.CheckID) || (!*check.Passed && !identifier.MatchString(check.ErrorKind)) || (*check.Passed && check.ErrorKind != "") {
		return nil
	}
	kind := "project_error"
	if *check.Passed {
		kind = "project_healthy"
	}
	resultHash, err := actionauth.ArgumentsHash(string(f.Result))
	if err != nil {
		return nil
	}
	return &Event{Kind: kind, ConditionDigest: hash([]string{source.ProjectRef, p.Digest, source.CheckToolName, check.CheckID, check.ErrorKind}), ResourceRevision: source.ProjectRevision, EvidenceDigest: hash([]string{"broker_project_check", f.RunID, f.CallID, b.ArgsHash, resultHash}), Confidence: 100, Importance: 70, ProducedByButler: f.Own, ObservedAttemptID: f.RunID, ObservedToolCallID: f.CallID, ObservedAt: f.At}
}

// Read only completed confirmed dispatches with historical owner review and
// current task/constraint/project revision. Omit raw run errors entirely.
const brokerFactsSQL = `SELECT r.run_id,c.tool_call_id,t.task_id,t.revision,t.constraints_revision,
 COALESCE(t.source_json->>'type'='initiative',false),c.started_at,c.finished_at,
 c.args,i.binding,c.result,
	 EXISTS(SELECT 1 FROM initiative_events e WHERE e.source_id=$7 AND e.event_json->>'observed_attempt_id'=r.run_id)
 FROM tool_calls c JOIN runs r ON r.run_id=c.run_id JOIN tasks t ON t.task_id=r.task_id
 JOIN action_intents i ON i.run_id=r.run_id AND i.tool_call_id=c.tool_call_id
 JOIN action_authorizations a ON a.action_id=i.action_id AND a.action_hash=i.action_hash AND a.state='dispatch_completed'
 JOIN action_effect_dispatches d ON d.authorization_id=a.authorization_id AND d.execution_action_id=i.action_id AND d.outcome='dispatch_completed'
 JOIN action_effects fx ON fx.action_id=i.effect_id AND fx.state='effect_confirmed'
 JOIN extension_reviews v ON v.manifest_hash=$5 AND v.owner_id=$1
 WHERE t.owner_id=$1 AND t.source_json->>'project_ref'=$2 AND t.source_json->>'project_revision'=$3
 AND r.task_revision=t.revision AND c.tool_name=$4 AND c.status='completed'
 AND c.finished_at>clock_timestamp()-INTERVAL '24 hours' AND c.finished_at>=c.started_at
 AND octet_length(c.result::text)<=65536 AND octet_length(c.args::text)<=4096
 AND i.binding->>'extension_manifest_hash'=$5 AND i.binding->>'resource'=$6
	AND i.binding->>'owner_id'=$1 AND i.binding->>'tool_name'=$4
	AND i.binding->>'action_hash'=i.action_hash`

func currentBrokerCondition(ctx context.Context, tx pgx.Tx, source Source, event Event) (bool, error) {
	if event.ObservedAttemptID == "" || event.ObservedToolCallID == "" || event.ResourceRevision != source.ProjectRevision {
		return false, nil
	}
	// All facts must still exist. A newer verified prepared correction or newer
	// actual passing check clears the old candidate, including after budget waits.
	p, err := extensions.NewStore(nil, "", nil, nil).ManifestByHashTx(ctx, tx, source.CheckManifestHash)
	if err != nil || !projectReadOperation(p, source.CheckToolName) {
		return false, nil
	}
	rows, err := tx.Query(ctx, brokerFactsSQL+` AND c.finished_at >= $8 ORDER BY c.finished_at DESC,c.tool_call_id DESC LIMIT 100`, source.OwnerID, source.ProjectRef, source.ProjectRevision, source.CheckToolName, source.CheckManifestHash, source.CheckResource, source.ID, event.ObservedAt)
	if err != nil {
		return false, err
	}
	found := false
	latest := true
	for rows.Next() {
		var f brokerFact
		if err = rows.Scan(&f.RunID, &f.CallID, &f.TaskID, &f.TaskRevision, &f.ConstraintsRevision, &f.Own, &f.StartedAt, &f.At, &f.Args, &f.Binding, &f.Result, &f.AlreadyObserved); err != nil {
			rows.Close()
			return false, err
		}
		e := f.event(source, p)
		if e == nil {
			continue
		}
		if latest && (e.Kind != "project_error" || e.ConditionDigest != event.ConditionDigest) {
			rows.Close()
			return false, nil
		}
		latest = false
		if f.RunID == event.ObservedAttemptID && f.CallID == event.ObservedToolCallID && e.ConditionDigest == event.ConditionDigest && e.EvidenceDigest == event.EvidenceDigest {
			found = true
		}
	}
	rows.Close()
	if err = rows.Err(); err != nil || !found {
		return false, err
	}
	var resolved bool
	err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM tasks h JOIN task_results z ON z.result_id=h.completion_result_id AND z.task_id=h.task_id
	 WHERE h.owner_id=$1 AND h.source_json->>'project_ref'=$2 AND h.source_json->>'project_revision'=$3
	 AND h.status='completed' AND z.verdict='verified' AND h.finished_at>=$4)`, source.OwnerID, source.ProjectRef, source.ProjectRevision, event.ObservedAt).Scan(&resolved)
	return !resolved, err
}

func (s *Store) currentConditionTx(ctx context.Context, tx pgx.Tx, source Source, event Event) (bool, error) {
	if source.ObserverKind == "broker_project" {
		if s.RunDataGuardTx == nil || s.RunDataGuardTx(ctx, tx, event.ObservedAttemptID) != nil {
			return false, nil
		}
	}
	return currentTaskCondition(ctx, tx, source, event)
}
