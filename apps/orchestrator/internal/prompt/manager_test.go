package prompt

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/butler/butler/internal/config"
	memoryservice "github.com/butler/butler/internal/memory/service"
)

func TestManagerGetFallsBackToDefaultPrompt(t *testing.T) {
	t.Parallel()

	manager := NewStaticManager()
	manager.env = func(string) (string, bool) { return "", false }

	state, err := manager.Get(context.Background())
	if err != nil {
		t.Fatalf("Get returned error: %v", err)
	}
	if state.EffectivePrompt != DefaultBasePrompt {
		t.Fatalf("expected default prompt, got %q", state.EffectivePrompt)
	}
	if !state.Enabled {
		t.Fatal("expected default prompt state to be enabled")
	}
	if state.Source != config.ConfigSourceDefault {
		t.Fatalf("expected default source, got %q", state.Source)
	}
}

func TestManagerUpdateRejectsSecretLikePrompt(t *testing.T) {
	t.Parallel()

	manager := NewManager(&stubSettingsStore{})
	if _, err := manager.Update(context.Background(), UpdateRequest{BasePrompt: "password=supersecret"}); err == nil {
		t.Fatal("expected secret-like prompt to be rejected")
	}
}

func TestManagerUpdatePersistsPromptAndEnabledFlag(t *testing.T) {
	t.Parallel()

	store := &stubSettingsStore{}
	manager := NewManager(store)
	enabled := false

	state, err := manager.Update(context.Background(), UpdateRequest{BasePrompt: "Operator prompt", Enabled: &enabled, UpdatedBy: "unit-test"})
	if err != nil {
		t.Fatalf("Update returned error: %v", err)
	}
	if got := store.values[SettingKeyBasePrompt].Value; got != "Operator prompt" {
		t.Fatalf("expected stored prompt, got %q", got)
	}
	if got := store.values[SettingKeyBasePromptEnabled].Value; got != "false" {
		t.Fatalf("expected stored enabled flag, got %q", got)
	}
	if state.Enabled {
		t.Fatal("expected updated state to be disabled")
	}
	if state.EffectivePrompt != DefaultBasePrompt {
		t.Fatalf("expected disabled prompt to fall back to default, got %q", state.EffectivePrompt)
	}
}

func TestAssemblerReplacesKnownPlaceholdersAndAppendsRemainingSections(t *testing.T) {
	t.Parallel()

	assembler := NewAssembler()
	result := assembler.Assemble(ConfigState{ConfiguredPrompt: "Base {{session_summary}}", EffectivePrompt: "Base {{session_summary}}", Enabled: true}, Context{
		SessionSummary: "Current task",
		Working:        memoryservice.WorkingContext{Goal: "Ship release", Status: "active"},
		Profile:        []map[string]any{{"key": "language", "summary": "Russian"}},
		ToolSummary:    "- http.request [http]: fetch data",
	})

	if result.FinalPrompt == "" {
		t.Fatal("expected final prompt to be assembled")
	}
	if want := "Session summary:\nCurrent task"; !contains(result.FinalPrompt, want) {
		t.Fatalf("expected placeholder section in prompt, got %q", result.FinalPrompt)
	}
	if want := "Working memory:\n- Goal: Ship release\n- Status: active"; !contains(result.FinalPrompt, want) {
		t.Fatalf("expected remaining working section to be appended, got %q", result.FinalPrompt)
	}
	if want := "Available tools:\n- http.request [http]: fetch data"; !contains(result.FinalPrompt, want) {
		t.Fatalf("expected tool summary section to be appended, got %q", result.FinalPrompt)
	}
	if len(result.UnknownPlaceholders) != 0 {
		t.Fatalf("expected no unknown placeholders, got %+v", result.UnknownPlaceholders)
	}
	if !result.Sections[0].Inserted {
		t.Fatalf("expected session summary section to be marked inserted, got %+v", result.Sections[0])
	}
}

func TestAssemblerOmitsUnknownPlaceholdersSafely(t *testing.T) {
	t.Parallel()

	assembler := NewAssembler()
	result := assembler.Assemble(ConfigState{ConfiguredPrompt: "Base {{unknown_value}}", EffectivePrompt: "Base {{unknown_value}}", Enabled: true}, Context{})

	if len(result.UnknownPlaceholders) != 1 || result.UnknownPlaceholders[0] != "unknown_value" {
		t.Fatalf("expected unknown placeholder to be reported, got %+v", result.UnknownPlaceholders)
	}
	if result.FinalPrompt != "Base" {
		t.Fatalf("expected unknown placeholder to be omitted safely, got %q", result.FinalPrompt)
	}
}

