package app

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/butler/butler/apps/orchestrator/internal/prompt"
	"github.com/butler/butler/internal/acceptance"
	memoryservice "github.com/butler/butler/internal/memory/service"
	"github.com/butler/butler/internal/transport"
)

const issue76FactorialControls = "testdata/acceptance/issue76-context-factorial-controls-v2.json"

// The common response contract is measurement plumbing, not another behavioral
// treatment. The behavioral instructions come from the production assembly.
const issue76AnswerContract = `В этом наблюдении нет исполняемых инструментов. Верни JSON: answer — обычный ответ пользователю; applied_sources — ссылки на действительно использованные источники из видимого контекста; needs_clarification — требуется ли ответ хозяина до следующего зависимого действия; proposed_action — следующий предлагаемый шаг, без заявления о выполнении; profile_update — требуется ли изменение постоянного профиля и почему. Никаких внешних действий этот ответ не выполняет.`

func issue76FactorialInstructions(guidance bool) string {
	text := (prompt.Assembly{EffectiveBasePrompt: prompt.DefaultBasePrompt}).TrustedInstructions()
	if guidance {
		text += "\n\n" + prompt.PreferenceApplicationProtocol
	}
	return text + "\n\n" + issue76AnswerContract
}

func issue76FactorialFreeze(t *testing.T) issue76Frozen {
	t.Helper()
	root := shadowRepoRoot()
	raw, err := os.ReadFile(filepath.Join(root, issue76FactorialControls))
	if err != nil {
		t.Fatal(err)
	}
	var corpus struct {
		Cases []issue76Case `json:"cases"`
	}
	if json.Unmarshal(raw, &corpus) != nil || len(corpus.Cases) != 8 {
		t.Fatal("eight independent factorial controls required")
	}
	protocol, err := os.ReadFile(filepath.Join(root, "docs/testing/issue-76-context-factorial-v2.md"))
	if err != nil {
		t.Fatal(err)
	}
	f := issue76Frozen{Version: "issue76-factorial-v2", Corpora: map[string]string{issue76FactorialControls: hashMemoryBytes(raw)}, ProtocolSHA: hashMemoryBytes(protocol), Schema: issue76Schema}
	arms := []string{"baseline", "goal", "guidance", "goal_guidance"}
	seen := map[string]bool{}
	for index, cs := range corpus.Cases {
		if cs.ID == "" || seen[cs.ID] || strings.TrimSpace(cs.Goal) == "" || strings.TrimSpace(cs.Question) == "" {
			t.Fatal("invalid independent case identity/task")
		}
		seen[cs.ID] = true
		for _, p := range cs.Profiles {
			if len(p.SourceRefs) == 0 || !(p.ScopeType == "user" && p.ScopeID == "owner" || p.ScopeType == "session" && p.ScopeID == "s") {
				t.Fatal("invalid accepted fixture profile scope/provenance")
			}
		}
		for offset := range arms {
			arm := arms[(index+offset)%len(arms)]
			svc := memoryservice.New(memoryservice.Config{ProfileStore: issue76Profiles(cs.Profiles), Semantic: issue76Documents(cs.Documents), WorkingStore: issue76Working(cs.Goal)})
			req := memoryservice.BundleRequest{SessionKey: "s", UserID: "owner", UserMessage: cs.Question, IncludeQuery: true}
			if arm == "goal" || arm == "goal_guidance" {
				req.ProfileTaskGoal = cs.Goal
			}
			bundle, err := svc.BuildBundle(context.Background(), req)
			if err != nil {
				t.Fatal(err)
			}
			mc := acceptance.MemoryCase{ID: cs.ID, Question: cs.Question, AsOfUTC: "2026-10-23T00:00:00Z", Task: acceptance.MemoryTask{Revision: 1, Goal: cs.Goal, Instruction: cs.Constraints}, Expected: acceptance.MemoryAnswer{Citations: []acceptance.CommandWitness{}}}
			input := acceptance.MemoryInput{CaseID: acceptance.MemoryOpaqueCaseID(mc), Arm: arm, Instructions: issue76FactorialInstructions(arm == "guidance" || arm == "goal_guidance"), Question: cs.Question, AsOfUTC: mc.AsOfUTC, Task: mc.Task, Memory: []acceptance.MemorySource{}, MemoryPrompt: bundle.Prompt, CaptureSHA256: acceptance.MemoryDigest(bundle)}
			projection, err := acceptance.MemoryModelProjection(input)
			if err != nil || len(projection) > 24000 {
				t.Fatal("oversized factorial projection")
			}
			f.Slots = append(f.Slots, issue76Slot{cs.ID, "controls", mc, input, bundle})
		}
	}
	raw, err = json.Marshal(f)
	if err != nil {
		t.Fatal(err)
	}
	var canonical issue76Frozen
	if json.Unmarshal(raw, &canonical) != nil {
		t.Fatal("factorial canonical capture invalid")
	}
	return canonical
}

