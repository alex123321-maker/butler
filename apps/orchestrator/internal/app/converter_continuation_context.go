package app

import (
	"context"
	"encoding/json"

	durableinput "github.com/butler/butler/apps/orchestrator/internal/input"
	flow "github.com/butler/butler/apps/orchestrator/internal/orchestrator"
	"github.com/butler/butler/internal/actionauth"
	"github.com/butler/butler/internal/budget"
	"github.com/butler/butler/internal/generatedtools"
	"github.com/butler/butler/internal/ingress"
	"github.com/butler/butler/internal/memory/catalog"
	"github.com/butler/butler/internal/memory/sanitize"
	"github.com/butler/butler/internal/ownership"
	"github.com/jackc/pgx/v5"
)

func (s *ConverterContinuations) LoadConverterContinuation(ctx context.Context, event ingress.InputEvent) (flow.ConverterContinuationContext, error) {
	var out flow.ConverterContinuationContext
	var binding durableinput.ConverterContinuationBinding
	tok, owned := ownership.TokenFromContext(ctx)
	if !owned || event.Source != "converter-return" || generatedtools.Decode([]byte(event.PayloadJSON), &binding) != nil || binding.ID == "" || binding.TaskID == "" {
		return out, generatedtools.ErrAccess
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return out, err
	}
	defer tx.Rollback(ctx)
	// Unlocked routing identity only. The private verifier locks current ownership,
	// session, run and task before the immutable055/current authority/source rows.
	var c generatedtools.Continuation
	c.Intent.ID, c.Intent.ParentTaskID = binding.ID, binding.TaskID
	if tx.QueryRow(ctx, `SELECT owner_id,parent_revision,constraints_revision,input_id,state,deadline FROM converter_parent_continuations WHERE continuation_id=$1 AND parent_task_id=$2 AND state='queued'`, binding.ID, binding.TaskID).Scan(&c.Intent.OwnerID, &c.Intent.ParentRevision, &c.Intent.ConstraintsRevision, &c.InputID, &c.State, &c.Intent.Deadline) != nil {
		return out, generatedtools.ErrAccess
	}
	if err = s.inputs.VerifyConverterContinuationTx(ctx, tx, event, c); err != nil {
		return out, err
	}
	c, err = generatedtools.ReadContinuationTx(ctx, tx, binding.ID)
	if err != nil {
		return out, err
	}
	snapshot, p, err := generatedtools.CheckContinuationTx(ctx, tx, s.sources, c)
	if err != nil {
		return out, err
	}
	if err = s.checkSourcesTx(ctx, tx, c, p); err != nil {
		return out, err
	}
	// Pin the complete original/past-parent DATA graph into this actual owned run.
	// This is ordinary Catalog access under its lease, never a synthesized actor.
	refs := append(append([]catalog.SourceRef(nil), c.Intent.OriginalSources...), c.Ready.Sources...)
	if _, err = s.sources.AccessTx(ctx, tx, refs, "memory_read"); err != nil {
		return out, err
	}
	out = flow.ConverterContinuationContext{RunID: tok.RunID, SessionKey: tok.SessionKey, TaskID: c.Intent.ParentTaskID, ID: c.Intent.ID, Purpose: c.Intent.Purpose, ChildTaskID: c.Intent.ChildTaskID}
	if c.Intent.Purpose == generatedtools.ActivatedCatalog {
		// CheckContinuationTx above verified the exact confirmed activation and
		// current installed generation. Candidate.Generation is an older snapshot.
		out.Activation = &flow.ConverterContinuationActivation{ConverterID: p.Candidate.ID, Digest: p.Candidate.Digest, Generation: c.Ready.Generation, ActionID: c.Ready.ActivationActionID, EffectID: c.Ready.ActivationEffectID}
		out.PriorRunID = c.Ready.PriorRunID
		out.ConfirmedPriorActions, err = converterContinuationActionsTx(ctx, tx, c, snapshot.Binding)
		if err != nil {
			return flow.ConverterContinuationContext{}, err
		}
	}
	// Recheck the private claim/current authority and complete prior source graph
	// after receipt reads, including source correction while those reads waited.
	if err = s.inputs.VerifyConverterContinuationTx(ctx, tx, event, c); err != nil {
		return flow.ConverterContinuationContext{}, err
	}
	if _, _, err = generatedtools.CheckContinuationTx(ctx, tx, s.sources, c); err != nil {
		return flow.ConverterContinuationContext{}, err
	}
	if err = s.checkSourcesTx(ctx, tx, c, p); err != nil {
		return flow.ConverterContinuationContext{}, err
	}
	return out, tx.Commit(ctx)
}

