package orchestrator

import (
	"encoding/json"
	"github.com/butler/butler/internal/memory/transcript"
	"github.com/butler/butler/internal/research"
	"github.com/butler/butler/internal/security/content"
	"strings"
)

type workflowExample struct {
	PublicationRef string   `json:"publication_ref"`
	Operations     []string `json:"operations"`
	Status         string   `json:"status"`
}

// The caller supplies only the already source-checked completed/published run
// selection. Trace metadata helps recognize repetition without replaying old
// tool bodies, values or credentials, and is never verified skill evidence.
func publishedWorkflowExamples(full transcript.Transcript, previous []string) ([]workflowExample, content.Origin) {
	requests := map[string]string{}
	publications := map[string]transcript.Message{}
	for _, m := range full.Messages {
		if m.Role == "user" && transcript.MessageKind(m.MetadataJSON) != "interaction_response" && requests[m.RunID] == "" {
			requests[m.RunID] = strings.ToLower(strings.Join(strings.Fields(m.Content), " "))
		}
		var meta struct{ Kind, Purpose string }
		_ = json.Unmarshal([]byte(m.MetadataJSON), &meta)
		if meta.Kind == "publication" && meta.Purpose == "result" {
			publications[m.RunID] = m
		}
	}
	out := []workflowExample{}
	seen := map[string]bool{}
	origins := []content.Origin{}
	for _, run := range previous {
		pub, ok := publications[run]
		request := requests[run]
		if !ok || request == "" || seen[request] || pub.MessageID == "" {
			continue
		}
		operations := []string{}
		for _, call := range full.ToolCalls {
			if call.RunID != run || call.Status != "completed" || call.FinishedAt == nil {
				continue
			}
			if research.IsReadAttempt(call) && call.ToolName != "web.search" {
				pages, _ := research.Pages([]transcript.ToolCall{call}, run)
				if len(pages) == 0 {
					continue
				}
			}
			name := call.ToolName
			if strings.HasPrefix(name, "tools.") || strings.HasPrefix(name, "communication.") || strings.HasPrefix(name, "credentials.") || strings.HasPrefix(name, "memory.") || strings.HasPrefix(name, "skills.") {
				continue
			}
			if name == "http.request" {
				var a struct{ Method string }
				if json.Unmarshal([]byte(call.ArgsJSON), &a) != nil {
					continue
				}
				method := strings.ToUpper(a.Method)
				if method == "" {
					method = "GET"
				}
				switch method {
				case "GET", "HEAD", "POST", "PUT", "PATCH", "DELETE", "OPTIONS":
					name += ":" + method
				default:
					continue
				}
			}
			if len(operations) == 0 || operations[len(operations)-1] != name {
				operations = append(operations, name)
			}
			if len(operations) == 12 {
				break
			}
		}
		if len(operations) == 0 {
			continue
		}
		seen[request] = true
		out = append(out, workflowExample{PublicationRef: pub.MessageID, Operations: operations, Status: "published_unverified"})
		origins = append(origins, historicalMessageOrigin(pub))
		if len(out) == 2 {
			break
		}
	}
	return out, content.Derive(content.MemoryDerived, origins...)
}

func conversationMessages(all []transcript.Message, current string, previous []string) ([]transcript.Message, bool) {
	allowed := map[string]bool{}
	for _, id := range previous {
		allowed[id] = true
	}
	items := []transcript.Message{}
	recorded := false
	for _, m := range all {
		if m.RunID == current {
			if m.Role == "user" && transcript.MessageKind(m.MetadataJSON) != "interaction_response" {
				recorded = true
				continue
			}
			items = append(items, m)
		} else if allowed[m.RunID] && (m.Role == "user" || transcript.MessageKind(m.MetadataJSON) == "publication") {
			items = append(items, m)
		}
	}
	return items, recorded
}
