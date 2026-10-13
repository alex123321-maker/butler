package acceptance

import (
	"encoding/json"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestFullFaultGatePreservesSignedHistoricalCriteriaAndHasExactCurrentTests(t *testing.T) {
	if CriteriaDigest() != "76437b99d5eb79baadb39c5eb6d7958a2d6108296c7b6dc6c6118f42b21cd058" {
		t.Fatal("independently signed V3 criteria were changed")
	}
	if len(FullFaultScenarios()) != 8 || FaultCriteriaDigest() == CriteriaDigest() {
		t.Fatal("mandatory faults require a separate versioned eight-group gate")
	}
	groups := map[string]bool{"model-load-oom": true}
	for _, group := range FullFaultGroups() {
		if groups[group.ID] {
			t.Fatal("duplicate fault group", group.ID)
		}
		groups[group.ID] = true
		functions := map[string]bool{}
		files, err := filepath.Glob(filepath.Join("..", "..", group.Package, "*_test.go"))
		if err != nil {
			t.Fatal(err)
		}
		for _, path := range files {
			f, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
			if err != nil {
				t.Fatal(err)
			}
			for _, declaration := range f.Decls {
				if fn, ok := declaration.(*ast.FuncDecl); ok && fn.Recv == nil {
					functions[fn.Name.Name] = true
				}
			}
		}
		seen := map[string]bool{}
		for _, test := range group.Tests {
			if seen[test] || !functions[strings.Split(test, "/")[0]] {
				t.Fatal("missing/duplicate exact owning test", group.ID, test)
			}
			seen[test] = true
		}
	}
	for _, scenario := range FullFaultScenarios() {
		if scenario.Pending != "" || len(scenario.Groups) == 0 {
			t.Fatal("current fault scenario cannot pass as an obsolete dependency label")
		}
		for _, group := range scenario.Groups {
			if !groups[group] {
				t.Fatal("unowned required fault group", group)
			}
		}
	}
}

func writeFaultEvents(t *testing.T, path string, group Group) {
	t.Helper()
	var lines strings.Builder
	for _, test := range group.Tests {
		for _, action := range []string{"run", "pass"} {
			raw, _ := json.Marshal(TestEvent{Action: action, Package: Module + group.Package, Test: test})
			lines.Write(raw)
			lines.WriteByte('\n')
		}
	}
	raw, _ := json.Marshal(TestEvent{Action: "pass", Package: Module + group.Package})
	lines.Write(raw)
	lines.WriteByte('\n')
	if err := os.WriteFile(path, []byte(lines.String()), 0600); err != nil {
		t.Fatal(err)
	}
}

func localOOMFixture(t *testing.T, root string) LocalLoadOOM {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(root, "scripts/dev"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "scripts/dev/local-model-load-oom-lab.sh"), []byte("fixture runner bytes\n"), 0600); err != nil {
		t.Fatal(err)
	}
	hash, _ := FileHash(filepath.Join(root, "scripts/dev/local-model-load-oom-lab.sh"))
	return LocalLoadOOM{Version: LocalLoadOOMVersion, RunnerSHA256: hash, Image: localOOMImage, WeightsSHA256: localOOMWeights, WeightsBytes: 522640096, ManifestSHA256: localOOMManifest, RegistryManifestSHA256: localOOMRegistryManifest, ActualModelLoadAttempts: 1, ModelMetadataRead: true, OOMKilled: true, ExitCode: 137, ElapsedSeconds: 5, MemoryBytes: 268435456, MemorySwapBytes: 268435456, NanoCPUs: 1000000000, PidsLimit: 64, Runtime: "runc", NetworkMode: "none", User: "10001:10001", ReadOnlyRoot: true, OnlyReadOnlyWeightMount: true, NoDeviceRequests: true, NoCapabilities: true, NoNewPrivileges: true, CPUOnly: true, Model: "qwen3:0.6b", Stage: "load_before_inference"}
}

