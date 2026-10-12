package app

import (
	"context"
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
	"sync"
	"sync/atomic"
	"testing"
	"time"

	approvals "github.com/butler/butler/apps/orchestrator/internal/approval"
	"github.com/butler/butler/apps/orchestrator/internal/goal"
	durableinput "github.com/butler/butler/apps/orchestrator/internal/input"
	runservice "github.com/butler/butler/apps/orchestrator/internal/run"
	"github.com/butler/butler/apps/orchestrator/internal/session"
	"github.com/butler/butler/apps/orchestrator/internal/task"
	"github.com/butler/butler/apps/orchestrator/internal/tools"
	"github.com/butler/butler/internal/actionauth"
	"github.com/butler/butler/internal/delegation"
	runv1 "github.com/butler/butler/internal/gen/run/v1"
	"github.com/butler/butler/internal/ingress"
	"github.com/butler/butler/internal/memory/transcript"
	"github.com/butler/butler/internal/transport"
)

func nativeDiagnosticBroker(t *testing.T, f *nativeScenarioFixture, width int) (*DelegationAdmission, *tools.BrokerClient) {
	t.Helper()
	admission, reader := f.composition(t, width)
	const token = "private-scripted-diagnostic-token-value"
	h, err := delegation.NewAdmissionHandler(token, admission)
	if err != nil {
		t.Fatal("private scripted admission unavailable")
	}
	mux := http.NewServeMux()
	mux.Handle("/context", actionauth.NewContextHandler(token, reader))
	mux.Handle("/delegate", h)
	control := httptest.NewServer(mux)
	t.Cleanup(control.Close)
	tokenFile := filepath.Join(f.private, "scripted-token")
	if os.WriteFile(tokenFile, []byte(token), 0600) != nil {
		t.Fatal("private scripted proof file unavailable")
	}
	u, err := url.Parse(f.pool.Config().ConnConfig.ConnString())
	if err != nil {
		t.Fatal("private scripted schema reference unavailable")
	}
	q := u.Query()
	q.Set("search_path", f.pool.Config().ConnConfig.RuntimeParams["search_path"])
	u.RawQuery = q.Encode()
	address := freeAddress(t)
	p := startNativeSubprocess(t, os.Getenv("BUTLER_DELEGATION_BROKER_BINARY"), "TestNativeDelegationBrokerHarness", u.String(), f.private, "scripted-broker", []string{"BUTLER_DELEGATION_NATIVE_BROKER_HARNESS=1", "BUTLER_DELEGATION_TOKEN_FILE=" + tokenFile, "BUTLER_DELEGATION_CONTEXT_URL=" + control.URL + "/context", "BUTLER_DELEGATION_ADMISSION_URL=" + control.URL + "/delegate", "BUTLER_DELEGATION_GRPC_ADDRESS=" + address, "BUTLER_DELEGATION_MANIFEST_FILE=" + f.manifest, "BUTLER_EXECUTION_BUDGET_POLICY_JSON=" + string(nativeJSON(nativeExperimentPolicy()))})
	t.Cleanup(func() { p.stop(t, false) })
	nativeWait(t, f.ctx, 10*time.Second, func() bool {
		c, e := net.DialTimeout("tcp", address, 50*time.Millisecond)
		if e != nil {
			return false
		}
		c.Close()
		return true
	})
	client, err := tools.DialWithTokenFile(address, tokenFile)
	if err != nil {
		t.Fatal("private scripted Broker unavailable")
	}
	t.Cleanup(func() { client.Close() })
	return admission, client
}

// No PG, credentials, HTTP client or native provider exists in this transport.
// It emits a fixed sequential parent protocol and independently denies children.
// Real owning workers/Broker/admission determine every run, receipt and effect.
type nativeScriptedObservationProvider struct {
	args          []delegation.AdmissionArgs
	mu            sync.Mutex
	receipts      map[string][]string
	parentStarts  atomic.Int32
	continuations atomic.Int32
	childEntries  atomic.Int32
	denySecond    bool
}

