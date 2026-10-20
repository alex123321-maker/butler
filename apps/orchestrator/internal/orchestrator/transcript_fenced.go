package orchestrator

import (
	"context"
	"fmt"
	"github.com/butler/butler/internal/memory/transcript"
	"github.com/butler/butler/internal/ownership"
)

// Keep alternate transcript adapters fenced as well as PostgreSQL stores.
type fencedTranscript struct{ TranscriptStore }

func (s fencedTranscript) GetTaskTranscript(ctx context.Context, sessionKey, taskID string) (transcript.Transcript, error) {
	r, ok := s.TranscriptStore.(interface {
		GetTaskTranscript(context.Context, string, string) (transcript.Transcript, error)
	})
	if !ok {
		return transcript.Transcript{}, fmt.Errorf("task transcript reader unavailable")
	}
	return r.GetTaskTranscript(ctx, sessionKey, taskID)
}

func (s fencedTranscript) AppendMessage(ctx context.Context, m transcript.Message) (transcript.Message, error) {
	ctx, release, err := ownership.Begin(ctx)
	if err != nil {
		return transcript.Message{}, err
	}
	defer release()
	return s.TranscriptStore.AppendMessage(ctx, m)
}
func (s fencedTranscript) AppendToolCall(ctx context.Context, m transcript.ToolCall) (transcript.ToolCall, error) {
	ctx, release, err := ownership.Begin(ctx)
	if err != nil {
		return transcript.ToolCall{}, err
	}
	defer release()
	return s.TranscriptStore.AppendToolCall(ctx, m)
}
