package app

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	approvals "github.com/butler/butler/apps/orchestrator/internal/approval"
	"github.com/butler/butler/apps/orchestrator/internal/completion"
	"github.com/butler/butler/apps/orchestrator/internal/goal"
	durableinput "github.com/butler/butler/apps/orchestrator/internal/input"
	flow "github.com/butler/butler/apps/orchestrator/internal/orchestrator"
	runservice "github.com/butler/butler/apps/orchestrator/internal/run"
	"github.com/butler/butler/apps/orchestrator/internal/session"
	"github.com/butler/butler/apps/orchestrator/internal/task"
	"github.com/butler/butler/apps/orchestrator/internal/tools"
	"github.com/butler/butler/internal/actionauth"
	"github.com/butler/butler/internal/budget"
	"github.com/butler/butler/internal/delegation"
	commonv1 "github.com/butler/butler/internal/gen/common/v1"
	toolbrokerv1 "github.com/butler/butler/internal/gen/toolbroker/v1"
	"github.com/butler/butler/internal/ingress"
	"github.com/butler/butler/internal/memory/catalog"
	"github.com/butler/butler/internal/memory/transcript"
	"github.com/butler/butler/internal/ownership"
	"github.com/butler/butler/internal/transport"
	"github.com/butler/butler/internal/transport/antigravity"
)

var errNativeDiagnosisEntry = errors.New("zero_inference_provider_entry_denied")

// No native provider, auth manager, HTTP client, model stream or model response
// exists here. The spy only proves entry and fails synchronously.
type nativeEntryDenialSpy struct{ entries atomic.Int32 }

type nativeDiagnosticContextReader struct {
	sources       *catalog.Store
	manager       *task.DelegationManager
	entered       atomic.Bool
	catalogDenied atomic.Bool
	markDenied    atomic.Bool
}

func (r *nativeDiagnosticContextReader) LoadDelegatedContext(ctx context.Context, runID string) (*flow.DelegatedContext, error) {
	r.entered.Store(true)
	selected, err := (delegatedContextReader{sources: r.sources, manager: r.manager}).LoadDelegatedContext(ctx, runID)
	if err != nil {
		// Repeat only current checks under the same still-current child token to
		// localize a sentinel without printing any body or untrusted error.
		if c, e := r.sources.ReadDelegatedRun(ctx, runID); e != nil {
			r.catalogDenied.Store(true)
		} else if c != nil {
			if _, e = r.manager.MarkExecuting(ctx, c.Contract.TaskID); e != nil {
				r.markDenied.Store(true)
			}
		}
	}
	return selected, err
}

func (*nativeEntryDenialSpy) Name() string                    { return antigravity.Name }
func (*nativeEntryDenialSpy) InferenceLimits() (int64, int64) { return 1048576, 8192 }
func (*nativeEntryDenialSpy) Capabilities(context.Context, transport.TransportRunContext) (transport.CapabilitySnapshot, error) {
	return transport.CapabilitySnapshot{SupportsStreaming: true, SupportsToolCalls: true, SupportsCancel: true, SupportsUsageMetadata: true}, nil
}
func (s *nativeEntryDenialSpy) StartRun(context.Context, transport.StartRunRequest) (transport.EventStream, error) {
	s.entries.Add(1)
	return nil, errNativeDiagnosisEntry
}
func (s *nativeEntryDenialSpy) ContinueRun(context.Context, transport.ContinueRunRequest) (transport.EventStream, error) {
	s.entries.Add(1)
	return nil, errNativeDiagnosisEntry
}
func (s *nativeEntryDenialSpy) SubmitToolResult(context.Context, transport.SubmitToolResultRequest) (transport.EventStream, error) {
	s.entries.Add(1)
	return nil, errNativeDiagnosisEntry
}
func (*nativeEntryDenialSpy) CancelRun(context.Context, transport.CancelRunRequest) (*transport.TransportEvent, error) {
	return nil, nil
}

