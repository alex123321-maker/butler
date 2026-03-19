package observability

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"time"
)

// Event type constants for observability events.
const (
	EventStateTransition   = "state_transition"
	EventMemoryLoaded      = "memory_loaded"
	EventToolsLoaded       = "tools_loaded"
	EventPromptAssembled   = "prompt_assembled"
	EventAssistantDelta    = "assistant_delta"
	EventAssistantFinal    = "assistant_final"
	EventToolStarted       = "tool_started"
	EventToolCompleted     = "tool_completed"
	EventApprovalRequested = "approval_requested"
	EventApprovalResolved  = "approval_resolved"
	EventRunError          = "run_error"
	EventRunCompleted      = "run_completed"
)

// Event represents a single observability event emitted during a run.
type Event struct {
	EventID    string         `json:"event_id"`
	RunID      string         `json:"run_id"`
	SessionKey string         `json:"session_key"`
	EventType  string         `json:"event_type"`
	Timestamp  time.Time      `json:"timestamp"`
	Payload    map[string]any `json:"payload,omitempty"`
}

// MarshalJSON produces the JSON representation of the event.
func (e Event) MarshalJSON() ([]byte, error) {
	type alias Event
	return json.Marshal(alias(e))
}

// NewEvent creates a new observability event with a generated ID and current timestamp.
func NewEvent(runID, sessionKey, eventType string, payload map[string]any) Event {
	return Event{
		EventID:    newObsEventID(),
		RunID:      runID,
		SessionKey: sessionKey,
		EventType:  eventType,
		Timestamp:  time.Now().UTC(),
		Payload:    payload,
	}
}

func newObsEventID() string {
	var raw [6]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return fmt.Sprintf("obs-%d", time.Now().UTC().UnixNano())
	}
	return fmt.Sprintf("obs-%d-%s", time.Now().UTC().UnixNano(), hex.EncodeToString(raw[:]))
}
