package app

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/butler/butler/internal/memory/catalog"
	"github.com/butler/butler/internal/memory/experience"
	"github.com/butler/butler/internal/memory/memorytest"
	"github.com/butler/butler/internal/skills"
	"github.com/butler/butler/internal/transport"
)

// Controlled provider only, never used by the opt-in native entry. This fixture
// validates the whole owning pipeline and reset/cost mechanics, not model skill
// generation, note quality or answers to the independent holdout corpus.
type experienceWholeControlledProvider struct {
	experienceOriginalControlProvider
	collector       *experienceNativeCollector
	mu              sync.Mutex
	runs            map[string]transport.ModelProvider
	firstOriginal03 string
}

func (p *experienceWholeControlledProvider) StartRun(ctx context.Context, q transport.StartRunRequest) (transport.EventStream, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	c := p.collector
	slot := c.provider.slots[c.provider.active]
	var provider transport.ModelProvider
	switch slot.Kind {
	case "original", "control":
		if slot.Kind == "original" && c.provider.active == 2 {
			if p.firstOriginal03 == "" {
				p.firstOriginal03 = q.Context.RunID
				provider = &experienceOriginalControlProvider{ready: "maybe"}
			} else {
				if len(c.originals[2].Unresolved) != 1 {
					return nil, fmt.Errorf("controlled actual original03 lookup missing")
				}
				provider = &experienceReconciliationControlProvider{url: c.originals[2].Unresolved[0].Lookup}
			}
		} else {
			provider = &experienceOriginalControlProvider{}
		}
	case "note":
		o := c.originals[c.provider.active-4].Observation
		note := experience.Notes{Summary: "Controlled owning-chain observation fixture", When: []string{"Exact observed environment and current permissions"}, NotApplicable: []string{"No additional instruction or outcome authority"}, Steps: []experience.Step{}}
		for _, a := range o.Actions {
			note.Steps = append(note.Steps, experience.Step{ActionID: a.ID, Comment: "Observed action; the owning outcome remains authoritative"})
		}
		provider = &experienceTerminalControlProvider{note: note, required: experience.SnapshotFormat}
	case "skill_preparation":
		manifest := skills.Manifest{Format: skills.Format, ID: "controlled-owning-procedure", Version: "1.0.0", Source: skills.Source{URI: "https://butler.invalid/controlled-owning-fixture", Revision: c.provider.window}, Description: "Controlled whole-collector procedure fixture", When: []string{"Current fixture environment"}, Inputs: []string{"Current task"}, NotApplicable: []string{"Changed interface or missing current permissions"}, Dependencies: c.originals[0].Observation.Environment.Tools, Steps: []skills.Step{}, Tests: []skills.Scenario{{Name: "success", Kind: "verified_success", Expected: "Independent source result exists"}, {Name: "failure", Kind: "failure_stop", Expected: "Stop on failed or unknown effect"}, {Name: "changed", Kind: "interface_change", Expected: "Reject changed interface"}}, Sources: []skills.EvidenceRef{}, LessonSources: []catalog.SourceRef{c.originals[0].Lesson, c.originals[1].Lesson}}
		for i, d := range manifest.Dependencies {
			manifest.Steps = append(manifest.Steps, skills.Step{ID: fmt.Sprintf("step-%d", i+1), Tool: d.Tool, Intent: "Controlled fixture step", SuccessCheck: "Check the actual owning observation", OnError: "stop", Retry: "never", ApprovalCheckpoint: true})
		}
		for i := 0; i < 2; i++ {
			o := c.originals[i].Observation
			manifest.Sources = append(manifest.Sources, skills.EvidenceRef{TaskID: o.TaskID, TaskRevision: o.TaskRevision, ResultID: o.OwnerResultID})
		}
		raw, _ := json.Marshal(manifest)
		provider = &experienceWholeControlledFinal{experienceOriginalControlProvider: experienceOriginalControlProvider{}, content: string(raw)}
	default:
		return nil, fmt.Errorf("controlled fixture slot kind unavailable")
	}
	if p.runs == nil {
		p.runs = map[string]transport.ModelProvider{}
	}
	p.runs[q.Context.RunID] = provider
	return provider.StartRun(ctx, q)
}

func (p *experienceWholeControlledProvider) SubmitToolResult(ctx context.Context, q transport.SubmitToolResultRequest) (transport.EventStream, error) {
	p.mu.Lock()
	provider := p.runs[q.RunID]
	p.mu.Unlock()
	if provider == nil {
		return nil, fmt.Errorf("controlled run provider missing")
	}
	return provider.SubmitToolResult(ctx, q)
}

type experienceWholeControlledFinal struct {
	experienceOriginalControlProvider
	content string
}

