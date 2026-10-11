package app

import (
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/butler/butler/apps/orchestrator/internal/tools"
	"github.com/butler/butler/internal/actionauth"
	"github.com/butler/butler/internal/delegation"
	toolbrokerv1 "github.com/butler/butler/internal/gen/toolbroker/v1"
	"google.golang.org/protobuf/proto"
)

// A separate actual Broker gRPC process, current HTTP authority and private
// admission handler exercise the intended production composition without enabling
// it on the shared deployment. No model, tool runtime or child success is claimed.
func TestDelegationAdmissionActualBrokerProcessIntegration(t *testing.T) {
	binary := os.Getenv("BUTLER_DELEGATION_BROKER_BINARY")
	if binary == "" {
		t.Skip("compiled private Broker harness required")
	}
	f := newDispatchAdmissionFixture(t)
	const token = "private-delegation-process-fixture-token-value"
	h, err := delegation.NewAdmissionHandler(token, f.service)
	if err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	mux.Handle("/context", actionauth.NewContextHandler(token, f.reader))
	mux.Handle("/delegate", h)
	control := httptest.NewServer(mux)
	defer control.Close()
	tokenPath := filepath.Join(t.TempDir(), "token")
	if err = os.WriteFile(tokenPath, []byte(token), 0600); err != nil {
		t.Fatal(err)
	}
	address := freeAddress(t)
	dsn, err := url.Parse(f.f.pool.Config().ConnConfig.ConnString())
	if err != nil {
		t.Fatal("fixture PostgreSQL URL required")
	}
	query := dsn.Query()
	query.Set("search_path", f.f.pool.Config().ConnConfig.RuntimeParams["search_path"])
	dsn.RawQuery = query.Encode()
	cmd := exec.Command(binary, "-test.run=^TestDelegationBrokerHarness$", "-test.timeout=50s")
	cmd.Env = append(os.Environ(), "BUTLER_DELEGATION_BROKER_HARNESS=1", "BUTLER_DELEGATION_POSTGRES_URL="+dsn.String(), "BUTLER_DELEGATION_TOKEN_FILE="+tokenPath, "BUTLER_DELEGATION_CONTEXT_URL="+control.URL+"/context", "BUTLER_DELEGATION_ADMISSION_URL="+control.URL+"/delegate", "BUTLER_DELEGATION_GRPC_ADDRESS="+address, "BUTLER_DELEGATION_MANIFEST_FILE="+f.manifestPath)
	log, err := os.OpenFile(filepath.Join(t.TempDir(), "broker.log"), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		t.Fatal(err)
	}
	defer log.Close()
	cmd.Stdout, cmd.Stderr = log, log
	if err = cmd.Start(); err != nil {
		t.Fatal(err)
	}
	done := make(chan struct{})
	var processErr error
	go func() { processErr = cmd.Wait(); close(done) }()
	t.Cleanup(func() {
		_ = cmd.Process.Signal(os.Interrupt)
		select {
		case <-done:
			if processErr != nil {
				t.Error("private Broker process failed", processErr)
			}
		case <-time.After(3 * time.Second):
			_ = cmd.Process.Kill()
			<-done
			t.Error("private Broker did not stop")
		}
		if t.Failed() {
			data, _ := os.ReadFile(log.Name())
			if len(data) > 8192 {
				data = data[len(data)-8192:]
			}
			t.Log(strings.ReplaceAll(string(data), token, "[private fixture token]"))
		}
	})
	ready := false
	for deadline := time.Now().Add(5 * time.Second); time.Now().Before(deadline); {
		conn, err := net.DialTimeout("tcp", address, 50*time.Millisecond)
		if err == nil {
			_ = conn.Close()
			ready = true
			break
		}
		select {
		case <-done:
			t.Fatal("Broker exited before ready", processErr)
		case <-time.After(25 * time.Millisecond):
		}
	}
	if !ready {
		t.Fatal("private Broker not ready")
	}
	client, err := tools.DialWithTokenFile(address, tokenPath)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	anonymous, err := tools.Dial(address)
	if err != nil {
		t.Fatal(err)
	}
	defer anonymous.Close()
	if _, err = anonymous.PrepareAction(f.f.ctx, f.call); err == nil {
		t.Fatal("untrusted runtime caller prepared delegation")
	}
	p, err := client.PrepareAction(f.f.ctx, f.call)
	if err != nil || !p.RequiresApproval || p.Binding.ActionHash != f.prepared.Binding.ActionHash {
		t.Fatal("actual Broker prepared different action", p, err)
	}
	if _, err = client.AuthorizeAction(f.f.ctx, f.call, p.Binding.ActionID, "", ""); err == nil {
		t.Fatal("Broker allowed unapproved local orchestration")
	}
	grant, err := client.AuthorizeAction(f.f.ctx, f.call, p.Binding.ActionID, f.grant.ApprovalID, "")
	if err != nil {
		t.Fatal(err)
	}
	changed := proto.Clone(f.call).(*toolbrokerv1.ToolCall)
	changed.ArgsJson = strings.Replace(changed.ArgsJson, `"tokens":10000`, `"tokens":10001`, 1)
	if result, err := client.ExecuteAuthorizedToolCall(f.f.ctx, changed, grant); err != nil || result.GetStatus() == "completed" {
		t.Fatal("changed approved allocation reached actual dispatch", result, err)
	}
	f.noAdmission(t)
	result, err := client.ExecuteAuthorizedToolCall(f.f.ctx, f.call, grant)
	if err != nil || result.GetStatus() != "completed" || result.GetEffectState() != string(actionauth.StateConfirmed) || result.GetActionId() != p.EffectID || len(result.GetEvidenceRefs()) != 1 {
		t.Fatal("actual admission/journal failed", result, err)
	}
	var receipt delegation.AdmissionReceipt
	if err = json.Unmarshal([]byte(result.GetResultJson()), &receipt); err != nil || receipt.TaskID == "" || receipt.State != "queued" {
		t.Fatal("Broker receipt invalid", receipt, err)
	}
	var count int
	if err = f.f.pool.QueryRow(f.f.ctx, `SELECT count(*) FROM delegation_admission_snapshots WHERE task_id=$1 AND effect_id=$2`, receipt.TaskID, result.GetActionId()).Scan(&count); err != nil || count != 1 {
		t.Fatal("ack preceded durable admission", count, err)
	}
	replayed, err := client.ExecuteAuthorizedToolCall(f.f.ctx, f.call, grant)
	if err != nil || replayed.GetStatus() == "completed" || replayed.GetActionId() != result.GetActionId() {
		t.Fatal("spent dispatch replayed", replayed, err)
	}
	if err = f.f.pool.QueryRow(f.f.ctx, `SELECT count(*) FROM task_delegations`).Scan(&count); err != nil || count != 1 {
		t.Fatal("replay duplicated child", count, err)
	}
	child, err := f.f.store.Get(f.f.ctx, receipt.TaskID)
	if err != nil || child.Status != "active" || !strings.Contains(child.Goal, "Delegated") {
		t.Fatal("receipt promoted child success", child, err)
	}
	t.Log("actual private Broker→HTTP admission→PG receipt confirmed; child work remains active/unverified")
}
