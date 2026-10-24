package app

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	durableinput "github.com/butler/butler/apps/orchestrator/internal/input"
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
	base := `CREATE TABLE sessions(session_key TEXT PRIMARY KEY,user_id TEXT);CREATE TABLE runs(run_id TEXT PRIMARY KEY,session_key TEXT,task_id TEXT,current_state TEXT,status TEXT,lease_id TEXT,updated_at TIMESTAMPTZ);CREATE TABLE tasks(task_id TEXT PRIMARY KEY,owner_id TEXT,revision BIGINT,constraints_revision BIGINT,status TEXT);CREATE TABLE server_browser_sessions(browser_session_id TEXT);CREATE TABLE session_ownership(session_key TEXT,run_id TEXT,lease_id TEXT,fence_epoch BIGINT,expires_at TIMESTAMPTZ);CREATE TABLE accepted_inputs(input_id TEXT PRIMARY KEY,claim_id TEXT,run_id TEXT,status TEXT,checkpoint TEXT,checkpoint_json JSONB,claim_expires_at TIMESTAMPTZ,expires_at TIMESTAMPTZ,not_before TIMESTAMPTZ,updated_at TIMESTAMPTZ);CREATE TABLE run_state_transitions(run_id TEXT,from_state TEXT,to_state TEXT,triggered_by TEXT,metadata_json JSONB,transitioned_at TIMESTAMPTZ);INSERT INTO sessions VALUES('session','owner');INSERT INTO runs VALUES('run','session',NULL,'tool_running','tool_running','lease',now());INSERT INTO session_ownership VALUES('session','run','lease',1,now()+interval '1 hour');INSERT INTO accepted_inputs VALUES('input','claim','run','running','tool_dispatching','{}',now()+interval '1 hour',now()+interval '1 hour',now(),now());`
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
	if _, err = pool.Exec(ctx, `UPDATE session_ownership SET expires_at=now()-interval '1 second'`); err != nil {
		t.Fatal("expire lease")
	}
	if _, err = kernel.ResumeInteraction(owned, "run", request.ID); err == nil {
		t.Fatal("expired lease resumed")
	}
}
