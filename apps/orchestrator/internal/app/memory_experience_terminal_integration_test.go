package app

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/butler/butler/apps/orchestrator/internal/api"
	"github.com/butler/butler/internal/actionauth"
	"github.com/butler/butler/internal/budget"
	"github.com/butler/butler/internal/memory/catalog"
	"github.com/butler/butler/internal/memory/experience"
	"github.com/butler/butler/internal/memory/jobs"
	"github.com/butler/butler/internal/memory/memorytest"
	"github.com/butler/butler/internal/transport"
)

// Controlled note output exercises the actual stopped-input adapter, owner HTTP
// API, worker, reservations and Catalog publication. It proves no native quality.
type experienceTerminalControlProvider struct {
	transport.ModelProvider
	note     experience.Notes
	before   func()
	calls    int
	required string
}

func (*experienceTerminalControlProvider) Name() string { return "antigravity" }
func (*experienceTerminalControlProvider) InferenceLimits() (int64, int64) {
	return 1048576, 32768
}
func (*experienceTerminalControlProvider) InferenceTimeout() time.Duration { return 300 * time.Second }
func (*experienceTerminalControlProvider) CancelRun(context.Context, transport.CancelRunRequest) (*transport.TransportEvent, error) {
	return nil, nil
}
func (p *experienceTerminalControlProvider) StartRun(ctx context.Context, request transport.StartRunRequest) (transport.EventStream, error) {
	if err := transport.CheckInferenceAuthority(ctx); err != nil {
		return nil, err
	}
	if err := transport.CheckInferenceDispatch(ctx); err != nil {
		return nil, err
	}
	raw, _ := json.Marshal(request.InputItems)
	required := p.required
	if required == "" {
		required = "terminal_attempt"
	}
	if !strings.Contains(string(raw), required) {
		return nil, experience.ErrEvidence
	}
	p.calls++
	if p.before != nil {
		p.before()
	}
	content, _ := json.Marshal(p.note)
	stream := make(chan transport.TransportEvent, 1)
	stream <- transport.NewAssistantFinalEvent(request.Context.RunID, p.Name(), transport.AssistantFinal{Content: string(content), Usage: &transport.Usage{InputTokens: 30, OutputTokens: 20, TotalTokens: 60}})
	close(stream)
	return stream, nil
}

