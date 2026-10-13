package app

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/butler/butler/apps/orchestrator/internal/completion"
	durableinput "github.com/butler/butler/apps/orchestrator/internal/input"
	"github.com/butler/butler/internal/generatedtools"
	"github.com/butler/butler/internal/ownership"
	"github.com/jackc/pgx/v5"
)

// The loader receives a genuine confirmed activation through the ordinary
// action journal, current lease/private input and durable 055 continuation.
// This is a model-free storage/Broker boundary test, not a native observation.
func TestConverterContinuationActivatedCurrentLoaderRecoveryIntegration(t *testing.T) {
	c := newContinuationFixture(t, 0)
	ctx, event, candidateInput := c.claimParent(t)
	if got, err := c.service.LoadConverterContinuation(ctx, event); err != nil || got.Purpose != generatedtools.CandidateReturn || got.Activation != nil {
		t.Fatal("candidate phase", err)
	}
	f := c.f
	base := f.f.ctx
	f.f.ctx = ctx
	if _, err := f.f.pool.Exec(ctx, `UPDATE runs SET status='tool_running',current_state='tool_running',autonomy_mode='mode_2' WHERE run_id='continuation-parent-worker'`); err != nil {
		t.Fatal(err)
	}
	collected, _, _ := f.dispatch(t, "continuation-parent-worker", "converters.collect", map[string]any{"child_task_id": c.child.TaskID})
	var candidate generatedtools.Record
	if json.Unmarshal([]byte(collected.ResultJson), &candidate) != nil || candidate.Generation != 1 {
		t.Fatal("immutable candidate snapshot")
	}
	f.dispatch(t, "continuation-parent-worker", "converters.test", map[string]any{"digest": candidate.Digest})
	f.dispatch(t, "continuation-parent-worker", "converters.trial", map[string]any{"digest": candidate.Digest, "input_ref": f.document})
	_, prepared, _ := f.dispatch(t, "continuation-parent-worker", "converters.activate", map[string]any{"digest": candidate.Digest, "generation": candidate.Generation})
	var activationEffect string
	if err := f.f.pool.QueryRow(ctx, `SELECT effect_id FROM action_intents WHERE action_id=$1`, prepared.Binding.ActionID).Scan(&activationEffect); err != nil {
		t.Fatal(err)
	}
	if _, err := f.f.pool.Exec(ctx, `UPDATE runs SET status='finalizing',current_state='finalizing' WHERE run_id='continuation-parent-worker'`); err != nil {
		t.Fatal(err)
	}
	finalizer := completion.New(f.f.pool)
	finalizer.Evidence = effectEvidence{journal: f.journal}
	finalizer.DataGuard = func(current context.Context, tx pgx.Tx, id string) error {
		if err := f.f.sources.CheckRunSourcesTx(current, tx, id); err != nil {
			return err
		}
		return generatedtools.CheckContinuationRunTx(current, tx, f.f.sources, id)
	}
	if _, err := finalizer.Save(ctx, completion.Request{RunID: "continuation-parent-worker", SessionKey: f.f.parent.SessionKey, TaskID: f.f.parent.TaskID, TaskRevision: f.f.parent.Revision, ConstraintsRevision: f.f.parent.ConstraintsRevision, Content: "Confirmed activation; original goal still requires its own result", AnswerState: "final"}); err != nil {
		t.Fatal(err)
	}
	if err := c.inputs.Finish(base, candidateInput, "completed", "", "", 0); err != nil {
		t.Fatal(err)
	}
	tok, _ := ownership.TokenFromContext(ctx)
	if _, err := f.f.leases.ReleaseLease(base, tok.LeaseID); err != nil {
		t.Fatal(err)
	}
	f.f.ctx = base
	var activationID string
	if err := f.f.pool.QueryRow(base, `SELECT continuation_id FROM converter_parent_continuations WHERE child_task_id=$1 AND purpose='activated_catalog'`, c.child.TaskID).Scan(&activationID); err != nil {
		t.Fatal(err)
	}
	if created, err := c.service.Enqueue(base, f.f.parent.TaskID, activationID); err != nil || !created {
		t.Fatal("activated enqueue", err)
	}
	r, err := c.inputs.Claim(base, time.Minute)
	if err != nil || r.Source != "converter-return" {
		t.Fatal("activated claim", err)
	}
	owned := f.f.leasedWorker(t, f.f.parent, "activated-parent-worker")
	event = r.Event
	event.IdempotencyKey = r.InputID
	if _, err = f.f.pool.Exec(base, `UPDATE runs SET idempotency_key=$1,input_event_id=$2 WHERE run_id='activated-parent-worker'`, r.InputID, event.EventID); err != nil {
		t.Fatal(err)
	}
	claimed := durableinput.WithClaim(owned, r)
	if err = c.inputs.SaveCheckpoint(claimed, "activated-parent-worker", "run_created", "{}"); err != nil {
		t.Fatal(err)
	}
	var revision int64
	if err = f.f.pool.QueryRow(base, `SELECT source_revision FROM transcript_heads WHERE session_key=$1`, f.f.parent.SessionKey).Scan(&revision); err != nil {
		t.Fatal(err)
	}
	if err = f.f.sources.PinTranscript(owned, f.f.parent.SessionKey, revision); err != nil {
		t.Fatal(err)
	}
	restarted, err := NewConverterContinuations(f.f.pool, f.f.store, c.inputs, f.f.sources, f.f.budgets, c.service.allowance, f.profile)
	if err != nil {
		t.Fatal(err)
	}
	for _, service := range []*ConverterContinuations{c.service, restarted} {
		got, e := service.LoadConverterContinuation(claimed, event)
		if e != nil || got.Purpose != generatedtools.ActivatedCatalog || got.Activation == nil || got.Activation.ConverterID != candidate.ID || got.Activation.Digest != candidate.Digest || got.Activation.Generation != 2 || got.Activation.ActionID != prepared.Binding.ActionID || got.Activation.EffectID != activationEffect {
			t.Fatal("current activated generation/confirmed receipt/recovery", e)
		}
	}
	currentToken, _ := ownership.TokenFromContext(claimed)
	currentToken.FenceEpoch++
	stale := ownership.WithToken(claimed, f.f.leases.Authority(), currentToken)
	if _, err = restarted.LoadConverterContinuation(stale, event); err == nil {
		t.Fatal("stale current authority recovered cached activation")
	}
	f.deleteSource(t, f.document)
	if _, err = restarted.LoadConverterContinuation(claimed, event); err == nil {
		t.Fatal("deleted original source recovered cached activation")
	}
}
