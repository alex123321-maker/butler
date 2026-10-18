package app

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/butler/butler/internal/budget"
	"github.com/butler/butler/internal/memory/catalog"
	"github.com/butler/butler/internal/memory/experience"
	"github.com/butler/butler/internal/transport"
)

// An explicit offline boundary control, not a provider replacement selected by
// environment. Only this opt-in test constructs it. The real native entry always
// supplies one capture+existing native transport for all15 generative phases.
type coherentOptionsControl struct {
	*consolidationReadControl
	target transport.ModelProvider
	phases []string
}

func (*coherentOptionsControl) InferenceLimits() (int64, int64) { return 1048576, 32768 }
func (p *coherentOptionsControl) StartRun(ctx context.Context, r transport.StartRunRequest) (transport.EventStream, error) {
	if p.target == nil {
		return nil, errors.New("offline phase absent")
	}
	return p.target.StartRun(ctx, r)
}
func (p *coherentOptionsControl) ContinueRun(ctx context.Context, r transport.ContinueRunRequest) (transport.EventStream, error) {
	return p.target.ContinueRun(ctx, r)
}
func (p *coherentOptionsControl) SubmitToolResult(ctx context.Context, r transport.SubmitToolResultRequest) (transport.EventStream, error) {
	return p.target.SubmitToolResult(ctx, r)
}
func (p *coherentOptionsControl) phase(pilot *coherentPilot, name string) error {
	p.phases = append(p.phases, name)
	control := &coherentPilotProvider{consolidationReadControl: pilot.provider.consolidationReadControl, source: pilot.provider.source}
	switch name {
	case "preference_candidate":
		control.mode = "candidate"
		p.target = control
	case "simple_question":
		control.mode = "simple"
		p.target = control
	case "new_conversation", "current_english_exception", "restarted_continuation":
		control.mode = "read"
		p.target = control
	case "long_primary_1", "long_primary_2", "long_primary_3", "long_primary_4", "long_primary_5":
		control.mode = "long"
		p.target = control
	case "primary_compaction":
		control.mode = "summary"
		p.target = control
	case "verified_original":
		p.target = &coherentExperienceOriginal{experienceOriginalControlProvider: &experienceOriginalControlProvider{}, preference: pilot.provider.source.Body}
	case "accepted_episode_note":
		var body string
		var observation experience.Observation
		if pilot.pool.QueryRow(pilot.ctx, `SELECT r.body FROM memory_experience_observations o JOIN memory_source_revisions r ON r.source_id=o.source_id AND r.revision=o.source_revision`).Scan(&body) != nil || json.Unmarshal([]byte(body), &observation) != nil || len(observation.Actions) == 0 {
			return errors.New("actual offline original observation absent")
		}
		p.target = &coherentExperienceNote{experienceNoteProvider: experienceNoteProvider{note: experience.Notes{Summary: "Controlled current independently checked unapplied artifact", When: []string{"Exact current resource and reviewed tools"}, NotApplicable: []string{"UNKNOWN or missing current permissions"}, Steps: []experience.Step{{ActionID: observation.Actions[0].ID, Comment: "Actual original evidence; no instruction authority"}}}}}
	case "native_consolidation":
		p.target = p.consolidationReadControl
	case "consolidated_read":
		var id, body string
		var lesson experience.Lesson
		if pilot.pool.QueryRow(pilot.ctx, `SELECT g.source_id FROM memory_consolidation_generations g WHERE view_key='coherent-episode-and-preference'`).Scan(&id) != nil {
			return errors.New("actual offline consolidated view absent")
		}
		view, err := pilot.cat.Get(pilot.ctx, id)
		if err != nil {
			return err
		}
		if pilot.pool.QueryRow(pilot.ctx, `SELECT r.body FROM memory_experience_lessons l JOIN memory_source_revisions r ON r.source_id=l.source_id AND r.revision=l.source_revision`).Scan(&body) != nil || json.Unmarshal([]byte(body), &lesson) != nil {
			return errors.New("actual offline lesson absent")
		}
		key := "web:" + experienceDigest("coherent-current-episode-consumer")
		p.target = &coherentViewRead{consolidationReadControl: &consolidationReadControl{routes: map[string][]catalog.SourceRef{key: {screenSourceRef(view)}}, runs: map[string]*consolidationControlRun{}}, want: pilot.provider.source.Body, ownerResult: lesson.Observation.OwnerResultID}
	case "mechanics_tail":
		p.target = nil
	default:
		return errors.New("unfrozen offline phase")
	}
	return nil
}

