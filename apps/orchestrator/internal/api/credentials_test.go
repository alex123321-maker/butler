package api

import (
	"context"
	"encoding/json"
	"github.com/butler/butler/internal/browserauth"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/butler/butler/internal/credentials"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestCredentialVaultOwnerAPIPrivacyLocal(t *testing.T) {
	dsn := os.Getenv("BUTLER_TEST_VAULT_POSTGRES_URL")
	if dsn == "" {
		t.Skip("requires disposable local PostgreSQL")
	}
	ctx := context.Background()
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil || !strings.HasSuffix(cfg.ConnConfig.Database, "_test") {
		t.Fatal("test database required")
	}
	admin, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatal("admin")
	}
	defer admin.Close()
	schema := "credential_api_" + time.Now().Format("150405000000")
	if _, err = admin.Exec(ctx, `CREATE SCHEMA `+schema); err != nil {
		t.Fatal("schema")
	}
	defer admin.Exec(ctx, `DROP SCHEMA `+schema+` CASCADE`)
	cfg.ConnConfig.RuntimeParams["search_path"] = schema
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatal("pool")
	}
	defer pool.Close()
	if _, err = pool.Exec(ctx, `CREATE TABLE server_browser_sessions(browser_session_id TEXT)`); err != nil {
		t.Fatal("base")
	}
	migration, err := os.ReadFile(filepath.Join("..", "..", "..", "..", "migrations", "096_credential_vault.up.sql"))
	if err != nil {
		t.Fatal("migration")
	}
	if _, err = pool.Exec(ctx, string(migration)); err != nil {
		t.Fatal("migrate")
	}
	loginMigration, err := os.ReadFile(filepath.Join("..", "..", "..", "..", "migrations", "106_browser_login_consent.up.sql"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, string(loginMigration)); err != nil {
		t.Fatal(err)
	}
	vault, err := credentials.NewVault(pool, strings.Repeat("synthetic-key-", 4))
	if err != nil {
		t.Fatal("vault")
	}
	ownerToken := strings.Repeat("owner-", 8)
	handler := CredentialVault(NewOwnerControlAuth(ownerToken, "owner"), vault)
	invoke := func(method, body, unlock string, authenticated bool) *httptest.ResponseRecorder {
		t.Helper()
		request := httptest.NewRequest(method, "/api/v4/credentials", strings.NewReader(body))
		if authenticated {
			request.Header.Set("Authorization", "Bearer "+ownerToken)
		}
		if unlock != "" {
			request.Header.Set("X-Butler-Vault", unlock)
		}
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		if response.Header().Get("Cache-Control") != "no-store" {
			t.Fatal("secret endpoint cached")
		}
		return response
	}
	if invoke("GET", "", "", false).Code != 401 {
		t.Fatal("owner proof missing")
	}
	if invoke("POST", `{"action":"list"}`, "", true).Code != 423 {
		t.Fatal("unlock missing")
	}
	opened := invoke("POST", `{"action":"unlock","pin":"654321","confirm":"654321"}`, "", true)
	var token struct {
		Value string `json:"unlock_token"`
	}
	if opened.Code != 200 || json.Unmarshal(opened.Body.Bytes(), &token) != nil || token.Value == "" {
		t.Fatal("unlock")
	}
	saved := invoke("POST", `{"action":"save","entry":{"alias":"shop","title":"Personal","username":"private-login@example.com","password":"synthetic-private-password"}}`, token.Value, true)
	if saved.Code != 200 {
		t.Fatal("save")
	}
	for _, response := range []*httptest.ResponseRecorder{saved, invoke("POST", `{"action":"list","query":"private-login@example.com"}`, token.Value, true)} {
		if strings.Contains(response.Body.String(), "private-login@example.com") || strings.Contains(response.Body.String(), "synthetic-private-password") {
			t.Fatal("private value in metadata response")
		}
	}
	malformed := invoke("POST", `{"action":"list","action":"save","password":"synthetic-private-password"}`, token.Value, true)
	if malformed.Code != 400 || strings.Contains(malformed.Body.String(), "synthetic-private-password") {
		t.Fatal("strict request or error privacy")
	}
	t.Run("old_client_cannot_approve_login_submission", func(t *testing.T) {
		if _, e := pool.Exec(ctx, `CREATE TABLE sessions(session_key TEXT PRIMARY KEY,user_id TEXT);CREATE TABLE runs(run_id TEXT PRIMARY KEY,session_key TEXT,task_id TEXT,current_state TEXT);CREATE TABLE tasks(task_id TEXT PRIMARY KEY,owner_id TEXT,revision BIGINT,constraints_revision BIGINT,status TEXT);INSERT INTO sessions VALUES('session','owner');INSERT INTO runs VALUES('run','session',NULL,'tool_running')`); e != nil {
			t.Fatal(e)
		}
		var receipt struct {
			Entry struct {
				ID string `json:"id"`
			} `json:"entry"`
		}
		if json.Unmarshal(saved.Body.Bytes(), &receipt) != nil || receipt.Entry.ID == "" {
			t.Fatal("entry receipt")
		}
		p := credentials.Principal{OwnerID: "owner", SessionKey: "session", RunID: "run"}
		scope := browserauth.Scope{Browser: "browser", ExpectedURL: "https://site.example/login", TargetURL: "https://site.example/session", Form: "form", Username: "#u", Password: "#p"}
		request, e := vault.CreateLoginRequest(ctx, p, "login", receipt.Entry.ID, "Sign in once", scope)
		if e != nil {
			t.Fatal(e)
		}
		body := map[string]any{"action": "respond", "id": request.ID, "record_id": receipt.Entry.ID, "allow": true}
		raw, _ := json.Marshal(body)
		if invoke("POST", string(raw), token.Value, true).Code != 409 {
			t.Fatal("old client upgraded access consent")
		}
		body["login_authorization"] = browserauth.Consent
		raw, _ = json.Marshal(body)
		if invoke("POST", string(raw), token.Value, true).Code != 200 {
			t.Fatal("explicit login consent rejected")
		}
		legacy, e := vault.CreateRequest(ctx, p, "access", "access", receipt.Entry.ID, scope.ExpectedURL, scope.Browser, "Local preparation", []string{"password"})
		if e != nil {
			t.Fatal(e)
		}
		body["id"] = legacy.ID
		delete(body, "login_authorization")
		raw, _ = json.Marshal(body)
		if invoke("POST", string(raw), token.Value, true).Code != 200 {
			t.Fatal("legacy client preparation consent changed")
		}
	})
	if invoke("POST", `{"action":"change_pin","pin":"123456","confirm":"123456"}`, token.Value, true).Code != 200 {
		t.Fatal("PIN change")
	}
	if invoke("POST", `{"action":"list"}`, token.Value, true).Code != 423 {
		t.Fatal("old unlock survived PIN change")
	}
}

func TestCredentialVaultUnavailableRequiresOwner(t *testing.T) {
	handler := CredentialVault(NewOwnerControlAuth(strings.Repeat("a", 32), "owner"), nil)
	r := httptest.NewRequest(http.MethodGet, "/api/v4/credentials", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)
	if w.Code != 401 {
		t.Fatal("unauthenticated vault readiness exposed")
	}
}
