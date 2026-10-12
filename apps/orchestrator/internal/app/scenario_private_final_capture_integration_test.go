package app

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/butler/butler/internal/acceptance"
	"github.com/butler/butler/internal/memory/memorytest"
	"github.com/butler/butler/internal/transport"
)

func TestScenarioPrivateFinalCapturePreservesChargedFailedCostIntegration(t *testing.T) {
	corpus, err := loadShadowV3Corpus()
	if err != nil {
		t.Fatal("fixed synthetic fixture source unavailable")
	}
	selection, err := prepareShadowV3Selection(corpus)
	if err != nil {
		t.Fatal("validated synthetic projection unavailable")
	}
	for _, collision := range []bool{false, true} {
		name := "raw_preserved"
		if collision {
			name = "immutable_collision"
		}
		t.Run(name, func(t *testing.T) {
			pool := memorytest.Database(t)
			w, err := createShadowV3WindowSelected(context.Background(), pool, time.Minute, selection)
			if err != nil {
				t.Fatal(err)
			}
			private := t.TempDir()
			if collision && os.WriteFile(filepath.Join(private, "final-249.raw"), []byte("immutable prior bytes"), 0600) != nil {
				t.Fatal("collision fixture unavailable")
			}
			base := &shadowV3FixtureProvider{mode: "malformed_raw_json", text: strings.Repeat("PRIVATE_VISIBLE_BODY", 250)}
			observer := &privateFinalCaptureProvider{ModelProvider: base, private: private, ordinal: 249}
			sampler := &shadowV3Sampler{provider: observer, window: w, private: private}
			c := selection.inputs.Cases[0]
			a, failure := sampler.call(context.Background(), "quality", c, acceptance.CloudRoleSingle, nil, nil)
			m := observer.finalMetadata()
			if failure == nil || a.Outcome != "transport_error" || string(a.Output) != "null" || m == nil || m.Reason != "json_syntax" || m.RawRetained == collision || observer.captureFailure() != collision || base.entries != 1 {
				t.Fatal("private capture promoted output, lost raw or repeated the provider")
			}
			var attempts, rows int
			var input, output, total int64
			if pool.QueryRow(context.Background(), `SELECT attempts FROM fixture_scenario_shadow_v3_limit`).Scan(&attempts) != nil || pool.QueryRow(context.Background(), `SELECT count(*) FROM fixture_scenario_shadow_v3_attempts`).Scan(&rows) != nil || pool.QueryRow(context.Background(), `SELECT input_tokens,output_tokens,total_tokens FROM fixture_scenario_shadow_v3_attempts WHERE ordinal=249 AND outcome='transport_error'`).Scan(&input, &output, &total) != nil || attempts != 249 || rows != 1 || input != 4 || output != 2 || total != 7 {
				t.Fatal("failed visible-final capture changed owning known spend")
			}
			raw, err := os.ReadFile(filepath.Join(private, "final-249.raw"))
			wanted := base.text
			if collision {
				wanted = "immutable prior bytes"
			}
			if err != nil || string(raw) != wanted {
				t.Fatal("private raw bytes repaired or overwritten")
			}
			for _, file := range []string{"attempt-249.json", "attempt-249-diagnostic.json"} {
				body, err := os.ReadFile(filepath.Join(private, file))
				if err != nil || strings.Contains(string(body), "PRIVATE_VISIBLE_BODY") {
					t.Fatal("visible raw body entered collector output or safe diagnostics")
				}
			}
			body, _ := json.Marshal(m)
			if strings.Contains(string(body), "PRIVATE_VISIBLE_BODY") || strings.Contains(string(body), private) {
				t.Fatal("private content/path entered safe encoding metadata")
			}
			if _, err := sampler.call(context.Background(), "quality", c, acceptance.CloudRoleSingle, nil, nil); err == nil || base.entries != 1 {
				t.Fatal("private failure reopened spent inference")
			}
		})
	}
}

type privateFinalAdversarialProvider struct {
	*shadowV3FixtureProvider
	events func(string) []transport.TransportEvent
}

func (p *privateFinalAdversarialProvider) StartRun(_ context.Context, r transport.StartRunRequest) (transport.EventStream, error) {
	p.entries++
	return initiativeProjectStream(p.events(r.Context.RunID)...), nil
}

