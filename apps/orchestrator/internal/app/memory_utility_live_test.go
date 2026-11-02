package app

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/butler/butler/internal/transport"
)

func utilityPlainText(e transport.TransportEvent) bool {
	return e.EventType == transport.EventTypeAssistantDelta && e.AssistantDelta != nil && e.AssistantDelta.DeltaType == "text" && e.ToolCall == nil && e.ToolCallBatch == nil && e.AssistantFinal == nil && e.TransportError == nil && e.Usage == nil
}

func TestMemoryUtilityLiveRetentionFailureReleasesNoAction(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	s := utilityLiveCapture(ctx, "run", "fixture", func(func(transport.TransportEvent)) (transport.EventStream, error) {
		return nil, errors.New("retention failed")
	})
	for e := range s {
		if e.EventType != transport.EventTypeTransportError || e.RunID != "run" || e.ToolCall != nil || e.AssistantFinal != nil {
			t.Fatal("failed retention released action")
		}
	}
	mixed := transport.NewEvent("run", transport.EventTypeAssistantDelta, "fixture")
	mixed.AssistantDelta = &transport.AssistantDelta{DeltaType: "text", Content: "mixed"}
	mixed.ToolCall = &transport.ToolCallRequest{ToolName: "fixture"}
	if utilityPlainText(mixed) {
		t.Fatal("mixed action treated as provisional text")
	}
}

// Text is provisional. Every other event, especially tool/final/usage, is held
// until the complete native method receipt has reached PostgreSQL and fsync.
func utilityLiveCapture(ctx context.Context, run, provider string, record func(func(transport.TransportEvent)) (transport.EventStream, error)) transport.EventStream {
	out := make(chan transport.TransportEvent, 32)
	go func() {
		defer close(out)
		send := func(e transport.TransportEvent) {
			select {
			case out <- e:
			case <-ctx.Done():
			}
		}
		stream, err := record(send)
		if err != nil {
			send(transport.NewTransportErrorEvent(run, provider, err))
			return
		}
		for e := range stream {
			if !utilityPlainText(e) {
				send(e)
			}
		}
	}()
	return out
}

func TestMemoryUtilityLiveTextBeforeReceiptAndActionsAfter(t *testing.T) {
	retained := make(chan struct{})
	text := transport.NewEvent("run", transport.EventTypeAssistantDelta, "fixture")
	text.AssistantDelta = &transport.AssistantDelta{DeltaType: "text", Content: "provisional"}
	tool := transport.NewEvent("run", transport.EventTypeToolCallRequested, "fixture")
	tool.ToolCall = &transport.ToolCallRequest{ToolName: "fixture"}
	s := utilityLiveCapture(context.Background(), "run", "fixture", func(send func(transport.TransportEvent)) (transport.EventStream, error) {
		send(text)
		<-retained
		out := make(chan transport.TransportEvent, 2)
		out <- text
		out <- tool
		close(out)
		return out, nil
	})
	select {
	case e := <-s:
		if e.AssistantDelta == nil {
			t.Fatal("text held")
		}
	case <-time.After(time.Second):
		t.Fatal("text buffered")
	}
	select {
	case <-s:
		t.Fatal("action before durable receipt")
	default:
	}
	close(retained)
	select {
	case e := <-s:
		if e.ToolCall == nil {
			t.Fatal("duplicate text or missing action")
		}
	case <-time.After(time.Second):
		t.Fatal("action lost")
	}
	if _, ok := <-s; ok {
		t.Fatal("duplicate event")
	}
}
