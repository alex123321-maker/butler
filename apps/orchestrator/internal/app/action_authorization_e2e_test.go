package app

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	approvals "github.com/butler/butler/apps/orchestrator/internal/approval"
	runservice "github.com/butler/butler/apps/orchestrator/internal/run"
	"github.com/butler/butler/apps/orchestrator/internal/session"
	"github.com/butler/butler/apps/orchestrator/internal/task"
	"github.com/butler/butler/apps/orchestrator/internal/tools"
	"github.com/butler/butler/internal/actionauth"
	"github.com/butler/butler/internal/credentials"
	commonv1 "github.com/butler/butler/internal/gen/common/v1"
	runv1 "github.com/butler/butler/internal/gen/run/v1"
	sessionv1 "github.com/butler/butler/internal/gen/session/v1"
	toolbrokerv1 "github.com/butler/butler/internal/gen/toolbroker/v1"
	"github.com/butler/butler/internal/memory/catalog"
	"github.com/butler/butler/internal/ownership"
	"github.com/butler/butler/internal/security/egress/egresstest"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"google.golang.org/protobuf/proto"
)

// This fixture isolates the Broker/approval/credential contract. Restartable
// ingress checkpoints are exercised by the durable lifecycle suite; these hooks
// deliberately have no execution authority and cannot approve or dispatch.
type boundaryCheckpointHooks struct{}

func (boundaryCheckpointHooks) SuspendApprovalTx(context.Context, pgx.Tx, string, string, string) error {
	return nil
}
func (boundaryCheckpointHooks) ReadyApprovalTx(context.Context, pgx.Tx, string, string) error {
	return nil
}

func e2eDatabase(t *testing.T) (*pgxpool.Pool, string) {
	return e2eDatabaseWithTracer(t, nil)
}

func e2eDatabaseWithTracer(t *testing.T, tracer pgx.QueryTracer) (*pgxpool.Pool, string) {
	return e2eDatabaseWithTracerAndPool(t, tracer, 2)
}

// Legacy boundary fixtures exercise a two-connection pool. The prospective
// native fixture can explicitly use the production orchestrator default10.
func releaseChainFixturePoolAllowed(max int32) bool { return max == 2 || max == 10 }

func e2eDatabaseWithTracerAndPool(t *testing.T, tracer pgx.QueryTracer, maximum int32) (*pgxpool.Pool, string) {
	t.Helper()
	if !releaseChainFixturePoolAllowed(maximum) {
		t.Fatal("fixed fixture PostgreSQL pool size required")
	}
	dsn := os.Getenv("BUTLER_TEST_ACTIONAUTH_POSTGRES_URL")
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil || !strings.HasSuffix(cfg.ConnConfig.Database, "_test") {
		t.Fatal("isolated *_test action database required")
	}
	ctx := context.Background()
	admin, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	schema := fmt.Sprintf("broker_proof_%d", time.Now().UnixNano())
	if _, err := admin.Exec(ctx, "CREATE SCHEMA "+schema); err != nil {
		t.Fatal(err)
	}
	cfg.ConnConfig.RuntimeParams["search_path"] = schema + ",public"
	cfg.MaxConns = maximum
	cfg.ConnConfig.Tracer = tracer
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { pool.Close(); _, _ = admin.Exec(ctx, "DROP SCHEMA "+schema+" CASCADE"); admin.Close() })
	files, err := filepath.Glob("migrations/*.up.sql")
	if err != nil || len(files) == 0 {
		t.Fatal("migration source missing")
	}
	sort.Strings(files)
	for _, path := range files {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := pool.Exec(ctx, string(data)); err != nil {
			t.Fatalf("%s: %v", filepath.Base(path), err)
		}
	}
	separator := "?"
	if strings.Contains(dsn, "?") {
		separator = "&"
	}
	return pool, dsn + separator + "search_path=" + schema + ",public"
}
func freeAddress(t *testing.T) string {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	address := listener.Addr().String()
	_ = listener.Close()
	return address
}
func childService(t *testing.T, binary, address string, environment ...string) {
	t.Helper()
	var arguments []string
	if binary == os.Getenv("BUTLER_BROWSER_E2E_BINARY") {
		arguments = []string{"-test.run=^TestBrowserRuntimeEgressHarness$"}
	}
	cmd := exec.Command(binary, arguments...)
	cmd.Env = append(os.Environ(), append(environment, "BUTLER_ENVIRONMENT=test", "BUTLER_GRPC_ADDR="+address, "BUTLER_HTTP_ADDR=127.0.0.1:0")...)
	cmd.Stdout, cmd.Stderr = io.Discard, io.Discard
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = cmd.Process.Signal(os.Interrupt); _ = cmd.Wait() })
	deadline := time.Now().Add(10 * time.Second)
	for {
		connection, err := net.DialTimeout("tcp", address, 100*time.Millisecond)
		if err == nil {
			_ = connection.Close()
			return
		}
		if time.Now().After(deadline) {
			t.Fatal("disposable service failed to start")
		}
		time.Sleep(25 * time.Millisecond)
	}
}

