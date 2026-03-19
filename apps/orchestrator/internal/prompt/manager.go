package prompt

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/butler/butler/internal/config"
	"github.com/butler/butler/internal/memory/sanitize"
	memoryservice "github.com/butler/butler/internal/memory/service"
)

const (
	SettingKeyBasePrompt        = "BUTLER_BASE_SYSTEM_PROMPT"
	SettingKeyBasePromptEnabled = "BUTLER_BASE_SYSTEM_PROMPT_ENABLED"

	defaultUpdatedBy = "orchestrator-api"
)

const DefaultBasePrompt = "You are Butler, a self-hosted long-lived personal agent. Always respond in Russian unless the user explicitly asks for a different language. Follow the user's request, rely on the provided runtime context when it is relevant, use structured tool definitions instead of guessing tool behavior, and never invent hidden state, secrets, or credentials."

var placeholderPattern = regexp.MustCompile(`\{\{\s*([a-z_]+)\s*\}\}`)

type SettingsStore interface {
	Get(context.Context, string) (config.Setting, error)
	Set(context.Context, config.Setting) (config.Setting, error)
}

type lookupEnvFunc func(string) (string, bool)

type Manager struct {
	store SettingsStore
	env   lookupEnvFunc
}

type ConfigState struct {
	ConfiguredPrompt string    `json:"configured_prompt"`
	EffectivePrompt  string    `json:"effective_prompt"`
	Enabled          bool      `json:"enabled"`
	Source           string    `json:"source"`
	UpdatedAt        time.Time `json:"updated_at,omitempty"`
	UpdatedBy        string    `json:"updated_by,omitempty"`
}

type UpdateRequest struct {
	BasePrompt string
	Enabled    *bool
	UpdatedBy  string
}

type Context struct {
	SessionSummary  string
	Working         memoryservice.WorkingContext
	Profile         []map[string]any
	Episodes        []map[string]any
	Chunks          []map[string]any
	ToolSummary     string
	BrowserStrategy string
}

type Section struct {
	Name          string `json:"name"`
	Label         string `json:"label"`
	Content       string `json:"content"`
	Inserted      bool   `json:"inserted"`
	Truncated     bool   `json:"truncated"`
	Omitted       bool   `json:"omitted"`
	OmittedReason string `json:"omitted_reason,omitempty"`
}

type Assembly struct {
	ConfiguredPrompt    string    `json:"configured_prompt"`
	EffectiveBasePrompt string    `json:"effective_base_prompt"`
	FinalPrompt         string    `json:"final_prompt"`
	Sections            []Section `json:"sections"`
	UnknownPlaceholders []string  `json:"unknown_placeholders,omitempty"`
	Truncated           bool      `json:"truncated"`
}

type Assembler struct {
	BasePromptLimit int
	SectionLimit    int
	TotalLimit      int
}

func NewManager(store SettingsStore) *Manager {
	return &Manager{store: store, env: os.LookupEnv}
}

func NewStaticManager() *Manager {
	return &Manager{env: os.LookupEnv}
}

func NewAssembler() *Assembler {
	return &Assembler{BasePromptLimit: 4000, SectionLimit: 2500, TotalLimit: 12000}
}

func (m *Manager) Get(ctx context.Context) (ConfigState, error) {
	if m == nil {
		return defaultConfigState(), nil
	}
	configuredPrompt, promptSource, promptSetting, err := m.resolveString(ctx, SettingKeyBasePrompt)
	if err != nil {
		return ConfigState{}, err
	}
	enabled, enabledSource, enabledSetting, err := m.resolveBool(ctx, SettingKeyBasePromptEnabled, true)
	if err != nil {
		return ConfigState{}, err
	}
	state := ConfigState{
		ConfiguredPrompt: configuredPrompt,
		EffectivePrompt:  effectivePrompt(configuredPrompt, enabled),
		Enabled:          enabled,
		Source:           combinedSource(promptSource, enabledSource),
	}
	if latest, ok := latestSetting(promptSetting, enabledSetting); ok {
		state.UpdatedAt = latest.UpdatedAt
		state.UpdatedBy = latest.UpdatedBy
	}
	return state, nil
}

