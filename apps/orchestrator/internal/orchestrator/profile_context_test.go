package orchestrator

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/butler/butler/internal/security/content"
	"github.com/butler/butler/internal/transport"
)

func TestProfileContextModelInputRetainsExceptionAsData(t *testing.T) {
	service := newFixtureService(nil, nil, nil, nil, nil, Config{}, nil)
	rule := strings.Repeat("Подробное описание оформления. ", 130) + "ИСКЛЮЧЕНИЕ: не публиковать без запроса."
	prepared := preparedRun{MemoryBundle: map[string]any{"profile": []map[string]any{
		{"key": "report_format", "summary": rule, "scope_type": "session", "scope_id": "session-76", "source_refs": []string{"source:rule-76"}},
	}}}
	if err := service.attachPromptContext(context.Background(), &prepared); err != nil {
		t.Fatal(err)
	}
	for _, item := range prepared.InputItems {
		var envelope struct {
			Data struct{ Section, Content string }
		}
		if json.Unmarshal([]byte(item.Content), &envelope) != nil || envelope.Data.Section != "profile_memory" {
			continue
		}
		if !strings.Contains(envelope.Data.Content, rule) || !strings.Contains(envelope.Data.Content, `scope_type="session"`) || !strings.Contains(envelope.Data.Content, "source:rule-76") {
			t.Fatal("actual model input lost preference, exception, scope or source")
		}
		if item.Role == "system" || content.ItemOrigin(item).IsInstruction() {
			t.Fatal("profile acquired instruction authority")
		}
		return
	}
	t.Fatal("model input has no profile section")
}

func TestProfileContextOverflowDropsWholeRecordsKeepsOwnerException(t *testing.T) {
	service := newFixtureService(nil, nil, nil, nil, nil, Config{}, nil)
	owner := content.CurrentOwnerMessage("В этот раз ответь подробно; ничего не публикуй.", "input:exception")
	mandatory := content.Instruction("Current task: analyse only; publication prohibited", content.Origin{Kind: content.OwnerPolicy, Sensitivity: content.Private, SourceRefs: []string{"task:76:revision:2"}})
	prepared := preparedRun{InputItems: []transport.InputItem{mandatory, owner}, MemoryBundle: map[string]any{"profile": []map[string]any{
		{"key": "style", "summary": "Кратко", "scope_type": "user", "scope_id": "owner"},
		{"key": "large_rule", "summary": strings.Repeat("Большое предпочтение. ", 2000) + "RULE-END", "scope_type": "user", "scope_id": "owner"},
	}}}
	baseline := prepared
	baseline.MemoryBundle = map[string]any{}
	if err := service.attachPromptContext(context.Background(), &baseline); err != nil {
		t.Fatal(err)
	}
	service.config.ContextBudgetTokens = baseline.observabilityPrompt["context_token_estimate"].(int) + 4096
	if err := service.attachPromptContext(context.Background(), &prepared); err != nil {
		t.Fatal(err)
	}
	entries, _ := prepared.MemoryBundle["profile"].([]map[string]any)
	if len(entries) != 1 || entries[0]["key"] != "style" {
		t.Fatal("did not preserve fitting record and remove oversized record whole")
	}
	foundOwner, foundTask := false, false
	for _, item := range prepared.InputItems {
		foundOwner = foundOwner || item.Content == owner.Content
		foundTask = foundTask || item.Content == mandatory.Content
		if strings.Contains(item.Content, "large_rule") || strings.Contains(item.Content, "[truncated]") {
			t.Fatal("partial preference reached model")
		}
	}
	if !foundOwner || !foundTask {
		t.Fatal("general profile displaced current owner exception or task constraint")
	}
}
