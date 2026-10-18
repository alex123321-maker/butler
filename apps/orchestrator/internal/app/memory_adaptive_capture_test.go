package app

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/butler/butler/internal/acceptance"
	"github.com/butler/butler/internal/security/content"
	"github.com/butler/butler/internal/transport"
)

func TestAdaptiveNativeCaptureDoesNotMutateRetainedTrace(t *testing.T) {
	p := &adaptiveNativeProvider{calls: []*adaptiveNativeCall{{ID: 1, RunID: "controlled-run", Kind: "start", Start: &transport.StartRunRequest{InputItems: []transport.InputItem{{Role: "tool", Content: "controlled visible DATA", Metadata: map[string]any{"butler_provenance": content.Origin{Kind: content.Document, SourceRefs: []string{"owned-source"}, Sensitivity: content.Public}}}}}}}}
	before, err := json.Marshal(p.calls)
	if err != nil {
		t.Fatal("synthetic trace unavailable")
	}
	copy, captured, err := p.captureRun("controlled-run")
	if err != nil {
		t.Fatal("controlled capture failed")
	}
	after, _ := json.Marshal(p.calls)
	if !bytes.Equal(before, after) {
		t.Fatal("capture decoder mutated retained metadata; original CaptureSHA cannot match exported trace")
	}
	if !bytes.Equal(before, captured) {
		t.Fatal("capture bytes changed before observation")
	}
	retained, _ := json.MarshalIndent(p.calls, "", "  ")
	var rawCalls []json.RawMessage
	if json.Unmarshal(retained, &rawCalls) != nil {
		t.Fatal("retained original unreadable")
	}
	compacted, _ := json.Marshal(rawCalls)
	if acceptance.MemoryBytesSHA256(compacted) != acceptance.MemoryBytesSHA256(captured) {
		t.Fatal("exact retained trace no longer proves original capture")
	}
	copy[0].Start.InputItems[0].Content = "changed copied observation"
	copy[0].Start.InputItems[0].Metadata["butler_provenance"] = "changed copied provenance"
	unchanged, _ := json.Marshal(p.calls)
	if !bytes.Equal(before, unchanged) {
		t.Fatal("independent observation still aliases retained native trace")
	}
}
