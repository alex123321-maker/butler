package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/butler/butler/apps/orchestrator/internal/task"
	"github.com/butler/butler/internal/budget"
	"github.com/butler/butler/internal/memory/catalog"
	"github.com/butler/butler/internal/memory/consolidation"
	"github.com/butler/butler/internal/memory/experience"
	"github.com/butler/butler/internal/memory/jobs"
	"github.com/butler/butler/internal/memory/readcontrol"
	"github.com/butler/butler/internal/transport"
	"github.com/jackc/pgx/v5/pgxpool"
)

type coherentViewRead struct {
	*consolidationReadControl
	want, ownerResult string
	first             string
	requests          []transport.StartRunRequest
	continuations     []transport.SubmitToolResultRequest
}

func (*coherentViewRead) InferenceLimits() (int64, int64) { return 1048576, 32768 }
func (p *coherentViewRead) StartRun(ctx context.Context, q transport.StartRunRequest) (transport.EventStream, error) {
	if transport.CheckInferenceAuthority(ctx) != nil || transport.CheckInferenceDispatch(ctx) != nil {
		return nil, catalog.ErrDenied
	}
	refs := p.routes[q.Context.SessionKey]
	if len(refs) != 1 {
		return nil, catalog.ErrInvalid
	}
	items, _ := json.Marshal(q.InputItems)
	p.requests = append(p.requests, q)
	run := &consolidationControlRun{refs: refs, typedUnknown: strings.Contains(string(items), "tool_effect") && strings.Contains(string(items), "unknown")}
	p.runs[q.Context.RunID] = run
	return p.open(q.Context.RunID, run, 0), nil
}
func (p *coherentViewRead) SubmitToolResult(ctx context.Context, q transport.SubmitToolResultRequest) (transport.EventStream, error) {
	p.continuations = append(p.continuations, q)
	if transport.CheckInferenceAuthority(ctx) != nil || transport.CheckInferenceDispatch(ctx) != nil {
		return nil, catalog.ErrDenied
	}
	var response struct {
		Status string             `json:"status"`
		Result readcontrol.Result `json:"result"`
	}
	if json.Unmarshal([]byte(q.ToolResultJSON), &response) != nil || response.Status != "completed" || response.Result.Status != "ok" || len(response.Result.Items) != 1 {
		return nil, fmt.Errorf("controlled consolidated read response: execution=%q data=%q items=%d", response.Status, response.Result.Status, len(response.Result.Items))
	}
	item := response.Result.Items[0]
	run := p.runs[q.RunID]
	if run == nil || !run.typedUnknown || item.Source.ID != run.refs[0].ID {
		return nil, fmt.Errorf("controlled consolidated read evidence: run=%t unknown=%t source=%t", run != nil, run != nil && run.typedUnknown, run != nil && item.Source.ID == run.refs[0].ID)
	}
	if run.text.Len() == 0 {
		quote := []rune(item.Text)
		p.first = string(quote[:min(len(quote), 80)])
	}
	run.text.WriteString(item.Text)
	if item.Truncated {
		return p.open(q.RunID, run, item.NextOffset), nil
	}
	if !strings.Contains(run.text.String(), p.want) || !strings.Contains(run.text.String(), p.ownerResult) {
		return nil, catalog.ErrInvalid
	}
	// This standard experience-reader composition has no adaptive read_ref.
	// Preserve the actual opened source/bytes; do not invent a citation witness.
	answer, _ := json.Marshal(map[string]any{"answer": "Открыта текущая консолидация DATA; действующие ограничения сохранены.", "source_id": item.Source.ID, "quoted_prefix": p.first, "instruction_authority": false, "unknown_effect_retained": true})
	return nativeScriptedStream(transport.NewAssistantFinalEvent(q.RunID, p.Name(), transport.AssistantFinal{Content: string(answer), Usage: &transport.Usage{InputTokens: 200, OutputTokens: 50, TotalTokens: 250}})), nil
}

