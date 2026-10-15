package app

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	approvals "github.com/butler/butler/apps/orchestrator/internal/approval"
	"github.com/butler/butler/apps/orchestrator/internal/tools"
	"github.com/butler/butler/internal/actionauth"
	"github.com/butler/butler/internal/extensions"
	"github.com/butler/butler/internal/extensions/projectfixture"
	commonv1 "github.com/butler/butler/internal/gen/common/v1"
	toolbrokerv1 "github.com/butler/butler/internal/gen/toolbroker/v1"
	"github.com/butler/butler/internal/security/egress/egresstest"
)

// This is a protocol/authority component proof with actual production Broker,
// extension interpreter and HTTP runtime. It does not create accepted lessons,
// fake original task results or claim a native model solved a corpus task.
func experienceRecipientBroker(t *testing.T, recipient *experienceRecipient) (*dispatchAdmissionFixture, *tools.BrokerClient) {
	t.Helper()
	for _, key := range []string{"BUTLER_BROKER_E2E_BINARY", "BUTLER_EXTENSION_E2E_BINARY", "BUTLER_EXPERIENCE_HTTP_BINARY"} {
		if os.Getenv(key) == "" {
			t.Fatal("compiled isolated component required: " + key)
		}
	}
	f := newDispatchAdmissionFixture(t)
	site := httptest.NewServer(recipient)
	t.Cleanup(site.Close)
	const coreToken = "experience-component-private-core-token-0600"
	const runtimeToken = "experience-component-private-runtime-token-0600"
	authority := httptest.NewServer(actionauth.NewContextHandler(coreToken, f.reader))
	t.Cleanup(authority.Close)
	file := func(name string, raw []byte) string {
		path := filepath.Join(t.TempDir(), name)
		if os.WriteFile(path, raw, 0600) != nil {
			t.Fatal("private component artifact unavailable")
		}
		return path
	}
	coreFile, runtimeFile := file("core.token", []byte(coreToken)), file("runtime.token", []byte(runtimeToken))
	extAddress, httpAddress, brokerAddress, brokerHTTP := freeAddress(t), freeAddress(t), freeAddress(t), freeAddress(t)
	extBinary := os.Getenv("BUTLER_EXTENSION_E2E_BINARY")
	binary, err := os.ReadFile(extBinary)
	if err != nil {
		t.Fatal(err)
	}
	pack, err := projectfixture.Manifest(experienceDigest(string(binary)), "experience-recipient-component-v1")
	if err != nil {
		t.Fatal(err)
	}
	profiles, _ := json.Marshal(map[string]extensions.Profile{"reference-http": {Target: extAddress, Dependency: pack.Manifest.Dependencies[0]}})
	extensionChild(t, extBinary, extAddress, "127.0.0.1:0", []string{"-test.run=^TestExtensionRuntimeEgressHarness$"}, "BUTLER_EXTENSION_EGRESS_HARNESS=1", "BUTLER_EXTENSION_FIXTURE_ENDPOINT="+site.URL, "BUTLER_EXTENSION_PROFILES_JSON="+string(profiles), "BUTLER_EXTENSION_RUNTIME_TOKEN_FILE="+runtimeFile)
	extensionChild(t, os.Getenv("BUTLER_EXPERIENCE_HTTP_BINARY"), httpAddress, "127.0.0.1:0", []string{"-test.run=^TestExperienceHTTPRuntimeHarness$"}, "BUTLER_EXPERIENCE_HTTP_HARNESS=1", "BUTLER_EXPERIENCE_RECIPIENT_ENDPOINT="+site.URL)
	registry, err := os.ReadFile("configs/tools.json")
	if os.IsNotExist(err) {
		registry, err = os.ReadFile(filepath.Join("..", "..", "..", "..", "configs", "tools.json"))
	}
	if err != nil {
		t.Fatal(err)
	}
	var configured struct {
		Tools []map[string]any `json:"tools"`
	}
	if json.Unmarshal(registry, &configured) != nil {
		t.Fatal("current HTTP contract unavailable")
	}
	selected := []map[string]any{}
	for _, contract := range configured.Tools {
		if contract["tool_name"] == "http.request" {
			contract["runtime_target"] = httpAddress
			contract["allowed_domains"] = []string{"external.test"}
			selected = append(selected, contract)
		}
	}
	if len(selected) != 1 {
		t.Fatal("one actual HTTP contract required")
	}
	raw, _ := json.Marshal(map[string]any{"tools": selected})
	registryFile := file("registry.json", raw)
	dsn, err := url.Parse(f.f.pool.Config().ConnConfig.ConnString())
	if err != nil {
		t.Fatal(err)
	}
	query := dsn.Query()
	query.Set("search_path", f.f.pool.Config().ConnConfig.RuntimeParams["search_path"])
	dsn.RawQuery = query.Encode()
	extensionChild(t, os.Getenv("BUTLER_BROKER_E2E_BINARY"), brokerAddress, brokerHTTP, nil, "BUTLER_POSTGRES_URL="+dsn.String(), "BUTLER_POSTGRES_MAX_CONNS=2", "BUTLER_POSTGRES_MIN_CONNS=0", "BUTLER_TOOL_REGISTRY_PATH="+registryFile, "BUTLER_ACTION_CONTEXT_URL="+authority.URL, "BUTLER_ACTION_AUTHORITY_TOKEN_FILE="+coreFile, "BUTLER_EXTENSION_PROFILES_JSON="+string(profiles), "BUTLER_EXTENSION_RUNTIME_TOKEN_FILE="+runtimeFile, "BUTLER_OWNER_CONTROL_OWNER_ID="+f.f.parent.OwnerID, "BUTLER_MEMORY_DELETION_MANIFEST_FILE="+f.manifestPath)
	controlClient := extensions.NewControlClient("http://"+brokerHTTP+"/internal/extensions", coreToken)
	control := func(q extensions.ControlRequest) extensions.ControlResponse {
		t.Helper()
		out, err := controlClient.Control(f.f.owner, q)
		if err != nil || out.Error != "" {
			t.Fatal("reviewed extension control", q.Operation, err, out.Error)
		}
		return out
	}
	control(extensions.ControlRequest{Operation: "stage", ManifestJSON: pack.Raw})
	control(extensions.ControlRequest{Operation: "review", ExtensionID: "project", ManifestHash: pack.Digest})
	state := control(extensions.ControlRequest{Operation: "list"}).Items[0].Record
	names := []string{"extension.project.inspect", "extension.project.prepare-patch", "extension.project.check"}
	state = control(extensions.ControlRequest{Operation: "configure", ExtensionID: "project", ManifestHash: pack.Digest, Generation: state.Generation, Configuration: map[string]string{"origin": egresstest.Origin}, Tools: names}).Item.Record
	control(extensions.ControlRequest{Operation: "probe", ExtensionID: "project", Generation: state.Generation})
	state = control(extensions.ControlRequest{Operation: "enable", ExtensionID: "project", Generation: state.Generation}).Item.Record
	control(extensions.ControlRequest{Operation: "grant_task", ExtensionID: "project", Access: extensions.TaskAccess{TaskID: f.f.parent.TaskID, TaskRevision: f.f.parent.Revision, ConstraintsRevision: f.f.parent.ConstraintsRevision, ManifestHash: pack.Digest, Generation: state.Generation, Tools: names, Resources: []actionauth.ResourceScope{{Origin: egresstest.Origin, PathPrefix: "/project/alpha"}}, ExpiresAt: time.Now().Add(10 * time.Minute)}})
	client, err := tools.DialWithTokenFile(brokerAddress, coreFile)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = client.Close() })
	return f, client
}