func (m *Manager) Update(ctx context.Context, req UpdateRequest) (ConfigState, error) {
	if m == nil || m.store == nil {
		return ConfigState{}, fmt.Errorf("prompt settings store is not configured")
	}
	if err := validatePrompt(req.BasePrompt); err != nil {
		return ConfigState{}, err
	}
	current, err := m.Get(ctx)
	if err != nil {
		return ConfigState{}, err
	}
	enabled := current.Enabled
	if req.Enabled != nil {
		enabled = *req.Enabled
	}
	updatedBy := strings.TrimSpace(req.UpdatedBy)
	if updatedBy == "" {
		updatedBy = defaultUpdatedBy
	}
	if _, err := m.store.Set(ctx, config.Setting{
		Key:       SettingKeyBasePrompt,
		Value:     req.BasePrompt,
		Component: "orchestrator",
		UpdatedBy: updatedBy,
	}); err != nil {
		return ConfigState{}, err
	}
	if _, err := m.store.Set(ctx, config.Setting{
		Key:       SettingKeyBasePromptEnabled,
		Value:     formatBool(enabled),
		Component: "orchestrator",
		UpdatedBy: updatedBy,
	}); err != nil {
		return ConfigState{}, err
	}
	return m.Get(ctx)
}

func (a *Assembler) Assemble(state ConfigState, ctx Context) Assembly {
	if a == nil {
		a = NewAssembler()
	}
	basePrompt, baseTruncated := truncate(strings.TrimSpace(state.EffectivePrompt), a.BasePromptLimit)
	sections := buildSections(ctx, a.SectionLimit)
	placeholderNames := placeholderNames(basePrompt)
	unknown := unknownPlaceholders(placeholderNames)
	inserted := map[string]struct{}{}
	assembledBase := placeholderPattern.ReplaceAllStringFunc(basePrompt, func(match string) string {
		name := placeholderName(match)
		section, ok := sectionByName(sections, name)
		if !ok {
			return ""
		}
		if strings.TrimSpace(section.Content) == "" {
			return ""
		}
		inserted[name] = struct{}{}
		return section.Content
	})
	parts := []string{strings.TrimSpace(assembledBase)}
	for idx := range sections {
		if _, ok := inserted[sections[idx].Name]; ok {
			sections[idx].Inserted = true
			continue
		}
		if sections[idx].Omitted {
			continue
		}
		parts = append(parts, sections[idx].Content)
	}
	finalPrompt, totalTruncated := truncate(strings.Join(nonEmpty(parts), "\n\n"), a.TotalLimit)
	return Assembly{
		ConfiguredPrompt:    state.ConfiguredPrompt,
		EffectiveBasePrompt: basePrompt,
		FinalPrompt:         finalPrompt,
		Sections:            sections,
		UnknownPlaceholders: unknown,
		Truncated:           baseTruncated || totalTruncated || anySectionTruncated(sections),
	}

}

func buildSections(ctx Context, limit int) []Section {
	sections := []Section{
		buildSection("session_summary", "Session summary", strings.TrimSpace(ctx.SessionSummary), limit),
		buildSection("working_memory", "Working memory", formatWorkingMemory(ctx.Working), limit),
		buildSection("profile_memory", "Profile memory", formatProfileEntries(ctx.Profile), limit),
		buildSection("episodic_memory", "Relevant episodes", formatEpisodeEntries(ctx.Episodes), limit),
		buildSection("document_chunks", "Relevant document chunks", formatChunkEntries(ctx.Chunks), limit),
		buildSection("tool_summary", "Available tools", strings.TrimSpace(ctx.ToolSummary), limit),
		buildSection("browser_strategy", "Browser strategy", strings.TrimSpace(ctx.BrowserStrategy), limit),
	}
	return sections
}

func buildSection(name, label, body string, limit int) Section {
	body = strings.TrimSpace(body)
	if body == "" {
		return Section{Name: name, Label: label, Omitted: true, OmittedReason: "empty"}
	}
	content, truncated := truncate(label+":\n"+body, limit)
	return Section{Name: name, Label: label, Content: content, Truncated: truncated}
}

