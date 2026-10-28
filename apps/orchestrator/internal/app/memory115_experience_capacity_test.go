package app

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/butler/butler/apps/orchestrator/internal/session"
	"github.com/butler/butler/apps/orchestrator/internal/task"
	"github.com/butler/butler/internal/memory/catalog"
	"github.com/butler/butler/internal/memory/memorytest"
	"github.com/butler/butler/internal/ownership"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestExperienceNativeRuntimeKeepsAuthorityCapacityIntegration(t *testing.T) {
	pool := memorytest.Database(t)
	ctx, cancel := context.WithTimeout(t.Context(), 45*time.Second)
	defer cancel()
	dir := t.TempDir()
	anchor := filepath.Join(dir, "anchor")
	manifest := catalog.NewFileManifest(anchor)
	if manifest.Initialize() != nil {
		t.Fatal("new privacy anchor required")
	}
	recipient, err := newExperienceRecipient(dir, experienceRecipientConfig{Version: "v1", Ready: "true", Files: map[string]string{"alpha": "Capacity fixture\nready=false\n", "beta": "Other fixture\nready=false\n"}})
	if err != nil {
		t.Fatal(err)
	}
	defer recipient.close()
	r := newExperienceNativeRuntime(t, pool, catalog.New(pool, manifest), anchor, dir, recipient, newExperienceNativeAddresses(t), experienceNativeSourcePolicy(), "Capacity observation only", experienceNativeArms[0], nil, time.Now().Add(time.Hour))
	defer r.close()
	if r.reads == pool || r.reads.Config().MaxConns != 1 {
		t.Fatal("runtime did not reserve exactly one observation connection")
	}
	record := r.createTask(t, ctx, "native-capacity-proof", "Capacity observation only", "alpha", "prepare_and_verify_exact_resource")
	const run = "native-capacity-proof-run"
	if _, err = pool.Exec(ctx, `INSERT INTO runs(run_id,session_key,input_event_id,status,autonomy_mode,current_state,model_provider,task_id,task_revision,started_at,updated_at)VALUES($1,$2,'capacity-input','running','mode_2','model_running','fixture',$3,$4,clock_timestamp(),clock_timestamp())`, run, record.SessionKey, record.TaskID, record.Revision); err != nil {
		t.Fatal(err)
	}
	leases := session.NewPostgresLeaseManagerWithAuthorityReads(pool, r.reads)
	lease, err := leases.AcquireLease(ctx, session.AcquireLeaseParams{SessionKey: record.SessionKey, RunID: run, OwnerID: "capacity-worker", TTL: time.Minute})
	if err != nil {
		t.Fatal(err)
	}
	held := []*pgxpool.Conn{}
	defer func() {
		for _, c := range held {
			c.Release()
		}
	}()
	for range pool.Config().MaxConns {
		c, e := pool.Acquire(ctx)
		if e != nil {
			t.Fatal(e)
		}
		held = append(held, c)
	}
	blocked, stop := context.WithTimeout(ctx, 50*time.Millisecond)
	err = task.NewStore(pool).ValidateRunAuthority(blocked, run)
	stop()
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatal("old shared-capacity failure not reproduced", err)
	}
	read, stopRead := context.WithTimeout(ctx, 750*time.Millisecond)
	if _, err = leases.GetLease(read, lease.LeaseID); err != nil {
		stopRead()
		t.Fatal("reserved lease observation failed", err)
	}
	if err = r.tasks.ValidateRunAuthority(read, run); err != nil {
		stopRead()
		t.Fatal("reserved task observation failed", err)
	}
	owned := ownership.WithToken(read, leases.Authority(), ownership.Token{SessionKey: lease.SessionKey, RunID: run, LeaseID: lease.LeaseID, FenceEpoch: lease.FenceEpoch})
	if err = r.cat.CheckRunSources(owned, run); err != nil {
		stopRead()
		t.Fatal("reserved source observation failed", err)
	}
	stopRead()
	for _, c := range held {
		c.Release()
	}
	held = nil
	if _, err = pool.Exec(ctx, `UPDATE tasks SET status='blocked' WHERE task_id=$1`, record.TaskID); err != nil {
		t.Fatal(err)
	}
	if err = r.tasks.ValidateRunAuthority(ctx, run); !errors.Is(err, task.ErrConflict) {
		t.Fatal("reserve bypassed real authority revocation", err)
	}
	t.Log("CAPACITY_PASS actual runtime: old shared pool times out; reserved lease/task/source checks pass within750ms; revocation still denies; native=0")
}