func nativeDiagnosisCore(f *nativeScenarioFixture, spy transport.ModelProvider, client *tools.BrokerClient, reader *nativeDiagnosticContextReader, checkpoints flow.CheckpointStore) *flow.Service {
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	bc := budget.NewController(f.pool, nativeExperimentPolicy())
	bc.SetAuthorityResolver(actionauth.ResolveBudgetAuthority)
	runs := runservice.NewService(runservice.NewPostgresRepository(f.pool), log)
	approvalRepo := approvals.NewPostgresRepository(f.pool)
	approvalRepo.SetCheckpointHooks(f.inputs)
	gate := flow.NewApprovalGate()
	approvalService := approvals.NewService(approvalRepo, gate)
	results := completion.New(f.pool)
	results.DataGuard = f.sources.CheckRunSourcesTx
	results.Evidence = effectEvidence{journal: actionauth.NewPostgresStore(f.pool)}
	return flow.NewService(f.sessions, session.NewPostgresLeaseManager(f.pool), runs, transcript.NewStore(f.pool), spy, flow.Config{
		ProviderName: antigravity.Name, ModelName: antigravity.DefaultModel, OwnerID: "native-fixture-worker", LeaseTTL: 60,
		AutonomyMode: commonv1.AutonomyMode_AUTONOMY_MODE_2, Tasks: f.tasks, Checkpoints: checkpoints, Completion: results,
		Budgets: bc, OwnerGoals: f.goals, MemorySources: f.sources, DelegationContexts: reader,
		Tools: client, ToolCatalog: client, ActionAuthorizer: client, ApprovalChecker: client, ApprovalGate: gate, ApprovalService: approvalService,
	}, log)
}

// These are explicit parent fixtures; actual Broker Prepare/Authorize/Claim and
// private Orchestrator admission create the child and its current source grants.
// The fixture never writes child/run/claim state or invents an admission receipt.
func nativeDiagnosisAdmission(t *testing.T, f *nativeScenarioFixture) (*DelegationAdmission, *tools.BrokerClient, delegation.AdmissionReceipt) {
	t.Helper()
	ctx := f.ctx
	if _, _, err := f.sessions.CreateSession(ctx, session.CreateSessionParams{SessionKey: "diagnosis-parent", UserID: nativeFixtureOwner, Channel: "web", MetadataJSON: `{}`}); err != nil {
		t.Fatal("diagnosis parent fixture session denied")
	}
	parent, err := f.goals.CreateOwnerTask(nativeOwner(ctx, "diagnosis-parent", "diagnosis-parent"), goal.TaskRequest{SessionKey: "diagnosis-parent", GoalID: f.goalID, MandateID: f.mandateID, Purpose: "native_research", Goal: "Explicit isolated recovery diagnosis", CompletionCriteria: []string{"Entry guard evidence only"}, InitiativeReason: "explicit lab owner request", ObservationRef: "fixture:diagnosis"})
	if err != nil {
		t.Fatal("diagnosis parent assignment denied")
	}
	admission, reader := f.composition(t, 1)
	if _, err = f.pool.Exec(ctx, `INSERT INTO runs(run_id,session_key,input_event_id,status,current_state,autonomy_mode,model_provider,started_at,updated_at,task_id,task_revision) VALUES('diagnosis-parent-run',$1,'diagnosis','tool_running','tool_running','mode_2','antigravity',clock_timestamp(),clock_timestamp(),$2,$3)`, parent.SessionKey, parent.TaskID, parent.Revision); err != nil {
		t.Fatal("diagnosis parent run fixture denied")
	}
	leases := session.NewPostgresLeaseManager(f.pool)
	lease, err := leases.AcquireLease(ctx, session.AcquireLeaseParams{SessionKey: parent.SessionKey, RunID: "diagnosis-parent-run", OwnerID: "diagnosis-parent-worker", TTL: time.Minute})
	if err != nil {
		t.Fatal("diagnosis actual parent lease denied")
	}
	if _, err = f.pool.Exec(ctx, `UPDATE runs SET lease_id=$1 WHERE run_id='diagnosis-parent-run'`, lease.LeaseID); err != nil {
		t.Fatal("diagnosis parent lease fixture pin denied")
	}
	owned := ownership.WithToken(ctx, leases.Authority(), ownership.Token{SessionKey: parent.SessionKey, RunID: "diagnosis-parent-run", LeaseID: lease.LeaseID, FenceEpoch: lease.FenceEpoch})
	if f.sources.PinTranscript(owned, parent.SessionKey, 0) != nil {
		t.Fatal("diagnosis actual parent transcript pin denied")
	}
	const token = "private-zero-inference-diagnosis-token-value"
	h, err := delegation.NewAdmissionHandler(token, admission)
	if err != nil {
		t.Fatal("diagnosis private admission unavailable")
	}
	mux := http.NewServeMux()
	mux.Handle("/context", actionauth.NewContextHandler(token, reader))
	mux.Handle("/delegate", h)
	control := httptest.NewServer(mux)
	t.Cleanup(control.Close)
	tokenFile := filepath.Join(f.private, "diagnosis-token")
	if os.WriteFile(tokenFile, []byte(token), 0600) != nil {
		t.Fatal("diagnosis private token file unavailable")
	}
	u, err := url.Parse(f.pool.Config().ConnConfig.ConnString())
	if err != nil {
		t.Fatal("diagnosis private schema reference invalid")
	}
	q := u.Query()
	q.Set("search_path", f.pool.Config().ConnConfig.RuntimeParams["search_path"])
	u.RawQuery = q.Encode()
	address := freeAddress(t)
	p := startNativeSubprocess(t, os.Getenv("BUTLER_DELEGATION_BROKER_BINARY"), "TestNativeDelegationBrokerHarness", u.String(), f.private, "diagnosis-broker", []string{"BUTLER_DELEGATION_NATIVE_BROKER_HARNESS=1", "BUTLER_DELEGATION_TOKEN_FILE=" + tokenFile, "BUTLER_DELEGATION_CONTEXT_URL=" + control.URL + "/context", "BUTLER_DELEGATION_ADMISSION_URL=" + control.URL + "/delegate", "BUTLER_DELEGATION_GRPC_ADDRESS=" + address, "BUTLER_DELEGATION_MANIFEST_FILE=" + f.manifest, "BUTLER_EXECUTION_BUDGET_POLICY_JSON=" + string(nativeJSON(nativeExperimentPolicy()))})
	t.Cleanup(func() { p.stop(t, false) })
	nativeWait(t, ctx, 10*time.Second, func() bool {
		c, e := net.DialTimeout("tcp", address, 50*time.Millisecond)
		if e != nil {
			return false
		}
		c.Close()
		return true
	})
	client, err := tools.DialWithTokenFile(address, tokenFile)
	if err != nil {
		t.Fatal("diagnosis actual private Broker unavailable")
	}
	t.Cleanup(func() { client.Close() })
	call := &toolbrokerv1.ToolCall{RunId: "diagnosis-parent-run", ToolCallId: "diagnosis-call", ToolName: delegation.AdmissionTool, ArgsJson: string(nativeJSON(f.args[0])), AutonomyMode: commonv1.AutonomyMode_AUTONOMY_MODE_2}
	prepared, err := client.PrepareAction(ctx, call)
	if err != nil || prepared.RequiresApproval {
		t.Fatal("diagnosis exact preparation denied")
	}
	grant, err := client.AuthorizeAction(ctx, call, prepared.Binding.ActionID, "", "")
	if err != nil {
		t.Fatal("diagnosis bounded authorization denied")
	}
	result, err := client.ExecuteAuthorizedToolCall(ctx, call, grant)
	if err != nil || result.GetStatus() != "completed" || result.GetEffectState() != string(actionauth.StateConfirmed) {
		t.Fatal("diagnosis actual dispatch admission denied")
	}
	var receipt delegation.AdmissionReceipt
	if json.Unmarshal([]byte(result.GetResultJson()), &receipt) != nil || receipt.TaskID == "" || receipt.InputID == "" || receipt.State != "queued" {
		t.Fatal("diagnosis actual receipt unavailable")
	}
	if _, err = leases.ReleaseLease(ctx, lease.LeaseID); err != nil {
		t.Fatal("diagnosis parent process release denied")
	}
	return admission, client, receipt
}

