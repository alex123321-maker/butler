package credentials

import (
	"context"
	"encoding/json"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/butler/butler/internal/actionauth"
	"github.com/butler/butler/internal/budget"
	"github.com/butler/butler/internal/credentialcontrol"
	"github.com/butler/butler/internal/externalauth"
	"github.com/jackc/pgx/v5"
)

func externalFixture(t *testing.T) (*Vault, Principal, Entry, externalauth.Scope, actionauth.Mandate, string) {
	t.Helper()
	ctx := t.Context()
	v, p, _, source := ssoFixture(t)
	base := `ALTER TABLE tasks ADD COLUMN parent_task_id TEXT,ADD COLUMN created_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),ADD COLUMN session_key TEXT NOT NULL DEFAULT 'session';
CREATE TABLE input_stopped_attempts(run_id TEXT PRIMARY KEY,owner_id TEXT,session_key TEXT,classification TEXT);
CREATE TABLE accepted_inputs(input_id TEXT PRIMARY KEY,run_id TEXT,session_key TEXT,owner_id TEXT,task_id TEXT,status TEXT,checkpoint TEXT,checkpoint_json JSONB,expires_at TIMESTAMPTZ,claim_id TEXT,claim_expires_at TIMESTAMPTZ);
INSERT INTO accepted_inputs VALUES('input','run','session','owner','task','running','tool_dispatching','{}',clock_timestamp()+interval '1 hour','claim',clock_timestamp()+interval '1 hour');`
	if _, e := v.Pool.Exec(ctx, base); e != nil {
		t.Fatal("external input fixture", e)
	}
	for _, name := range []string{"031_action_authorization", "035_external_effect_journal", "037_execution_budgets", "038_owner_goals_mandates", "038_owner_purpose_policies", "111_browser_external_auth"} {
		raw, e := os.ReadFile(filepath.Join("..", "..", "migrations", name+".up.sql"))
		if e != nil {
			t.Fatal("migration file", e)
		}
		if _, e = v.Pool.Exec(ctx, string(raw)); e != nil {
			t.Fatal("migration", name, e)
		}
	}
	s := externalauth.Scope{Browser: source.Browser, RPPage: source.RPPage, RPURL: source.RPURL, Provider: "google", VerifyURL: "https://rp.example/auth/google/account"}
	cfg := externalauth.GoogleConfig{ClientID: "123-test.apps.googleusercontent.com", ClientSecret: "synthetic-client-secret-canary", RedirectURL: "https://rp.example/auth/google/callback"}
	budgets := budget.NewController(v.Pool, budget.DefaultPolicy())
	budgets.SetAuthorityResolver(actionauth.ResolveBudgetAuthority)
	if v.ConfigureExternalAuth(ctx, cfg, s.RPURL, s.VerifyURL, budgets) != nil {
		t.Fatal("operator profile")
	}
	entry, e := v.Save(ctx, p.OwnerID, EntryInput{Alias: "google.test", Title: "Dedicated Google fixture", Username: "synthetic@example.invalid", Password: "synthetic-private-password-canary"})
	if e != nil {
		t.Fatal("account")
	}
	now := time.Now().UTC().Truncate(time.Microsecond)
	m := actionauth.Mandate{ID: "mandate-external", OwnerID: p.OwnerID, ScopeKind: "owner_goal", GoalID: "goal-external", GoalRevision: 1, Revision: 1, Status: "active", Purpose: "external_auth_test", Tools: []string{externalauth.RequestTool}, Effects: []actionauth.Effect{actionauth.EffectPrepare, actionauth.EffectCommit}, ExpiresAt: now.Add(time.Hour), ResourceScopes: []actionauth.ResourceScope{{Origin: "https://rp.example", PathPrefix: "/"}, {Origin: "https://accounts.google.com", PathPrefix: "/o/oauth2/v2/auth"}, {Origin: "https://oauth2.googleapis.com", PathPrefix: "/token"}, {Origin: "https://www.googleapis.com", PathPrefix: "/oauth2/v3/certs"}}}
	m.Budget = actionauth.MandateBudget{ModelTurns: 64, Tokens: 67108864, ExternalCalls: 32, Children: 8, ActiveMillis: 3600000, EgressBytes: 134217728, ProviderInputBytes: 268435456}
	m.BudgetDeadline = m.ExpiresAt
	m.Privacy = "cloud_allowed"
	goal := actionauth.GoalPolicy{ID: m.GoalID, OwnerID: m.OwnerID, Kind: "objective", Status: "active", Revision: 1, Purpose: m.Purpose, ExpiresAt: m.ExpiresAt}
	graw, _ := json.Marshal(goal)
	mraw, _ := json.Marshal(m)
	for _, stmt := range []struct {
		sql  string
		args []any
	}{
		{`INSERT INTO owner_goals(goal_id,owner_id,kind,status,revision,expires_at,goal_data,source_data) VALUES($1,$2,'objective','active',1,$3,$4,'{}')`, []any{m.GoalID, m.OwnerID, m.ExpiresAt, graw}},
		{`INSERT INTO owner_purpose_policies VALUES($1,$2,0)`, []any{m.OwnerID, m.Purpose}},
		{`INSERT INTO action_mandates(mandate_id,owner_id,task_id,revision,mandate_data,expires_at,scope_kind,goal_id,goal_revision,status) VALUES($1,$2,'',1,$3,$4,'owner_goal',$5,1,'active')`, []any{m.ID, m.OwnerID, mraw, m.ExpiresAt, m.GoalID}},
		{`INSERT INTO task_authority_bindings(task_id,owner_id,goal_id,goal_revision,mandate_id,mandate_revision,purpose,initiative_reason,observation_ref) VALUES($1,$2,$3,1,$4,1,$5,'owner-request','fixture')`, []any{p.TaskID, p.OwnerID, m.GoalID, m.ID, m.Purpose}},
	} {
		if _, e := v.Pool.Exec(ctx, stmt.sql, stmt.args...); e != nil {
			t.Fatal("authority fixture", e)
		}
	}
	a := externalauth.Request{RecordID: entry.ID, Purpose: "Verify one external test login", Scope: s}
	args, _ := json.Marshal(a)
	b := actionauth.ActionBinding{ActionID: actionauth.StableActionID(p.RunID, "external-once"), RunID: p.RunID, ToolCallID: "external-once", ToolName: externalauth.RequestTool, ContractRevision: "1", Resource: "tool:" + externalauth.RequestTool, OwnerID: p.OwnerID, TaskID: p.TaskID, TaskRevision: p.TaskRevision, ConstraintsRevision: p.ConstraintsRevision, GoalID: m.GoalID, GoalRevision: 1, MandateID: m.ID, MandateRevision: 1, Purpose: m.Purpose, Effect: actionauth.EffectPrepare, ExternalAuth: &s}
	b.ArgsHash, _ = actionauth.ArgumentsHash(string(args))
	b.ContractHash, _ = actionauth.ContractHash(credentialcontrol.Contract(b.ToolName))
	br, _ := json.Marshal(b)
	effectID := "effect_" + strings.Repeat("f", 64)
	if _, e := v.Pool.Exec(ctx, `INSERT INTO action_effects(action_id,fingerprint,initial_binding,policy,state,authorization_id,dispatch_outcome) VALUES($1,'fixture',$2,'{}','executing','grant','executing');`, effectID, br); e != nil {
		t.Fatal("effect fixture", e)
	}
	if _, e := v.Pool.Exec(ctx, `INSERT INTO action_intents(action_id,run_id,tool_call_id,action_hash,binding,requires_approval,expires_at,effect_id,effect_fingerprint,effect_policy) VALUES($1,$2,$3,'fixture',$4,false,clock_timestamp()+interval '1 hour',$5,'fixture','{}')`, b.ActionID, p.RunID, b.ToolCallID, br, effectID); e != nil {
		t.Fatal("source fixture", e)
	}
	if _, e := v.Pool.Exec(ctx, `INSERT INTO action_effect_dispatches(authorization_id,action_id,execution_action_id,outcome) VALUES('grant',$1,$2,'executing')`, effectID, b.ActionID); e != nil {
		t.Fatal("dispatch fixture", e)
	}
	return v, p, entry, s, m, string(args)
}