func TestCoherentNativeOptionsOfflineLifecycleIntegration(t *testing.T) {
	if os.Getenv("BUTLER_COHERENT_NATIVE_OPTIONS_CONTROL") != "1" {
		t.Skip("explicit own PG/compiled runtime/dump/restore/8KiB fault required")
	}
	if os.Getenv("BUTLER_MEMORY_COHERENT_RESTORE_DATABASE") != "coherent_lifecycle_restore_test" {
		t.Fatal("exact own empty restore target required")
	}
	corpus, err := readCoherentNativeCorpus("../../../../deploy/memory-coherent/long-v1")
	if err != nil {
		t.Fatal(err)
	}
	policy, err := coherentNativePolicy(budget.Units{})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 12*time.Minute)
	defer cancel()
	now := time.Now().UTC()
	history := experienceNativeLineage{Hash: experienceDigest("controlled-empty-native92-history"), Day: now.Format("2006-01-02"), Deadline: time.Date(now.Year(), now.Month(), now.Day()+1, 0, 0, 0, 0, time.UTC)}
	dsn := os.Getenv("BUTLER_TEST_MEMORY_POSTGRES_URL")
	sealSHA := experienceDigest(t.Name())
	pool, err := claimCoherentNativePool(ctx, dsn, sealSHA, history)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	if _, err = claimCoherentNativePool(ctx, dsn, sealSHA, history); err == nil {
		t.Fatal("same native92 domain reopened")
	}
	if _, err = claimCoherentNativePool(ctx, dsn, experienceDigest("changed-control-seal"), history); err == nil {
		t.Fatal("new hash reopened consumed native92 namespace")
	}
	if err = applyCoherentNativeHistory(ctx, pool, history, policy); err != nil {
		t.Fatal(err)
	}
	if err = applyCoherentNativeHistory(ctx, pool, history, policy); err == nil {
		t.Fatal("coherent carry imported twice")
	}
	control := &coherentOptionsControl{consolidationReadControl: &consolidationReadControl{routes: map[string][]catalog.SourceRef{}, runs: map[string]*consolidationControlRun{}}}
	captureProof := filepath.Join(filepath.Dir(os.Getenv("BUTLER_MEMORY_COHERENT_PROOF")), "controlled-capture")
	if err = os.Mkdir(captureProof, 0700); err != nil {
		t.Fatal(err)
	}
	// The same journal/cost code is exercised using a deterministic leaf. These
	// are controlled method entries, never calls to Gemini or native usage.
	capture := &coherentNativeCapture{ModelProvider: coherentUnobservedControl{ModelProvider: control}, proof: captureProof, pool: pool, deadline: now.Add(12 * time.Minute), slots: coherentNativeSchedule(), active: -1}
	options := &coherentPilotOptions{Context: ctx, Pool: pool, Policy: policy, Provider: capture, LongInputs: corpus.Inputs, ContextTokens: 131072, Phase: func(p *coherentPilot, name string) error {
		if err := control.phase(p, name); err != nil {
			return err
		}
		return capture.phase(ctx, name)
	}, ValidateRead: capture.completeRead}
	runMemoryCoherentRestoreWithOptions(t, options)
	if err = capture.close(ctx, false); err != nil {
		t.Fatal(err)
	}
	if err = coherentNativeExport(pool, captureProof, capture.entries, history); err != nil {
		t.Fatal("controlled actual cost/journal mechanics", err)
	}
	if err = coherentNativeWriteNew(captureProof, "controlled-provider-kind.json", map[string]any{"controlled_only": true, "actual_native_calls": 0, "auth": 0, "controlled_method_entries": capture.count(), "scripted_usage_reported": false, "native_usage_fields_in_cost_fixture_are_controlled_method_population": true}); err != nil {
		t.Fatal(err)
	}
	if len(control.phases) != 16 || control.phases[15] != "mechanics_tail" {
		t.Fatal("offline options omitted a generative phase")
	}
	var timeline struct {
		ControlledOnly bool `json:"controlled_only"`
		NativeCalls    int  `json:"native_calls"`
		Steps          []coherentPilotStep
	}
	raw, err := os.ReadFile(filepath.Join(os.Getenv("BUTLER_MEMORY_COHERENT_PROOF"), "timeline-lifecycle.json"))
	if err != nil || json.Unmarshal(raw, &timeline) != nil || !timeline.ControlledOnly || timeline.NativeCalls != 0 || len(timeline.Steps) != 25 {
		t.Fatal("offline option control mislabeled as native or incomplete")
	}
	for i, name := range coherentNativeCheckpoints() {
		if timeline.Steps[i].Name != name {
			t.Fatal("offline options changed ordered checkpoint")
		}
	}
}