func TestFullFaultReportNeedsEveryCurrentNamedFaultAndPhysicalModelOOM(t *testing.T) {
	root := t.TempDir()
	events := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "migrations"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "migrations/fixture.sql"), []byte("fixture migration"), 0600); err != nil {
		t.Fatal(err)
	}
	for _, group := range FullFaultGroups() {
		writeFaultEvents(t, filepath.Join(events, group.ID+".jsonl"), group)
	}
	p := localOOMFixture(t, root)
	proofPath := filepath.Join(t.TempDir(), "oom.json")
	if err := WriteJSON(proofPath, p); err != nil {
		t.Fatal(err)
	}
	hardware := json.RawMessage(`{"fixture":true}`)
	if err := WriteFaultRunPin(root, events, strings.Repeat("a", 40), hardware); err != nil {
		t.Fatal(err)
	}
	r, err := BuildFullFaultReport(root, events, strings.Repeat("a", 40), json.RawMessage(`{"fixture":true}`), proofPath)
	if err != nil || !r.MandatoryFaultsPassed || r.Issue33Ready {
		t.Fatal("fault-only proof must not approve full issue/model quality", r, err)
	}
	missing, err := BuildFullFaultReport(root, events, strings.Repeat("a", 40), hardware, "")
	if err != nil || missing.MandatoryFaultsPassed {
		t.Fatal("controlled worker fixture substituted for actual cached model OOM")
	}
	p.ModelMetadataRead = false
	if err := WriteJSON(proofPath, p); err != nil {
		t.Fatal(err)
	}
	failedPhysical, err := BuildFullFaultReport(root, events, strings.Repeat("a", 40), hardware, proofPath)
	if err != nil || failedPhysical.MandatoryFaultsPassed || failedPhysical.LocalModelLoadOOM == nil || failedPhysical.LocalModelLoadOOM.ModelMetadataRead || !failedPhysical.LocalModelLoadOOM.OOMKilled || failedPhysical.LocalModelLoadOOM.ActualModelLoadAttempts != 1 {
		t.Fatal("failed physical proof was promoted or known observations lost", err)
	}
	p.ModelMetadataRead = true
	if err := WriteJSON(proofPath, p); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(events, "single-tab.jsonl")
	raw, _ := os.ReadFile(path)
	for _, altered := range []string{
		strings.ReplaceAll(string(raw), "TestSingleTabResourceActualChromiumNativeHostIntegration/lost_ack_restart", "TestOther"),
		strings.Replace(string(raw), `"Action":"pass"`, `"Action":"skip"`, 1),
		strings.ReplaceAll(string(raw), Module+"apps/tool-broker/internal/broker", Module+"wrong-package"),
	} {
		if err := os.WriteFile(path, []byte(altered), 0600); err != nil {
			t.Fatal(err)
		}
		r, err = BuildFullFaultReport(root, events, strings.Repeat("a", 40), hardware, proofPath)
		if err != nil || r.MandatoryFaultsPassed {
			t.Fatal("missing/skipped/foreign fault passed", err)
		}
	}
	writeFaultEvents(t, path, FullFaultGroups()[len(FullFaultGroups())-1])
	for _, commit := range []string{strings.Repeat("a", 40), strings.Repeat("b", 40)} {
		r, err = BuildFullFaultReport(root, events, commit, json.RawMessage(`{"fixture":false}`), proofPath)
		if err != nil || r.MandatoryFaultsPassed {
			t.Fatal("stale release/hardware pin accepted", err)
		}
	}
	if err := os.WriteFile(filepath.Join(root, "migrations/new.sql"), []byte("new schema"), 0600); err != nil {
		t.Fatal(err)
	}
	r, err = BuildFullFaultReport(root, events, strings.Repeat("a", 40), hardware, proofPath)
	if err != nil || r.MandatoryFaultsPassed {
		t.Fatal("events from previous source/schema accepted", err)
	}
}

func TestPhysicalModelOOMProofRejectsGracefulFailureInferenceIsolationAndRelabeling(t *testing.T) {
	root := t.TempDir()
	valid := localOOMFixture(t, root)
	proof := filepath.Join(t.TempDir(), "oom.json")
	mutations := []func(*LocalLoadOOM){
		func(p *LocalLoadOOM) { p.OOMKilled = false }, func(p *LocalLoadOOM) { p.ExitCode = 1 }, func(p *LocalLoadOOM) { p.ActualModelLoadAttempts = 2 },
		func(p *LocalLoadOOM) { p.ActualInferenceCalls = 1 }, func(p *LocalLoadOOM) { p.ActualNativeCalls = 1 }, func(p *LocalLoadOOM) { p.ModelMetadataRead = false },
		func(p *LocalLoadOOM) { p.MemorySwapBytes = 536870912 }, func(p *LocalLoadOOM) { p.ElapsedSeconds = 91 }, func(p *LocalLoadOOM) { p.NetworkMode = "bridge" },
		func(p *LocalLoadOOM) { p.NoDeviceRequests = false }, func(p *LocalLoadOOM) { p.OnlyReadOnlyWeightMount = false }, func(p *LocalLoadOOM) { p.NoCapabilities = false },
		func(p *LocalLoadOOM) { p.NoNewPrivileges = false }, func(p *LocalLoadOOM) { p.ButlerKernelReceivedOOM = true }, func(p *LocalLoadOOM) { p.Model = "3.8 flash high" },
		func(p *LocalLoadOOM) { p.RunnerSHA256 = strings.Repeat("b", 64) }, func(p *LocalLoadOOM) { p.WeightsSHA256 = strings.Repeat("b", 64) },
		func(p *LocalLoadOOM) { p.RegistryManifestSHA256 = strings.Repeat("b", 64) },
	}
	for i, change := range mutations {
		p := valid
		change(&p)
		if err := WriteJSON(proof, p); err != nil {
			t.Fatal(err)
		}
		if _, err := ReadLocalLoadOOM(root, proof); err == nil {
			t.Fatal("invalid physical load proof accepted", i)
		}
	}
	if err := WriteJSON(proof, valid); err != nil {
		t.Fatal(err)
	}
	raw, _ := os.ReadFile(proof)
	for _, altered := range []string{strings.Replace(string(raw), `"oom_killed": true`, `"oom_killed": true, "OOM_KILLED": false`, 1), strings.TrimSpace(string(raw)) + `{}`, strings.Replace(string(raw), `"oom_killed": true`, `"oom_killed": true, "oom_killed": true`, 1)} {
		if err := os.WriteFile(proof, []byte(altered), 0600); err != nil {
			t.Fatal(err)
		}
		if _, err := ReadLocalLoadOOM(root, proof); err == nil {
			t.Fatal("ambiguous load proof accepted")
		}
	}
}
