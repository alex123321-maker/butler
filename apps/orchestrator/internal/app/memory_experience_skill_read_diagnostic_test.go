package app

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/butler/butler/internal/budget"
	commonv1 "github.com/butler/butler/internal/gen/common/v1"
	"github.com/butler/butler/internal/memory/catalog"
	"github.com/butler/butler/internal/memory/memorytest"
	"github.com/butler/butler/internal/memory/readcontrol"
	"github.com/butler/butler/internal/transport"
)

// This provider reads only fresh synthetic accepted lessons through ordinary
// Core/Broker dispatch. It never calls a cloud provider or prepares an artifact.
type experienceSkillReadDiagnosticProvider struct {
	experienceOriginalControlProvider
	sources    []catalog.SourceRef
	toolNames  []string
	readStatus []string
}

func (p *experienceSkillReadDiagnosticProvider) open() transport.EventStream {
	ref := p.sources[p.phase-1]
	return p.emit(readcontrol.Open, readcontrol.Args{SourceID: ref.ID, Revision: ref.Revision, AccessRevision: ref.AccessRevision, Hash: ref.Hash, MaxChars: 8192})
}

func (p *experienceSkillReadDiagnosticProvider) StartRun(ctx context.Context, q transport.StartRunRequest) (transport.EventStream, error) {
	if transport.CheckInferenceAuthority(ctx) != nil || transport.CheckInferenceDispatch(ctx) != nil {
		return nil, fmt.Errorf("controlled lesson read lacks ordinary admission")
	}
	p.run, p.phase = q.Context.RunID, 1
	for _, tool := range q.ToolDefinitions {
		p.toolNames = append(p.toolNames, tool.Name)
	}
	return p.open(), nil
}

func (p *experienceSkillReadDiagnosticProvider) SubmitToolResult(ctx context.Context, q transport.SubmitToolResultRequest) (transport.EventStream, error) {
	if transport.CheckInferenceAuthority(ctx) != nil || transport.CheckInferenceDispatch(ctx) != nil {
		return nil, fmt.Errorf("controlled lesson continuation lacks ordinary admission")
	}
	var envelope struct {
		Status string          `json:"status"`
		Result json.RawMessage `json:"result"`
	}
	if json.Unmarshal([]byte(q.ToolResultJSON), &envelope) != nil || envelope.Status != "completed" {
		return nil, fmt.Errorf("controlled lesson result unavailable")
	}
	result, err := readcontrol.DecodeResult(envelope.Result)
	if err != nil {
		return nil, fmt.Errorf("controlled lesson result malformed")
	}
	p.readStatus = append(p.readStatus, result.Status)
	if result.Status != "ok" || len(result.Items) != 1 {
		return nil, fmt.Errorf("controlled lesson read was not available")
	}
	p.phase++
	if p.phase <= len(p.sources) {
		return p.open(), nil
	}
	stream := make(chan transport.TransportEvent, 1)
	stream <- transport.NewAssistantFinalEvent(p.run, p.Name(), transport.AssistantFinal{Content: "Controlled fixture: both current lesson DATA were read; no recipient action requested.", FinishReason: "STOP", Usage: &transport.Usage{InputTokens: 30, OutputTokens: 20, TotalTokens: 60}})
	close(stream)
	return stream, nil
}

// Opt-in local diagnosis, not native acceptance. All originals, owner decisions,
// notes and consumer tasks are freshly created in this disposable fixture. No
// sealed native database, private auth, old lesson or old window is reopened.
func TestExperienceNativeSkillReadPermissionDiagnosticIntegration(t *testing.T) {
	if os.Getenv("BUTLER_EXPERIENCE_SKILL_READ_DIAGNOSTIC") != "fresh-scripted-lessons-only" {
		t.Skip("explicit local zero-native applicability diagnostic only")
	}
	runExperienceSkillReadPermissionDiagnostic(t, "prepare_and_verify_exact_resource")
}