func experienceComponentDispatch(t *testing.T, f *dispatchAdmissionFixture, client *tools.BrokerClient, name, path string, body any) (*toolbrokerv1.ToolCall, actionauth.PreparedAction, string, *toolbrokerv1.ToolResult) {
	t.Helper()
	args := map[string]any{"url": egresstest.Origin + path}
	if name == "http.request" {
		args["method"] = "GET"
	}
	if body != nil {
		args["body"] = body
	}
	raw, _ := json.Marshal(args)
	call := &toolbrokerv1.ToolCall{RunId: f.call.RunId, ToolCallId: fmt.Sprintf("component-%d", time.Now().UnixNano()), ToolName: name, ArgsJson: string(raw), AutonomyMode: commonv1.AutonomyMode_AUTONOMY_MODE_1}
	if _, err := f.f.pool.Exec(f.f.ctx, `UPDATE runs SET status='tool_pending',current_state='tool_pending' WHERE run_id=$1`, call.RunId); err != nil {
		t.Fatal(err)
	}
	prepared, err := client.PrepareAction(f.f.ctx, call)
	if err != nil {
		t.Fatal("prepare actual component action", name, err)
	}
	approvalID := ""
	if prepared.RequiresApproval {
		expires := prepared.ExpiresAt
		pending, err := f.approvals.CreatePendingApproval(f.owned, approvals.CreatePendingParams{RunID: call.RunId, SessionKey: f.f.parent.SessionKey, ToolCallID: call.ToolCallId, ToolName: call.ToolName, ArgsJSON: call.ArgsJson, RequestedVia: "web", ActionBinding: prepared.Binding, ContinuationJSON: `{"version":1}`, Summary: prepared.Summary, ExpiresAt: &expires})
		if err != nil {
			t.Fatal(err)
		}
		actor := approvals.WithActor(f.f.ctx, approvals.Actor{OwnerID: f.f.parent.OwnerID, Channel: "web", SessionKey: f.f.parent.SessionKey, DeliveryID: "approve-" + call.ToolCallId})
		if _, _, err = f.approvals.ResolveByApprovalID(actor, approvals.ResolveByApprovalIDParams{ApprovalID: pending.ApprovalID, Approved: true, ResolvedVia: "web", ActorType: "web", ActorID: f.f.parent.OwnerID, ResolvedBy: f.f.parent.OwnerID}); err != nil {
			t.Fatal(err)
		}
		approvalID = pending.ApprovalID
	}
	grant, err := client.AuthorizeAction(f.f.ctx, call, prepared.Binding.ActionID, approvalID, "")
	if err != nil {
		t.Fatal("authorize actual component action", name, err)
	}
	result, err := client.ExecuteAuthorizedToolCall(f.f.ctx, call, grant)
	if err != nil || result == nil {
		t.Fatal("actual component RPC", name, err)
	}
	return call, prepared, grant, result
}

