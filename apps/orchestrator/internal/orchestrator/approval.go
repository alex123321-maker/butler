package orchestrator

import (
	"context"
	"fmt"
	"strings"
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
	pending map[string]*PendingApproval
}

// PendingApproval is registered before a request becomes visible to the user.
// It accepts one decision and must be closed on every exit from the tool flow.
type PendingApproval struct {
	gate       *ApprovalGate
	toolCallID string
	ctx        context.Context
	cancel     context.CancelFunc
	response   chan ApprovalResponse
	resolved   bool // guarded by gate.mu
}

// NewApprovalGate creates a new ApprovalGate.
func NewApprovalGate() *ApprovalGate {
	return &ApprovalGate{pending: make(map[string]*PendingApproval)}
}

func (g *ApprovalGate) Register(ctx context.Context, toolCallID string) (*PendingApproval, error) {
	if strings.TrimSpace(toolCallID) == "" {
		return nil, fmt.Errorf("tool call ID is required")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	if _, exists := g.pending[toolCallID]; exists {
		return nil, fmt.Errorf("approval already registered for tool call %s", toolCallID)
	}
	waitCtx, cancel := context.WithTimeout(ctx, 15*time.Minute)
	waiter := &PendingApproval{gate: g, toolCallID: toolCallID, ctx: waitCtx, cancel: cancel, response: make(chan ApprovalResponse, 1)}
	g.pending[toolCallID] = waiter
	return waiter, nil
}

func (p *PendingApproval) Close() {
	p.gate.mu.Lock()
	defer p.gate.mu.Unlock()
	p.cancel()
	if p.gate.pending[p.toolCallID] == p {
		delete(p.gate.pending, p.toolCallID)
	}
}

func (p *PendingApproval) Wait() (ApprovalResponse, error) {
	select {
	case resp := <-p.response:
		// Cancellation/expiry wins over a buffered decision.
		if err := p.ctx.Err(); err != nil {
			return ApprovalResponse{}, fmt.Errorf("approval wait cancelled or timed out: %w", err)
		}
		return resp, nil
	case <-p.ctx.Done():
		return ApprovalResponse{}, fmt.Errorf("approval wait cancelled or timed out: %w", p.ctx.Err())
	}
}

// Wait registers a pending approval for the given tool call ID and blocks
// until the approval is resolved or the context is cancelled. A default
// TTL of 15 minutes is applied to prevent infinite blocking.
func (g *ApprovalGate) Wait(ctx context.Context, toolCallID string) (ApprovalResponse, error) {
	waiter, err := g.Register(ctx, toolCallID)
	if err != nil {
		return ApprovalResponse{}, err
	}
	defer waiter.Close()
	return waiter.Wait()
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
	defer g.mu.Unlock()
	waiter, ok := g.pending[toolCallID]
	if !ok || waiter.resolved || waiter.ctx.Err() != nil {
		return false
	}
	if channel == "" {
		channel = "unknown"
	}
	waiter.resolved = true
	waiter.response <- ApprovalResponse{ToolCallID: toolCallID, Approved: approved, Channel: channel, ResolvedBy: resolvedBy}
	return true
}
