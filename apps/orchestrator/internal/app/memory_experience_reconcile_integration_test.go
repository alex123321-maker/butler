package app

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/butler/butler/apps/orchestrator/internal/api"
	approvals "github.com/butler/butler/apps/orchestrator/internal/approval"
	runservice "github.com/butler/butler/apps/orchestrator/internal/run"
	"github.com/butler/butler/apps/orchestrator/internal/session"
	"github.com/butler/butler/apps/orchestrator/internal/task"
	"github.com/butler/butler/internal/actionauth"
	"github.com/butler/butler/internal/budget"
	"github.com/butler/butler/internal/extensions"
	commonv1 "github.com/butler/butler/internal/gen/common/v1"
	"github.com/butler/butler/internal/memory/catalog"
	"github.com/butler/butler/internal/memory/experience"
	"github.com/butler/butler/internal/memory/jobs"
	"github.com/butler/butler/internal/memory/memorytest"
	"github.com/butler/butler/internal/security/egress/egresstest"
	"github.com/butler/butler/internal/transport"
	"github.com/jackc/pgx/v5/pgxpool"
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
	for _, scenario := range []string{"accepted", "corrected_original_tool", "late_control", "late_original_link", "late_continuation_input", "late_lookup", "late_effect", "late_witness", "late_parent", "late_pause", "fence_control", "fence_input", "fence_effect", "control_tombstone"} {
		t.Run(scenario, func(t *testing.T) { experienceOriginalOwnerResumeRead(t, scenario) })
	}
}

