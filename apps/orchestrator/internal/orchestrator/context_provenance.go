package orchestrator

import (
	"fmt"
	"github.com/butler/butler/internal/memory/transcript"
	"github.com/butler/butler/internal/security/content"
	"github.com/butler/butler/internal/transport"
)

func historicalMessageItem(message transcript.Message, role, text string) transport.InputItem {
	ref := "transcript:" + message.MessageID
	if message.MessageID == "" {
		ref = fmt.Sprintf("session:%s:run:%s:sequence:%d", message.SessionKey, message.RunID, message.Sequence)
	}
	return content.HistoricalMessage(role, text, ref)
}

func memorySourceRefs(bundle map[string]any, section string) []string {
	keys := map[string]string{"profile_memory": "profile", "episodic_memory": "episodes", "document_chunks": "chunks", "working_memory": "working"}
	refs := []string{"retrieval:" + section}
	for _, entry := range sliceOfMaps(bundle[keys[section]]) {
		if origin, ok := entry["origin"].(content.Origin); ok {
			refs = append(refs, origin.SourceRefs...)
			refs = append(refs, origin.ParentRefs...)
		}
		for _, ref := range []string{"source_ref", "source", "key"} {
			if value, ok := entry[ref].(string); ok && value != "" {
				refs = append(refs, value)
			}
		}
	}
	return refs
}
