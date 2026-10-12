package app

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/butler/butler/apps/orchestrator/internal/completion"
	durableinput "github.com/butler/butler/apps/orchestrator/internal/input"
	runservice "github.com/butler/butler/apps/orchestrator/internal/run"
	"github.com/butler/butler/apps/orchestrator/internal/session"
	"github.com/butler/butler/internal/delegation"
	runv1 "github.com/butler/butler/internal/gen/run/v1"
	"github.com/butler/butler/internal/ingress"
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
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Second)
	defer cancel()
	f := newNativeScenarioFixture(t, ctx)
	admission, broker := nativeDiagnosticBroker(t, f, 2)
	const key = "full-high-zero-inference-parent"
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
	provider := &fullHighPreflightProvider{nativeScriptedObservationProvider: &nativeScriptedObservationProvider{args: f.args, receipts: map[string][]string{}}, f: f}
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
		return f.pool.QueryRow(ctx, `SELECT count(*) FROM task_delegations WHERE parent_task_id=$1 AND state='result_ready'`, parent.TaskID).Scan(&ready) == nil && ready == 2
	})
	stop()
	if <-done != nil {
		t.Fatal("actual preflight worker failed to stop")
	}
	stopped = true
	v := nativeWidthEvidence{Width: 2, TaskID: parent.TaskID, InputID: receipt.InputID, FullAcceptance: &fullHighWidthAcceptance{ResolutionPolicy: "owner_selected_source_a_for_fixture"}}
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
	verifyFullHighOwnerDecisions(t, f, admission, v, v.FullAcceptance)
	var spent int
	if f.pool.QueryRow(ctx, `SELECT attempts FROM fixture_native_limit`).Scan(&spent) != nil || spent != 27 || provider.parentStarts.Load() != 1 || provider.continuations.Load() != 2 || provider.childEntries.Load() != 2 {
		t.Fatal("zero-inference preflight bypassed owning flow or charged native calls")
	}
	final, err := completion.New(f.pool).GetByRun(ctx, v.ParentRunID)
	if err != nil || final.Verdict != "unverified" {
		t.Fatal("source-relative decisions promoted parent verdict")
	}
}