func formatWorkingMemory(working memoryservice.WorkingContext) string {
	if working.IsEmpty() {
		return ""
	}
	lines := []string{}
	if goal := strings.TrimSpace(working.Goal); goal != "" {
		lines = append(lines, "- Goal: "+goal)
	}
	if entities := formatJSONValue(working.ActiveEntities); entities != "" {
		lines = append(lines, "- Active entities: "+entities)
	}
	if pending := formatJSONValue(working.PendingSteps); pending != "" {
		lines = append(lines, "- Pending steps: "+pending)
	}
	if status := strings.TrimSpace(working.Status); status != "" && status != "idle" {
		lines = append(lines, "- Status: "+status)
	}
	return strings.Join(lines, "\n")
}

func formatProfileEntries(entries []map[string]any) string {
	if len(entries) == 0 {
		return ""
	}
	lines := make([]string, 0, len(entries))
	for _, entry := range entries {
		key := strings.TrimSpace(stringValue(entry["key"]))
		summary := strings.TrimSpace(stringValue(entry["summary"]))
		if key == "" || summary == "" {
			continue
		}
		lines = append(lines, "- "+key+": "+summary)
	}
	return strings.Join(lines, "\n")
}

func formatEpisodeEntries(entries []map[string]any) string {
	if len(entries) == 0 {
		return ""
	}
	lines := make([]string, 0, len(entries))
	for _, entry := range entries {
		summary := strings.TrimSpace(stringValue(entry["summary"]))
		if summary == "" {
			continue
		}
		lines = append(lines, "- "+summary)
	}
	return strings.Join(lines, "\n")
}

func formatChunkEntries(entries []map[string]any) string {
	if len(entries) == 0 {
		return ""
	}
	lines := make([]string, 0, len(entries))
	for _, entry := range entries {
		title := strings.TrimSpace(stringValue(entry["title"]))
		summary := strings.TrimSpace(stringValue(entry["summary"]))
		if title == "" || summary == "" {
			continue
		}
		lines = append(lines, "- "+title+": "+summary)
	}
	return strings.Join(lines, "\n")
}

func validatePrompt(value string) error {
	if len(value) > 16000 {
		return fmt.Errorf("base prompt exceeds 16000 characters")
	}
	if sanitize.Text(value) != value {
		return fmt.Errorf("base prompt contains secret-like content")
	}
	return nil
}

func (m *Manager) resolveString(ctx context.Context, key string) (string, string, config.Setting, error) {
	if value, ok := lookupNonEmptyEnv(m.env, key); ok {
		return value, config.ConfigSourceEnv, config.Setting{}, nil
	}
	if m == nil || m.store == nil {
		return "", config.ConfigSourceDefault, config.Setting{}, nil
	}
	setting, err := m.store.Get(ctx, key)
	if err != nil {
		if err == config.ErrSettingNotFound {
			return "", config.ConfigSourceDefault, config.Setting{}, nil
		}
		return "", "", config.Setting{}, err
	}
	return setting.Value, config.ConfigSourceDB, setting, nil
}

func (m *Manager) resolveBool(ctx context.Context, key string, fallback bool) (bool, string, config.Setting, error) {
	if value, ok := lookupNonEmptyEnv(m.env, key); ok {
		parsed, err := parseBool(value)
		if err != nil {
			return fallback, "", config.Setting{}, err
		}
		return parsed, config.ConfigSourceEnv, config.Setting{}, nil
	}
	if m == nil || m.store == nil {
		return fallback, config.ConfigSourceDefault, config.Setting{}, nil
	}
	setting, err := m.store.Get(ctx, key)
	if err != nil {
		if err == config.ErrSettingNotFound {
			return fallback, config.ConfigSourceDefault, config.Setting{}, nil
		}
		return fallback, "", config.Setting{}, err
	}
	parsed, err := parseBool(setting.Value)
	if err != nil {
		return fallback, "", config.Setting{}, err
	}
	return parsed, config.ConfigSourceDB, setting, nil
}

func defaultConfigState() ConfigState {
	return ConfigState{EffectivePrompt: DefaultBasePrompt, Enabled: true, Source: config.ConfigSourceDefault}
}

