package app

// Research-only wall-clock accounting. Provider wait includes network and remote
// processing, not an inferred duration of hidden reasoning. Tool RPC time is a
// subset of Butler time; overlapping subspans must never be added twice.
import (
	"context"
	"github.com/butler/butler/apps/orchestrator/internal/tools"
	toolbrokerv1 "github.com/butler/butler/internal/gen/toolbroker/v1"
	"sync"
	"time"
)

type utilityTimingSpan struct {
	Kind        string `json:"kind"`
	Name        string `json:"name"`
	Nanoseconds int64  `json:"nanoseconds"`
}
type utilityTimings struct {
	mu    sync.Mutex
	spans []utilityTimingSpan
}

func (t *utilityTimings) add(kind, name string, elapsed time.Duration) {
	if t == nil {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	t.spans = append(t.spans, utilityTimingSpan{kind, name, elapsed.Nanoseconds()})
}
func (t *utilityTimings) snapshot() []utilityTimingSpan {
	t.mu.Lock()
	defer t.mu.Unlock()
	return append([]utilityTimingSpan{}, t.spans...)
}

type utilityTimedTools struct {
	*tools.BrokerClient
	timing *utilityTimings
}

func (b utilityTimedTools) ExecuteToolCall(ctx context.Context, c *toolbrokerv1.ToolCall) (*toolbrokerv1.ToolResult, error) {
	start := time.Now()
	defer func() { b.timing.add("tool_rpc", c.GetToolName(), time.Since(start)) }()
	return b.BrokerClient.ExecuteToolCall(ctx, c)
}

func (b utilityTimedTools) ExecuteAuthorizedToolCall(ctx context.Context, c *toolbrokerv1.ToolCall, grant string) (*toolbrokerv1.ToolResult, error) {
	start := time.Now()
	defer func() { b.timing.add("tool_rpc", c.GetToolName(), time.Since(start)) }()
	return b.BrokerClient.ExecuteAuthorizedToolCall(ctx, c, grant)
}
