package observability

import (
	"sync"
)

const defaultSubscriberBuffer = 128

// Hub is an in-process pub/sub hub for live observability events, keyed by run ID.
// Subscribers receive events on a buffered channel. If the channel is full, the event
// is dropped (non-blocking) to prevent blocking the orchestrator hot path.
type Hub struct {
	mu          sync.RWMutex
	subscribers map[string]map[*subscriber]struct{}
}

type subscriber struct {
	ch     chan Event
	closed bool
}

// NewHub creates a new Hub.
func NewHub() *Hub {
	return &Hub{
		subscribers: make(map[string]map[*subscriber]struct{}),
	}
}

// Subscribe returns a channel that receives observability events for the given run ID,
// and a cancel function that must be called to unsubscribe.
func (h *Hub) Subscribe(runID string) (<-chan Event, func()) {
	sub := &subscriber{
		ch: make(chan Event, defaultSubscriberBuffer),
	}

	h.mu.Lock()
	if h.subscribers[runID] == nil {
		h.subscribers[runID] = make(map[*subscriber]struct{})
	}
	h.subscribers[runID][sub] = struct{}{}
	h.mu.Unlock()

	cancel := func() {
		h.mu.Lock()
		defer h.mu.Unlock()
		if subs, ok := h.subscribers[runID]; ok {
			delete(subs, sub)
			if len(subs) == 0 {
				delete(h.subscribers, runID)
			}
		}
		if !sub.closed {
			sub.closed = true
			close(sub.ch)
		}
	}

	return sub.ch, cancel
}

// Publish sends an event to all subscribers for the given run ID.
// Non-blocking: if a subscriber's buffer is full, the event is dropped for that subscriber.
func (h *Hub) Publish(runID string, event Event) {
	h.mu.RLock()
	subs := h.subscribers[runID]
	h.mu.RUnlock()

	for sub := range subs {
		select {
		case sub.ch <- event:
		default:
			// subscriber buffer full, drop event to avoid blocking orchestrator
		}
	}
}

// SubscriberCount returns the number of active subscribers for a run ID.
func (h *Hub) SubscriberCount(runID string) int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.subscribers[runID])
}

// CleanupRun removes all subscriber entries for a run. Useful for terminal run states.
func (h *Hub) CleanupRun(runID string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if subs, ok := h.subscribers[runID]; ok {
		for sub := range subs {
			if !sub.closed {
				sub.closed = true
				close(sub.ch)
			}
		}
		delete(h.subscribers, runID)
	}
}
