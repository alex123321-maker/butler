package app

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
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
	approvals "github.com/butler/butler/apps/orchestrator/internal/approval"
	runservice "github.com/butler/butler/apps/orchestrator/internal/run"
	"github.com/butler/butler/apps/orchestrator/internal/session"
	"github.com/butler/butler/apps/orchestrator/internal/task"
	"github.com/butler/butler/apps/orchestrator/internal/tools"
	"github.com/butler/butler/internal/actionauth"
	"github.com/butler/butler/internal/extensions"
	"github.com/butler/butler/internal/extensions/projectfixture"
	commonv1 "github.com/butler/butler/internal/gen/common/v1"
	runv1 "github.com/butler/butler/internal/gen/run/v1"
	runtimev1 "github.com/butler/butler/internal/gen/runtime/v1"
	sessionv1 "github.com/butler/butler/internal/gen/session/v1"
	toolbrokerv1 "github.com/butler/butler/internal/gen/toolbroker/v1"
	"github.com/butler/butler/internal/ownership"
	"github.com/butler/butler/internal/security/egress/egresstest"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

func extensionChild(t *testing.T, binary, grpcAddress, httpAddress string, args []string, environment ...string) {
	t.Helper()
	cmd := exec.Command(binary, args...)
	// The executor child receives no parent PG/provider/owner environment. The
	// Broker gets only the explicit test DSN and core service token below.
	cmd.Env = append([]string{"PATH=" + os.Getenv("PATH"), "BUTLER_ENVIRONMENT=test", "BUTLER_GRPC_ADDR=" + grpcAddress, "BUTLER_HTTP_ADDR=" + httpAddress}, environment...)
	cmd.Stdout, cmd.Stderr = io.Discard, io.Discard
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = cmd.Process.Signal(os.Interrupt); _ = cmd.Wait() })
	until := time.Now().Add(10 * time.Second)
	for {
		conn, err := net.DialTimeout("tcp", grpcAddress, 100*time.Millisecond)
		if err == nil {
			conn.Close()
			return
		}
		if time.Now().After(until) {
			t.Fatal("isolated extension service did not start")
		}
		time.Sleep(20 * time.Millisecond)
	}
}
func TestInstalledProjectExtensionRealBrokerApprovalE2E(t *testing.T) {
	if os.Getenv("BUTLER_EXTENSION_E2E") != "1" {
		t.Skip("isolated real-process extension acceptance")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	pool, dsn := e2eDatabase(t)
	sessions := session.NewPostgresRepository(pool)
	sessionKey := "web:owner"
	if _, _, err := sessions.CreateSession(ctx, session.CreateSessionParams{SessionKey: sessionKey, UserID: "owner", Channel: "web", MetadataJSON: "{}"}); err != nil {
		t.Fatal(err)
	}
	runs := runservice.NewService(runservice.NewPostgresRepository(pool), nil)
	run, err := runs.CreateRun(ctx, &sessionv1.CreateRunRequest{SessionKey: sessionKey, InputEvent: &runv1.InputEvent{EventId: "extension-fixture-input"}, AutonomyMode: commonv1.AutonomyMode_AUTONOMY_MODE_2, ModelProvider: "fixture", MetadataJson: "{}"})
	if err != nil {
		t.Fatal(err)
	}
	leases := session.NewPostgresLeaseManager(pool)
	lease, err := leases.AcquireLease(ctx, session.AcquireLeaseParams{SessionKey: sessionKey, RunID: run.GetRunId(), OwnerID: "worker", TTL: 3 * time.Minute})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, `UPDATE runs SET status='tool_pending',current_state='tool_pending',lease_id=$2 WHERE run_id=$1`, run.GetRunId(), lease.LeaseID); err != nil {
		t.Fatal(err)
	}
	tasks := task.NewStore(pool)
	record, err := tasks.Create(ctx, task.CreateRequest{SessionKey: sessionKey, OwnerID: "owner", Goal: "Prepare a reviewed fixture patch", Constraints: []string{"Only the exact synthetic README; leave unapplied"}, CompletionCriteria: []string{"prepared patch checks pass"}})
	if err != nil {
		t.Fatal(err)
	}
	if err = tasks.AttachRun(ctx, record.TaskID, record.Revision, run.GetRunId(), sessionKey); err != nil {
		t.Fatal(err)
	}
	owned := ownership.WithToken(ctx, leases.Authority(), ownership.Token{SessionKey: sessionKey, RunID: run.GetRunId(), LeaseID: lease.LeaseID, FenceEpoch: lease.FenceEpoch})
	repo := approvals.NewPostgresRepository(pool)
	repo.SetCheckpointHooks(boundaryCheckpointHooks{})
	approvalService := approvals.NewService(repo, nil)
	coreToken := strings.Repeat("core-service-fixture-", 3)
	runtimeToken := strings.Repeat("runtime-service-fixture-", 3)
	ownerToken := strings.Repeat("owner-control-fixture-", 3)
	reader := &actionContextReader{runs: runs, sessions: sessions, leases: leases, tasks: tasks, approvals: approvalService, pool: pool}
	authority := httptest.NewServer(actionauth.NewContextHandler(coreToken, reader))
	defer authority.Close()
	fixture, err := projectfixture.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer fixture.Close()
	site := httptest.NewServer(fixture)
	defer site.Close()
	runtimeAddress, brokerAddress, brokerHTTP := freeAddress(t), freeAddress(t), freeAddress(t)
	runtimeBinary := os.Getenv("BUTLER_EXTENSION_E2E_BINARY")
	binaryBytes, err := os.ReadFile(runtimeBinary)
	if err != nil {
		t.Fatal("fixture runtime artifact unavailable")
	}
	sum := sha256.Sum256(binaryBytes)
	p, err := projectfixture.Manifest(hex.EncodeToString(sum[:]), "operator-reviewed-fixture-v1")
	if err != nil {
		t.Fatal(err)
	}
	profiles := map[string]extensions.Profile{"reference-http": {Target: runtimeAddress, Dependency: p.Manifest.Dependencies[0]}}
	profileJSON, _ := json.Marshal(profiles)
	file := func(name, value string) string {
		path := filepath.Join(t.TempDir(), name)
		if err := os.WriteFile(path, []byte(value), 0600); err != nil {
			t.Fatal(err)
		}
		return path
	}
	coreFile, runtimeFile := file("core.token", coreToken), file("runtime.token", runtimeToken)
	extensionChild(t, runtimeBinary, runtimeAddress, "127.0.0.1:0", []string{"-test.run=^TestExtensionRuntimeEgressHarness$"}, "BUTLER_EXTENSION_EGRESS_HARNESS=1", "BUTLER_EXTENSION_FIXTURE_ENDPOINT="+site.URL, "BUTLER_EXTENSION_PROFILES_JSON="+string(profileJSON), "BUTLER_EXTENSION_RUNTIME_TOKEN_FILE="+runtimeFile)
	unauthorized, err := grpc.NewClient(runtimeAddress, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatal(err)
	}
	defer unauthorized.Close()
	if _, err = runtimev1.NewToolRuntimeServiceClient(unauthorized).ProbeExtension(ctx, &runtimev1.ProbeExtensionRequest{ProbeId: "forged"}); err == nil {
		t.Fatal("external executor accepted unauthenticated RPC")
	}
	extensionChild(t, os.Getenv("BUTLER_BROKER_E2E_BINARY"), brokerAddress, brokerHTTP, nil, "BUTLER_POSTGRES_URL="+dsn, "BUTLER_TOOL_REGISTRY_PATH=configs/tools.json", "BUTLER_ACTION_CONTEXT_URL="+authority.URL, "BUTLER_ACTION_AUTHORITY_TOKEN_FILE="+coreFile, "BUTLER_EXTENSION_PROFILES_JSON="+string(profileJSON), "BUTLER_EXTENSION_RUNTIME_TOKEN_FILE="+runtimeFile, "BUTLER_OWNER_CONTROL_OWNER_ID=owner")
	controlServer := httptest.NewServer(api.ReviewedExtensions(api.NewOwnerControlAuth(ownerToken, "owner"), extensions.NewControlClient("http://"+brokerHTTP+"/internal/extensions", coreToken)))
	defer controlServer.Close()
	control := func(req extensions.ControlRequest) extensions.ControlResponse {
		t.Helper()
		body, _ := json.Marshal(req)
		request, _ := http.NewRequestWithContext(ctx, "POST", controlServer.URL, strings.NewReader(string(body)))
		request.Header.Set("Authorization", "Bearer "+ownerToken)
		request.Header.Set("X-Butler-Delivery-ID", "owner-extension-"+req.Operation)
		response, err := http.DefaultClient.Do(request)
		if err != nil {
			t.Fatal(err)
		}
		defer response.Body.Close()
		var out extensions.ControlResponse
		if json.NewDecoder(response.Body).Decode(&out) != nil || response.StatusCode != 200 || out.Error != "" {
			t.Fatalf("owner operation %s failed: %s", req.Operation, out.Error)
		}
		return out
	}
	denied, _ := http.Get(controlServer.URL)
	denied.Body.Close()
	if denied.StatusCode != 401 {
		t.Fatal("unverified owner read installation history")
	}
	control(extensions.ControlRequest{Operation: "stage", ManifestJSON: p.Raw})
	control(extensions.ControlRequest{Operation: "review", ExtensionID: "project", ManifestHash: p.Digest})
	state := control(extensions.ControlRequest{Operation: "list"}).Items[0].Record
	selected := []string{"extension.project.inspect", "extension.project.prepare-patch", "extension.project.check"}
	state = control(extensions.ControlRequest{Operation: "configure", ExtensionID: "project", ManifestHash: p.Digest, Generation: state.Generation, Configuration: map[string]string{}, Tools: selected}).Item.Record
	if !strings.Contains(state.HealthReason, "origin") || state.Available {
		t.Fatal("missing mandatory origin was hidden")
	}
	state = control(extensions.ControlRequest{Operation: "configure", ExtensionID: "project", ManifestHash: p.Digest, Generation: state.Generation, Configuration: map[string]string{"origin": egresstest.Origin}, Tools: selected}).Item.Record
	control(extensions.ControlRequest{Operation: "probe", ExtensionID: "project", Generation: state.Generation})
	state = control(extensions.ControlRequest{Operation: "enable", ExtensionID: "project", Generation: state.Generation}).Item.Record
	client, err := tools.DialWithTokenFile(brokerAddress, coreFile)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	catalog, err := client.ListToolsForRun(ctx, run.GetRunId())
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range catalog {
		if c.GetExtension() != nil {
			t.Fatal("enablement granted task access")
		}
	}
	control(extensions.ControlRequest{Operation: "grant_task", ExtensionID: "project", Access: extensions.TaskAccess{TaskID: record.TaskID, TaskRevision: record.Revision, ConstraintsRevision: record.ConstraintsRevision, ManifestHash: p.Digest, Generation: state.Generation, Tools: selected, Resources: []actionauth.ResourceScope{{Origin: egresstest.Origin, PathPrefix: "/project"}}, ExpiresAt: time.Now().Add(time.Hour)}})
	catalog, err = client.ListToolsForRun(ctx, run.GetRunId())
	if err != nil {
		t.Fatal(err)
	}
	count := 0
	for _, c := range catalog {
		if c.GetExtension() != nil {
			count++
		}
	}
	if count != 3 {
		t.Fatal("current reviewed extension catalog unavailable")
	}
	sequence := 0
	call := func(name, path string, body any) *toolbrokerv1.ToolCall {
		sequence++
		args := map[string]any{"url": egresstest.Origin + path}
		if body != nil {
			args["body"] = body
		}
		raw, _ := json.Marshal(args)
		return &toolbrokerv1.ToolCall{RunId: run.GetRunId(), ToolCallId: fmt.Sprintf("extension-call-%d", sequence), ToolName: name, ArgsJson: string(raw), AutonomyMode: commonv1.AutonomyMode_AUTONOMY_MODE_2}
	}
	prepare := func(call *toolbrokerv1.ToolCall) (actionauth.PreparedAction, string) {
		t.Helper()
		p, err := client.PrepareAction(ctx, call)
		if err != nil {
			t.Fatal(err)
		}
		expiry := p.ExpiresAt
		a, err := approvalService.CreatePendingApproval(owned, approvals.CreatePendingParams{RunID: run.GetRunId(), SessionKey: sessionKey, ToolCallID: call.ToolCallId, ToolName: call.ToolName, ArgsJSON: call.ArgsJson, RequestedVia: "web", ActionBinding: p.Binding, ContinuationJSON: `{"version":1}`, Summary: p.Summary, ExpiresAt: &expiry})
		if err != nil {
			t.Fatal(err)
		}
		actor := approvals.WithActor(ctx, approvals.Actor{OwnerID: "owner", Channel: "web", SessionKey: sessionKey, DeliveryID: "approve-" + call.ToolCallId})
		if _, _, err = approvalService.ResolveByApprovalID(actor, approvals.ResolveByApprovalIDParams{ApprovalID: a.ApprovalID, Approved: true, ResolvedVia: "web", ActorType: "web", ActorID: "owner", ResolvedBy: "owner"}); err != nil {
			t.Fatal(err)
		}
		return p, a.ApprovalID
	}
	dispatch := func(call *toolbrokerv1.ToolCall) map[string]any {
		t.Helper()
		p, approval := prepare(call)
		grant, err := client.AuthorizeAction(ctx, call, p.Binding.ActionID, approval, "")
		if err != nil {
			t.Fatal(err)
		}
		result, err := client.ExecuteAuthorizedToolCall(ctx, call, grant)
		if err != nil || result.GetStatus() != "completed" || result.GetEffectState() != string(actionauth.StateConfirmed) || len(result.GetEvidenceRefs()) != 1 {
			t.Fatalf("installed connector dispatch lacks verified journal: %+v %v", result, err)
		}
		var envelope struct{ Body string }
		var value map[string]any
		if json.Unmarshal([]byte(result.GetResultJson()), &envelope) != nil || json.Unmarshal([]byte(envelope.Body), &value) != nil {
			t.Fatal("invalid stored connector observation")
		}
		if _, err = pool.Exec(ctx, `UPDATE runs SET status='tool_pending',current_state='tool_pending' WHERE run_id=$1`, run.GetRunId()); err != nil {
			t.Fatal(err)
		}
		return value
	}
	inspect := dispatch(call("extension.project.inspect", "/project", nil))
	if inspect["checks_passed"] != false || inspect["revision_sha256"] != projectfixture.Revision() {
		t.Fatal("fixture inspection misrepresented check")
	}
	proposal := projectfixture.Proposal{ProjectRevision: projectfixture.Revision(), Changes: []projectfixture.Change{{Path: "README.md", Before: projectfixture.Original, After: projectfixture.Prepared}}}
	patch := dispatch(call("extension.project.prepare-patch", "/project/patches", proposal))
	artifact, _ := patch["artifact_id"].(string)
	if artifact == "" {
		t.Fatal("recipient artifact absent")
	}
	checked := dispatch(call("extension.project.check", "/project/patches/"+artifact+"/checks", nil))
	if checked["checks_passed"] != true || checked["applied"] != false {
		t.Fatal("prepared patch check lost verification boundary")
	}
	pending := call("extension.project.inspect", "/project", nil)
	saved, approvalID := prepare(pending)
	state = control(extensions.ControlRequest{Operation: "disable", ExtensionID: "project", Generation: state.Generation}).Item.Record
	if _, err = client.AuthorizeAction(ctx, pending, saved.Binding.ActionID, approvalID, ""); err == nil {
		t.Fatal("disabled installed capability reused saved owner approval")
	}
	versions := control(extensions.ControlRequest{Operation: "versions", ExtensionID: "project"}).Versions
	if len(versions) != 1 || versions[0].Digest != p.Digest {
		t.Fatal("disable lost immutable version history")
	}
}