func createExternalFixtureRequest(t *testing.T, v *Vault, p Principal, args string) AccessRequest {
	t.Helper()
	q, e := v.CreateExternalRequest(t.Context(), p, "external-once", actionauth.DispatchIdentity{ActionID: "effect_" + strings.Repeat("f", 64), Fingerprint: "fixture"}, args)
	if e != nil {
		t.Fatal("external request creation", e)
	}
	if _, e := v.Pool.Exec(t.Context(), `WITH e AS (UPDATE action_effects SET state='effect_confirmed',dispatch_outcome='dispatch_completed' RETURNING action_id), d AS (UPDATE action_effect_dispatches SET outcome='dispatch_completed',finished_at=clock_timestamp() RETURNING action_id) UPDATE accepted_inputs SET status='waiting_for_user',checkpoint='awaiting_interaction',checkpoint_json=jsonb_build_object('interaction',jsonb_build_object('interaction_id',$1::text))`, q.ID); e != nil {
		t.Fatal("canonical wait fixture", e)
	}
	return q
}

func TestVaultExternalAuthOneConsentAndOneBrowserLaunch(t *testing.T) {
	v, p, entry, _, _, args := externalFixture(t)
	q := createExternalFixtureRequest(t, v, p, args)
	if q.Kind != "external_login" || len(q.Fields) != 0 || q.ExternalScope == nil {
		t.Fatal("unsafe request projection")
	}
	if _, e := v.GetByAlias(t.Context(), q.ID); e == nil {
		t.Fatal("external request became a credential alias")
	}
	var wake atomic.Int32
	v.AfterResponse = func(context.Context, pgx.Tx, string, string) error { wake.Add(1); return nil }
	launch, e := v.RespondExternal(t.Context(), p.OwnerID, q.ID, entry.ID, true, externalauth.Consent, q.ScopeDigest)
	if e != nil {
		t.Fatal("protected external decision", e)
	}
	if wake.Load() != 0 {
		t.Fatal("consent woke the Input before external authentication")
	}
	u, e := url.Parse(launch.URL)
	if e != nil || u.Path != "/auth/google/start" || len(u.Query()) != 1 {
		t.Fatal("private launch shape")
	}
	ticket := u.Query().Get("ticket")
	var count atomic.Int32
	var wg sync.WaitGroup
	for range 6 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			start, e := v.StartExternal(t.Context(), ticket)
			if e == nil {
				if start.CookieValue == "" || !strings.HasPrefix(start.CookieName, "__Host-") {
					t.Error("browser binding absent")
				}
				count.Add(1)
			}
		}()
	}
	wg.Wait()
	if count.Load() != 1 {
		t.Fatal("launch ticket replayed")
	}
	for table, want := range map[string]int{"browser_external_auth_uses": 1, "browser_external_auth_launches": 1, "browser_external_auth_exchanges": 0, "browser_external_auth_results": 0} {
		var n int
		if v.Pool.QueryRow(t.Context(), `SELECT count(*) FROM `+table).Scan(&n) != nil || n != want {
			t.Fatal("wrong retained phase", table)
		}
	}
	for _, sql := range []string{`UPDATE browser_external_auth_requests SET expires_at=expires_at+interval '1 minute'`, `DELETE FROM browser_external_auth_requests`, `UPDATE browser_external_auth_uses SET private_cipher='replacement'`, `DELETE FROM browser_external_auth_launches`, `TRUNCATE browser_external_auth_uses CASCADE`} {
		if _, e := v.Pool.Exec(t.Context(), sql); e == nil {
			t.Fatal("retained authority could be rewritten")
		}
	}
}

