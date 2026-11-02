package browsercontrol

import (
	"context"
	"encoding/json"
	"github.com/butler/butler/internal/actionauth"
	"github.com/butler/butler/internal/browserauth"
	"github.com/butler/butler/internal/browsersso"
	toolbrokerv1 "github.com/butler/butler/internal/gen/toolbroker/v1"
	"google.golang.org/grpc/metadata"
	"strings"
	"testing"
	"time"
)

type ssoContext struct{ value actionauth.ContextSnapshot }

func (c ssoContext) GetActionContext(context.Context, string) (actionauth.ContextSnapshot, error) {
	return c.value, nil
}

// The pre-dispatch page fence runs before the authorizer resolves either field.
// It must require the prepared intent and owner decision, not an already-spent
// use claim. Claim/contention and final action dispatch are checked separately.
func TestSSOPreDispatchPageFenceIntegration(t *testing.T) {
	s := pageDB(t)
	if pageTestMigration(t, s.Pool, "108_browser_page_execution.up.sql") != nil {
		t.Fatal("page execution schema")
	}
	p := pageSession(t, s)
	if _, e := s.Pool.Exec(t.Context(), `ALTER TABLE server_browser_sessions ADD COLUMN observed_url TEXT NOT NULL DEFAULT ''`); e != nil {
		t.Fatal(e)
	}
	if _, e := s.Pool.Exec(t.Context(), `UPDATE server_browser_sessions SET task_id='',page_mode=true,page_origin='https://rp.example',observed_url='https://rp.example/' WHERE browser_session_id=$1`, p.SessionID); e != nil {
		t.Fatal(e)
	}
	p.TaskID = ""
	if _, e := s.Pool.Exec(t.Context(), `CREATE TABLE task_authority_bindings(task_id TEXT,owner_id TEXT,goal_id TEXT,goal_revision BIGINT,mandate_id TEXT,mandate_revision BIGINT,purpose TEXT,initiative_reason TEXT,observation_ref TEXT,policy_revision BIGINT)`); e != nil {
		t.Fatal(e)
	}
	root := readyPage(t, s, p, nil, "https://rp.example/")
	if _, e := s.Pool.Exec(t.Context(), `UPDATE server_browser_pages SET runtime_version=3,observation_version=observation_version+1 WHERE browser_session_id=$1`, p.SessionID); e != nil {
		t.Fatal(e)
	}
	root.Version++
	// Narrow fixture for the persisted-intent and immutable consent reads. Full
	// ordered ledger migrations and field-use races run in the vault profile.
	if _, e := s.Pool.Exec(t.Context(), `CREATE TABLE action_intents(action_id TEXT PRIMARY KEY,binding JSONB,requires_approval BOOLEAN,expires_at TIMESTAMPTZ,effect_id TEXT,effect_fingerprint TEXT,effect_policy JSONB);CREATE TABLE credential_sso_requests(id TEXT PRIMARY KEY,run_id TEXT,scope_digest TEXT,context_epoch BIGINT,state TEXT,expires_at TIMESTAMPTZ);CREATE TABLE credential_sso_uses(request_id TEXT PRIMARY KEY,run_id TEXT,tool_call_id TEXT);`); e != nil {
		t.Fatal(e)
	}
	scope := browsersso.Scope{Browser: p.SessionID, RPPage: root.PageRef, RPURL: root.URL, Trigger: "#login", IdPURL: "https://idp.example/authorize", TargetURL: "https://idp.example/login", VerifyURL: "https://rp.example/account", Form: "form", Username: "#u", Password: "#p"}
	hash, _ := scope.Digest(0, 1)
	alias := "ssoq_" + strings.Repeat("a", 64)
	args := browsersso.Args{Scope: scope, Digest: hash, UsernameRef: browserauth.Ref{Type: "credential_ref", Alias: alias, Field: "username"}, PasswordRef: browserauth.Ref{Type: "credential_ref", Alias: alias, Field: "password"}}
	raw, _ := json.Marshal(args)
	call := &toolbrokerv1.ToolCall{RunId: p.RunID, ToolCallId: "sso", ToolName: browsersso.Tool, ArgsJson: string(raw)}
	b := actionauth.ActionBinding{ActionID: actionauth.StableActionID(p.RunID, call.ToolCallId), RunID: p.RunID, ToolCallID: call.ToolCallId, ToolName: browsersso.Tool, OwnerID: p.OwnerID, BrowserSSO: &scope, CredentialMetadataHash: strings.Repeat("b", 64)}
	b.ArgsHash, _ = actionauth.ArgumentsHash(call.ArgsJson)
	encoded, _ := json.Marshal(b)
	if _, e := s.Pool.Exec(t.Context(), `INSERT INTO action_intents VALUES($1,$2,false,now()+interval '1 minute','effect','fingerprint','{}');`, b.ActionID, string(encoded)); e != nil {
		t.Fatal(e)
	}
	if _, e := s.Pool.Exec(t.Context(), `INSERT INTO credential_sso_requests VALUES($1,$2,$3,0,'pending',now()+interval '1 minute')`, alias, p.RunID, hash); e != nil {
		t.Fatal(e)
	}
	c := ssoContext{actionauth.ContextSnapshot{RunID: p.RunID, OwnerID: p.OwnerID}}
	s.Contexts = c
	if _, e := s.Before(t.Context(), call); e == nil {
		t.Fatal("pending owner decision released dispatch")
	}
	if _, e := s.Pool.Exec(t.Context(), `UPDATE credential_sso_requests SET state='approved'`); e != nil {
		t.Fatal(e)
	}
	dispatched, e := s.Before(t.Context(), call)
	if e != nil {
		t.Fatal("approved pre-claim scope refused", e)
	}
	md, _ := metadata.FromOutgoingContext(dispatched)
	d, ok := IncomingSSODispatch(metadata.NewIncomingContext(t.Context(), md), scope)
	if !ok || d.Page.Ref != root.PageRef || d.Page.RuntimeVersion != 3 || d.Page.Epoch != 0 || !d.ExpiresAt.After(time.Now()) || !d.Allows(scope.IdPURL) || !d.Allows(scope.VerifyURL) {
		t.Fatal("incorrect private dispatch")
	}
	if _, e := s.Pool.Exec(t.Context(), `INSERT INTO credential_sso_uses VALUES($1,$2,'foreign')`, alias, p.RunID); e != nil {
		t.Fatal(e)
	}
	if _, e := s.Before(t.Context(), call); e == nil {
		t.Fatal("another call spent owner consent")
	}
	if _, e := s.Pool.Exec(t.Context(), `UPDATE credential_sso_uses SET tool_call_id='sso'`); e != nil {
		t.Fatal(e)
	}
	c.value.DenyResources = []actionauth.ResourceScope{{Origin: "https://idp.example", PathPrefix: "/login"}}
	s.Contexts = c
	if _, e := s.Before(t.Context(), call); e == nil {
		t.Fatal("current IdP path denial ignored")
	}
	c.value.DenyResources = nil
	s.Contexts = c
	if _, e := s.Pool.Exec(t.Context(), `UPDATE server_browser_sessions SET epoch=epoch+1 WHERE browser_session_id=$1`, p.SessionID); e != nil {
		t.Fatal(e)
	}
	if _, e := s.Before(t.Context(), call); e == nil {
		t.Fatal("owner epoch change reused RP consent")
	}
}
