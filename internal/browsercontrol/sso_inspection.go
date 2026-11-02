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

const inspectionHeader = "butler-browser-sso-inspection"

func IncomingSSOInspection(ctx context.Context, a browsersso.Inspection) (browsersso.Dispatch, bool) {
	var d browsersso.Dispatch
	md, _ := metadata.FromIncomingContext(ctx)
	values := md.Get(inspectionHeader)
	if len(values) != 1 || len(values[0]) > 32768 || json.Unmarshal([]byte(values[0]), &d) != nil || !d.ValidInspection(a) {
		return d, false
	}
	return d, true
}

func (s *Store) beforeSSOInspection(ctx context.Context, call *toolbrokerv1.ToolCall, epoch int64) (context.Context, error) {
	a, err := browsersso.ParseInspection(call.ArgsJson)
	if err != nil || s.Contexts == nil {
		return ctx, ErrDenied
	}
	c, err := s.Contexts.GetActionContext(ctx, call.RunId)
	if err != nil || c.Delegation != nil || c.Authority == nil {
		return ctx, ErrDenied
	}
	intent, err := actionauth.NewPostgresStore(s.Pool).GetIntent(ctx, actionauth.StableActionID(call.RunId, call.ToolCallId))
	b := intent.Binding
	hash, hashErr := actionauth.ArgumentsHash(call.ArgsJson)
	if err != nil || hashErr != nil || b.ArgsHash != hash || b.ToolName != browsersso.InspectTool || b.OwnerID != c.OwnerID || b.TaskID != c.TaskID || b.BrowserSSOInspection == nil || *b.BrowserSSOInspection != a || len(b.CredentialRefs) != 0 {
		return ctx, ErrDenied
	}
	lookup := b
	lookup.ToolName = browserpage.Snapshot
	lookup.Resource = "browser:" + a.Browser + ":"
	page, err := s.BrowserPageScope(ctx, lookup, browserpage.Args{SessionID: a.Browser, Page: &a.RPPage})
	if err != nil || page == nil || page.Epoch != epoch || page.URL != a.RPURL {
		return ctx, ErrDenied
	}
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return ctx, ErrDenied
	}
	defer tx.Rollback(ctx)
	if actionauth.CheckTaskActionTx(ctx, tx, b, time.Now()) != nil || actionauth.CheckDenyOverrides(b, c.DenyTools, c.DenyResources) != nil {
		return ctx, ErrDenied
	}
	m, _, err := actionauth.CheckAssignmentTx(ctx, tx, *c.Authority, time.Now())
	if err != nil {
		return ctx, ErrDenied
	}
	d := browsersso.Dispatch{Page: *page, Digest: a.Digest(), ExpiresAt: time.Now().Add(20 * time.Second)}
	var expiry time.Time
	if tx.QueryRow(ctx, `SELECT expires_at FROM server_browser_sessions WHERE browser_session_id=$1 AND owner_id=$2 AND run_id=$3 AND task_id=$4 AND epoch=$5 AND mode='agent' AND page_mode FOR SHARE`, a.Browser, c.OwnerID, call.RunId, c.TaskID, epoch).Scan(&expiry) != nil {
		return ctx, ErrDenied
	}
	var current bool
	if tx.QueryRow(ctx, `SELECT TRUE FROM server_browser_pages WHERE browser_session_id=$1 AND page_id=$2 AND document_id=$3 AND observation_version=$4 AND observed_epoch=$5 AND observed_url=$6 AND runtime_version=$7 AND state='ready' FOR SHARE`, a.Browser, page.Ref.ID, page.Ref.DocumentID, page.Ref.Version, epoch, a.RPURL, page.RuntimeVersion).Scan(&current) != nil || !current {
		return ctx, ErrDenied
	}
	for _, end := range []time.Time{expiry, c.AuthorityDeadline, c.LeaseExpiresAt, m.ExpiresAt} {
		if !end.IsZero() && end.Before(d.ExpiresAt) {
			d.ExpiresAt = end
		}
	}
	for _, cap := range m.ResourceScopes {
		if strings.EqualFold(cap.Origin, browsersso.Origin(a.RPURL)) || strings.EqualFold(cap.Origin, a.IdPOrigin) {
			d.Allowed = append(d.Allowed, browsersso.NetworkScope{Origin: cap.Origin, PathPrefix: cap.PathPrefix})
		}
	}
	for _, cap := range c.DenyResources {
		if strings.EqualFold(cap.Origin, browsersso.Origin(a.RPURL)) || strings.EqualFold(cap.Origin, a.IdPOrigin) {
			d.Denied = append(d.Denied, browsersso.NetworkScope{Origin: cap.Origin, PathPrefix: cap.PathPrefix})
		}
	}
	raw, err := json.Marshal(d)
	if err != nil || len(raw) > 32768 || !d.ValidInspection(a) || !d.ExpiresAt.After(time.Now()) || tx.Commit(ctx) != nil {
		return ctx, ErrDenied
	}
	md, _ := metadata.FromOutgoingContext(ctx)
	md = md.Copy()
	md.Set(inspectionHeader, string(raw))
	return WithPageDispatch(WithDispatchEpoch(metadata.NewOutgoingContext(ctx, md), epoch), *page), nil
}

