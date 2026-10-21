package pipeline

import (
	"github.com/butler/butler/internal/memory/transcript"
	"strings"
	"testing"
)

func TestCommunicationExtractionPreservesPublicationProvenance(t *testing.T) {
	source := transcript.Transcript{Messages: []transcript.Message{{Role: "assistant", Content: "visible", MetadataJSON: `{"kind":"publication"}`}, {Role: "system", Content: "internal", MetadataJSON: `{"kind":"attempt_final"}`}}}
	text := formatTranscriptForExtraction("session", source)
	if !strings.Contains(text, "[internal_attempt_final; not published] internal") || !strings.Contains(text, "assistant publication; delivery tracked separately") {
		t.Fatal(text)
	}
}
