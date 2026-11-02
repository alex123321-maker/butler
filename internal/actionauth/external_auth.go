package actionauth

import (
	"context"
	"encoding/json"

	"github.com/butler/butler/internal/browserpage"
	"github.com/butler/butler/internal/credentialcontrol"
	"github.com/butler/butler/internal/externalauth"
	toolbrokerv1 "github.com/butler/butler/internal/gen/toolbroker/v1"
	"github.com/butler/butler/internal/security/egress"
	"google.golang.org/protobuf/proto"
)

func validExternalScope(b ActionBinding) bool {
	return b.ToolName == externalauth.RequestTool && b.ExternalAuth != nil && b.ExternalAuth.Validate() == nil
}

func (s *Service) bindExternalAuth(ctx context.Context, b ActionBinding, call *toolbrokerv1.ToolCall, contract *toolbrokerv1.ToolContract) (ActionBinding, error) {
	a, e := externalauth.ParseRequest(call.ArgsJson)
	if e != nil || b.TaskID == "" || b.GoalID == "" || b.MandateID == "" || b.MandateRevision <= 0 || len(call.CredentialRefs) != 0 || !proto.Equal(contract, credentialcontrol.Contract(externalauth.RequestTool)) {
		return b, externalauth.ErrProvider
	}
	for _, target := range a.Scope.Targets() {
		if _, e := (egress.Policy{}).ValidateURL(target); e != nil {
			return b, externalauth.ErrProvider
		}
	}
	reader, ok := s.browserScopes.(BrowserPageReader)
	if !ok {
		return b, externalauth.ErrProvider
	}
	lookup := b
	lookup.ToolName = browserpage.Snapshot
	lookup.Resource = "browser:" + a.Scope.Browser + ":"
	page, e := reader.BrowserPageScope(ctx, lookup, browserpage.Args{SessionID: a.Scope.Browser, Page: &a.Scope.RPPage})
	if e != nil || page == nil || !page.Valid(true) || page.Ref != a.Scope.RPPage || page.URL != a.Scope.RPURL {
		return b, externalauth.ErrProvider
	}
	copyPage := *page
	b.BrowserPage = &copyPage
	b.ExternalAuth = &a.Scope
	b.ActionHash = ""
	raw, _ := json.Marshal(b)
	b.ActionHash = digest(string(raw))
	return b, nil
}

func externalOwnerResources(m Mandate, b ActionBinding) bool {
	if !validExternalScope(b) {
		return false
	}
	for _, target := range b.ExternalAuth.Targets() {
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
