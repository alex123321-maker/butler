package orchestrator

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/butler/butler/apps/orchestrator/internal/task"
	runv1 "github.com/butler/butler/internal/gen/run/v1"
	sessionv1 "github.com/butler/butler/internal/gen/session/v1"
	toolbrokerv1 "github.com/butler/butler/internal/gen/toolbroker/v1"
	"github.com/butler/butler/internal/ingress"
	"github.com/butler/butler/internal/security/content"
	"github.com/butler/butler/internal/transport"
)

func TestInitialSkillSelectionRejectsInvalidWholeSetBeforeDispatch(t *testing.T) {
	for _, digests := range [][]string{{"invalid"}, {strings.Repeat("a", 64), strings.Repeat("a", 64)}, {strings.Repeat("g", 64)}, {strings.Repeat("a", 64), "invalid"}, make([]string, 5)} {
		s := &Service{config: Config{Skills: instructionSkillCatalog{}, InitialSkillSelection: func(context.Context, string, string) ([]string, error) { return digests, nil }}}
		current := &sessionv1.RunRecord{RunId: "run"}
		p := &preparedRun{Task: &task.Record{TaskID: "task"}}
		if _, err := s.loadInitialSkills(context.Background(), nil, current, "lease", p); err == nil {
			t.Fatal("invalid selection admitted")
		}
	}
}

func TestSkillCompactContextKeepsExactNativeSchemasAndOwner(t *testing.T) {
	provider := &mockProvider{startEvents: []transport.TransportEvent{
		transport.NewAssistantFinalEvent("", "openai", transport.AssistantFinal{Content: "done", FinishReason: "completed"}),
		transport.NewTerminalEvent("", transport.EventTypeRunCompleted, "openai"),
	}}
	schema := `{"type":"object","properties":{"path":{"type":"string","maxLength":100}},"required":["path"]}`
	s := newFixtureService(&memorySessionRepo{}, &memoryLeaseManager{}, newMemoryRunManager(), &memoryTranscriptStore{}, provider, Config{
		ProviderName: "openai", ModelName: "fixture", CompactModelContext: true,
		ToolCatalog: stubToolCatalog{contracts: []*toolbrokerv1.ToolContract{{ToolName: "fixture.read", Description: "read", InputSchemaJson: schema}}},
	}, nil)
	_, err := s.Execute(context.Background(), ingress.InputEvent{EventID: "compact", IdempotencyKey: "compact", SessionKey: "compact", Source: "web", EventType: runv1.InputEventType_INPUT_EVENT_TYPE_USER_MESSAGE, PayloadJSON: `{"text":"Preserve exact owner constraint"}`, CreatedAt: time.Now()})
	if err != nil {
		t.Fatal(err)
	}
	q := provider.startRequests[0]
	if len(q.ToolDefinitions) != 1 || q.ToolDefinitions[0].SchemaJSON != schema {
		t.Fatal("native schema changed")
	}
	text := fixtureContextText(q.InputItems)
	if strings.Contains(text, "Available tools:") || !strings.Contains(text, "Preserve exact owner constraint") {
		t.Fatal("summary retained or owner dropped")
	}
}

func TestInitialSkillSelectionIsAbsentUnlessTrustedResolverConfigured(t *testing.T) {
	p := &preparedRun{Task: &task.Record{TaskID: "task"}, InputPayload: map[string]any{"initial_skills": []string{strings.Repeat("a", 64)}}}
	s := &Service{config: Config{Skills: instructionSkillCatalog{}}}
	if _, err := s.loadInitialSkills(context.Background(), nil, &sessionv1.RunRecord{RunId: "run"}, "lease", p); err != nil {
		t.Fatal(err)
	}
}

func TestSkillContextRefreshDoesNotDuplicateDescriptionsOrProcedures(t *testing.T) {
	s := &Service{config: Config{Skills: instructionSkillCatalog{}}}
	owner := content.CurrentOwnerMessage("owner constraint unchanged", "input:owner")
	p := preparedRun{CurrentRunID: "run", Task: &task.Record{}, InputItems: []transport.InputItem{owner}}
	for i := 0; i < 3; i++ {
		if err := s.attachSkillContext(context.Background(), &p, false); err != nil {
			t.Fatal(err)
		}
	}
	if len(p.InputItems) != 3 || p.InputItems[0].Content != owner.Content {
		t.Fatal("duplicate catalog or lost owner")
	}
	for i := 0; i < 3; i++ {
		if err := s.attachSkillContext(context.Background(), &p, true); err != nil {
			t.Fatal(err)
		}
	}
	if len(p.InputItems) != 3 || !strings.Contains(p.InputItems[2].Content, "UNTRUSTED_PROCEDURE") {
		t.Fatal("duplicate procedures or missing DATA")
	}
}