func TestExperienceRecipientActualBrokerEffectProtocolIntegration(t *testing.T) {
	if os.Getenv("BUTLER_EXPERIENCE_RECIPIENT_BROKER") != "1" {
		t.Skip("explicit actual Broker/extension/HTTP component driver required")
	}
	for _, fault := range []string{"confirmed_artifact", "rejected_value", "persisted_unknown"} {
		t.Run(fault, func(t *testing.T) {
			config := experienceRecipientConfig{Version: "v1", Ready: "true", Files: map[string]string{"alpha": "Synthetic\nready=false\n<!-- provenance: fixed-current-a -->\n", "beta": "Synthetic\nready=false\n<!-- provenance: fixed-current-b -->\n"}, Outage: fault == "persisted_unknown"}
			recipient, err := newExperienceRecipient(t.TempDir(), config)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = recipient.close() })
			f, client := experienceRecipientBroker(t, recipient)
			_, _, _, inspected := experienceComponentDispatch(t, f, client, "extension.project.inspect", "/project/alpha", nil)
			if inspected.GetEffectState() != string(actionauth.StateConfirmed) || !strings.Contains(inspected.GetResultJson(), "accepted_ready_value") {
				t.Fatal("current ABI DATA not read through actual Broker")
			}
			value := "true"
			if fault == "rejected_value" {
				value = "maybe"
			}
			call, prepared, grant, result := experienceComponentDispatch(t, f, client, "extension.project.prepare-patch", "/project/alpha/patches", experienceProposal(recipient, "alpha", value))
			effect, err := f.journal.GetEffect(f.f.ctx, prepared.EffectID)
			audit := recipient.snapshot()
			if err != nil || len(effect.EvidenceRefs) != 1 || len(audit.Entries) != 1 || audit.Dispatches != 1 {
				t.Fatal("actual dispatch/journal identity missing", err)
			}
			if fault == "confirmed_artifact" {
				if effect.State != actionauth.StateConfirmed || result.GetEffectState() != string(actionauth.StateConfirmed) {
					t.Fatal("recipient receipt not verified")
				}
				entry := audit.Entries[effect.IdempotencyKey]
				_, _, _, checked := experienceComponentDispatch(t, f, client, "extension.project.check", "/project/alpha/patches/"+entry.Artifact+"/checks", nil)
				var envelope struct {
					Body string `json:"body"`
				}
				if json.Unmarshal([]byte(checked.GetResultJson()), &envelope) != nil || checked.GetEffectState() != string(actionauth.StateConfirmed) || !strings.Contains(envelope.Body, `"applied":false`) || !strings.Contains(envelope.Body, "fixed-current-a") {
					t.Fatal("current exact artifact check absent", checked)
				}
			} else {
				if effect.State != actionauth.StateUnknown || result.GetEffectState() != string(actionauth.StateUnknown) {
					t.Fatal("rejection/lost receipt invented confirmed/absent state", effect.State, result.GetStatus())
				}
				if fault == "persisted_unknown" {
					duplicate, err := client.ExecuteAuthorizedToolCall(f.f.ctx, call, grant)
					if err != nil || duplicate.GetEffectState() != string(actionauth.StateUnknown) || duplicate.GetError().GetRetryable() || recipient.snapshot().Dispatches != 1 {
						t.Fatal("UNKNOWN replay reached recipient", err)
					}
				}
				_, _, observedGrantID, observed := experienceComponentDispatch(t, f, client, "http.request", "/project/effects/"+effect.IdempotencyKey, nil)
				observation, err := f.journal.GetGrant(f.f.ctx, observedGrantID)
				if err != nil {
					t.Fatal(err)
				}
				current, err := f.auth.Reconcile(f.f.ctx, effect.ActionID, observation, observed)
				if fault == "rejected_value" {
					if err != nil || current.State != actionauth.StateAbsent || recipient.snapshot().Entries[effect.IdempotencyKey].Artifact != "" {
						t.Fatal("independent exact lookup did not prove typed absence", current.State, err)
					}
				} else {
					if err == nil {
						t.Fatal("outage observation proved hidden committed state")
					}
					current, err = actionauth.NewPostgresStore(f.f.pool).GetEffect(context.Background(), effect.ActionID)
					if err != nil || current.State != actionauth.StateUnknown || recipient.snapshot().Entries[effect.IdempotencyKey].State != "effect_confirmed" {
						t.Fatal("reconstructed journal consumed hidden audit", current.State, err)
					}
				}
			}
			// Same v1 environment includes beta, but current task only grants alpha.
			raw, _ := json.Marshal(map[string]string{"url": egresstest.Origin + "/project/beta"})
			foreign := &toolbrokerv1.ToolCall{RunId: f.call.RunId, ToolCallId: "outside-current-resource", ToolName: "extension.project.inspect", ArgsJson: string(raw), AutonomyMode: commonv1.AutonomyMode_AUTONOMY_MODE_1}
			if _, err = client.PrepareAction(f.f.ctx, foreign); err == nil {
				t.Fatal("same environment widened current resource scope")
			}
		})
	}
}
