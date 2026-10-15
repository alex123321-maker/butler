package app

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/butler/butler/internal/memory/catalog"
	"github.com/butler/butler/internal/memory/memorytest"
	"github.com/butler/butler/internal/memory/working"
	"github.com/butler/butler/internal/owneridentity"
	"github.com/butler/butler/internal/ownership"
	redisstore "github.com/butler/butler/internal/storage/redis"
)

func TestLifecycleRedisCacheWithdrawalIntegration(t *testing.T) {
	dsn := os.Getenv("BUTLER_TEST_REDIS_URL")
	if dsn == "" {
		t.Skip("isolated Redis required")
	}
	pool := memorytest.Database(t)
	manifest := catalog.NewFileManifest(filepath.Join(t.TempDir(), "anchor"))
	if e := manifest.Initialize(); e != nil {
		t.Fatal(e)
	}
	sources := catalog.New(pool, manifest)
	owner := owneridentity.WithActor(context.Background(), owneridentity.Actor{OwnerID: "owner", Channel: "web", DeliveryID: "lifecycle-cache"})
	_, e := pool.Exec(owner, `INSERT INTO sessions(session_key,user_id,channel) VALUES('web:owner','owner','web'); INSERT INTO runs(run_id,session_key,input_event_id,status,autonomy_mode,current_state,model_provider,started_at,updated_at) VALUES('run','web:owner','input','running','mode_1','model_running','fixture',clock_timestamp(),clock_timestamp()); INSERT INTO session_ownership(session_key,run_id,lease_id,fence_epoch,expires_at) VALUES('web:owner','run','lease',1,clock_timestamp()+INTERVAL '1 hour')`)
	if e != nil {
		t.Fatal(e)
	}
	live := ownership.WithToken(context.Background(), ownership.NewAuthority(pool), ownership.Token{SessionKey: "web:owner", RunID: "run", LeaseID: "lease", FenceEpoch: 1})
	src, e := sources.Put(owner, catalog.PutSource{ExternalKey: "private-cache", Kind: "document", ScopeType: "user", ScopeID: "owner", Body: "PRIVATE_LIFECYCLE_SYNTHETIC_83"})
	if e != nil {
		t.Fatal(e)
	}
	if _, e = sources.Get(live, src.ID); e != nil {
		t.Fatal(e)
	}
	redis, e := redisstore.Open(owner, redisstore.Config{URL: dsn}, nil)
	if e != nil {
		t.Fatal(e)
	}
	defer redis.Close()
	transient := working.NewTransientStore(redis.Client())
	for _, state := range []working.TransientState{{SessionKey: "web:owner", RunID: "run", ScratchJSON: `{"private":"PRIVATE_LIFECYCLE_SYNTHETIC_83"}`}, {SessionKey: "web:other", RunID: "independent", ScratchJSON: `{"valid":"keep"}`}} {
		if _, e = transient.Save(owner, state, time.Hour); e != nil {
			t.Fatal(e)
		}
	}
	defer transient.Clear(context.Background(), "web:other", "independent")
	if _, e = sources.Delete(owner, src.ID, "delete", 1); e != nil {
		t.Fatal(e)
	}
	// Reconstruct the owning store and production cleaner. The pending PG
	// cursor survives the process; transient data must be removed, not hidden.
	restarted := catalog.New(pool, manifest)
	cleaner := privacyCacheCleaner{store: working.NewTransientStore(redis.Client())}
	for range 3 {
		if _, e = restarted.CleanupPending(owner, cleaner); e != nil {
			t.Fatal(e)
		}
	}
	if _, e = transient.Get(owner, "web:owner", "run"); !errors.Is(e, working.ErrTransientStateNotFound) {
		t.Fatal("deleted cache retained private bytes", e)
	}
	if state, e := transient.Get(owner, "web:other", "independent"); e != nil || state.ScratchJSON != `{"valid":"keep"}` {
		t.Fatal("foreign cache removed", e)
	}
	called := false
	if e = restarted.GuardCacheWrite(live, "run", func(ctx context.Context) error {
		called = true
		_, e := transient.Save(ctx, working.TransientState{SessionKey: "web:owner", RunID: "run", ScratchJSON: `{"private":"late"}`}, time.Hour)
		return e
	}); e == nil || called {
		t.Fatal("late cache writer resurrected private data", e)
	}
	if e = cleaner.PurgeWorking(owner, catalog.CacheRef{SessionKey: "web:owner", RunID: "run"}); e != nil {
		t.Fatal("retry cleanup not idempotent", e)
	}
}
