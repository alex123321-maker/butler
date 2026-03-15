package orchestrator

import (
	"context"
	"fmt"
	"sync"
)

// ApprovalChecker determines whether a tool requires human approval before execution.
type ApprovalChecker interface {
	RequiresApproval(ctx context.Context, toolName string) (bool, error)
}

// ApprovalGate manages pending approval requests, allowing the orchestrator to
// block on user decisions while channel adapters resolve them asynchronously.
type ApprovalGate struct {
	mu      sync.Mutex
	pending map[string]chan ApprovalResponse
}

// NewApprovalGate creates a new ApprovalGate.
func NewApprovalGate() *ApprovalGate {
	return &ApprovalGate{pending: make(map[string]chan ApprovalResponse)}
}

// Wait registers a pending approval for the given tool call ID and blocks
// until the approval is resolved or the context is cancelled.
func (g *ApprovalGate) Wait(ctx context.Context, toolCallID string) (ApprovalResponse, error) {
	ch := make(chan ApprovalResponse, 1)
	g.mu.Lock()
	g.pending[toolCallID] = ch
	g.mu.Unlock()

	defer func() {
		g.mu.Lock()
		delete(g.pending, toolCallID)
		g.mu.Unlock()
	}()

	select {
	case resp := <-ch:
		return resp, nil
	case <-ctx.Done():
		return ApprovalResponse{}, fmt.Errorf("approval wait cancelled: %w", ctx.Err())
	}
}

// Resolve delivers an approval decision for the given tool call ID.
// Returns false if no pending approval was found.
func (g *ApprovalGate) Resolve(toolCallID string, approved bool) bool {
	g.mu.Lock()
	ch, ok := g.pending[toolCallID]
	g.mu.Unlock()
	if !ok {
		return false
	}
	select {
	case ch <- ApprovalResponse{ToolCallID: toolCallID, Approved: approved}:
		return true
	default:
		return false
	}
}
