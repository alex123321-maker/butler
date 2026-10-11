package delegation

import (
	"strings"
	"testing"
	"time"
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

func TestContractRejectsAmbiguousOrUnboundedPins(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Microsecond)
	valid := func() Contract {
		return Contract{TaskID: "child", ParentTaskID: "parent", RootTaskID: "parent", ParentRevision: 1, ConstraintsRevision: 1, Revision: 1, Depth: 1, Deadline: now.Add(time.Hour), ContractSource: SourceRef{ID: "proposal", Revision: 1, AccessRevision: 1, Hash: strings.Repeat("a", 64)}, Sources: []SourceRef{{ID: "selected", Revision: 1, AccessRevision: 1, Hash: strings.Repeat("b", 64)}}, Permissions: []Permission{{Tool: "http.request", ContractHash: strings.Repeat("c", 64), Resources: []string{"https://approved.example/a"}, Effects: []string{"read"}}}}
	}
	if err := Validate(valid(), now); err != nil {
		t.Fatal("bounded contract denied", err)
	}
	for name, mutate := range map[string]func(*Contract){
		"self root":          func(c *Contract) { c.RootTaskID = c.TaskID },
		"expired":            func(c *Contract) { c.Deadline = now },
		"unbounded deadline": func(c *Contract) { c.Deadline = now.Add(25 * time.Hour) },
		"negative budget":    func(c *Contract) { c.Budget.Children = -1 },
		"ambiguous source":   func(c *Contract) { c.Sources = append(c.Sources, c.ContractSource) },
		"non hex pin":        func(c *Contract) { c.ContractSource.Hash = strings.Repeat("z", 64) },
		"uppercase pin":      func(c *Contract) { c.Permissions[0].ContractHash = strings.Repeat("A", 64) },
		"wildcard tool":      func(c *Contract) { c.Permissions[0].Tool = "http.*" },
		"duplicate effect":   func(c *Contract) { c.Permissions[0].Effects = []string{"read", "read"} },
		"duplicate resource": func(c *Contract) {
			c.Permissions[0].Resources = append(c.Permissions[0].Resources, c.Permissions[0].Resources[0])
		},
	} {
		t.Run(name, func(t *testing.T) {
			c := valid()
			mutate(&c)
			if Validate(c, now) == nil {
				t.Fatal("ambiguous or unbounded contract admitted")
			}
		})
	}
}
