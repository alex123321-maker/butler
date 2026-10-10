package pipeline

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"strings"
	"testing"

	"github.com/butler/butler/internal/memory/transcript"
	"github.com/butler/butler/internal/security/content"
)

type forbiddenModelCall struct{}

func (forbiddenModelCall) Complete(context.Context, string, string) (string, error) {
	panic("secret or denied source reached model")
}

func sensitiveSource(t *testing.T) transcript.Transcript {
	t.Helper()
	metadata, _ := json.Marshal(map[string]any{"butler_provenance": content.Origin{Kind: content.SubagentOutput, SourceRefs: []string{"document:private-source", "child:observation"}, Sensitivity: content.Secret}})
	return transcript.Transcript{Messages: []transcript.Message{{MessageID: "private-message", Role: "assistant", Content: "synthetic private source", MetadataJSON: string(metadata)}}}
}

func TestSecretSourceCannotReachExtractionOrSessionSummarizer(t *testing.T) {
	source := sensitiveSource(t)
	derived := transcriptOrigin(source, "run")
	if derived.Sensitivity != content.Secret || derived.IsInstruction() || !strings.Contains(strings.Join(derived.ParentRefs, "|"), "document:private-source") {
		t.Fatal("derived memory lowered sensitivity or discarded source provenance")
	}
	extractor := NewLLMExtractor(forbiddenModelCall{})
	if _, err := extractor.Extract(context.Background(), "session", source); !errors.Is(err, content.ErrSecret) {
		t.Fatal("secret extraction accepted", err)
	}
	if _, err := extractor.SummarizeSession(context.Background(), "session", "", source.Messages); !errors.Is(err, content.ErrSecret) {
		t.Fatal("secret summary accepted", err)
	}
	worker := NewWorker(nil, sessionTestReader{source: source}, extractor, nil, nil, nil, nil, WorkerConfig{}, nil, nil)
	if err := worker.processPostRun(context.Background(), slog.Default(), &Job{SessionKey: "session", RunID: "run"}); !errors.Is(err, content.ErrSecret) {
		t.Fatal("secret worker source accepted", err)
	}
	worker.SetDataReleaseGuard(func(context.Context, string) error { return errors.New("private policy detail") })
	if err := worker.processPostRun(context.Background(), slog.Default(), &Job{SessionKey: "session", RunID: "run"}); err == nil || strings.Contains(err.Error(), "private policy detail") {
		t.Fatal("guard denial lost or leaked authority context")
	}
}
