package orchestrator

import (
	"encoding/json"
	"github.com/butler/butler/internal/transport"
	"testing"
)

func TestModelTurnUsageIndependentMetadataCopies(t *testing.T) {
	var a, b transport.Usage
	raw := []byte(`{"InputTokens":12,"OutputTokens":8,"TotalTokens":20,"CachedInputTokens":4,"OutputReasoningTokens":5}`)
	_ = json.Unmarshal(raw, &a)
	_ = json.Unmarshal(raw, &b)
	e := transport.NewAssistantFinalEvent("run", "openai", transport.AssistantFinal{Content: "done", Usage: &a})
	e.Usage = &b
	if _, err := modelTurnUsage(e); err != nil {
		t.Fatal("equal values falsely conflict", err)
	}
	*b.OutputReasoningTokens = 4
	if _, err := modelTurnUsage(e); err == nil {
		t.Fatal("conflicting values accepted")
	}
}
