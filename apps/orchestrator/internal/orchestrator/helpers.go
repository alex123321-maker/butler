package orchestrator

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"hash/fnv"
	"strings"
	"time"

	"github.com/butler/butler/apps/orchestrator/internal/observability"
	"github.com/butler/butler/apps/orchestrator/internal/session"
	"github.com/butler/butler/internal/domain"
	"github.com/butler/butler/internal/domain/convert"
	commonv1 "github.com/butler/butler/internal/gen/common/v1"
	runv1 "github.com/butler/butler/internal/gen/run/v1"
	sessionv1 "github.com/butler/butler/internal/gen/session/v1"
	toolbrokerv1 "github.com/butler/butler/internal/gen/toolbroker/v1"
	"github.com/butler/butler/internal/ingress"
	memoryservice "github.com/butler/butler/internal/memory/service"
	"github.com/butler/butler/internal/memory/transcript"
	"github.com/butler/butler/internal/transport"
)

// --- Input extraction helpers ---

func extractUserMessage(payload map[string]any) string {
	for _, key := range []string{"text", "message", "content", "prompt"} {
		if value := strings.TrimSpace(stringFromAny(payload[key])); value != "" {
			return value
		}
	}
	if message, ok := payload["message"].(map[string]any); ok {
		for _, key := range []string{"text", "content"} {
			if value := strings.TrimSpace(stringFromAny(message[key])); value != "" {
				return value
			}
		}
	}
	return ""
}

func firstString(payload map[string]any, keys ...string) string {
	for _, key := range keys {
		if value := strings.TrimSpace(stringFromAny(payload[key])); value != "" {
			return value
		}
	}
	return ""
}

func stringFromAny(value any) string {
	switch typed := value.(type) {
	case string:
		return typed
	default:
		return ""
	}
}

// --- JSON helpers ---

func decodeJSONValue(raw string, fallback any) any {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" || !json.Valid([]byte(trimmed)) {
		return fallback
	}
	var decoded any
	if err := json.Unmarshal([]byte(trimmed), &decoded); err != nil {
		return fallback
	}
	return decoded
}

func decodeJSONObject(raw string) map[string]any {
	decoded := decodeJSONValue(raw, map[string]any{})
	object, ok := decoded.(map[string]any)
	if !ok {
		return map[string]any{}
	}
	return object
}

func isEmptyJSONValue(value any) bool {
	switch typed := value.(type) {
	case nil:
		return true
	case map[string]any:
		return len(typed) == 0
	case []any:
		return len(typed) == 0
	case []string:
		return len(typed) == 0
	case string:
		return strings.TrimSpace(typed) == ""
	default:
		return false
	}
}

func formatJSONValueForPrompt(value any) string {
	if isEmptyJSONValue(value) {
		return ""
	}
	encoded, err := json.Marshal(value)
	if err != nil {
		return ""
	}
	return string(encoded)
}

func mustMarshalJSON(value any) string {
	encoded, err := json.Marshal(value)
	if err != nil {
		return "{}"
	}
	return string(encoded)
}

func normalizeJSON(value, fallback string) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" || !json.Valid([]byte(trimmed)) {
		return fallback
	}
	return trimmed
}

// --- Prompt context helpers ---

func workingContextToPromptContext(value any) memoryservice.WorkingContext {
	workingMap, ok := value.(map[string]any)
	if !ok {
		return memoryservice.WorkingContext{Status: "idle"}
	}
	return memoryservice.WorkingContext{
		Goal:           stringFromAny(workingMap["goal"]),
		ActiveEntities: workingMap["active_entities"],
		PendingSteps:   workingMap["pending_steps"],
		Status:         stringFromAny(workingMap["working_status"]),
	}
}

func sliceOfMaps(value any) []map[string]any {
	items, ok := value.([]map[string]any)
	if ok {
		return items
	}
	rawItems, ok := value.([]any)
	if !ok {
		return nil
	}
	result := make([]map[string]any, 0, len(rawItems))
	for _, item := range rawItems {
		entry, ok := item.(map[string]any)
		if !ok {
			continue
		}
		result = append(result, entry)
	}
	return result
}

func compactSentence(value string) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return ""
	}
	if len(trimmed) > 120 {
		trimmed = strings.TrimSpace(trimmed[:117]) + "..."
	}
	return trimmed
}

// --- Tool result helpers ---

func toolResultEnvelope(result *toolbrokerv1.ToolResult) string {
	payload := map[string]any{"status": normalizeToolStatus(result.GetStatus())}
	if json.Valid([]byte(result.GetResultJson())) {
		payload["result"] = json.RawMessage(result.GetResultJson())
	}
	if result.GetError() != nil {
		payload["error"] = map[string]any{"error_class": result.GetError().GetErrorClass().String(), "message": result.GetError().GetMessage(), "retryable": result.GetError().GetRetryable(), "details_json": result.GetError().GetDetailsJson()}
	}
	return mustMarshalJSON(payload)
}

func toolResultPayload(result *toolbrokerv1.ToolResult) string {
	return normalizeJSON(result.GetResultJson(), "{}")
}

