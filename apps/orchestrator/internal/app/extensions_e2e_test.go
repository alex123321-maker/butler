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
	"sync/atomic"
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
	memorycatalog "github.com/butler/butler/internal/memory/catalog"
	"github.com/butler/butler/internal/ownership"
	"github.com/butler/butler/internal/security/egress/egresstest"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/protobuf/proto"
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
	manifestPath := filepath.Join(t.TempDir(), "deletions.log")
	manifest := memorycatalog.NewFileManifest(manifestPath)
	if err = manifest.Initialize(); err != nil {
		t.Fatal(err)
	}
	sourceCatalog := memorycatalog.New(pool, manifest)
	if err = sourceCatalog.PinTranscript(owned, sessionKey, 0); err != nil {
		t.Fatal("owned extension transcript source pin", err)
	}
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
	fixtureSecret := "synthetic-extension-private-token"
	var contacted atomic.Int32
	site := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		contacted.Add(1)
		if request.URL.Path == "/project/private" {
			if request.Header.Get("Authorization") != "Bearer "+fixtureSecret {
				w.WriteHeader(http.StatusForbidden)
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"checks_passed": false, "reflected_token": fixtureSecret})
			return
		}
		fixture.ServeHTTP(w, request)
	}))
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
	extensionChild(t, os.Getenv("BUTLER_BROKER_E2E_BINARY"), brokerAddress, brokerHTTP, nil, "BUTLER_POSTGRES_URL="+dsn, "BUTLER_TOOL_REGISTRY_PATH=configs/tools.json", "BUTLER_ACTION_CONTEXT_URL="+authority.URL, "BUTLER_ACTION_AUTHORITY_TOKEN_FILE="+coreFile, "BUTLER_EXTENSION_PROFILES_JSON="+string(profileJSON), "BUTLER_EXTENSION_RUNTIME_TOKEN_FILE="+runtimeFile, "BUTLER_OWNER_CONTROL_OWNER_ID=owner", "BUTLER_MEMORY_DELETION_MANIFEST_FILE="+manifestPath)
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
	var inspectContract *toolbrokerv1.ToolContract
	for _, contract := range catalog {
		if contract.ToolName == "extension.project.inspect" {
			inspectContract = contract
		}
	}
	if inspectContract == nil {
		t.Fatal("reviewed inspect contract absent")
	}
	// These probes use the authenticated real subprocess, not an in-process
	// mock. Even a malformed private RPC cannot acquire a shell/workspace sink
	// or contact an internal/out-of-contract recipient.
	runtimeClient := runtimev1.NewToolRuntimeServiceClient(unauthorized)
	runtimeContext := metadata.AppendToOutgoingContext(ctx, "authorization", "Bearer "+runtimeToken)
	for _, args := range []string{
		`{"url":"http://127.0.0.1/private"}`,
		`{"url":"` + egresstest.Origin + `/project-other"}`,
		`{"url":"` + egresstest.Origin + `/project/%2e%2e/admin"}`,
		`{"url":"` + egresstest.Origin + `/project","command":"sh"}`,
		`{"url":"` + egresstest.Origin + `/project","workspace":"/srv/storage"}`,
		`{"url":"` + egresstest.Origin + `/project","authorization":"raw-secret"}`,
		`{"url":"` + egresstest.Origin + `/project","url":"http://127.0.0.1/private"}`,
	} {
		before := contacted.Load()
		forged := &toolbrokerv1.ToolCall{RunId: run.GetRunId(), ToolCallId: "malicious-runtime-probe", ToolName: inspectContract.ToolName, ArgsJson: args}
		response, err := runtimeClient.Execute(runtimeContext, &runtimev1.ExecuteRequest{ToolCall: forged, Contract: proto.Clone(inspectContract).(*toolbrokerv1.ToolContract), Context: &runtimev1.ExecutionContext{RunId: forged.RunId, ToolCallId: forged.ToolCallId, ToolName: forged.ToolName, ActionId: "fixture-action", ActionFingerprint: "fixture-fingerprint", TimeoutMs: 5000}})
		if err != nil || response.GetResult().GetStatus() != "failed" || contacted.Load() != before {
			t.Fatal("real executor accepted a dangerous proposal or contacted recipient")
		}
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
	updated := p.Manifest
	updated.Version = "2.0.0"
	updated.Description = "Reviewed version two of the same bounded fixture"
	updated.Credentials = []extensions.Credential{{Input: "authorization", Field: "token"}}
	for i := range updated.Operations {
		var schema map[string]any
		if err = json.Unmarshal(updated.Operations[i].InputSchema, &schema); err != nil {
			t.Fatal(err)
		}
		properties := schema["properties"].(map[string]any)
		properties["authorization"] = map[string]any{"type": "object", "required": []string{"type", "alias", "field"}, "properties": map[string]any{"type": map[string]any{"type": "string", "enum": []string{"credential_ref"}}, "alias": map[string]any{"type": "string"}, "field": map[string]any{"type": "string", "enum": []string{"token"}}}, "additionalProperties": false}
		updated.Operations[i].InputSchema, _ = json.Marshal(schema)
	}
	updatedRaw, _ := json.Marshal(updated)
	control(extensions.ControlRequest{Operation: "stage", ManifestJSON: updatedRaw})
	versionTwo, err := extensions.Parse(strings.NewReader(string(updatedRaw)))
	if err != nil {
		t.Fatal(err)
	}
	control(extensions.ControlRequest{Operation: "review", ExtensionID: "project", ManifestHash: versionTwo.Digest})
	credential := actionauth.CredentialRef{Alias: "extension-account", Field: "token"}
	state = control(extensions.ControlRequest{Operation: "configure", ExtensionID: "project", ManifestHash: versionTwo.Digest, Generation: state.Generation, Configuration: map[string]string{"origin": egresstest.Origin}, CredentialRefs: map[string]actionauth.CredentialRef{"authorization": credential}, Tools: selected}).Item.Record
	if state.Available || state.Status != "disabled" || state.Health == "healthy" {
		t.Fatal("updated manifest inherited readiness/enablement")
	}
	if _, err = client.AuthorizeAction(ctx, pending, saved.Binding.ActionID, approvalID, ""); err == nil {
		t.Fatal("updated capability reused old approved manifest")
	}
	// A stale suspended action ends its attempt. Updating an extension never
	// reopens that continuation or converts its old approval into a new intent.
	if err = approvalService.MarkActionCheckpoint(owned, run.GetRunId(), pending.ToolCallId, "blocked"); err != nil {
		t.Fatal(err)
	}
	if _, err = runs.TransitionRun(owned, &sessionv1.UpdateRunStateRequest{RunId: run.GetRunId(), FromState: commonv1.RunState_RUN_STATE_AWAITING_APPROVAL, ToState: commonv1.RunState_RUN_STATE_FAILED, LeaseId: lease.LeaseID, ErrorType: commonv1.ErrorClass_ERROR_CLASS_POLICY_DENIED, ErrorMessage: "saved action is no longer authorized"}); err != nil {
		t.Fatal(err)
	}
	if released, e := leases.ReleaseLease(ctx, lease.LeaseID); e != nil || !released {
		t.Fatal("stale attempt lease was not released")
	}
	run, err = runs.CreateRun(ctx, &sessionv1.CreateRunRequest{SessionKey: sessionKey, InputEvent: &runv1.InputEvent{EventId: "extension-updated-fixture-input"}, AutonomyMode: commonv1.AutonomyMode_AUTONOMY_MODE_2, ModelProvider: "fixture", MetadataJson: "{}"})
	if err != nil {
		t.Fatal(err)
	}
	lease, err = leases.AcquireLease(ctx, session.AcquireLeaseParams{SessionKey: sessionKey, RunID: run.GetRunId(), OwnerID: "worker", TTL: 3 * time.Minute})
	if err != nil {
		t.Fatal(err)
	}
	owned = ownership.WithToken(ctx, leases.Authority(), ownership.Token{SessionKey: sessionKey, RunID: run.GetRunId(), LeaseID: lease.LeaseID, FenceEpoch: lease.FenceEpoch})
	if err = tasks.AttachRun(owned, record.TaskID, record.Revision, run.GetRunId(), sessionKey); err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, `UPDATE runs SET status='tool_pending',current_state='tool_pending',lease_id=$2 WHERE run_id=$1`, run.GetRunId(), lease.LeaseID); err != nil {
		t.Fatal(err)
	}
	if err = sourceCatalog.PinTranscript(owned, sessionKey, 0); err != nil {
		t.Fatal("fresh extension attempt transcript source pin", err)
	}
	control(extensions.ControlRequest{Operation: "probe", ExtensionID: "project", Generation: state.Generation})
	state = control(extensions.ControlRequest{Operation: "enable", ExtensionID: "project", Generation: state.Generation}).Item.Record
	stale := call("extension.project.inspect", "/project", nil)
	if _, err = client.PrepareAction(ctx, stale); err == nil {
		t.Fatal("updated generation inherited prior task permission")
	}
	control(extensions.ControlRequest{Operation: "grant_task", ExtensionID: "project", Access: extensions.TaskAccess{TaskID: record.TaskID, TaskRevision: record.Revision, ConstraintsRevision: record.ConstraintsRevision, ManifestHash: versionTwo.Digest, Generation: state.Generation, Revision: 1, Tools: selected, Resources: []actionauth.ResourceScope{{Origin: egresstest.Origin, PathPrefix: "/project"}}, CredentialRefs: []actionauth.CredentialRef{credential}, ExpiresAt: time.Now().Add(time.Hour)}})
	dispatch(call("extension.project.inspect", "/project", nil))
	privateCall := call("extension.project.inspect", "/project/private", nil)
	privateCall.ArgsJson = `{"url":"` + egresstest.Origin + `/project/private","authorization":{"type":"credential_ref","alias":"extension-account","field":"token"}}`
	privateCall.CredentialRefs = []*toolbrokerv1.CredentialRef{{Type: "credential_ref", Alias: credential.Alias, Field: credential.Field}}
	beforePrivate := contacted.Load()
	// V2 account grants are scoped to browser preparation. Historical env token
	// aliases cannot be made usable by configuring or approving an extension.
	if _, err = client.PrepareAction(ctx, privateCall); err == nil || contacted.Load() != beforePrivate {
		t.Fatal("removed legacy extension credential reached recipient")
	}
	state = control(extensions.ControlRequest{Operation: "rollback", ExtensionID: "project", ManifestHash: p.Digest, Generation: state.Generation, Configuration: map[string]string{"origin": egresstest.Origin}, Tools: selected}).Item.Record
	if state.ManifestHash != p.Digest || state.Available || state.Health == "healthy" {
		t.Fatal("rollback silently restored prior authority")
	}
	control(extensions.ControlRequest{Operation: "probe", ExtensionID: "project", Generation: state.Generation})
	state = control(extensions.ControlRequest{Operation: "enable", ExtensionID: "project", Generation: state.Generation}).Item.Record
	control(extensions.ControlRequest{Operation: "grant_task", ExtensionID: "project", Access: extensions.TaskAccess{TaskID: record.TaskID, TaskRevision: record.Revision, ConstraintsRevision: record.ConstraintsRevision, ManifestHash: p.Digest, Generation: state.Generation, Revision: 2, Tools: selected, Resources: []actionauth.ResourceScope{{Origin: egresstest.Origin, PathPrefix: "/project"}}, ExpiresAt: time.Now().Add(time.Hour)}})
	checked = dispatch(call("extension.project.check", "/project/patches/"+artifact+"/checks", nil))
	if checked["checks_passed"] != true {
		t.Fatal("rollback lost recipient artifact history")
	}
	pending = call("extension.project.inspect", "/project", nil)
	saved, approvalID = prepare(pending)
	state = control(extensions.ControlRequest{Operation: "disable", ExtensionID: "project", Generation: state.Generation}).Item.Record
	if _, err = client.AuthorizeAction(ctx, pending, saved.Binding.ActionID, approvalID, ""); err == nil {
		t.Fatal("disabled installed capability reused saved owner approval")
	}
	versions := control(extensions.ControlRequest{Operation: "versions", ExtensionID: "project"}).Versions
	if len(versions) != 2 || versions[0].Digest != p.Digest {
		t.Fatal("disable lost immutable version history")
	}
}
