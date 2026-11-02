package actionauth

import (
	"context"
	"encoding/json"

	"github.com/butler/butler/internal/browserpage"
	"github.com/butler/butler/internal/browsersso"
	"github.com/butler/butler/internal/credentialcontrol"
	toolbrokerv1 "github.com/butler/butler/internal/gen/toolbroker/v1"
	"github.com/butler/butler/internal/security/egress"
	"google.golang.org/protobuf/proto"
)

func validSSOScope(b ActionBinding) bool {
	return b.BrowserSSO != nil && b.BrowserSSO.Validate() == nil && (b.ToolName == browsersso.Tool || b.ToolName == browsersso.RequestTool)
}

// The request itself is preparation, but it must not borrow a standard local
// credential-request exemption to propose an unassigned RP or IdP destination.
func (s *Service) bindSSO(ctx context.Context, b ActionBinding, call *toolbrokerv1.ToolCall, contract *toolbrokerv1.ToolContract) (ActionBinding, error) {
	var scope browsersso.Scope
	if call.ToolName == browsersso.Tool {
		a, e := browsersso.Parse(call.ArgsJson)
		if e != nil || !proto.Equal(contract, browsersso.Contract()) {
			return b, browsersso.ErrScope
		}
		scope = a.Scope
	} else {
		a, e := credentialcontrol.DecodeToolArgs(call.ToolName, call.ArgsJson)
		if e != nil || a.SSOScope == nil || len(call.CredentialRefs) != 0 || !proto.Equal(contract, credentialcontrol.Contract(browsersso.RequestTool)) {
			return b, browsersso.ErrScope
		}
		scope = *a.SSOScope
	}
	for _, target := range scope.Targets() {
		if _, err := (egress.Policy{}).ValidateURL(target); err != nil {
			return b, browsersso.ErrScope
		}
	}
	reader, ok := s.browserScopes.(BrowserPageReader)
	if !ok {
		return b, browsersso.ErrScope
	}
	lookup := b
	lookup.ToolName = browserpage.Snapshot
	lookup.Resource = "browser:" + scope.Browser + ":"
	bound, e := reader.BrowserPageScope(ctx, lookup, browserpage.Args{SessionID: scope.Browser, Page: &scope.RPPage})
	if e != nil || bound == nil || !bound.Valid(true) || bound.Ref != scope.RPPage || bound.URL != scope.RPURL {
		return b, browsersso.ErrScope
	}
	copyBound := *bound
	b.BrowserPage = &copyBound
	b.BrowserSSO = &scope
	b.ActionHash = ""
	raw, _ := json.Marshal(b)
	b.ActionHash = digest(string(raw))
	return b, nil
}

func ssoOwnerResources(m Mandate, b ActionBinding) bool {
	if !validSSOScope(b) {
		return false
	}
	for _, target := range b.BrowserSSO.Targets() {
		allowed := false
		for _, scope := range m.ResourceScopes {
			if resourceWithin(scope, target) {
				allowed = true
				break
			}
		}
		if !allowed {
			return false
		}
	}
	return true
}
