package session

import (
	"context"
	"os"
	"testing"
	"time"

	redisstore "github.com/butler/butler/internal/storage/redis"
)

func TestRedisLeaseManagerIntegration(t *testing.T) {
	redisURL := os.Getenv("BUTLER_TEST_REDIS_URL")
	if redisURL == "" {
		t.Skip("BUTLER_TEST_REDIS_URL is not set")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	store, err := redisstore.Open(ctx, redisstore.Config{URL: redisURL}, nil)
	if err != nil {
		t.Fatalf("Open returned error: %v", err)
	}
	defer func() {
		if err := store.Close(); err != nil {
			t.Fatalf("Close returned error: %v", err)
		}
	}()

	manager := NewRedisLeaseManager(store.Client(), nil)
	params := AcquireLeaseParams{
		SessionKey: "integration:lease:session",
		RunID:      "run-1",
		OwnerID:    "owner-1",
		TTL:        2 * time.Second,
	}

	_ = store.Client().Del(ctx, leaseKey(params.SessionKey), leaseIndexKey(buildLeaseID(params.SessionKey, params.RunID, params.OwnerID))).Err()

	lease, err := manager.AcquireLease(ctx, params)
	if err != nil {
		t.Fatalf("AcquireLease returned error: %v", err)
	}

	if _, err := manager.AcquireLease(ctx, AcquireLeaseParams{
		SessionKey: params.SessionKey,
		RunID:      "run-2",
		OwnerID:    "owner-2",
		TTL:        2 * time.Second,
	}); err != ErrLeaseConflict {
		t.Fatalf("expected lease conflict, got %v", err)
	}

	renewed, err := manager.RenewLease(ctx, lease.LeaseID, 3*time.Second)
	if err != nil {
		t.Fatalf("RenewLease returned error: %v", err)
	}
	if renewed.TTLSeconds != 3 {
		t.Fatalf("expected renewed ttl 3, got %d", renewed.TTLSeconds)
	}

	reacquiredSameOwner, err := manager.AcquireLease(ctx, AcquireLeaseParams{
		SessionKey: params.SessionKey,
		RunID:      params.RunID,
		OwnerID:    params.OwnerID,
		TTL:        4 * time.Second,
	})
	if err != nil {
		t.Fatalf("AcquireLease same owner returned error: %v", err)
	}
	if reacquiredSameOwner.TTLSeconds != 4 {
		t.Fatalf("expected reacquired ttl 4, got %d", reacquiredSameOwner.TTLSeconds)
	}
	if !reacquiredSameOwner.ExpiresAt.After(renewed.ExpiresAt) {
		t.Fatalf("expected reacquired expiry %s to be after renewed expiry %s", reacquiredSameOwner.ExpiresAt, renewed.ExpiresAt)
	}

	released, err := manager.ReleaseLease(ctx, lease.LeaseID)
	if err != nil {
		t.Fatalf("ReleaseLease returned error: %v", err)
	}
	if !released {
		t.Fatal("expected released=true")
	}

	expiring, err := manager.AcquireLease(ctx, AcquireLeaseParams{
		SessionKey: params.SessionKey,
		RunID:      "run-expire",
		OwnerID:    "owner-expire",
		TTL:        time.Second,
	})
	if err != nil {
		t.Fatalf("AcquireLease for expiry returned error: %v", err)
	}

	time.Sleep(1200 * time.Millisecond)

	if _, err := manager.RenewLease(ctx, expiring.LeaseID, time.Second); err != ErrLeaseNotFound {
		t.Fatalf("expected lease not found after expiry, got %v", err)
	}

	reacquired, err := manager.AcquireLease(ctx, AcquireLeaseParams{
		SessionKey: params.SessionKey,
		RunID:      "run-3",
		OwnerID:    "owner-3",
		TTL:        time.Second,
	})
	if err != nil {
		t.Fatalf("AcquireLease after expiry returned error: %v", err)
	}
	if reacquired.RunID != "run-3" {
		t.Fatalf("expected reacquired run_id run-3, got %q", reacquired.RunID)
	}

	_, _ = manager.ReleaseLease(ctx, reacquired.LeaseID)
}
