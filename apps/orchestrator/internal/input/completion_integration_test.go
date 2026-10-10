package input

import (
	"context"
	"errors"
	"github.com/butler/butler/apps/orchestrator/internal/artifacts"
	"github.com/butler/butler/apps/orchestrator/internal/completion"
	flow "github.com/butler/butler/apps/orchestrator/internal/orchestrator"
	"github.com/butler/butler/apps/orchestrator/internal/session"
	"github.com/jackc/pgx/v5"
	"testing"
)

type crashAfterFinalCommit struct {
	completion.Writer
	crash context.CancelFunc
}

func (w crashAfterFinalCommit) Save(ctx context.Context, q completion.Request) (completion.FinalRecord, error) {
	saved, err := w.Writer.Save(ctx, q)
	if err == nil {
		w.crash()
	}
	return saved, err
}

type unavailableFinalArtifact struct{}

func (unavailableFinalArtifact) SaveFinalTx(context.Context, pgx.Tx, artifacts.CreateParams) error {
	return errors.New("artifact unavailable")
}

func TestFinalResultRecoveryNeverRepeatsExternalCallIntegration(t *testing.T) {
	for _, fault := range []string{"artifact_unavailable", "after_commit_before_delivery"} {
		t.Run(fault, func(t *testing.T) {
			f := newLifecycleFixture(t)
			ordinary, _ := f.worker(f.store, nil)
			ordinary.process(f.ctx, f.claim(t))
			if _, _, err := f.resolve(t, true, ownerActor()); err != nil {
				t.Fatal(err)
			}
			coordinator := completion.New(f.pool)
			var writer completion.Writer = coordinator
			executeCtx := f.ctx
			var crash context.CancelFunc
			if fault == "artifact_unavailable" {
				coordinator.Artifacts = unavailableFinalArtifact{}
			} else {
				executeCtx, crash = context.WithCancel(f.ctx)
				writer = crashAfterFinalCommit{Writer: coordinator, crash: crash}
			}
			provider := &lifecycleProvider{}
			executor := flow.NewService(f.sessions, session.NewPostgresLeaseManager(f.pool), f.runs, f.transcript, provider, flow.Config{Completion: writer, ProviderName: "fixture", ModelName: "fixture", OwnerID: "worker", LeaseTTL: 30, Tasks: f.tasks, Checkpoints: f.store, ApprovalService: f.approvals, ActionAuthorizer: f.tools, Tools: f.tools}, nil)
			worker := NewService(f.store, f.sessions, executor, f.runs, nil)
			worker.process(executeCtx, f.claim(t))
			if f.tools.calls != 1 {
				t.Fatal("fixture did not execute exactly one authorized effect")
			}
			var claim Record
			if fault == "artifact_unavailable" {
				f.assertReceipt(t, "queued")
				var n int
				_ = f.pool.QueryRow(f.ctx, `SELECT COUNT(*) FROM run_final_results WHERE run_id=(SELECT run_id FROM accepted_inputs WHERE input_id=$1)`, f.receipt.InputID).Scan(&n)
				if n != 0 {
					t.Fatal("failed mandatory artifact allowed result")
				}
				_, _ = f.pool.Exec(f.ctx, `UPDATE accepted_inputs SET not_before=NOW() WHERE input_id=$1`, f.receipt.InputID)
				claim = f.claim(t)
			} else {
				claim = f.reclaimAfterCrash(t)
			}
			restarted, p := f.worker(f.store, nil)
			restarted.process(f.ctx, claim)
			receipt := f.assertReceipt(t, "completed")
			if f.tools.calls != 1 || len(p.calls) != 0 {
				t.Fatalf("persistence/delivery fault repeated work: effects=%d model_calls=%d", f.tools.calls, len(p.calls))
			}
			result, err := completion.New(f.pool).GetByRun(f.ctx, receipt.RunID)
			if err != nil || result.ArtifactID == "" || result.Verdict != "unverified" {
				t.Fatal("final result not authoritative", err)
			}
			notifications, err := completion.New(f.pool).Outbox.ListByRun(f.ctx, receipt.RunID)
			if err != nil || len(notifications) != 1 || notifications[0].Status != "result_ready" {
				t.Fatal("restart lost Web readiness", err)
			}
		})
	}
}
