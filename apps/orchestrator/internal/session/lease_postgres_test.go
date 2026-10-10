package session

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/butler/butler/internal/ownership"
	postgresstore "github.com/butler/butler/internal/storage/postgres"
)

func TestDurableLeaseFenceTakeoverIntegration(t *testing.T) {
	dsn := os.Getenv("BUTLER_TEST_POSTGRES_URL")
	if dsn == "" {
		t.Skip("BUTLER_TEST_POSTGRES_URL is not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	db, err := postgresstore.Open(ctx, postgresstore.Config{URL: dsn, MaxConns: 2, MinConns: 1}, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err = db.RunMigrations(ctx, filepath.Join("..", "..", "..", "..", "migrations")); err != nil {
		t.Fatal(err)
	}
	key := "integration:fence:" + time.Now().UTC().Format("150405.000000000")
	keys := []string{key, key + ":other"}
	for _, k := range keys {
		if _, err = db.Pool().Exec(ctx, `INSERT INTO sessions(session_key,user_id,channel) VALUES($1,'owner','web')`, k); err != nil {
			t.Fatal(err)
		}
	}
	defer func() {
		for _, k := range keys {
			_, _ = db.Pool().Exec(context.Background(), `DELETE FROM sessions WHERE session_key=$1`, k)
		}
	}()
	leases := NewPostgresLeaseManager(db.Pool())
	first, err := leases.AcquireLease(ctx, AcquireLeaseParams{SessionKey: key, RunID: "run-a", OwnerID: "worker-a", TTL: time.Minute})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = leases.AcquireLease(ctx, AcquireLeaseParams{SessionKey: key, RunID: "run-b", OwnerID: "worker-b", TTL: time.Minute}); !errors.Is(err, ErrLeaseConflict) {
		t.Fatalf("concurrent ownership: %v", err)
	}
	fenced := ownership.WithToken(ctx, leases.Authority(), ownership.Token{SessionKey: key, RunID: first.RunID, LeaseID: first.LeaseID, FenceEpoch: first.FenceEpoch})
	guarded, release, err := ownership.Begin(fenced)
	if err != nil {
		t.Fatal(err)
	}
	// Expire in the acquisition expression by releasing from another goroutine:
	// it must wait for the write guard rather than supersede it mid-write.
	takeover := make(chan error, 1)
	go func() { _, err := leases.ReleaseLease(ctx, first.LeaseID); takeover <- err }()
	select {
	case <-takeover:
		t.Fatal("lease release crossed a guarded authority write")
	case <-time.After(40 * time.Millisecond):
	}
	if _, err = db.Pool().Exec(guarded, `UPDATE sessions SET metadata=jsonb_build_object('fenced_write',true) WHERE session_key=$1`, key); err != nil {
		t.Fatal(err)
	}
	release()
	if err = <-takeover; err != nil {
		t.Fatal(err)
	}
	second, err := leases.AcquireLease(ctx, AcquireLeaseParams{SessionKey: key, RunID: "run-b", OwnerID: "worker-b", TTL: time.Minute})
	if err != nil {
		t.Fatal(err)
	}
	if second.FenceEpoch <= first.FenceEpoch || second.LeaseID == first.LeaseID {
		t.Fatal("takeover did not advance unique fencing token")
	}
	if _, err = leases.RenewLease(ctx, first.LeaseID, time.Minute); !errors.Is(err, ErrLeaseNotFound) {
		t.Fatalf("stale renew: %v", err)
	}
	if released, err := leases.ReleaseLease(ctx, first.LeaseID); err != nil || released {
		t.Fatalf("stale release removed replacement: %v %v", released, err)
	}
	if _, _, err = ownership.Begin(fenced); !errors.Is(err, ownership.ErrStale) {
		t.Fatalf("stale write accepted: %v", err)
	}
	other, err := leases.AcquireLease(ctx, AcquireLeaseParams{SessionKey: keys[1], RunID: "run-c", OwnerID: "worker-c", TTL: time.Minute})
	if err != nil {
		t.Fatal(err)
	}
	// A two-connection pool and two concurrent sessions must remain deadlock-free.
	done := make(chan error, 2)
	for _, lease := range []LeaseRecord{second, other} {
		go func(lease LeaseRecord) {
			tokenCtx := ownership.WithToken(ctx, leases.Authority(), ownership.Token{SessionKey: lease.SessionKey, RunID: lease.RunID, LeaseID: lease.LeaseID, FenceEpoch: lease.FenceEpoch})
			writeCtx, release, err := ownership.Begin(tokenCtx)
			if err == nil {
				_, err = db.Pool().Exec(writeCtx, `UPDATE sessions SET summary='guarded' WHERE session_key=$1`, lease.SessionKey)
				release()
			}
			done <- err
		}(lease)
	}
	for range 2 {
		select {
		case err := <-done:
			if err != nil {
				t.Fatal(err)
			}
		case <-ctx.Done():
			t.Fatal("guard/write connection starvation")
		}
	}
	if _, err = db.Pool().Exec(ctx, `UPDATE session_ownership SET expires_at=clock_timestamp()-INTERVAL '1 second' WHERE lease_id=$1`, second.LeaseID); err != nil {
		t.Fatal(err)
	}
	if _, err = leases.RenewLease(ctx, second.LeaseID, time.Minute); !errors.Is(err, ErrLeaseNotFound) {
		t.Fatalf("expired lease revived: %v", err)
	}
	third, err := leases.AcquireLease(ctx, AcquireLeaseParams{SessionKey: key, RunID: "run-d", OwnerID: "worker-d", TTL: time.Minute})
	if err != nil || third.FenceEpoch <= second.FenceEpoch {
		t.Fatalf("expiry takeover: %#v %v", third, err)
	}
}
