package orchestrator

import (
	"context"
	runv1 "github.com/butler/butler/internal/gen/run/v1"
	"github.com/butler/butler/internal/ingress"
	memoryservice "github.com/butler/butler/internal/memory/service"
	"github.com/butler/butler/internal/metrics"
	"github.com/butler/butler/internal/transport"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestOrdinaryRunUsesCompositionMemoryRegistry(t *testing.T) {
	m := metrics.New()
	provider := &mockProvider{startEvents: []transport.TransportEvent{transport.NewAssistantFinalEvent("", "openai", transport.AssistantFinal{Content: "observed", FinishReason: "completed"})}}
	s := newFixtureService(&memorySessionRepo{}, &memoryLeaseManager{}, newMemoryRunManager(), &memoryTranscriptStore{}, provider, Config{Metrics: m, ProfileStore: stubProfileStore{entries: []memoryservice.ProfileEntry{stubProfileEntry{key: "language", summary: "Russian"}}}}, nil)
	_, err := s.Execute(context.Background(), ingress.InputEvent{EventID: "metrics-request", EventType: runv1.InputEventType_INPUT_EVENT_TYPE_USER_MESSAGE, SessionKey: "web:metrics-fixture", Source: "web", PayloadJSON: `{"text":"continue"}`, CreatedAt: time.Now(), IdempotencyKey: "metrics-request"})
	if err != nil {
		t.Fatal(err)
	}
	w := httptest.NewRecorder()
	m.Handler().ServeHTTP(w, httptest.NewRequest("GET", "/metrics", nil))
	out := w.Body.String()
	for _, want := range []string{`butler_memory_operations_total{outcome="success",stage="bundle"} 1`, `butler_memory_operation_duration_seconds_count{stage="context"} 1`, `butler_memory_retrievals_total{memory_type="profile",service="memory-service",status="hit"} 1`} {
		if !strings.Contains(out, want) {
			t.Fatalf("ordinary request did not expose %s", want)
		}
	}
}
