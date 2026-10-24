package broker

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
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/butler/butler/apps/tool-broker/internal/registry"
	"github.com/butler/butler/apps/tool-broker/internal/runtimeclient"
	"github.com/butler/butler/internal/actionauth"
	"github.com/butler/butler/internal/credentials"
	commonv1 "github.com/butler/butler/internal/gen/common/v1"
	toolbrokerv1 "github.com/butler/butler/internal/gen/toolbroker/v1"
	"github.com/butler/butler/internal/security/egress/egresstest"
	"github.com/jackc/pgx/v5/pgxpool"
)

type browserCredentialMetadata struct{}

func (browserCredentialMetadata) GetByAlias(_ context.Context, alias string) (credentials.Record, error) {
	if alias != "browser-fixture" {
		return credentials.Record{}, credentials.ErrRecordNotFound
	}
	return credentials.Record{Alias: alias, Status: credentials.StatusActive, AllowedDomains: []string{"external.test"}, AllowedTools: []string{"browser.prepare_fill"}, ApprovalPolicy: credentials.ApprovalPolicyAlwaysConfirm, SecretRef: "fixture://browser-password"}, nil
}

type browserFixtureSecret struct{}

func (browserFixtureSecret) ResolveSecretRef(context.Context, string) (string, error) {
	return "browser-private-password", nil
}

