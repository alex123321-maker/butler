package catalog_test

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"
	"time"

	"github.com/butler/butler/internal/actionauth"
	"github.com/butler/butler/internal/budget"
	"github.com/butler/butler/internal/memory/catalog"
	"github.com/butler/butler/internal/memory/jobs"
	"github.com/butler/butler/internal/memory/memorytest"
	"github.com/butler/butler/internal/owneridentity"
)

func TestFreshPublicationSourceRequiresConstructorTransactionAndCurrentParentsIntegration(t *testing.T) {
	for _, mode := range []string{"fresh", "savepoint", "serialized", "reloaded", "wrong_transaction", "changed_identity", "withdrawn_access", "cancelled_claim"} {
		t.Run(mode, func(t *testing.T) {
			pool := memorytest.Database(t)
			owner := owneridentity.WithActor(context.Background(), owneridentity.Actor{OwnerID: "owner", Channel: "web", DeliveryID: "fresh-publication"})
			manifest := catalog.NewFileManifest(filepath.Join(t.TempDir(), "anchor"))
			if err := manifest.Initialize(); err != nil {
				t.Fatal(err)
			}
			c := catalog.New(pool, manifest)
			parent, err := c.Put(owner, catalog.PutSource{ExternalKey: "publication-input", Kind: "document", ScopeType: "user", ScopeID: "owner", Body: "Current bounded original"})
			if err != nil {
				t.Fatal(err)
			}
			ref := catalog.SourceRef{ID: parent.ID, Revision: parent.Revision, AccessRevision: parent.AccessRevision, Hash: parent.Hash}
			b := budget.NewController(pool, budget.DefaultPolicy())
			b.SetAuthorityResolver(actionauth.ResolveBudgetAuthority)
			j := jobs.New(pool, c, b, nil)
			if _, err = j.Enqueue(owner, jobs.Request{ID: "fresh-publication-job", Kind: "ingest", Purpose: "local_extract", Sources: []catalog.SourceRef{ref}, Allowance: budget.Units{ModelTurns: 1, Tokens: 4096, ActiveMillis: 60000, ProviderInputBytes: 4096}, Deadline: time.Now().Add(time.Hour)}); err != nil {
				t.Fatal(err)
			}
			job, ok, err := j.ClaimPurpose(context.Background(), "ingest", "local_extract")
			if err != nil || !ok {
				t.Fatal(ok, err)
			}
			ctx := j.Context(context.Background(), job)
			tx, err := pool.Begin(ctx)
			if err != nil {
				t.Fatal(err)
			}
			defer tx.Rollback(ctx)
			construction := tx
			if mode == "savepoint" {
				construction, err = tx.Begin(ctx)
				if err != nil {
					t.Fatal(err)
				}
			}
			fresh, err := c.CreateDerivedTx(ctx, construction, []catalog.SourceRef{ref}, job.Purpose, "Neutral output", "source-bound generated DATA", json.RawMessage(`{}`))
			if err != nil {
				t.Fatal(err)
			}
			if mode == "savepoint" {
				if err = construction.Commit(ctx); err != nil {
					t.Fatal(err)
				}
			}
			switch mode {
			case "serialized":
				raw, _ := json.Marshal(fresh)
				var decoded catalog.Source
				if json.Unmarshal(raw, &decoded) != nil {
					t.Fatal("source serialization")
				}
				fresh = decoded
			case "reloaded":
				rows, err := c.AccessTx(owner, tx, []catalog.SourceRef{{ID: fresh.ID}}, "memory_read")
				if err != nil {
					t.Fatal(err)
				}
				fresh = rows[0]
			case "changed_identity":
				fresh.ScopeID = "another-owner"
			case "wrong_transaction":
				if err = tx.Commit(ctx); err != nil {
					t.Fatal(err)
				}
				tx, err = pool.Begin(ctx)
				if err != nil {
					t.Fatal(err)
				}
				defer tx.Rollback(ctx)
			case "withdrawn_access":
				if _, err = tx.Exec(ctx, `UPDATE memory_sources SET access_revision=access_revision+1 WHERE source_id=$1`, parent.ID); err != nil {
					t.Fatal(err)
				}
			case "cancelled_claim":
				if _, err = tx.Exec(ctx, `UPDATE memory_aux_jobs SET status='cancelled',claim_id='',claim_until=NULL WHERE job_id=$1`, job.ID); err != nil {
					t.Fatal(err)
				}
			}
			publication, err := c.WithFreshPublicationSourceTx(ctx, tx, fresh, []catalog.SourceRef{ref}, job.Purpose)
			positive := mode == "fresh" || mode == "savepoint"
			if positive {
				if err != nil {
					t.Fatal("constructor output denied", err)
				}
				claim, err := c.ProposeTx(publication, tx, catalog.ClaimRequest{MemoryType: "episodic", SourceID: fresh.ID, SourceRevision: fresh.Revision, Key: "episode.test", Summary: "Neutral candidate", Value: json.RawMessage(`{"unverified":true}`), DedupKey: "fresh-output"})
				if err != nil || claim.Status != "candidate" {
					t.Fatal("fresh source candidate failed", err)
				}
				if _, err = c.ProposeTx(publication, tx, catalog.ClaimRequest{MemoryType: "episodic", SourceID: "unlisted-source", SourceRevision: 1, Key: "episode.invalid", Summary: "Denied", Value: json.RawMessage(`{}`), DedupKey: "unlisted"}); err == nil {
					t.Fatal("publication context granted other source")
				}
			} else if err == nil {
				t.Fatal("forged/old/withdrawn source blessed")
			}
			if !positive {
				var count int
				if err = tx.QueryRow(ctx, `SELECT count(*) FROM memory_claims`).Scan(&count); err != nil || count != 0 {
					t.Fatal("denial wrote a claim", count, err)
				}
			}
		})
	}
}
