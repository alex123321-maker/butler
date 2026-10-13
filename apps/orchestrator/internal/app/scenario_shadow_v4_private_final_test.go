package app

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/butler/butler/internal/acceptance"
	"github.com/butler/butler/internal/memory/memorytest"
)

func TestScenarioShadowV4PrivateFinalExactFailureEvidenceIntegration(t *testing.T) {
	corpus, err := loadShadowV4Corpus()
	if err != nil {
		t.Fatal("corpus denied")
	}
	selection, err := prepareShadowV4Selection(corpus)
	if err != nil {
		t.Fatal("selection denied")
	}
	for _, tc := range []struct{ name, text, reason string }{
		{"syntax", `{"refs":`, "json_syntax"},
		{"duplicate", `{"refs":[],"r\u0065fs":[]}`, "duplicate_key"},
		{"shape", `[]`, "non_object"},
		{"utf8", string([]byte{'{', '"', 'x', '"', ':', '"', 0xff, '"', '}'}), "invalid_utf8"},
		{"oversize", strings.Repeat("x", privateFinalMaximumBytes+1), "visible_bound"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			pool := memorytest.Database(t)
			w, err := createShadowV4WindowSelected(ctx, pool, time.Minute, selection)
			if err != nil {
				t.Fatal("window denied")
			}
			private := t.TempDir()
			p := &shadowV4FixtureProvider{mode: "known", text: tc.text}
			s := &shadowV4Sampler{provider: p, window: w, private: private}
			a, err := s.call(ctx, "quality", selection.inputs.Cases[0], acceptance.CloudRoleSingle, nil, nil)
			if err == nil || p.entries != 1 || a.Outcome != "transport_error" || string(a.Output) != "null" {
				t.Fatal("rejected final released or request retried")
			}
			var input, output, total int64
			var reason, hash string
			var retained, clean bool
			var size, finals, terminals int
			if pool.QueryRow(ctx, `SELECT a.input_tokens,a.output_tokens,a.total_tokens,d.encoding_reason,d.final_hash,d.final_bytes,d.raw_retained,d.final_count,d.terminal_count,d.lifecycle_clean FROM fixture_scenario_shadow_v4_attempts a JOIN fixture_scenario_shadow_v4_diagnostics d USING(ordinal) WHERE ordinal=269`).Scan(&input, &output, &total, &reason, &hash, &size, &retained, &finals, &terminals, &clean) != nil || input != 4 || output != 2 || total != 7 || reason != tc.reason || size != len(tc.text) || hash != privateFinalFacts([]byte(tc.text)).SHA256 || finals != 1 || terminals != 1 || !clean {
				t.Fatal("failure reason, spend, bytes or lifecycle lost")
			}
			raw, rawErr := os.ReadFile(filepath.Join(private, "final-269.raw"))
			if tc.reason == "visible_bound" {
				if retained || !os.IsNotExist(rawErr) {
					t.Fatal("oversized final retained beyond private bound")
				}
			} else if rawErr != nil || !retained || !bytes.Equal(raw, []byte(tc.text)) {
				t.Fatal("exact rejected original bytes lost")
			}
			if _, err := pool.Exec(ctx, `UPDATE fixture_scenario_shadow_v4_diagnostics SET encoding_reason='json_object'`); err == nil {
				t.Fatal("diagnostic observation mutable")
			}
		})
	}
}

func TestScenarioShadowV4PrivateFinalRetentionFailureKeepsSpendIntegration(t *testing.T) {
	corpus, err := loadShadowV4Corpus()
	if err != nil {
		t.Fatal("corpus denied")
	}
	selection, err := prepareShadowV4Selection(corpus)
	if err != nil {
		t.Fatal("selection denied")
	}
	ctx := context.Background()
	pool := memorytest.Database(t)
	w, err := createShadowV4WindowSelected(ctx, pool, time.Minute, selection)
	if err != nil {
		t.Fatal("window denied")
	}
	private := t.TempDir()
	path := filepath.Join(private, "final-269.raw")
	if os.WriteFile(path, []byte("prior immutable file"), 0600) != nil {
		t.Fatal("collision setup denied")
	}
	p := &shadowV4FixtureProvider{mode: "known"}
	s := &shadowV4Sampler{provider: p, window: w, private: private}
	a, err := s.call(ctx, "quality", selection.inputs.Cases[0], acceptance.CloudRoleSingle, nil, nil)
	if err == nil || p.entries != 1 || a.TotalTokens == nil || *a.TotalTokens != 7 {
		t.Fatal("retention failure lost cost or continued")
	}
	var in, out, total int64
	var retained, ok bool
	if pool.QueryRow(ctx, `SELECT a.input_tokens,a.output_tokens,a.total_tokens,d.raw_retained,d.retention_ok FROM fixture_scenario_shadow_v4_attempts a JOIN fixture_scenario_shadow_v4_diagnostics d USING(ordinal) WHERE ordinal=269`).Scan(&in, &out, &total, &retained, &ok) != nil || in != 4 || out != 2 || total != 7 || retained || ok {
		t.Fatal("owning SQL spend or retention failure lost")
	}
	raw, err := os.ReadFile(path)
	if err != nil || string(raw) != "prior immutable file" {
		t.Fatal("prior raw file overwritten")
	}
}
