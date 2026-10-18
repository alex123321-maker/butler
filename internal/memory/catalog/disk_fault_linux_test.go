package catalog

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"

	"github.com/butler/butler/internal/memory/memorytest"
	"github.com/butler/butler/internal/owneridentity"
)

// A dedicated eight-KiB tmpfs bounds this real ENOSPC experiment. It never fills
// the host disk, the operator anchor, a shared filesystem or a production DB.
func TestActualFullAnchorDiskFailsBoundedlyWithoutClaimingDeletionIntegration(t *testing.T) {
	if os.Getenv("BUTLER_MEMORY_TINY_ANCHOR") != "/bounded-anchor-fault" {
		t.Skip("explicit private eight-KiB tmpfs required")
	}
	const root = "/bounded-anchor-fault"
	var fs syscall.Statfs_t
	if err := syscall.Statfs(root, &fs); err != nil || fs.Type != 0x01021994 || fs.Blocks*uint64(fs.Bsize) > 8192 {
		t.Fatal("bounded tmpfs identity required")
	}
	if err := os.WriteFile(filepath.Join(root, "filler"), make([]byte, 4096), 0600); err != nil {
		t.Fatal(err)
	}
	manifest := NewFileManifest(filepath.Join(root, "anchor"))
	if err := manifest.Initialize(); err != nil {
		t.Fatal(err)
	}
	pool := memorytest.Database(t)
	ctx, stop := context.WithTimeout(context.Background(), 45*time.Second)
	defer stop()
	owner := owneridentity.WithActor(ctx, owneridentity.Actor{OwnerID: "disk-fault-owner", Channel: "web", DeliveryID: "bounded-disk-fault"})
	s := New(pool, manifest)
	for i := 0; i < 32; i++ {
		source, err := s.Put(owner, PutSource{ExternalKey: fmt.Sprintf("bounded-disk-%d", i), Kind: "document", ScopeType: "user", ScopeID: "disk-fault-owner", Body: "Explicitly synthetic disk fault source"})
		if err != nil {
			t.Fatal(err)
		}
		var before int
		if err = pool.QueryRow(owner, `SELECT count(*) FROM memory_deletions`).Scan(&before); err != nil {
			t.Fatal(err)
		}
		call, done := context.WithTimeout(owner, 5*time.Second)
		_, err = s.Delete(call, source.ID, "delete", source.Revision)
		done()
		if err == nil {
			continue
		}
		if !errors.Is(err, syscall.ENOSPC) {
			t.Fatal("expected actual bounded ENOSPC", err)
		}
		var status string
		var after int
		if err = pool.QueryRow(owner, `SELECT status,(SELECT count(*)FROM memory_deletions)FROM memory_sources WHERE source_id=$1`, source.ID).Scan(&status, &after); err != nil || status != "active" || before != after {
			t.Fatal("failed anchor write became committed deletion", err)
		}
		// A partial append can make the anchor unavailable. That must deny use
		// and reconciliation explicitly; no fallback creates an empty anchor.
		check := s.CheckRestore(owner)
		if check != nil {
			if !errors.Is(check, ErrRestore) {
				t.Fatal(check)
			}
			if _, err = s.Get(owner, source.ID); !errors.Is(err, ErrRestore) {
				t.Fatal("corrupt anchor still served memory", err)
			}
			if err = s.Reconcile(owner); !errors.Is(err, ErrRestore) {
				t.Fatal("corrupt anchor was silently reset", err)
			}
			t.Log("actual ENOSPC retained unchanged PG deletion state; partial anchor is explicitly fail-closed")
		} else {
			t.Log("actual ENOSPC retained unchanged PG deletion state and an intact prior anchor")
		}
		return
	}
	t.Fatal("bounded filesystem did not exhaust; missing actual disk fault")
}