func (*nativeScriptedObservationProvider) Name() string { return "antigravity" }
func (*nativeScriptedObservationProvider) InferenceLimits() (int64, int64) {
	return 1048576, 8192
}
func (*nativeScriptedObservationProvider) Capabilities(context.Context, transport.TransportRunContext) (transport.CapabilitySnapshot, error) {
	return transport.CapabilitySnapshot{SupportsStreaming: true, SupportsToolCalls: true, SupportsCancel: true, SupportsUsageMetadata: true}, nil
}
func nativeScriptedStream(events ...transport.TransportEvent) transport.EventStream {
	stream := make(chan transport.TransportEvent, len(events))
	for _, e := range events {
		stream <- e
	}
	close(stream)
	return stream
}
func (p *nativeScriptedObservationProvider) tool(runID string, index int) transport.EventStream {
	return nativeScriptedStream(transport.NewToolCallRequestedEvent(runID, p.Name(), transport.ToolCallRequest{ToolCallRef: runID + "-scripted-call-" + string(rune('0'+index)), ToolName: delegation.AdmissionTool, ArgsJSON: string(nativeJSON(p.args[index])), SequenceNo: index + 1}))
}
func (p *nativeScriptedObservationProvider) StartRun(_ context.Context, r transport.StartRunRequest) (transport.EventStream, error) {
	if len(r.ToolDefinitions) == 0 {
		p.childEntries.Add(1)
		return nil, errNativeDiagnosisEntry
	}
	if len(r.ToolDefinitions) != 1 || r.ToolDefinitions[0].Name != delegation.AdmissionTool {
		return nil, errNativeFixtureDenied
	}
	p.parentStarts.Add(1)
	p.mu.Lock()
	p.receipts[r.Context.RunID] = nil
	p.mu.Unlock()
	return p.tool(r.Context.RunID, 0), nil
}
func (p *nativeScriptedObservationProvider) SubmitToolResult(_ context.Context, r transport.SubmitToolResultRequest) (transport.EventStream, error) {
	p.continuations.Add(1)
	var envelope struct {
		Status string                      `json:"status"`
		Result delegation.AdmissionReceipt `json:"result"`
	}
	if json.Unmarshal([]byte(r.ToolResultJSON), &envelope) != nil || envelope.Status != "completed" || envelope.Result.TaskID == "" || (envelope.Result.State != "queued" && envelope.Result.State != "waiting") || envelope.Result.State == "queued" && envelope.Result.InputID == "" {
		return nil, errNativeFixtureDenied
	}
	p.mu.Lock()
	ids := append(p.receipts[r.RunID], envelope.Result.TaskID)
	p.receipts[r.RunID] = ids
	p.mu.Unlock()
	if len(ids) == 1 {
		if p.denySecond {
			// Contains deliberate private-looking material; only the fixed run
			// error enum may enter the owned diagnostic export.
			return nil, errors.New("scripted-provider-denial SECRET_DIAGNOSTIC_MARKER password=fixture-private")
		}
		return p.tool(r.RunID, 1), nil
	}
	if len(ids) != 2 {
		return nil, errNativeFixtureDenied
	}
	return nativeScriptedStream(transport.NewAssistantFinalEvent(r.RunID, p.Name(), transport.AssistantFinal{Content: string(nativeJSON(map[string]any{"admitted_task_ids": ids})), FinishReason: "stop"}), transport.NewTerminalEvent(r.RunID, transport.EventTypeRunCompleted, p.Name())), nil
}
func (*nativeScriptedObservationProvider) ContinueRun(context.Context, transport.ContinueRunRequest) (transport.EventStream, error) {
	return nil, errNativeDiagnosisEntry
}
func (*nativeScriptedObservationProvider) CancelRun(context.Context, transport.CancelRunRequest) (*transport.TransportEvent, error) {
	return nil, nil
}

func nativeAuditTestPath(t *testing.T, name string) string {
	t.Helper()
	if dir := os.Getenv("BUTLER_DELEGATION_DIAGNOSTIC_REPORT_DIR"); dir != "" {
		return filepath.Join(dir, name+".json")
	}
	return filepath.Join(t.TempDir(), name+".json")
}

func nativeScriptedTaskRequest(f *nativeScenarioFixture, key string) goal.TaskRequest {
	return goal.TaskRequest{SessionKey: key, GoalID: f.goalID, MandateID: f.mandateID, Purpose: "native_research", Goal: "Fixed scripted observability only", CompletionCriteria: []string{"No model acceptance"}, InitiativeReason: "explicit isolated diagnostic", ObservationRef: "fixture:scripted"}
}

