package budget

import (
	"errors"
	"testing"
	"time"

	"github.com/butler/butler/internal/security/content"
)

func TestLocalTargetIsIndependentOfDeclaredZeroPrice(t *testing.T) {
	for _, endpoint := range []string{"", "https://example.invalid", "local-models.example:9090", "local-models:8093"} {
		if _, e := NewLocalTarget(endpoint, "operator"); !errors.Is(e, ErrInvalid) {
			t.Fatal("unproved target", endpoint, e)
		}
	}
	if _, e := NewLocalTarget("local-models:9090", ""); !errors.Is(e, ErrInvalid) {
		t.Fatal("missing pinned identity", e)
	}
	target, e := NewLocalTarget("local-models:9090", "operator")
	if e != nil || target.Endpoint() != "local-models:9090" || target.Identity() != "operator" {
		t.Fatal(target, e)
	}
	// Target construction alone never supplies pricing or grants an attempt.
	c := &Controller{Policy: DefaultPolicy()}
	if _, e = c.localQuote(target, LocalOperation{Kind: "query", Items: 1, InputBytes: 16, Timeout: time.Second, Origin: content.Derive(content.MemoryDerived)}); !errors.Is(e, ErrInvalid) {
		t.Fatal("unconfigured controller allowed call", e)
	}
}
