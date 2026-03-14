package transport

import (
	"context"
	"errors"
	"io"
	"net"
	"testing"
	"time"
)

func TestNewRunStartedEvent(t *testing.T) {
	t.Parallel()

	capabilities := CapabilitySnapshot{
		SupportsStreaming:        true,
		SupportsToolCalls:        true,
		SupportsBatchToolCalls:   true,
		SupportsStatefulSessions: true,
		SupportsCancel:           true,
	}
	ref := &ProviderSessionRef{ProviderName: "openai", SessionRef: "sess_123", ResponseRef: "resp_123"}

	event := NewRunStartedEvent("run-123", "openai", capabilities, ref)

	if event.EventType != EventTypeRunStarted {
		t.Fatalf("expected run_started, got %s", event.EventType)
	}
	if event.RunID != "run-123" {
		t.Fatalf("expected run id to be preserved")
	}
	if event.CapabilitiesSnapshot == nil || !event.CapabilitiesSnapshot.SupportsStreaming {
		t.Fatalf("expected capabilities snapshot on event")
	}
	if event.ProviderSessionRef == nil || event.ProviderSessionRef.SessionRef != "sess_123" {
		t.Fatalf("expected provider session ref on event")
	}
	if event.PayloadJSON == "" {
		t.Fatalf("expected payload json to be populated")
	}
	if event.Timestamp.IsZero() {
		t.Fatalf("expected timestamp to be set")
	}
	if event.EventID == "" {
		t.Fatalf("expected event id to be set")
	}
}

func TestNewTransportErrorEventNormalizesError(t *testing.T) {
	t.Parallel()

	event := NewTransportErrorEvent("run-123", "openai", context.DeadlineExceeded)

	if event.EventType != EventTypeTransportError {
		t.Fatalf("expected transport_error, got %s", event.EventType)
	}
	if event.TransportError == nil {
		t.Fatalf("expected normalized transport error")
	}
	if event.TransportError.Type != ErrorTypeProviderTimeout {
		t.Fatalf("expected provider timeout, got %s", event.TransportError.Type)
	}
	if !event.TransportError.Retryable {
		t.Fatalf("expected timeout to be retryable")
	}
}

func TestNormalizeError(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		err       error
		wantType  ErrorType
		retryable bool
	}{
		{name: "timeout", err: context.DeadlineExceeded, wantType: ErrorTypeProviderTimeout, retryable: true},
		{name: "rate limit", err: &HTTPStatusError{StatusCode: 429, Message: "rate limited", Code: "too_many_requests"}, wantType: ErrorTypeRateLimited, retryable: true},
		{name: "service unavailable", err: &HTTPStatusError{StatusCode: 503, Message: "provider unavailable"}, wantType: ErrorTypeProviderUnavailable, retryable: true},
		{name: "bad request", err: &HTTPStatusError{StatusCode: 400, Message: "bad request"}, wantType: ErrorTypeProviderProtocolError, retryable: false},
		{name: "network", err: io.EOF, wantType: ErrorTypeInternalTransportError, retryable: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			normalized := NormalizeError(tt.err, "openai")
			if normalized.Type != tt.wantType {
				t.Fatalf("expected %s, got %s", tt.wantType, normalized.Type)
			}
			if normalized.Retryable != tt.retryable {
				t.Fatalf("expected retryable=%t, got %t", tt.retryable, normalized.Retryable)
			}
			if normalized.ProviderName != "openai" {
				t.Fatalf("expected provider name to be preserved")
			}
		})
	}
}

func TestNormalizeErrorHandlesNetworkErrors(t *testing.T) {
	t.Parallel()

	err := NormalizeError(&net.DNSError{IsTimeout: true, Err: "i/o timeout"}, "openai")
	if err.Type != ErrorTypeProviderTimeout {
		t.Fatalf("expected network timeout to map to provider timeout, got %s", err.Type)
	}
	if !err.Retryable {
		t.Fatalf("expected network timeout to be retryable")
	}
}

func TestRequestValidation(t *testing.T) {
	t.Parallel()

	req := StartRunRequest{
		Context: TransportRunContext{
			RunID:        "run-123",
			SessionKey:   "telegram:chat:1",
			ProviderName: "openai",
			ModelName:    "gpt-5-mini",
		},
		TransportOptionsJSON: `{"stream":true}`,
	}

	if err := req.Validate(); err != nil {
		t.Fatalf("expected valid request, got %v", err)
	}

	invalid := SubmitToolResultRequest{RunID: "run-123", ToolCallRef: "tool_123", ToolResultJSON: "not-json"}
	if err := invalid.Validate(); err == nil {
		t.Fatalf("expected tool result validation error")
	}
}

func TestErrorIs(t *testing.T) {
	t.Parallel()

	err := &Error{Type: ErrorTypeProviderUnavailable, ProviderName: "openai", Message: "provider down"}
	if !errors.Is(err, &Error{Type: ErrorTypeProviderUnavailable}) {
		t.Fatalf("expected errors.Is to match transport error type")
	}
	if errors.Is(err, &Error{Type: ErrorTypeRateLimited}) {
		t.Fatalf("expected errors.Is not to match different error type")
	}

	if got := err.Error(); got != "provider down" {
		t.Fatalf("unexpected error string %q", got)
	}

	blank := &Error{Type: ErrorTypeCapabilityMismatch}
	if blank.Error() != string(ErrorTypeCapabilityMismatch) {
		t.Fatalf("expected fallback error string")
	}

	if NewTerminalEvent("run-123", EventTypeRunCompleted, "openai").Timestamp.After(time.Now().UTC().Add(time.Second)) {
		t.Fatalf("unexpected terminal event timestamp")
	}
}

func TestProviderSessionRefRoundTrip(t *testing.T) {
	t.Parallel()

	ref := &ProviderSessionRef{ProviderName: "openai", ResponseRef: "resp_123"}
	encoded, err := MarshalProviderSessionRef(ref)
	if err != nil {
		t.Fatalf("MarshalProviderSessionRef returned error: %v", err)
	}
	parsed, err := ParseProviderSessionRef(encoded)
	if err != nil {
		t.Fatalf("ParseProviderSessionRef returned error: %v", err)
	}
	if parsed == nil || parsed.ResponseRef != "resp_123" {
		t.Fatalf("expected provider session ref round-trip, got %+v", parsed)
	}

	blank, err := ParseProviderSessionRef("")
	if err != nil {
		t.Fatalf("unexpected parse error for blank value: %v", err)
	}
	if blank != nil {
		t.Fatalf("expected nil provider session ref for blank value")
	}
}