func TestNativeDelegationScriptedOwnedObservabilityIntegration(t *testing.T) {
	if os.Getenv("BUTLER_DELEGATION_BROKER_BINARY") == "" {
		t.Skip("actual private Broker binary required")
	}
	for _, c := range []struct {
		name  string
		width int
		deny  bool
	}{{"width-one", 1, false}, {"width-two", 2, false}, {"provider-denial", 1, true}} {
		t.Run(c.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 40*time.Second)
			defer cancel()
			f := newNativeScenarioFixture(t, ctx)
			admission, broker := nativeDiagnosticBroker(t, f, c.width)
			key := "scripted-parent-" + c.name
			if _, _, e := f.sessions.CreateSession(ctx, session.CreateSessionParams{SessionKey: key, UserID: nativeFixtureOwner, Channel: "web", MetadataJSON: `{}`}); e != nil {
				t.Fatal("scripted parent session denied")
			}
			parent, e := f.goals.CreateOwnerTask(nativeOwner(ctx, key, key), nativeScriptedTaskRequest(f, key))
			if e != nil {
				t.Fatal("scripted parent assignment denied")
			}
			event, e := ingress.NormalizeEvent(ingress.NormalizeRequest{EventType: runv1.InputEventType_INPUT_EVENT_TYPE_USER_MESSAGE, EventID: key, SessionKey: key, Source: "web", Payload: map[string]any{"user_id": nativeFixtureOwner, "task_id": parent.TaskID, "text": "Fixed two-source zero-network diagnostic"}, DeliveryID: key})
			if e != nil {
				t.Fatal("scripted input fixture denied")
			}
			receipt, _, e := f.inputs.Accept(nativeOwner(ctx, key, key), event, nativeFixtureOwner, parent.TaskID)
			if e != nil {
				t.Fatal("scripted ordered input denied")
			}
			provider := &nativeScriptedObservationProvider{args: f.args, receipts: map[string][]string{}, denySecond: c.deny}
			reader := &nativeDiagnosticContextReader{sources: f.sources, manager: admission.Manager}
			log := slog.New(slog.NewTextHandler(io.Discard, nil))
			runs := runservice.NewService(runservice.NewPostgresRepository(f.pool), log)
			worker := durableinput.NewService(f.inputs, f.sessions, nativeDiagnosisCore(f, provider, broker, reader, f.inputs), runs, log)
			worker.SetMaintenance((&delegationMaintenance{pool: f.pool, manager: admission.Manager}).Sweep)
			workerCtx, stop := context.WithCancel(ctx)
			done := make(chan error, 1)
			go func() { done <- worker.Run(workerCtx) }()
			path := nativeAuditTestPath(t, "scripted-"+c.name)
			var audit nativeOwnedAudit
			ownerStopped := false
			// This defer executes before memorytest t.Cleanup even after t.Fatal.
			// First stop/wait every actual owning worker, then capture/export.
			defer func() {
				stop()
				select {
				case <-done:
				case <-time.After(8 * time.Second):
					t.Error("owning diagnostic worker stop deadline")
					return
				}
				var err error
				audit, err = collectNativeOwnedAudit(context.Background(), f.pool, true)
				if err != nil || writeNativeOwnedAudit(path, audit) != nil {
					t.Error("bounded owned diagnostic export unavailable")
				}
			}()
			nativeWait(t, ctx, 20*time.Second, func() bool {
				var state string
				var denied int
				if f.pool.QueryRow(ctx, `SELECT status FROM accepted_inputs WHERE input_id=$1`, receipt.InputID).Scan(&state) != nil {
					return false
				}
				if c.deny {
					return state == "blocked"
				}
				if state != "completed" {
					return false
				}
				return f.pool.QueryRow(ctx, `SELECT count(*) FROM accepted_inputs i JOIN tasks t USING(task_id) WHERE t.parent_task_id=$1 AND i.status='blocked'`, parent.TaskID).Scan(&denied) == nil && denied == c.width
			})
			var snapshots, confirmed, attempts int
			if f.pool.QueryRow(ctx, `SELECT count(*) FROM delegation_admission_snapshots`).Scan(&snapshots) != nil || f.pool.QueryRow(ctx, `SELECT count(*) FROM action_effects WHERE state='effect_confirmed'`).Scan(&confirmed) != nil || f.pool.QueryRow(ctx, `SELECT attempts FROM fixture_native_limit`).Scan(&attempts) != nil {
				t.Fatal("scripted actual admission/effect metadata unavailable")
			}
			expected := 2
			if c.deny {
				expected = 1
			}
			if snapshots != expected || confirmed != expected || attempts != nativeDelegationPriorAttempts || provider.parentStarts.Load() != 1 || provider.continuations.Load() != int32(expected) || !c.deny && provider.childEntries.Load() != int32(c.width) {
				t.Fatal("scripted flow did not traverse exact actual admission boundaries")
			}
			if c.width == 2 && !c.deny {
				// Both actual core entries failed without a child final. A separate
				// authenticated owner Stop must durably terminate the branch rather
				// than letting process shutdown imply task/delegation cancellation.
				var failed int
				if f.pool.QueryRow(ctx, `SELECT count(*) FROM runs r JOIN tasks t USING(task_id) WHERE t.parent_task_id=$1 AND r.status='failed' AND NOT EXISTS(SELECT 1 FROM run_final_results z WHERE z.run_id=r.run_id)`, parent.TaskID).Scan(&failed) != nil || failed != 2 {
					t.Fatal("actual failed-child/no-final baseline unavailable")
				}
				leases := session.NewPostgresLeaseManager(f.pool)
				controls := task.NewController(f.tasks, leases.Authority(), leases, runservice.NewPostgresRepository(f.pool), approvals.NewPostgresRepository(f.pool), f.inputs, transcript.NewStore(f.pool))
				if _, err := controls.Apply(nativeOwner(ctx, "failed-provider-owner-stop", parent.SessionKey), task.ControlRequest{TaskID: parent.TaskID, ExpectedRevision: parent.Revision, Kind: task.ControlStop}); err != nil {
					t.Fatal("actual owner stop after child provider failure denied", err)
				}
				var children, active int
				if f.pool.QueryRow(ctx, `SELECT count(*) FROM tasks WHERE parent_task_id=$1 AND status='cancelled'`, parent.TaskID).Scan(&children) != nil || children != 2 || f.pool.QueryRow(ctx, `SELECT count(*) FROM accepted_inputs WHERE status IN ('queued','running')`).Scan(&active) != nil || active != 0 {
					t.Fatal("owner stop did not durably terminate failed children/inputs")
				}
				if _, err := admission.Manager.Advance(nativeOwner(ctx, "failed-provider-stopped-advance", ""), parent.TaskID); err == nil {
					t.Fatal("failed branch continued after actual owner stop")
				}
				if f.pool.QueryRow(ctx, `SELECT count(*) FROM action_effects WHERE state='effect_confirmed'`).Scan(&confirmed) != nil || confirmed != 2 || f.pool.QueryRow(ctx, `SELECT attempts FROM fixture_native_limit`).Scan(&attempts) != nil || attempts != nativeDelegationPriorAttempts {
					t.Fatal("failed branch termination lost effects or added inference")
				}
				ownerStopped = true
			}
			// Register after the export defer was created, but cleanup callbacks
			// always run later. It proves bytes were exported before schema DROP.
			t.Cleanup(func() {
				b, err := os.ReadFile(path)
				if err != nil || strings.Contains(string(b), "SECRET_DIAGNOSTIC_MARKER") || !audit.Available || !audit.WorkersStopped || audit.Status != "captured_before_cleanup" {
					t.Error("content-free before-cleanup export missing")
					return
				}
				blocked, actions, finals, cancelled, failedRuns := 0, 0, 0, 0, 0
				for _, r := range audit.Rows {
					if r.Kind == "input" && r.Status == "blocked" {
						blocked++
					}
					if r.Kind == "action" && r.State == "effect_confirmed" {
						actions++
					}
					if r.Kind == "input" && r.Status == "cancelled" {
						cancelled++
					}
					if r.Kind == "run" && r.Status == "failed" {
						failedRuns++
					}
					if r.Kind == "final" {
						finals++
						if r.Status != "unverified" {
							t.Error("scripted final promoted authority")
						}
					}
				}
				if actions != expected || !ownerStopped && blocked < 1 || ownerStopped && (cancelled != 2 || failedRuns != 2) || !c.deny && finals != 1 || c.deny && finals != 0 {
					t.Error("owned export lost actual stopped boundary")
				}
				t.Logf("zero_inference=true width=%d actual_admissions=%d child_entry_denials=%d durable_owner_stop=%t stopped_state_export=true audit_sha256=%s", c.width, actions, provider.childEntries.Load(), ownerStopped, nativeAuditRef(string(b)))
			})
		})
	}
}
