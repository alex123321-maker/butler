package app

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	durableinput "github.com/butler/butler/apps/orchestrator/internal/input"
	"github.com/butler/butler/internal/browserpage"
	"github.com/butler/butler/internal/browsersso"
	"github.com/butler/butler/internal/credentials"
	"github.com/butler/butler/internal/ownership"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestCredentialVaultLocalWaitResumeAndExpiry(t *testing.T) {
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
		t.Fatal("admin pool")
	}
	defer admin.Close()
	schema := "credential_kernel_" + time.Now().Format("150405000000")
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
	base := `CREATE TABLE sessions(session_key TEXT PRIMARY KEY,user_id TEXT);CREATE TABLE runs(run_id TEXT PRIMARY KEY,session_key TEXT,task_id TEXT,current_state TEXT,status TEXT,lease_id TEXT,updated_at TIMESTAMPTZ);CREATE TABLE tasks(task_id TEXT PRIMARY KEY,owner_id TEXT,revision BIGINT,constraints_revision BIGINT,status TEXT);CREATE TABLE server_browser_sessions(browser_session_id TEXT PRIMARY KEY,run_id TEXT,owner_id TEXT,task_id TEXT,mode TEXT,epoch BIGINT NOT NULL DEFAULT 0,expires_at TIMESTAMPTZ);CREATE TABLE session_ownership(session_key TEXT,run_id TEXT,lease_id TEXT,fence_epoch BIGINT,expires_at TIMESTAMPTZ);CREATE TABLE accepted_inputs(input_id TEXT PRIMARY KEY,claim_id TEXT,run_id TEXT,status TEXT,checkpoint TEXT,checkpoint_json JSONB,claim_expires_at TIMESTAMPTZ,expires_at TIMESTAMPTZ,not_before TIMESTAMPTZ,updated_at TIMESTAMPTZ);CREATE TABLE run_state_transitions(run_id TEXT,from_state TEXT,to_state TEXT,triggered_by TEXT,metadata_json JSONB,transitioned_at TIMESTAMPTZ);INSERT INTO sessions VALUES('session','owner');INSERT INTO runs VALUES('run','session',NULL,'tool_running','tool_running','lease',now());INSERT INTO session_ownership VALUES('session','run','lease',1,now()+interval '1 hour');INSERT INTO accepted_inputs VALUES('input','claim','run','running','tool_dispatching','{}',now()+interval '1 hour',now()+interval '1 hour',now(),now());`
	if _, err = pool.Exec(ctx, base); err != nil {
		t.Fatal("fixture")
	}
	migration, err := os.ReadFile(filepath.Join("..", "..", "..", "..", "migrations", "096_credential_vault.up.sql"))
	if err != nil {
		t.Fatal("migration")
	}
	if _, err = pool.Exec(ctx, string(migration)); err != nil {
		t.Fatal("migration apply")
	}
	loginMigration, err := os.ReadFile(filepath.Join("..", "..", "..", "..", "migrations", "106_browser_login_consent.up.sql"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, string(loginMigration)); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"107_browser_pages", "108_browser_page_execution", "109_browser_sso_consent", "110_browser_sso_use"} {
		raw, e := os.ReadFile(filepath.Join("..", "..", "..", "..", "migrations", name+".up.sql"))
		if e != nil {
			t.Fatal(e)
		}
		if _, e = pool.Exec(ctx, string(raw)); e != nil {
			t.Fatal(e)
		}
	}
	vault, err := credentials.NewVault(pool, strings.Repeat("synthetic-key-", 4))
	if err != nil {
		t.Fatal("vault")
	}
	kernel := &CredentialInteractions{Vault: vault, Inputs: durableinput.NewStore(pool)}
	vault.AfterResponse = kernel.readyTx
	p := credentials.Principal{OwnerID: "owner", SessionKey: "session", RunID: "run"}
	request, err := vault.CreateRequest(ctx, p, "call", "create", "", "https://example.com/login", "browser", "Sign in", []string{"username", "password"})
	if err != nil {
		t.Fatal("request")
	}
	claim := durableinput.Record{InputID: "input", ClaimID: "claim", RunID: "run"}
	owned := ownership.WithToken(durableinput.WithClaim(ctx, claim), ownership.NewAuthority(pool), ownership.Token{SessionKey: "session", RunID: "run", LeaseID: "lease", FenceEpoch: 1})
	if _, err = kernel.SuspendRequest(owned, "run", "call"); err != nil {
		t.Fatal("suspend")
	}
	var stage string
	if pool.QueryRow(ctx, `SELECT checkpoint FROM accepted_inputs WHERE input_id='input'`).Scan(&stage) != nil || stage != "awaiting_interaction" {
		t.Fatal("wait checkpoint")
	}
	if vault.Respond(ctx, "owner", request.ID, "", false, "") != nil {
		t.Fatal("response")
	}
	if pool.QueryRow(ctx, `SELECT checkpoint FROM accepted_inputs WHERE input_id='input'`).Scan(&stage) != nil || stage != "interaction_ready" {
		t.Fatal("ready checkpoint")
	}
	answer, err := kernel.ResumeInteraction(owned, "run", request.ID)
	if err != nil || !strings.Contains(answer, `"state":"denied"`) {
		t.Fatal("canonical resume")
	}
	reset := func() {
		t.Helper()
		if _, e := pool.Exec(ctx, `UPDATE runs SET current_state='tool_running',status='tool_running';UPDATE accepted_inputs SET checkpoint='tool_dispatching',checkpoint_json='{}',status='running'`); e != nil {
			t.Fatal("reset fixture")
		}
	}
	reset()
	early, err := vault.CreateRequest(ctx, p, "early-call", "create", "", "https://another.example/login", "browser", "Sign in", []string{"username", "password"})
	if err != nil {
		t.Fatal("early request")
	}
	entry, err := vault.Save(ctx, "owner", credentials.EntryInput{Alias: "personal", Title: "Personal", Username: "synthetic-user", Password: "synthetic-password"})
	if err != nil {
		t.Fatal("early entry")
	}
	if vault.Respond(ctx, "owner", early.ID, entry.ID, true, "") != nil {
		t.Fatal("early owner response")
	}
	if _, err = kernel.SuspendRequest(owned, "run", "early-call"); err != nil {
		t.Fatal("early response suspension")
	}
	if pool.QueryRow(ctx, `SELECT checkpoint FROM accepted_inputs WHERE input_id='input'`).Scan(&stage) != nil || stage != "interaction_ready" {
		t.Fatal("early response lost")
	}
	answer, err = kernel.ResumeInteraction(owned, "run", early.ID)
	if err != nil || !strings.Contains(answer, `"state":"approved"`) || strings.Contains(answer, "synthetic-password") {
		t.Fatal("approved response privacy")
	}
	reset()
	expiring, err := vault.CreateRequest(ctx, p, "expiring-call", "create", "", "https://example.com/login", "browser", "Sign in", []string{"username", "password"})
	if err != nil {
		t.Fatal("expiring request")
	}
	if _, err = kernel.SuspendRequest(owned, "run", "expiring-call"); err != nil {
		t.Fatal("expiry suspension")
	}
	if _, err = pool.Exec(ctx, `UPDATE credential_vault_requests SET expires_at=now()-interval '1 second' WHERE id=$1`, expiring.ID); err != nil {
		t.Fatal("expire request")
	}
	if kernel.Sweep(ctx) != nil {
		t.Fatal("expiry maintenance")
	}
	answer, err = kernel.ResumeInteraction(owned, "run", expiring.ID)
	if err != nil || !strings.Contains(answer, `"state":"expired"`) {
		t.Fatal("expired request did not wake owning input")
	}
	// Close delivered cards even after the Input moves to another interaction.
	if _, err = pool.Exec(ctx, `UPDATE credential_vault_requests SET notice_state='unknown';UPDATE accepted_inputs SET checkpoint_json='{}'`); err != nil {
		t.Fatal("notice fixture")
	}
	if _, err = pool.Exec(ctx, `UPDATE credential_vault_requests SET notice_state='sent',notice_ack='fixture-ack',notice_phase='pending' WHERE id=$1`, request.ID); err != nil {
		t.Fatal("notice fixture")
	}
	closed := 0
	kernel.Notify = func(_ context.Context, r credentials.AccessRequest, ack string) (string, error) {
		if r.ID != request.ID || r.State != "denied" || ack != "fixture-ack" {
			t.Fatal("incorrect closed-card projection")
		}
		closed++
		return ack, nil
	}
	if kernel.Sweep(ctx) != nil || kernel.Sweep(ctx) != nil || closed != 1 {
		t.Fatal("closed card was lost or replayed")
	}
	t.Run("SSO_same_Input_decision_and_lost_context", func(t *testing.T) {
		reset()
		kernel.Notify = nil
		scope := browsersso.Scope{Browser: strings.Repeat("a", 32), RPPage: browserpage.Ref{ID: strings.Repeat("b", 32), DocumentID: strings.Repeat("c", 32), Version: 1}, RPURL: "https://rp.example/", IdPURL: "https://idp.example/authorize", TargetURL: "https://idp.example/login", VerifyURL: "https://rp.example/account", Trigger: "#sso", Form: "form", Username: "input[name=username]", Password: "input[type=password]"}
		if _, e := pool.Exec(ctx, `INSERT INTO server_browser_sessions(browser_session_id,run_id,owner_id,task_id,mode,expires_at,page_mode,page_origin) VALUES($1,'run','owner','','agent',now()+interval '10 minutes',true,'https://rp.example')`, scope.Browser); e != nil {
			t.Fatal(e)
		}
		if _, e := pool.Exec(ctx, `INSERT INTO server_browser_pages(browser_session_id,page_id,document_id,observation_version,observed_epoch,observed_url,state,runtime_version) VALUES($1,$2,$3,1,0,'https://rp.example/','ready',1)`, scope.Browser, scope.RPPage.ID, scope.RPPage.DocumentID); e != nil {
			t.Fatal(e)
		}
		r, e := vault.CreateSSORequest(ctx, p, "sso-request", entry.ID, "One SSO", scope)
		if e != nil {
			t.Fatal(e)
		}
		if id, e := kernel.SuspendRequest(owned, "run", "sso-request", browsersso.RequestTool); e != nil || id != r.ID {
			t.Fatal("SSO suspend", e)
		}
		if _, e := kernel.ResumeInteraction(owned, "run", r.ID); e == nil {
			t.Fatal("pending request resumed")
		}
		notices := 0
		kernel.Notify = func(_ context.Context, got credentials.AccessRequest, ack string) (string, error) {
			if got.ID != r.ID || got.SSOScope == nil || got.SSOScope.IdPURL != scope.IdPURL {
				t.Fatal("incorrect SSO notice")
			}
			notices++
			if notices > 1 && ack != "sso-ack" {
				t.Fatal("notice lost original ack")
			}
			return "sso-ack", nil
		}
		if kernel.Sweep(ctx) != nil || notices != 1 {
			t.Fatal("pending SSO notice")
		}
		if vault.RespondSSO(ctx, p.OwnerID, r.ID, entry.ID, true, browsersso.Consent, r.ScopeDigest) != nil {
			t.Fatal("SSO owner decision")
		}
		result, e := kernel.ResumeInteraction(owned, "run", r.ID)
		if e != nil || !strings.Contains(result, `"state":"approved"`) || !strings.Contains(result, r.ScopeDigest) {
			t.Fatal("same-Input SSO resume", e)
		}
		if kernel.Sweep(ctx) != nil || kernel.Sweep(ctx) != nil || notices != 2 {
			t.Fatal("SSO decision notice repeated")
		}
		kernel.Notify = nil
		reset()
		lost, e := vault.CreateSSORequest(ctx, p, "lost-sso", entry.ID, "Lost context", scope)
		if e != nil {
			t.Fatal(e)
		}
		if _, e = kernel.SuspendRequest(owned, "run", "lost-sso", browsersso.RequestTool); e != nil {
			t.Fatal(e)
		}
		if _, e = pool.Exec(ctx, `UPDATE server_browser_sessions SET epoch=epoch+1 WHERE browser_session_id=$1`, scope.Browser); e != nil {
			t.Fatal(e)
		}
		if kernel.Sweep(ctx) != nil {
			t.Fatal("SSO expiry wake")
		}
		result, e = kernel.ResumeInteraction(owned, "run", lost.ID)
		if e != nil || !strings.Contains(result, `"state":"expired"`) {
			t.Fatal("lost SSO context stayed pending", e)
		}
	})
	if _, err = pool.Exec(ctx, `UPDATE session_ownership SET expires_at=now()-interval '1 second'`); err != nil {
		t.Fatal("expire lease")
	}
	if _, err = kernel.ResumeInteraction(owned, "run", request.ID); err == nil {
		t.Fatal("expired lease resumed")
	}
}
