package app

import (
	"context"
	"testing"

	"github.com/butler/butler/internal/actionauth"
	"github.com/butler/butler/internal/owneridentity"
	"github.com/jackc/pgx/v5/pgxpool"
)

// The recipient rejects an actually dispatched unsupported value. Its HTTP400
// cannot establish absence through this ABI, so the owning effect stays UNKNOWN.
// No recipient-side verdict is injected into the model, journal or working state.
type coherentUnknownEffect struct {
	RunID, InputID, TaskID, ActionID string
	Dispatches                       int
}

func coherentActualUnknown(t *testing.T, p *coherentPilot, r *experienceNativeRuntime, recipient *experienceRecipient) coherentUnknownEffect {
	t.Helper()
	id := experienceDigest("coherent-actual-unknown-original")
	record := r.createTask(t, p.ctx, id, "Prepare only the current bounded alpha artifact; an unconfirmed effect must stop without replay", "alpha", "prepare_and_verify_exact_resource")
	provider := &coherentExperienceOriginal{experienceOriginalControlProvider: &experienceOriginalControlProvider{ready: "maybe"}, preference: p.provider.source.Body}
	before := recipient.snapshot().Dispatches
	out := r.execute(t, owneridentity.WithActor(p.ctx, owneridentity.Actor{}), record, id, "Inspect the current alpha and prepare its requested unapplied artifact. Stop on any unknown effect; do not retry or claim success.", provider)
	if out.Err != nil || out.Result == nil || out.Result.ReceiptStatus != "blocked" || out.Result.ErrorCode != "effect_unknown" || provider.phase != 2 || len(provider.requests) != 1 {
		t.Fatal("coherent actual dispatched rejection did not stop at UNKNOWN", out.Err)
	}
	var inputID string
	if err := p.pool.QueryRow(p.ctx, `SELECT input_id FROM accepted_inputs WHERE run_id=$1`, out.Result.RunID).Scan(&inputID); err != nil {
		t.Fatal(err)
	}
	receipt, err := r.inputs.Get(p.ctx, inputID)
	if err != nil || receipt.Status != "blocked" || receipt.ErrorCode != "effect_unknown" {
		t.Fatal("coherent UNKNOWN lacks its owning input checkpoint", err)
	}
	effects, err := r.journal.GetRunEvidence(p.ctx, out.Result.RunID)
	if err != nil || len(effects) != 2 {
		t.Fatal("coherent UNKNOWN did not retain inspect and prepare effects", err)
	}
	var target actionauth.EffectRecord
	for _, e := range effects {
		if e.EffectState == actionauth.StateUnknown {
			target, err = r.journal.GetEffect(p.ctx, e.ActionID)
		}
	}
	if err != nil || target.ActionID == "" || target.Binding.TaskID != record.TaskID || target.Binding.ToolName != "extension.project.prepare-patch" || target.State != actionauth.StateUnknown {
		t.Fatal("coherent UNKNOWN lacks its actual current action binding", err)
	}
	if _, err = r.results.GetByRun(p.ctx, out.Result.RunID); err == nil {
		t.Fatal("coherent unknown original invented a final result")
	}
	after := recipient.snapshot().Dispatches
	if after != before+1 {
		t.Fatal("coherent unknown original repeated the recipient dispatch")
	}
	witness := coherentUnknownEffect{RunID: out.Result.RunID, InputID: inputID, TaskID: record.TaskID, ActionID: target.ActionID, Dispatches: after}
	witness.check(t, p.ctx, p.pool)
	p.record("actual_broker_runtime_unknown_effect_stops_without_replay_or_final", &receipt, map[string]any{"witness": witness, "effect": target, "actual_start_requests": provider.requests, "native_calls": 0})
	return witness
}

func (w coherentUnknownEffect) check(t *testing.T, ctx context.Context, pool *pgxpool.Pool) {
	t.Helper()
	var state, toolStatus string
	var finals, calls int
	if err := pool.QueryRow(ctx, `SELECT e.state,c.status,
 (SELECT count(*) FROM run_final_results WHERE run_id=$2),
 (SELECT count(*) FROM tool_calls WHERE run_id=$2 AND tool_name='extension.project.prepare-patch')
 FROM action_effects e JOIN action_intents i ON i.effect_id=e.action_id
 JOIN tool_calls c USING(tool_call_id)
 WHERE e.action_id=$1 AND i.run_id=$2 AND i.binding->>'task_id'=$3`, w.ActionID, w.RunID, w.TaskID).Scan(&state, &toolStatus, &finals, &calls); err != nil || state != string(actionauth.StateUnknown) || toolStatus != "effect_unknown" || finals != 0 || calls != 1 {
		t.Fatal("coherent original UNKNOWN was cleared, replayed or made successful", state, toolStatus, finals, calls, err)
	}
}
