package orchestrator

import "github.com/butler/butler/internal/memory/transcript"

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
