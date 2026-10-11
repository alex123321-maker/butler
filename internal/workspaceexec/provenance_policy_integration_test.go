package workspaceexec

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/butler/butler/internal/memory/catalog"
	"github.com/butler/butler/internal/owneridentity"
	"github.com/butler/butler/internal/workspace"
	"github.com/jackc/pgx/v5"
)

type falseAuxiliaryProof struct{ calls *int }

func (f falseAuxiliaryProof) CheckMemoryAccessTx(context.Context, pgx.Tx, []catalog.SourceRef, string) (string, error) {
	*f.calls++
	return "foreign-owner", nil
}
func (falseAuxiliaryProof) FinalizeMemoryAccessTx(context.Context, pgx.Tx, []catalog.SourceRef, string) error {
	return nil
}
func (falseAuxiliaryProof) PinnedMemorySources() []catalog.SourceRef { return nil }

func TestWorkspaceProvenanceRejectsOwnerJobAndForeignScopeIntegration(t *testing.T) {
	f := newSourceFixture(t)
	ref := f.create(t)
	foreignActor := owneridentity.WithActor(f.ctx, owneridentity.Actor{OwnerID: "foreign-owner", Channel: "web", DeliveryID: "fixture"})
	src, err := f.catalog.Put(foreignActor, catalog.PutSource{ExternalKey: "foreign", Kind: "document", ScopeType: "user", ScopeID: "foreign-owner", Title: "Foreign", Body: "foreign synthetic fixture"})
	if err != nil {
		t.Fatal(err)
	}
	for _, mode := range []string{"owner_actor", "job_checker", "foreign_binding", "task_revision", "constraints_revision", "zero_context"} {
		t.Run(mode, func(t *testing.T) {
			tx, err := f.pool.Begin(f.ctx)
			if err != nil {
				t.Fatal(err)
			}
			defer tx.Rollback(context.Background())
			c, err := f.provenance.PrepareTx(f.ctx, tx, ref, false)
			if err != nil {
				t.Fatal(err)
			}
			ctx := f.ctx
			calls := 0
			switch mode {
			case "owner_actor":
				ctx = owneridentity.WithActor(ctx, owneridentity.Actor{OwnerID: "wp-owner", Channel: "web", DeliveryID: "fixture"})
			case "job_checker":
				ctx = catalog.WithAccessChecker(ctx, falseAuxiliaryProof{&calls})
				c.parents = []catalog.SourceRef{{ID: src.ID, Revision: src.Revision, AccessRevision: src.AccessRevision, Hash: src.Hash}}
			case "foreign_binding":
				c.ref.Binding.OwnerID = "foreign-owner"
			case "task_revision":
				c.ref.Binding.TaskRevision++
			case "constraints_revision":
				c.ref.Binding.ConstraintsRevision++
			case "zero_context":
				c = SourceContext{}
			}
			if err = f.provenance.CheckTx(ctx, tx, c, ""); err == nil || calls != 0 {
				t.Fatal("source authority shortcut", mode, calls, err)
			}
		})
	}
}