func (p *experienceWholeControlledFinal) StartRun(ctx context.Context, q transport.StartRunRequest) (transport.EventStream, error) {
	if transport.CheckInferenceAuthority(ctx) != nil || transport.CheckInferenceDispatch(ctx) != nil {
		return nil, fmt.Errorf("controlled final lacks current owning admission")
	}
	stream := make(chan transport.TransportEvent, 1)
	stream <- transport.NewAssistantFinalEvent(q.Context.RunID, p.Name(), transport.AssistantFinal{Content: p.content, Usage: &transport.Usage{InputTokens: 30, OutputTokens: 20, TotalTokens: 60}})
	close(stream)
	return stream, nil
}

func TestExperienceNativeWholeCollectorControlledIntegration(t *testing.T) {
	ctx, stop := context.WithTimeout(context.Background(), 20*time.Minute)
	defer stop()
	corpus := experienceWindowFixture()
	corpus.Originals.CommonCurrentInstruction = "Controlled fixture: execute only the current task using current tools; no patch application."
	seeds := experienceNativeSeeds{Original: "Controlled README\nready=false\n{exact_declared_canary}\n", V1: "Controlled README\nready=false\n<!-- {sha256_control_seed} -->\n", V2: "Controlled README v2\nready=pending\n<!-- {sha256_control_seed} -->\n", Canaries: map[string]string{}}
	for i := range corpus.Originals.Tasks {
		item := &corpus.Originals.Tasks[i]
		item.EnvironmentProfile, item.Reset, item.PermissionProfile = "project-v1-normal", "empty_patch_store_original_readme", "prepare_and_verify_exact_resource"
		item.Instruction = "Inspect alpha and prepare and check the current unapplied artifact"
		seeds.Canaries[item.ID] = fmt.Sprintf("<!-- controlled-original-%d -->", i)
	}
	corpus.Originals.Tasks[3].EnvironmentProfile = "project-v1-receipt-outage"
	for i := range corpus.Controls.Cases {
		item := &corpus.Controls.Cases[i]
		item.EnvironmentProfile, item.Reset, item.PermissionProfile = "project-v1-normal", "empty_patch_store_original_readme", "prepare_and_verify_exact_resource"
		item.Instruction = "Inspect alpha and prepare and check this current unapplied artifact"
	}
	corpus.Originals.RecipientSeeds, _ = json.Marshal(seeds)
	proof := t.TempDir()
	if root := os.Getenv("BUTLER_EXPERIENCE_PROCESS_PROOF"); filepath.IsAbs(root) {
		if err := os.MkdirAll(root, 0700); err != nil {
			t.Fatal(err)
		}
		var err error
		proof, err = os.MkdirTemp(root, "controlled-whole-")
		if err != nil {
			t.Fatal(err)
		}
	}
	p, err := newExperienceNativeProvider(proof, experienceNativeSchedule(corpus))
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := p.export(); err != nil {
			t.Error(err)
		}
	}()
	// Schedule and full-denominator abort export precede this isolated PG fixture
	// or any source work. Native execution uses a separately retained global DB.
	global := memorytest.Database(t)
	if err = p.installGlobal(ctx, global, experienceDigest(t.Name()+time.Now().String())); err != nil {
		t.Fatal(err)
	}
	databases, err := newExperienceNativeClones(ctx, p)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if databases.source != nil {
			databases.source.Close()
		}
	}()
	anchor := filepath.Join(proof, "source-deletion-anchor")
	if err = catalog.NewFileManifest(anchor).Initialize(); err != nil {
		t.Fatal(err)
	}
	c := &experienceNativeCollector{t: t, ctx: ctx, provider: p, databases: databases, corpus: corpus, proof: proof, anchor: anchor, deadline: time.Now().Add(19 * time.Minute), policy: experienceNativeBudgetPolicy(), sourcePolicy: experienceNativeSourcePolicy(), addresses: newExperienceNativeAddresses(t)}
	p.ModelProvider = &experienceWholeControlledProvider{collector: c}
	c.collect()
	if t.Failed() {
		return
	}
	for _, slot := range p.slots {
		if slot.Status != "completed" {
			t.Error("whole controlled collector did not retain all57 completed slots", slot.Sequence, slot.Status)
		}
	}
	if len(p.entries) > 640 || len(p.entries) < 57 {
		t.Fatal("whole controlled capture lost its actual entries")
	}
	var count, used int
	if global.QueryRow(ctx, `SELECT count(*) FROM experience_native_case_databases WHERE window_id=$1 AND state='created'`, p.window).Scan(&count) != nil || count != 48 {
		t.Fatal("whole controlled collector did not create48 fresh control databases")
	}
	if global.QueryRow(ctx, `SELECT used FROM experience_native_windows WHERE window_id=$1`, p.window).Scan(&used) != nil || used != len(p.entries) {
		t.Fatal("whole controlled global spend differs from actual captured admissions")
	}
	if err = p.writeNew("controlled-lifecycle-only.json", map[string]any{"native_provider_calls": 0, "controlled_entries": len(p.entries), "logical_slots": 57, "control_databases": count, "release_qualified": false}); err != nil {
		t.Fatal(err)
	}
}
