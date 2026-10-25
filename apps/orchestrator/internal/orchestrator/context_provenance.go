package orchestrator

import (
	"encoding/json"
	"fmt"
	"github.com/butler/butler/internal/memory/transcript"
	"github.com/butler/butler/internal/security/content"
	"github.com/butler/butler/internal/transport"
)

func historicalMessageItem(message transcript.Message, role, text string) transport.InputItem {
	// Provenance metadata is not serialized by every provider. Keep the owning
	// run and message kind in model-visible DATA, including canonical answers.
	item := content.Data(map[string]any{
		"kind": "historical_message", "run_ref": message.RunID,
		"message_kind":  transcript.MessageKind(message.MetadataJSON),
		"original_role": role, "content": text,
	}, historicalMessageOrigin(message))
	item.Role = role
	return item
}

func historicalMessageOrigin(message transcript.Message) content.Origin {
	ref := "transcript:" + message.MessageID
	if message.MessageID == "" {
		ref = fmt.Sprintf("session:%s:run:%s:sequence:%d", message.SessionKey, message.RunID, message.Sequence)
	}
	var metadata struct {
		Provenance json.RawMessage `json:"butler_provenance"`
	}
	_ = json.Unmarshal([]byte(message.MetadataJSON), &metadata)
	return content.StoredDataOrigin(content.MemoryDerived, ref, string(metadata.Provenance))
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