func TestWorkspaceProvenanceExpiresDuringActualSourceWaitIntegration(t *testing.T) {
	f := newSourceFixture(t)
	ref := f.create(t)
	tx, err := f.pool.Begin(f.ctx)
	if err != nil {
		t.Fatal(err)
	}
	c, err := f.provenance.PrepareTx(f.ctx, tx, ref, false)
	if err != nil {
		t.Fatal(err)
	}
	if err = tx.Commit(f.ctx); err != nil {
		t.Fatal(err)
	}
	if _, err = f.pool.Exec(f.ctx, `UPDATE session_ownership SET expires_at=clock_timestamp()+interval '1100 milliseconds' WHERE session_key='wp-session'`); err != nil {
		t.Fatal(err)
	}
	blocker, err := f.pool.Begin(f.ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer blocker.Rollback(context.Background())
	if _, err = blocker.Exec(f.ctx, `SELECT source_id FROM memory_sources WHERE source_id=$1 FOR UPDATE`, f.source.ID); err != nil {
		t.Fatal(err)
	}
	var blockerPID int
	if err = blocker.QueryRow(f.ctx, `SELECT pg_backend_pid()`).Scan(&blockerPID); err != nil {
		t.Fatal(err)
	}
	bounded, stop := context.WithTimeout(f.ctx, 4*time.Second)
	defer stop()
	waiter, err := f.pool.Begin(bounded)
	if err != nil {
		t.Fatal(err)
	}
	defer waiter.Rollback(context.Background())
	var waiterPID int
	if err = waiter.QueryRow(bounded, `SELECT pg_backend_pid()`).Scan(&waiterPID); err != nil {
		t.Fatal(err)
	}
	result := make(chan error, 1)
	go func() { result <- f.provenance.CheckTx(bounded, waiter, c, "") }()
	observed := false
	for end := time.Now().Add(time.Second); time.Now().Before(end); time.Sleep(10 * time.Millisecond) {
		if err = blocker.QueryRow(f.ctx, `SELECT $1=ANY(pg_blocking_pids($2))`, blockerPID, waiterPID).Scan(&observed); err != nil {
			t.Fatal(err)
		}
		if observed {
			break
		}
	}
	if !observed {
		t.Fatal("source row wait was not observed")
	}
	expired := false
	for !expired {
		if err = blocker.QueryRow(f.ctx, `SELECT expires_at<=clock_timestamp() FROM session_ownership WHERE session_key='wp-session'`).Scan(&expired); err != nil {
			t.Fatal(err)
		}
		if !expired {
			time.Sleep(10 * time.Millisecond)
		}
	}
	if err = blocker.Rollback(f.ctx); err != nil {
		t.Fatal(err)
	}
	select {
	case err = <-result:
		if err == nil {
			t.Fatal("source wait extended worker authority")
		}
	case <-bounded.Done():
		t.Fatal("source wait did not finish")
	}
}

type failedPurge struct{}

func (failedPurge) PurgeWorkspace(context.Context, string) error {
	return errors.New("synthetic erasure failure")
}

type eraseThenLoseReceipt struct{ runtime *workspace.DeletionRuntime }

func (r eraseThenLoseReceipt) PurgeWorkspace(ctx context.Context, id string) error {
	if err := r.runtime.PurgeWorkspace(ctx, id); err != nil {
		return err
	}
	return errors.New("synthetic lost erasure acknowledgement")
}

func TestWorkspacePrivacyErasureSurvivesLostAcknowledgementIntegration(t *testing.T) {
	f := newSourceFixture(t)
	ref := f.create(t)
	owner := owneridentity.WithActor(f.ctx, owneridentity.Actor{OwnerID: "wp-owner", Channel: "web", DeliveryID: "delete"})
	if _, err := f.catalog.Delete(owner, f.source.ID, "delete", f.source.Revision); err != nil {
		t.Fatal(err)
	}
	if worked, err := f.provenance.PurgeDeleted(f.ctx, eraseThenLoseReceipt{f.purge}); err == nil || !worked {
		t.Fatal("lost acknowledgement falsely completed", worked, err)
	}
	if _, err := os.Lstat(filepath.Join(f.root, ref.ID)); !os.IsNotExist(err) {
		t.Fatal("fixture did not actually erase namespace", err)
	}
	var status, state string
	if err := f.pool.QueryRow(f.ctx, `SELECT d.status,w.state FROM code_workspace_deletions d JOIN code_workspaces w USING(workspace_id) WHERE workspace_id=$1`, ref.ID).Scan(&status, &state); err != nil || status != "pending" || state != "blocked" {
		t.Fatal("lost acknowledgement lost pending job", status, state, err)
	}
	// A new runtime opens the fixed root after the first physical effect. It
	// syncs even an already absent namespace before publishing the durable mark.
	restarted, err := workspace.NewDeletionRuntime(f.root)
	if err != nil {
		t.Fatal(err)
	}
	defer restarted.Close()
	if worked, err := f.provenance.PurgeDeleted(f.ctx, restarted); err != nil || !worked {
		t.Fatal("restarted privacy cleanup failed", worked, err)
	}
}
func TestWorkspacePrivacyErasureRetriesAndCannotUnblockIntegration(t *testing.T) {
	f := newSourceFixture(t)
	ref := f.create(t)
	owner := owneridentity.WithActor(f.ctx, owneridentity.Actor{OwnerID: "wp-owner", Channel: "web", DeliveryID: "delete"})
	if _, err := f.catalog.Delete(owner, f.source.ID, "delete", f.source.Revision); err != nil {
		t.Fatal(err)
	}
	if worked, err := f.provenance.PurgeDeleted(f.ctx, failedPurge{}); err == nil || !worked {
		t.Fatal("failed erasure falsely completed", worked, err)
	}
	var status, state string
	if err := f.pool.QueryRow(f.ctx, `SELECT d.status,w.state FROM code_workspace_deletions d JOIN code_workspaces w USING(workspace_id) WHERE workspace_id=$1`, ref.ID).Scan(&status, &state); err != nil || status != "pending" || state != "blocked" {
		t.Fatal("erasure failure lost durable blockage", status, state, err)
	}
	if worked, err := f.provenance.PurgeDeleted(f.ctx, f.purge); err != nil || !worked {
		t.Fatal("erasure retry failed", worked, err)
	}
	tx, err := f.pool.Begin(f.ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(context.Background())
	if _, err = f.provenance.PrepareTx(f.ctx, tx, ref, false); err == nil {
		t.Fatal("physical erasure reopened namespace")
	}
}

func TestWorkspaceProvenanceHistoryCannotBeOverwrittenIntegration(t *testing.T) {
	f := newSourceFixture(t)
	ref := f.create(t)
	for _, query := range []string{`UPDATE code_workspace_sources SET observation_hash=repeat('0',64) WHERE workspace_id=$1`, `DELETE FROM code_workspace_sources WHERE workspace_id=$1`} {
		if _, err := f.pool.Exec(f.ctx, query, ref.ID); err == nil {
			t.Fatal("immutable workspace lineage changed")
		}
	}
	ctx, proof := f.prepare(t, ref, "write", "main.go", []byte("package main"))
	if _, err := f.store.Write(ctx, ref, proof, workspace.Snapshot{Path: "main.go"}, []byte("package main")); err != nil {
		t.Fatal("denied history mutation damaged actual copy", err)
	}
}