// Error text remains only in runtime memory. Output contains fixed classifications
// plus a hash; no input/result body, credentials or untrusted error is printed.
func nativeDiagnosisReasons(message string) []string {
	out := []string{}
	for _, code := range []string{"source", "delegation", "authority", "revision", "budget", "lease", "context", "scope", "provider", "task", "contract", "tool", "model", "input", "ownership", "checkpoint", "zero_inference_provider_entry_denied"} {
		if strings.Contains(strings.ToLower(message), code) {
			out = append(out, code)
		}
	}
	return out
}

type nativeCheckpointsWithoutVerifier struct{ flow.CheckpointStore }

type nativeMutatedInput struct {
	*flow.Service
	mutation   string
	staleClaim *durableinput.Record
}

func (m nativeMutatedInput) Execute(ctx context.Context, event ingress.InputEvent) (*flow.ExecutionResult, error) {
	switch m.mutation {
	case "forged-key":
		event.IdempotencyKey = "forged-unaccepted-input"
	case "forged-task":
		var p map[string]any
		_ = json.Unmarshal([]byte(event.PayloadJSON), &p)
		p["task_id"] = "unaccepted-task"
		event.PayloadJSON = string(nativeJSON(p))
	case "forged-source":
		event.Source = "internal"
	case "forged-contract":
		var p struct {
			TaskID string               `json:"task_id"`
			Source delegation.SourceRef `json:"delegation_source"`
		}
		_ = json.Unmarshal([]byte(event.PayloadJSON), &p)
		p.Source.Hash = strings.Repeat("f", 64)
		event.PayloadJSON = string(nativeJSON(p))
	case "stale-claim":
		ctx = durableinput.WithClaim(ctx, *m.staleClaim)
	}
	return m.Service.Execute(ctx, event)
}

