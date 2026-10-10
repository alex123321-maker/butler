package delegation

import (
	"strings"
	"testing"
)

func TestDelegatedPermissionCannotExpandAnyAuthorityDimension(t *testing.T) {
	hash := strings.Repeat("a", 64)
	ref := CredentialRef{Alias: "reviewed-service", Field: "header"}
	c := &Contract{Permissions: []Permission{{Tool: "http.request", ContractHash: hash, Resources: []string{"https://approved.example/a"}, Effects: []string{"read"}, Credentials: []CredentialRef{ref}}}}
	if !Allows(c, "http.request", hash, "https://approved.example/a", "read", []CredentialRef{ref}) {
		t.Fatal("exact narrowed permission denied")
	}
	for _, x := range []struct {
		tool, hash, resource, effect string
		refs                         []CredentialRef
	}{
		{"http.request", hash, "https://approved.example/b", "read", nil},
		{"http.request", hash, "https://approved.example/a", "commit", nil},
		{"http.request", strings.Repeat("b", 64), "https://approved.example/a", "read", nil},
		{"browser.read", hash, "https://approved.example/a", "read", nil},
		{"http.request", hash, "https://approved.example/a", "read", []CredentialRef{{Alias: "other-service", Field: "header"}}},
	} {
		if Allows(c, x.tool, x.hash, x.resource, x.effect, x.refs) {
			t.Fatalf("expanded permission admitted: %s %s", x.tool, x.effect)
		}
	}
	if !Allows(nil, "http.request", hash, "https://approved.example/a", "read", nil) {
		t.Fatal("ordinary task behavior changed")
	}
	if Allows(&Contract{}, "http.request", hash, "https://approved.example/a", "read", nil) {
		t.Fatal("empty delegated allowlist widened to all tools")
	}
}
