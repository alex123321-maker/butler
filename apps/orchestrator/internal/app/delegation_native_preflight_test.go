package app

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/butler/butler/apps/orchestrator/internal/goal"
	"github.com/butler/butler/apps/orchestrator/internal/session"
	"github.com/butler/butler/apps/orchestrator/internal/tools"
	"github.com/butler/butler/internal/actionauth"
	"github.com/butler/butler/internal/delegation"
	commonv1 "github.com/butler/butler/internal/gen/common/v1"
	toolbrokerv1 "github.com/butler/butler/internal/gen/toolbroker/v1"
	"github.com/butler/butler/internal/ownership"
)

// Exercise the exact new finite mandate, native experiment policy and scoped
// Broker composition before spending anything on a model. There is no Provider
// construction, scripted output or invented admission receipt here.
func TestNativeDelegationBrokerPreflightWithoutInferenceIntegration(t *testing.T) {
	binary := os.Getenv("BUTLER_DELEGATION_BROKER_BINARY")
	if binary == "" {
		t.Skip("private compiled Broker required")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	f := newNativeScenarioFixture(t, ctx)
	if _, _, err := f.sessions.CreateSession(ctx, session.CreateSessionParams{SessionKey: "preflight-root", UserID: nativeFixtureOwner, Channel: "web", MetadataJSON: `{}`}); err != nil {
		t.Fatal("preflight session denied")
	}
	parent, err := f.goals.CreateOwnerTask(nativeOwner(ctx, "preflight-root", "preflight-root"), goal.TaskRequest{SessionKey: "preflight-root", GoalID: f.goalID, MandateID: f.mandateID, Purpose: "native_research", Goal: "Preflight exact context-only research", CompletionCriteria: []string{"Independent source evidence"}, InitiativeReason: "explicit lab owner request", ObservationRef: "fixture:preflight"})
	if err != nil {
		t.Fatal("preflight assigned task denied")
	}
	admission, reader := f.composition(t, 2)
	if _, err = f.pool.Exec(ctx, `INSERT INTO runs(run_id,session_key,input_event_id,status,current_state,autonomy_mode,model_provider,started_at,updated_at,task_id,task_revision) VALUES('preflight-run',$1,'preflight','tool_running','tool_running','mode_2','antigravity',clock_timestamp(),clock_timestamp(),$2,$3)`, parent.SessionKey, parent.TaskID, parent.Revision); err != nil {
		t.Fatal("preflight run fixture unavailable")
	}
	leases := session.NewPostgresLeaseManager(f.pool)
	lease, err := leases.AcquireLease(ctx, session.AcquireLeaseParams{SessionKey: parent.SessionKey, RunID: "preflight-run", OwnerID: "preflight-worker", TTL: time.Minute})
	if err != nil {
		t.Fatal("actual preflight lease denied")
	}
	if _, err = f.pool.Exec(ctx, `UPDATE runs SET lease_id=$1 WHERE run_id='preflight-run'`, lease.LeaseID); err != nil {
		t.Fatal("preflight run lease pin unavailable")
	}
	owned := ownership.WithToken(ctx, leases.Authority(), ownership.Token{SessionKey: parent.SessionKey, RunID: "preflight-run", LeaseID: lease.LeaseID, FenceEpoch: lease.FenceEpoch})
	if f.sources.PinTranscript(owned, parent.SessionKey, 0) != nil {
		t.Fatal("actual transcript pin denied")
	}
	const token = "private-native-preflight-authority-token-value"
	h, err := delegation.NewAdmissionHandler(token, admission)
	if err != nil {
		t.Fatal("actual private admission unavailable")
	}
	mux := http.NewServeMux()
	mux.Handle("/context", actionauth.NewContextHandler(token, reader))
	mux.Handle("/delegate", h)
	control := httptest.NewServer(mux)
	defer control.Close()
	tokenFile := filepath.Join(f.private, "preflight-token")
	if os.WriteFile(tokenFile, []byte(token), 0600) != nil {
		t.Fatal("private preflight token file unavailable")
	}
	u, err := url.Parse(f.pool.Config().ConnConfig.ConnString())
	if err != nil {
		t.Fatal("private schema URL unavailable")
	}
	q := u.Query()
	q.Set("search_path", f.pool.Config().ConnConfig.RuntimeParams["search_path"])
	u.RawQuery = q.Encode()
	address := freeAddress(t)
	p := startNativeSubprocess(t, binary, "TestNativeDelegationBrokerHarness", u.String(), f.private, "preflight-broker", []string{"BUTLER_DELEGATION_NATIVE_BROKER_HARNESS=1", "BUTLER_DELEGATION_TOKEN_FILE=" + tokenFile, "BUTLER_DELEGATION_CONTEXT_URL=" + control.URL + "/context", "BUTLER_DELEGATION_ADMISSION_URL=" + control.URL + "/delegate", "BUTLER_DELEGATION_GRPC_ADDRESS=" + address, "BUTLER_DELEGATION_MANIFEST_FILE=" + f.manifest, "BUTLER_EXECUTION_BUDGET_POLICY_JSON=" + string(nativeJSON(nativeExperimentPolicy()))})
	defer p.stop(t, false)
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
		t.Fatal("private preflight gRPC unavailable")
	}
	defer client.Close()
	call := &toolbrokerv1.ToolCall{RunId: "preflight-run", ToolCallId: "preflight-call", ToolName: delegation.AdmissionTool, ArgsJson: string(nativeJSON(f.args[0])), AutonomyMode: commonv1.AutonomyMode_AUTONOMY_MODE_2}
	prepared, err := client.PrepareAction(ctx, call)
	if err != nil || prepared.RequiresApproval {
		t.Fatal("reviewed bounded owner mandate did not allow exact prepare", err, prepared.RequiresApproval)
	}
	grant, err := client.AuthorizeAction(ctx, call, prepared.Binding.ActionID, "", "")
	if err != nil {
		t.Fatal("actual finite mandate authorization denied")
	}
	result, err := client.ExecuteAuthorizedToolCall(ctx, call, grant)
	if err != nil || result.GetStatus() != "completed" || result.GetEffectState() != string(actionauth.StateConfirmed) {
		t.Fatal("actual Broker preflight admission failed")
	}
	var receipt delegation.AdmissionReceipt
	if json.Unmarshal([]byte(result.GetResultJson()), &receipt) != nil || receipt.TaskID == "" || receipt.InputID == "" || receipt.State != "queued" {
		t.Fatal("actual durable preflight receipt invalid")
	}
	var attempts int
	if f.pool.QueryRow(ctx, `SELECT attempts FROM fixture_native_limit`).Scan(&attempts) != nil || attempts != 0 {
		t.Fatal("preflight performed inference")
	}
	var n int
	if f.pool.QueryRow(ctx, `SELECT count(*) FROM delegation_admission_snapshots WHERE task_id=$1`, receipt.TaskID).Scan(&n) != nil || n != 1 {
		t.Fatal("preflight receipt lacks accepted snapshot")
	}
}
