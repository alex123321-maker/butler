package broker

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/butler/butler/internal/credentialcontrol"
	"github.com/butler/butler/internal/credentials"
	toolbrokerv1 "github.com/butler/butler/internal/gen/toolbroker/v1"
)

func TestCredentialControlBrokerLocalPostgres(t *testing.T) {
	dsn := os.Getenv("BUTLER_TEST_VAULT_POSTGRES_URL")
	if dsn == "" {
		t.Skip("requires disposable local PostgreSQL")
	}
	_, source, _, _ := runtime.Caller(0)
	vault := browserVaultFixture(t, dsn, filepath.Join(filepath.Dir(source), "..", "..", "..", ".."))
	ctx := context.Background()
	entry, err := vault.Save(ctx, "owner", credentials.EntryInput{Alias: "shop", Title: "Personal", Username: "synthetic-login@example.com", Password: "synthetic-password"})
	if err != nil {
		t.Fatal("save fixture")
	}
	if _, err = vault.Pool.Exec(ctx, `INSERT INTO server_browser_sessions(browser_session_id,run_id,owner_id,mode,expires_at,fresh_needed,observed_url) VALUES('browser','broker-run','owner','agent',now()+interval '10 minutes',false,'https://example.com/login')`); err != nil {
		t.Fatal("browser fixture")
	}
	reg := loadTestRegistry(t)
	if reg.RegisterOwned(credentialcontrol.Contracts()) != nil {
		t.Fatal("register canonical contracts")
	}
	creds := credentials.NewToolCallBroker(credentials.NewBroker(vault), vault)
	server := NewServer(reg, nil, creds, nil)
	authority := setTestAuthority(server, creds)
	authority.inputClaimHash = strings.Repeat("a", 64)
	executor := &CredentialExecutor{Vault: vault, Contexts: authority}
	server.SetCatalogExecutor(executor)
	invoke := func(id, tool string, args any) *toolbrokerv1.ToolResult {
		t.Helper()
		encoded, _ := json.Marshal(args)
		result := dispatchForTest(t, server, authority, &toolbrokerv1.ToolCall{RunId: "broker-run", ToolCallId: id, ToolName: tool, ArgsJson: string(encoded)}, false)
		if strings.Contains(result.ResultJson, "synthetic-password") || strings.Contains(result.ResultJson, "synthetic-login@example.com") {
			t.Fatal("credential value exposed")
		}
		return result
	}
	if result := invoke("search", "credentials.search", map[string]any{"query": "synthetic-login@example.com"}); result.Status != "completed" || !strings.Contains(result.ResultJson, entry.ID) {
		t.Fatal("canonical private search failed")
	}
	args := map[string]any{"record_id": entry.ID, "browser_session_id": "browser", "expected_url": "https://other.example/login", "purpose": "Sign in", "fields": []string{"username", "password"}}
	if result := invoke("stale", "credentials.request_access", args); result.Status == "completed" {
		t.Fatal("model destination bypassed browser observation")
	}
	args["expected_url"] = "https://example.com/login"
	result := invoke("request", "credentials.request_access", args)
	var body struct {
		Request credentials.AccessRequest `json:"request"`
	}
	if result.Status != "completed" || json.Unmarshal([]byte(result.ResultJson), &body) != nil || body.Request.State != "pending" || body.Request.RecordID != entry.ID {
		t.Fatal("access widget request failed")
	}
	if _, err = vault.GetByAlias(ctx, body.Request.ID); err == nil {
		t.Fatal("requesting access released a secret")
	}
	if vault.Respond(ctx, "owner", body.Request.ID, entry.ID, true, "") != nil {
		t.Fatal("owner response")
	}
	if result = invoke("status", "credentials.request_status", map[string]any{"request_id": body.Request.ID}); result.Status != "completed" || !strings.Contains(result.ResultJson, `"state":"approved"`) {
		t.Fatal("approved safe status")
	}
	delete(args, "record_id")
	if result = invoke("create", "credentials.request_create", args); result.Status != "completed" || !strings.Contains(result.ResultJson, `"kind":"create"`) {
		t.Fatal("create widget request")
	}
	delete(args, "fields")
	args["access_request_id"] = body.Request.ID
	if result = invoke("otp", "credentials.request_otp", args); result.Status != "completed" || !strings.Contains(result.ResultJson, `"kind":"otp"`) {
		t.Fatal("OTP widget request")
	}
	if _, err = vault.Pool.Exec(ctx, `UPDATE server_browser_sessions SET page_mode=true WHERE browser_session_id='browser'`); err != nil {
		t.Fatal("managed context fixture")
	}
	var before, after int
	if vault.Pool.QueryRow(ctx, `SELECT count(*) FROM credential_vault_requests`).Scan(&before) != nil {
		t.Fatal("request count")
	}
	if result = invoke("managed-denied", "credentials.request_access", map[string]any{"record_id": entry.ID, "browser_session_id": "browser", "expected_url": "https://example.com/login", "purpose": "Sign in", "fields": []string{"username", "password"}}); result.Status == "completed" {
		t.Fatal("legacy credential consent accepted managed context")
	}
	if vault.Pool.QueryRow(ctx, `SELECT count(*) FROM credential_vault_requests`).Scan(&after) != nil || after != before {
		t.Fatal("managed denial created a vault request")
	}
	if _, err = executor.Execute(ctx, &toolbrokerv1.ToolCall{ToolName: "credentials.search", RunId: "broker-run", ArgsJson: `{}`}, credentialcontrol.Contract("credentials.search"), nil); err == nil {
		t.Fatal("direct executor bypassed dispatch authority")
	}
}