func TestBrokerBrowserSequenceE2E(t *testing.T) {
	if os.Getenv("BUTLER_BROWSER_E2E") != "1" {
		t.Skip("real browser e2e requires isolated lab image")
	}
	binary := os.Getenv("BUTLER_BROWSER_E2E_BINARY")
	if binary == "" {
		t.Fatal("BUTLER_BROWSER_E2E_BINARY is required")
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := listener.Addr().String()
	_ = listener.Close()
	site := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/submit" {
			_ = r.ParseForm()
			_, _ = fmt.Fprintf(w, "<body>%s|%s</body>", r.Form.Get("a"), r.Form.Get("b"))
			return
		}
		_, _ = fmt.Fprint(w, `<body><form action="/submit" method="post"><input name="a" id="a"><input name="b" id="b"><input type="password" id="password"><button id="submit">Send</button></form></body>`)
	}))
	defer site.Close()
	cmd := exec.Command(binary, "-test.run=^TestBrowserRuntimeEgressHarness$")
	cmd.Env = append(os.Environ(), "BUTLER_GRPC_ADDR="+addr, "BUTLER_BROWSER_EGRESS_HARNESS=1", "BUTLER_BROWSER_FIXTURE_ENDPOINT="+site.URL)
	cmd.Stdout, cmd.Stderr = io.Discard, io.Discard
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if runtime.GOOS == "windows" {
			_ = cmd.Process.Kill()
		} else {
			_ = cmd.Process.Signal(os.Interrupt)
		}
		_ = cmd.Wait()
	})
	deadline := time.Now().Add(10 * time.Second)
	for {
		conn, err := net.DialTimeout("tcp", addr, 100*time.Millisecond)
		if err == nil {
			_ = conn.Close()
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("browser runtime did not start")
		}
		time.Sleep(25 * time.Millisecond)
	}
	var catalog struct {
		Tools []map[string]any `json:"tools"`
	}
	_, source, _, _ := runtime.Caller(0)
	repoRoot := filepath.Join(filepath.Dir(source), "..", "..", "..", "..")
	payload, err := os.ReadFile(filepath.Join(repoRoot, "configs", "tools.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(payload, &catalog); err != nil {
		t.Fatal(err)
	}
	for _, tool := range catalog.Tools {
		if tool["tool_class"] == "browser" {
			tool["runtime_target"] = addr
		}
	}
	payload, _ = json.Marshal(catalog)
	path := filepath.Join(t.TempDir(), "tools.json")
	if err := os.WriteFile(path, payload, 0600); err != nil {
		t.Fatal(err)
	}
	reg, err := registry.Load(path, addr)
	if err != nil {
		t.Fatal(err)
	}
	router := runtimeclient.New(nil)
	defer router.Close()
	// Use the real Credential Broker policy and resolver boundary. This fixture
	// intentionally requires approval and is denied before reading the secret.
	credentialBroker := credentials.NewToolCallBroker(credentials.NewBroker(browserCredentialMetadata{}), browserFixtureSecret{})
	server := NewServer(reg, router, credentialBroker, nil)
	fixture := setTestAuthority(server, credentialBroker)
	sequence := 0
	invoke := func(tool string, args map[string]any, refs ...*toolbrokerv1.CredentialRef) *toolbrokerv1.ToolResult {
		t.Helper()
		payload, _ := json.Marshal(args)
		ctx, cancel := context.WithTimeout(context.Background(), 40*time.Second)
		defer cancel()
		_ = ctx
		sequence++
		return dispatchForTest(t, server, fixture, &toolbrokerv1.ToolCall{RunId: "broker-run", ToolCallId: fmt.Sprintf("broker-call-%d", sequence), ToolName: tool, ArgsJson: string(payload), AutonomyMode: commonv1.AutonomyMode_AUTONOMY_MODE_2, CredentialRefs: refs}, len(refs) == 0)
	}
	var observation map[string]any
	accept := func(result *toolbrokerv1.ToolResult) {
		t.Helper()
		if result.GetStatus() != "completed" && (result.GetStatus() != "effect_unknown" || result.GetEffectState() != "effect_unknown" || result.GetActionId() == "" || result.GetError().GetRetryable()) {
			t.Fatalf("broker sequence failed: %+v", result)
		}
		if err := json.Unmarshal([]byte(result.GetResultJson()), &observation); err != nil {
			t.Fatal(err)
		}
	}
	args := func(selector string) map[string]any {
		return map[string]any{"browser_session_id": observation["browser_session_id"], "observation_version": observation["observation_version"], "expected_url": observation["final_url"], "selector": selector}
	}
	accept(invoke("browser.open", map[string]any{"url": egresstest.Origin + "/form"}))
	secretArgs := args("#password")
	secretArgs["value"] = map[string]string{"type": "credential_ref", "alias": "browser-fixture", "field": "password"}
	denied := invoke("browser.prepare_fill", secretArgs, &toolbrokerv1.CredentialRef{Type: "credential_ref", Alias: "browser-fixture", Field: "password"})
	if denied.GetStatus() != "failed" || !strings.Contains(denied.GetError().GetMessage(), "requires approval") {
		t.Fatal("credential approval was bypassed")
	}
	// Exercise the actual encrypted store and scoped resolver through Broker and
	// the Chromium runtime. Only action/session ownership is a boundary fixture.
	if dsn := os.Getenv("BUTLER_TEST_VAULT_POSTGRES_URL"); dsn != "" {
		vault := browserVaultFixture(t, dsn, repoRoot)
		entry, e := vault.Save(context.Background(), "owner", credentials.EntryInput{Alias: "personal", Title: "Synthetic account", Username: "fixture-login", Password: "private-vault-browser-password"})
		if e != nil {
			t.Fatal("vault entry")
		}
		r, e := vault.CreateRequest(context.Background(), credentials.Principal{OwnerID: "owner", SessionKey: "web:owner", RunID: "broker-run", TaskID: "task", TaskRevision: 1, ConstraintsRevision: 1}, "access", "access", entry.ID, observation["final_url"].(string), observation["browser_session_id"].(string), "Sign in to fixture", []string{"password"})
		if e != nil {
			t.Fatal("vault request")
		}
		credentialBroker = credentials.NewToolCallBroker(credentials.NewBroker(vault), vault)
		server = NewServer(reg, router, credentialBroker, nil)
		fixture = setTestAuthority(server, credentialBroker)
		fixture.inputClaimHash = strings.Repeat("a", 64)
		fixture.ownerMandate = &actionauth.Mandate{ID: "vault-goal-mandate", ScopeKind: "owner_goal", OwnerID: "owner", GoalID: "goal", GoalRevision: 1, Revision: 1, Status: "active", Purpose: "fixture-login", Tools: []string{"browser.prepare_fill"}, ResourceScopes: []actionauth.ResourceScope{{Origin: egresstest.Origin, PathPrefix: "/form"}}, Effects: []actionauth.Effect{actionauth.EffectPrepare}, ExpiresAt: time.Now().Add(time.Hour)}
		request := args("#password")
		request["value"] = map[string]string{"type": "credential_ref", "alias": r.ID, "field": "password"}
		ref := &toolbrokerv1.CredentialRef{Type: "credential_ref", Alias: r.ID, Field: "password"}
		encoded, _ := json.Marshal(request)
		pending, e := server.PrepareToolCall(context.Background(), &toolbrokerv1.PrepareToolCallRequest{ToolCall: &toolbrokerv1.ToolCall{RunId: "broker-run", ToolCallId: "pending-vault", ToolName: "browser.prepare_fill", ArgsJson: string(encoded), CredentialRefs: []*toolbrokerv1.CredentialRef{ref}, AutonomyMode: commonv1.AutonomyMode_AUTONOMY_MODE_2}})
		if e == nil && pending.GetError() == nil {
			t.Fatal("pending vault request released credential")
		}
		if vault.Respond(context.Background(), "owner", r.ID, entry.ID, true, "") != nil {
			t.Fatal("vault consent")
		}
		accept(invoke("browser.prepare_fill", request, ref))
		if observation["credential_verified"] != true || observation["network_state"] != "frozen" {
			t.Fatal("private browser comparison missing")
		}
		encoded, _ = json.Marshal(observation)
		if strings.Contains(string(encoded), "private-vault-browser-password") {
			t.Fatal("credential reflected to model")
		}
		if vault.Remove(context.Background(), "owner", entry.ID, entry.Revision, false) != nil {
			t.Fatal("revoke vault entry")
		}
		request = args("#password")
		request["value"] = map[string]string{"type": "credential_ref", "alias": r.ID, "field": "password"}
		encoded, _ = json.Marshal(request)
		revoked, e := server.PrepareToolCall(context.Background(), &toolbrokerv1.PrepareToolCallRequest{ToolCall: &toolbrokerv1.ToolCall{RunId: "broker-run", ToolCallId: "revoked-vault", ToolName: "browser.prepare_fill", ArgsJson: string(encoded), CredentialRefs: []*toolbrokerv1.CredentialRef{ref}, AutonomyMode: commonv1.AutonomyMode_AUTONOMY_MODE_2}})
		if e == nil && revoked.GetError() == nil {
			t.Fatal("revoked credential remained usable")
		}
		fixture.ownerMandate = nil
		t.Log("encrypted PostgreSQL vault: pending denial, owner consent under bounded goal mandate, Chromium equality, reflection masking and revocation verified")
	}
	for _, field := range []string{"a", "b"} {
		request := args("#" + field)
		request["value"] = "value-" + field
		accept(invoke("browser.prepare_fill", request))
		if observation["local_preparation"] != true || observation["network_state"] != "frozen" {
			t.Fatal("local preparation lacks frozen-network proof")
		}
	}
	accept(invoke("browser.snapshot", map[string]any{"browser_session_id": observation["browser_session_id"]}))
	accept(invoke("browser.click", args("#submit")))
	if observation["text"] != "value-a|value-b" {
		t.Fatalf("Broker/runtime sequence lost state: %v", observation["text"])
	}
	accept(invoke("browser.close", map[string]any{"browser_session_id": observation["browser_session_id"]}))
}

