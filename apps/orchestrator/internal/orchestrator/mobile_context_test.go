package orchestrator

import (
	"context"
	"strings"
	"testing"

	"github.com/butler/butler/apps/orchestrator/internal/task"
	"github.com/butler/butler/internal/memory/transcript"
)

type mobileHistoryStore struct {
	*memoryTranscriptStore
	selected transcript.Transcript
	task     string
}

func (s *mobileHistoryStore) GetTaskTranscript(_ context.Context, _ string, id string) (transcript.Transcript, error) {
	s.task = id
	return s.selected, nil
}

func TestMobileTaskHistoryDoesNotGrowWithOtherBatches(t *testing.T) {
	for _, oldCount := range []int{0, 20, 1000} {
		history := make([]transcript.Message, oldCount)
		for i := range history {
			history[i] = transcript.Message{Role: "user", Content: strings.Repeat("old batch instructions ", 300)}
		}
		store := &mobileHistoryStore{memoryTranscriptStore: &memoryTranscriptStore{messages: history}, selected: transcript.Transcript{SourceRevision: 7, Messages: []transcript.Message{{RunID: "previous-attempt", Role: "assistant", Content: "One item remains; durable decisions are authoritative."}, {RunID: "current", Role: "user", Content: "current input"}}}}
		service := newFixtureService(&memorySessionRepo{}, &memoryLeaseManager{}, newMemoryRunManager(), store, &mockProvider{}, Config{}, nil)
		prepared := preparedRun{CurrentRunID: "current", Task: &task.Record{TaskID: "current-task", Source: map[string]any{"type": "initiative", "mobile_batch_id": "current-batch"}}}
		if e := service.attachTranscriptContext(context.Background(), "session", &prepared); e != nil {
			t.Fatal(e)
		}
		if store.task != "current-task" || len(prepared.InputItems) != 1 || !prepared.InputAlreadyRecorded {
			t.Fatalf("wrong task/current-input coverage: %d", len(prepared.InputItems))
		}
		if strings.Contains(prepared.InputItems[0].Content, "old batch") {
			t.Fatal("unrelated batch history leaked into new task")
		}
	}
}
