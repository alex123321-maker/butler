package browsersso

import (
	"net/url"
	"path"
	"strings"
	"time"

	"github.com/butler/butler/internal/browserpage"
)

type NetworkScope struct {
	Origin     string `json:"origin"`
	PathPrefix string `json:"path_prefix"`
}
type Dispatch struct {
	Page      browserpage.Binding `json:"page"`
	Digest    string              `json:"scope_digest"`
	ExpiresAt time.Time           `json:"expires_at"`
	Allowed   []NetworkScope      `json:"allowed"`
	Denied    []NetworkScope      `json:"denied"`
}

func within(scope NetworkScope, raw string) bool {
	u, e := url.Parse(raw)
	if e != nil || u.User != nil || u.Scheme != "https" || u.Host == "" || !strings.EqualFold(u.Scheme+"://"+u.Host, scope.Origin) {
		return false
	}
	p := u.Path
	if p == "" {
		p = "/"
	}
	escaped := strings.ToLower(u.EscapedPath())
	if strings.Contains(escaped, "%2f") || strings.Contains(escaped, "%5c") || strings.Contains(escaped, "%2e") || strings.Contains(p, "\\") || path.Clean(p) != strings.TrimSuffix(p, "/") && p != "/" {
		return false
	}
	prefix := strings.TrimSuffix(scope.PathPrefix, "/")
	return prefix == "" || p == prefix || strings.HasPrefix(p, prefix+"/")
}
func (d Dispatch) Allows(raw string) bool {
	for _, s := range d.Denied {
		if within(s, raw) {
			return false
		}
	}
	for _, s := range d.Allowed {
		if within(s, raw) {
			return true
		}
	}
	return false
}
func (d Dispatch) Valid(s Scope) bool {
	if s.Validate() != nil || !d.Page.Valid(true) || d.Page.Ref != s.RPPage || d.Page.URL != s.RPURL || !digest(d.Digest) || len(d.Allowed) == 0 || len(d.Allowed) > 64 || len(d.Denied) > 64 || d.ExpiresAt.IsZero() {
		return false
	}
	for _, scope := range append(append([]NetworkScope{}, d.Allowed...), d.Denied...) {
		prefix, err := url.Parse(scope.PathPrefix)
		canonicalOrigin := Origin(scope.Origin + "/")
		if canonicalOrigin == "" || canonicalOrigin != scope.Origin || scope.PathPrefix == "" || !strings.HasPrefix(scope.PathPrefix, "/") || len(scope.PathPrefix) > 2048 || err != nil || prefix.Host != "" || prefix.Scheme != "" || prefix.RawQuery != "" || prefix.ForceQuery || prefix.Fragment != "" || prefix.RawPath != "" || strings.Contains(scope.PathPrefix, "\\") || path.Clean(scope.PathPrefix) != strings.TrimSuffix(scope.PathPrefix, "/") && scope.PathPrefix != "/" {
			return false
		}
	}
	for _, raw := range s.Targets() {
		if !d.Allows(raw) {
			return false
		}
	}
	return true
}
