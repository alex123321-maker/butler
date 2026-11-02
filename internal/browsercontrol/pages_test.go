package browsercontrol

import (
	"strings"
	"testing"
)

func TestPageBindingURLRemovesCallbackMaterial(t *testing.T) {
	for _, raw := range []string{
		"https://rp.example/callback?code=private-code-canary&state=private-state-canary#private-fragment-canary",
		"https://rp.example/callback?",
		"https://rp.example/callback#private-fragment-canary",
	} {
		clean, err := pageURL(raw)
		if err != nil || clean != "https://rp.example/callback" {
			t.Fatal("callback material retained or valid page URL denied")
		}
	}
	for _, raw := range []string{"https://user:private-password@rp.example/", "javascript:alert(1)", "file:///etc/passwd", "//rp.example/path", "https://rp.example/\n", "https://rp.example/" + strings.Repeat("a", 2048)} {
		if clean, err := pageURL(raw); err == nil || clean != "" {
			t.Fatal("invalid/private URL was accepted")
		}
	}
}

func TestPageBindingReferencesAreExact(t *testing.T) {
	id := strings.Repeat("a", 32)
	doc := strings.Repeat("b", 32)
	if !validPageRef(PageRef{ID: id}) || !validPageRef(PageRef{ID: id, DocumentID: doc, Version: 1}) {
		t.Fatal("valid pending/observed reference rejected")
	}
	for _, ref := range []PageRef{{ID: strings.ToUpper(id)}, {ID: id, DocumentID: doc}, {ID: id, Version: 1}, {ID: id, DocumentID: doc, Version: -1}, {ID: "same-title", DocumentID: doc, Version: 1}} {
		if validPageRef(ref) {
			t.Fatal("ambiguous or malformed page reference accepted")
		}
	}
}
