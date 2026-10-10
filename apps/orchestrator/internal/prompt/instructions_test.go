package prompt

import (
	"strings"
	"testing"
)

func TestTrustedPromptDoesNotExpandDataPlaceholder(t *testing.T) {
	a := NewAssembler().Assemble(ConfigState{EffectivePrompt: "Operator {{session_summary}} {{document_chunks}} {{browser_strategy}}"}, Context{SessionSummary: "MALICIOUS SUMMARY", Chunks: []map[string]any{{"title": "x", "summary": "MALICIOUS DOCUMENT"}}, BrowserStrategy: "MALICIOUS STRATEGY"})
	trusted := a.TrustedInstructions()
	if strings.Contains(trusted, "MALICIOUS") || !strings.Contains(trusted, BrowserStrategyContent) {
		t.Fatal("preview data entered trusted instructions")
	}
	if !strings.Contains(a.FinalPrompt, "MALICIOUS SUMMARY") {
		t.Fatal("operator preview lost context")
	}
}
