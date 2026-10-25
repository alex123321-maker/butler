package input

import (
	"fmt"
	"github.com/butler/butler/internal/transport"
	"strings"
	"testing"
)

func TestExecutionFailureRetainsFiniteProviderCauseWithoutDetails(t *testing.T) {
	for _, kind := range []transport.ErrorType{transport.ErrorTypeProviderUnavailable, transport.ErrorTypeRateLimited, transport.ErrorTypeMemorySourceStale, "PRIVATE_UNKNOWN"} {
		failure := fmt.Errorf("PRIVATE_PREFIX: %w", &transport.Error{Type: kind, Message: "PRIVATE_MESSAGE", ProviderDetails: map[string]any{"token": "PRIVATE_TOKEN"}})
		code, message := executionFailure(failure)
		if code != transport.DiagnosticCode(failure) || message != code || strings.Contains(code+message, "PRIVATE") {
			t.Fatal(code, message)
		}
	}
}
