package actionauth

import "testing"

func TestLocalResourceScopesAreExactAndDoNotAuthorizeNetwork(t *testing.T) {
	s := ResourceScope{Origin: "tool:skills.load"}
	if !validResourceScope(s) || !resourceWithin(s, "tool:skills.load") {
		t.Fatal("exact local resource rejected")
	}
	for _, raw := range []string{"tool:skills.load-other", "tool:skills.load/anything", "tool:skills.load?private=secret", "https://example.com/", "tool:skills.propose"} {
		if resourceWithin(s, raw) {
			t.Fatalf("local scope broadened to %s", raw)
		}
	}
	for _, origin := range []string{"tool:*", "tool:skills.*", "tool:skills.load?x", "tool:skills.load/", "tool:skills.load\n", "tool:skills.load:root"} {
		if validResourceScope(ResourceScope{Origin: origin}) {
			t.Fatalf("invalid local scope accepted: %q", origin)
		}
	}
	if validResourceScope(ResourceScope{Origin: s.Origin, PathPrefix: "/"}) {
		t.Fatal("local path widened")
	}
}
