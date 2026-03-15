package orchestrator

import (
	"context"
	"testing"
	"time"
)

func TestApprovalGateWaitAndResolveApproved(t *testing.T) {
	t.Parallel()

	gate := NewApprovalGate()
	go func() {
		time.Sleep(10 * time.Millisecond)
		gate.Resolve("call-1", true)
	}()

	resp, err := gate.Wait(context.Background(), "call-1")
	if err != nil {
		t.Fatalf("Wait returned error: %v", err)
	}
	if !resp.Approved {
		t.Fatal("expected approval to be true")
	}
	if resp.ToolCallID != "call-1" {
		t.Fatalf("expected tool call id call-1, got %q", resp.ToolCallID)
	}
}

func TestApprovalGateWaitAndResolveRejected(t *testing.T) {
	t.Parallel()

	gate := NewApprovalGate()
	go func() {
		time.Sleep(10 * time.Millisecond)
		gate.Resolve("call-2", false)
	}()

	resp, err := gate.Wait(context.Background(), "call-2")
	if err != nil {
		t.Fatalf("Wait returned error: %v", err)
	}
	if resp.Approved {
		t.Fatal("expected approval to be false")
	}
}

func TestApprovalGateWaitContextCancelled(t *testing.T) {
	t.Parallel()

	gate := NewApprovalGate()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()

	_, err := gate.Wait(ctx, "call-3")
	if err == nil {
		t.Fatal("expected error when context is cancelled")
	}
}

func TestApprovalGateResolveWithoutPending(t *testing.T) {
	t.Parallel()

	gate := NewApprovalGate()
	if gate.Resolve("nonexistent", true) {
		t.Fatal("expected Resolve to return false for nonexistent tool call")
	}
}

func TestApprovalGateCleanupAfterWait(t *testing.T) {
	t.Parallel()

	gate := NewApprovalGate()
	go func() {
		time.Sleep(10 * time.Millisecond)
		gate.Resolve("call-4", true)
	}()

	_, err := gate.Wait(context.Background(), "call-4")
	if err != nil {
		t.Fatalf("Wait returned error: %v", err)
	}

	// After Wait returns, the pending entry should be cleaned up.
	if gate.Resolve("call-4", true) {
		t.Fatal("expected Resolve to return false after Wait completed")
	}
}
