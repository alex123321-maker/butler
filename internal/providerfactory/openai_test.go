package providerfactory

import (
	"context"
	"github.com/butler/butler/internal/transport/openai"
	"testing"
	"time"
)

func TestOpenAIProfileUsesOrdinaryFactory(t *testing.T) {
	result, err := New(nil, nil).Build(context.Background(), BuildConfig{ActiveProvider: "openai", OpenAIModel: "gpt-6-luna", OpenAIReasoningEffort: "high", OpenAIMaximumInputTokens: 65536, OpenAIMaximumOutputTokens: 8192, Timeout: 90 * time.Second})
	if err != nil || result.ProviderName != "openai" || result.ModelName != "gpt-6-luna" {
		t.Fatal("profile construction failed", err)
	}
	p, ok := result.Provider.(*openai.Provider)
	if !ok {
		t.Fatal("not the normal OpenAI provider")
	}
	defer p.CloseAllSessions()
	in, out := p.InferenceLimits()
	if in != 65536 || out != 8192 || p.InferenceTimeout() != 90*time.Second {
		t.Fatal("profile limits changed")
	}
	if _, err = New(nil, nil).Build(context.Background(), BuildConfig{ActiveProvider: "openai", OpenAIReasoningEffort: "unsupported"}); err == nil {
		t.Fatal("invalid profile accepted")
	}
}
