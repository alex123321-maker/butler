package orchestrator

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"testing"

	commonv1 "github.com/butler/butler/internal/gen/common/v1"
	sessionv1 "github.com/butler/butler/internal/gen/session/v1"
	"github.com/butler/butler/internal/transport"
)

func TestFailedRunPersistsSafeTransportSubtype(t *testing.T) {
	const private = "PRIVATE_DIAGNOSTIC_MARKER"
	for _, kind := range []transport.ErrorType{transport.ErrorTypeProviderTimeout,
		transport.ErrorTypeRateLimited, transport.ErrorTypeMemorySourceTimeout, transport.ErrorType(private)} {
		t.Run(string(kind), func(t *testing.T) {
			runs := newMemoryRunManager()
			current := &sessionv1.RunRecord{RunId: "diagnostic-run", SessionKey: "web:fixture", CurrentState: commonv1.RunState_RUN_STATE_MODEL_RUNNING}
			runs.records[current.RunId] = current
			var logs bytes.Buffer
			log := slog.New(slog.NewJSONHandler(&logs, nil))
			service := newFixtureService(&memorySessionRepo{}, &memoryLeaseManager{}, runs, &memoryTranscriptStore{}, &mockProvider{}, Config{}, log)
			original := fmt.Errorf("%s: %w", private, &transport.Error{Type: kind, Message: private, ProviderDetails: map[string]any{"secret": private}})
			got := service.failRun(context.Background(), current, "fixture-lease", log, original)
			want := transport.DiagnosticCode(original)
			var typed *transport.Error
			if !errors.As(got, &typed) || got.Error() != want || current.ErrorMessage != want || current.CurrentState != commonv1.RunState_RUN_STATE_FAILED {
				t.Fatal("failed run did not persist/return safe subtype")
			}
			if strings.Contains(logs.String(), private) {
				t.Fatal("provider material leaked into execution logs")
			}
		})
	}
}
