package bridge

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/butler/butler/apps/browser-bridge/internal/protocol"
)

func TestDispatcherDispatchAction(t *testing.T) {
	t.Parallel()

	var dispatcher *Dispatcher
	dispatcher = NewDispatcher(func(request protocol.Request) error {
		go dispatcherResolve(dispatcher, protocol.Response{
			ID: request.ID,
			OK: true,
			Result: map[string]any{
				"single_tab_session_id": "single-tab-1",
				"session_status":        "ACTIVE",
				"result_json":           `{"ok":true}`,
				"current_url":           "https://example.com",
				"current_title":         "Example",
			},
		})
		return nil
	})

	result, err := dispatcher.DispatchAction(context.Background(), protocol.ActionDispatchParams{
		SingleTabSessionID: "single-tab-1",
		BoundTabRef:        "12",
		ActionType:         "navigate",
		ArgsJSON:           `{"url":"https://example.com"}`,
	})
	if err != nil {
		t.Fatalf("DispatchAction returned error: %v", err)
	}
	if result.CurrentURL != "https://example.com" {
		t.Fatalf("expected current url to round-trip, got %q", result.CurrentURL)
	}
}

func TestDispatcherDispatchActionReturnsNativeError(t *testing.T) {
	t.Parallel()

	var dispatcher *Dispatcher
	dispatcher = NewDispatcher(func(request protocol.Request) error {
		go dispatcherResolve(dispatcher, protocol.Response{
			ID: request.ID,
			OK: false,
			Error: &protocol.ErrorPayload{
				Code:    "tab_closed",
				Message: "tab is closed",
			},
		})
		return nil
	})

	_, err := dispatcher.DispatchAction(context.Background(), protocol.ActionDispatchParams{
		SingleTabSessionID: "single-tab-2",
		BoundTabRef:        "14",
		ActionType:         "click",
	})
	var dispatchErr *DispatchError
	if !errors.As(err, &dispatchErr) {
		t.Fatalf("expected DispatchError, got %v", err)
	}
	if dispatchErr.Code != "tab_closed" {
		t.Fatalf("expected tab_closed code, got %q", dispatchErr.Code)
	}
}

func TestDispatcherDispatchActionWithoutNativeClient(t *testing.T) {
	t.Parallel()

	dispatcher := NewDispatcher(nil)
	_, err := dispatcher.DispatchAction(context.Background(), protocol.ActionDispatchParams{})
	if !errors.Is(err, ErrNoNativeClient) {
		t.Fatalf("expected ErrNoNativeClient, got %v", err)
	}
}

func dispatcherResolve(dispatcher *Dispatcher, response protocol.Response) {
	time.Sleep(5 * time.Millisecond)
	dispatcher.Resolve(response)
}