func TestAssemblerIncludesBrowserStrategySection(t *testing.T) {
	t.Parallel()

	assembler := NewAssembler()
	result := assembler.Assemble(ConfigState{EffectivePrompt: "Base prompt", Enabled: true}, Context{
		BrowserStrategy: BrowserStrategyContent,
	})

	if result.FinalPrompt == "" {
		t.Fatal("expected final prompt to be assembled")
	}
	if !contains(result.FinalPrompt, "Browser strategy:") {
		t.Fatalf("expected browser strategy section label in prompt, got %q", result.FinalPrompt)
	}
	if !contains(result.FinalPrompt, "DIRECT NAVIGATION FIRST") {
		t.Fatalf("expected browser strategy content in prompt, got %q", result.FinalPrompt)
	}
	if !contains(result.FinalPrompt, "ALWAYS SNAPSHOT AFTER NAVIGATE") {
		t.Fatalf("expected snapshot rule in browser strategy, got %q", result.FinalPrompt)
	}
	if !contains(result.FinalPrompt, "NUMBERED FOLLOW-UPS") {
		t.Fatalf("expected numbered follow-up rule in browser strategy, got %q", result.FinalPrompt)
	}
	if !contains(result.FinalPrompt, "SINGLE-TAB AUTONOMY") {
		t.Fatalf("expected single-tab autonomy rule in browser strategy, got %q", result.FinalPrompt)
	}
	if !contains(result.FinalPrompt, "NO RECOVERY MENUS BY DEFAULT") {
		t.Fatalf("expected recovery-menu rule in browser strategy, got %q", result.FinalPrompt)
	}
}

func TestAssemblerOmitsBrowserStrategySectionWhenEmpty(t *testing.T) {
	t.Parallel()

	assembler := NewAssembler()
	result := assembler.Assemble(ConfigState{EffectivePrompt: "Base prompt", Enabled: true}, Context{
		BrowserStrategy: "",
	})

	if contains(result.FinalPrompt, "Browser strategy:") {
		t.Fatalf("expected browser strategy section to be omitted when empty, got %q", result.FinalPrompt)
	}
}

func TestBrowserStrategyPlaceholderIsAllowed(t *testing.T) {
	t.Parallel()

	assembler := NewAssembler()
	result := assembler.Assemble(ConfigState{
		ConfiguredPrompt: "Base {{browser_strategy}}",
		EffectivePrompt:  "Base {{browser_strategy}}",
		Enabled:          true,
	}, Context{
		BrowserStrategy: BrowserStrategyContent,
	})

	if len(result.UnknownPlaceholders) != 0 {
		t.Fatalf("expected browser_strategy to be a known placeholder, got unknown: %+v", result.UnknownPlaceholders)
	}
	if !contains(result.FinalPrompt, "DIRECT NAVIGATION FIRST") {
		t.Fatalf("expected browser strategy content to be inserted via placeholder, got %q", result.FinalPrompt)
	}
}

func TestBrowserStrategySurvivesOperatorPromptOverride(t *testing.T) {
	t.Parallel()

	assembler := NewAssembler()
	result := assembler.Assemble(ConfigState{
		ConfiguredPrompt: "Custom operator prompt without placeholders",
		EffectivePrompt:  "Custom operator prompt without placeholders",
		Enabled:          true,
	}, Context{
		BrowserStrategy: BrowserStrategyContent,
		ToolSummary:     "- browser.navigate [browser]: open page",
	})

	if !contains(result.FinalPrompt, "Custom operator prompt") {
		t.Fatalf("expected operator prompt in final output, got %q", result.FinalPrompt)
	}
	if !contains(result.FinalPrompt, "Browser strategy:") {
		t.Fatalf("expected browser strategy section to be appended after operator prompt, got %q", result.FinalPrompt)
	}
	if !contains(result.FinalPrompt, "Available tools:") {
		t.Fatalf("expected tool summary section to be appended, got %q", result.FinalPrompt)
	}
}

type stubSettingsStore struct {
	values map[string]config.Setting
	now    time.Time
}

func (s *stubSettingsStore) Get(_ context.Context, key string) (config.Setting, error) {
	if s.values == nil {
		return config.Setting{}, config.ErrSettingNotFound
	}
	item, ok := s.values[key]
	if !ok {
		return config.Setting{}, config.ErrSettingNotFound
	}
	return item, nil
}

func (s *stubSettingsStore) Set(_ context.Context, setting config.Setting) (config.Setting, error) {
	if s.values == nil {
		s.values = map[string]config.Setting{}
	}
	if s.now.IsZero() {
		s.now = time.Date(2026, 3, 16, 12, 0, 0, 0, time.UTC)
	}
	setting.UpdatedAt = s.now
	s.values[setting.Key] = setting
	return setting, nil
}

func contains(haystack, needle string) bool {
	return strings.Contains(haystack, needle)
}
