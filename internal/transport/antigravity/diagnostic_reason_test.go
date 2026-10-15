package antigravity

import (
	"context"
	"encoding/json"
	"github.com/butler/butler/internal/transport"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
)

func TestHTTPInvalidArgumentReasonIsBoundedMaskedAndNeverRetries(t *testing.T) {
	for _, tc := range []struct{ message, status, want string }{{`PRIVATE_ACCOUNT Unknown name "minItems" at SECRET_PATH`, `INVALID_ARGUMENT`, "unsupported_schema_min_items"}, {`PRIVATE_ACCOUNT responseSchema SECRET_PATH`, `INVALID_ARGUMENT`, "response_schema_rejected"}, {`PRIVATE_ACCOUNT`, `INVALID_ARGUMENT`, "upstream_invalid_argument"}, {strings.Repeat("PRIVATE", 12000), `INVALID_ARGUMENT`, ""}, {`PRIVATE_ACCOUNT Unknown name "minItems"`, `PRIVATE_STATUS`, ""}} {
		var calls atomic.Int32
		raw, _ := json.Marshal(map[string]any{"error": map[string]string{"status": tc.status, "message": tc.message}})
		p := newFixture(t, func(*http.Request) (*http.Response, error) {
			calls.Add(1)
			return &http.Response{StatusCode: 400, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(string(raw)))}, nil
		})
		stream, err := p.StartRun(context.Background(), request("run"))
		if err != nil {
			t.Fatal(err)
		}
		failure := findError(collect(t, stream))
		if failure == nil || calls.Load() != 1 || failure.Retryable || transport.DiagnosticReason(failure) != tc.want || transport.DiagnosticHTTPStatus(failure) != 400 || strings.Contains(failure.Message, "PRIVATE") || strings.Contains(failure.Message, "SECRET") || failure.ProviderDetails != nil {
			t.Fatal("safe provider diagnostic contract violated")
		}
	}
	if got := transport.DiagnosticReason(&transport.Error{SafeReason: "PRIVATE_INJECTED"}); got != "" {
		t.Fatal("arbitrary reason exported")
	}
}
