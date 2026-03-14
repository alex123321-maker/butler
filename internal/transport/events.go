package transport

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"time"
)

type EventType string

const (
	EventTypeRunStarted             EventType = "run_started"
	EventTypeProviderSessionBound   EventType = "provider_session_bound"
	EventTypeAssistantDelta         EventType = "assistant_delta"
	EventTypeAssistantFinal         EventType = "assistant_final"
	EventTypeToolCallRequested      EventType = "tool_call_requested"
	EventTypeToolCallBatchRequested EventType = "tool_call_batch_requested"
	EventTypeTransportWarning       EventType = "transport_warning"
	EventTypeTransportError         EventType = "transport_error"
	EventTypeRunCancelled           EventType = "run_cancelled"
	EventTypeRunTimedOut            EventType = "run_timed_out"
	EventTypeRunCompleted           EventType = "run_completed"
)

type AssistantDelta struct {
	DeltaType  string
	Content    string
	SequenceNo int
}

type Usage struct {
	InputTokens  int
	OutputTokens int
	TotalTokens  int
}

type AssistantFinal struct {
	Content      string
	FinishReason string
	Usage        *Usage
}

type ToolCallRequest struct {
	ToolCallRef string
	ToolName    string
	ArgsJSON    string
	SequenceNo  int
}

type ToolCallBatch struct {
	ToolCalls  []ToolCallRequest
	BatchRef   string
	SequenceNo int
}

type TransportEvent struct {
	EventID              string
	RunID                string
	EventType            EventType
	ProviderName         string
	PayloadJSON          string
	Timestamp            time.Time
	ProviderSessionRef   *ProviderSessionRef
	CapabilitiesSnapshot *CapabilitySnapshot
	AssistantDelta       *AssistantDelta
	AssistantFinal       *AssistantFinal
	ToolCall             *ToolCallRequest
	ToolCallBatch        *ToolCallBatch
	TransportError       *Error
}

func NewEvent(runID string, eventType EventType, providerName string) TransportEvent {
	return TransportEvent{
		EventID:      newEventID(),
		RunID:        runID,
		EventType:    eventType,
		ProviderName: providerName,
		Timestamp:    time.Now().UTC(),
	}
}

func NewRunStartedEvent(runID, providerName string, capabilities CapabilitySnapshot, sessionRef *ProviderSessionRef) TransportEvent {
	event := NewEvent(runID, EventTypeRunStarted, providerName)
	event.CapabilitiesSnapshot = &capabilities
	event.ProviderSessionRef = sessionRef
	event.PayloadJSON = mustJSON(map[string]any{
		"provider_session_ref":  sessionRef,
		"capabilities_snapshot": capabilities,
	})
	return event
}

func NewProviderSessionBoundEvent(runID, providerName string, sessionRef ProviderSessionRef) TransportEvent {
	event := NewEvent(runID, EventTypeProviderSessionBound, providerName)
	event.ProviderSessionRef = &sessionRef
	event.PayloadJSON = mustJSON(map[string]any{"provider_session_ref": sessionRef})
	return event
}

func NewAssistantDeltaEvent(runID, providerName string, delta AssistantDelta) TransportEvent {
	event := NewEvent(runID, EventTypeAssistantDelta, providerName)
	event.AssistantDelta = &delta
	event.PayloadJSON = mustJSON(delta)
	return event
}

func NewAssistantFinalEvent(runID, providerName string, final AssistantFinal) TransportEvent {
	event := NewEvent(runID, EventTypeAssistantFinal, providerName)
	event.AssistantFinal = &final
	event.PayloadJSON = mustJSON(final)
	return event
}

func NewToolCallRequestedEvent(runID, providerName string, toolCall ToolCallRequest) TransportEvent {
	event := NewEvent(runID, EventTypeToolCallRequested, providerName)
	event.ToolCall = &toolCall
	event.PayloadJSON = mustJSON(toolCall)
	return event
}

func NewToolCallBatchRequestedEvent(runID, providerName string, batch ToolCallBatch) TransportEvent {
	event := NewEvent(runID, EventTypeToolCallBatchRequested, providerName)
	event.ToolCallBatch = &batch
	event.PayloadJSON = mustJSON(batch)
	return event
}

func NewTransportWarningEvent(runID, providerName, message string, details map[string]any) TransportEvent {
	event := NewEvent(runID, EventTypeTransportWarning, providerName)
	event.PayloadJSON = mustJSON(map[string]any{"message": message, "details": details})
	return event
}

func NewTransportErrorEvent(runID, providerName string, err error) TransportEvent {
	event := NewEvent(runID, EventTypeTransportError, providerName)
	normalized := NormalizeError(err, providerName)
	event.TransportError = normalized
	event.PayloadJSON = mustJSON(normalized)
	return event
}

func NewTerminalEvent(runID string, eventType EventType, providerName string) TransportEvent {
	return NewEvent(runID, eventType, providerName)
}

func newEventID() string {
	var raw [6]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return fmt.Sprintf("transport-event-%d", time.Now().UTC().UnixNano())
	}
	return fmt.Sprintf("transport-event-%d-%s", time.Now().UTC().UnixNano(), hex.EncodeToString(raw[:]))
}

func mustJSON(value any) string {
	encoded, err := json.Marshal(value)
	if err != nil {
		return "{}"
	}
	return string(encoded)
}
