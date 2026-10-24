package orchestrator

import (
	"github.com/butler/butler/internal/memory/transcript"
	"testing"
)

func TestConversationContextKeepsPublishedFollowupAndCurrentWork(t *testing.T) {
	all := []transcript.Message{
		{RunID: "abandoned", Role: "user", Content: "obsolete widget request"},
		{RunID: "prior", Role: "user", Content: "original question"},
		{RunID: "prior", Role: "system", Content: "unpublished attempt", MetadataJSON: `{"kind":"attempt_final"}`},
		{RunID: "prior", Role: "assistant", Content: "visible answer", MetadataJSON: `{"kind":"publication"}`},
		{RunID: "prior", Role: "tool", Content: "old tool output"},
		{RunID: "current", Role: "user", Content: "followup"},
		{RunID: "current", Role: "tool", Content: "current observation"},
	}
	items, recorded := conversationMessages(all, "current", []string{"prior"})
	if !recorded || len(items) != 3 || items[0].Content != "original question" || items[1].Content != "visible answer" || items[2].Content != "current observation" {
		t.Fatalf("wrong context: %+v %v", items, recorded)
	}
}
