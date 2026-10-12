package app

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	approvals "github.com/butler/butler/apps/orchestrator/internal/approval"
	"github.com/butler/butler/apps/orchestrator/internal/completion"
	durableinput "github.com/butler/butler/apps/orchestrator/internal/input"
	runservice "github.com/butler/butler/apps/orchestrator/internal/run"
	"github.com/butler/butler/apps/orchestrator/internal/session"
	"github.com/butler/butler/apps/orchestrator/internal/task"
	"github.com/butler/butler/internal/delegation"
	runv1 "github.com/butler/butler/internal/gen/run/v1"
	"github.com/butler/butler/internal/ingress"
	"github.com/butler/butler/internal/memory/transcript"
	"github.com/butler/butler/internal/transport"
	"github.com/jackc/pgx/v5"
)

// Scripted provider values verify the actual owning report/owner-decision path
// before a live window. They are never native generation acceptance evidence.
type fullHighPreflightProvider struct {
	*nativeScriptedObservationProvider
	f *nativeScenarioFixture
}

func (p *fullHighPreflightProvider) StartRun(ctx context.Context, request transport.StartRunRequest) (transport.EventStream, error) {
	if nativeContextObserver(p.f.pool, p.f.corpus)(ctx, request) != nil {
		return nil, errNativeFixtureDenied
	}
	var raw []byte
	err := p.f.pool.QueryRow(ctx, `SELECT d.contract_json FROM task_delegations d JOIN runs r ON r.task_id=d.task_id WHERE r.run_id=$1`, request.Context.RunID).Scan(&raw)
	if err == pgx.ErrNoRows {
		return p.nativeScriptedObservationProvider.StartRun(ctx, request)
	}
	var contract delegation.Contract
	if err != nil || json.Unmarshal(raw, &contract) != nil || len(contract.Sources) != 1 {
		return nil, errNativeFixtureDenied
	}
	for i, args := range p.f.args {
		if args.Sources[0] == contract.Sources[0] {
			p.childEntries.Add(1)
			observation := delegation.NumericObservation{SourceID: args.Sources[0].ID, SourceRevision: args.Sources[0].Revision, SourceHash: args.Sources[0].Hash, ObservedValue: p.f.corpus.Sources[i].ExpectedValue, ObservedDate: p.f.corpus.Sources[i].ExpectedDate, Uncertainty: "source_only"}
			return nativeScriptedStream(transport.NewAssistantFinalEvent(request.Context.RunID, p.Name(), transport.AssistantFinal{Content: string(nativeJSON(observation)), FinishReason: "stop"})), nil
		}
	}
	return nil, errNativeFixtureDenied
}

