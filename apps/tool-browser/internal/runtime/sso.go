package runtime

import (
	"context"
	"time"

	"github.com/butler/butler/internal/browserauth"
	"github.com/butler/butler/internal/browsercontrol"
	"github.com/butler/butler/internal/browserpage"
	"github.com/butler/butler/internal/browsersso"
	commonv1 "github.com/butler/butler/internal/gen/common/v1"
	runtimev1 "github.com/butler/butler/internal/gen/runtime/v1"
	toolbrokerv1 "github.com/butler/butler/internal/gen/toolbroker/v1"
	"github.com/butler/butler/internal/security/egress"
	"google.golang.org/protobuf/proto"
)

// Private runner data only. Public arguments contain deferred references.
type ssoRequest struct {
	Scope    browsersso.Scope    `json:"scope"`
	Dispatch browsersso.Dispatch `json:"dispatch"`
	Username string              `json:"username"`
	Password string              `json:"password"`
}

func (s *Server) executeSSO(ctx context.Context, req *runtimev1.ExecuteRequest) (*runtimev1.ExecuteResponse, error) {
	fail := func() (*runtimev1.ExecuteResponse, error) {
		return &runtimev1.ExecuteResponse{Result: failedResult(req, commonv1.ErrorClass_ERROR_CLASS_POLICY_DENIED, "SSO unavailable or unverified; never repeat the POST automatically", `{}`)}, nil
	}
	call := req.GetToolCall()
	a, e := browsersso.Parse(call.GetArgsJson())
	if e != nil || !proto.Equal(req.Contract, browsersso.Contract()) || req.GetContext().GetRunId() != call.RunId || call.RunId == "" || len(call.CredentialRefs) != 2 || len(req.ResolvedCredentials) != 2 {
		return fail()
	}
	fields := map[string]string{}
	refs := map[string]bool{}
	for _, ref := range call.CredentialRefs {
		if ref == nil || ref.Type != "credential_ref" || ref.Alias != a.UsernameRef.Alias || refs[ref.Field] || ref.Field != "username" && ref.Field != "password" {
			return fail()
		}
		refs[ref.Field] = true
	}
	for _, value := range req.ResolvedCredentials {
		if value == nil || value.Alias != a.UsernameRef.Alias || value.Field != "username" && value.Field != "password" || fields[value.Field] != "" || value.Value == "" || len(value.Value) > 4096 {
			return fail()
		}
		fields[value.Field] = value.Value
	}
	d, ok := browsercontrol.IncomingSSODispatch(ctx, a.Scope)
	if !ok || d.Digest != a.Digest || !d.ExpiresAt.After(time.Now()) {
		return fail()
	}
	for _, raw := range a.Targets() {
		if _, e := (egress.Policy{}).ValidateURL(raw); e != nil {
			return fail()
		}
	}
	session, code := s.sessions.acquire(a.Browser, call.RunId)
	if code != "" {
		return fail()
	}
	defer session.mu.Unlock()
	epoch, ok := browsercontrol.IncomingDispatchEpoch(ctx)
	page, known := session.pages[a.RPPage.ID]
	if !ok || !known || epoch != session.controlEpoch || epoch != d.Page.Epoch || !session.managedPages || session.closed || session.controller != "" || page.State != "ready" || page.DocumentID != a.RPPage.DocumentID || page.RuntimeVersion != d.Page.RuntimeVersion || page.URL != a.RPURL || session.origin != browsersso.Origin(a.RPURL) || !session.expires.After(time.Now()) {
		return fail()
	}
	deadline := time.Now().Add(35 * time.Second)
	if d.ExpiresAt.Before(deadline) {
		deadline = d.ExpiresAt
	}
	if session.expires.Before(deadline) {
		deadline = session.expires
	}
	runCtx, cancel := context.WithDeadline(ctx, deadline)
	defer cancel()
	r := Request{ToolName: browsersso.Tool, SessionID: session.id, ManagedPages: true, PageID: page.ID, DocumentID: page.DocumentID, ObservationVersion: uint64(page.RuntimeVersion), ExpectedURL: page.URL, ScopeOrigin: session.origin, ExpiresAt: session.expires.UTC().Format(time.RFC3339Nano), AllowedDomains: session.allowedDomains, SSO: &ssoRequest{Scope: a.Scope, Dispatch: d, Username: fields["username"], Password: fields["password"]}, SecretValues: []string{fields["username"], fields["password"]}}
	result, e := s.runner.Run(runCtx, r)
	if e != nil {
		s.sessions.invalidateAll()
		return fail()
	}
	if !result.OK || result.Error != nil || result.NetworkState != "frozen" || result.SelectedPageID != page.ID || result.FinalURL != a.VerifyURL || len(result.PrivatePages) == 0 || len(result.PrivatePages) > browsercontrol.MaxLifetimePages {
		return fail()
	}
	pages := map[string]browserpage.Report{}
	for _, p := range result.PrivatePages {
		if !browserpage.ID(p.ID) || pages[p.ID].ID != "" || p.OpenerID != "" && !browserpage.ID(p.OpenerID) || p.State != "ready" && p.State != "pending" && p.State != "closed" && p.State != "lost" {
			return fail()
		}
		if p.State == "ready" && (!browserpage.ID(p.DocumentID) || p.RuntimeVersion <= 0 || originOf(p.URL) != session.origin) {
			return fail()
		}
		pages[p.ID] = p
	}
	if pages[page.ID].State != "ready" {
		return fail()
	}
	session.pages = pages
	result.SessionID, result.ScopeOrigin, result.ExpiresAt = session.id, session.origin, session.expires.UTC().Format(time.RFC3339Nano)
	result.Text = truncate(result.Text, maxSnapshotTextLen)
	return &runtimev1.ExecuteResponse{Result: &toolbrokerv1.ToolResult{RunId: call.RunId, ToolCallId: call.ToolCallId, ToolName: call.ToolName, Status: "completed", ResultJson: buildResultJSON(call.ToolName, result), FinishedAt: time.Now().UTC().Format(time.RFC3339Nano)}}, nil
}

