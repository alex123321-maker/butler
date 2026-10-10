package orchestrator

import (
	"context"
	"github.com/butler/butler/internal/memory/transcript"
	"github.com/butler/butler/internal/ownership"
)

// Keep alternate transcript adapters fenced as well as PostgreSQL stores.
type fencedTranscript struct{ TranscriptStore }

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
