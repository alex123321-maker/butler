package main

import (
	"testing"

	"github.com/butler/butler/internal/transport"
)

func TestOperatorSmokeRejectsPartialAnswer(t *testing.T) {
	stream := make(chan transport.TransportEvent, 1)
	stream <- transport.NewAssistantFinalEvent("smoke", "antigravity", transport.AssistantFinal{Content: "partial answer", FinishReason: "MAX_TOKENS"})
	close(stream)
	if _, _, err := collect(stream); err == nil {
		t.Fatal("operator smoke certified an incomplete response")
	}
}
