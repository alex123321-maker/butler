package api

import (
	"context"
	"testing"
	"time"
)

func TestExtensionRelayDispatchResolveSuccess(t *testing.T) {
	t.Parallel()

	relay := NewExtensionActionRelay()
	done := make(chan struct{})

	go func() {
		defer close(done)
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		pending, ok, err := relay.PollNext(ctx, "telegram:chat:1")
		if err != nil || !ok {
			t.Errorf("expected pending dispatch, got ok=%v err=%v", ok, err)
			return
		}
		if pending.ActionType != "navigate" {
			t.Errorf("unexpected action type: %s", pending.ActionType)
			return
		}
		if err := relay.ResolveSuccess(pending.DispatchID, "", map[string]any{"session_status": "ACTIVE", "result_json": `{"ok":true}`}); err != nil {
			t.Errorf("resolve success returned error: %v", err)
		}
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	result, err := relay.Dispatch(ctx, "telegram:chat:1", ExtensionDispatchParams{
		SingleTabSessionID: "single-tab-1",
		BoundTabRef:        "17",
		ActionType:         "navigate",
		ArgsJSON:           `{"url":"https://example.com"}`,
	})
	if err != nil {
		t.Fatalf("dispatch returned error: %v", err)
	}
	if result["session_status"] != "ACTIVE" {
		t.Fatalf("unexpected dispatch result: %+v", result)
	}

	<-done
}

func TestExtensionRelayDispatchTimesOutWithoutPoller(t *testing.T) {
	t.Parallel()

	relay := NewExtensionActionRelay()
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	_, err := relay.Dispatch(ctx, "telegram:chat:2", ExtensionDispatchParams{
		SingleTabSessionID: "single-tab-2",
		BoundTabRef:        "18",
		ActionType:         "status",
	})
	if err == nil {
		t.Fatal("expected timeout relay error")
	}
	relayErr, ok := err.(*ExtensionRelayError)
	if !ok {
		t.Fatalf("expected ExtensionRelayError, got %T", err)
	}
	if relayErr.Code != "host_unavailable" {
		t.Fatalf("expected host_unavailable code, got %s", relayErr.Code)
	}
}

func TestExtensionRelayResolveErrorForUnknownDispatch(t *testing.T) {
	t.Parallel()

	relay := NewExtensionActionRelay()
	err := relay.ResolveError("unknown-dispatch", "", "runtime_error", "failed")
	if err != ErrExtensionDispatchNotFound {
		t.Fatalf("expected ErrExtensionDispatchNotFound, got %v", err)
	}
}

func TestExtensionRelayResolveRejectsWrongBrowserInstance(t *testing.T) {
	t.Parallel()

	relay := NewExtensionActionRelay()
	done := make(chan struct{})

	go func() {
		defer close(done)
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		pending, ok, err := relay.PollNext(ctx, "telegram:chat:3")
		if err != nil || !ok {
			t.Errorf("expected pending dispatch, got ok=%v err=%v", ok, err)
			return
		}
		if err := relay.ResolveSuccess(pending.DispatchID, "browser-b", map[string]any{"ok": true}); err != ErrExtensionDispatchNotFound {
			t.Errorf("expected ErrExtensionDispatchNotFound on wrong browser instance, got %v", err)
			return
		}
		if err := relay.ResolveSuccess(pending.DispatchID, "browser-a", map[string]any{"session_status": "ACTIVE"}); err != nil {
			t.Errorf("expected resolve success for matching browser instance, got %v", err)
		}
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	result, err := relay.Dispatch(ctx, "telegram:chat:3", ExtensionDispatchParams{
		SingleTabSessionID: "single-tab-3",
		BoundTabRef:        "33",
		ActionType:         "status",
		BrowserInstanceID:  "browser-a",
	})
	if err != nil {
		t.Fatalf("dispatch returned error: %v", err)
	}
	if result["session_status"] != "ACTIVE" {
		t.Fatalf("unexpected result payload: %+v", result)
	}

	<-done
}
