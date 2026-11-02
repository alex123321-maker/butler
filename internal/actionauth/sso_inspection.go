package actionauth

import (
	"context"
	"encoding/json"
	"github.com/butler/butler/internal/browserpage"
	"github.com/butler/butler/internal/browsersso"
	toolbrokerv1 "github.com/butler/butler/internal/gen/toolbroker/v1"
	"github.com/butler/butler/internal/security/egress"
	"google.golang.org/protobuf/proto"
)

func validSSOInspection(b ActionBinding) bool {
	return b.ToolName == browsersso.InspectTool && b.Effect == EffectRead && b.BrowserSSOInspection != nil && b.BrowserSSOInspection.Validate() == nil
}
func (s *Service) bindSSOInspection(ctx context.Context, b ActionBinding, call *toolbrokerv1.ToolCall, c *toolbrokerv1.ToolContract) (ActionBinding, error) {
	a, err := browsersso.ParseInspection(call.ArgsJson)
	reader, ok := s.browserScopes.(BrowserPageReader)
	if err != nil || !ok || !proto.Equal(c, browsersso.InspectContract()) || len(call.CredentialRefs) != 0 || b.Effect != EffectRead || b.TaskID == "" || b.GoalID == "" || b.MandateID == "" {
		return b, browsersso.ErrScope
	}
	for _, url := range a.Targets() {
		if _, err := (egress.Policy{}).ValidateURL(url); err != nil {
			return b, browsersso.ErrScope
		}
	}
	lookup := b
	lookup.ToolName = browserpage.Snapshot
	lookup.Resource = "browser:" + a.Browser + ":"
	page, err := reader.BrowserPageScope(ctx, lookup, browserpage.Args{SessionID: a.Browser, Page: &a.RPPage})
	if err != nil || page == nil || !page.Valid(true) || page.Ref != a.RPPage || page.URL != a.RPURL {
		return b, browsersso.ErrScope
	}
	copy := *page
	b.BrowserPage = &copy
	b.BrowserSSOInspection = &a
	b.ActionHash = ""
	raw, _ := json.Marshal(b)
	b.ActionHash = digest(string(raw))
	return b, nil
}
func inspectionOwnerResources(m Mandate, b ActionBinding) bool {
	if !validSSOInspection(b) {
		return false
	}
	for _, url := range b.BrowserSSOInspection.Targets() {
		allowed := false
		for _, scope := range m.ResourceScopes {
			if resourceWithin(scope, url) {
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
