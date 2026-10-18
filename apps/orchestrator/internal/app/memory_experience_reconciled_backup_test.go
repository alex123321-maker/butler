package app

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"testing"
	"time"

	"github.com/butler/butler/internal/budget"
	"github.com/butler/butler/internal/memory/catalog"
	"github.com/butler/butler/internal/memory/experience"
	"github.com/butler/butler/internal/memory/jobs"
	"github.com/jackc/pgx/v5/pgxpool"
)

type experienceReconciledBackupReceipt struct {
	Original, Witness, Snapshot, Lesson, Unrelated  catalog.SourceRef
	ClaimID                                         string
	EpisodeID, UnrelatedEpisodeID                   int64
	UnrelatedBodyHash, AccountingHash, MetadataHash string
	OriginalRun, ReadRun, TargetAction              string
	Descendant, ConsumerFinal                       catalog.SourceRef
	DescendantClaimID, ConsumerRun                  string
	DescendantEpisodeID                             int64
	OriginalBodyHash, OriginalPayloadHash           string
}

func experienceReconciledBackupPool(t *testing.T, seed bool) *pgxpool.Pool {
	t.Helper()
	config, err := pgxpool.ParseConfig(os.Getenv("BUTLER_TEST_EXPERIENCE_RECONCILED_POSTGRES_URL"))
	if err != nil || config.ConnConfig.Database != "butler_experience_reconciled_backup_test" && config.ConnConfig.Database != "butler_experience_reconciled_restore_test" {
		t.Fatal("dedicated reconciled backup/restore DB required")
	}
	config.MaxConns = 2
	config.ConnConfig.RuntimeParams["search_path"] = "public"
	pool, err := pgxpool.NewWithConfig(context.Background(), config)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	if seed {
		if config.ConnConfig.Database != "butler_experience_reconciled_backup_test" {
			t.Fatal("only fresh backup database can be seeded")
		}
		if _, err = pool.Exec(context.Background(), `CREATE EXTENSION IF NOT EXISTS vector WITH SCHEMA public`); err != nil {
			t.Fatal(err)
		}
		paths, err := filepath.Glob("../../../../migrations/*.up.sql")
		if err != nil || len(paths) == 0 {
			t.Fatal("ordered migrations unavailable", err)
		}
		sort.Strings(paths)
		for _, path := range paths {
			raw, e := os.ReadFile(path)
			if e != nil {
				t.Fatal(e)
			}
			if _, e = pool.Exec(context.Background(), string(raw)); e != nil {
				t.Fatal(filepath.Base(path), e)
			}
		}
	}
	return pool
}

func experienceReconciledBackupHashes(t *testing.T, ctx context.Context, pool *pgxpool.Pool) (string, string) {
	t.Helper()
	var accounting, metadata string
	if err := pool.QueryRow(ctx, `SELECT jsonb_build_object('foreground',(SELECT jsonb_agg(to_jsonb(a)ORDER BY operation_id)FROM execution_model_attempts a),'notes',(SELECT jsonb_agg(to_jsonb(a)ORDER BY operation_id)FROM memory_model_attempts a),'reservations',(SELECT jsonb_agg(to_jsonb(r)ORDER BY reservation_id)FROM execution_budget_reservations r))::text,
jsonb_build_object('observations',(SELECT jsonb_agg(to_jsonb(o)ORDER BY source_id)FROM memory_experience_observations o),'terminal',(SELECT jsonb_agg(to_jsonb(a)ORDER BY source_id)FROM memory_experience_terminal_attempts a),'terminal_actions',(SELECT jsonb_agg(to_jsonb(a)ORDER BY source_id,action_id)FROM memory_experience_terminal_actions a),'reconciled',(SELECT jsonb_agg(to_jsonb(a)ORDER BY source_id)FROM memory_experience_reconciled_attempts a),'pins',(SELECT jsonb_agg(to_jsonb(p)ORDER BY source_id,kind,row_id)FROM memory_experience_reconciled_pins p),'effects',(SELECT jsonb_agg(to_jsonb(e)ORDER BY action_id)FROM action_effects e),'evidence',(SELECT jsonb_agg(to_jsonb(e)ORDER BY evidence_id)FROM action_effect_evidence e))::text`).Scan(&accounting, &metadata); err != nil {
		t.Fatal(err)
	}
	return experienceDigest(accounting), experienceDigest(metadata)
}

