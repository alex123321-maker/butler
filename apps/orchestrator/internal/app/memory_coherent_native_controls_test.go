package app

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/butler/butler/internal/budget"
	"github.com/butler/butler/internal/memory/catalog"
	"github.com/butler/butler/internal/memory/readcontrol"
	"github.com/butler/butler/internal/transport"
)

func TestCoherentNativeDelegateConsumesDispatchOnlyAtLeaf(t *testing.T) {
	for _, delegated := range []bool{false, true} {
		calls := 0
		ctx := transport.WithInferenceDispatch(context.Background(), func(context.Context) error {
			calls++
			if calls != 1 {
				return errors.New("one-use dispatch already consumed")
			}
			return nil
		})
		leaf := &coherentPilotProvider{mode: "simple"}
		provider := leaf
		if delegated {
			provider = &coherentPilotProvider{delegate: leaf}
		}
		stream, err := provider.StartRun(ctx, transport.StartRunRequest{})
		if err != nil {
			t.Fatal("transparent delegate consumed leaf dispatch", err)
		}
		for range stream {
		}
		if calls != 1 {
			t.Fatal("actual leaf did not consume exactly one dispatch", calls)
		}
	}
}

func TestCoherentNativePresealRejectsChangedWholeContract(t *testing.T) {
	root := t.TempDir()
	corpus := filepath.Join(root, "deploy/memory-coherent/long-v1")
	if os.MkdirAll(corpus, 0700) != nil {
		t.Fatal("fixture root")
	}
	files, err := os.ReadDir("../../../../deploy/memory-coherent/long-v1")
	if err != nil {
		t.Fatal(err)
	}
	for _, file := range files {
		raw, err := os.ReadFile(filepath.Join("../../../../deploy/memory-coherent/long-v1", file.Name()))
		if err != nil || os.WriteFile(filepath.Join(corpus, file.Name()), raw, 0600) != nil {
			t.Fatal("fixture copy")
		}
	}
	if os.WriteFile(filepath.Join(root, "go.mod"), []byte("module synthetic-frozen-source\n"), 0600) != nil {
		t.Fatal("fixture module")
	}
	rubricPath := filepath.Join(root, "docs/testing/fixtures/issue92-native/rubric-v1.json")
	if os.MkdirAll(filepath.Dir(rubricPath), 0700) != nil {
		t.Fatal("fixture rubric parent")
	}
	rubric, err := os.ReadFile("../../../../docs/testing/fixtures/issue92-native/rubric-v1.json")
	if err != nil || os.WriteFile(rubricPath, rubric, 0600) != nil {
		t.Fatal("fixture rubric")
	}
	binaries := map[string]string{}
	for _, name := range []string{"recipient", "broker", "broker_control", "extension", "http"} {
		path := filepath.Join(t.TempDir(), name)
		if os.WriteFile(path, []byte("bounded-artifact-"+name), 0500) != nil {
			t.Fatal("artifact")
		}
		binaries[name] = path
	}
	now := time.Now().UTC()
	dayEnd := time.Date(now.Year(), now.Month(), now.Day()+1, 0, 0, 0, 0, time.UTC)
	if dayEnd.Sub(now) < 11*time.Minute {
		now = now.Add(-time.Hour)
	} // fixed past instant in same UTC day, never extends execution
	h := experienceNativeLineage{Hash: strings.Repeat("b", 64), Day: now.Format("2006-01-02"), Deadline: dayEnd, Carry: budget.Units{ModelTurns: 3, Tokens: 3 * 1081344}, Usage: temporalV5CurrentUsage{ProductAttempts: 3, ProductNULL: 3, ProductWithoutNativeEntry: 3}}
	seal, err := prepareCoherentNativeSeal(now, root, h, binaries)
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(seal)
	if _, err = checkCoherentNativeSeal(raw, experienceDigest(string(raw)), now, root, h, binaries); err != nil {
		t.Fatal("own exact seal rejected", err)
	}
	for name, change := range map[string]func(*coherentNativeSeal){
		"slot_order":   func(s *coherentNativeSeal) { s.Schedule[0], s.Schedule[1] = s.Schedule[1], s.Schedule[0] },
		"maximum":      func(s *coherentNativeSeal) { s.Schedule[1].Maximum++ },
		"question":     func(s *coherentNativeSeal) { s.Corpus.Inputs[0] += " changed" },
		"carry_refund": func(s *coherentNativeSeal) { s.Policy.Day.ModelTurns-- },
		"context":      func(s *coherentNativeSeal) { s.ContextTokens++ },
		"whole_time":   func(s *coherentNativeSeal) { s.Deadline = s.Deadline.Add(time.Minute) },
		"handback":     func(s *coherentNativeSeal) { s.AuthReturn = "none" },
		"temporal":     func(s *coherentNativeSeal) { s.SelectedFeatures["temporal"] = false },
		"predicate":    func(s *coherentNativeSeal) { s.TemporalPredicate = "hardcoded true" },
		"resources":    func(s *coherentNativeSeal) { s.Resources["client_cpu"]++ },
	} {
		t.Run(name, func(t *testing.T) {
			var forged coherentNativeSeal
			_ = json.Unmarshal(raw, &forged)
			change(&forged)
			body, _ := json.Marshal(forged)
			if _, err := checkCoherentNativeSeal(body, experienceDigest(string(body)), now, root, h, binaries); err == nil {
				t.Fatal("self-consistent forged seal accepted")
			}
		})
	}
	if _, err = checkCoherentNativeSeal(raw, experienceDigest(string(raw)), seal.Deadline, root, h, binaries); err == nil {
		t.Fatal("expired window accepted")
	}
	if os.WriteFile(filepath.Join(root, "go.mod"), []byte("changed\n"), 0600) != nil {
		t.Fatal("source mutation")
	}
	if _, err = checkCoherentNativeSeal(raw, experienceDigest(string(raw)), now, root, h, binaries); err == nil {
		t.Fatal("changed actual source accepted")
	}
}