func TestExperienceStoppedOriginalOwnerNotePipelineIntegration(t *testing.T) {
	for _, scenario := range []string{"rejected", "lost_receipt", "rebase_before_policy", "late_input", "late_pause", "late_delete"} {
		t.Run(scenario, func(t *testing.T) {
			pool := memorytest.Database(t)
			ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
			defer cancel()
			owner := experienceNativeActor(ctx, "explicit-stopped-original-owner")
			dir := t.TempDir()
			anchor := filepath.Join(dir, "anchor")
			manifest := catalog.NewFileManifest(anchor)
			if err := manifest.Initialize(); err != nil {
				t.Fatal(err)
			}
			cat := catalog.New(pool, manifest)
			recipient, err := newExperienceRecipient(dir, experienceRecipientConfig{Version: "v1", Ready: "true", Files: map[string]string{"alpha": "Controlled current original\nready=false\n", "beta": "Controlled other resource\nready=false\n"}, Outage: scenario != "rejected"})
			if err != nil {
				t.Fatal(err)
			}
			defer recipient.close()
			r := newExperienceNativeRuntime(t, pool, cat, anchor, dir, recipient, newExperienceNativeAddresses(t), experienceNativeSourcePolicy(), "Выполни только текущее поручение; не применяй артефакт; неизвестный эффект не считай успехом.", experienceNativeArms[0], nil, time.Now().Add(time.Hour))
			id := experienceDigest("controlled-stopped-original")
			record := r.createTask(t, owner, id, "Prepare a bounded unapplied current artifact", "alpha", "prepare_and_verify_exact_resource")
			r.acceptCurrentProfile(t, owner, record)
			original := &experienceOriginalControlProvider{}
			if scenario == "rejected" {
				original.ready = "maybe"
			}
			out := r.execute(t, ctx, record, id, "Inspect alpha and prepare the requested current artifact without applying it", original)
			if out.Err != nil || out.Result == nil || out.Result.ReceiptStatus != "blocked" || out.Result.ErrorCode != "effect_unknown" || original.phase != 2 {
				t.Fatal("ordinary original did not stop on its actual unknown effect", out.Err, out.Result, original.phase)
			}
			if _, err = r.results.GetByRun(owner, out.Result.RunID); err == nil {
				t.Fatal("stopped source invented an assistant final")
			}
			evidence := &skillEvidence{tasks: r.tasks, journal: r.journal, transcript: r.history, budgets: budget.NewLedger(pool), results: r.results, sources: cat, lessons: r.lessons}
			observation, parents, err := evidence.taskObservation(owner, experienceNativeOwner, out.Result.RunID, r.environment)
			if err != nil || observation.TerminalAttempt == nil || observation.ResultID != "" || observation.AgentReport != "" || observation.OwnerVerdict != "unknown" || len(observation.Actions) != 2 || len(parents) == 0 {
				t.Fatal("owning terminal observation lost actual evidence", err)
			}
			unknown := 0
			for _, action := range observation.Actions {
				if action.Effect == actionauth.StateUnknown {
					unknown++
				}
			}
			if unknown != 1 || recipient.snapshot().Dispatches != 1 {
				t.Fatal("hidden recipient audit changed UNKNOWN or replayed source")
			}
			jobStore := jobs.New(pool, cat, r.budgets, nil)
			provider := &experienceTerminalControlProvider{note: experience.Notes{Summary: "An actual stopped original; recipient outcome remains unknown", When: []string{"The exact current environment and evidence"}, NotApplicable: []string{"No independent successful task verification"}, Steps: []experience.Step{{ActionID: observation.Actions[0].ID, Comment: "An actual original action; typed effect is authoritative"}}}}
			worker := &jobs.ModelWorker{Jobs: jobStore, Provider: provider, Model: "gemini-3.8-flash-high", Handlers: map[string]jobs.ModelHandler{experience.Version: experience.Handler{}}}
			token := strings.Repeat("controlled-private-owner-", 3)
			server := httptest.NewServer((&api.MemoryCatalogServer{Store: cat, Auth: api.NewOwnerControlAuth(token, experienceNativeOwner), ModelJobs: jobStore, ModelWorkerConfigured: true, ExperiencePolicies: &experiencePolicies{r.lessons, jobStore, evidence}, Experience: r.lessons}).Handle())
			defer server.Close()
			q := experience.PolicyRequest{RunID: out.Result.RunID, Policy: jobs.ModelPolicyRequest{ID: "controlled-terminal-note", Privacy: "cloud_allowed", IntervalSeconds: 60, MaxJobs: 1, Job: jobs.Request{Deadline: time.Now().Add(10 * time.Minute), Allowance: budget.Units{ModelTurns: 1, Tokens: 1081344, ActiveMillis: 300000, ProviderInputBytes: 8388608}}}}
			if scenario == "rebase_before_policy" {
				sealed, e := r.lessons.Seal(owner, observation, parents)
				if e != nil {
					t.Fatal(e)
				}
				if _, e = pool.Exec(owner, `UPDATE tasks SET revision=revision+1 WHERE task_id=$1`, record.TaskID); e != nil {
					t.Fatal(e)
				}
				q.Policy.Handler, q.Policy.Trigger = experience.Version, "source_changed"
				q.Policy.Job.TaskID, q.Policy.Job.OriginalRunID = record.TaskID, out.Result.RunID
				q.Policy.Job.Sources = []catalog.SourceRef{sealed}
				if _, e = jobStore.CreateTerminalExperiencePolicy(owner, q.Policy, sealed.ID); e == nil {
					t.Fatal("fresh policy silently rebased an old stopped attempt onto a new task revision")
				}
				var attempts int
				if pool.QueryRow(ctx, `SELECT count(*) FROM memory_model_attempts`).Scan(&attempts) != nil || attempts != 0 {
					t.Fatal("stale stopped attempt spent a note quote")
				}
				return
			}
			screenOwnerAPI(t, server.URL, token, http.MethodPost, "/api/v4/memory/model-experience-policies", q, nil)
			screenOwnerAPI(t, server.URL, token, http.MethodPost, "/api/v4/memory/model-worker-control", map[string]string{"state": "enabled"}, nil)
			var snapshot string
			if err = pool.QueryRow(ctx, `SELECT source_id FROM memory_experience_terminal_attempts`).Scan(&snapshot); err != nil {
				t.Fatal(err)
			}
			sealed, err := cat.Get(owner, snapshot)
			if err != nil {
				t.Fatal("sealed terminal source unavailable", err)
			}
			generic := q.Policy
			generic.ID, generic.Handler, generic.Trigger = "ordinary-cannot-borrow-terminal", experience.Version, "source_changed"
			generic.Job.TaskID, generic.Job.OriginalRunID = record.TaskID, out.Result.RunID
			generic.Job.Sources = []catalog.SourceRef{screenSourceRef(sealed)}
			if _, err = jobStore.CreateModelPolicy(owner, generic); err == nil {
				t.Fatal("ordinary model policy borrowed the blocked-task capability")
			}
			// Even a still-active task whose attempt failed cannot opt into this
			// capability by serializing the handler and genuine source IDs alone.
			if _, err = pool.Exec(owner, `UPDATE tasks SET status='active' WHERE task_id=$1`, record.TaskID); err != nil {
				t.Fatal(err)
			}
			if _, err = jobStore.CreateModelPolicy(owner, generic); err == nil {
				t.Fatal("generic active-task policy bypassed terminal snapshot authority")
			}
			if _, err = pool.Exec(owner, `UPDATE tasks SET status='blocked' WHERE task_id=$1`, record.TaskID); err != nil {
				t.Fatal(err)
			}
			if err = jobStore.PollModelPolicies(ctx, func(string) bool { return true }); err != nil {
				t.Fatal(err)
			}
			switch scenario {
			case "late_input":
				provider.before = func() {
					if _, e := pool.Exec(owner, `UPDATE accepted_inputs SET checkpoint_json=checkpoint_json||'{"controlled_late_change":true}'::jsonb WHERE input_id=$1`, observation.TerminalAttempt.InputID); e != nil {
						t.Fatal(e)
					}
				}
			case "late_pause":
				provider.before = func() {
					if _, e := pool.Exec(owner, `UPDATE tasks SET status='paused',revision=revision+1 WHERE task_id=$1`, record.TaskID); e != nil {
						t.Fatal(e)
					}
				}
			case "late_delete":
				provider.before = func() {
					if _, e := cat.Delete(owner, parents[0].ID, "delete", parents[0].Revision); e != nil {
						t.Fatal(e)
					}
				}
			}
			worked, processErr := worker.ProcessOne(ctx)
			if !worked || provider.calls != 1 {
				t.Fatal("one fresh explicitly authorized note attempt required", worked, processErr, provider.calls)
			}
			var count, attempts, observed int
			if pool.QueryRow(ctx, `SELECT count(*) FROM memory_experience_lessons`).Scan(&count) != nil || pool.QueryRow(ctx, `SELECT count(*),count(usage) FROM memory_model_attempts`).Scan(&attempts, &observed) != nil || attempts != 1 || observed != 1 {
				t.Fatal("original/failed note accounting lost known usage")
			}
			if strings.HasPrefix(scenario, "late_") {
				if count != 0 {
					t.Fatal("late original/authority/privacy change published note")
				}
				return
			}
			if processErr != nil || count != 1 {
				t.Fatal("actual stopped original did not publish one unaccepted note", processErr, count)
			}
			var claim string
			if err = pool.QueryRow(ctx, `SELECT claim_id FROM memory_experience_lessons`).Scan(&claim); err != nil {
				t.Fatal(err)
			}
			screenOwnerAPI(t, server.URL, token, http.MethodPost, "/api/v4/memory/decisions", catalog.Decision{ClaimID: claim}, nil)
			var body string
			if err = pool.QueryRow(ctx, `SELECT content FROM memory_episodes`).Scan(&body); err != nil {
				t.Fatal(err)
			}
			var lesson experience.Lesson
			if json.Unmarshal([]byte(body), &lesson) != nil || lesson.Verification != "task_unverified" || lesson.Observation.TerminalAttempt == nil || lesson.Observation.ResultID != "" || lesson.Observation.AgentReport != "" || lesson.Observation.OwnerVerdict != "unknown" || lesson.InstructionAuthority {
				t.Fatal("accepted note invented original final or successful verification")
			}
			migration, err := os.ReadFile("../../../../migrations/080_episodic_terminal_attempts.down.sql")
			if err != nil {
				t.Fatal(err)
			}
			if _, err = pool.Exec(ctx, string(migration)); err == nil {
				t.Fatal("schema rollback dropped populated stopped-attempt fences")
			}
		})
	}
}