func TestNativeDelegationZeroInferenceRecoveryDiagnosisIntegration(t *testing.T) {
	if os.Getenv("BUTLER_DELEGATION_BROKER_BINARY") == "" {
		t.Skip("actual private Broker binary required")
	}
	for _, name := range []string{"fresh-queued", "expired-input-claim", "forged-key", "forged-task", "forged-source", "forged-contract", "stale-claim", "missing-verifier"} {
		t.Run(name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 35*time.Second)
			defer cancel()
			f := newNativeScenarioFixture(t, ctx)
			admission, client, receipt := nativeDiagnosisAdmission(t, f)
			tx, err := f.pool.Begin(ctx)
			if err != nil {
				t.Fatal("diagnosis post-admission read unavailable")
			}
			_, err = delegation.LoadTx(ctx, tx, receipt.TaskID)
			_ = tx.Rollback(ctx)
			t.Logf("post_admission_contract_load_valid=%t", err == nil)
			var oldClaim *durableinput.Record
			if name == "expired-input-claim" || name == "stale-claim" {
				claim, err := f.inputs.Claim(ctx, 80*time.Millisecond)
				if err != nil || claim.InputID != receipt.InputID || claim.ClaimExpiresAt == nil {
					t.Fatal("diagnosis real prior claim denied")
				}
				nativeWait(t, ctx, time.Second, func() bool { return time.Now().After(*claim.ClaimExpiresAt) })
				oldClaim = &claim
			}
			spy := &nativeEntryDenialSpy{}
			reader := &nativeDiagnosticContextReader{sources: f.sources, manager: admission.Manager}
			log := slog.New(slog.NewTextHandler(io.Discard, nil))
			runs := runservice.NewService(runservice.NewPostgresRepository(f.pool), log)
			var checkpoints flow.CheckpointStore = f.inputs
			if name == "missing-verifier" {
				checkpoints = nativeCheckpointsWithoutVerifier{f.inputs}
			}
			worker := durableinput.NewService(f.inputs, f.sessions, nativeMutatedInput{Service: nativeDiagnosisCore(f, spy, client, reader, checkpoints), mutation: name, staleClaim: oldClaim}, runs, log)
			workerCtx, stop := context.WithCancel(ctx)
			done := make(chan error, 1)
			go func() { done <- worker.Run(workerCtx) }()
			t.Cleanup(func() { stop(); <-done })
			var status, code, message, runState, taskState string
			nativeWait(t, ctx, 15*time.Second, func() bool {
				return f.pool.QueryRow(ctx, `SELECT status,error_code,error_message FROM accepted_inputs WHERE input_id=$1`, receipt.InputID).Scan(&status, &code, &message) == nil && (status == "blocked" || status == "failed" || status == "completed")
			})
			_ = f.pool.QueryRow(ctx, `SELECT COALESCE(r.current_state,''),t.status FROM tasks t LEFT JOIN runs r ON r.run_id=(SELECT run_id FROM accepted_inputs WHERE input_id=$1) WHERE t.task_id=$2`, receipt.InputID, receipt.TaskID).Scan(&runState, &taskState)
			hash := sha256.Sum256([]byte(message))
			evidence := map[string]any{"case": name, "input_status": status, "error_code": code, "reason_categories": nativeDiagnosisReasons(message), "error_sha256": hex.EncodeToString(hash[:]), "run_state": runState, "task_state": taskState, "provider_spy_entries": spy.entries.Load(), "native_attempts": 0, "context_reader_entered": reader.entered.Load(), "catalog_check_denied": reader.catalogDenied.Load(), "mark_executing_denied": reader.markDenied.Load()}
			t.Log(string(nativeJSON(evidence)))
			var nativeAttempts int
			if f.pool.QueryRow(ctx, `SELECT attempts FROM fixture_native_limit`).Scan(&nativeAttempts) != nil || nativeAttempts != nativeDelegationPriorAttempts {
				t.Fatal("diagnosis attempted native inference")
			}
			// Initially this assertion exposes the real pre-provider denial. After
			// a narrow fix it proves all current guards reach only the denied spy.
			positive := name == "fresh-queued" || name == "expired-input-claim"
			if positive && (spy.entries.Load() != 1 || !strings.Contains(message, errNativeDiagnosisEntry.Error())) {
				t.Fatal("actual queued child stopped before zero-inference spy entry")
			}
			if !positive && (spy.entries.Load() != 0 || status == "completed") {
				t.Fatal("forged or stale private input reached provider spy")
			}
		})
	}
}