func experienceReconciledBackupSeed(t *testing.T, ctx context.Context, r *experienceNativeRuntime, cat *catalog.Store, recipient *experienceRecipient, anchor, endpoint, token string, snapshot catalog.Source, witness catalog.SourceRef, originalRun, readRun, target string) {
	t.Helper()
	owner := experienceNativeActor(ctx, "actual-reconciled-backup-seed")
	id := experienceDigest("actual-unrelated-current-original")
	record := r.createTask(t, owner, id, "Prepare and independently verify another current unapplied artifact", "alpha", "prepare_and_verify_exact_resource")
	r.acceptCurrentProfile(t, owner, record)
	result := r.execute(t, ctx, record, id, "Inspect alpha, prepare readiness true, and check this separate current unapplied artifact", &experienceOriginalControlProvider{})
	if result.Err != nil || result.Result == nil {
		t.Fatal("unrelated actual original failed", result.Err)
	}
	if _, err := r.verifyPreparedOriginal(ctx, record, result.Result.RunID, "alpha", recipient); err != nil {
		t.Fatal("unrelated independent task verification", err)
	}
	evidence := configureExperiencePolicies(r.lessons, nil, r.tasks, r.history, r.results, cat).(*experiencePolicies).evidence
	observation, _, err := evidence.taskObservation(owner, experienceNativeOwner, result.Result.RunID, r.environment)
	if err != nil || observation.OwnerVerdict != "verified" {
		t.Fatal("unrelated owning observation", err)
	}
	jobStore := jobs.New(r.pool, cat, r.budgets, nil)
	provider := &experienceTerminalControlProvider{required: "result_id", note: experience.Notes{Summary: "Separate current original with independent artifact verification", When: []string{"This exact current scope/environment"}, NotApplicable: []string{"Changed conditions require revalidation"}, Steps: []experience.Step{{ActionID: observation.Actions[0].ID, Comment: "An observed action in the separate successful original"}}}}
	worker := &jobs.ModelWorker{Jobs: jobStore, Provider: provider, Model: "gemini-3.8-flash-high", Handlers: map[string]jobs.ModelHandler{experience.Version: experience.Handler{}}}
	request := experience.PolicyRequest{RunID: result.Result.RunID, Policy: jobs.ModelPolicyRequest{ID: "actual-unrelated-backup-note", Privacy: "cloud_allowed", IntervalSeconds: 60, MaxJobs: 1, Job: jobs.Request{Deadline: time.Now().Add(10 * time.Minute), Allowance: budget.Units{ModelTurns: 1, Tokens: 1081344, ActiveMillis: 300000, ProviderInputBytes: 8388608}}}}
	screenOwnerAPI(t, endpoint, token, http.MethodPost, "/api/v4/memory/model-experience-policies", request, nil)
	if err = jobStore.PollModelPolicies(ctx, func(string) bool { return true }); err != nil {
		t.Fatal(err)
	}
	if worked, e := worker.ProcessOne(ctx); !worked || e != nil || provider.calls != 1 {
		t.Fatal("one actual unrelated bounded note required", worked, e, provider.calls)
	}
	p := experienceReconciledBackupReceipt{Witness: witness, Snapshot: screenSourceRef(snapshot), OriginalRun: originalRun, ReadRun: readRun, TargetAction: target}
	var originalSource, lessonSource, unrelatedSource, unrelatedClaim, unrelatedBody string
	if err = r.pool.QueryRow(ctx, `SELECT 'message:'||message_id FROM messages WHERE run_id=$1 AND role='user' ORDER BY message_id LIMIT 1`, originalRun).Scan(&originalSource); err != nil {
		t.Fatal(err)
	}
	original, err := cat.Get(owner, originalSource)
	if err != nil {
		t.Fatal(err)
	}
	p.Original = screenSourceRef(original)
	if err = r.pool.QueryRow(ctx, `SELECT source_id,claim_id FROM memory_experience_lessons WHERE run_id=$1`, originalRun).Scan(&lessonSource, &p.ClaimID); err != nil {
		t.Fatal(err)
	}
	lesson, err := cat.Get(owner, lessonSource)
	if err != nil {
		t.Fatal(err)
	}
	p.Lesson = screenSourceRef(lesson)
	if err = r.pool.QueryRow(ctx, `SELECT source_id,claim_id FROM memory_experience_lessons WHERE run_id=$1`, result.Result.RunID).Scan(&unrelatedSource, &unrelatedClaim); err != nil {
		t.Fatal(err)
	}
	screenOwnerAPI(t, endpoint, token, http.MethodPost, "/api/v4/memory/decisions", catalog.Decision{ClaimID: unrelatedClaim}, nil)
	unrelated, err := cat.Get(owner, unrelatedSource)
	if err != nil {
		t.Fatal(err)
	}
	p.Unrelated = screenSourceRef(unrelated)
	if err = r.pool.QueryRow(ctx, `SELECT episode_id FROM memory_claims WHERE claim_id=$1`, p.ClaimID).Scan(&p.EpisodeID); err != nil {
		t.Fatal(err)
	}
	if err = r.pool.QueryRow(ctx, `SELECT c.episode_id,e.content FROM memory_claims c JOIN memory_episodes e ON e.id=c.episode_id WHERE c.claim_id=$1`, unrelatedClaim).Scan(&p.UnrelatedEpisodeID, &unrelatedBody); err != nil {
		t.Fatal(err)
	}
	p.UnrelatedBodyHash = experienceDigest(unrelatedBody)
	p.AccountingHash, p.MetadataHash = experienceReconciledBackupHashes(t, ctx, r.pool)
	path := os.Getenv("BUTLER_TEST_EXPERIENCE_RECONCILED_RECEIPT")
	if !filepath.IsAbs(path) || !filepath.IsAbs(anchor) {
		t.Fatal("private receipt/anchor paths required")
	}
	raw, _ := json.Marshal(p)
	if err = os.WriteFile(path, raw, 0600); err != nil {
		t.Fatal(err)
	}
}

