package antigravityacp

import (
	"context"
	"testing"
	"time"

	"github.com/butler/butler/internal/acp"
)

func TestACPDeclaredTimeMatchesConfiguredFiniteBound(t *testing.T) {
	runner := func(context.Context, acp.AntigravityProcess, string, string, func(context.Context) error) (acp.Result, error) {
		return acp.Result{}, nil
	}
	for _, timeout := range []time.Duration{time.Second, 30 * time.Second, 120 * time.Second, 300 * time.Second} {
		provider, err := newProvider(Config{Model: "gemini-fixture", Timeout: timeout}, runner)
		if err != nil || provider.AgentExecutionLimits().WallTime != timeout || !provider.AgentExecutionLimits().Valid() {
			t.Fatal("configured execution bound lost", timeout, err)
		}
	}
	for _, timeout := range []time.Duration{0, -time.Second, 301 * time.Second} {
		if _, err := newProvider(Config{Model: "gemini-fixture", Timeout: timeout}, runner); err == nil {
			t.Fatal("unbounded/unqualified duration accepted")
		}
	}
}
