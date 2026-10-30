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
	// Optional separately observed reasoning count. Nil is unavailable, not zero.
	ThinkingTokens *int `json:",omitempty"`
	// OpenAI reasoning is already included in OutputTokens; cached input in InputTokens.
	OutputReasoningTokens *int `json:",omitempty"`
	CachedInputTokens     *int `json:",omitempty"`
	CacheWriteInputTokens *int `json:",omitempty"`
	CacheWrite30mTokens   *int `json:",omitempty"`
	CacheWrite12hTokens   *int `json:",omitempty"`
}

// Equal compares observed values, including unavailable versus explicit zero.
// Independently decoded identical counters must not conflict by pointer identity.
func (u Usage) Equal(other Usage) bool {
	if u.InputTokens != other.InputTokens || u.OutputTokens != other.OutputTokens || u.TotalTokens != other.TotalTokens {
		return false
	}
	pairs := [][2]*int{{u.ThinkingTokens, other.ThinkingTokens}, {u.OutputReasoningTokens, other.OutputReasoningTokens}, {u.CachedInputTokens, other.CachedInputTokens}, {u.CacheWriteInputTokens, other.CacheWriteInputTokens}, {u.CacheWrite30mTokens, other.CacheWrite30mTokens}, {u.CacheWrite12hTokens, other.CacheWrite12hTokens}}
	for _, pair := range pairs {
		if (pair[0] == nil) != (pair[1] == nil) {
			return false
		}
		if pair[0] != nil && *pair[0] != *pair[1] {
			return false
		}
	}
	return true
}

// Validate accepts nonnegative observed counts without overflowing a sum. A
// reported nonzero total may include provider reasoning/cached dimensions, but
// cannot be less than the separately reported input/output subtotal. Zero total
// remains compatible with adapters that report only the two measured dimensions.
func (u Usage) Validate() error {
	maxInt := int(^uint(0) >> 1)
	if u.InputTokens < 0 || u.OutputTokens < 0 || u.TotalTokens < 0 || u.InputTokens > maxInt-u.OutputTokens {
		return fmt.Errorf("invalid observed token usage")
	}
	if u.TotalTokens > 0 && u.TotalTokens < u.InputTokens+u.OutputTokens {
		return fmt.Errorf("invalid observed token total")
	}
	if u.ThinkingTokens != nil && (*u.ThinkingTokens < 0 || *u.ThinkingTokens > u.TotalTokens-u.InputTokens-u.OutputTokens) {
		return fmt.Errorf("invalid observed thinking usage")
	}
	if u.OutputReasoningTokens != nil && (*u.OutputReasoningTokens < 0 || *u.OutputReasoningTokens > u.OutputTokens) {
		return fmt.Errorf("invalid observed output reasoning usage")
	}
	if u.CachedInputTokens != nil && (*u.CachedInputTokens < 0 || *u.CachedInputTokens > u.InputTokens) {
		return fmt.Errorf("invalid observed cached input usage")
	}
	cached := 0
	if u.CachedInputTokens != nil {
		cached = *u.CachedInputTokens
	}
	if u.CacheWriteInputTokens != nil && (*u.CacheWriteInputTokens < 0 || *u.CacheWriteInputTokens > u.InputTokens-cached) {
		return fmt.Errorf("invalid observed cache write usage")
	}
	for _, count := range []*int{u.CacheWrite30mTokens, u.CacheWrite12hTokens} {
		if count != nil && (*count < 0 || u.CacheWriteInputTokens == nil || *count > *u.CacheWriteInputTokens) {
			return fmt.Errorf("invalid observed cache lifetime usage")
		}
	}
	if u.CacheWrite30mTokens != nil && u.CacheWrite12hTokens != nil && *u.CacheWrite30mTokens != *u.CacheWriteInputTokens-*u.CacheWrite12hTokens {
		return fmt.Errorf("invalid observed cache lifetime total")
	}
	return nil
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
	// Usage is the complete observation for one admitted inference request,
	// independent of whether that turn requests tools or returns final text.
	// It is never a per-tool allowance or permission to run another turn.
	Usage *Usage
	// ProviderModelVersion is optional observed version metadata, not a claim
	// about proprietary weight bytes or authority to change the selected model.
	ProviderModelVersion *string
	// Optional observed serving tier, separate from requested operator pricing.
	ProviderServiceTier *string
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
	event.Usage = final.Usage
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
