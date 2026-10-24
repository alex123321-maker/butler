package api

import (
	"context"
	"encoding/json"
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