func TestCoherentNativeReadRequiresEveryActualCurrentOriginalPage(t *testing.T) {
	source := catalog.Source{ID: "source", Revision: 3, AccessRevision: 4, Hash: "frozen-hash", Body: "first Русский last"}
	text := []rune(source.Body)
	entry := func(start, end int) *experienceNativeEntry {
		item := readcontrol.Item{Source: readcontrol.SourceRef{ID: source.ID, Revision: source.Revision, AccessRevision: source.AccessRevision, Hash: source.Hash}, Text: string(text[start:end]), NextOffset: end, Truncated: end < len(text)}
		body, _ := json.Marshal(map[string]any{"status": "completed", "result": readcontrol.Result{Status: "ok", Items: []readcontrol.Item{item}}})
		request, _ := json.Marshal(transport.SubmitToolResultRequest{RunID: "run", ToolResultJSON: string(body)})
		return &experienceNativeEntry{RunID: "run", Kind: "tool_result", Request: request}
	}
	whole := []*experienceNativeEntry{entry(0, 7), entry(7, len(text))}
	if err := (&coherentNativeCapture{entries: whole}).completeRead("run", source); err != nil {
		t.Fatal("exact Unicode pages rejected", err)
	}
	for name, entries := range map[string][]*experienceNativeEntry{"missing_page": {entry(0, 7)}, "gap": {entry(0, 7), entry(8, len(text))}, "empty": {}} {
		t.Run(name, func(t *testing.T) {
			if (&coherentNativeCapture{entries: entries}).completeRead("run", source) == nil {
				t.Fatal("incomplete original accepted")
			}
		})
	}
	wrong := source
	wrong.AccessRevision++
	if (&coherentNativeCapture{entries: whole}).completeRead("run", wrong) == nil {
		t.Fatal("stale access pin accepted")
	}
	wrong = source
	wrong.Body = "First Русский last"
	if (&coherentNativeCapture{entries: whole}).completeRead("run", wrong) == nil {
		t.Fatal("modified original bytes accepted")
	}
	if (&coherentNativeCapture{entries: whole}).completeRead("other-run", source) == nil {
		t.Fatal("foreign run supplied original pages")
	}
}

func TestCoherentNativeCaptureCannotCompleteAnUnenteredSlot(t *testing.T) {
	ctx := context.Background()
	p := &coherentNativeCapture{proof: t.TempDir(), slots: coherentNativeSchedule(), active: -1, deadline: time.Now().Add(time.Minute)}
	if err := p.phase(ctx, "preference_candidate"); err != nil {
		t.Fatal(err)
	}
	if err := p.phase(ctx, "new_conversation"); err == nil {
		t.Fatal("empty slot marked complete")
	}
	if _, err := p.reserve(ctx, "run", "start", map[string]string{"text": "safe"}); err == nil {
		t.Fatal("unadmitted storage/provider released")
	}
	if err := p.close(ctx, true); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(filepath.Join(p.proof, "native-window-final.json"))
	if err != nil {
		t.Fatal(err)
	}
	var final struct {
		Slots   []coherentNativeSlot
		Entries []*experienceNativeEntry
	}
	if json.Unmarshal(raw, &final) != nil || len(final.Slots) != 15 || len(final.Entries) != 0 || final.Slots[0].State != "failed" || final.Slots[14].State != "not_executed" {
		t.Fatal("setup failure hid denominator")
	}
}

func TestCoherentControlledFaultNeverReportsSyntheticUsage(t *testing.T) {
	usage := &transport.Usage{InputTokens: 3, OutputTokens: 2, TotalTokens: 8}
	event := transport.NewAssistantFinalEvent("run", "controlled", transport.AssistantFinal{Content: "controlled", Usage: usage})
	event.Usage = usage
	in := make(chan transport.TransportEvent, 1)
	in <- event
	close(in)
	out, err := coherentWithoutSyntheticUsage(context.Background(), in, nil)
	if err != nil {
		t.Fatal(err)
	}
	got := <-out
	if got.Usage != nil || got.AssistantFinal == nil || got.AssistantFinal.Usage != nil || event.Usage != usage || event.AssistantFinal.Usage != usage {
		t.Fatal("controlled usage entered product/native evidence or original event mutated")
	}
}
