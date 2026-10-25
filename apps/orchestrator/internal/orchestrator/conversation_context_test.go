package orchestrator

import (
	"encoding/json"
	"github.com/butler/butler/internal/memory/transcript"
	"strings"
	"testing"
	"time"
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

func TestConversationWorkflowExamplesAreBoundedUnverifiedAndValueFree(t *testing.T) {
	now := time.Now()
	full := transcript.Transcript{Messages: []transcript.Message{
		{RunID: "old", Role: "user", Content: "Check the first project"},
		{RunID: "old", MessageID: "publication:old", Role: "assistant", MetadataJSON: `{"kind":"publication","purpose":"result"}`},
		{RunID: "new", Role: "user", Content: "Check the first project"},
		{RunID: "new", MessageID: "publication:new", Role: "assistant", MetadataJSON: `{"kind":"publication","purpose":"result"}`},
		{RunID: "other", Role: "user", Content: "Check the other project"},
		{RunID: "other", MessageID: "publication:other", Role: "assistant", MetadataJSON: `{"kind":"publication","purpose":"result"}`},
		{RunID: "failed", Role: "user", Content: "Unfinished request"},
		{RunID: "failed", MessageID: "publication:failed", Role: "assistant", MetadataJSON: `{"kind":"publication","purpose":"blocked"}`},
	}, ToolCalls: []transcript.ToolCall{
		{RunID: "old", ToolName: "web.fetch", Status: "completed", ResultJSON: `{"status_code":200,"final_url":"https://issuer.example/","content_text":"Actual source observation."}`, FinishedAt: &now},
		{RunID: "new", ToolName: "tools.search", Status: "completed", FinishedAt: &now},
		{RunID: "new", ToolName: "web.fetch", Status: "failed", FinishedAt: &now},
		{RunID: "new", ToolName: "web.fetch", Status: "completed", ResultJSON: `{"status_code":200,"final_url":"https://issuer.example/","content_text":"Actual source observation."}`, FinishedAt: &now, ArgsJSON: `{"url":"https://do-not-copy.example/private"}`},
		{RunID: "new", ToolName: "web.fetch", Status: "completed", ResultJSON: `{"status_code":200,"final_url":"https://issuer.example/","content_text":"Actual source observation."}`, FinishedAt: &now},
		{RunID: "new", ToolName: "credentials.request_access", Status: "completed", FinishedAt: &now},
		{RunID: "other", ToolName: "http.request", Status: "completed", FinishedAt: &now, ArgsJSON: `{"method":"POST","body":"do-not-copy"}`},
	}}
	items, _ := publishedWorkflowExamples(full, []string{"new", "old", "other", "failed"})
	if len(items) != 2 || items[0].PublicationRef != "publication:new" || len(items[0].Operations) != 1 || items[0].Operations[0] != "web.fetch" || items[1].Operations[0] != "http.request:POST" || items[0].Status != "published_unverified" {
		t.Fatal(items)
	}
	encoded, _ := json.Marshal(items)
	if strings.Contains(string(encoded), "do-not-copy") || strings.Contains(string(encoded), "credentials") {
		t.Fatal("trace copied private values", string(encoded))
	}
	items, _ = publishedWorkflowExamples(full, []string{"failed"})
	if len(items) != 0 {
		t.Fatal("unfinished run became a workflow example")
	}
}