func (r *PlaywrightRunner) sso(ctx context.Context, req Request) (Result, error) {
	proxy := r.proxies[req.SessionID]
	if proxy == nil || req.SSO == nil || !req.SSO.Dispatch.Valid(req.SSO.Scope) {
		return Result{}, egress.ErrDenied
	}
	for _, target := range req.SSO.Scope.Targets() {
		if _, e := r.policy.ValidateURL(target); e != nil {
			return Result{}, e
		}
	}
	proxy.Freeze()
	defer proxy.Freeze()
	deadline := time.Now().Add(30 * time.Second)
	if d, ok := ctx.Deadline(); ok && d.Before(deadline) {
		deadline = d
	}
	if e := proxy.AllowUntil(deadline); e != nil {
		return Result{}, e
	}
	req.SSOPhase = "plan"
	plan, e := r.exchange(ctx, req)
	proxy.Freeze()
	if e != nil || !plan.OK || len(plan.LoginNonce) != 36 {
		return plan, e
	}
	if e = proxy.AllowUntil(deadline); e != nil {
		return Result{}, e
	}
	req.SSOPhase, req.LoginNonce = "submit", plan.LoginNonce
	result, e := r.exchange(ctx, req)
	proxy.Freeze()
	result.Authentication = nil
	if e != nil || !result.OK || result.LoginPosts != 1 || result.NetworkState != "frozen" || result.FinalURL != req.SSO.Scope.VerifyURL {
		return result, e
	}
	// Probe only RP cookies and the exact protected RP endpoint. An IdP page or
	// popup closure cannot produce this proof, nor can a helper-supplied flag.
	scope := browserauth.Scope{Browser: req.SessionID, ExpectedURL: req.SSO.Scope.RPURL, TargetURL: req.SSO.Scope.VerifyURL}
	if verifyLoginAccess(ctx, r.policy, scope, req.SSO.Scope.VerifyURL, result.LoginCookies) {
		result.Authentication = &browserauth.Proof{Protocol: browsersso.Verification, SubmittedOnce: true, AuthenticatedRead: true, AnonymousChallenge: true}
	}
	return result, nil
}
