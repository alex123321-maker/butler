package pipeline

import (
	"encoding/json"
	"github.com/butler/butler/internal/memory/transcript"
	"github.com/butler/butler/internal/security/content"
)

// Extraction has no right to mint provenance. All candidates inherit the
// complete source observation references and default private sensitivity.
func transcriptOrigin(source transcript.Transcript, runID string) content.Origin {
	parents := []content.Origin{{Kind: content.MemoryDerived, SourceRefs: []string{"run:" + runID}, Sensitivity: content.Private}}
	for _, message := range source.Messages {
		parents = append(parents, content.Origin{Kind: content.MemoryDerived, SourceRefs: []string{"transcript:" + message.MessageID}, Sensitivity: content.Private})
		var metadata struct {
			Origin content.Origin `json:"butler_provenance"`
		}
		if json.Unmarshal([]byte(message.MetadataJSON), &metadata) == nil {
			encoded, _ := json.Marshal(metadata.Origin)
			parents = append(parents, content.StoredDataOrigin(content.MemoryDerived, "transcript:"+message.MessageID, string(encoded)))
		}
	}
	for _, call := range source.ToolCalls {
		parents = append(parents, content.Origin{Kind: content.ToolOutput, SourceRefs: []string{"tool_call:" + call.ToolCallID}, Sensitivity: content.Private})
	}
	return content.Derive(content.MemoryDerived, parents...)
}

func originJSON(source transcript.Transcript, runID string) string {
	value, _ := json.Marshal(transcriptOrigin(source, runID))
	return string(value)
}

func firstTranscript(sources []transcript.Transcript) transcript.Transcript {
	if len(sources) > 0 {
		return sources[0]
	}
	return transcript.Transcript{}
}
