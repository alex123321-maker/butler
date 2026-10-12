package transport

import "testing"

func TestUsageValidationKeepsAbsentAndExplicitZeroDistinct(t *testing.T) {
	absent := NewToolCallRequestedEvent("run", "fixture", ToolCallRequest{ToolCallRef: "call"})
	if absent.Usage != nil {
		t.Fatal("missing usage became zero")
	}
	zero := Usage{}
	final := NewAssistantFinalEvent("run", "fixture", AssistantFinal{Usage: &zero})
	if final.Usage == nil || final.AssistantFinal.Usage == nil || final.Usage.Validate() != nil {
		t.Fatal("explicit zero observation or final compatibility lost")
	}
	if (Usage{InputTokens: 17, OutputTokens: 14}).Validate() != nil {
		t.Fatal("legacy adapter without total became invalid")
	}
	if (Usage{InputTokens: 17, OutputTokens: 14, TotalTokens: 35}).Validate() != nil {
		t.Fatal("provider reasoning tokens cannot be represented")
	}
}