func browserVaultFixture(t *testing.T, dsn, repoRoot string) *credentials.Vault {
	t.Helper()
	ctx := context.Background()
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil || !strings.HasSuffix(cfg.ConnConfig.Database, "_test") {
		t.Fatal("disposable vault database required")
	}
	admin, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatal("fixture database")
	}
	schema := fmt.Sprintf("vault_browser_%d", time.Now().UnixNano())
	if _, err = admin.Exec(ctx, "CREATE SCHEMA "+schema); err != nil {
		t.Fatal("fixture schema")
	}
	cfg.ConnConfig.RuntimeParams["search_path"] = schema
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatal("fixture pool")
	}
	t.Cleanup(func() { pool.Close(); _, _ = admin.Exec(ctx, "DROP SCHEMA "+schema+" CASCADE"); admin.Close() })
	if _, err = pool.Exec(ctx, `CREATE TABLE sessions(session_key TEXT,user_id TEXT);CREATE TABLE runs(run_id TEXT,session_key TEXT,task_id TEXT,current_state TEXT);CREATE TABLE tasks(task_id TEXT,owner_id TEXT,revision BIGINT,constraints_revision BIGINT,status TEXT);CREATE TABLE server_browser_sessions(browser_session_id TEXT,run_id TEXT,owner_id TEXT,mode TEXT,expires_at TIMESTAMPTZ,fresh_needed BOOLEAN);INSERT INTO sessions VALUES('web:owner','owner');INSERT INTO runs VALUES('broker-run','web:owner','task','tool_pending');INSERT INTO tasks VALUES('task','owner',1,1,'active')`); err != nil {
		t.Fatal("fixture tables")
	}
	migration, err := os.ReadFile(filepath.Join(repoRoot, "migrations", "096_credential_vault.up.sql"))
	if err != nil {
		t.Fatal("fixture migration")
	}
	if _, err = pool.Exec(ctx, string(migration)); err != nil {
		t.Fatal("fixture migration SQL")
	}
	vault, err := credentials.NewVault(pool, strings.Repeat("synthetic-vault-key-", 3))
	if err != nil {
		t.Fatal("fixture vault")
	}
	return vault
}