func TestIssue76FactorialFreeze(t *testing.T) {
	writeIssue76Frozen(t, issue76FactorialFreeze(t), issue76FactorialControls, "")
}

func TestIssue76FactorialMatrix(t *testing.T) {
	f := issue76FactorialFreeze(t)
	raw, _ := json.MarshalIndent(f, "", "  ")
	var restored issue76Frozen
	if json.Unmarshal(raw, &restored) != nil || acceptance.MemoryDigest(f) != acceptance.MemoryDigest(restored) || len(f.Slots) != 32 {
		t.Fatal("factorial round trip/denominator differs")
	}
	for start := 0; start < len(f.Slots); start += 4 {
		byArm := map[string]acceptance.MemoryInput{}
		for _, slot := range f.Slots[start : start+4] {
			byArm[slot.Input.Arm] = slot.Input
			projection, _ := acceptance.MemoryModelProjection(slot.Input)
			if strings.Contains(string(projection), "must_select_sources") || strings.Contains(string(projection), "forbidden_behaviors") {
				t.Fatal("verifier rubric leaked")
			}
		}
		if len(byArm) != 4 {
			t.Fatal("missing or duplicated factorial arm")
		}
		for _, pair := range [][2]string{{"baseline", "guidance"}, {"goal", "goal_guidance"}} {
			a, _ := acceptance.MemoryModelProjection(byArm[pair[0]])
			b, _ := acceptance.MemoryModelProjection(byArm[pair[1]])
			if string(a) != string(b) || byArm[pair[0]].Instructions != issue76FactorialInstructions(false) || byArm[pair[1]].Instructions != issue76FactorialInstructions(true) {
				t.Fatal("guidance changed selected data or production instruction factor")
			}
		}
	}
}

func TestIssue76FactorialCollectorIntegration(t *testing.T) {
	c := memoryCollectorFixture(t)
	c.freeAnswer = true
	c.nativeSchema = issue76Schema
	c.factory = func(schema json.RawMessage) (transport.ModelProvider, error) {
		if acceptance.MemoryDigest(schema) != acceptance.MemoryDigest(issue76Schema) {
			return nil, errors.New("factorial schema differs")
		}
		return &memoryFixtureProvider{raw: `{"answer":"fixture","applied_sources":[],"needs_clarification":false,"proposed_action":"none","profile_update":"none"}`}, nil
	}
	for _, slot := range issue76FactorialFreeze(t).Slots[:4] {
		o, err := c.sample(context.Background(), slot.Case, slot.Input)
		if err != nil || o.Status != "completed" {
			t.Fatal("factorial collector fixture failed", err, o.Reason)
		}
	}
	var attempts, arms int
	if err := c.pool.QueryRow(context.Background(), `SELECT count(*),count(DISTINCT arm) FROM fixture_memory_eval_attempts`).Scan(&attempts, &arms); err != nil || attempts != 4 || arms != 4 {
		t.Fatal("factorial attempts collapsed", attempts, arms, err)
	}
}

func TestIssue76FactorialNativeTrial(t *testing.T) {
	runIssue76Native(t, issue76TrialSpec{"BUTLER_ISSUE76_FACTORIAL_NATIVE", "TestIssue76FactorialNativeTrial", "issue76-factorial-v2", "issue76-factorial-v2", "synthetic-profile-production-instructions-factorial-v2", issue76FactorialControls, "", issue76FactorialFreeze})
}