func (s *Store) recordSSOInspection(ctx context.Context, call *toolbrokerv1.ToolCall, b actionauth.ActionBinding, result *toolbrokerv1.ToolResult) error {
	deny := func() error { result.ResultJson = "{}"; return ErrDenied }
	a, err := browsersso.ParseInspection(call.ArgsJson)
	var output struct {
		Forms []browsersso.InspectedForm `json:"inspection_forms"`
	}
	if err != nil || b.BrowserSSOInspection == nil || *b.BrowserSSOInspection != a || json.Unmarshal([]byte(result.ResultJson), &output) != nil || len(output.Forms) < 1 || len(output.Forms) > 4 {
		return deny()
	}
	for _, f := range output.Forms {
		if _, err := f.Scope(a); err != nil {
			return deny()
		}
	}
	// Reuse the complete managed-page CAS/authority/release mediation. The
	// inspection's child is closed; ordinary tools still see only RP pages.
	shadow := proto.Clone(call).(*toolbrokerv1.ToolCall)
	shadow.ToolName = browserpage.Snapshot
	args, _ := json.Marshal(browserpage.Args{SessionID: a.Browser, Page: &a.RPPage})
	shadow.ArgsJson = string(args)
	if s.recordPageExecution(ctx, shadow, b, result) != nil {
		return deny()
	}
	var raw map[string]json.RawMessage
	var selected struct {
		Page browserpage.Ref `json:"page"`
	}
	if json.Unmarshal([]byte(result.ResultJson), &raw) != nil || json.Unmarshal([]byte(result.ResultJson), &selected) != nil || !selected.Page.Valid() || selected.Page.ID != a.RPPage.ID {
		return deny()
	}
	c, err := s.Contexts.GetActionContext(ctx, call.RunId)
	if err != nil || c.OwnerID != b.OwnerID || c.TaskID != b.TaskID || c.Authority == nil || c.Delegation != nil {
		return deny()
	}
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return deny()
	}
	defer tx.Rollback(ctx)
	if actionauth.CheckTaskActionTx(ctx, tx, b, time.Now()) != nil || actionauth.CheckDenyOverrides(b, c.DenyTools, c.DenyResources) != nil {
		return deny()
	}
	scopes := make([]browsersso.Scope, 0, len(output.Forms))
	for _, f := range output.Forms {
		scope, _ := f.Scope(a)
		scope.RPPage = selected.Page
		for _, url := range []string{scope.IdPURL, scope.TargetURL} {
			// This additional release check cannot replace the original all-of RP /
			// IdP inspection authority. It only checks each newly observed path.
			check := b
			check.BrowserSSOInspection = nil
			check.Resource = url
			if actionauth.CheckTaskActionTx(ctx, tx, check, time.Now()) != nil || actionauth.CheckDenyOverrides(check, c.DenyTools, c.DenyResources) != nil {
				return deny()
			}
		}
		scopes = append(scopes, scope)
	}
	if tx.Commit(ctx) != nil {
		return deny()
	}
	delete(raw, "inspection_forms")
	raw["sso_scopes"], _ = json.Marshal(scopes)
	encoded, _ := json.Marshal(raw)
	result.ResultJson = string(encoded)
	return nil
}
