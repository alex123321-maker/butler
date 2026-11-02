package broker

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/butler/butler/internal/actionauth"
	"github.com/butler/butler/internal/browsercontrol"
	"github.com/butler/butler/internal/browserpage"
	"github.com/butler/butler/internal/budget"
	"github.com/butler/butler/internal/credentialcontrol"
	"github.com/butler/butler/internal/credentials"
	"github.com/butler/butler/internal/externalauth"
	commonv1 "github.com/butler/butler/internal/gen/common/v1"
	toolbrokerv1 "github.com/butler/butler/internal/gen/toolbroker/v1"
)

// Real Broker preparation/authorization/dispatch and PostgreSQL journal/vault.
// Only the upstream worker ContextReader is a fixture; there is no provider or
// browser executor call. The permanent effect MUST differ from its intent ID.
func TestExternalCredentialBrokerPermanentEffectAndExecutionIdentity(t *testing.T) {
	dsn := os.Getenv("BUTLER_TEST_VAULT_POSTGRES_URL")
	if dsn == "" {
		t.Skip("requires disposable *_test PostgreSQL")
	}
	repo := filepath.Join("..", "..", "..", "..")
	v := browserVaultFixture(t, dsn, repo)
	ctx := context.Background()
	base := `ALTER TABLE sessions ADD PRIMARY KEY(session_key);ALTER TABLE runs ADD PRIMARY KEY(run_id),ADD COLUMN status TEXT DEFAULT 'tool_pending',ADD COLUMN task_revision BIGINT DEFAULT 1;ALTER TABLE tasks ADD PRIMARY KEY(task_id),ADD COLUMN parent_task_id TEXT,ADD COLUMN session_key TEXT DEFAULT 'web:owner',ADD COLUMN created_at TIMESTAMPTZ DEFAULT clock_timestamp(),ADD COLUMN execution_policy TEXT DEFAULT 'normal';ALTER TABLE server_browser_sessions ADD PRIMARY KEY(browser_session_id),ADD COLUMN task_id TEXT DEFAULT 'task',ADD COLUMN epoch BIGINT DEFAULT 0;
CREATE TABLE session_ownership(session_key TEXT PRIMARY KEY,run_id TEXT,lease_id TEXT,fence_epoch BIGINT,expires_at TIMESTAMPTZ);INSERT INTO session_ownership VALUES('web:owner','broker-run','fixture-lease',1,clock_timestamp()+interval '1 hour');
CREATE TABLE accepted_inputs(input_id TEXT PRIMARY KEY,run_id TEXT,session_key TEXT,owner_id TEXT,task_id TEXT,status TEXT,checkpoint TEXT,checkpoint_json JSONB,expires_at TIMESTAMPTZ,claim_id TEXT,claim_expires_at TIMESTAMPTZ);INSERT INTO accepted_inputs VALUES('input','broker-run','web:owner','owner','task','running','tool_dispatching','{}',clock_timestamp()+interval '1 hour','claim',clock_timestamp()+interval '1 hour');`
	if _, e := v.Pool.Exec(ctx, base); e != nil {
		t.Fatal("source fixture", e)
	}
	// The older generic browser fixture pre-seeds page_mode without migration
	// 108. This empty schema must instead apply the actual ordered contract.
	if _, e := v.Pool.Exec(ctx, `ALTER TABLE server_browser_sessions DROP COLUMN page_mode`); e != nil {
		t.Fatal(e)
	}
	for _, name := range []string{"031_action_authorization", "035_external_effect_journal", "037_execution_budgets", "038_owner_goals_mandates", "038_owner_purpose_policies", "102_stopped_input_finalization", "107_browser_pages", "108_browser_page_execution", "109_browser_sso_consent", "110_browser_sso_use", "111_browser_external_auth"} {
		raw, e := os.ReadFile(filepath.Join(repo, "migrations", name+".up.sql"))
		if e != nil {
			t.Fatal(e)
		}
		if _, e = v.Pool.Exec(ctx, string(raw)); e != nil {
			t.Fatal(name, e)
		}
	}
	scope := externalauth.Scope{Browser: strings.Repeat("a", 32), RPPage: browserpage.Ref{ID: strings.Repeat("b", 32), DocumentID: strings.Repeat("c", 32), Version: 1}, RPURL: "https://rp.example/", Provider: "google", VerifyURL: "https://rp.example/auth/google/account"}
	if _, e := v.Pool.Exec(ctx, `INSERT INTO server_browser_sessions(browser_session_id,run_id,owner_id,task_id,mode,epoch,expires_at,page_mode,page_origin) VALUES($1,'broker-run','owner','task','agent',0,clock_timestamp()+interval '10 minutes',true,'https://rp.example')`, scope.Browser); e != nil {
		t.Fatal(e)
	}
	if _, e := v.Pool.Exec(ctx, `INSERT INTO server_browser_pages(browser_session_id,page_id,document_id,observation_version,observed_epoch,observed_url,state,runtime_version) VALUES($1,$2,$3,1,0,'https://rp.example/','ready',1)`, scope.Browser, scope.RPPage.ID, scope.RPPage.DocumentID); e != nil {
		t.Fatal(e)
	}
	controller := budget.NewController(v.Pool, budget.DefaultPolicy())
	controller.SetAuthorityResolver(actionauth.ResolveBudgetAuthority)
	if v.ConfigureExternalAuth(ctx, externalauth.GoogleConfig{ClientID: "123-test.apps.googleusercontent.com", ClientSecret: "synthetic-client-canary", RedirectURL: "https://rp.example/auth/google/callback"}, scope.RPURL, scope.VerifyURL, controller) != nil {
		t.Fatal("private Core profile")
	}
	entry, e := v.Save(ctx, "owner", credentials.EntryInput{Alias: "google.fixture", Title: "Dedicated Google", Username: "synthetic@gmail.com", Password: "synthetic-private-password-canary"})
	if e != nil {
		t.Fatal(e)
	}
	// Broker gets the vault key/public PG profile, never the private provider.
	brokerVault, e := credentials.NewVault(v.Pool, strings.Repeat("synthetic-vault-key-", 3))
	if e != nil {
		t.Fatal(e)
	}
	m := actionauth.Mandate{ID: "mandate", OwnerID: "owner", ScopeKind: "owner_goal", GoalID: "goal", GoalRevision: 1, Revision: 1, Status: "active", Purpose: "external_broker_test", Tools: []string{externalauth.RequestTool}, Effects: []actionauth.Effect{actionauth.EffectPrepare, actionauth.EffectCommit}, ExpiresAt: time.Now().Add(time.Hour), ResourceScopes: []actionauth.ResourceScope{{Origin: "https://rp.example", PathPrefix: "/"}, {Origin: "https://accounts.google.com", PathPrefix: "/o/oauth2/v2/auth"}, {Origin: "https://oauth2.googleapis.com", PathPrefix: "/token"}, {Origin: "https://www.googleapis.com", PathPrefix: "/oauth2/v3/certs"}}}
	m.Budget = actionauth.MandateBudget{ModelTurns: 32, Tokens: 67108864, ExternalCalls: 32, Children: 1, ActiveMillis: 3600000, EgressBytes: 134217728, ProviderInputBytes: 268435456}
	m.BudgetDeadline = m.ExpiresAt
	m.Privacy = "cloud_allowed"
	g := actionauth.GoalPolicy{ID: m.GoalID, OwnerID: m.OwnerID, Kind: "objective", Status: "active", Revision: 1, Purpose: m.Purpose, ExpiresAt: m.ExpiresAt}
	graw, _ := json.Marshal(g)
	mraw, _ := json.Marshal(m)
	if _, e = v.Pool.Exec(ctx, `INSERT INTO owner_goals(goal_id,owner_id,kind,status,revision,expires_at,goal_data,source_data) VALUES('goal','owner','objective','active',1,$1,$2,'{}')`, g.ExpiresAt, graw); e != nil {
		t.Fatal(e)
	}
	if _, e = v.Pool.Exec(ctx, `INSERT INTO action_mandates(mandate_id,owner_id,task_id,revision,mandate_data,expires_at,scope_kind,goal_id,goal_revision,status) VALUES('mandate','owner','',1,$1,$2,'owner_goal','goal',1,'active')`, mraw, m.ExpiresAt); e != nil {
		t.Fatal(e)
	}
	if _, e = v.Pool.Exec(ctx, `INSERT INTO owner_purpose_policies VALUES('owner','external_broker_test',0);INSERT INTO task_authority_bindings(task_id,owner_id,goal_id,goal_revision,mandate_id,mandate_revision,purpose,initiative_reason,observation_ref) VALUES('task','owner','goal',1,'mandate',1,'external_broker_test','owner-request','fixture')`); e != nil {
		t.Fatal(e)
	}
	reader := newAuthFixture()
	reader.ownerMandate = &m
	reader.inputClaimHash = actionauth.InputClaimHash("input", "claim")
	reg := loadTestRegistry(t)
	if reg.RegisterOwned(credentialcontrol.Contracts(true)) != nil {
		t.Fatal("registry")
	}
	creds := credentials.NewToolCallBroker(credentials.NewBroker(brokerVault), brokerVault)
	server := NewServer(reg, nil, creds, nil)
	journal := actionauth.NewPostgresStore(v.Pool)
	journal.SetBudgets(controller)
	authorizer := actionauth.NewService(journal, reader, creds)
	control := &browsercontrol.Store{Pool: v.Pool, Contexts: reader}
	authorizer.SetBrowserScopes(control)
	server.SetActionAuthorizer(authorizer)
	server.SetBudgets(controller)
	server.SetCatalogExecutor(&CredentialExecutor{Vault: brokerVault, Contexts: reader})
	args, _ := json.Marshal(externalauth.Request{RecordID: entry.ID, Purpose: "One external Google login", Scope: scope})
	call := &toolbrokerv1.ToolCall{RunId: "broker-run", ToolCallId: "external-broker", ToolName: externalauth.RequestTool, ArgsJson: string(args), AutonomyMode: commonv1.AutonomyMode_AUTONOMY_MODE_2}
	prepared, e := server.PrepareToolCall(ctx, &toolbrokerv1.PrepareToolCallRequest{ToolCall: call, LogicalActionRef: "native-external-slot-1"})
	if e != nil || prepared.GetError() != nil {
		t.Fatal("prepare", e, prepared.GetError())
	}
	intent, e := journal.GetIntent(ctx, prepared.Action.ActionId)
	if e != nil || intent.RequiresApproval || intent.EffectID == intent.Binding.ActionID {
		t.Fatal("real journal identity/approval mismatch", e, "requires_approval", intent.RequiresApproval, "distinct_ids", intent.EffectID != intent.Binding.ActionID)
	}
	grant, e := server.AuthorizeToolCall(ctx, &toolbrokerv1.AuthorizeToolCallRequest{ToolCall: call, ActionId: intent.Binding.ActionID})
	if e != nil || grant.GetError() != nil {
		t.Fatal("authorize", e, grant.GetError())
	}
	reply, e := server.ExecuteToolCall(ctx, &toolbrokerv1.ExecuteToolCallRequest{ToolCall: call, AuthorizationId: grant.AuthorizationId})
	if e != nil || reply.GetResult().GetStatus() != "completed" {
		t.Fatal("actual Broker identity did not reach vault", e, reply.GetResult().GetStatus())
	}
	var body struct {
		Request credentials.AccessRequest `json:"request"`
	}
	if json.Unmarshal([]byte(reply.Result.ResultJson), &body) != nil || body.Request.Kind != "external_login" || body.Request.State != "pending" {
		t.Fatal("request metadata")
	}
	var sourceAction, effectState string
	var uses int
	if v.Pool.QueryRow(ctx, `SELECT q.source_action_id,e.state,(SELECT count(*) FROM browser_external_auth_uses) FROM browser_external_auth_requests q JOIN action_intents a ON a.action_id=q.source_action_id JOIN action_effects e ON e.action_id=a.effect_id WHERE q.id=$1`, body.Request.ID).Scan(&sourceAction, &effectState, &uses) != nil || sourceAction != intent.Binding.ActionID || effectState != "effect_confirmed" || uses != 0 {
		t.Fatal("metadata receipt claimed consent or lost its source")
	}
	if strings.Contains(reply.Result.ResultJson, "synthetic-private-password-canary") || strings.Contains(reply.Result.ResultJson, "synthetic@gmail.com") {
		t.Fatal("private account fields escaped Broker")
	}
	if _, e = v.RespondExternal(ctx, "owner", body.Request.ID, entry.ID, true, externalauth.Consent, body.Request.ScopeDigest); e == nil {
		t.Fatal("owner approval preceded the canonical Core wait")
	}
}