func TestVaultExternalAuthDoesNotAskOwnerForMissingCommitAuthority(t *testing.T) {
	v, p, _, _, _, args := externalFixture(t)
	if _, e := v.Pool.Exec(t.Context(), `UPDATE action_mandates SET mandate_data=jsonb_set(mandate_data,'{effects}','["prepare"]')`); e != nil {
		t.Fatal(e)
	}
	if _, e := v.CreateExternalRequest(t.Context(), p, "external-once", actionauth.DispatchIdentity{ActionID: "effect_" + strings.Repeat("f", 64), Fingerprint: "fixture"}, args); e == nil {
		t.Fatal("unusable owner request created")
	}
	var n int
	if v.Pool.QueryRow(t.Context(), `SELECT count(*) FROM browser_external_auth_requests`).Scan(&n) != nil || n != 0 {
		t.Fatal("failed authority created a consent")
	}
}

func TestVaultExternalAuthDispatchIdentityCannotBeSubstituted(t *testing.T) {
	v, p, _, _, _, args := externalFixture(t)
	for _, dispatch := range []actionauth.DispatchIdentity{{}, {ActionID: actionauth.StableActionID(p.RunID, "external-once"), Fingerprint: "fixture"}, {ActionID: "effect_" + strings.Repeat("e", 64), Fingerprint: "fixture"}, {ActionID: "effect_" + strings.Repeat("f", 64), Fingerprint: "other"}} {
		if _, e := v.CreateExternalRequest(t.Context(), p, "external-once", dispatch, args); e == nil {
			t.Fatal("foreign or execution-only identity admitted")
		}
	}
	createExternalFixtureRequest(t, v, p, args)
}