func toolErrorPayload(toolErr *toolbrokerv1.ToolError) string {
	if toolErr == nil {
		return "{}"
	}
	payload := map[string]any{"error_class": toolErr.GetErrorClass().String(), "message": toolErr.GetMessage(), "retryable": toolErr.GetRetryable()}
	if details := strings.TrimSpace(toolErr.GetDetailsJson()); details != "" {
		if json.Valid([]byte(details)) {
			payload["details"] = json.RawMessage(details)
		} else {
			payload["details"] = details
		}
	}
	return mustMarshalJSON(payload)
}

func normalizeToolStatus(value string) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return "completed"
	}
	return trimmed
}

func fallbackToolCallID(runID string, now time.Time) string {
	h := fnv.New64a()
	_, _ = h.Write([]byte(runID))
	_, _ = h.Write([]byte(now.Format(time.RFC3339Nano)))
	return fmt.Sprintf("tool-%016x", h.Sum64())
}

// transcriptToolCallFromExecution builds a transcript.ToolCall from execution context.
func transcriptToolCallFromExecution(toolCallID, runID string, requested *transport.ToolCallRequest, brokerCall *toolbrokerv1.ToolCall, result *toolbrokerv1.ToolResult, startedAt, finishedAt time.Time) transcript.ToolCall {
	finished := finishedAt
	return transcript.ToolCall{
		ToolCallID:    toolCallID,
		RunID:         runID,
		ToolName:      requested.ToolName,
		ArgsJSON:      normalizeJSON(requested.ArgsJSON, "{}"),
		Status:        normalizeToolStatus(result.GetStatus()),
		RuntimeTarget: brokerCall.GetRuntimeTarget(),
		StartedAt:     startedAt,
		FinishedAt:    &finished,
		ResultJSON:    toolResultPayload(result),
		ErrorJSON:     toolErrorPayload(result.GetError()),
	}
}

// --- Provider session ref helpers ---

func providerSessionRefFromRun(runRecord *sessionv1.RunRecord) *transport.ProviderSessionRef {
	if runRecord == nil {
		return nil
	}
	ref, err := transport.ParseProviderSessionRef(runRecord.GetProviderSessionRef())
	if err != nil {
		return nil
	}
	return ref
}

// --- Transport error helpers ---

func transportError(err *transport.Error) error {
	if err == nil {
		return errors.New("transport error")
	}
	return err
}

// --- Error classification ---

// toErrorClass converts a domain ErrorClass string to its proto enum equivalent.
func toErrorClass(value string) commonv1.ErrorClass {
	return convert.ErrorClassStringToProto(strings.TrimSpace(value))
}

// classifyExecutionError maps a runtime error to a domain error class string.
func classifyExecutionError(err error) string {
	switch {
	case err == nil:
		return ""
	case errors.Is(err, context.DeadlineExceeded):
		return string(domain.ErrorClassTimeout)
	case errors.Is(err, context.Canceled):
		return string(domain.ErrorClassCancelled)
	case errors.Is(err, errLeaseRenewalFailed), errors.Is(err, session.ErrLeaseConflict), errors.Is(err, session.ErrLeaseNotFound):
		return string(domain.ErrorClassInternal)
	}
	var transportErr *transport.Error
	if errors.As(err, &transportErr) {
		return string(domain.ErrorClassTransport)
	}
	return string(domain.ErrorClassInternal)
}

// --- Validation ---

func validateInputEvent(event ingress.InputEvent) error {
	if strings.TrimSpace(event.EventID) == "" {
		return fmt.Errorf("event_id is required")
	}
	if strings.TrimSpace(event.SessionKey) == "" {
		return fmt.Errorf("session_key is required")
	}
	if strings.TrimSpace(event.Source) == "" {
		return fmt.Errorf("source is required")
	}
	if event.EventType == runv1.InputEventType_INPUT_EVENT_TYPE_UNSPECIFIED {
		return fmt.Errorf("event_type is required")
	}
	return nil
}

// --- Observability helpers ---

// runStateString converts a proto RunState to a human-readable lowercase string.
func runStateString(state commonv1.RunState) string {
	domainState, err := convert.ProtoToRunState(state)
	if err != nil {
		return "unknown"
	}
	return string(domainState)
}

// emitEvent publishes an observability event if the EventHub is configured.
// This is non-blocking and fire-and-forget — it never blocks the run.
func (s *Service) emitEvent(runID, sessionKey, eventType string, payload map[string]any) {
	event := observability.NewEvent(runID, sessionKey, eventType, payload)
	if s.config.Activity != nil {
		_ = s.config.Activity.RecordFromObservabilityEvent(context.Background(), event)
	}
	if s.config.EventHub == nil {
		return
	}
	s.config.EventHub.Publish(runID, event)
}

// truncateForObservability shortens a string for safe observability payloads.
func truncateForObservability(value string, maxLen int) string {
	if len(value) <= maxLen {
		return value
	}
	return value[:maxLen] + "..."
}

// scheduleHubCleanup fires a goroutine that waits briefly for SSE clients to
// drain terminal events, then calls CleanupRun to free the subscriber map.
// This is fire-and-forget and never blocks the run.
func (s *Service) scheduleHubCleanup(runID string) {
	if s.config.EventHub == nil {
		return
	}
	go func() {
		time.Sleep(5 * time.Second)
		s.config.EventHub.CleanupRun(runID)
	}()
}
