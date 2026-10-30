package actionauth

import (
	"encoding/hex"
	"encoding/json"
	"net/url"
	"strings"
)

// BrowserContextRead identifies no-navigation context operations whose public
// arguments contain an ID, rather than a URL. Input/gesture tools never qualify.
func BrowserContextRead(b ActionBinding) bool {
	if b.Effect != EffectRead || len(b.CredentialRefs) != 0 {
		return false
	}
	switch b.ToolName {
	case "browser.snapshot", "browser.freeze_snapshot", "browser.extract_text", "browser.close":
	default:
		return false
	}
	id := strings.TrimSuffix(strings.TrimPrefix(b.Resource, "browser:"), ":")
	_, err := hex.DecodeString(id)
	return err == nil && len(id) == 32 && b.Resource == "browser:"+id+":" && !strings.ContainsAny(id, ":/\\")
}

func bindBrowserReadScope(b ActionBinding, resource string, epoch int64) (ActionBinding, error) {
	u, e := url.Parse(resource)
	if !BrowserContextRead(b) || e != nil || u.Hostname() == "" || u.User != nil || (u.Scheme != "http" && u.Scheme != "https") || epoch < 0 {
		return b, ErrOwnerAuthority
	}
	b.BrowserScopeURL, b.BrowserScopeEpoch = resource, epoch
	b.ActionHash = ""
	encoded, _ := json.Marshal(b)
	b.ActionHash = digest(string(encoded))
	return b, nil
}

func scopedResourceWithin(s ResourceScope, b ActionBinding) bool {
	return resourceWithin(s, b.Resource) || (BrowserContextRead(b) && b.BrowserScopeURL != "" && resourceWithin(s, b.BrowserScopeURL))
}
