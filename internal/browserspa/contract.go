package browserspa

import (
	"net/url"
	"path"
	"strings"

	toolbrokerv1 "github.com/butler/butler/internal/gen/toolbroker/v1"
	"github.com/butler/butler/internal/jsonstrict"
	"github.com/butler/butler/internal/security/egress"
	"google.golang.org/protobuf/proto"
)

const Tool = "browser.read_spa"
const Profile = "graphql-read-stream-v1"
const AuthorityPath = "/internal/browser-spa-resources"
const CapabilityPath = "/internal/browser-spa-capabilities"

type Filter struct {
	Selector string `json:"selector"`
	Value    string `json:"value"`
}
type Args struct {
	URL           string  `json:"url"`
	Filter        *Filter `json:"filter,omitempty"`
	ReadySelector string  `json:"ready_selector,omitempty"`
	SettleMS      int     `json:"settle_ms,omitempty"`
}

func Parse(raw string) (Args, error) {
	var a Args
	if len(raw) > 12*1024 || jsonstrict.Decode([]byte(raw), &a) != nil || a.Validate() != nil {
		return Args{}, ErrDenied
	}
	return a, nil
}
func (a Args) Validate() error {
	u, err := (egress.Policy{}).ValidateURL(a.URL)
	if err != nil || u.Scheme != "https" || len(a.URL) > 4096 || u.Fragment != "" || u.ForceQuery || !NetworkURL(a.URL, a.URL) || a.SettleMS < 0 || a.SettleMS > 3000 || len(a.ReadySelector) > 1024 || strings.ContainsAny(a.ReadySelector, "\x00\r\n") {
		return ErrDenied
	}
	if a.Filter != nil && (strings.TrimSpace(a.Filter.Selector) == "" || len(a.Filter.Selector) > 1024 || len(a.Filter.Value) > 4096 || strings.ContainsAny(a.Filter.Selector+a.Filter.Value, "\x00\r\n")) {
		return ErrDenied
	}
	return nil
}

// NetworkURL is a same-origin/path canonicalization check, not authority.
// Actual path allow/deny and current source/goal/mandate checks run in Broker.
func NetworkURL(initial, target string) bool {
	a, e1 := url.Parse(initial)
	b, e2 := url.Parse(target)
	if e1 != nil || e2 != nil || a.Scheme != "https" || b.Scheme != a.Scheme || b.Host != a.Host || b.User != nil || b.Fragment != "" || len(target) > 4096 {
		return false
	}
	p := b.Path
	if p == "" {
		p = "/"
	}
	escaped := strings.ToLower(b.EscapedPath())
	return !strings.ContainsAny(p, "\\") && !strings.Contains(escaped, "%2f") && !strings.Contains(escaped, "%5c") && !strings.Contains(escaped, "%2e") && (p == "/" || path.Clean(p) == strings.TrimSuffix(p, "/"))
}

func Contract() *toolbrokerv1.ToolContract {
	return &toolbrokerv1.ToolContract{ToolName: Tool, ContractRevision: "1", ToolClass: "browser", RuntimeTarget: "tool-browser:9090", RiskLevel: "medium", Status: "active", Description: "Read a same-origin HTTPS SPA in a fresh, one-operation browser. The fixed query profile allows GET/HEAD, strictly parsed single GraphQL query POSTs, passive WSS or typed graphql-transport-ws read query/subscriptions, and SSE/ReadableStream including service workers. Optionally fill one public text/search/select filter observed on the site, then read visible results. No arbitrary POST, mutation/batch, untyped outgoing WS data or auth-init payload, credential references, persisted browser state or cross-origin navigation. All connections and workers are destroyed before returning; no continuation/replay. Requires current owner-goal authority; ordinary release and approval rules apply. Use ordinary browser.read for simple pages.", InputSchemaJson: `{"type":"object","required":["url"],"properties":{"url":{"type":"string","maxLength":4096},"filter":{"type":"object","required":["selector","value"],"properties":{"selector":{"type":"string","maxLength":1024},"value":{"type":"string","maxLength":4096}},"additionalProperties":false},"ready_selector":{"type":"string","maxLength":1024},"settle_ms":{"type":"integer","minimum":0,"maximum":3000}},"additionalProperties":false}`, OutputSchemaJson: `{"type":"object"}`, EffectContract: &toolbrokerv1.EffectContract{RetrySafety: "read_only", VerificationMethod: "read_result"}}
}
func ValidContract(c *toolbrokerv1.ToolContract) bool { return proto.Equal(c, Contract()) }
