package app

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/butler/butler/apps/orchestrator/internal/api"
	"github.com/butler/butler/apps/orchestrator/internal/task"
	"github.com/butler/butler/internal/actionauth"
	"github.com/butler/butler/internal/budget"
	commonv1 "github.com/butler/butler/internal/gen/common/v1"
	"github.com/butler/butler/internal/memory/catalog"
	"github.com/butler/butler/internal/memory/experience"
	"github.com/butler/butler/internal/memory/jobs"
	"github.com/butler/butler/internal/skills"
	"github.com/jackc/pgx/v5/pgxpool"
)

type experienceNativeSeeds struct {
	Original string            `json:"original_readme_template"`
	V1       string            `json:"fresh_control_readme_template_v1"`
	V2       string            `json:"fresh_control_readme_template_v2"`
	Canaries map[string]string `json:"canaries"`
}

type experienceNativeOriginal struct {
	Task           task.Record                       `json:"task"`
	Run            string                            `json:"run_id"`
	Recipient      experienceRecipientState          `json:"recipient_audit"`
	HTTPStatuses   []experienceRecipientHTTPStatus   `json:"independent_http_statuses"`
	Observation    experience.Observation            `json:"observation"`
	Reconciliation *experience.ReconciliationRequest `json:"reconciliation,omitempty"`
	OriginalSource catalog.SourceRef                 `json:"original_source"`
	Lesson         catalog.SourceRef                 `json:"lesson"`
	Generation     int64                             `json:"extension_generation"`
	Environment    string                            `json:"environment"`
	Unresolved     []experienceNativeCurrentEffect   `json:"current_unresolved_effects"`
}

type experienceNativeCurrentEffect struct {
	ActionID    string                 `json:"action_id"`
	Tool        string                 `json:"tool"`
	Resource    string                 `json:"resource"`
	Lookup      string                 `json:"lookup"`
	Fingerprint string                 `json:"fingerprint"`
	Effect      actionauth.EffectState `json:"effect"`
}

// Whole cohort execution only. Authentication/profile/history construction is
// deliberately outside it and must finish before this receives a real provider.
// No case oracle, note body, recipe or alternative original enters this runner.
type experienceNativeCollector struct {
	t            *testing.T
	ctx          context.Context
	provider     *experienceNativeProvider
	databases    *experienceNativeClones
	corpus       experienceNativeCorpus
	seeds        experienceNativeSeeds
	proof        string
	anchor       string
	deadline     time.Time
	policy       budget.Policy
	sourcePolicy budget.Policy
	addresses    experienceNativeAddresses
	originals    [4]experienceNativeOriginal
	skill        skills.Record
}

func (c *experienceNativeCollector) collect() {
	c.t.Helper()
	// This defer also runs after a failed original/note/skill. Retain all actual
	// source admissions and owning rows, rather than exporting only preparation
	// that passed the template freeze gates.
	sourceExported := false
	defer func() {
		if !sourceExported && c.databases != nil && c.databases.source != nil {
			if err := c.exportDatabase("source-database.json", c.databases.source); err != nil {
				c.t.Error(err)
			}
		}
	}()
	if c.provider.global == nil || c.databases.source == nil || c.provider.ModelProvider == nil || len(c.provider.slots) != 57 || c.policy.Task.ModelTurns != 12 || c.sourcePolicy.Task.ModelTurns != 13 || !c.deadline.After(time.Now()) || json.Unmarshal(c.corpus.Originals.RecipientSeeds, &c.seeds) != nil || len(c.seeds.Canaries) != 4 {
		c.t.Fatal("complete predeclared source/notes/skill/control cohort required")
	}
	for i := 0; i < 4; i++ {
		index := i
		if !c.t.Run(fmt.Sprintf("original_%d", i+1), func(t *testing.T) { c.original(t, index) }) {
			return
		}
	}
	for i := 0; i < 4; i++ {
		index := i
		if !c.t.Run(fmt.Sprintf("note_%d", i+1), func(t *testing.T) { c.note(t, index) }) {
			return
		}
	}
	if !c.t.Run("skill_preparation", c.promoteSkill) {
		return
	}
	if err := c.provider.writeNew("frozen-preparation.json", map[string]any{"originals": c.originals, "skill": c.skill, "source_policy": c.sourcePolicy, "control_policy": c.policy}); err != nil {
		c.t.Fatal(err)
	}
	if err := c.exportDatabase("source-database.json", c.databases.source); err != nil {
		c.t.Fatal(err)
	}
	sourceExported = true
	if err := c.databases.freeze(c.ctx); err != nil {
		c.t.Fatal(err)
	}
	for i := 9; i < 57; i++ {
		index := i
		// A failed current case is retained; its database and slot are never
		// reused. Independent remaining cases keep their frozen denominator.
		c.t.Run(fmt.Sprintf("control_%03d", i+1), func(t *testing.T) { c.control(t, index) })
		if c.ctx.Err() != nil {
			return
		}
	}
}

