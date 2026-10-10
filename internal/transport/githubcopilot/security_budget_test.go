package githubcopilot

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/butler/butler/internal/transport"
)

func TestCopilotFullConversationCannotExceedReleaseBound(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls.Add(1) }))
	defer server.Close()
	provider, _ := NewProvider(Config{AuthSource: staticCopilotAuth{baseURL: server.URL}}, server.Client())
	_, err := provider.StartRun(transport.WithInputByteLimit(context.Background(), 8), transport.StartRunRequest{Context: transport.TransportRunContext{RunID: "run", SessionKey: "session", ProviderName: providerName, ModelName: "gpt-4o"}, InputItems: []transport.InputItem{{Role: "user", Content: "private observation"}}, StreamingEnabled: true})
	if err == nil || calls.Load() != 0 {
		t.Fatal("oversized retained conversation sent HTTP")
	}
}
