package prompt

import (
	"strings"
	"testing"

	memoryservice "github.com/butler/butler/internal/memory/service"
)

// Development regressions for issue76; these synthetic cases are not an
// independent holdout or a measurement of model preference compliance.
func TestProfileContextPreservesScopeAndWholeExceptions(t *testing.T) {
	for _, tc := range []struct {
		name    string
		entries []map[string]any
		want    []string
	}{
		{
			name: "same_key_different_scopes",
			entries: []map[string]any{
				{"key": "response_style", "summary": "Кратко", "scope_type": "user", "scope_id": "owner", "source_refs": []string{"source:general"}},
				{"key": "response_style", "summary": "Подробно только для этой сессии", "scope_type": "session", "scope_id": "session-76", "source_refs": []string{"source:exception"}},
			},
			want: []string{`scope_type="user"`, `scope_id="owner"`, `scope_type="session"`, `scope_id="session-76"`, "Кратко", "Подробно только для этой сессии", "source:general", "source:exception"},
		},
		{
			name:    "long_preference_trailing_exception",
			entries: []map[string]any{{"key": "report_format", "summary": strings.Repeat("Сохранять подробности наблюдения. ", 130) + "ИСКЛЮЧЕНИЕ: личные данные не включать.", "scope_type": "user", "scope_id": "owner", "source_refs": []string{"source:long-rule"}}},
			want:    []string{"ИСКЛЮЧЕНИЕ: личные данные не включать.", "source:long-rule"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			assembly := NewAssembler().Assemble(defaultConfigState(), Context{Profile: tc.entries})
			var profile Section
			for _, section := range assembly.Sections {
				if section.Name == "profile_memory" {
					profile = section
				}
			}
			bundlePrompt := memoryservice.FormatPrompt(memoryservice.WorkingContext{}, tc.entries, nil, nil, "")
			for _, want := range tc.want {
				if !strings.Contains(profile.Content, want) {
					t.Errorf("model section lost %q", want)
				}
				if !strings.Contains(bundlePrompt, want) {
					t.Errorf("bundle prompt lost %q", want)
				}
			}
			if profile.Truncated {
				t.Error("preference or its scope/exception was shortened")
			}
		})
	}
}
