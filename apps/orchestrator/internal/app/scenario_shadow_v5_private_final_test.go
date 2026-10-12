package app

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/butler/butler/internal/acceptance"
	"github.com/butler/butler/internal/memory/memorytest"
)

func TestScenarioShadowV5PrivateFinalExactFailureEvidenceIntegration(t *testing.T) {
	corpus, err := loadShadowV5Corpus()
	if err != nil {
		t.Fatal("corpus denied")
	}
	selection, err := prepareShadowV5Selection(corpus)
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
			w, err := createShadowV5WindowSelected(ctx, pool, time.Minute, selection)
			if err != nil {
				t.Fatal("window denied")
			}
			private := t.TempDir()
			p := &shadowV5FixtureProvider{mode: "known", text: tc.text}
			s := &shadowV5Sampler{provider: p, window: w, private: private}
			a, err := s.call(ctx, "quality", selection.inputs.Cases[0], acceptance.CloudRoleSingle, nil, nil)
			if err == nil || p.entries != 1 || a.Outcome != "transport_error" || string(a.Output) != "null" {
				t.Fatal("rejected final released or request retried")
			}
			var input, output, total int64
			var reason, hash string
			var retained, clean bool
			var size, finals, terminals int
			if pool.QueryRow(ctx, `SELECT a.input_tokens,a.output_tokens,a.total_tokens,d.encoding_reason,d.final_hash,d.final_bytes,d.raw_retained,d.final_count,d.terminal_count,d.lifecycle_clean FROM fixture_scenario_shadow_v5_attempts a JOIN fixture_scenario_shadow_v5_diagnostics d USING(ordinal) WHERE ordinal=284`).Scan(&input, &output, &total, &reason, &hash, &size, &retained, &finals, &terminals, &clean) != nil || input != 4 || output != 2 || total != 7 || reason != tc.reason || size != len(tc.text) || hash != privateFinalFacts([]byte(tc.text)).SHA256 || finals != 1 || terminals != 1 || !clean {
				t.Fatal("failure reason, spend, bytes or lifecycle lost")
			}
			raw, rawErr := os.ReadFile(filepath.Join(private, "final-284.raw"))
			if rawErr != nil || !retained || !bytes.Equal(raw, []byte(tc.text)) {
				t.Fatal("exact rejected original bytes lost")
			}
			if _, err := pool.Exec(ctx, `UPDATE fixture_scenario_shadow_v5_diagnostics SET encoding_reason='json_object'`); err == nil {
				t.Fatal("diagnostic observation mutable")
			}
		})
	}
}

func TestScenarioShadowV5PrivateFinalRetentionFailureKeepsSpendIntegration(t *testing.T) {
	corpus, err := loadShadowV5Corpus()
	if err != nil {
		t.Fatal("corpus denied")
	}
	selection, err := prepareShadowV5Selection(corpus)
	if err != nil {
		t.Fatal("selection denied")
	}
	ctx := context.Background()
	pool := memorytest.Database(t)
	w, err := createShadowV5WindowSelected(ctx, pool, time.Minute, selection)
	if err != nil {
		t.Fatal("window denied")
	}
	private := t.TempDir()
	path := filepath.Join(private, "final-284.raw")
	if os.WriteFile(path, []byte("prior immutable file"), 0600) != nil {
		t.Fatal("collision setup denied")
	}
	p := &shadowV5FixtureProvider{mode: "known"}
	s := &shadowV5Sampler{provider: p, window: w, private: private}
	a, err := s.call(ctx, "quality", selection.inputs.Cases[0], acceptance.CloudRoleSingle, nil, nil)
	if err == nil || p.entries != 1 || a.TotalTokens == nil || *a.TotalTokens != 7 {
		t.Fatal("retention failure lost cost or continued")
	}
	var in, out, total int64
	var retained, ok bool
	if pool.QueryRow(ctx, `SELECT a.input_tokens,a.output_tokens,a.total_tokens,d.raw_retained,d.retention_ok FROM fixture_scenario_shadow_v5_attempts a JOIN fixture_scenario_shadow_v5_diagnostics d USING(ordinal) WHERE ordinal=284`).Scan(&in, &out, &total, &retained, &ok) != nil || in != 4 || out != 2 || total != 7 || retained || ok {
		t.Fatal("owning SQL spend or retention failure lost")
	}
	raw, err := os.ReadFile(path)
	if err != nil || string(raw) != "prior immutable file" {
		t.Fatal("prior raw file overwritten")
	}
	if _, err = s.call(ctx, "quality", selection.inputs.Cases[1], acceptance.CloudRoleSingle, nil, nil); err == nil || p.entries != 1 {
		t.Fatal("retention failure reopened next admission")
	}
}

