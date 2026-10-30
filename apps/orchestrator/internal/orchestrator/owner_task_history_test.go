package orchestrator

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/butler/butler/apps/orchestrator/internal/task"
	"github.com/butler/butler/internal/memory/transcript"
	"github.com/butler/butler/internal/security/content"
)

func TestOwnerGoalTaskHistoryRetainsOwnAttemptsAndInteraction(t *testing.T) {
	old := []transcript.Message{{Role: "user", Content: strings.Repeat("unrelated session work ", 50000)}}
	selected := transcript.Transcript{SourceRevision: 9, Messages: []transcript.Message{
		{RunID: "previous", Role: "assistant", Content: "Own previous failed attempt and UNKNOWN remain"},
		{RunID: "current", Role: "user", Content: "current input"},
		{RunID: "current", Role: "user", Content: "Own exact owner response", MetadataJSON: `{"kind":"interaction_response"}`},
	}}
	store := &mobileHistoryStore{memoryTranscriptStore: &memoryTranscriptStore{messages: old}, selected: selected}
	s := newFixtureService(&memorySessionRepo{}, &memoryLeaseManager{}, newMemoryRunManager(), store, &mockProvider{}, Config{}, nil)
	p := preparedRun{CurrentRunID: "current", Task: &task.Record{TaskID: "owner-task", Source: map[string]any{"type": "owner_goal_task"}}}
	if err := s.attachTranscriptContext(context.Background(), "session", &p); err != nil {
		t.Fatal(err)
	}
	text := fixtureContextText(p.InputItems)
	if store.task != "owner-task" || !p.InputAlreadyRecorded || strings.Contains(text, "unrelated") || !strings.Contains(text, "previous failed") || !strings.Contains(text, "exact owner response") || len(p.InputItems) != 2 {
		t.Fatal("lost own history/response or replayed unrelated session", len(p.InputItems))
	}
	if p.observabilityHistory["coverage_mode"] != "owner_goal_task_history" {
		t.Fatal("missing actual task coverage")
	}
}

func TestOwnerGoalTaskHistoryCannotLaunderSecretOrMissingReader(t *testing.T) {
	origin := content.Origin{Kind: content.OwnerPolicy, SourceRefs: []string{"message:secret"}, Sensitivity: content.Secret}
	meta, _ := json.Marshal(map[string]any{"butler_provenance": origin})
	store := &mobileHistoryStore{memoryTranscriptStore: &memoryTranscriptStore{}, selected: transcript.Transcript{Messages: []transcript.Message{{Role: "assistant", Content: "synthetic secret", MetadataJSON: string(meta)}}}}
	s := newFixtureService(&memorySessionRepo{}, &memoryLeaseManager{}, newMemoryRunManager(), store, &mockProvider{}, Config{}, nil)
	p := preparedRun{Task: &task.Record{TaskID: "owner-task", Source: map[string]any{"type": "owner_goal_task"}}}
	if err := s.attachTranscriptContext(context.Background(), "session", &p); !errors.Is(err, content.ErrSecret) || len(p.InputItems) != 0 {
		t.Fatal("secret task source released", err)
	}
	s = newFixtureService(&memorySessionRepo{}, &memoryLeaseManager{}, newMemoryRunManager(), &memoryTranscriptStore{}, &mockProvider{}, Config{}, nil)
	if err := s.attachTranscriptContext(context.Background(), "session", &p); err == nil {
		t.Fatal("missing scoped reader fell back to session replay")
	}
}
