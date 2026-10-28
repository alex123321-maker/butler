package app

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/butler/butler/apps/orchestrator/internal/session"
	"github.com/butler/butler/internal/memory/memorytest"
)

// Reproduces connection starvation without a model or provider credentials.
// It proves capacity isolation, not the unique cause of the closed native delay.
func TestGuided115SeparateSourceAndAuthorityCapacityIntegration(t *testing.T) {
	pool := memorytest.Database(t)
	reads, sources := newExperienceNativeObservationPools(t, pool)
	if reads == sources || reads == pool || sources == pool || reads.Config().MaxConns != 1 || sources.Config().MaxConns != 1 {
		t.Fatal("distinct bounded reserves required")
	}
	ctx := t.Context()
	key := "guided-capacity-" + time.Now().UTC().Format("150405.000000000")
	if _, err := pool.Exec(ctx, `INSERT INTO sessions(session_key,user_id,channel) VALUES($1,'owner','web')`, key); err != nil {
		t.Fatal(err)
	}
	manager := session.NewPostgresLeaseManagerWithAuthorityReads(pool, reads)
	lease, err := manager.AcquireLease(ctx, session.AcquireLeaseParams{SessionKey: key, RunID: "capacity-run", OwnerID: "capacity-worker", TTL: time.Minute})
	if err != nil {
		t.Fatal(err)
	}
	occupied, err := sources.Acquire(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer occupied.Release()
	shared := session.NewPostgresLeaseManagerWithAuthorityReads(pool, sources)
	denied, cancel := context.WithTimeout(ctx, 75*time.Millisecond)
	_, err = shared.GetLease(denied, lease.LeaseID)
	cancel()
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatal("shared reserve did not reproduce capacity timeout", err)
	}
	live, cancel := context.WithTimeout(ctx, 750*time.Millisecond)
	_, err = manager.ValidateLease(live, lease.LeaseID, lease.RunID, lease.FenceEpoch)
	cancel()
	if err != nil {
		t.Fatal("independent observer blocked on source capacity", err)
	}
	if _, err = manager.ReleaseLease(ctx, lease.LeaseID); err != nil {
		t.Fatal(err)
	}
	live, cancel = context.WithTimeout(ctx, 750*time.Millisecond)
	defer cancel()
	if _, err = manager.ValidateLease(live, lease.LeaseID, lease.RunID, lease.FenceEpoch); err == nil {
		t.Fatal("capacity isolation bypassed revocation")
	}
}