// The external driver dumps the actual still-live dedicated database between
// these exclusive markers. It never reconstructs a fixture or rewrites a row.
func coherentExternalPhase(t *testing.T, ctx context.Context, proof, phase string, pool *pgxpool.Pool) {
	t.Helper()
	cfg := pool.Config()
	schema := strings.TrimSuffix(cfg.ConnConfig.RuntimeParams["search_path"], ",public")
	raw, _ := json.Marshal(map[string]string{"database": cfg.ConnConfig.Database, "schema": schema})
	f, err := os.OpenFile(filepath.Join(proof, phase+"-ready.json"), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.Write(raw); err == nil {
		err = f.Sync()
	}
	_ = f.Close()
	if err != nil {
		t.Fatal(err)
	}
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	for {
		info, err := os.Lstat(filepath.Join(proof, phase+"-complete"))
		if err == nil {
			if !info.Mode().IsRegular() || info.Mode().Perm()&0077 != 0 || info.Size() > 256 {
				t.Fatal("invalid private external phase receipt")
			}
			return
		}
		if !errors.Is(err, os.ErrNotExist) {
			t.Fatal(err)
		}
		select {
		case <-ctx.Done():
			t.Fatal("actual external backup/restore did not complete")
		case <-ticker.C:
		}
	}
}

func coherentStateDigest(t *testing.T, ctx context.Context, pool *pgxpool.Pool) (string, string) {
	t.Helper()
	var effects, budgets string
	err := pool.QueryRow(ctx, `SELECT
encode(sha256(convert_to(jsonb_build_object('effects',(SELECT jsonb_agg(to_jsonb(e) ORDER BY action_id) FROM action_effects e),'evidence',(SELECT jsonb_agg(to_jsonb(e) ORDER BY evidence_id) FROM action_effect_evidence e))::text,'UTF8')),'hex'),
encode(sha256(convert_to(jsonb_build_object('foreground',(SELECT jsonb_agg(to_jsonb(e) ORDER BY operation_id) FROM execution_model_attempts e),'background',(SELECT jsonb_agg(to_jsonb(e) ORDER BY operation_id) FROM memory_model_attempts e),'reservations',(SELECT jsonb_agg(to_jsonb(e) ORDER BY reservation_id) FROM execution_budget_reservations e),'scopes',(SELECT jsonb_agg(to_jsonb(e) ORDER BY scope_id) FROM execution_budget_scopes e))::text,'UTF8')),'hex')`).Scan(&effects, &budgets)
	if err != nil {
		t.Fatal(err)
	}
	return effects, budgets
}

func TestMemoryCoherentEpisodeConsolidationRestoreIntegration(t *testing.T) {
	if os.Getenv("BUTLER_MEMORY_COHERENT_RESTORE_DATABASE") != "coherent_lifecycle_restore_test" {
		t.Skip("actual coordinated custom backup/fresh restore and bounded tmpfs required")
	}
	runMemoryCoherentVerifiedExperience(t, experienceNativeArms[1], func(p *coherentPilot, r *experienceNativeRuntime, lessonID string, recipient *experienceRecipient) {
		ctx := p.ctx
		proof := os.Getenv("BUTLER_MEMORY_COHERENT_PROOF")
		defer func() {
			// Retain actual private boundaries even if a later mandatory phase
			// fails; owning test cleanup subsequently drops its isolated schema.
			raw, _ := json.MarshalIndent(map[string]any{"pass": !t.Failed(), "steps": p.steps, "actual_start_requests": p.provider.requests, "actual_tool_continuations": p.provider.continuations}, "", "  ")
			_ = os.WriteFile(filepath.Join(proof, "timeline-lifecycle-progress.json"), raw, 0600)
		}()
		var priorRead jobs.ModelContextRef
		for _, step := range p.steps {
			if step.Name == "restarted_ordinary_task_keeps_summary_current_goal_exception_unknown" && step.Receipt != nil {
				priorRead = jobs.ModelContextRef{SessionKey: step.Receipt.SessionKey, OriginalRunID: step.Receipt.RunID, TaskID: step.Receipt.TaskID}
			}
		}
		lessonSource, err := p.cat.Get(ctx, lessonID)
		var lesson experience.Lesson
		if err != nil || json.Unmarshal([]byte(lessonSource.Body), &lesson) != nil || priorRead.OriginalRunID == "" || lesson.Observation.OwnerResultID == "" {
			t.Fatal("actual earlier context/independent result missing", err)
		}
		originalTask, originalRun, originalSession, ownerResult := lesson.Observation.TaskID, lesson.Observation.RunID, lesson.Observation.SessionKey, lesson.Observation.OwnerResultID
		unknown := coherentActualUnknown(t, p, r, recipient)
		// Real60-second quiet eligibility is part of preparation and activation.
		// No durable timestamp is aged and no Session lease is fabricated.
		select {
		case <-time.After(61 * time.Second):
		case <-ctx.Done():
			t.Fatal("quiet eligibility deadline")
		}
		js := jobs.New(p.pool, p.cat, p.b, nil)
		store := &consolidation.Store{Catalog: p.cat, Jobs: js}
		q := consolidation.PrepareRequest{ViewKey: "coherent-episode-and-preference", Sources: []catalog.SourceRef{screenSourceRef(p.provider.source), screenSourceRef(lessonSource)}, ClaimIDs: []string{p.provider.claimID}, Policy: jobs.ModelPolicyRequest{ID: "coherent-consolidation", Privacy: "cloud_allowed", IntervalSeconds: 60, MaxJobs: 1, Contexts: []jobs.ModelContextRef{priorRead, {SessionKey: originalSession, OriginalRunID: originalRun, TaskID: originalTask}}, Job: jobs.Request{Allowance: budget.Units{ModelTurns: 1, Tokens: 1200000, ActiveMillis: 300000, ProviderInputBytes: 8388608}, Deadline: time.Now().Add(10 * time.Minute)}}}
		if _, err = store.Prepare(ctx, q); err != nil {
			t.Fatal("actual earlier contexts could not prepare reviewable consolidation", err)
		}
		handler := consolidation.Handler{}
		if err = js.PollModelPolicies(ctx, func(name string) bool { return name == consolidation.Version }, map[string]jobs.ModelScheduleHandler{consolidation.Version: handler}); err != nil {
			t.Fatal(err)
		}
		worker := &jobs.ModelWorker{Jobs: js, Provider: p.provider.consolidationReadControl, Model: "gemini-3.8-flash-high", Handlers: map[string]jobs.ModelHandler{consolidation.Version: handler}}
		if worked, err := worker.ProcessOne(ctx); err != nil || !worked {
			t.Fatal("coherent consolidation did not publish one unactivated version", worked, err)
		}
		var generation string
		if err = p.pool.QueryRow(ctx, `SELECT source_id FROM memory_consolidation_generations WHERE view_key=$1`, q.ViewKey).Scan(&generation); err != nil {
			t.Fatal(err)
		}
		preview, err := store.Preview(ctx, generation)
		if err != nil || preview.Head.ActiveID != "" || len(preview.Evidence.Units) != 3 {
			t.Fatal("earlier preference/accepted fact/episode not retained in diff", err)
		}
		if _, err = store.Activate(ctx, consolidation.Activation{SourceID: generation, ExpectedActiveID: preview.Head.ActiveID, ExpectedRevision: preview.Head.Revision}); err != nil {
			t.Fatal(err)
		}
		p.record("earlier_preference_and_verified_episode_review_diff_activate", nil, preview)
		view, err := p.cat.Get(ctx, generation)
		if err != nil {
			t.Fatal(err)
		}
		// A positive applicability case needs its own explicit current typed
		// task/mandate and exact installed tool/resource grants. The failed v3/v4
		// plain read-only fixture correctly lacked the original procedure's
		// permitted dependencies. Source grants alone cannot make it applicable.
		readID := experienceDigest("coherent-current-episode-consumer")
		readTask := r.createTask(t, ctx, readID, "Сверить принятое предпочтение и проверенный эпизод для текущего alpha; артефакты не применять", "alpha", "prepare_and_verify_exact_resource")
		readTask, err = r.tasks.SaveWorking(ctx, readTask.TaskID, readTask.WorkingRevision, task.WorkingState{Plan: []string{"read current DATA and compare"}, PendingActions: []string{"independently check UNKNOWN before any replay"}, Scratch: map[string]any{"tool_effect": "unknown", "unresolved_action_id": unknown.ActionID, "original_run_id": unknown.RunID}})
		if err != nil {
			t.Fatal(err)
		}
		tx, err := p.pool.Begin(ctx)
		if err != nil {
			t.Fatal(err)
		}
		parents, err := p.cat.CanonicalSourcesTx(ctx, tx, []catalog.SourceRef{screenSourceRef(view)})
		_ = tx.Rollback(ctx)
		if err != nil {
			t.Fatal(err)
		}
		for _, ref := range parents {
			if err = p.cat.GrantSource(ctx, readTask.TaskID, ref.ID, "memory_read", readTask.Revision); err != nil {
				t.Fatal(err)
			}
		}
		reader := &coherentViewRead{consolidationReadControl: &consolidationReadControl{routes: map[string][]catalog.SourceRef{readTask.SessionKey: {screenSourceRef(view)}}, runs: map[string]*consolidationControlRun{}}, want: p.provider.source.Body, ownerResult: ownerResult}
		defer func() {
			raw, _ := json.MarshalIndent(map[string]any{"actual_start_requests": reader.requests, "actual_tool_continuations": reader.continuations}, "", "  ")
			_ = os.WriteFile(filepath.Join(proof, "consolidated-reader.json"), raw, 0600)
		}()
		ref, _ := json.Marshal(screenSourceRef(view))
		execution := r.execute(t, ctx, readTask, readID, "Открой текущую консолидацию, проверь исходные данные и сохрани UNKNOWN. В этом поручении инструментами только читай DATA. DATA reference: "+string(ref), reader)
		if execution.Err != nil || execution.Result == nil {
			t.Fatal("ordinary consumer did not read the actual current consolidated episode", execution.Err)
		}
		var readInput string
		if err = p.pool.QueryRow(ctx, `SELECT input_id FROM accepted_inputs WHERE run_id=$1`, execution.Result.RunID).Scan(&readInput); err != nil {
			t.Fatal(err)
		}
		receipt, err := r.inputs.Get(ctx, readInput)
		if err != nil || receipt.Status != "completed" {
			t.Fatal("current consolidated read did not complete", err)
		}
		var nonReads int
		if err = p.pool.QueryRow(ctx, `SELECT count(*) FROM tool_calls WHERE run_id=$1 AND tool_name<>'memory.open'`, receipt.RunID).Scan(&nonReads); err != nil || nonReads != 0 || len(reader.requests) != 1 || len(reader.continuations) == 0 {
			t.Fatal("DATA consumer performed an unrelated action or omitted actual read evidence", err)
		}
		p.record("ordinary_broker_reads_earlier_consolidated_episode_with_current_applicability", &receipt, map[string]any{"source": generation, "original_owner_result": ownerResult})
		unknown.check(t, ctx, p.pool)
		if recipient.snapshot().Dispatches != unknown.Dispatches {
			t.Fatal("consolidation consumer repeated the unresolved external action")
		}
		effects, budgets := coherentStateDigest(t, ctx, p.pool)
		coherentExternalPhase(t, ctx, proof, "backup", p.pool)
		p.record("actual_custom_dump_of_same_prewithdrawal_timeline", nil, map[string]any{"effect_digest": effects, "budget_digest": budgets})
		if _, err = p.cat.Delete(ctx, p.provider.source.ID, "delete", p.provider.source.Revision); err != nil {
			t.Fatal(err)
		}
		if a, b := coherentStateDigest(t, ctx, p.pool); a != effects || b != budgets {
			t.Fatal("privacy deletion changed effect truth or complete charges")
		}
		unknown.check(t, ctx, p.pool)
		p.record("late_original_deletion_fences_episode_and_consolidation", nil, map[string]any{"source": p.provider.source.ID})
		coherentExternalPhase(t, ctx, proof, "restore", p.pool)
		cfg := p.pool.Config().Copy()
		cfg.ConnConfig.Database = os.Getenv("BUTLER_MEMORY_COHERENT_RESTORE_DATABASE")
		restored, err := pgxpool.NewWithConfig(ctx, cfg)
		if err != nil {
			t.Fatal(err)
		}
		defer restored.Close()
		current := catalog.New(restored, catalog.NewFileManifest(filepath.Join(proof, "anchor")))
		if !errors.Is(current.CheckRestore(ctx), catalog.ErrRestore) {
			t.Fatal("old custom dump was accepted before latest-anchor replay")
		}
		if _, err = current.Get(ctx, p.provider.source.ID); !errors.Is(err, catalog.ErrRestore) {
			t.Fatal("restored original released before reconciliation")
		}
		for i := 0; i < 2; i++ {
			if err = current.Reconcile(ctx); err != nil {
				t.Fatal(err)
			}
		}
		for _, id := range []string{p.provider.source.ID, lessonID, generation} {
			if _, err = current.Get(ctx, id); err == nil {
				t.Fatal("restored original/episode/consolidation revived")
			}
		}
		var copies int
		if err = restored.QueryRow(ctx, `SELECT
(SELECT count(*) FROM memory_source_revisions WHERE body LIKE '%'||$1||'%' OR (source_id=ANY($2::text[]) AND body<>''))+
(SELECT count(*) FROM tool_calls WHERE result::text LIKE '%'||$1||'%')+
(SELECT count(*) FROM memory_episodes WHERE content LIKE '%'||$1||'%' OR (source_id=$3 AND content<>''))+
(SELECT count(*) FROM run_final_results WHERE content LIKE '%'||$1||'%')+
(SELECT count(*) FROM accepted_inputs WHERE event_json::text LIKE '%'||$1||'%' OR checkpoint_json::text LIKE '%'||$1||'%')+
(SELECT count(*) FROM runs WHERE metadata_json::text LIKE '%'||$1||'%')+
(SELECT count(*) FROM messages WHERE content LIKE '%'||$1||'%' OR metadata::text LIKE '%'||$1||'%')+
(SELECT count(*) FROM session_summaries)+
(SELECT count(*) FROM sessions WHERE summary LIKE '%'||$1||'%')+
(SELECT count(*) FROM memory_claims WHERE source_id=ANY($2::text[]) AND (value_json<>'{}'::jsonb OR summary<>''))+
(SELECT count(*) FROM memory_profile WHERE source_id=ANY($2::text[]) AND (value_json<>'{}'::jsonb OR summary<>''))`, p.provider.source.Body, []string{p.provider.source.ID, lessonID, generation}, lessonID).Scan(&copies); err != nil || copies != 0 {
			t.Fatal("restored timeline retained withdrawn copies", copies, err)
		}
		if a, b := coherentStateDigest(t, ctx, restored); a != effects || b != budgets {
			t.Fatal("actual restore/replay changed independent effects or full budget ledger")
		}
		unknown.check(t, ctx, restored)
		p.record("fresh_custom_restore_latest_anchor_erases_original_episode_view_retains_all_charges", nil, map[string]any{"remaining_private_copies": copies, "effect_digest": effects, "budget_digest": budgets})
		coherentRestoredDiskFault(t, ctx, proof, current)
		p.record("same_restored_timeline_actual_enospc_keeps_failed_deletion_uncommitted", nil, map[string]any{"enospc_observed": true})
		raw, _ := json.MarshalIndent(map[string]any{"controlled_only": true, "pass": !t.Failed(), "native_calls": 0, "steps": p.steps, "restored": true, "actual_unknown_effect": unknown, "erased_private_copies": copies, "effect_journal_unchanged": true, "budget_ledger_unchanged": true, "enospc_observed": true, "limits": "controlled integrated mechanics only; supported-mode qualification, native quality/cost and staged human review remain pending"}, "", "  ")
		if err = os.WriteFile(filepath.Join(proof, "timeline-lifecycle.json"), raw, 0600); err != nil {
			t.Fatal(err)
		}
	})
}

func coherentRestoredDiskFault(t *testing.T, ctx context.Context, proof string, current *catalog.Store) {
	t.Helper()
	const root = "/bounded-anchor-fault"
	if os.Getenv("BUTLER_MEMORY_TINY_ANCHOR") != root {
		t.Fatal("explicit private eight-KiB tmpfs required")
	}
	mounts, err := os.ReadFile("/proc/self/mountinfo")
	if err != nil || !strings.Contains(string(mounts), " "+root+" ") {
		t.Fatal("dedicated bounded anchor mount missing")
	}
	var valid bool
	for _, line := range strings.Split(string(mounts), "\n") {
		if strings.Contains(line, " "+root+" ") && strings.Contains(line, " - tmpfs ") && strings.Contains(line, "size=8k") {
			valid = true
		}
	}
	if !valid {
		t.Fatal("eight-KiB tmpfs identity not proved")
	}
	anchor, err := os.ReadFile(filepath.Join(proof, "anchor"))
	if err != nil {
		t.Fatal(err)
	}
	if os.WriteFile(filepath.Join(root, "filler"), make([]byte, 4096), 0600) != nil || os.WriteFile(filepath.Join(root, "anchor"), anchor, 0600) != nil {
		t.Fatal("bounded disk setup failed")
	}
	// An explicit private copy of the latest anchor supplies this final fault
	// capsule. It starts from the restored timeline's exact deletion history.
	fault := catalog.New(current.Pool(), catalog.NewFileManifest(filepath.Join(root, "anchor")))
	if err = fault.CheckRestore(ctx); err != nil {
		t.Fatal("fault capsule did not start from latest restored history", err)
	}
	for i := 0; i < 32; i++ {
		src, err := fault.Put(ctx, catalog.PutSource{ExternalKey: fmt.Sprintf("coherent-bounded-disk-%d", i), Kind: "document", ScopeType: "user", ScopeID: "owner", Body: "Controlled final disk-failure sentinel"})
		if err != nil {
			t.Fatal(err)
		}
		var before, after int
		if err = fault.Pool().QueryRow(ctx, `SELECT count(*) FROM memory_deletions`).Scan(&before); err != nil {
			t.Fatal(err)
		}
		call, cancel := context.WithTimeout(ctx, 5*time.Second)
		_, err = fault.Delete(call, src.ID, "delete", src.Revision)
		cancel()
		if err == nil {
			continue
		}
		if !errors.Is(err, syscall.ENOSPC) {
			t.Fatal("expected actual bounded ENOSPC", err)
		}
		var status string
		if err = fault.Pool().QueryRow(ctx, `SELECT status,(SELECT count(*) FROM memory_deletions) FROM memory_sources WHERE source_id=$1`, src.ID).Scan(&status, &after); err != nil || status != "active" || before != after {
			t.Fatal("failed anchor write claimed durable deletion", status, before, after, err)
		}
		if err = fault.CheckRestore(ctx); err != nil {
			if !errors.Is(err, catalog.ErrRestore) || fault.Reconcile(ctx) == nil {
				t.Fatal("partial fault anchor silently recovered or accepted")
			}
		}
		return
	}
	t.Fatal("bounded filesystem never exhausted")
}