func (c *experienceNativeCollector) directory(t *testing.T, name string) string {
	t.Helper()
	path := filepath.Join(c.proof, name)
	if err := os.Mkdir(path, 0700); err != nil {
		t.Fatal("one-use private stage directory unavailable")
	}
	return path
}

func (c *experienceNativeCollector) recipient(t *testing.T, directory string, item experienceNativeTask, repetition int, original bool) *experienceRecipient {
	t.Helper()
	config := experienceRecipientConfig{Version: "v1", Ready: "true", Files: map[string]string{}}
	switch item.EnvironmentProfile {
	case "project-v1-normal":
	case "project-v1-receipt-outage":
		config.Outage = true
	case "project-v2-normal":
		config.Version, config.Ready = "v2", "verified"
	default:
		t.Fatal("unrecognized predeclared environment")
	}
	if !original && item.Reset == "exact_actual_original04_recipient_and_content_free_effect_snapshot" {
		r, err := newExperienceRecipient(directory, c.originals[3].Recipient.Config)
		if err != nil {
			t.Fatal(err)
		}
		if err = r.persist(c.originals[3].Recipient); err != nil {
			t.Fatal(err)
		}
		return r
	}
	for _, resource := range []string{"alpha", "beta"} {
		var body string
		if original {
			canary := c.seeds.Canaries[item.ID]
			if canary == "" || strings.Count(c.seeds.Original, "{exact_declared_canary}") != 1 {
				t.Fatal("fixed original seed unavailable")
			}
			body = strings.ReplaceAll(c.seeds.Original, "{exact_declared_canary}", canary)
		} else {
			template := c.seeds.V1
			if config.Version == "v2" {
				template = c.seeds.V2
			}
			seed := experienceDigest("butler-experience90-control-seed-v1\x00" + item.ID + "\x00" + strconv.Itoa(repetition))
			if strings.Count(template, "{sha256_control_seed}") != 1 {
				t.Fatal("fixed current seed unavailable")
			}
			body = strings.ReplaceAll(template, "{sha256_control_seed}", seed)
		}
		config.Files[resource] = body
	}
	r, err := newExperienceRecipient(directory, config)
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func (c *experienceNativeCollector) runtime(t *testing.T, pool *pgxpool.Pool, directory, anchor string, recipient *experienceRecipient, policy budget.Policy, arm string, subset []catalog.SourceRef) *experienceNativeRuntime {
	t.Helper()
	c.provider.mu.Lock()
	c.provider.casePool = pool
	c.provider.mu.Unlock()
	return newExperienceNativeRuntime(t, pool, catalog.New(pool, catalog.NewFileManifest(anchor)), anchor, directory, recipient, c.addresses, policy, c.corpus.Originals.CommonCurrentInstruction, arm, subset, c.deadline)
}

func (c *experienceNativeCollector) finish(t *testing.T, status, blocker string) {
	t.Helper()
	c.provider.mu.Lock()
	ended := c.provider.ended
	c.provider.mu.Unlock()
	if ended != nil {
		select {
		case <-ended:
		case <-c.ctx.Done():
			t.Fatal("native capture did not join before slot closure")
		}
	}
	if err := c.provider.end(status, blocker); err != nil {
		t.Fatal(err)
	}
}

func (c *experienceNativeCollector) original(t *testing.T, index int) {
	t.Helper()
	if err := c.provider.begin(index); err != nil {
		t.Fatal(err)
	}
	item := c.corpus.Originals.Tasks[index]
	directory := c.directory(t, fmt.Sprintf("original-%03d", index+1))
	recipient := c.recipient(t, directory, item, 0, true)
	defer recipient.close()
	r := c.runtime(t, c.databases.source, directory, c.anchor, recipient, c.sourcePolicy, experienceNativeArms[0], nil)
	defer r.close()
	id := experienceDigest(c.provider.window + "\x00" + c.provider.slots[index].ID)
	record := r.createTask(t, c.ctx, id, item.Instruction, item.Resource, item.PermissionProfile)
	r.acceptCurrentProfile(t, c.ctx, record)
	out := r.execute(t, c.ctx, record, id, item.Instruction, c.provider)
	if out.Result == nil {
		c.finish(t, "failed", "original_result_unavailable")
		t.Fatal("original produced no owning outcome")
	}
	original := &c.originals[index]
	original.Task, original.Run, original.Generation, original.Environment = record, out.Result.RunID, r.generation, r.environment
	if index < 2 {
		if out.Err != nil || out.Result.CurrentState != commonv1.RunState_RUN_STATE_COMPLETED {
			c.finish(t, "failed", "required_source_success_not_verified")
			t.Fatal("required independent original did not complete")
		}
		if _, err := r.verifyPreparedOriginal(c.ctx, record, original.Run, item.Resource, recipient); err != nil {
			c.finish(t, "failed", "required_source_success_not_verified")
			t.Fatal(err)
		}
	}
	verifier := &skillEvidence{tasks: r.tasks, journal: r.journal, transcript: r.history, budgets: budget.NewLedger(r.pool), results: r.results, sources: r.cat, lessons: r.lessons}
	observation, parents, err := verifier.taskObservation(experienceNativeActor(c.ctx, id+":capture"), experienceNativeOwner, original.Run, r.environment)
	if err != nil {
		c.finish(t, "failed", "original_snapshot_unavailable")
		t.Fatal(err)
	}
	original.Observation = observation
	original.HTTPStatuses = recipient.httpStatusReceipts()
	for _, action := range observation.Actions {
		if action.Effect != actionauth.StateUnknown {
			continue
		}
		effect, e := r.journal.GetEffect(c.ctx, action.ID)
		if e != nil {
			t.Fatal(e)
		}
		original.Unresolved = append(original.Unresolved, experienceNativeCurrentEffect{ActionID: effect.ActionID, Tool: effect.Binding.ToolName, Resource: effect.Binding.Resource, Lookup: strings.ReplaceAll(effect.Policy.VerificationURL, "{idempotency_key}", effect.IdempotencyKey), Fingerprint: effect.Fingerprint, Effect: effect.State})
	}
	owner := experienceNativeActor(c.ctx, id+":original-provenance")
	tx, err := r.pool.Begin(owner)
	if err != nil {
		t.Fatal(err)
	}
	graph, err := r.cat.CanonicalSourcesTx(owner, tx, parents)
	tx.Rollback(owner)
	if err != nil {
		t.Fatal(err)
	}
	for _, parent := range graph {
		if strings.HasPrefix(parent.ID, "message:") {
			var role string
			if r.pool.QueryRow(c.ctx, `SELECT role FROM messages WHERE 'message:'||message_id=$1 AND run_id=$2`, parent.ID, original.Run).Scan(&role) == nil && role == "user" {
				original.OriginalSource = parent
				break
			}
		}
	}
	if original.OriginalSource.ID == "" {
		c.finish(t, "failed", "original_primary_source_unavailable")
		t.Fatal("owning original message source missing")
	}
	if index == 2 {
		original.Reconciliation = c.resumeAndReconcile(t, r, record, original, recipient)
	}
	if index == 3 {
		if observation.TerminalAttempt == nil || observation.ResultID != "" || observation.AgentReport != "" {
			c.finish(t, "failed", "required_terminal_unknown_unavailable")
			t.Fatal("original04 did not retain terminal UNKNOWN without a final")
		}
		var unknown bool
		for _, effect := range observation.Actions {
			unknown = unknown || effect.Effect == actionauth.StateUnknown
		}
		if !unknown {
			c.finish(t, "failed", "required_terminal_unknown_unavailable")
			t.Fatal("original04 has no actual unknown action")
		}
	}
	original.Recipient = recipient.snapshot()
	if err = c.provider.writeNew(fmt.Sprintf("original-%03d.json", index+1), original); err != nil {
		t.Fatal(err)
	}
	c.finish(t, "completed", "")
}

func (c *experienceNativeCollector) ownerAPI(t *testing.T, r *experienceNativeRuntime, modelJobs *jobs.Store) (string, string, func()) {
	t.Helper()
	token := strings.Repeat("experience-native-private-owner-", 3)
	s := httptest.NewServer((&api.MemoryCatalogServer{Store: r.cat, Auth: api.NewOwnerControlAuth(token, experienceNativeOwner), ModelJobs: modelJobs, ModelWorkerConfigured: modelJobs != nil, ExperiencePolicies: configureExperiencePolicies(r.lessons, modelJobs, r.tasks, r.history, r.results, r.cat), Experience: r.lessons}).Handle())
	return s.URL, token, s.Close
}

func (c *experienceNativeCollector) note(t *testing.T, index int) {
	t.Helper()
	if err := c.provider.begin(4 + index); err != nil {
		t.Fatal(err)
	}
	directory := c.directory(t, fmt.Sprintf("note-%03d", index+1))
	original := &c.originals[index]
	recipient, err := newExperienceRecipient(directory, original.Recipient.Config)
	if err != nil {
		t.Fatal(err)
	}
	defer recipient.close()
	if err = recipient.persist(original.Recipient); err != nil {
		t.Fatal(err)
	}
	r := c.runtime(t, c.databases.source, directory, c.anchor, recipient, c.sourcePolicy, experienceNativeArms[0], nil)
	defer r.close()
	if r.generation != original.Generation || r.environment != original.Environment {
		t.Fatal("source restart changed frozen environment or extension generation")
	}
	store := jobs.New(r.pool, r.cat, r.budgets, nil)
	endpoint, token, closeAPI := c.ownerAPI(t, r, store)
	defer closeAPI()
	q := experience.PolicyRequest{RunID: original.Run, Reconciliation: original.Reconciliation, Policy: jobs.ModelPolicyRequest{ID: experienceDigest(c.provider.window + ":note:" + original.Run), Privacy: "cloud_allowed", IntervalSeconds: 60, MaxJobs: 1, Job: jobs.Request{Deadline: c.deadline, Allowance: budget.Units{ModelTurns: 1, Tokens: 1081344, ActiveMillis: 300000, ProviderInputBytes: 8388608}}}}
	screenOwnerAPI(t, endpoint, token, http.MethodPost, "/api/v4/memory/model-experience-policies", q, nil)
	screenOwnerAPI(t, endpoint, token, http.MethodPost, "/api/v4/memory/model-worker-control", map[string]string{"state": "enabled"}, nil)
	if err = store.PollModelPolicies(c.ctx, func(string) bool { return true }); err != nil {
		t.Fatal(err)
	}
	worker := &jobs.ModelWorker{Jobs: store, Provider: c.provider, Model: "gemini-3.8-flash-high", Handlers: map[string]jobs.ModelHandler{experience.Version: experience.Handler{}}}
	worked, processErr := worker.ProcessOne(c.ctx)
	if !worked || processErr != nil {
		c.finish(t, "failed", "actual_note_not_published")
		t.Fatal("one actual model note failed", processErr)
	}
	var source, claim string
	if err = r.pool.QueryRow(c.ctx, `SELECT source_id,claim_id FROM memory_experience_lessons WHERE run_id=$1`, original.Run).Scan(&source, &claim); err != nil {
		c.finish(t, "failed", "actual_note_not_published")
		t.Fatal(err)
	}
	screenOwnerAPI(t, endpoint, token, http.MethodPost, "/api/v4/memory/decisions", catalog.Decision{ClaimID: claim}, nil)
	note, err := r.cat.Get(experienceNativeActor(c.ctx, "accept-note:"+original.Run), source)
	if err != nil {
		t.Fatal(err)
	}
	original.Lesson = screenSourceRef(note)
	if err = c.provider.writeNew(fmt.Sprintf("note-%03d.json", index+1), note); err != nil {
		t.Fatal(err)
	}
	c.finish(t, "completed", "")
}

func (c *experienceNativeCollector) grant(t *testing.T, r *experienceNativeRuntime, record task.Record, roots []catalog.SourceRef) {
	t.Helper()
	owner := experienceNativeActor(c.ctx, "current-memory-grants:"+record.TaskID)
	tx, err := r.pool.Begin(owner)
	if err != nil {
		t.Fatal(err)
	}
	refs, err := r.cat.CanonicalSourcesTx(owner, tx, roots)
	tx.Rollback(owner)
	if err != nil {
		t.Fatal(err)
	}
	for _, ref := range refs {
		if err = r.cat.GrantSource(owner, record.TaskID, ref.ID, "memory_read", record.Revision); err != nil {
			t.Fatal(err)
		}
	}
}

func (c *experienceNativeCollector) control(t *testing.T, index int) {
	t.Helper()
	if err := c.provider.begin(index); err != nil {
		t.Fatal(err)
	}
	slot := c.provider.slots[index]
	var item experienceNativeTask
	for _, candidate := range c.corpus.Controls.Cases {
		if candidate.ID == slot.CaseID {
			item = candidate
			break
		}
	}
	if item.ID == "" {
		t.Fatal("predeclared control missing")
	}
	pool, err := c.databases.clone(c.ctx, index)
	if err != nil {
		c.finish(t, "failed", "control_database_preparation_failed")
		t.Fatal(err)
	}
	defer pool.Close()
	defer func() {
		if err := c.exportDatabase(fmt.Sprintf("case-%03d-database.json", slot.Sequence), pool); err != nil {
			t.Error(err)
		}
	}()
	directory := c.directory(t, fmt.Sprintf("case-%03d", slot.Sequence))
	anchor := filepath.Join(directory, "deletion-anchor")
	raw, err := os.ReadFile(c.anchor)
	if err != nil || os.WriteFile(anchor, raw, 0600) != nil {
		t.Fatal("frozen external deletion anchor unavailable")
	}
	recipient := c.recipient(t, directory, item, slot.Repetition, false)
	defer recipient.close()
	refs := []catalog.SourceRef{}
	for _, original := range c.originals {
		refs = append(refs, original.Lesson)
	}
	r := c.runtime(t, pool, directory, anchor, recipient, c.policy, slot.Arm, refs)
	defer r.close()
	if item.Reset == "empty_patch_store_original_readme_and_delete_original01_source_after_frozen_notes_skills" {
		original := c.originals[0].OriginalSource
		if _, err = r.cat.Delete(experienceNativeActor(c.ctx, "delete-original-before-current-input"), original.ID, "delete", original.Revision); err != nil {
			t.Fatal(err)
		}
	}
	id := experienceDigest(c.provider.window + "\x00" + slot.ID)
	record := r.createTask(t, c.ctx, id, item.Instruction, item.Resource, item.PermissionProfile)
	r.acceptCurrentProfile(t, c.ctx, record)
	if slot.Arm != experienceNativeArms[0] {
		roots := []catalog.SourceRef{}
		for i, original := range c.originals {
			if c.corpus.Originals.Tasks[i].Resource != item.Resource {
				continue
			}
			if _, e := r.cat.Get(experienceNativeActor(c.ctx, "current-source-grants"), original.Lesson.ID); e == nil {
				roots = append(roots, original.Lesson)
			}
		}
		if slot.Arm == experienceNativeArms[2] {
			if _, e := r.cat.Get(experienceNativeActor(c.ctx, "current-skill-grants"), c.skill.BodySource.ID); e == nil {
				all := true
				for i := 0; i < 2; i++ {
					all = all && c.corpus.Originals.Tasks[i].Resource == item.Resource
				}
				if all {
					roots = append(roots, c.skill.BodySource)
				}
			}
		}
		c.grant(t, r, record, roots)
	}
	instruction := item.Instruction
	if item.CurrentState != nil {
		unknown := c.originals[3].Unresolved
		if len(unknown) == 0 {
			t.Fatal("required actual original04 effect reference missing")
		}
		state, _ := json.Marshal(map[string]any{"effect": item.CurrentState.Effect, "effects": unknown, "new_prepare_allowed": item.CurrentState.NewPrepareAllowed})
		instruction += "\nCurrent owner-declared effect state (DATA): " + string(state)
	}
	out := r.execute(t, c.ctx, record, id, instruction, c.provider)
	status, blocker := "completed", ""
	if out.Err != nil || out.Result == nil || out.Result.CurrentState != commonv1.RunState_RUN_STATE_COMPLETED {
		status, blocker = "failed", "owning_current_task_not_completed"
	}
	if err = c.provider.writeNew(fmt.Sprintf("case-%03d-outcome.json", slot.Sequence), map[string]any{"slot": slot, "task": record, "result": out.Result, "flow_error": out.Err != nil, "recipient_audit": recipient.snapshot(), "environment": r.environment, "extension_generation": r.generation}); err != nil {
		t.Fatal(err)
	}
	c.finish(t, status, blocker)
}

func (c *experienceNativeCollector) exportDatabase(name string, pool *pgxpool.Pool) error {
	ctx, stop := context.WithTimeout(context.Background(), 15*time.Second)
	defer stop()
	out := map[string]json.RawMessage{}
	for _, table := range []string{"execution_budget_scopes", "execution_budget_reservations", "execution_model_attempts", "memory_model_attempts", "memory_aux_jobs", "memory_model_policies", "tasks", "runs", "accepted_inputs", "action_effects", "action_effect_evidence", "action_effect_dispatches", "tool_calls", "run_final_results", "memory_experience_lessons", "skill_catalog", "skill_versions", "skill_states"} {
		var raw json.RawMessage
		if err := pool.QueryRow(ctx, `SELECT COALESCE(jsonb_agg(to_jsonb(t)),'[]'::jsonb) FROM `+table+` t`).Scan(&raw); err != nil {
			return fmt.Errorf("complete private database export failed: %s", table)
		}
		out[table] = raw
	}
	var historyExists bool
	if err := pool.QueryRow(ctx, `SELECT to_regclass('experience_native_source_history') IS NOT NULL`).Scan(&historyExists); err != nil {
		return err
	}
	raw := json.RawMessage(`[]`)
	if historyExists {
		if err := pool.QueryRow(ctx, `SELECT COALESCE(jsonb_agg(to_jsonb(t)),'[]'::jsonb) FROM experience_native_source_history t`).Scan(&raw); err != nil {
			return err
		}
	}
	out["experience_native_source_history"] = raw
	return c.provider.writeNew(name, out)
}