func TestScenarioShadowV5PrivateFinalForensicCapNeverWidensVisibleAcceptance(t *testing.T) {
	for _, size := range []int{8193, shadowV5ForensicMaximumBytes, shadowV5ForensicMaximumBytes + 1} {
		t.Run(fmt.Sprint(size), func(t *testing.T) {
			text := strings.Repeat("x", size)
			private := t.TempDir()
			m, err := captureShadowV5PrivateFinal(private, 284, text)
			within := size <= shadowV5ForensicMaximumBytes
			if (err == nil) != within || m.RawRetained != within || m.Bytes != size || m.Reason != "visible_bound" || m.SHA256 != privateFinalFacts([]byte(text)).SHA256 {
				t.Fatal("forensic/visible bounds conflated")
			}
			raw, readErr := os.ReadFile(filepath.Join(private, "final-284.raw"))
			if within && (readErr != nil || string(raw) != text) || !within && !os.IsNotExist(readErr) {
				t.Fatal("original bytes dropped, repaired or retained beyond provider bound")
			}
			if within {
				if _, err = captureShadowV5PrivateFinal(private, 284, "replacement"); err == nil {
					t.Fatal("forensic evidence overwritten")
				}
			}
		})
	}
}

func TestScenarioShadowV5PrivateFinalHardForensicFailureRetainsChargeAndDeniesNextIntegration(t *testing.T) {
	corpus, err := loadShadowV5Corpus()
	if err != nil {
		t.Fatal(err)
	}
	selection, err := prepareShadowV5Selection(corpus)
	if err != nil {
		t.Fatal(err)
	}
	pool := memorytest.Database(t)
	w, err := createShadowV5WindowSelected(context.Background(), pool, time.Minute, selection)
	if err != nil {
		t.Fatal(err)
	}
	p := &shadowV5FixtureProvider{mode: "known", text: strings.Repeat("x", shadowV5ForensicMaximumBytes+1)}
	private := t.TempDir()
	s := &shadowV5Sampler{provider: p, window: w, private: private}
	a, err := s.call(context.Background(), "quality", selection.inputs.Cases[0], acceptance.CloudRoleSingle, nil, nil)
	if err == nil || p.entries != 1 || a.TotalTokens == nil || *a.TotalTokens != 7 || a.Outcome != "transport_error" || string(a.Output) != "null" {
		t.Fatal("hard retention failure promoted or refunded")
	}
	var stage, reason string
	var retained, retentionOK bool
	var size int
	if pool.QueryRow(context.Background(), `SELECT collector_stage,encoding_reason,final_bytes,raw_retained,retention_ok FROM fixture_scenario_shadow_v5_diagnostics WHERE ordinal=284`).Scan(&stage, &reason, &size, &retained, &retentionOK) != nil || stage != "forensic_retention_bound" || reason != "visible_bound" || size != shadowV5ForensicMaximumBytes+1 || retained || retentionOK {
		t.Fatal("explicit forensic failure metadata lost")
	}
	if _, err = os.Stat(filepath.Join(private, "final-284.raw")); !os.IsNotExist(err) {
		t.Fatal("unbounded forensic file written")
	}
	if _, err = s.call(context.Background(), "quality", selection.inputs.Cases[1], acceptance.CloudRoleSingle, nil, nil); err == nil || p.entries != 1 {
		t.Fatal("hard forensic failure admitted another case")
	}
}
