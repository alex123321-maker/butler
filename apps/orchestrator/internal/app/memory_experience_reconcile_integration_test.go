package app

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"

	approvals "github.com/butler/butler/apps/orchestrator/internal/approval"
	runservice "github.com/butler/butler/apps/orchestrator/internal/run"
	"github.com/butler/butler/apps/orchestrator/internal/session"
	"github.com/butler/butler/apps/orchestrator/internal/task"
	"github.com/butler/butler/internal/actionauth"
	"github.com/butler/butler/internal/budget"
	"github.com/butler/butler/internal/extensions"
	commonv1 "github.com/butler/butler/internal/gen/common/v1"
	"github.com/butler/butler/internal/memory/catalog"
	"github.com/butler/butler/internal/memory/memorytest"
	"github.com/butler/butler/internal/security/egress/egresstest"
	"github.com/butler/butler/internal/transport"
)

// This controlled provider exercises only the owning read and durable control
// boundary. Its authored final is never an original result or a native answer.
type experienceReconciliationControlProvider struct {
	experienceOriginalControlProvider
	url, result string
}

func (p *experienceReconciliationControlProvider) StartRun(ctx context.Context, q transport.StartRunRequest) (transport.EventStream, error) {
	if transport.CheckInferenceAuthority(ctx) != nil || transport.CheckInferenceDispatch(ctx) != nil {
		return nil, fmt.Errorf("current continuation inference authority required")
	}
	p.run, p.phase = q.Context.RunID, 1
	return p.emit("http.request", map[string]string{"method": "GET", "url": p.url}), nil
}

func (p *experienceReconciliationControlProvider) SubmitToolResult(ctx context.Context, q transport.SubmitToolResultRequest) (transport.EventStream, error) {
	if transport.CheckInferenceAuthority(ctx) != nil || transport.CheckInferenceDispatch(ctx) != nil || p.phase != 1 {
		return nil, fmt.Errorf("one exact authorized continuation read required")
	}
	var envelope struct {
		Status string          `json:"status"`
		Result json.RawMessage `json:"result"`
	}
	if json.Unmarshal([]byte(q.ToolResultJSON), &envelope) != nil || envelope.Status != "completed" || len(envelope.Result) == 0 {
		return nil, fmt.Errorf("actual continuation read failed")
	}
	// Preserve the actual Broker response bytes. PostgreSQL JSONB serialization
	// is not a substitute for the journal's captured runtime response digest.
	p.result, p.phase = string(envelope.Result), 2
	stream := make(chan transport.TransportEvent, 1)
	stream <- transport.NewAssistantFinalEvent(p.run, p.Name(), transport.AssistantFinal{Content: "Получен отдельный ответ чтения по исходному действию; исходная попытка не завершилась успешно.", Usage: &transport.Usage{InputTokens: 30, OutputTokens: 20, TotalTokens: 60}})
	close(stream)
	return stream, nil
}

