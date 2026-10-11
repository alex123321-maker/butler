package transport

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strings"
)

type ErrorType string

const (
	ErrorTypeProviderUnavailable      ErrorType = "provider_unavailable"
	ErrorTypeTransportConnectionError ErrorType = "transport_connection_error"
	ErrorTypeProviderTimeout          ErrorType = "provider_timeout"
	ErrorTypeProviderProtocolError    ErrorType = "provider_protocol_error"
	ErrorTypeCapabilityMismatch       ErrorType = "capability_mismatch"
	ErrorTypeInvalidToolRequest       ErrorType = "invalid_tool_request"
	ErrorTypeStatefulSessionLost      ErrorType = "stateful_session_lost"
	ErrorTypeRateLimited              ErrorType = "rate_limited"
	ErrorTypeModelCapacityUnavailable ErrorType = "model_capacity_unavailable"
	ErrorTypeInternalTransportError   ErrorType = "internal_transport_error"
	ErrorTypeMemorySourceStale        ErrorType = "memory_source_stale"
	ErrorTypeMemorySourceTimeout      ErrorType = "memory_source_check_timeout"
)

// DiagnosticCode projects an execution failure onto a fixed, content-free
// vocabulary. Error messages, provider codes/details and unknown type strings
// must never become durable diagnostics or exported acceptance evidence.
func DiagnosticCode(err error) string {
	var failure *Error
	if !errors.As(err, &failure) || failure == nil {
		return ""
	}
	switch failure.Type {
	case ErrorTypeProviderUnavailable, ErrorTypeTransportConnectionError,
		ErrorTypeProviderTimeout, ErrorTypeProviderProtocolError,
		ErrorTypeCapabilityMismatch, ErrorTypeInvalidToolRequest,
		ErrorTypeStatefulSessionLost, ErrorTypeRateLimited, ErrorTypeModelCapacityUnavailable,
		ErrorTypeInternalTransportError, ErrorTypeInputBytesExceeded,
		ErrorTypeMemorySourceStale, ErrorTypeMemorySourceTimeout:
		return string(failure.Type)
	default:
		return string(ErrorTypeInternalTransportError)
	}
}

type Error struct {
	Type            ErrorType
	Message         string
	Retryable       bool
	ProviderName    string
	ProviderCode    string
	ProviderDetails map[string]any
}

func (e *Error) Error() string {
	if e == nil {
		return ""
	}
	if strings.TrimSpace(e.Message) == "" {
		return string(e.Type)
	}
	return e.Message
}

func (e *Error) Is(target error) bool {
	t, ok := target.(*Error)
	if !ok {
		return false
	}
	if t.Type != "" && e.Type != t.Type {
		return false
	}
	if t.ProviderName != "" && e.ProviderName != t.ProviderName {
		return false
	}
	return true
}

type HTTPStatusError struct {
	StatusCode int
	Message    string
	Code       string
	Details    map[string]any
}

func (e *HTTPStatusError) Error() string {
	if e == nil {
		return ""
	}
	if strings.TrimSpace(e.Message) != "" {
		return e.Message
	}
	return fmt.Sprintf("http status %d", e.StatusCode)
}

func NormalizeError(err error, providerName string) *Error {
	if err == nil {
		return nil
	}
	var transportErr *Error
	if errors.As(err, &transportErr) {
		if transportErr.ProviderName == "" {
			transportErr.ProviderName = providerName
		}
		if transportErr.Message == "" {
			transportErr.Message = err.Error()
		}
		return transportErr
	}

	result := &Error{
		Type:         ErrorTypeInternalTransportError,
		Message:      "provider operation failed",
		Retryable:    false,
		ProviderName: providerName,
	}

	switch {
	case errors.Is(err, context.DeadlineExceeded):
		result.Type = ErrorTypeProviderTimeout
		result.Retryable = true
		return result
	case errors.Is(err, context.Canceled):
		result.Type = ErrorTypeTransportConnectionError
		return result
	}

	var httpErr *HTTPStatusError
	if errors.As(err, &httpErr) {
		// HTTP error text, codes and details originate in an untrusted response.
		// Status is the complete safe diagnostic contract.
		result.Message = fmt.Sprintf("provider returned HTTP %d", httpErr.StatusCode)
		switch httpErr.StatusCode {
		case http.StatusTooManyRequests:
			result.Type = ErrorTypeRateLimited
			result.Retryable = true
		case http.StatusRequestTimeout, http.StatusGatewayTimeout:
			result.Type = ErrorTypeProviderTimeout
			result.Retryable = true
		case http.StatusBadRequest, http.StatusUnprocessableEntity:
			result.Type = ErrorTypeProviderProtocolError
		case http.StatusServiceUnavailable, http.StatusBadGateway:
			result.Type = ErrorTypeProviderUnavailable
			result.Retryable = true
		default:
			if httpErr.StatusCode >= 500 {
				result.Type = ErrorTypeProviderUnavailable
				result.Retryable = true
			} else {
				result.Type = ErrorTypeProviderProtocolError
			}
		}
		return result
	}

	var netErr net.Error
	if errors.As(err, &netErr) {
		result.Type = ErrorTypeTransportConnectionError
		result.Retryable = true
		if netErr.Timeout() {
			result.Type = ErrorTypeProviderTimeout
		}
		return result
	}

	return result
}
