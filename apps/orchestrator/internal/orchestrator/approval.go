package orchestrator

import (
	"context"
	"fmt"
	"sync"
	"time"
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
// until the approval is resolved or the context is cancelled. A default
// TTL of 15 minutes is applied to prevent infinite blocking.
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

	timeoutCtx, cancel := context.WithTimeout(ctx, 15*time.Minute)
	defer cancel()

	select {
	case resp := <-ch:
		return resp, nil
	case <-timeoutCtx.Done():
		return ApprovalResponse{}, fmt.Errorf("approval wait cancelled or timed out: %w", timeoutCtx.Err())
	}
}

// Resolve delivers an approval decision for the given tool call ID.
// Returns false if no pending approval was found.
func (g *ApprovalGate) Resolve(toolCallID string, approved bool) bool {
	return g.ResolveWithChannel(toolCallID, approved, "unknown", "")
}

// ResolveWithChannel delivers an approval decision and resolution metadata.
// Returns false if no pending approval was found.
func (g *ApprovalGate) ResolveWithChannel(toolCallID string, approved bool, channel, resolvedBy string) bool {
	g.mu.Lock()
	ch, ok := g.pending[toolCallID]
	g.mu.Unlock()
	if !ok {
		return false
	}
	if channel == "" {
		channel = "unknown"
	}
	select {
	case ch <- ApprovalResponse{ToolCallID: toolCallID, Approved: approved, Channel: channel, ResolvedBy: resolvedBy}:
		return true
	default:
		return false
	}
}