func TestExperienceOriginalOwnerResumeReadReconcilesAbsentIntegration(t *testing.T) {
	pool := memorytest.Database(t)
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	owner := experienceNativeActor(ctx, "explicit-original-reconciliation-owner")
	dir := t.TempDir()
	anchor := filepath.Join(dir, "anchor")
	manifest := catalog.NewFileManifest(anchor)
	if err := manifest.Initialize(); err != nil {
		t.Fatal(err)
	}
	cat := catalog.New(pool, manifest)
	recipient, err := newExperienceRecipient(dir, experienceRecipientConfig{Version: "v1", Ready: "true", Files: map[string]string{"alpha": "Controlled original rejection\nready=false\n", "beta": "Other resource\nready=false\n"}})
	if err != nil {
		t.Fatal(err)
	}
	defer recipient.close()
	policy := experienceNativeSourcePolicy()
	if policy.Task.ModelTurns != 13 || policy.Day.ModelTurns != 640 {
		t.Fatal("prospective source/global allowance changed")
	}
	r := newExperienceNativeRuntime(t, pool, cat, anchor, dir, recipient, newExperienceNativeAddresses(t), policy, "Не повторяй неподтверждённое действие. После отдельного поручения владельца допустимо только чтение и сверка исходного эффекта.", experienceNativeArms[0], nil, time.Now().Add(time.Hour))
	id := experienceDigest("controlled-original03-owner-resume")
	record := r.createTask(t, owner, id, "Prepare the bounded current unapplied artifact", "alpha", "prepare_and_verify_exact_resource")
	r.acceptCurrentProfile(t, owner, record)
	original := &experienceOriginalControlProvider{ready: "maybe"}
	first := r.execute(t, ctx, record, id, "Inspect alpha and prepare the current requested artifact without applying it", original)
	if first.Err != nil || first.Result == nil || first.Result.ReceiptStatus != "blocked" || first.Result.ErrorCode != "effect_unknown" || original.phase != 2 {
		t.Fatal("actual 400 did not retain the original UNKNOWN boundary", first.Err, first.Result)
	}
	verifier := &skillEvidence{tasks: r.tasks, journal: r.journal, transcript: r.history, budgets: budget.NewLedger(pool), results: r.results, sources: cat, lessons: r.lessons}
	observation, parents, err := verifier.taskObservation(owner, experienceNativeOwner, first.Result.RunID, r.environment)
	if err != nil || observation.TerminalAttempt == nil {
		t.Fatal("original terminal observation unavailable", err)
	}
	originalObservation, _ := json.Marshal(observation)
	sealed, err := r.lessons.Seal(owner, observation, parents)
	if err != nil {
		t.Fatal("original terminal seal", err)
	}
	before, err := r.inputs.Get(ctx, observation.TerminalAttempt.InputID)
	if err != nil || before.Status != "blocked" || before.ErrorCode != "effect_unknown" {
		t.Fatal("actual owning failed input missing", err)
	}
	effects, err := r.journal.GetRunEvidence(ctx, first.Result.RunID)
	if err != nil {
		t.Fatal(err)
	}
	var target actionauth.EffectRecord
	for _, effect := range effects {
		if effect.EffectState == actionauth.StateUnknown {
			target, err = r.journal.GetEffect(ctx, effect.ActionID)
		}
	}
	if err != nil || target.ActionID == "" || target.Binding.ToolName != "extension.project.prepare-patch" || recipient.snapshot().Dispatches != 1 {
		t.Fatal("one actual original rejected dispatch required", err)
	}
	leases := session.NewPostgresLeaseManager(pool)
	approvalRepo := approvals.NewPostgresRepository(pool)
	approvalRepo.SetCheckpointHooks(r.inputs)
	controls := task.NewController(r.tasks, leases.Authority(), leases, runservice.NewPostgresRepository(pool), approvalRepo, r.inputs, r.history)
	lookup := strings.ReplaceAll(target.Policy.VerificationURL, "{idempotency_key}", target.IdempotencyKey)
	resumed, err := controls.Apply(owner, task.ControlRequest{TaskID: record.TaskID, ExpectedRevision: record.Revision, Kind: task.ControlResume, Instruction: "Выполни только отдельный GET по " + lookup + ". Сверь исходное действие; не повторяй prepare, commit или исходный запрос."})
	if err != nil || resumed.ContinuationInputID == "" || resumed.InputID == "" || resumed.Task.TaskID != record.TaskID || resumed.Task.Revision != record.Revision+1 || resumed.Task.ConstraintsRevision != record.ConstraintsRevision+1 {
		t.Fatal("ordinary owner Resume did not admit exact visible continuation", err)
	}
	unchanged, err := r.journal.GetEffect(ctx, target.ActionID)
	if err != nil || unchanged.State != actionauth.StateUnknown || len(unchanged.EvidenceRefs) != len(target.EvidenceRefs) {
		t.Fatal("owner Resume cleared the unresolved effect", err)
	}
	old, err := r.inputs.Get(ctx, before.InputID)
	if err != nil || old.RunID != first.Result.RunID || old.Status != "cancelled" || old.ErrorCode != "owner_control" || old.ErrorMessage != resumed.ControlID || old.Attempts != before.Attempts || old.Checkpoint != before.Checkpoint || old.CheckpointJSON != before.CheckpointJSON {
		t.Fatal("owner control did not preserve the original attempt checkpoint", err)
	}
	var linked bool
	if err = pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM task_control_events c JOIN task_control_targets t USING(control_id) JOIN run_state_transitions s ON s.metadata_json->>'control_id'=c.control_id WHERE c.control_id=$1 AND c.owner_id=$2 AND c.kind='resume' AND c.task_id=$3 AND t.task_id=$3 AND c.revision_before=$4 AND c.revision_after=$4+1 AND t.revision_before=$4 AND t.revision_after=$4+1 AND c.result_json->>'continuation_input_id'=$5 AND s.run_id=$6 AND s.from_state='tool_running' AND s.to_state='cancelled' AND s.triggered_by='owner')`, resumed.ControlID, experienceNativeOwner, record.TaskID, record.Revision, resumed.ContinuationInputID, first.Result.RunID).Scan(&linked); err != nil || !linked {
		t.Fatal("actual original-control-revision-continuation provenance missing", err)
	}
	if _, err = cat.Get(owner, sealed.ID); err == nil {
		t.Fatal("old terminal snapshot silently rebased onto resumed task")
	}
	access := extensions.TaskAccess{TaskID: resumed.Task.TaskID, TaskRevision: resumed.Task.Revision, ConstraintsRevision: resumed.Task.ConstraintsRevision, ManifestHash: r.pack.Digest, Generation: r.generation, Tools: []string{"extension.project.inspect", "extension.project.check"}, Resources: []actionauth.ResourceScope{{Origin: egresstest.Origin, PathPrefix: "/project/alpha"}}, ExpiresAt: r.expires}
	currentAccess, err := r.extensions.Control(experienceNativeActor(ctx, "read-current-extension-access"), extensions.ControlRequest{Operation: "task_access", ExtensionID: "project", Access: extensions.TaskAccess{TaskID: record.TaskID}})
	if err != nil || currentAccess.Access == nil || currentAccess.Access.TaskRevision != record.Revision {
		t.Fatal("actual old extension grant missing", err)
	}
	access.Revision = currentAccess.Access.Revision
	if grant, e := r.extensions.Control(experienceNativeActor(ctx, "explicit-fresh-readonly-grant"), extensions.ControlRequest{Operation: "grant_task", ExtensionID: "project", Access: access}); e != nil || grant.Error != "" {
		t.Fatal("fresh read-only extension grant denied", e)
	}
	read := &experienceReconciliationControlProvider{url: lookup}
	second := r.executeQueued(t, ctx, resumed.Task, resumed.ContinuationInputID, read)
	if second.Err != nil || second.Result == nil || second.Result.CurrentState != commonv1.RunState_RUN_STATE_COMPLETED || second.Result.RunID == first.Result.RunID || read.phase != 2 || read.result == "" {
		t.Fatal("exact read-only owner continuation failed", second.Err, second.Result)
	}
	readEffects, err := r.journal.GetRunEvidence(ctx, second.Result.RunID)
	if err != nil || len(readEffects) != 1 || readEffects[0].EffectState != actionauth.StateConfirmed {
		t.Fatal("separate actual read effect missing", err)
	}
	readEffect, err := r.journal.GetEffect(ctx, readEffects[0].ActionID)
	if err != nil || readEffect.Binding.Effect != actionauth.EffectRead || readEffect.Binding.ToolName != "http.request" || readEffect.Binding.Resource != lookup || readEffect.Binding.TaskID != target.Binding.TaskID || readEffect.Binding.TaskRevision != resumed.Task.Revision {
		t.Fatal("lookup borrowed old or unrelated authority", err)
	}
	if _, err = r.reconcile(ctx, target.ActionID, readEffect.AuthorizationID, read.result+" "); err == nil {
		t.Fatal("reconciliation accepted a response without the exact captured digest")
	}
	reconciled, err := r.reconcile(ctx, target.ActionID, readEffect.AuthorizationID, read.result)
	if err != nil || reconciled.EffectState != actionauth.StateAbsent || reconciled.ActionID != target.ActionID {
		t.Fatal("actual authorized lookup did not reconcile ABSENT", err)
	}
	if _, err = r.results.GetByRun(owner, first.Result.RunID); err == nil {
		t.Fatal("read continuation fabricated an original completion")
	}
	after, _ := json.Marshal(observation)
	if string(after) != string(originalObservation) || recipient.snapshot().Dispatches != 1 {
		t.Fatal("original observation changed or rejected action was retried")
	}
	var unknown, runs, modelTurns int
	if err = pool.QueryRow(ctx, `SELECT count(*) FROM tool_calls WHERE run_id=$1 AND status='effect_unknown'`, first.Result.RunID).Scan(&unknown); err != nil || unknown != 1 {
		t.Fatal("historical UNKNOWN tool outcome changed", err)
	}
	if err = pool.QueryRow(ctx, `SELECT count(*) FROM runs WHERE task_id=$1`, record.TaskID).Scan(&runs); err != nil || runs != 2 {
		t.Fatal("visible two-run case did not preserve exact cardinality", err)
	}
	if err = pool.QueryRow(ctx, `SELECT count(*) FROM execution_model_attempts m JOIN runs r USING(run_id) WHERE r.task_id=$1`, record.TaskID).Scan(&modelTurns); err != nil || modelTurns != 4 || modelTurns > 12 {
		t.Fatal("original and read continuation did not share the foreground bound", err, modelTurns)
	}
}
