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
	"sync/atomic"
	"testing"
	"time"

	"github.com/butler/butler/apps/tool-broker/internal/registry"
	"github.com/butler/butler/apps/tool-broker/internal/runtimeclient"
	"github.com/butler/butler/internal/actionauth"
	"github.com/butler/butler/internal/browserauth"
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
	var loginPosts, accountReads atomic.Int32
	site := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/login" {
			fmt.Fprint(w, `<html><body><form method="post" action="/session"><input name="username" id="u"><input name="password" id="p" type="password"><input name="csrf" type="hidden" value="private-csrf"><button>Login</button></form></body></html>`)
			return
		}
		if r.URL.Path == "/session" {
			loginPosts.Add(1)
			_ = r.ParseForm()
			if r.Method != "POST" || r.PostForm.Get("username") != "private-login-user" || r.PostForm.Get("password") != "private-login-password" || r.PostForm.Get("csrf") != "private-csrf" {
				w.WriteHeader(401)
				return
			}
			http.SetCookie(w, &http.Cookie{Name: "auth", Value: "private-cookie", Path: "/", HttpOnly: true})
			http.Redirect(w, r, "/account", 303)
			return
		}
		if r.URL.Path == "/account" {
			c, e := r.Cookie("auth")
			if e != nil || c.Value != "private-cookie" {
				http.Redirect(w, r, "/login", 302)
				return
			}
			accountReads.Add(1)
			fmt.Fprint(w, `<body>Account reference B-432. private-cookie private-login-password</body>`)
			return
		}
		if r.URL.Path == "/changing-form" {
			_, _ = fmt.Fprint(w, `<body><input id="a"><span id="help">Loading</span><script>setTimeout(()=>document.getElementById('help').textContent='Ready',700)</script></body>`)
			return
		}
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
	accept(invoke("browser.open", map[string]any{"url": egresstest.Origin + "/changing-form"}))
	time.Sleep(900 * time.Millisecond)
	stale := args("#a")
	stale["value"] = "ordinary"
	rejected := invoke("browser.prepare_fill", stale)
	if rejected.GetStatus() != "failed" || rejected.GetEffectState() != "effect_absent" || !strings.Contains(rejected.GetError().GetDetailsJson(), `"input_dispatched":false`) {
		t.Fatal("actual Chromium stale preparation lacked no-input proof", rejected.GetStatus(), rejected.GetEffectState())
	}
	accept(invoke("browser.snapshot", map[string]any{"browser_session_id": observation["browser_session_id"]}))
	fresh := args("#a")
	fresh["value"] = "ordinary"
	prepared := invoke("browser.prepare_fill", fresh)
	if prepared.GetEffectState() != "effect_confirmed" {
		t.Fatal("fresh observed local preparation could not continue")
	}
	accept(prepared)
	accept(invoke("browser.close", map[string]any{"browser_session_id": observation["browser_session_id"]}))
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
		// New consent is a distinct one-use protocol; the legacy preparation
		// request above cannot authorize it. No exact action approval/report.
		accept(invoke("browser.close", map[string]any{"browser_session_id": observation["browser_session_id"]}))
		accept(invoke("browser.open", map[string]any{"url": egresstest.Origin + "/login"}))
		loginEntry, e := vault.Save(context.Background(), "owner", credentials.EntryInput{Alias: "single-login", Title: "Single login", Username: "private-login-user", Password: "private-login-password"})
		if e != nil {
			t.Fatal(e)
		}
		scope := browserauth.Scope{Browser: observation["browser_session_id"].(string), ExpectedURL: observation["final_url"].(string), TargetURL: egresstest.Origin + "/session", Form: "form", Username: "#u", Password: "#p"}
		consent, e := vault.CreateLoginRequest(context.Background(), credentials.Principal{OwnerID: "owner", SessionKey: "web:owner", RunID: "broker-run", TaskID: "task", TaskRevision: 1, ConstraintsRevision: 1}, "single-login-request", loginEntry.ID, "One verified login", scope)
		if e != nil {
			t.Fatal(e)
		}
		if e = vault.Respond(context.Background(), "owner", consent.ID, loginEntry.ID, true, ""); e != nil {
			t.Fatal(e)
		}
		fixture.ownerMandate = &actionauth.Mandate{ID: "login-mandate", ScopeKind: "owner_goal", OwnerID: "owner", GoalID: "login-goal", GoalRevision: 1, Revision: 1, Status: "active", Purpose: "single-login", Tools: []string{browserauth.Tool}, ResourceScopes: []actionauth.ResourceScope{{Origin: egresstest.Origin, PathPrefix: "/"}}, Effects: []actionauth.Effect{actionauth.EffectCommit}, ExpiresAt: time.Now().Add(time.Hour)}
		loginArgs := map[string]any{"browser_session_id": scope.Browser, "expected_url": scope.ExpectedURL, "target_url": scope.TargetURL, "form_selector": scope.Form, "username_selector": scope.Username, "password_selector": scope.Password, "observation_version": observation["observation_version"], "username": map[string]string{"type": "credential_ref", "alias": consent.ID, "field": "username"}, "password": map[string]string{"type": "credential_ref", "alias": consent.ID, "field": "password"}}
		refs := []*toolbrokerv1.CredentialRef{{Type: "credential_ref", Alias: consent.ID, Field: "username"}, {Type: "credential_ref", Alias: consent.ID, Field: "password"}}
		logged := invoke(browserauth.Tool, loginArgs, refs...)
		if logged.GetStatus() != "completed" || logged.GetEffectState() != string(actionauth.StateConfirmed) {
			t.Fatal("single-consent login not confirmed", logged.GetError())
		}
		accept(logged)
		if !strings.Contains(observation["text"].(string), "B-432") || strings.Contains(logged.ResultJson, "private-") || loginPosts.Load() != 1 || accountReads.Load() != 2 {
			t.Fatal("single login, verification, masking or account observation failed")
		}
		replayed := invoke(browserauth.Tool, loginArgs, refs...)
		if replayed.GetStatus() == "completed" || loginPosts.Load() != 1 {
			t.Fatal("spent login consent replayed POST")
		}
		fixture.ownerMandate = nil
		accept(invoke("browser.close", map[string]any{"browser_session_id": observation["browser_session_id"]}))
		accept(invoke("browser.open", map[string]any{"url": egresstest.Origin + "/form"}))
		t.Log("scoped login: one vault consent, zero extra action approvals or factual reports, one POST, two authenticated reads, no credential/cookie leak, replay denied")
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

	loginMigration, loginErr := os.ReadFile(filepath.Join(repoRoot, "migrations", "106_browser_login_consent.up.sql"))
	if loginErr != nil {
		t.Fatal(loginErr)
	}
	if _, loginErr = pool.Exec(ctx, string(loginMigration)); loginErr != nil {
		t.Fatal(loginErr)
	}
	vault, err := credentials.NewVault(pool, strings.Repeat("synthetic-vault-key-", 3))
	if err != nil {
		t.Fatal("fixture vault")
	}
	return vault
}
