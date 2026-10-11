package goal_test

import (
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/butler/butler/apps/orchestrator/internal/api"
	"github.com/butler/butler/apps/orchestrator/internal/completion"
	"github.com/butler/butler/apps/orchestrator/internal/task"
	"github.com/butler/butler/apps/orchestrator/internal/tools"
	"github.com/butler/butler/internal/actionauth"
	commonv1 "github.com/butler/butler/internal/gen/common/v1"
	toolbrokerv1 "github.com/butler/butler/internal/gen/toolbroker/v1"
	"github.com/butler/butler/internal/generatedtools"
	"github.com/butler/butler/internal/ownership"
	"github.com/jackc/pgx/v5"
)

type converterRunEffects struct{ journal *actionauth.PostgresStore }

func (r converterRunEffects) ReadRunEffects(ctx context.Context, run string) ([]completion.Effect, error) {
	items, e := r.journal.GetRunEvidence(ctx, run)
	result := []completion.Effect{}
	for _, v := range items {
		result = append(result, completion.Effect{ActionID: v.ActionID, EffectState: string(v.EffectState), EvidenceRefs: v.EvidenceRefs})
	}
	return result, e
}

// The agent helper starts the actual production converter in a separate
// network-none container, with this client and the production Broker sharing
// only a read-only fixed Unix socket. All original input and program bytes are
// Catalog DATA. This is Broker/runtime proof, not a delegated-child proof.
func TestGeneratedConverterRealBrokerContainerE2E(t *testing.T) {
	if os.Getenv("BUTLER_CONVERTER_BROKER_E2E") != "1" {
		t.Skip("explicit isolated production Broker/container harness required")
	}
	profile, err := generatedtools.DecodeProfile([]byte(os.Getenv("BUTLER_CONVERTER_PROFILE_JSON")))
	if err != nil {
		t.Fatal(err)
	}
	f := setupConverter(t, profile)
	coreToken := strings.Repeat("core-converter-fixture-", 3)
	coreFile := filepath.Join(t.TempDir(), "core.token")
	if err = os.WriteFile(coreFile, []byte(coreToken), 0600); err != nil {
		t.Fatal(err)
	}
	authority := httptest.NewServer(actionauth.NewContextHandler(coreToken, f.env))
	defer authority.Close()
	address := func() string {
		listener, e := net.Listen("tcp", "127.0.0.1:0")
		if e != nil {
			t.Fatal(e)
		}
		s := listener.Addr().String()
		listener.Close()
		return s
	}
	grpcAddress, httpAddress := address(), address()
	broker := exec.Command(os.Getenv("BUTLER_CONVERTER_BROKER_BINARY"))
	// Neither the runtime nor a model chooses this configuration. The Broker
	// receives only the explicit isolated schema, core service and runtime tokens.
	broker.Env = []string{"PATH=" + os.Getenv("PATH"), "BUTLER_ENVIRONMENT=test", "BUTLER_GRPC_ADDR=" + grpcAddress, "BUTLER_HTTP_ADDR=" + httpAddress,
		"BUTLER_POSTGRES_URL=" + f.f.pool.Config().ConnString(), "BUTLER_TOOL_REGISTRY_PATH=/src/configs/tools.json", "BUTLER_ACTION_CONTEXT_URL=" + authority.URL,
		"BUTLER_ACTION_AUTHORITY_TOKEN_FILE=" + coreFile, "BUTLER_OWNER_CONTROL_OWNER_ID=" + f.f.owner, "BUTLER_SKILLS_ENABLED=true", "BUTLER_CONVERTERS_ENABLED=true",
		"BUTLER_CONVERTER_PROFILE_JSON=" + os.Getenv("BUTLER_CONVERTER_PROFILE_JSON"), "BUTLER_CONVERTER_RUNTIME_TARGET=" + generatedtools.RuntimeTarget,
		"BUTLER_CONVERTER_RUNTIME_TOKEN_FILE=" + os.Getenv("BUTLER_CONVERTER_RUNTIME_TOKEN_FILE"), "BUTLER_MEMORY_DELETION_MANIFEST_FILE=" + f.manifestPath}
	broker.Stdout, broker.Stderr = io.Discard, io.Discard
	if err = broker.Start(); err != nil {
		t.Fatal("isolated production Broker did not start")
	}
	t.Cleanup(func() { _ = broker.Process.Signal(os.Interrupt); _ = broker.Wait() })
	until := time.Now().Add(10 * time.Second)
	for {
		c, e := net.DialTimeout("tcp", grpcAddress, 100*time.Millisecond)
		if e == nil {
			c.Close()
			break
		}
		if time.Now().After(until) {
			t.Fatal("production Broker startup failed")
		}
		time.Sleep(20 * time.Millisecond)
	}
	client, err := tools.DialWithTokenFile(grpcAddress, coreFile)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	sequence := 0
	call := func(name string, args any) *toolbrokerv1.ToolCall {
		sequence++
		body, _ := json.Marshal(args)
		return &toolbrokerv1.ToolCall{RunId: f.env.run, ToolCallId: "real-converter-" + string(rune('a'+sequence)), ToolName: name, ArgsJson: string(body), AutonomyMode: commonv1.AutonomyMode_AUTONOMY_MODE_2}
	}
	dispatch := func(call *toolbrokerv1.ToolCall, out any) {
		t.Helper()
		prepared, e := client.PrepareAction(f.f.ctx, call)
		if e != nil || prepared.RequiresApproval || len(prepared.Binding.DataSources) == 0 {
			t.Fatal("exact current converter mandate/source preparation", e)
		}
		grant, e := client.AuthorizeAction(f.f.ctx, call, prepared.Binding.ActionID, "", "")
		if e != nil {
			t.Fatal("converter grant", e)
		}
		result, e := client.ExecuteAuthorizedToolCall(f.f.ctx, call, grant)
		if e != nil || result.GetStatus() != "completed" || result.GetEffectState() != string(actionauth.StateConfirmed) || len(result.GetEvidenceRefs()) != 1 {
			t.Fatalf("production converter %s lacks local preparation evidence: %s %s %v", call.ToolName, result.GetStatus(), result.GetError().GetMessage(), e)
		}
		if strings.Contains(result.GetResultJson(), "widget") || strings.Contains(result.GetResultJson(), "delimited_to_objects") || strings.Contains(result.GetResultJson(), coreToken) {
			t.Fatal("working document, program or service token exposed as tool context")
		}
		if out != nil && json.Unmarshal([]byte(result.GetResultJson()), out) != nil {
			t.Fatal("invalid typed converter result")
		}
		// Reusing the actual consumed grant cannot cause another developer/test/
		// conversion invocation or publication.
		if replay, e := client.ExecuteAuthorizedToolCall(f.f.ctx, call, grant); e == nil && replay.GetStatus() != "failed" {
			t.Fatal("consumed converter grant replayed")
		}
	}
	dispatch(call("converters.discover", map[string]any{"source_ref": f.needRef}), nil)
	var candidate generatedtools.Record
	dispatch(call("converters.develop", map[string]any{"source_ref": f.needRef, "version": "1.0.0"}), &candidate)
	var report generatedtools.Report
	dispatch(call("converters.test", map[string]any{"digest": candidate.Digest}), &report)
	if !report.Passed || report.Checker != generatedtools.Checker {
		t.Fatal("independent production checker did not pass")
	}
	var trial generatedtools.TrialResult
	dispatch(call("converters.trial", map[string]any{"digest": candidate.Digest, "input_ref": f.inputRef}), &trial)
	if trial.Status != "passed" || trial.Output == nil {
		t.Fatal("bounded production trial missing")
	}
	var active generatedtools.Record
	dispatch(call("converters.activate", map[string]any{"digest": candidate.Digest, "generation": candidate.Generation}), &active)
	if !active.Active || active.Generation != candidate.Generation+1 {
		t.Fatal("actual catalog version activation missing")
	}
	var converted generatedtools.TrialResult
	dispatch(call("converter.inventory.convert", map[string]any{"input_ref": f.inputRef}), &converted)
	if converted.Output == nil || converted.Output.InstructionAuthority {
		t.Fatal("checked output DATA source missing")
	}
	output, err := f.sources.Get(f.f.actor("review-output"), converted.Output.Source.ID)
	if err != nil || output.Body != `[{"label":"widget","quantity":2}]` || generatedtools.HashBytes([]byte(output.Body)) != converted.Output.SHA256 {
		t.Fatal("original document did not produce independently checkable JSON")
	}
	// The local technical acknowledgement does not complete the semantic task.
	var taskStatus string
	if f.f.pool.QueryRow(f.f.ctx, `SELECT status FROM tasks WHERE task_id=(SELECT task_id FROM runs WHERE run_id=$1)`, f.env.run).Scan(&taskStatus) != nil || taskStatus != "active" {
		t.Fatal("converter result invented semantic completion")
	}
	ownerToken := strings.Repeat("converter-owner-fixture-", 3)
	control, err := generatedtools.NewClient("http://"+httpAddress+"/internal/converters", coreToken)
	if err != nil {
		t.Fatal(err)
	}
	ownerAPI := httptest.NewServer(api.GeneratedConverters(api.NewOwnerControlAuth(ownerToken, f.f.owner), control))
	defer ownerAPI.Close()
	req, _ := http.NewRequest("GET", ownerAPI.URL, nil)
	unauthorized, e := http.DefaultClient.Do(req)
	if e != nil || unauthorized.StatusCode != 401 {
		t.Fatal("private converter review exposed without owner proof")
	}
	unauthorized.Body.Close()
	req.Header.Set("Authorization", "Bearer "+ownerToken)
	response, e := http.DefaultClient.Do(req)
	if e != nil {
		t.Fatal(e)
	}
	defer response.Body.Close()
	var view generatedtools.ControlResponse
	if json.NewDecoder(response.Body).Decode(&view) != nil || response.StatusCode != 200 || len(view.Items) != 1 || view.Items[0].Uses != 1 || view.Items[0].PassedTrials != 1 || view.Items[0].Checks == nil || !view.Items[0].Checks.Passed || len(view.Items[0].Outputs) != 1 {
		t.Fatal("owner review lacks actual version/checks/limits/output evidence")
	}
	// No arbitrary implementation or new credentials can enter the production
	// Broker even when the owner granted this exact converter class.
	for _, args := range []any{map[string]any{"source_ref": f.needRef, "version": "2.0.0", "code": "exec"}, map[string]any{"source_ref": f.needRef, "version": "2.0.0", "credential_ref": map[string]string{"alias": "account"}}} {
		if _, e = client.PrepareAction(f.f.ctx, call("converters.develop", args)); e == nil {
			t.Fatal("generated tool widened core/process/credential surface")
		}
	}
	current, e := f.env.GetActionContext(f.f.ctx, f.env.run)
	if e != nil {
		t.Fatal(e)
	}
	owned := ownership.WithToken(f.f.ctx, f.f.leases.Authority(), ownership.Token{SessionKey: current.SessionKey, RunID: current.RunID, LeaseID: current.LeaseID, FenceEpoch: current.LeaseEpoch})
	if _, e = f.f.pool.Exec(f.f.ctx, `UPDATE runs SET status='finalizing',current_state='finalizing' WHERE run_id=$1`, current.RunID); e != nil {
		t.Fatal(e)
	}
	finalizer := completion.New(f.f.pool)
	finalizer.Evidence = converterRunEffects{journal: f.journal}
	finalizer.DataGuard = func(ctx context.Context, tx pgx.Tx, run string) error {
		if e := f.sources.CheckRunSourcesTx(ctx, tx, run); e != nil {
			return e
		}
		return generatedtools.CheckPinnedLifecycleTx(ctx, tx, run)
	}
	content, _ := json.Marshal(converted.Output)
	final, e := finalizer.Save(owned, completion.Request{RunID: current.RunID, SessionKey: current.SessionKey, TaskID: current.TaskID, TaskRevision: current.TaskRevision, ConstraintsRevision: current.ConstraintsRevision, Content: string(content)})
	if e != nil || final.Verdict != "unverified" || len(final.Effects) != 6 {
		t.Fatal("atomic original-task result lost truthful converter evidence", e)
	}
	// The fixture owner independently checked the original document and output
	// above. Only that explicit trusted owner verdict, not runtime completed or
	// model text, may now satisfy the existing task completion criteria.
	record, e := f.f.tasks.Get(f.f.ctx, current.TaskID)
	if e != nil {
		t.Fatal(e)
	}
	verified, e := f.f.tasks.SaveVerifiedResult(f.f.actor("owner-verifies-original-document"), task.Result{TaskID: record.TaskID, TaskRevision: record.Revision, RunID: current.RunID, Summary: "Owner checked exact original document conversion and fixed independent checks", EvidenceRefs: []string{"result:" + final.ResultID, "source:" + converted.Output.Source.ID, "sha256:" + converted.Output.SHA256}, VerifiedCriteria: record.CompletionCriteria, Verifier: "owner"})
	if e != nil {
		t.Fatal("independent trusted owner verification", e)
	}
	if record, e = f.f.tasks.Transition(f.f.actor("owner-completes-original-document"), record.TaskID, record.Revision, "completed", verified.ResultID); e != nil || record.Status != "completed" {
		t.Fatal("verified original task did not complete", e)
	}
}
