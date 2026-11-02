package credentials

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"testing"

	"github.com/butler/butler/internal/browserauth"
	"github.com/butler/butler/internal/browsersso"
	toolbrokerv1 "github.com/butler/butler/internal/gen/toolbroker/v1"
)

func ssoUseCall(run, id string, r SSORequest) *toolbrokerv1.ToolCall {
	a := browsersso.Args{Scope: r.Scope, Digest: r.ScopeDigest, UsernameRef: browserauth.Ref{Type: "credential_ref", Alias: r.ID, Field: "username"}, PasswordRef: browserauth.Ref{Type: "credential_ref", Alias: r.ID, Field: "password"}}
	raw, _ := json.Marshal(a)
	return &toolbrokerv1.ToolCall{RunId: run, ToolCallId: id, ToolName: browsersso.Tool, ArgsJson: string(raw), CredentialRefs: []*toolbrokerv1.CredentialRef{{Type: "credential_ref", Alias: r.ID, Field: "username"}, {Type: "credential_ref", Alias: r.ID, Field: "password"}}}
}

func TestVaultSSOUseOneCallIntegration(t *testing.T) {
	ctx := context.Background()
	v, p, entry, scope := ssoFixture(t)
	r, e := v.CreateSSORequest(ctx, p, "sso-use", entry.ID, "One consented login", scope)
	if e != nil {
		t.Fatal(e)
	}
	call := ssoUseCall(p.RunID, "first-use", r)
	if v.CheckGrantedCall(ctx, call) {
		t.Fatal("pending decision authorized use")
	}
	if v.RespondSSO(ctx, p.OwnerID, r.ID, entry.ID, true, browsersso.Consent, r.ScopeDigest) != nil {
		t.Fatal("approve")
	}
	metadata, e := v.GetByAlias(ctx, r.ID)
	if e != nil {
		t.Fatal(e)
	}
	var wg sync.WaitGroup
	type outcome struct {
		id string
		ok bool
	}
	answers := make(chan outcome, 2)
	for _, id := range []string{"first-use", "second-use"} {
		wg.Add(1)
		go func(id string) {
			defer wg.Done()
			c := ssoUseCall(p.RunID, id, r)
			_, e := v.ResolveField(ctx, c, metadata, "username")
			answers <- outcome{id, e == nil}
		}(id)
	}
	wg.Wait()
	close(answers)
	winner := ""
	for a := range answers {
		if a.ok {
			if winner != "" {
				t.Fatal("two calls spent consent")
			}
			winner = a.id
		}
	}
	if winner == "" {
		t.Fatal("no winning use")
	}
	selected := ssoUseCall(p.RunID, winner, r)
	if !v.CheckGrantedCall(ctx, selected) {
		t.Fatal("same-call second field denied")
	}
	value, e := v.ResolveField(ctx, selected, metadata, "password")
	if e != nil || value != "synthetic-private-password" {
		t.Fatal("private second field unavailable")
	}
	for _, mutate := range []func(*toolbrokerv1.ToolCall){func(c *toolbrokerv1.ToolCall) { c.RunId = "foreign" }, func(c *toolbrokerv1.ToolCall) { c.ToolCallId = "another" }, func(c *toolbrokerv1.ToolCall) { c.ToolName = "browser.login" }, func(c *toolbrokerv1.ToolCall) { c.ToolName = "browser.prepare_fill" }, func(c *toolbrokerv1.ToolCall) {
		c.ArgsJson = strings.Replace(c.ArgsJson, r.ScopeDigest, strings.Repeat("0", 64), 1)
	}, func(c *toolbrokerv1.ToolCall) {
		c.ArgsJson = strings.Replace(c.ArgsJson, "https://idp.example/login", "https://idp.example/other", 1)
	}} {
		c := ssoUseCall(p.RunID, winner, r)
		mutate(c)
		if v.CheckGrantedCall(ctx, c) {
			t.Fatal("foreign/rebound use accepted")
		}
		if _, e = v.ResolveField(ctx, c, metadata, "password"); e == nil {
			t.Fatal("foreign/rebound field released")
		}
	}
	status, e := v.RequestStatus(ctx, p, r.ID)
	if e != nil || status.State != "consumed" {
		t.Fatal("consumed state unavailable")
	}
	if _, e = v.Pool.Exec(ctx, `UPDATE server_browser_sessions SET epoch=epoch+1`); e != nil {
		t.Fatal(e)
	}
	if v.CheckGrantedCall(ctx, selected) {
		t.Fatal("old runtime generation reused")
	}
	if _, e = v.ResolveField(ctx, selected, metadata, "password"); e == nil {
		t.Fatal("stale generation released")
	}
	for _, sql := range []string{`DELETE FROM credential_sso_uses`, `TRUNCATE credential_sso_uses`, `UPDATE credential_sso_uses SET tool_call_id='replacement'`} {
		if _, e = v.Pool.Exec(ctx, sql); e == nil {
			t.Fatal("use history mutated")
		}
	}
}
