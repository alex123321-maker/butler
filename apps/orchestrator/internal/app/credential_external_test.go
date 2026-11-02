package app

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	durableinput "github.com/butler/butler/apps/orchestrator/internal/input"
	"github.com/butler/butler/internal/actionauth"
	"github.com/butler/butler/internal/browserpage"
	"github.com/butler/butler/internal/budget"
	"github.com/butler/butler/internal/credentialcontrol"
	"github.com/butler/butler/internal/credentials"
	"github.com/butler/butler/internal/externalauth"
	"github.com/butler/butler/internal/ownership"
	"github.com/jackc/pgx/v5/pgxpool"
)

// This composes the real source journal, vault request, protected decision and
// Core Input suspension/resumption. No model, provider or channel is simulated
// as successful authentication: the terminal control here is cancellation.
func TestCredentialExternalSameInputWaitAndCancel(t *testing.T) {
	testCredentialExternalProjection(t, false)
}

func TestCredentialExternalVerifiedFactProjection(t *testing.T) {
	testCredentialExternalProjection(t, true)
}

func testCredentialExternalProjection(t *testing.T, verified bool) {
	dsn := os.Getenv("BUTLER_TEST_VAULT_POSTGRES_URL")
	if dsn == "" {
		t.Skip("requires disposable *_test PostgreSQL")
	}
	cfg, e := pgxpool.ParseConfig(dsn)
	if e != nil || !strings.HasSuffix(cfg.ConnConfig.Database, "_test") {
		t.Fatal("isolated PG required")
	}
	ctx := context.Background()
	admin, e := pgxpool.NewWithConfig(ctx, cfg)
	if e != nil {
		t.Fatal(e)
	}
	defer admin.Close()
	schema := "external_kernel_" + time.Now().Format("150405000000000")
	if _, e = admin.Exec(ctx, `CREATE SCHEMA `+schema); e != nil {
		t.Fatal(e)
	}
	defer admin.Exec(ctx, `DROP SCHEMA `+schema+` CASCADE`)
	cfg.ConnConfig.RuntimeParams["search_path"] = schema
	pool, e := pgxpool.NewWithConfig(ctx, cfg)
	if e != nil {
		t.Fatal(e)
	}
	defer pool.Close()
	base := `CREATE TABLE sessions(session_key TEXT PRIMARY KEY,user_id TEXT);CREATE TABLE runs(run_id TEXT PRIMARY KEY,session_key TEXT,task_id TEXT,current_state TEXT,status TEXT,lease_id TEXT,updated_at TIMESTAMPTZ);CREATE TABLE tasks(task_id TEXT PRIMARY KEY,owner_id TEXT,revision BIGINT,constraints_revision BIGINT,status TEXT,parent_task_id TEXT,session_key TEXT,created_at TIMESTAMPTZ DEFAULT clock_timestamp());CREATE TABLE server_browser_sessions(browser_session_id TEXT PRIMARY KEY,run_id TEXT,owner_id TEXT,task_id TEXT,mode TEXT,epoch BIGINT NOT NULL DEFAULT 0,expires_at TIMESTAMPTZ);CREATE TABLE session_ownership(session_key TEXT,run_id TEXT,lease_id TEXT,fence_epoch BIGINT,expires_at TIMESTAMPTZ);CREATE TABLE accepted_inputs(input_id TEXT PRIMARY KEY,claim_id TEXT,run_id TEXT,status TEXT,checkpoint TEXT,checkpoint_json JSONB,claim_expires_at TIMESTAMPTZ,expires_at TIMESTAMPTZ,not_before TIMESTAMPTZ,updated_at TIMESTAMPTZ,owner_id TEXT,session_key TEXT,task_id TEXT);CREATE TABLE run_state_transitions(run_id TEXT,from_state TEXT,to_state TEXT,triggered_by TEXT,metadata_json JSONB,transitioned_at TIMESTAMPTZ);INSERT INTO sessions VALUES('session','owner');INSERT INTO tasks VALUES('task','owner',1,1,'active',NULL,'session',now());INSERT INTO runs VALUES('run','session','task','tool_running','tool_running','lease',now());INSERT INTO session_ownership VALUES('session','run','lease',1,now()+interval '1 hour');INSERT INTO accepted_inputs VALUES('input','claim','run','running','tool_dispatching','{}',now()+interval '1 hour',now()+interval '1 hour',now(),now(),'owner','session','task');`
	if _, e = pool.Exec(ctx, base); e != nil {
		t.Fatal("base", e)
	}
	if _, e = pool.Exec(ctx, `ALTER TABLE tasks ADD COLUMN updated_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp()`); e != nil {
		t.Fatal(e)
	}
	for _, name := range []string{"031_action_authorization", "035_external_effect_journal", "037_execution_budgets", "038_owner_goals_mandates", "038_owner_purpose_policies", "096_credential_vault", "106_browser_login_consent", "107_browser_pages", "108_browser_page_execution", "109_browser_sso_consent", "110_browser_sso_use", "111_browser_external_auth"} {
		raw, e := os.ReadFile(filepath.Join("..", "..", "..", "..", "migrations", name+".up.sql"))
		if e != nil {
			t.Fatal(e)
		}
		if _, e = pool.Exec(ctx, string(raw)); e != nil {
			t.Fatal(name, e)
		}
	}
	v, e := credentials.NewVault(pool, strings.Repeat("synthetic-key-", 4))
	if e != nil {
		t.Fatal(e)
	}
	controller := budget.NewController(pool, budget.DefaultPolicy())
	controller.SetAuthorityResolver(actionauth.ResolveBudgetAuthority)
	if v.ConfigureExternalAuth(ctx, externalauth.GoogleConfig{ClientID: "123-test.apps.googleusercontent.com", ClientSecret: "synthetic-client-canary", RedirectURL: "https://rp.example/auth/google/callback"}, "https://rp.example/", "https://rp.example/auth/google/account", controller) != nil {
		t.Fatal("profile")
	}
	k := &CredentialInteractions{Vault: v, Inputs: durableinput.NewStore(pool)}
	v.AfterResponse = k.readyTx
	p := credentials.Principal{OwnerID: "owner", SessionKey: "session", RunID: "run", TaskID: "task", TaskRevision: 1, ConstraintsRevision: 1}
	entry, e := v.Save(ctx, p.OwnerID, credentials.EntryInput{Title: "Dedicated Google", Alias: "google.fixture", Username: "synthetic@gmail.com", Password: "synthetic-password-canary"})
	if e != nil {
		t.Fatal(e)
	}
	scope := externalauth.Scope{Browser: strings.Repeat("a", 32), RPPage: browserpage.Ref{ID: strings.Repeat("b", 32), DocumentID: strings.Repeat("c", 32), Version: 1}, RPURL: "https://rp.example/", Provider: "google", VerifyURL: "https://rp.example/auth/google/account"}
	if _, e = pool.Exec(ctx, `INSERT INTO server_browser_sessions(browser_session_id,run_id,owner_id,task_id,mode,expires_at,page_mode,page_origin) VALUES($1,'run','owner','task','agent',now()+interval '10 minutes',true,'https://rp.example');`, scope.Browser); e != nil {
		t.Fatal(e)
	}
	if _, e = pool.Exec(ctx, `INSERT INTO server_browser_pages(browser_session_id,page_id,document_id,observation_version,observed_epoch,observed_url,state,runtime_version) VALUES($1,$2,$3,1,0,'https://rp.example/','ready',1)`, scope.Browser, scope.RPPage.ID, scope.RPPage.DocumentID); e != nil {
		t.Fatal(e)
	}
	m := actionauth.Mandate{ID: "mandate", OwnerID: p.OwnerID, ScopeKind: "owner_goal", GoalID: "goal", GoalRevision: 1, Revision: 1, Status: "active", Purpose: "external_test", Tools: []string{externalauth.RequestTool}, Effects: []actionauth.Effect{actionauth.EffectPrepare, actionauth.EffectCommit}, ExpiresAt: time.Now().Add(time.Hour), ResourceScopes: []actionauth.ResourceScope{{Origin: "https://rp.example", PathPrefix: "/"}, {Origin: "https://accounts.google.com", PathPrefix: "/o/oauth2/v2/auth"}, {Origin: "https://oauth2.googleapis.com", PathPrefix: "/token"}, {Origin: "https://www.googleapis.com", PathPrefix: "/oauth2/v3/certs"}}}
	g := actionauth.GoalPolicy{ID: m.GoalID, OwnerID: p.OwnerID, Kind: "objective", Status: "active", Revision: 1, Purpose: m.Purpose, ExpiresAt: m.ExpiresAt}
	graw, _ := json.Marshal(g)
	mraw, _ := json.Marshal(m)
	if _, e = pool.Exec(ctx, `INSERT INTO owner_goals(goal_id,owner_id,kind,status,revision,expires_at,goal_data,source_data) VALUES('goal','owner','objective','active',1,$1,$2,'{}')`, g.ExpiresAt, graw); e != nil {
		t.Fatal(e)
	}
	if _, e = pool.Exec(ctx, `INSERT INTO action_mandates(mandate_id,owner_id,task_id,revision,mandate_data,expires_at,scope_kind,goal_id,goal_revision,status) VALUES('mandate','owner','',1,$1,$2,'owner_goal','goal',1,'active');`, mraw, m.ExpiresAt); e != nil {
		t.Fatal(e)
	}
	if _, e = pool.Exec(ctx, `INSERT INTO owner_purpose_policies VALUES('owner','external_test',0);INSERT INTO task_authority_bindings(task_id,owner_id,goal_id,goal_revision,mandate_id,mandate_revision,purpose,initiative_reason,observation_ref) VALUES('task','owner','goal',1,'mandate',1,'external_test','owner-request','fixture')`); e != nil {
		t.Fatal(e)
	}
	args, _ := json.Marshal(externalauth.Request{RecordID: entry.ID, Purpose: "One external test login", Scope: scope})
	b := actionauth.ActionBinding{ActionID: actionauth.StableActionID("run", "external"), RunID: "run", ToolCallID: "external", ToolName: externalauth.RequestTool, ContractRevision: "1", Resource: "tool:" + externalauth.RequestTool, OwnerID: p.OwnerID, TaskID: p.TaskID, TaskRevision: 1, ConstraintsRevision: 1, GoalID: m.GoalID, GoalRevision: 1, MandateID: m.ID, MandateRevision: 1, Purpose: m.Purpose, Effect: actionauth.EffectPrepare, ExternalAuth: &scope}
	b.ArgsHash, _ = actionauth.ArgumentsHash(string(args))
	b.ContractHash, _ = actionauth.ContractHash(credentialcontrol.Contract(b.ToolName))
	br, _ := json.Marshal(b)
	dispatch := actionauth.DispatchIdentity{ActionID: "effect_" + strings.Repeat("f", 64), Fingerprint: "fixture"}
	if _, e = pool.Exec(ctx, `INSERT INTO action_effects(action_id,fingerprint,initial_binding,policy,state,authorization_id,dispatch_outcome) VALUES($1,'fixture',$2,'{}','executing','grant','executing')`, dispatch.ActionID, br); e != nil {
		t.Fatal(e)
	}
	if _, e = pool.Exec(ctx, `INSERT INTO action_intents(action_id,run_id,tool_call_id,action_hash,binding,requires_approval,expires_at,effect_id,effect_fingerprint,effect_policy) VALUES($1,'run','external','fixture',$2,false,now()+interval '1 hour',$3,'fixture','{}')`, b.ActionID, br, dispatch.ActionID); e != nil {
		t.Fatal(e)
	}
	if _, e = pool.Exec(ctx, `INSERT INTO action_effect_dispatches(authorization_id,action_id,execution_action_id,outcome) VALUES('grant',$1,$2,'executing')`, dispatch.ActionID, b.ActionID); e != nil {
		t.Fatal(e)
	}
	r, e := v.CreateExternalRequest(ctx, p, b.ToolCallID, dispatch, string(args))
	if e != nil {
		t.Fatal("real external request", e)
	}
	if _, e = pool.Exec(ctx, `UPDATE action_effects SET state='effect_confirmed',dispatch_outcome='dispatch_completed';UPDATE action_effect_dispatches SET outcome='dispatch_completed',finished_at=now()`); e != nil {
		t.Fatal(e)
	}
	claim := durableinput.Record{InputID: "input", ClaimID: "claim", RunID: "run"}
	owned := ownership.WithToken(durableinput.WithClaim(ctx, claim), ownership.NewAuthority(pool), ownership.Token{SessionKey: "session", RunID: "run", LeaseID: "lease", FenceEpoch: 1})
	if id, e := k.SuspendRequest(owned, "run", "external", externalauth.RequestTool); e != nil || id != r.ID {
		t.Fatal("same Input suspension", e)
	}
	if _, e = k.ResumeInteraction(owned, "run", r.ID); e == nil {
		t.Fatal("pending flow resumed")
	}
	notices := 0
	k.Notify = func(_ context.Context, got credentials.AccessRequest, ack string) (string, error) {
		notices++
		if got.ID != r.ID || got.ExternalScope == nil || len(got.Fields) != 0 {
			t.Fatal("unsafe notice")
		}
		return "external-ack", nil
	}
	if k.notifyExternal(ctx) != nil || k.notifyExternal(ctx) != nil || notices != 1 {
		t.Fatal("notice duplicate")
	}
	if _, e = v.RespondExternal(ctx, p.OwnerID, r.ID, entry.ID, true, externalauth.Consent, r.ScopeDigest); e != nil {
		t.Fatal("decision", e)
	}
	var checkpoint string
	if pool.QueryRow(ctx, `SELECT checkpoint FROM accepted_inputs WHERE input_id='input'`).Scan(&checkpoint) != nil || checkpoint != "awaiting_interaction" {
		t.Fatal("approval prematurely woke Input")
	}
	if _, e = k.ResumeInteraction(owned, "run", r.ID); e == nil {
		t.Fatal("approval mistaken for authentication")
	}
	if verified {
		// Seed a terminal owning ledger fact to test only Core projection. This
		// is not provider/Chromium/login acceptance; those have separate tests.
		if _, e = pool.Exec(ctx, `INSERT INTO browser_external_auth_launches(request_id) VALUES($1)`, r.ID); e != nil {
			t.Fatal(e)
		}
		if _, e = pool.Exec(ctx, `INSERT INTO browser_external_auth_exchanges(request_id,effect_id) VALUES($1,$2)`, r.ID, dispatch.ActionID); e != nil {
			t.Fatal(e)
		}
		if _, e = pool.Exec(ctx, `INSERT INTO browser_external_auth_results(request_id,outcome,rp_authenticated_read,rp_anonymous_challenge,rp_reference) VALUES($1,'verified',true,true,'mechanical-rp-reference')`, r.ID); e != nil {
			t.Fatal(e)
		}
		tx, err := pool.Begin(ctx)
		if err != nil {
			t.Fatal(err)
		}
		defer tx.Rollback(ctx)
		if k.readyTx(ctx, tx, "run", r.ID) != nil || tx.Commit(ctx) != nil {
			t.Fatal("terminal callback readiness")
		}
	} else if v.CancelExternal(ctx, p.OwnerID, r.ID, r.ScopeDigest) != nil {
		t.Fatal("cancel")
	}
	// Simulate the existing dispatcher reclaim, not a new Input or Task.
	if _, e = pool.Exec(ctx, `UPDATE accepted_inputs SET status='running',claim_id='claim',claim_expires_at=now()+interval '1 hour' WHERE input_id='input'`); e != nil {
		t.Fatal(e)
	}
	result, e := k.ResumeInteraction(owned, "run", r.ID)
	want := `"state":"cancelled"`
	if verified {
		want = `"state":"verified"`
	}
	if e != nil || !strings.Contains(result, want) || strings.Contains(result, "credential_alias") || strings.Contains(result, "synthetic-password-canary") {
		t.Fatal("terminal same Input projection", e)
	}
	if verified && (!strings.Contains(result, `"reference":"mechanical-rp-reference"`) || !strings.Contains(result, `"verify_url":"https://rp.example/auth/google/account"`) || !strings.Contains(result, `"browser_session_shared":false`) || !strings.Contains(result, `"rp_access_verified":true`)) {
		t.Fatal("verified fact needs unnecessary server-browser checks")
	}
}