func experienceOriginalOwnerResumeRead(t *testing.T, scenario string) {
	t.Helper()
	var pool *pgxpool.Pool
	if scenario == "backup_seed" {
		pool = experienceReconciledBackupPool(t, true)
	} else {
		pool = memorytest.Database(t)
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	owner := experienceNativeActor(ctx, "explicit-original-reconciliation-owner")
	dir := t.TempDir()
	anchor := filepath.Join(dir, "anchor")
	if scenario == "backup_seed" {
		anchor = os.Getenv("BUTLER_TEST_EXPERIENCE_RECONCILED_ANCHOR")
		if !filepath.IsAbs(anchor) {
			t.Fatal("private independent reconciliation anchor required")
		}
	}
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
	observation, _, err := verifier.taskObservation(owner, experienceNativeOwner, first.Result.RunID, r.environment)
	if err != nil || observation.TerminalAttempt == nil {
		t.Fatal("original terminal observation unavailable", err)
	}
	originalObservation, _ := json.Marshal(observation)
	// This is the same owner-authenticated production operation required before
	// Resume. It creates no note policy, job, reservation or model attempt.
	sealToken := strings.Repeat("controlled-owner-observation-", 3)
	policies := configureExperiencePolicies(r.lessons, nil, r.tasks, r.history, r.results, cat)
	if policies == nil {
		t.Fatal("production observation creator depends on model jobs")
	}
	sealServer := httptest.NewServer((&api.MemoryCatalogServer{Store: cat, Auth: api.NewOwnerControlAuth(sealToken, experienceNativeOwner), ExperiencePolicies: policies}).Handle())
	defer sealServer.Close()
	sealRequest := func(endpoint, token, method, body, delivery string, want int) []byte {
		t.Helper()
		request, e := http.NewRequestWithContext(ctx, method, endpoint+"/api/v4/memory/experience-observations", strings.NewReader(body))
		if e != nil {
			t.Fatal(e)
		}
		request.Header.Set("Authorization", "Bearer "+token)
		request.Header.Set("X-Butler-Delivery-ID", delivery)
		response, e := http.DefaultClient.Do(request)
		if e != nil {
			t.Fatal("owner observation endpoint unavailable")
		}
		defer response.Body.Close()
		raw, e := io.ReadAll(io.LimitReader(response.Body, 65536))
		if e != nil || response.StatusCode != want {
			t.Fatal("owner observation response", response.StatusCode, want, e)
		}
		return raw
	}
	sealBody, _ := json.Marshal(map[string]string{"run_id": first.Result.RunID})
	sealRequest(sealServer.URL, "unauthenticated", http.MethodPost, string(sealBody), "bad-auth", http.StatusUnauthorized)
	sealRequest(sealServer.URL, sealToken, http.MethodGet, string(sealBody), "bad-method", http.StatusMethodNotAllowed)
	sealRequest(sealServer.URL, sealToken, http.MethodPost, string(sealBody), "", http.StatusBadRequest)
	sealRequest(sealServer.URL, sealToken, http.MethodPost, strings.TrimSuffix(string(sealBody), "}")+`,"owner_id":"other","body":"invented"}`, "bad-fields", http.StatusBadRequest)
	other := httptest.NewServer((&api.MemoryCatalogServer{Store: cat, Auth: api.NewOwnerControlAuth(sealToken, "other-owner"), ExperiencePolicies: policies}).Handle())
	defer other.Close()
	sealRequest(other.URL, sealToken, http.MethodPost, string(sealBody), "wrong-owner", http.StatusConflict)
	sealRequest(sealServer.URL, sealToken, http.MethodPost, `{"run_id":"missing-original-run"}`, "missing-run", http.StatusConflict)
	var sealed catalog.SourceRef
	if raw := sealRequest(sealServer.URL, sealToken, http.MethodPost, string(sealBody), "seal-original-before-resume", http.StatusCreated); json.Unmarshal(raw, &sealed) != nil || sealed.ID == "" || sealed.Hash == "" || bytes.Contains(raw, []byte("terminal_attempt")) || bytes.Contains(raw, []byte("Controlled original rejection")) {
		t.Fatal("seal response must contain only a content-free source receipt")
	}
	var repeated catalog.SourceRef
	if raw := sealRequest(sealServer.URL, sealToken, http.MethodPost, string(sealBody), "same-original-idempotent-seal", http.StatusCreated); json.Unmarshal(raw, &repeated) != nil || repeated != sealed {
		t.Fatal("unchanged explicit observation did not reuse its seal")
	}
	var notes int
	if err = pool.QueryRow(ctx, `SELECT (SELECT count(*) FROM memory_model_policies)+(SELECT count(*) FROM memory_aux_jobs)+(SELECT count(*) FROM memory_model_attempts)`).Scan(&notes); err != nil || notes != 0 {
		t.Fatal("seal-only owner operation created background authorization or spending", err)
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
	selected := experience.ReconciliationRequest{OriginalSnapshotID: sealed.ID, ControlID: resumed.ControlID}
	if scenario == "corrected_original_tool" {
		if _, err := pool.Exec(ctx, `UPDATE tool_calls SET result='{"corrected_original":true}'::jsonb WHERE tool_call_id=$1`, observation.ToolPins[0].ID); err != nil {
			t.Fatal(err)
		}
		if _, _, err := verifier.reconciledTaskObservation(owner, experienceNativeOwner, first.Result.RunID, selected, r.environment); err == nil {
			t.Fatal("post-Resume projection borrowed a corrected original tool row")
		}
		var attempts int
		if err := pool.QueryRow(ctx, `SELECT count(*) FROM memory_model_attempts`).Scan(&attempts); err != nil || attempts != 0 {
			t.Fatal("invalid original witness created a background attempt", err)
		}
		return
	}
	for _, invalid := range []experience.ReconciliationRequest{{OriginalSnapshotID: "missing-witness", ControlID: resumed.ControlID}, {OriginalSnapshotID: sealed.ID, ControlID: "missing-owner-control"}} {
		if _, _, e := verifier.reconciledTaskObservation(owner, experienceNativeOwner, first.Result.RunID, invalid, r.environment); e == nil {
			t.Fatal("reconciled projection accepted a missing witness/control")
		}
	}
	if _, _, e := verifier.reconciledTaskObservation(owner, experienceNativeOwner, second.Result.RunID, selected, r.environment); e == nil {
		t.Fatal("read final was accepted as the stopped original")
	}
	if _, _, e := verifier.reconciledTaskObservation(ctx, experienceNativeOwner, first.Result.RunID, selected, r.environment); e == nil {
		t.Fatal("reconciled projection accepted a caller without owner authority")
	}
	supplemented, supplementParents, err := verifier.reconciledTaskObservation(owner, experienceNativeOwner, first.Result.RunID, selected, r.environment)
	if err != nil || supplemented.ReconciledAttempt == nil || supplemented.TerminalAttempt == nil || supplemented.ResultID != "" || supplemented.AgentReport != "" || supplemented.OwnerVerdict != "unknown" || supplemented.TaskRevision != record.Revision || supplemented.ReconciledAttempt.CurrentTaskRevision != resumed.Task.Revision || supplemented.ReconciledAttempt.CurrentEffect != actionauth.StateAbsent || len(supplementParents) < 3 {
		t.Fatal("separate reconciled projection lost original or current provenance", err)
	}
	jobStore := jobs.New(pool, cat, r.budgets, nil)
	noteProvider := &experienceTerminalControlProvider{note: experience.Notes{Summary: "The original rejected attempt remained UNKNOWN; a separately authorized owner read later proved ABSENT", When: []string{"This exact observed environment and current owner scope"}, NotApplicable: []string{"No original final or independent successful task verification"}, Steps: []experience.Step{{ActionID: target.ActionID, Comment: "The original failed action is distinct from the later read"}}}}
	worker := &jobs.ModelWorker{Jobs: jobStore, Provider: noteProvider, Model: "gemini-3.8-flash-high", Handlers: map[string]jobs.ModelHandler{experience.Version: experience.Handler{}}}
	noteServer := httptest.NewServer((&api.MemoryCatalogServer{Store: cat, Auth: api.NewOwnerControlAuth(sealToken, experienceNativeOwner), ModelJobs: jobStore, ModelWorkerConfigured: true, ExperiencePolicies: configureExperiencePolicies(r.lessons, jobStore, r.tasks, r.history, r.results, cat), Experience: r.lessons}).Handle())
	defer noteServer.Close()
	request := experience.PolicyRequest{RunID: first.Result.RunID, Reconciliation: &selected, Policy: jobs.ModelPolicyRequest{ID: "controlled-reconciled-original-note", Privacy: "cloud_allowed", IntervalSeconds: 60, MaxJobs: 1, Job: jobs.Request{Deadline: time.Now().Add(10 * time.Minute), Allowance: budget.Units{ModelTurns: 1, Tokens: 1081344, ActiveMillis: 300000, ProviderInputBytes: 8388608}}}}
	screenOwnerAPI(t, noteServer.URL, sealToken, http.MethodPost, "/api/v4/memory/model-experience-policies", request, nil)
	var currentSnapshot string
	if err = pool.QueryRow(ctx, `SELECT source_id FROM memory_experience_reconciled_attempts`).Scan(&currentSnapshot); err != nil {
		t.Fatal(err)
	}
	currentSource, err := cat.Get(owner, currentSnapshot)
	if err != nil {
		t.Fatal("fresh reconciled source unavailable", err)
	}
	generic := request.Policy
	generic.ID, generic.Handler, generic.Trigger = "generic-cannot-borrow-reconciled-proof", experience.Version, "source_changed"
	generic.Job.TaskID, generic.Job.OriginalRunID = record.TaskID, first.Result.RunID
	generic.Job.Sources = []catalog.SourceRef{screenSourceRef(currentSource)}
	if _, err = jobStore.CreateModelPolicy(owner, generic); err == nil {
		t.Fatal("generic policy borrowed exact reconciled experience authority")
	}
	screenOwnerAPI(t, noteServer.URL, sealToken, http.MethodPost, "/api/v4/memory/model-worker-control", map[string]string{"state": "enabled"}, nil)
	if err = jobStore.PollModelPolicies(ctx, func(string) bool { return true }); err != nil {
		t.Fatal(err)
	}
	if strings.HasPrefix(scenario, "late_") {
		noteProvider.before = func() {
			var e error
			switch scenario {
			case "late_control":
				_, e = pool.Exec(ctx, `UPDATE task_control_events SET request_json=jsonb_set(request_json,'{instruction}',to_jsonb('changed owner instruction'::text)) WHERE control_id=$1`, resumed.ControlID)
			case "late_original_link":
				_, e = pool.Exec(ctx, `UPDATE accepted_inputs SET error_message='changed-control-reference' WHERE input_id=$1`, before.InputID)
			case "late_continuation_input":
				_, e = pool.Exec(ctx, `UPDATE accepted_inputs SET event_json=jsonb_set(event_json,'{PayloadJSON}',to_jsonb('{"control_id":"changed"}'::text)) WHERE input_id=$1`, resumed.ContinuationInputID)
			case "late_lookup":
				_, e = pool.Exec(ctx, `UPDATE tool_calls SET result='{"changed_observation":true}'::jsonb WHERE tool_call_id=$1`, supplemented.ReconciledAttempt.LookupPin.ID)
			case "late_effect":
				_, e = pool.Exec(ctx, `UPDATE action_effects SET state='effect_unknown' WHERE action_id=$1`, target.ActionID)
			case "late_witness":
				_, e = cat.Delete(owner, sealed.ID, "delete", sealed.Revision)
			case "late_parent":
				_, e = cat.Delete(owner, supplementParents[0].ID, "delete", supplementParents[0].Revision)
			case "late_pause":
				_, e = controls.Apply(experienceNativeActor(ctx, "late-current-owner-pause"), task.ControlRequest{TaskID: record.TaskID, ExpectedRevision: resumed.Task.Revision, Kind: task.ControlPause, Instruction: "Pause this current task"})
			}
			if e != nil {
				t.Fatal("actual late control unavailable", scenario, e)
			}
		}
	}
	worked, processErr := worker.ProcessOne(ctx)
	if !worked || noteProvider.calls != 1 {
		t.Fatal("one bounded separately authorized reconciled note required", worked, processErr, noteProvider.calls)
	}
	var published, attempts, observed int
	if pool.QueryRow(ctx, `SELECT count(*) FROM memory_experience_lessons`).Scan(&published) != nil || pool.QueryRow(ctx, `SELECT count(*),count(usage) FROM memory_model_attempts`).Scan(&attempts, &observed) != nil || attempts != 1 || observed != 1 {
		t.Fatal("reconciled note lost original/known failed-attempt spending")
	}
	if strings.HasPrefix(scenario, "late_") {
		if published != 0 {
			t.Fatal("late evidence/privacy/authority change published a note", scenario)
		}
		if scenario != "late_pause" {
			if _, e := cat.Get(owner, currentSnapshot); e == nil {
				t.Fatal("late material reconciliation proof change left source readable", scenario)
			}
		}
		return
	}
	if processErr != nil || published != 1 {
		t.Fatal("current reconciled note did not publish", processErr, published)
	}
	var claim string
	if err = pool.QueryRow(ctx, `SELECT claim_id FROM memory_experience_lessons`).Scan(&claim); err != nil {
		t.Fatal(err)
	}
	screenOwnerAPI(t, noteServer.URL, sealToken, http.MethodPost, "/api/v4/memory/decisions", catalog.Decision{ClaimID: claim}, nil)
	var body string
	if err = pool.QueryRow(ctx, `SELECT content FROM memory_episodes`).Scan(&body); err != nil {
		t.Fatal(err)
	}
	var lesson experience.Lesson
	if json.Unmarshal([]byte(body), &lesson) != nil || lesson.Verification != "task_unverified" || lesson.Observation.RunID != first.Result.RunID || lesson.Observation.ResultID != "" || lesson.Observation.AgentReport != "" || lesson.Observation.OwnerVerdict != "unknown" || lesson.Observation.ReconciledAttempt == nil || lesson.Observation.ReconciledAttempt.CurrentEffect != actionauth.StateAbsent || lesson.Observation.ReconciledAttempt.ContinuationRunID != second.Result.RunID || lesson.InstructionAuthority {
		t.Fatal("reconciled lesson invented an original success/final or hid owner intervention")
	}
	if scenario == "backup_seed" {
		experienceReconciledBackupSeed(t, ctx, r, cat, recipient, anchor, noteServer.URL, sealToken, currentSource, sealed, first.Result.RunID, second.Result.RunID, target.ActionID)
		return
	}
	if strings.HasPrefix(scenario, "privacy_witness_") {
		p := experienceReconciledBackupReceipt{Witness: sealed, Snapshot: screenSourceRef(currentSource), OriginalRun: first.Result.RunID, ReadRun: second.Result.RunID, TargetAction: target.ActionID}
		experienceReconciledPrivacyCopies(t, ctx, r, recipient, anchor, noteServer.URL, sealToken, &p)
		p.AccountingHash, p.MetadataHash = experienceReconciledBackupHashes(t, ctx, pool)
		action := strings.TrimPrefix(scenario, "privacy_witness_")
		if _, err := cat.Delete(owner, sealed.ID, action, sealed.Revision); err != nil {
			t.Fatal("witness-only privacy operation", err)
		}
		experienceAssertReconciledCopiesErased(t, ctx, cat, p, false)
		accounting, metadata := experienceReconciledBackupHashes(t, ctx, pool)
		if accounting != p.AccountingHash || metadata != p.MetadataHash {
			t.Fatal("witness-only privacy action changed original/reconciliation evidence or spending")
		}
		return
	}
	if strings.HasPrefix(scenario, "publication_") {
		p := experienceReconciledBackupReceipt{Witness: sealed, Snapshot: screenSourceRef(currentSource), OriginalRun: first.Result.RunID, ReadRun: second.Result.RunID, TargetAction: target.ActionID}
		experienceReconciledPrivacyCopies(t, ctx, r, recipient, anchor, noteServer.URL, sealToken, &p, strings.TrimPrefix(scenario, "publication_"))
		return
	}
	if scenario == "accepted" {
		down, e := os.ReadFile("../../../../migrations/081_episodic_reconciled_attempts.down.sql")
		if e != nil {
			t.Fatal(e)
		}
		if _, e = pool.Exec(ctx, string(down)); e == nil || !strings.Contains(e.Error(), "retain separately reconciled attempt provenance") {
			t.Fatal("populated reconciliation rollback did not preserve its fence", e)
		}
		if _, e = pool.Exec(ctx, `UPDATE memory_experience_reconciled_attempts SET control_id='invented' WHERE source_id=$1`, currentSnapshot); e == nil {
			t.Fatal("immutable reconciliation metadata could be overwritten")
		}
		if _, e = pool.Exec(ctx, `UPDATE memory_experience_terminal_actions SET observed_effect='effect_absent' WHERE source_id=$1`, sealed.ID); e == nil {
			t.Fatal("immutable original action witness could be rewritten")
		}
	}
	if strings.HasPrefix(scenario, "fence_") {
		read, e := pool.Begin(ctx)
		if e != nil {
			t.Fatal(e)
		}
		defer read.Rollback(ctx)
		if _, e = cat.AccessTx(owner, read, []catalog.SourceRef{screenSourceRef(currentSource)}, "memory_read"); e != nil {
			t.Fatal("current read fence unavailable", e)
		}
		writerPID, writerDone := make(chan uint32, 1), make(chan error, 1)
		go func() {
			connection, writerErr := pool.Acquire(ctx)
			if writerErr != nil {
				writerDone <- writerErr
				return
			}
			defer connection.Release()
			writerPID <- connection.Conn().PgConn().PID()
			switch scenario {
			case "fence_control":
				_, writerErr = connection.Exec(ctx, `UPDATE task_control_events SET request_json=jsonb_set(request_json,'{instruction}',to_jsonb('changed after current read'::text)) WHERE control_id=$1`, resumed.ControlID)
			case "fence_input":
				_, writerErr = connection.Exec(ctx, `UPDATE accepted_inputs SET error_message='changed-after-current-read' WHERE input_id=$1`, before.InputID)
			case "fence_effect":
				_, writerErr = connection.Exec(ctx, `UPDATE action_effects SET state='effect_unknown' WHERE action_id=$1`, target.ActionID)
			}
			writerDone <- writerErr
		}()
		var pid uint32
		select {
		case pid = <-writerPID:
		case e = <-writerDone:
			t.Fatal("metadata writer unavailable", e)
		case <-ctx.Done():
			t.Fatal("earlier metadata fence deadline")
		}
		blocked := false
		for until := time.Now().Add(8 * time.Second); time.Now().Before(until); {
			if e = read.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM pg_locks WHERE pid=$1 AND locktype='advisory' AND NOT granted)`, pid).Scan(&blocked); e != nil || blocked {
				break
			}
			time.Sleep(10 * time.Millisecond)
		}
		if !blocked || e != nil {
			t.Fatal("material writer did not wait for actual current source release", scenario, e)
		}
		if e = read.Commit(ctx); e != nil {
			t.Fatal(e)
		}
		if e = <-writerDone; e != nil {
			t.Fatal("material writer did not commit after release", e)
		}
		if _, e = cat.Get(owner, currentSnapshot); e == nil {
			t.Fatal("committed material writer left old reconciliation readable", scenario)
		}
	}
	if scenario == "control_tombstone" {
		var controlJSON, targetJSON, beforeHash string
		var beforeRevision int64
		if e := pool.QueryRow(ctx, `SELECT to_jsonb(c)::text,to_jsonb(ct)::text,h.row_hash,h.revision FROM task_control_events c JOIN task_control_targets ct USING(control_id) JOIN memory_experience_reconciled_heads h ON h.kind='control' AND h.row_id=c.control_id WHERE c.control_id=$1 AND ct.task_id=$2`, resumed.ControlID, record.TaskID).Scan(&controlJSON, &targetJSON, &beforeHash, &beforeRevision); e != nil {
			t.Fatal(e)
		}
		if _, e := pool.Exec(ctx, `DELETE FROM task_control_events WHERE control_id=$1`, resumed.ControlID); e != nil {
			t.Fatal(e)
		}
		if _, e := pool.Exec(ctx, `INSERT INTO task_control_events SELECT (jsonb_populate_record(NULL::task_control_events,$1::jsonb)).*`, controlJSON); e != nil {
			t.Fatal(e)
		}
		if _, e := pool.Exec(ctx, `INSERT INTO task_control_targets SELECT (jsonb_populate_record(NULL::task_control_targets,$1::jsonb)).*`, targetJSON); e != nil {
			t.Fatal(e)
		}
		var afterHash string
		var afterRevision int64
		if e := pool.QueryRow(ctx, `SELECT row_hash,revision FROM memory_experience_reconciled_heads WHERE kind='control' AND row_id=$1 AND alive`, resumed.ControlID).Scan(&afterHash, &afterRevision); e != nil || afterHash != beforeHash || afterRevision <= beforeRevision {
			t.Fatal("identical restored control lost its tombstone", e)
		}
		if _, e := cat.Get(owner, currentSnapshot); e == nil {
			t.Fatal("identical control reinsert revived an old reconciliation")
		}
	}
}