func TestExperienceNativeSkillPreparationSuccessorReadIntegration(t *testing.T) {
	if os.Getenv("BUTLER_ISSUE115_SKILL_SUCCESSOR_READ") != "fresh-scripted-latest232-v1" {
		t.Skip("explicit new preparation successor zero-native gate required")
	}
	history := experienceNativeConfiguredHistory(t)
	profile, err := experienceNativeSkillPreparationProfile(history)
	if err != nil || history.SkillPreparationSuccessor == nil || profile != "prepare_and_verify_exact_resource" {
		t.Fatal("new exact recipe did not select the authorized bounded preparation profile", err)
	}
	runExperienceSkillReadPermissionDiagnostic(t, profile)
}

func runExperienceSkillReadPermissionDiagnostic(t *testing.T, completeProfile string) {
	t.Helper()
	if completeProfile != "prepare_and_verify_exact_resource" {
		t.Fatal("fixed existing prepare/check profile required")
	}
	ctx, stop := context.WithTimeout(t.Context(), 20*time.Minute)
	defer stop()
	corpus := experienceWindowFixture()
	corpus.Originals.CommonCurrentInstruction = "Controlled fixture: only current scoped work; never apply an artifact."
	seeds := experienceNativeSeeds{Original: "Controlled README\nready=false\n{exact_declared_canary}\n", V1: "Controlled README\nready=false\n<!-- {sha256_control_seed} -->\n", V2: "Controlled README v2\nready=pending\n<!-- {sha256_control_seed} -->\n", Canaries: map[string]string{}}
	for i := range corpus.Originals.Tasks {
		item := &corpus.Originals.Tasks[i]
		item.EnvironmentProfile, item.Reset, item.PermissionProfile = "project-v1-normal", "empty_patch_store_original_readme", "prepare_and_verify_exact_resource"
		item.Instruction = "Inspect alpha and prepare and check the current unapplied artifact"
		seeds.Canaries[item.ID] = fmt.Sprintf("<!-- controlled-source-%d -->", i)
	}
	corpus.Originals.Tasks[3].EnvironmentProfile = "project-v1-receipt-outage"
	for i := range corpus.Controls.Cases {
		item := &corpus.Controls.Cases[i]
		item.EnvironmentProfile, item.Reset, item.PermissionProfile = "project-v1-normal", "empty_patch_store_original_readme", "prepare_and_verify_exact_resource"
	}
	corpus.Originals.RecipientSeeds, _ = json.Marshal(seeds)
	proof := t.TempDir()
	if root := os.Getenv("BUTLER_EXPERIENCE_PROCESS_PROOF"); filepath.IsAbs(root) {
		if err := os.MkdirAll(root, 0700); err != nil {
			t.Fatal("private diagnostic proof directory unavailable")
		}
		var err error
		proof, err = os.MkdirTemp(root, "skill-read-diagnostic-")
		if err != nil {
			t.Fatal("new diagnostic identity unavailable")
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
	global := memorytest.Database(t)
	if err = p.installGlobal(ctx, global, experienceDigest(t.Name()+time.Now().String())); err != nil {
		t.Fatal(err)
	}
	databases, err := newExperienceNativeClones(ctx, p)
	if err != nil {
		t.Fatal(err)
	}
	defer databases.source.Close()
	now := time.Now().UTC()
	history := experienceNativeLineage{Hash: strings.Repeat("f", 64), Day: now.Format("2006-01-02"), Deadline: now.Truncate(24 * time.Hour).Add(24 * time.Hour), Carry: budget.Units{ModelTurns: 3, Tokens: 3 * 1081344, ActiveMillis: 900000, ProviderInputBytes: 3 * 8388608}, Usage: temporalV5CurrentUsage{ProductAttempts: 3, ProductNULL: 3, ProductWithoutNativeEntry: 3}}
	policy, err := experienceNativeCarryPolicy(false, history.Carry)
	if err != nil {
		t.Fatal(err)
	}
	sourcePolicy, err := experienceNativeCarryPolicy(true, history.Carry)
	if err != nil || applyExperienceNativeHistory(ctx, p, databases.source, history, sourcePolicy) != nil {
		t.Fatal("fresh synthetic controlled history unavailable")
	}
	anchor := filepath.Join(proof, "source-deletion-anchor")
	if catalog.NewFileManifest(anchor).Initialize() != nil {
		t.Fatal("fresh diagnostic deletion anchor unavailable")
	}
	c := &experienceNativeCollector{t: t, ctx: ctx, provider: p, databases: databases, corpus: corpus, proof: proof, anchor: anchor, deadline: now.Add(19 * time.Minute), policy: policy, sourcePolicy: sourcePolicy, addresses: newExperienceNativeAddresses(t), seeds: seeds}
	p.ModelProvider = &experienceWholeControlledProvider{collector: c}
	for i := 0; i < 4; i++ {
		if !t.Run(fmt.Sprintf("fresh_original_%d", i+1), func(t *testing.T) { c.original(t, i) }) {
			return
		}
	}
	for i := 0; i < 4; i++ {
		if !t.Run(fmt.Sprintf("fresh_note_%d", i+1), func(t *testing.T) { c.note(t, i) }) {
			return
		}
	}
	refs := []catalog.SourceRef{c.originals[0].Lesson, c.originals[1].Lesson}
	for _, permissions := range []string{"inspect_and_check_only", completeProfile} {
		t.Run(permissions, func(t *testing.T) {
			directory := c.directory(t, "consumer-"+permissions)
			recipient, err := newExperienceRecipient(directory, c.originals[0].Recipient.Config)
			if err != nil || recipient.persist(c.originals[0].Recipient) != nil {
				t.Fatal("ordinary diagnostic recipient unavailable")
			}
			defer recipient.close()
			r := c.runtime(t, databases.source, directory, anchor, recipient, policy, experienceNativeArms[2], refs)
			defer r.close()
			if r.environment != c.originals[0].Environment || r.generation != c.originals[0].Generation {
				t.Fatal("controlled consumer changed the source environment")
			}
			id := experienceDigest(p.window + ":consumer:" + permissions)
			record := r.createTask(t, ctx, id, "Read only the two permitted current lesson sources; do not act at the recipient.", corpus.Originals.Tasks[0].Resource, permissions)
			r.acceptCurrentProfile(t, ctx, record)
			c.grant(t, r, record, refs)
			provider := &experienceSkillReadDiagnosticProvider{sources: refs}
			before := recipient.snapshot().Dispatches
			out := r.execute(t, ctx, record, id, "Read only the two permitted current lesson sources; no recipient action.", provider)
			hasPrepare := false
			for _, name := range provider.toolNames {
				hasPrepare = hasPrepare || name == "extension.project.prepare-patch"
			}
			if recipient.snapshot().Dispatches != before {
				t.Fatal("controlled lesson read changed the recipient")
			}
			var durableState string
			if provider.run == "" || r.pool.QueryRow(ctx, `SELECT current_state FROM runs WHERE run_id=$1 AND task_id=$2`, provider.run, record.TaskID).Scan(&durableState) != nil {
				t.Fatal("controlled lesson read lost its actual durable run")
			}
			if permissions == "inspect_and_check_only" {
				// Core can reject stale source currentness before calling the
				// provider continuation. The actual durable tool result below,
				// rather than a provider callback, must prove the denied read.
				if hasPrepare || out.Err == nil || durableState != "failed" || len(provider.readStatus) > 1 || len(provider.readStatus) == 1 && provider.readStatus[0] != "denied" {
					t.Fatalf("missing source dependency boundary failed: prepare=%t error_present=%t durable_state=%s callbacks=%d", hasPrepare, out.Err != nil, durableState, len(provider.readStatus))
				}
				var denied bool
				if r.pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM tool_calls WHERE run_id=$1 AND tool_name='memory.open' AND result->>'status'='denied' AND jsonb_array_length(result->'items')=0)`, provider.run).Scan(&denied) != nil || !denied {
					t.Fatal("ordinary denied lesson read was not retained")
				}
			} else if !hasPrepare || out.Err != nil || out.Result == nil || out.Result.CurrentState != commonv1.RunState_RUN_STATE_COMPLETED || durableState != "completed" || len(provider.readStatus) != 2 || provider.readStatus[0] != "ok" || provider.readStatus[1] != "ok" {
				t.Fatal("current complete dependency grant did not permit both ordinary reads")
			}
			t.Logf("CONTROLLED_SKILL_READ profile=%s prepare_contract_visible=%t durable_state=%s observed_read_results=%d recipient_changed=false native_auth_calls=0 original_closed_failure_not_reclassified=true", permissions, hasPrepare, durableState, len(provider.readStatus))
		})
	}
}