func TestFullHighDelegationIndependentDecisionsWithoutInferenceIntegration(t *testing.T) {
	if os.Getenv("BUTLER_DELEGATION_BROKER_BINARY") == "" {
		t.Skip("actual private Broker binary required")
	}
	t.Setenv("BUTLER_DELEGATION_HIGH_ACCEPTANCE_WINDOW", "1")
	ctx, cancel := context.WithTimeout(context.Background(), 75*time.Second)
	defer cancel()
	f := newNativeScenarioFixture(t, ctx)
	provider := &fullHighPreflightProvider{nativeScriptedObservationProvider: &nativeScriptedObservationProvider{args: f.args, receipts: map[string][]string{}}, f: f}
	evidence := fullHighControlPreflightEvidence{Version: 1, Status: "failed", ExecutionMode: "all_model_turns_scripted_actual_core_broker", NativeCalls: 0}
	path := nativeAuditTestPath(t, "full-high-control-matrix")
	defer func() {
		var err error
		evidence.OwnedState, err = collectNativeOwnedAudit(context.Background(), f.pool, true)
		if err != nil || os.WriteFile(path, nativeJSON(evidence), 0600) != nil {
			t.Error("safe same-fixture control evidence unavailable")
		}
	}()
	var admissions []*DelegationAdmission
	var widths []nativeWidthEvidence
	for _, width := range []int{1, 2} {
		admission, observed := fullHighScriptedWidth(t, f, width, provider)
		admissions = append(admissions, admission)
		widths = append(widths, observed)
		evidence.Widths = append(evidence.Widths, fullHighControlWidthEvidence{Width: width, TaskID: observed.TaskID, ParentRunID: observed.ParentRunID, ChildTaskIDs: observed.ChildTaskIDs, ChildRunIDs: observed.ChildRunIDs, SourceRefs: observed.SourceRefs, ReportSourceRefs: observed.ReportSourceRefs, IndependentChecks: observed.FullAcceptance})
		if width == 1 {
			verifyFullHighOwnerDecisions(t, f, admission, observed, observed.FullAcceptance)
		} else {
			verifyFullHighIndependentResults(t, f, admission, observed, observed.FullAcceptance, false)
		}
	}
	first, second := widths[0], widths[1]
	admission := admissions[1]
	checkFullHighCurrentRevisionBaseline(t, f, admission, second)
	leases := session.NewPostgresLeaseManager(f.pool)
	controller := task.NewController(f.tasks, leases.Authority(), leases, runservice.NewPostgresRepository(f.pool), approvals.NewPostgresRepository(f.pool), f.inputs, transcript.NewStore(f.pool))
	parent, err := f.tasks.Get(ctx, second.TaskID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = controller.Apply(nativeOwner(ctx, "preflight-amend", parent.SessionKey), task.ControlRequest{TaskID: parent.TaskID, ExpectedRevision: parent.Revision, Kind: task.ControlAmend, Instruction: "Preserve independent observations; obsolete reports are not new verification."}); err != nil {
		t.Fatal("actual same-fixture amendment failed", err)
	}
	for i, child := range second.ChildTaskIDs {
		if _, err = admission.Manager.CollectGenerated(ctx, parent.TaskID, child, second.ChildResultIDs[i]); err == nil {
			t.Fatal("obsolete collection survived actual amendment")
		}
		if checkRecordedInitiativeSources(ctx, f.pool, f.sources, second.ChildRunIDs[i]) == nil {
			t.Fatal("obsolete source release survived actual amendment")
		}
	}
	evidence.RevisionDenies = true
	checkFullHighOriginalsBeforeDeletion(t, f)
	if _, err = f.sources.Delete(nativeOwner(ctx, "preflight-delete", ""), f.args[0].Sources[0].ID, "delete", f.args[0].Sources[0].Revision); err != nil {
		t.Fatal("actual same-fixture deletion failed", err)
	}
	checkFullHighOriginalDeletion(t, f)
	evidence.DeletionDenies = true
	parent, err = f.tasks.Get(ctx, first.TaskID)
	if err != nil {
		t.Fatal(err)
	}
	// Use the same authenticated owner for the current scheduling baseline and
	// denial. This does not claim source release from already-decided children.
	owner := nativeOwner(ctx, "preflight-stop-scheduling-proof", "")
	if _, err = admissions[0].Manager.Advance(owner, parent.TaskID); err != nil {
		t.Fatal("current first parent scheduling denied before stop", err)
	}
	if _, err = controller.Apply(nativeOwner(ctx, "preflight-stop-first", parent.SessionKey), task.ControlRequest{TaskID: parent.TaskID, ExpectedRevision: parent.Revision, Kind: task.ControlStop}); err != nil {
		t.Fatal("actual first-root stop after deletion failed", err)
	}
	if _, err = admissions[0].Manager.Advance(owner, parent.TaskID); err == nil {
		t.Fatal("stopped first parent continued")
	}
	evidence.OwnerStopDenies = true
	// Terminate the fresh amendment input too, using its current owner revision.
	// No queued continuation is executed and no inference is added by cleanup.
	current, err := f.tasks.Get(ctx, second.TaskID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = controller.Apply(nativeOwner(ctx, "preflight-stop-amended", current.SessionKey), task.ControlRequest{TaskID: current.TaskID, ExpectedRevision: current.Revision, Kind: task.ControlStop}); err != nil {
		t.Fatal("actual amended-root stop after deletion failed", err)
	}
	if _, err = admission.Manager.Advance(owner, current.TaskID); err == nil {
		t.Fatal("stopped amended parent continued")
	}
	var active bool
	if f.pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM accepted_inputs WHERE status IN ('queued','running')) OR EXISTS(SELECT 1 FROM session_ownership WHERE run_id IS NOT NULL AND expires_at>clock_timestamp())`).Scan(&active) != nil || active {
		t.Fatal("same-fixture controls left executable inputs or live workers")
	}
	evidence.DurableTermination = true
	if f.pool.QueryRow(ctx, `SELECT count(DISTINCT e.action_id) FROM action_effects e JOIN action_intents a ON a.effect_id=e.action_id JOIN runs r ON r.run_id=a.run_id WHERE r.task_id IN ($1,$2) AND e.state='effect_confirmed'`, first.TaskID, second.TaskID).Scan(&evidence.ConfirmedEffects) != nil || evidence.ConfirmedEffects != 4 {
		t.Fatal("same-fixture controls lost four actual Broker effects")
	}
	var spent int
	if f.pool.QueryRow(ctx, `SELECT attempts FROM fixture_native_limit`).Scan(&spent) != nil || spent != 27 || provider.parentStarts.Load() != 2 || provider.continuations.Load() != 4 || provider.childEntries.Load() != 4 {
		t.Fatal("same-fixture preflight bypassed owning flow or charged native calls")
	}
	evidence.ScriptedModelTurns = int(provider.parentStarts.Load() + provider.continuations.Load() + provider.childEntries.Load())
	for _, width := range widths {
		final, err := completion.New(f.pool).GetByRun(ctx, width.ParentRunID)
		if err != nil || final.Verdict != "unverified" {
			t.Fatal("source-relative decisions promoted parent verdict", err)
		}
	}
	evidence.Status = "passed_without_inference"
	t.Logf("same_fixture_widths=1,2 independent_owner_api=true revision_denies=true deletion_denies=true stop_denies=true durable_termination=true confirmed_effects=4 scripted_model_turns=%d native_calls=0", evidence.ScriptedModelTurns)
}

type fullHighControlPreflightEvidence struct {
	Version            int                            `json:"version"`
	Status             string                         `json:"status"`
	ExecutionMode      string                         `json:"execution_mode"`
	NativeCalls        int                            `json:"native_calls"`
	ScriptedModelTurns int                            `json:"scripted_model_turns"`
	Widths             []fullHighControlWidthEvidence `json:"widths"`
	RevisionDenies     bool                           `json:"revision_denies"`
	DeletionDenies     bool                           `json:"deletion_denies"`
	OwnerStopDenies    bool                           `json:"owner_stop_denies"`
	DurableTermination bool                           `json:"durable_termination"`
	ConfirmedEffects   int                            `json:"confirmed_effects_after_controls"`
	OwnedState         nativeOwnedAudit               `json:"owned_state"`
}

// This projection records only measured control/verification boundaries. Native
// latency, generation quality and overlap remain in their separate sealed proof.
type fullHighControlWidthEvidence struct {
	Width             int                      `json:"width"`
	TaskID            string                   `json:"task_id"`
	ParentRunID       string                   `json:"parent_run_id"`
	ChildTaskIDs      []string                 `json:"child_task_ids"`
	ChildRunIDs       []string                 `json:"child_run_ids"`
	SourceRefs        []delegation.SourceRef   `json:"source_refs"`
	ReportSourceRefs  []delegation.SourceRef   `json:"report_source_refs"`
	IndependentChecks *fullHighWidthAcceptance `json:"independent_checks_before_controls"`
}

func fullHighScriptedWidth(t *testing.T, f *nativeScenarioFixture, width int, provider *fullHighPreflightProvider) (*DelegationAdmission, nativeWidthEvidence) {
	t.Helper()
	ctx := f.ctx
	// Both Broker processes share the actual Catalog, policy, budgets, journals
	// and input owners. Separate private directories preserve immutable logs and
	// let each trusted composition use its real width1/width2 admission limit.
	brokerFixture := *f
	brokerFixture.private = filepath.Join(f.private, "width-"+strconv.Itoa(width))
	if err := os.Mkdir(brokerFixture.private, 0700); err != nil {
		t.Fatal(err)
	}
	admission, broker := nativeDiagnosticBroker(t, &brokerFixture, width)
	key := "full-high-zero-inference-parent-" + strconv.Itoa(width)
	if _, _, err := f.sessions.CreateSession(ctx, session.CreateSessionParams{SessionKey: key, UserID: nativeFixtureOwner, Channel: "web", MetadataJSON: `{}`}); err != nil {
		t.Fatal("bounded preflight session unavailable")
	}
	parent, err := f.goals.CreateOwnerTask(nativeOwner(ctx, key, key), nativeScriptedTaskRequest(f, key))
	if err != nil {
		t.Fatal("bounded preflight task unavailable")
	}
	event, err := ingress.NormalizeEvent(ingress.NormalizeRequest{EventType: runv1.InputEventType_INPUT_EVENT_TYPE_USER_MESSAGE, EventID: key, SessionKey: key, Source: "web", Payload: map[string]any{"user_id": nativeFixtureOwner, "task_id": parent.TaskID, "text": fullHighParentPrompt(f.args)}, DeliveryID: key})
	if err != nil {
		t.Fatal("bounded preflight input unavailable")
	}
	receipt, _, err := f.inputs.Accept(nativeOwner(ctx, key, key), event, nativeFixtureOwner, parent.TaskID)
	if err != nil {
		t.Fatal("actual preflight input unavailable")
	}
	reader := &nativeDiagnosticContextReader{sources: f.sources, manager: admission.Manager}
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	runs := runservice.NewService(runservice.NewPostgresRepository(f.pool), log)
	worker := durableinput.NewService(f.inputs, f.sessions, nativeDiagnosisCore(f, provider, broker, reader, f.inputs), runs, log)
	worker.SetMaintenance((&delegationMaintenance{pool: f.pool, manager: admission.Manager}).Sweep)
	workerCtx, stop := context.WithCancel(ctx)
	done := make(chan error, 1)
	go func() { done <- worker.Run(workerCtx) }()
	stopped := false
	defer func() {
		if !stopped {
			stop()
			<-done
		}
	}()
	nativeWait(t, ctx, 25*time.Second, func() bool {
		var ready int
		return f.pool.QueryRow(ctx, `SELECT count(*) FROM task_delegations d JOIN accepted_inputs i ON i.task_id=d.task_id WHERE d.parent_task_id=$1 AND d.state='result_ready' AND i.status='completed'`, parent.TaskID).Scan(&ready) == nil && ready == 2
	})
	stop()
	if <-done != nil {
		t.Fatal("actual preflight worker failed to stop")
	}
	stopped = true
	v := nativeWidthEvidence{Width: width, TaskID: parent.TaskID, InputID: receipt.InputID, SourceRefs: []delegation.SourceRef{f.args[0].Sources[0], f.args[1].Sources[0]}, FullAcceptance: &fullHighWidthAcceptance{ResolutionPolicy: "owner_selected_source_a_for_fixture"}}
	if f.pool.QueryRow(ctx, `SELECT run_id FROM accepted_inputs WHERE input_id=$1 AND status='completed'`, receipt.InputID).Scan(&v.ParentRunID) != nil {
		t.Fatal("preflight parent final unavailable")
	}
	checkFullHighParentReceipts(t, f, parent.TaskID, v.ParentRunID, v.FullAcceptance)
	rows, err := f.pool.Query(ctx, `SELECT d.task_id,f.run_id,f.result_id,d.decision_evidence_json FROM task_delegations d JOIN run_final_results f USING(task_id) WHERE d.parent_task_id=$1 ORDER BY d.task_id`, parent.TaskID)
	if err != nil {
		t.Fatal("actual preflight children unavailable")
	}
	for rows.Next() {
		var child, run, result string
		var raw []byte
		var ref struct {
			Report *delegation.SourceRef `json:"report_source"`
		}
		if rows.Scan(&child, &run, &result, &raw) != nil || json.Unmarshal(raw, &ref) != nil || ref.Report == nil {
			rows.Close()
			t.Fatal("actual preflight child projection unavailable")
		}
		v.ChildTaskIDs = append(v.ChildTaskIDs, child)
		v.ChildRunIDs = append(v.ChildRunIDs, run)
		v.ChildResultIDs = append(v.ChildResultIDs, result)
		v.ReportSourceRefs = append(v.ReportSourceRefs, *ref.Report)
	}
	err = rows.Err()
	rows.Close()
	if err != nil || len(v.ChildTaskIDs) != 2 {
		t.Fatal("actual preflight child collection incomplete")
	}
	return admission, v
}
