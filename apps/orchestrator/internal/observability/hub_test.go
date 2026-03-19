package observability

import (
	"sync"
	"testing"
	"time"
)

func TestHub_SubscribeAndPublish(t *testing.T) {
	hub := NewHub()

	ch, cancel := hub.Subscribe("run-1")
	defer cancel()

	evt := NewEvent("run-1", "session-1", EventStateTransition, map[string]any{"from": "a", "to": "b"})
	hub.Publish("run-1", evt)

	select {
	case received := <-ch:
		if received.RunID != "run-1" {
			t.Errorf("expected run_id run-1, got %s", received.RunID)
		}
		if received.EventType != EventStateTransition {
			t.Errorf("expected event_type state_transition, got %s", received.EventType)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for event")
	}
}

func TestHub_PublishToWrongRunID(t *testing.T) {
	hub := NewHub()

	ch, cancel := hub.Subscribe("run-1")
	defer cancel()

	// Publish to a different run.
	hub.Publish("run-2", NewEvent("run-2", "s", EventRunCompleted, nil))

	select {
	case evt := <-ch:
		t.Errorf("should not receive event for different run, got %+v", evt)
	case <-time.After(50 * time.Millisecond):
		// expected — no event received
	}
}

func TestHub_MultipleSubscribers(t *testing.T) {
	hub := NewHub()

	ch1, cancel1 := hub.Subscribe("run-1")
	defer cancel1()
	ch2, cancel2 := hub.Subscribe("run-1")
	defer cancel2()

	if hub.SubscriberCount("run-1") != 2 {
		t.Errorf("expected 2 subscribers, got %d", hub.SubscriberCount("run-1"))
	}

	hub.Publish("run-1", NewEvent("run-1", "s", EventToolStarted, nil))

	for _, ch := range []<-chan Event{ch1, ch2} {
		select {
		case evt := <-ch:
			if evt.EventType != EventToolStarted {
				t.Errorf("expected tool_started, got %s", evt.EventType)
			}
		case <-time.After(time.Second):
			t.Fatal("timed out")
		}
	}
}

func TestHub_CancelUnsubscribes(t *testing.T) {
	hub := NewHub()

	_, cancel := hub.Subscribe("run-1")

	if hub.SubscriberCount("run-1") != 1 {
		t.Fatalf("expected 1 subscriber, got %d", hub.SubscriberCount("run-1"))
	}

	cancel()

	if hub.SubscriberCount("run-1") != 0 {
		t.Errorf("expected 0 subscribers after cancel, got %d", hub.SubscriberCount("run-1"))
	}
}

func TestHub_CleanupRun(t *testing.T) {
	hub := NewHub()

	ch, _ := hub.Subscribe("run-1")

	hub.CleanupRun("run-1")

	if hub.SubscriberCount("run-1") != 0 {
		t.Errorf("expected 0 subscribers after cleanup, got %d", hub.SubscriberCount("run-1"))
	}

	// Channel should be closed.
	select {
	case _, ok := <-ch:
		if ok {
			t.Error("expected channel to be closed")
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for channel close")
	}
}

func TestHub_NonBlockingPublish(t *testing.T) {
	hub := NewHub()

	ch, cancel := hub.Subscribe("run-1")
	defer cancel()

	// Fill the buffer.
	for i := 0; i < defaultSubscriberBuffer+10; i++ {
		hub.Publish("run-1", NewEvent("run-1", "s", EventAssistantDelta, map[string]any{"i": i}))
	}

	// Should have buffer-full events, not panic or block.
	count := 0
	for {
		select {
		case <-ch:
			count++
		default:
			goto done
		}
	}
done:
	if count != defaultSubscriberBuffer {
		t.Errorf("expected %d buffered events, got %d", defaultSubscriberBuffer, count)
	}
}

func TestHub_ConcurrentPublish(t *testing.T) {
	hub := NewHub()

	ch, cancel := hub.Subscribe("run-1")
	defer cancel()

	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 10; j++ {
				hub.Publish("run-1", NewEvent("run-1", "s", EventStateTransition, nil))
			}
		}()
	}
	wg.Wait()

	// Drain and count — should have received up to buffer size events without panic.
	count := 0
	for {
		select {
		case <-ch:
			count++
		default:
			goto done2
		}
	}
done2:
	if count == 0 {
		t.Error("expected some events from concurrent publish")
	}
}

func TestHub_CleanupThenCancelNoPanic(t *testing.T) {
	hub := NewHub()

	_, cancel := hub.Subscribe("run-1")

	// CleanupRun closes all subscriber channels for run-1.
	hub.CleanupRun("run-1")

	// Calling cancel after CleanupRun already closed the channel must not panic.
	cancel()

	if hub.SubscriberCount("run-1") != 0 {
		t.Errorf("expected 0 subscribers, got %d", hub.SubscriberCount("run-1"))
	}
}

func TestHub_DoubleCancelNoPanic(t *testing.T) {
	hub := NewHub()

	_, cancel := hub.Subscribe("run-1")

	cancel()
	cancel() // second call must not panic
}

func TestNewEvent_IDGeneration(t *testing.T) {
	evt1 := NewEvent("r1", "s1", EventRunCompleted, nil)
	evt2 := NewEvent("r1", "s1", EventRunCompleted, nil)

	if evt1.EventID == "" {
		t.Error("expected non-empty event ID")
	}
	if evt1.EventID == evt2.EventID {
		t.Error("expected unique event IDs")
	}
	if evt1.Timestamp.IsZero() {
		t.Error("expected non-zero timestamp")
	}
}
