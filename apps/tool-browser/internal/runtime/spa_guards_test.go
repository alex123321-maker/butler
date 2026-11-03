package runtime

import (
	"context"
	"testing"

	"github.com/butler/butler/internal/browserspa"
)

func TestSPARawRuntimeCannotBypassResourceWrapper(t *testing.T) {
	for _, enabled := range []bool{false, true} {
		s := NewServer(&stubRunner{}, nil)
		s.SPAEnabled = enabled
		req := browserCall(browserspa.Tool, "run-1", map[string]any{"url": "https://example.com/"})
		req.Contract = browserspa.Contract()
		response, err := s.Execute(context.Background(), req)
		if err != nil || response.GetResult().GetStatus() == "completed" {
			t.Fatal("raw SPA execution bypassed running authority")
		}
	}
}
