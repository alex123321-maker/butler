package browsercontrol

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"github.com/butler/butler/internal/actionauth"
	"github.com/butler/butler/internal/browserpage"
	"github.com/butler/butler/internal/browsersso"
	toolbrokerv1 "github.com/butler/butler/internal/gen/toolbroker/v1"
	"google.golang.org/grpc/metadata"
	"google.golang.org/protobuf/proto"
)

const ssoDispatchHeader = "butler-browser-sso"

func IncomingSSODispatch(ctx context.Context, s browsersso.Scope) (browsersso.Dispatch, bool) {
	var d browsersso.Dispatch
	md, _ := metadata.FromIncomingContext(ctx)
	values := md.Get(ssoDispatchHeader)
	if len(values) != 1 || len(values[0]) > 32768 || json.Unmarshal([]byte(values[0]), &d) != nil || !d.Valid(s) {
		return d, false
	}
	return d, true
}

func (s *Store) beforeSSO(ctx context.Context, call *toolbrokerv1.ToolCall, epoch int64) (context.Context, error) {
	a, e := browsersso.Parse(call.ArgsJson)
	if e != nil || s.Contexts == nil {
		return ctx, ErrDenied
	}
	c, e := s.Contexts.GetActionContext(ctx, call.RunId)
	if e != nil || c.Delegation != nil {
		return ctx, ErrDenied
	}
	i, e := actionauth.NewPostgresStore(s.Pool).GetIntent(ctx, actionauth.StableActionID(call.RunId, call.ToolCallId))
	argsHash, hashErr := actionauth.ArgumentsHash(call.ArgsJson)
	b := i.Binding
	if e != nil || hashErr != nil || b.ArgsHash != argsHash || b.RunID != call.RunId || b.ToolCallID != call.ToolCallId || b.ToolName != browsersso.Tool || b.OwnerID != c.OwnerID || b.TaskID != c.TaskID || len(b.CredentialMetadataHash) != 64 || b.BrowserSSO == nil || *b.BrowserSSO != a.Scope {
		return ctx, ErrDenied
	}
	lookup := b
	lookup.ToolName = browserpage.Snapshot
	lookup.Resource = "browser:" + a.Browser + ":"
	bound, e := s.BrowserPageScope(ctx, lookup, browserpage.Args{SessionID: a.Browser, Page: &a.RPPage})
	if e != nil || bound.Epoch != epoch || bound.URL != a.RPURL {
		return ctx, ErrDenied
	}
	b.BrowserPage = bound
	tx, e := s.Pool.Begin(ctx)
	if e != nil {
		return ctx, ErrDenied
	}
	defer tx.Rollback(ctx)
	if actionauth.CheckTaskActionTx(ctx, tx, b, time.Now()) != nil || actionauth.CheckDenyOverrides(b, c.DenyTools, c.DenyResources) != nil {
		return ctx, ErrDenied
	}
	var current bool
	if tx.QueryRow(ctx, `SELECT TRUE FROM server_browser_sessions s JOIN server_browser_pages p USING(browser_session_id) WHERE s.browser_session_id=$1 AND s.run_id=$2 AND s.owner_id=$3 AND s.task_id=$4 AND s.mode='agent' AND s.page_mode AND s.epoch=$5 AND s.expires_at>clock_timestamp() AND p.page_id=$6 AND p.document_id=$7 AND p.observation_version=$8 AND p.observed_epoch=s.epoch AND p.state='ready' AND p.observed_url=$9 FOR SHARE OF s,p`, a.Browser, call.RunId, c.OwnerID, c.TaskID, epoch, a.RPPage.ID, a.RPPage.DocumentID, a.RPPage.Version, a.RPURL).Scan(&current) != nil || !current {
		return ctx, ErrDenied
	}
	d := browsersso.Dispatch{Page: *bound, Digest: a.Digest}
	if tx.QueryRow(ctx, `SELECT q.expires_at FROM credential_sso_requests q WHERE q.id=$1 AND q.run_id=$2 AND q.scope_digest=$4 AND q.context_epoch=$5 AND q.state='approved' AND q.expires_at>clock_timestamp() AND NOT EXISTS(SELECT 1 FROM credential_sso_uses u WHERE u.request_id=q.id AND (u.run_id<>q.run_id OR u.tool_call_id<>$3)) FOR SHARE OF q`, a.UsernameRef.Alias, call.RunId, call.ToolCallId, a.Digest, epoch).Scan(&d.ExpiresAt) != nil {
		return ctx, ErrDenied
	}
	if c.Authority != nil {
		m, _, e := actionauth.CheckAssignmentTx(ctx, tx, *c.Authority, time.Now())
		if e != nil {
			return ctx, ErrDenied
		}
		for _, scope := range m.ResourceScopes {
			if strings.EqualFold(scope.Origin, browsersso.Origin(a.RPURL)) || strings.EqualFold(scope.Origin, browsersso.Origin(a.IdPURL)) {
				d.Allowed = append(d.Allowed, browsersso.NetworkScope{Origin: scope.Origin, PathPrefix: scope.PathPrefix})
			}
		}
	} else {
		d.Allowed = []browsersso.NetworkScope{{Origin: browsersso.Origin(a.RPURL), PathPrefix: "/"}, {Origin: browsersso.Origin(a.IdPURL), PathPrefix: "/"}}
	}
	for _, scope := range c.DenyResources {
		if strings.EqualFold(scope.Origin, browsersso.Origin(a.RPURL)) || strings.EqualFold(scope.Origin, browsersso.Origin(a.IdPURL)) {
			d.Denied = append(d.Denied, browsersso.NetworkScope{Origin: scope.Origin, PathPrefix: scope.PathPrefix})
		}
	}
	raw, marshalErr := json.Marshal(d)
	if marshalErr != nil || len(raw) > 32768 || !d.Valid(a.Scope) || tx.Commit(ctx) != nil {
		return ctx, ErrDenied
	}
	md, _ := metadata.FromOutgoingContext(ctx)
	md = md.Copy()
	md.Set(ssoDispatchHeader, string(raw))
	ctx = metadata.NewOutgoingContext(ctx, md)
	return WithPageDispatch(WithDispatchEpoch(ctx, epoch), *bound), nil
}

// Reuse the managed ledger's final authority and retention checks without
// pretending that the original action was a generic approved page click.
func (s *Store) recordSSO(ctx context.Context, call *toolbrokerv1.ToolCall, b actionauth.ActionBinding, result *toolbrokerv1.ToolResult) error {
	a, e := browsersso.Parse(call.ArgsJson)
	if e != nil || b.BrowserSSO == nil || *b.BrowserSSO != a.Scope {
		result.ResultJson = "{}"
		return ErrDenied
	}
	shadow := proto.Clone(call).(*toolbrokerv1.ToolCall)
	shadow.ToolName = browserpage.Click
	raw, _ := json.Marshal(browserpage.Args{SessionID: a.Browser, Page: &a.RPPage, Selector: a.Trigger})
	shadow.ArgsJson = string(raw)
	return s.recordPageExecution(ctx, shadow, b, result)
}