// The checked055 identity selects one completed ROOT attempt, not arbitrary
// session history or child reports. No model/provider context or execution is
// replayed. The result digest uses the existing journal canonical-result
// contract; original authorization fingerprints remain untouched.
func converterContinuationActionsTx(ctx context.Context, tx pgx.Tx, c generatedtools.Continuation, expected actionauth.ActionBinding) ([]flow.ConverterContinuationAction, error) {
	if c.Intent.Purpose != generatedtools.ActivatedCatalog || c.Ready.PriorRunID == "" || c.Ready.PriorRunID == c.Intent.OriginalRunID {
		return nil, generatedtools.ErrAccess
	}
	var incomplete bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM action_intents i LEFT JOIN tool_calls tc ON tc.run_id=i.run_id AND tc.tool_call_id=i.tool_call_id WHERE i.run_id=$1 AND tc.tool_call_id IS NULL)`, c.Ready.PriorRunID).Scan(&incomplete); err != nil {
		return nil, err
	} else if incomplete {
		return nil, generatedtools.ErrAccess
	}
	rows, err := tx.Query(ctx, `SELECT tc.run_id,tc.tool_call_id,tc.tool_name,tc.args::text,tc.result::text,

 COALESCE(i.binding,'{}'::jsonb),COALESCE(i.action_id,''),COALESCE(i.effect_id,''),
 tc.status='completed' AND tc.finished_at IS NOT NULL AND COALESCE(tc.error,'{}'::jsonb)='{}'::jsonb
 AND r.current_state='completed' AND r.task_id=$2 AND r.task_revision=$4
 AND e.state='effect_confirmed' AND e.dispatch_outcome='dispatch_completed'
 AND a.state='dispatch_completed' AND a.finished_at IS NOT NULL
 AND d.outcome='dispatch_completed' AND d.execution_action_id=i.action_id AND d.finished_at IS NOT NULL,
 COALESCE((SELECT ev.evidence->>'normalized_result_digest' FROM action_effect_evidence ev
 WHERE ev.action_id=i.effect_id AND ev.source_action_id=i.action_id
 AND ev.evidence->>'source'='runtime_response' AND ev.evidence->>'verdict'='effect_confirmed'
 AND ev.evidence->>'source_run_id'=tc.run_id AND ev.evidence->>'source_tool_call_id'=tc.tool_call_id
 ORDER BY ev.recorded_at DESC,ev.evidence_id DESC LIMIT 1),'')
 FROM tool_calls tc JOIN runs r ON r.run_id=tc.run_id
 LEFT JOIN action_intents i ON i.run_id=tc.run_id AND i.tool_call_id=tc.tool_call_id
 LEFT JOIN action_effects e ON e.action_id=i.effect_id
 LEFT JOIN action_authorizations a ON a.action_id=i.action_id
 LEFT JOIN action_effect_dispatches d ON d.authorization_id=a.authorization_id AND d.action_id=i.effect_id
 WHERE tc.run_id=$1 AND r.task_id=$2 AND r.session_key=(SELECT session_key FROM tasks WHERE task_id=$2 AND owner_id=$3)
 ORDER BY tc.started_at,tc.tool_call_id LIMIT 33`, c.Ready.PriorRunID, c.Intent.ParentTaskID, c.Intent.OwnerID, c.Intent.ParentRevision)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []flow.ConverterContinuationAction
	bytes := 0
	for rows.Next() {
		var observation flow.ConverterContinuationAction
		var args, result, binding []byte
		var valid *bool
		var digest string
		if err = rows.Scan(&observation.RunID, &observation.ToolCallID, &observation.ToolName, &args, &result, &binding, &observation.ActionID, &observation.EffectID, &valid, &digest); err != nil {
			return nil, err
		}
		bytes += len(args) + len(result) + len(binding)
		if len(out) >= 32 || bytes > 256<<10 {
			return nil, generatedtools.ErrAccess
		}
		var b actionauth.ActionBinding
		actualDigest, digestErr := budget.NormalizeResult(string(result))
		if valid == nil || !*valid || json.Unmarshal(binding, &b) != nil ||
			b.ActionID != observation.ActionID || b.RunID != observation.RunID || b.ToolCallID != observation.ToolCallID || b.ToolName != observation.ToolName ||
			b.OwnerID != c.Intent.OwnerID || b.TaskID != c.Intent.ParentTaskID || b.TaskRevision != c.Intent.ParentRevision || b.ConstraintsRevision != c.Intent.ConstraintsRevision ||
			b.GoalID != expected.GoalID || b.GoalRevision != expected.GoalRevision || b.MandateID != expected.MandateID || b.MandateRevision != expected.MandateRevision || b.OwnerPolicyRevision != expected.OwnerPolicyRevision || b.Purpose != expected.Purpose ||
			b.ArgsHash == "" || digestErr != nil || digest == "" || digest != actualDigest || !json.Valid(args) || !json.Valid(result) {
			return nil, generatedtools.ErrAccess
		}
		observation.ArgsHash = b.ArgsHash
		observation.Effect = b.Effect
		if b.Effect != actionauth.EffectRead && b.Effect != actionauth.EffectPrepare && b.Effect != actionauth.EffectCommit {
			return nil, generatedtools.ErrAccess
		}
		// PostgreSQL JSONB can change numeric spelling. The original journal hash
		// remains exact; an unverifiable argument projection is omitted, not
		// promoted into proof or used to reconstruct/dispatch a tool call.
		if hash, e := actionauth.ArgumentsHash(string(args)); e == nil && hash == b.ArgsHash {
			observation.EquivalentArgsHash, err = budget.NormalizedArgumentsHash(string(args))
			if err != nil {
				return nil, generatedtools.ErrAccess
			}
			observation.Args = json.RawMessage(sanitize.TranscriptToolArgsJSON(string(args)))
		}
		observation.Result = json.RawMessage(sanitize.TranscriptToolResultJSON(string(result)))
		out = append(out, observation)
	}
	if err = rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}