// The external driver surrounds these phases with actual pg_dump/pg_restore.
// Seed is an ordinary actual input/Broker/owner Resume/read/note chain; no model
// network, authored observation/lesson or hidden recipient audit supplies proof.
func TestActualReconciledExperienceDumpRestoreDeletionIntegration(t *testing.T) {
	phase := os.Getenv("BUTLER_TEST_EXPERIENCE_RECONCILED_PHASE")
	if phase == "" {
		t.Skip("external actual reconciled dump/restore procedure required")
	}
	if phase == "seed" {
		experienceOriginalOwnerResumeRead(t, "backup_seed")
		return
	}
	if phase != "delete" && phase != "restore" {
		t.Fatal("unsupported reconciled backup phase")
	}
	pool := experienceReconciledBackupPool(t, false)
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	owner := experienceNativeActor(ctx, "actual-reconciled-backup-"+phase)
	anchor, receipt := os.Getenv("BUTLER_TEST_EXPERIENCE_RECONCILED_ANCHOR"), os.Getenv("BUTLER_TEST_EXPERIENCE_RECONCILED_RECEIPT")
	if !filepath.IsAbs(anchor) || !filepath.IsAbs(receipt) {
		t.Fatal("private persistent anchor/receipt required")
	}
	raw, err := os.ReadFile(receipt)
	var p experienceReconciledBackupReceipt
	if err != nil || json.Unmarshal(raw, &p) != nil || p.ClaimID == "" || p.EpisodeID < 1 {
		t.Fatal("content-free actual backup receipt invalid")
	}
	cat := catalog.New(pool, catalog.NewFileManifest(anchor))
	if phase == "delete" {
		if _, err = cat.Delete(owner, p.Original.ID, "delete", p.Original.Revision); err != nil {
			t.Fatal(err)
		}
	} else {
		if err = cat.CheckRestore(owner); !errors.Is(err, catalog.ErrRestore) {
			t.Fatal("old restored database did not fail closed", err)
		}
		if _, err = cat.Get(owner, p.Lesson.ID); err == nil {
			t.Fatal("old accepted reconciled lesson resurrected before anchor replay")
		}
		if err = cat.Reconcile(owner); err != nil {
			t.Fatal(err)
		}
		if err = cat.Reconcile(owner); err != nil {
			t.Fatal("second actual anchor replay", err)
		}
	}
	for _, ref := range []catalog.SourceRef{p.Original, p.Witness, p.Snapshot, p.Lesson} {
		if _, err = cat.Get(owner, ref.ID); err == nil {
			t.Fatal("deleted original/reconciled source remained readable")
		}
		var retained int
		if err = pool.QueryRow(ctx, `SELECT COALESCE(sum(octet_length(body)+octet_length(title)+octet_length(author)),0) FROM memory_source_revisions WHERE source_id=$1`, ref.ID).Scan(&retained); err != nil || retained != 0 {
			t.Fatal("copied source bytes retained", ref.ID, retained, err)
		}
	}
	var originalPayloadErased bool
	if err = pool.QueryRow(ctx, `SELECT count(*)>0 AND bool_and(args='{}'::jsonb AND error='{}'::jsonb AND result->>'privacy_redacted'='true') FROM tool_calls WHERE run_id=$1`, p.OriginalRun).Scan(&originalPayloadErased); err != nil || !originalPayloadErased {
		t.Fatal("actual original cached tool payload survived deletion/anchor replay", err)
	}
	var retained int
	if err = pool.QueryRow(ctx, `SELECT count(*) FROM memory_episodes WHERE id=$1`, p.EpisodeID).Scan(&retained); err != nil || retained != 0 {
		t.Fatal("accepted reconciled episode survived", err)
	}
	var status, value, summary string
	if err = pool.QueryRow(ctx, `SELECT status,value_json::text,summary FROM memory_claims WHERE claim_id=$1`, p.ClaimID).Scan(&status, &value, &summary); err != nil || status != "deleted" || value != "{}" || summary != "" {
		t.Fatal("reconciled claim retained copied text", err)
	}
	if _, err = cat.Decide(owner, catalog.Decision{ClaimID: p.ClaimID}); err == nil {
		t.Fatal("deleted reconciled note could be reaccepted")
	}
	unrelated, err := cat.Get(owner, p.Unrelated.ID)
	if err != nil || unrelated.Hash != p.Unrelated.Hash {
		t.Fatal("unrelated actual accepted lesson changed", err)
	}
	var body string
	if err = pool.QueryRow(ctx, `SELECT content FROM memory_episodes WHERE id=$1`, p.UnrelatedEpisodeID).Scan(&body); err != nil || experienceDigest(body) != p.UnrelatedBodyHash {
		t.Fatal("unrelated actual episode changed", err)
	}
	accounting, metadata := experienceReconciledBackupHashes(t, ctx, pool)
	if accounting != p.AccountingHash || metadata != p.MetadataHash {
		t.Fatal("actual usage/reservations or immutable original/reconciliation evidence changed")
	}
	var preserved bool
	if err = pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM action_effects e JOIN runs r ON r.run_id=$2 WHERE e.action_id=$1 AND e.state='effect_absent' AND r.current_state='cancelled' AND NOT EXISTS(SELECT 1 FROM run_final_results f WHERE f.run_id=r.run_id))`, p.TargetAction, p.OriginalRun).Scan(&preserved); err != nil || !preserved {
		t.Fatal("privacy/restore invented an original final or changed reconciled ABSENT", err)
	}
}