func effectivePrompt(configured string, enabled bool) string {
	if enabled && strings.TrimSpace(configured) != "" {
		return strings.TrimSpace(configured)
	}
	return DefaultBasePrompt
}

func latestSetting(left, right config.Setting) (config.Setting, bool) {
	switch {
	case left.Key == "" && right.Key == "":
		return config.Setting{}, false
	case left.Key == "":
		return right, true
	case right.Key == "":
		return left, true
	case right.UpdatedAt.After(left.UpdatedAt):
		return right, true
	default:
		return left, true
	}
}

func combinedSource(promptSource, enabledSource string) string {
	if promptSource == config.ConfigSourceEnv || enabledSource == config.ConfigSourceEnv {
		return config.ConfigSourceEnv
	}
	if promptSource == config.ConfigSourceDB || enabledSource == config.ConfigSourceDB {
		return config.ConfigSourceDB
	}
	return config.ConfigSourceDefault
}

func lookupNonEmptyEnv(get lookupEnvFunc, key string) (string, bool) {
	if get == nil {
		return "", false
	}
	value, ok := get(key)
	if !ok || strings.TrimSpace(value) == "" {
		return "", false
	}
	return value, true
}

func parseBool(value string) (bool, error) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "1", "true", "yes", "on":
		return true, nil
	case "0", "false", "no", "off":
		return false, nil
	default:
		return false, fmt.Errorf("invalid boolean value %q", value)
	}
}

func formatBool(value bool) string {
	if value {
		return "true"
	}
	return "false"
}

func truncate(value string, limit int) (string, bool) {
	trimmed := strings.TrimSpace(value)
	if limit <= 0 || len(trimmed) <= limit {
		return trimmed, false
	}
	if limit <= len("[truncated]") {
		return trimmed[:limit], true
	}
	return strings.TrimSpace(trimmed[:limit-len("[truncated]")-1]) + "\n[truncated]", true
}

func sectionByName(sections []Section, name string) (*Section, bool) {
	for idx := range sections {
		if sections[idx].Name == name {
			return &sections[idx], true
		}
	}
	return nil, false
}

func anySectionTruncated(sections []Section) bool {
	for _, section := range sections {
		if section.Truncated {
			return true
		}
	}
	return false
}

func placeholderNames(prompt string) []string {
	matches := placeholderPattern.FindAllStringSubmatch(prompt, -1)
	seen := map[string]struct{}{}
	result := make([]string, 0, len(matches))
	for _, match := range matches {
		if len(match) < 2 {
			continue
		}
		name := strings.TrimSpace(match[1])
		if _, ok := seen[name]; ok {
			continue
		}
		seen[name] = struct{}{}
		result = append(result, name)
	}
	sort.Strings(result)
	return result
}

func placeholderName(match string) string {
	parts := placeholderPattern.FindStringSubmatch(match)
	if len(parts) < 2 {
		return ""
	}
	return strings.TrimSpace(parts[1])
}

func unknownPlaceholders(names []string) []string {
	allowed := map[string]struct{}{
		"session_summary":  {},
		"working_memory":   {},
		"profile_memory":   {},
		"episodic_memory":  {},
		"document_chunks":  {},
		"tool_summary":     {},
		"browser_strategy": {},
	}
	unknown := make([]string, 0)
	for _, name := range names {
		if _, ok := allowed[name]; ok {
			continue
		}
		unknown = append(unknown, name)
	}
	return unknown
}

func nonEmpty(items []string) []string {
	result := make([]string, 0, len(items))
	for _, item := range items {
		if trimmed := strings.TrimSpace(item); trimmed != "" {
			result = append(result, trimmed)
		}
	}
	return result
}

func stringValue(value any) string {
	if str, ok := value.(string); ok {
		return str
	}
	return ""
}

func formatJSONValue(value any) string {
	if value == nil {
		return ""
	}
	encoded, err := json.Marshal(value)
	if err != nil {
		return ""
	}
	trimmed := strings.TrimSpace(string(encoded))
	if trimmed == "null" || trimmed == "{}" || trimmed == "[]" || trimmed == `""` {
		return ""
	}
	return trimmed
}
