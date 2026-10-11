package transport

import (
	"errors"
	"fmt"
	"strings"
	"testing"
)

func TestDiagnosticCodeExcludesUntrustedProviderMaterial(t *testing.T) {
	const private = "PRIVATE_DIAGNOSTIC_MARKER"
	for _, kind := range []ErrorType{ErrorTypeProviderUnavailable, ErrorTypeProviderTimeout,
		ErrorTypeProviderProtocolError, ErrorTypeRateLimited, ErrorTypeMemorySourceStale,
		ErrorTypeMemorySourceTimeout, ErrorTypeInputBytesExceeded, ErrorType(private)} {
		err := fmt.Errorf("%s: %w", private, &Error{Type: kind, Message: private,
			ProviderName: private, ProviderCode: private, ProviderDetails: map[string]any{"token": private}})
		got := DiagnosticCode(err)
		want := string(kind)
		if kind == ErrorType(private) {
			want = string(ErrorTypeInternalTransportError)
		}
		if got != want || strings.Contains(got, private) {
			t.Fatalf("diagnostic = %q, want %q", got, want)
		}
	}
	var empty *Error
	for _, err := range []error{nil, errors.New(private), empty} {
		if got := DiagnosticCode(err); got != "" {
			t.Fatalf("non-provider diagnostic = %q", got)
		}
	}
}