func TestScenarioPrivateFinalCapturePreservesOriginalUncertaintyIntegration(t *testing.T) {
	corpus, err := loadShadowV3Corpus()
	if err != nil {
		t.Fatal(err)
	}
	selection, err := prepareShadowV3Selection(corpus)
	if err != nil {
		t.Fatal(err)
	}
	for _, mode := range []string{"legacy_conflict_with_capture_collision", "tail_conflict_after_error", "duplicate_final"} {
		t.Run(mode, func(t *testing.T) {
			pool := memorytest.Database(t)
			w, err := createShadowV3WindowSelected(context.Background(), pool, time.Minute, selection)
			if err != nil {
				t.Fatal(err)
			}
			private := t.TempDir()
			if mode != "duplicate_final" && os.WriteFile(filepath.Join(private, "final-249.raw"), []byte("immutable prior bytes"), 0600) != nil {
				t.Fatal("collision fixture unavailable")
			}
			base := &privateFinalAdversarialProvider{shadowV3FixtureProvider: &shadowV3FixtureProvider{}, events: func(run string) []transport.TransportEvent {
				version := "gemini-3.8-flash-observed"
				known := &transport.Usage{InputTokens: 4, OutputTokens: 2, TotalTokens: 7}
				changed := &transport.Usage{InputTokens: 4, OutputTokens: 2, TotalTokens: 8}
				first := transport.NewAssistantFinalEvent(run, "fixture", transport.AssistantFinal{Content: `{"refs":[]}`, FinishReason: "STOP", Usage: known})
				first.Usage, first.ProviderModelVersion = known, &version
				terminal := transport.NewTerminalEvent(run, transport.EventTypeRunCompleted, "fixture")
				if mode == "legacy_conflict_with_capture_collision" {
					first.Usage = changed
					return []transport.TransportEvent{first, terminal}
				}
				if mode == "tail_conflict_after_error" {
					failed := transport.NewTransportErrorEvent(run, "fixture", &transport.Error{Type: transport.ErrorTypeProviderProtocolError})
					other := "gemini-3.8-flash-other"
					terminal.Usage, terminal.ProviderModelVersion = changed, &other
					return []transport.TransportEvent{first, failed, terminal}
				}
				second := first
				copy := *first.AssistantFinal
				copy.Content = `{"refs":["foreign"]}`
				second.AssistantFinal = &copy
				return []transport.TransportEvent{first, second, terminal}
			}}
			observer := &privateFinalCaptureProvider{ModelProvider: base, private: private, ordinal: 249}
			sampler := &shadowV3Sampler{provider: observer, window: w, private: private}
			a, failure := sampler.call(context.Background(), "quality", selection.inputs.Cases[0], acceptance.CloudRoleSingle, nil, nil)
			if failure == nil || a.Outcome != "transport_error" || !observer.captureFailure() || base.entries != 1 {
				t.Fatal("capture changed original failure/uncertainty or repeated inference")
			}
			if mode != "duplicate_final" && (a.InputTokens != nil || a.OutputTokens != nil || a.TotalTokens != nil) {
				t.Fatal("capture fault converted ambiguous original measurements into known cost")
			}
			if mode == "tail_conflict_after_error" && a.ProviderModelVersion != nil {
				t.Fatal("observer hid a contradictory original version tail")
			}
			if mode == "duplicate_final" {
				body, err := os.ReadFile(filepath.Join(private, "final-249.raw"))
				if err != nil || string(body) != `{"refs":[]}` || !observer.finalMetadata().RawRetained || a.TotalTokens == nil || *a.TotalTokens != 7 {
					t.Fatal("duplicate final overwrote first private bytes or measured spend")
				}
			}
			var rows int
			if pool.QueryRow(context.Background(), `SELECT count(*) FROM fixture_scenario_shadow_v3_attempts WHERE outcome='transport_error'`).Scan(&rows) != nil || rows != 1 {
				t.Fatal("owning failed entry missing")
			}
		})
	}
}

func TestScenarioPrivateFinalCaptureDoesNotStoreForeignOrMisTypedFinal(t *testing.T) {
	for _, foreign := range []bool{true, false} {
		private := t.TempDir()
		event := transport.NewAssistantFinalEvent("capture-run", "fixture", transport.AssistantFinal{Content: "PRIVATE_FOREIGN_FINAL"})
		if foreign {
			event.RunID = "foreign-run"
		} else {
			event.EventType = transport.EventTypeAssistantDelta
		}
		base := &chainHighStreamSpy{converterWorkerProvider: &converterWorkerProvider{}, events: []transport.TransportEvent{event}}
		p := &privateFinalCaptureProvider{ModelProvider: base, private: private, ordinal: 267}
		stream, err := p.StartRun(context.Background(), transport.StartRunRequest{Context: transport.TransportRunContext{RunID: "capture-run"}})
		if err != nil {
			t.Fatal(err)
		}
		for range stream {
		}
		if p.finalMetadata() != nil || p.captureFailure() {
			t.Fatal("foreign or malformed event interpreted as owned final")
		}
		if _, err := os.Stat(filepath.Join(private, "final-267.raw")); !os.IsNotExist(err) {
			t.Fatal("foreign/mis-typed private output retained")
		}
	}
}

func TestScenarioPrivateFinalCaptureIgnoresDeltaAndKeepsOriginalEvent(t *testing.T) {
	private := t.TempDir()
	base := &chainHighStreamSpy{converterWorkerProvider: &converterWorkerProvider{}, events: []transport.TransportEvent{
		transport.NewAssistantDeltaEvent("capture-run", "fixture", transport.AssistantDelta{Content: "PRIVATE_DELTA_NOT_FINAL"}),
		transport.NewAssistantFinalEvent("capture-run", "fixture", transport.AssistantFinal{Content: `{"refs":[]}`}),
		transport.NewTerminalEvent("capture-run", transport.EventTypeRunCompleted, "fixture"),
	}}
	p := &privateFinalCaptureProvider{ModelProvider: base, private: private, ordinal: 267}
	stream, err := p.StartRun(context.Background(), transport.StartRunRequest{Context: transport.TransportRunContext{RunID: "capture-run"}})
	if err != nil {
		t.Fatal(err)
	}
	count := 0
	for event := range stream {
		count++
		if event.AssistantFinal != nil && event.AssistantFinal.Content != `{"refs":[]}` {
			t.Fatal("observer repaired visible model output")
		}
	}
	raw, err := os.ReadFile(filepath.Join(private, "final-267.raw"))
	if err != nil || string(raw) != `{"refs":[]}` || count != 3 || p.finalMetadata() == nil || p.finalMetadata().Reason != "json_object" {
		t.Fatal("delta captured or transport event altered")
	}
	if _, err := p.ContinueRun(context.Background(), transport.ContinueRunRequest{}); err == nil {
		t.Fatal("unsupported continuation reached provider")
	}
	if _, err := p.SubmitToolResult(context.Background(), transport.SubmitToolResultRequest{}); err == nil {
		t.Fatal("tool-result path reached provider")
	}
}
