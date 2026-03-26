package api

import (
	"context"
	"testing"
	"time"
)

func TestExtensionBindRelayDispatchAndResolve_GlobalQueue(t *testing.T) {
	t.Parallel()

	relay := NewExtensionBindRelay()
	dispatchDone := make(chan ExtensionBindResolveResult, 1)
	dispatchErr := make(chan error, 1)
	go func() {
		result, err := relay.Dispatch(context.Background(), ExtensionBindDispatchParams{
			RunID:      "run-1",
			SessionKey: "telegram:chat:1",
			ToolCallID: "call-bind-1",
		})
		if err != nil {
			dispatchErr <- err
			return
		}
		dispatchDone <- result
	}()

	pollCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	pending, ok, err := relay.PollNext(pollCtx, "browser-1")
	if err != nil {
		t.Fatalf("PollNext returned error: %v", err)
	}
	if !ok {
		t.Fatal("expected pending bind dispatch")
	}
	if pending.RunID != "run-1" {
		t.Fatalf("unexpected run_id: %s", pending.RunID)
	}

	if err := relay.ResolveSuccess(pending.DispatchID, "browser-1", ExtensionBindResolveResult{
		BrowserInstanceID: "browser-1",
		BrowserHint:       "Chrome",
		TabCandidates: []createBindCandidateEntry{{
			InternalTabRef: "17",
			Title:          "Docs",
			Domain:         "docs.example.com",
			CurrentURL:     "https://docs.example.com",
		}},
	}); err != nil {
		t.Fatalf("ResolveSuccess returned error: %v", err)
	}

	select {
	case err := <-dispatchErr:
		t.Fatalf("Dispatch failed: %v", err)
	case result := <-dispatchDone:
		if result.BrowserInstanceID != "browser-1" {
			t.Fatalf("unexpected browser_instance_id %q", result.BrowserInstanceID)
		}
		if len(result.TabCandidates) != 1 {
			t.Fatalf("expected one tab candidate, got %d", len(result.TabCandidates))
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for dispatch result")
	}
}

func TestExtensionBindRelayTargetedBrowserQueue(t *testing.T) {
	t.Parallel()

	relay := NewExtensionBindRelay()
	dispatchDone := make(chan ExtensionBindResolveResult, 1)
	dispatchErr := make(chan error, 1)
	go func() {
		result, err := relay.Dispatch(context.Background(), ExtensionBindDispatchParams{
			RunID:             "run-2",
			SessionKey:        "telegram:chat:2",
			ToolCallID:        "call-bind-2",
			BrowserInstanceID: "browser-target",
		})
		if err != nil {
			dispatchErr <- err
			return
		}
		dispatchDone <- result
	}()

	pollCtx, cancel := context.WithTimeout(context.Background(), 600*time.Millisecond)
	defer cancel()
	_, ok, err := relay.PollNext(pollCtx, "browser-other")
	if err != nil {
		t.Fatalf("PollNext for other browser returned error: %v", err)
	}
	if ok {
		t.Fatal("expected no pending dispatch for non-target browser")
	}

	pollCtx2, cancel2 := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel2()
	pending, ok, err := relay.PollNext(pollCtx2, "browser-target")
	if err != nil {
		t.Fatalf("PollNext for target browser returned error: %v", err)
	}
	if !ok {
		t.Fatal("expected pending dispatch for target browser")
	}
	if pending.PreferredBrowserID != "browser-target" {
		t.Fatalf("unexpected preferred_browser_instance_id: %q", pending.PreferredBrowserID)
	}

	if err := relay.ResolveSuccess(pending.DispatchID, "browser-target", ExtensionBindResolveResult{
		BrowserInstanceID: "browser-target",
		BrowserHint:       "Chrome",
		TabCandidates: []createBindCandidateEntry{{
			InternalTabRef: "42",
			Title:          "Inbox",
			Domain:         "mail.example.com",
			CurrentURL:     "https://mail.example.com/inbox",
		}},
	}); err != nil {
		t.Fatalf("ResolveSuccess returned error: %v", err)
	}

	select {
	case err := <-dispatchErr:
		t.Fatalf("Dispatch failed: %v", err)
	case result := <-dispatchDone:
		if result.BrowserInstanceID != "browser-target" {
			t.Fatalf("unexpected browser_instance_id %q", result.BrowserInstanceID)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for targeted dispatch result")
	}
}