func TestApprovedCredentialBrokerBrowserE2E(t *testing.T) {
	if os.Getenv("BUTLER_ACTIONAUTH_E2E") != "1" {
		t.Skip("private authority + real Chromium integration runs only on isolated lab")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	pool, dsn := e2eDatabase(t)
	sessions := session.NewPostgresRepository(pool)
	sessionKey := "web:owner"
	_, _, err := sessions.CreateSession(ctx, session.CreateSessionParams{SessionKey: sessionKey, UserID: "owner", Channel: "web", MetadataJSON: "{}"})
	if err != nil {
		t.Fatal(err)
	}
	runs := runservice.NewService(runservice.NewPostgresRepository(pool), nil)
	run, err := runs.CreateRun(ctx, &sessionv1.CreateRunRequest{SessionKey: sessionKey, InputEvent: &runv1.InputEvent{EventId: "fixture-input"}, AutonomyMode: commonv1.AutonomyMode_AUTONOMY_MODE_2, ModelProvider: "openai", MetadataJson: "{}"})
	if err != nil {
		t.Fatal(err)
	}
	leases := session.NewPostgresLeaseManager(pool)
	lease, err := leases.AcquireLease(ctx, session.AcquireLeaseParams{SessionKey: sessionKey, RunID: run.GetRunId(), OwnerID: "worker", TTL: 2 * time.Minute})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `UPDATE runs SET status='tool_pending',current_state='tool_pending',lease_id=$2 WHERE run_id=$1`, run.GetRunId(), lease.LeaseID); err != nil {
		t.Fatal(err)
	}
	tasks := task.NewStore(pool)
	record, err := tasks.Create(ctx, task.CreateRequest{SessionKey: sessionKey, OwnerID: "owner", Goal: "test authenticated multi-step form", Constraints: []string{"only the disposable fixture origin"}, CompletionCriteria: []string{"one form submission"}})
	if err != nil {
		t.Fatal(err)
	}
	if err := tasks.AttachRun(ctx, record.TaskID, record.Revision, run.GetRunId(), sessionKey); err != nil {
		t.Fatal(err)
	}
	authoritativeCtx := ownership.WithToken(ctx, leases.Authority(), ownership.Token{SessionKey: sessionKey, RunID: run.GetRunId(), LeaseID: lease.LeaseID, FenceEpoch: lease.FenceEpoch})
	manifestPath := filepath.Join(t.TempDir(), "deletions.log")
	manifest := catalog.NewFileManifest(manifestPath)
	if err = manifest.Initialize(); err != nil {
		t.Fatal(err)
	}
	sourceCatalog := catalog.New(pool, manifest)
	if err = sourceCatalog.PinTranscript(authoritativeCtx, sessionKey, 0); err != nil {
		t.Fatal("owned transcript source pin", err)
	}
	approvalRepo := approvals.NewPostgresRepository(pool)
	approvalRepo.SetCheckpointHooks(boundaryCheckpointHooks{})
	approvalService := approvals.NewService(approvalRepo, nil)
	token := strings.Repeat("private-fixture-token-", 3)
	reader := &actionContextReader{runs: runs, sessions: sessions, leases: leases, tasks: tasks, approvals: approvalService, pool: pool}
	authority := httptest.NewServer(actionauth.NewContextHandler(token, reader))
	defer authority.Close()
	secret := "private-browser-password-7"
	_, err = credentials.NewStore(pool).Create(ctx, credentials.Record{Alias: "browser-account", SecretType: "password", TargetType: "browser", Status: credentials.StatusActive, AllowedDomains: []string{"external.test"}, AllowedTools: []string{"browser.prepare_fill"}, ApprovalPolicy: credentials.ApprovalPolicyAlwaysConfirm, SecretRef: "env://BUTLER_BROWSER_TEST_SECRET", MetadataJSON: "{}"})
	if err != nil {
		t.Fatal(err)
	}
	var submits atomic.Int32
	site := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/submit" {
			submits.Add(1)
			_ = r.ParseForm()
			if r.Form.Get("password") != secret || r.Form.Get("a") != "one" || r.Form.Get("b") != "two" {
				http.Error(w, "invalid form state", 400)
				return
			}
			http.SetCookie(w, &http.Cookie{Name: "auth", Value: secret})
			_, _ = fmt.Fprintf(w, "<body>authenticated %s %s %s</body>", r.Form.Get("a"), r.Form.Get("b"), r.Form.Get("password"))
			return
		}
		_, _ = fmt.Fprint(w, `<body><form action="/submit" method="post"><input id="a" name="a"><input id="b" name="b"><input type="password" id="password" name="password"><button id="submit">Send</button></form></body>`)
	}))
	defer site.Close()
	browserAddress := freeAddress(t)
	childService(t, os.Getenv("BUTLER_BROWSER_E2E_BINARY"), browserAddress, "BUTLER_BROWSER_EGRESS_HARNESS=1", "BUTLER_BROWSER_FIXTURE_ENDPOINT="+site.URL)
	var catalog struct {
		Tools []map[string]any `json:"tools"`
	}
	data, err := os.ReadFile("configs/tools.json")
	if err != nil {
		t.Fatal(err)
	}
	if json.Unmarshal(data, &catalog) != nil {
		t.Fatal("invalid catalog")
	}
	for _, tool := range catalog.Tools {
		if tool["tool_class"] == "browser" {
			tool["runtime_target"] = browserAddress
		}
	}
	data, _ = json.Marshal(catalog)
	catalogPath := filepath.Join(t.TempDir(), "tools.json")
	if os.WriteFile(catalogPath, data, 0600) != nil {
		t.Fatal("catalog fixture unavailable")
	}
	tokenPath := filepath.Join(t.TempDir(), "authority.token")
	if os.WriteFile(tokenPath, []byte(token), 0600) != nil {
		t.Fatal("private token fixture unavailable")
	}
	brokerAddress := freeAddress(t)
	childService(t, os.Getenv("BUTLER_BROKER_E2E_BINARY"), brokerAddress, "BUTLER_POSTGRES_URL="+dsn, "BUTLER_TOOL_REGISTRY_PATH="+catalogPath, "BUTLER_ACTION_CONTEXT_URL="+authority.URL, "BUTLER_ACTION_AUTHORITY_TOKEN_FILE="+tokenPath, "BUTLER_BROWSER_TEST_SECRET="+secret, "BUTLER_MEMORY_DELETION_MANIFEST_FILE="+manifestPath)
	client, err := tools.DialWithTokenFile(brokerAddress, tokenPath)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	untrusted, err := tools.Dial(brokerAddress)
	if err != nil {
		t.Fatal(err)
	}
	defer untrusted.Close()
	if _, err := untrusted.PrepareAction(ctx, &toolbrokerv1.ToolCall{RunId: run.GetRunId(), ToolCallId: "forged-worker", ToolName: "browser.open", ArgsJson: `{"url":"https://example.com"}`, AutonomyMode: commonv1.AutonomyMode_AUTONOMY_MODE_2}); err == nil {
		t.Fatal("knowing run_id allowed untrusted caller to prepare actions")
	}
	var observation map[string]any
	sequence := 0
	newCall := func(name string, args map[string]any, refs ...*toolbrokerv1.CredentialRef) *toolbrokerv1.ToolCall {
		sequence++
		encoded, _ := json.Marshal(args)
		return &toolbrokerv1.ToolCall{RunId: run.GetRunId(), ToolCallId: fmt.Sprintf("proof-call-%d", sequence), ToolName: name, ArgsJson: string(encoded), AutonomyMode: commonv1.AutonomyMode_AUTONOMY_MODE_2, CredentialRefs: refs}
	}
	approve := func(call *toolbrokerv1.ToolCall, p actionauth.PreparedAction) string {
		t.Helper()
		expires := p.ExpiresAt
		approval, err := approvalService.CreatePendingApproval(authoritativeCtx, approvals.CreatePendingParams{RunID: run.GetRunId(), SessionKey: sessionKey, ToolCallID: call.GetToolCallId(), ToolName: call.GetToolName(), ArgsJSON: call.GetArgsJson(), RequestedVia: "web", ActionBinding: p.Binding, ContinuationJSON: `{"version":1}`, Summary: p.Summary, ExpiresAt: &expires})
		if err != nil {
			t.Fatal(err)
		}
		actorCtx := approvals.WithActor(ctx, approvals.Actor{OwnerID: "owner", Channel: "web", SessionKey: sessionKey, DeliveryID: "decision-" + call.GetToolCallId()})
		resolved, _, err := approvalService.ResolveByApprovalID(actorCtx, approvals.ResolveByApprovalIDParams{ApprovalID: approval.ApprovalID, Approved: true, ResolvedVia: "web", ActorType: "web", ActorID: "owner", ResolvedBy: "owner"})
		if err != nil || resolved.Status != approvals.StatusApproved {
			t.Fatalf("durable owner approval failed: %v", err)
		}
		return approval.ApprovalID
	}
	assertRecordedDispatch := func(result *toolbrokerv1.ToolResult, err error) {
		t.Helper()
		if err != nil || result == nil || result.GetActionId() == "" || len(result.GetEvidenceRefs()) != 1 || result.GetError().GetRetryable() {
			t.Fatalf("approved Broker dispatch was not recorded: %+v %v", result, err)
		}
		// A browser DOM response proves runtime execution, not an external
		// recipient receipt. The fixture may inspect it without claiming that
		// an autonomous run has verified or completed an external effect.
		if (result.GetStatus() != "completed" || result.GetEffectState() != string(actionauth.StateConfirmed)) &&
			(result.GetStatus() != "effect_unknown" || result.GetEffectState() != string(actionauth.StateUnknown)) {
			t.Fatalf("incorrect dispatch/effect classification: %+v", result)
		}
		recorded, journalErr := actionauth.NewPostgresStore(pool).GetEffect(ctx, result.GetActionId())
		if journalErr != nil || string(recorded.State) != result.GetEffectState() || recorded.DispatchOutcome != "dispatch_completed" {
			t.Fatalf("durable journal differs from Broker response: %+v %v", recorded, journalErr)
		}
	}
	dispatch := func(call *toolbrokerv1.ToolCall) *toolbrokerv1.ToolResult {
		t.Helper()
		p, err := client.PrepareAction(ctx, call)
		if err != nil {
			t.Fatal(err)
		}
		id := ""
		if p.RequiresApproval {
			id = approve(call, p)
		}
		grant, err := client.AuthorizeAction(ctx, call, p.Binding.ActionID, id, "")
		if err != nil {
			t.Fatal(err)
		}
		result, err := client.ExecuteAuthorizedToolCall(ctx, call, grant)
		assertRecordedDispatch(result, err)
		if strings.Contains(result.GetResultJson(), secret) {
			t.Fatal("resolved secret leaked to model observation")
		}
		if json.Unmarshal([]byte(result.GetResultJson()), &observation) != nil {
			t.Fatal("invalid observation")
		}
		if strings.HasPrefix(call.GetToolName(), "browser.prepare_") &&
			(result.GetStatus() != "completed" || result.GetEffectState() != string(actionauth.StateConfirmed) ||
				observation["local_preparation"] != true || observation["network_state"] != "frozen" || submits.Load() != 0) {
			t.Fatal("local preparation lacks frozen-network proof or produced an external submission")
		}
		if _, err := pool.Exec(ctx, `UPDATE runs SET status='tool_pending',current_state='tool_pending' WHERE run_id=$1`, run.GetRunId()); err != nil {
			t.Fatal(err)
		}
		return result
	}
	args := func(selector string) map[string]any {
		return map[string]any{"browser_session_id": observation["browser_session_id"], "expected_url": observation["final_url"], "observation_version": observation["observation_version"], "selector": selector}
	}
	dispatch(newCall("browser.open", map[string]any{"url": egresstest.Origin + "/form"}))
	password := args("#password")
	password["value"] = map[string]string{"type": "credential_ref", "alias": "browser-account", "field": "password"}
	ref := &toolbrokerv1.CredentialRef{Type: "credential_ref", Alias: "browser-account", Field: "password"}
	denied := newCall("browser.prepare_fill", password, ref)
	prepared, err := client.PrepareAction(ctx, denied)
	if err != nil || !prepared.RequiresApproval {
		t.Fatal("credential preparation did not require approval")
	}
	if _, err := client.AuthorizeAction(ctx, denied, prepared.Binding.ActionID, "", ""); err == nil {
		t.Fatal("unapproved credential action allowed")
	}
	passwordCall := newCall("browser.prepare_fill", password, ref)
	dispatch(passwordCall)
	for _, field := range []struct{ id, value string }{{"a", "one"}, {"b", "two"}} {
		request := args("#" + field.id)
		request["value"] = field.value
		dispatch(newCall("browser.prepare_fill", request))
	}
	buy := newCall("browser.click", args("#submit"))
	p, err := client.PrepareAction(ctx, buy)
	if err != nil {
		t.Fatal(err)
	}
	approvalID := approve(buy, p)
	// An exact approved password action is not reusable for the submit action.
	var priorApproval string
	if err := pool.QueryRow(ctx, `SELECT approval_id FROM approval_actions WHERE action_id=$1`, actionauth.StableActionID(run.GetRunId(), passwordCall.GetToolCallId())).Scan(&priorApproval); err != nil {
		t.Fatal(err)
	}
	if _, err := client.AuthorizeAction(ctx, buy, p.Binding.ActionID, priorApproval, ""); err == nil {
		t.Fatal("credential preparation approval authorized submit")
	}
	grant, err := client.AuthorizeAction(ctx, buy, p.Binding.ActionID, approvalID, "")
	if err != nil {
		t.Fatal(err)
	}
	changed := proto.Clone(buy).(*toolbrokerv1.ToolCall)
	changed.ArgsJson = strings.Replace(buy.GetArgsJson(), "#submit", "#password", 1)
	if _, err := client.ExecuteAuthorizedToolCall(ctx, changed, grant); err != nil {
		t.Fatal(err)
	}
	if submits.Load() != 0 {
		t.Fatal("changed action reached submit")
	}
	result, err := client.ExecuteAuthorizedToolCall(ctx, buy, grant)
	assertRecordedDispatch(result, err)
	if result.GetStatus() != "effect_unknown" || result.GetEffectState() != string(actionauth.StateUnknown) {
		t.Fatal("DOM acknowledgement incorrectly proved the external submit")
	}
	if strings.Contains(result.GetResultJson(), secret) || !strings.Contains(result.GetResultJson(), "authenticated one two") {
		t.Fatal("authenticated session failed or secret leaked")
	}
	repeated, err := client.ExecuteAuthorizedToolCall(ctx, buy, grant)
	if err != nil || repeated.GetStatus() != "effect_unknown" || repeated.GetEffectState() != string(actionauth.StateUnknown) || repeated.GetError().GetRetryable() || repeated.GetActionId() != result.GetActionId() || submits.Load() != 1 {
		t.Fatal("approved action replayed or its uncertainty was hidden")
	}
	if len(repeated.GetEvidenceRefs()) != 1 || repeated.GetEvidenceRefs()[0] != result.GetEvidenceRefs()[0] {
		t.Fatal("repeated request created new dispatch evidence")
	}
	var leaks int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM approval_actions WHERE binding_json::text LIKE $1 OR continuation_json::text LIKE $1`, "%"+secret+"%").Scan(&leaks); err != nil || leaks != 0 {
		t.Fatal("raw secret persisted in approval binding/continuation")
	}
}
