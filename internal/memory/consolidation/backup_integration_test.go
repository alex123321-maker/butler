package consolidation

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"testing"
	"time"

	"github.com/butler/butler/internal/memory/catalog"
	"github.com/butler/butler/internal/memory/jobs"
	"github.com/butler/butler/internal/owneridentity"
	"github.com/jackc/pgx/v5/pgxpool"
)

type consolidationBackupReceipt struct {
	Version                                     int    `json:"version"`
	Target                                      string `json:"target"`
	Original, Primary, Input, Result, Unrelated catalog.SourceRef
	ClaimID, UnrelatedClaimID, SpendHash        string
	Head                                        Head
}

// This is three independent processes around real custom pg_dump/pg_restore,
// never a same-database replay pretending to validate backup recovery.
func TestActualPostgresConsolidationDumpRestoreDeletionIntegration(t *testing.T) {
	phase := os.Getenv("BUTLER_TEST_CONSOLIDATION_BACKUP_PHASE")
	if phase == "" {
		t.Skip("external real consolidation backup driver required")
	}
	target := os.Getenv("BUTLER_TEST_CONSOLIDATION_BACKUP_TARGET")
	if target != "direct" && target != "ancestor" {
		t.Fatal("explicit backup target required")
	}
	cfg, err := pgxpool.ParseConfig(os.Getenv("BUTLER_TEST_CONSOLIDATION_BACKUP_POSTGRES_URL"))
	if err != nil || cfg == nil {
		t.Fatal("dedicated database required")
	}
	if cfg.ConnConfig.Database != "butler_consolidation_"+target+"_backup_test" && cfg.ConnConfig.Database != "butler_consolidation_"+target+"_restore_test" || (phase == "restore") != (cfg.ConnConfig.Database == "butler_consolidation_"+target+"_restore_test") {
		t.Fatal("isolated backup/restore database required")
	}
	cfg.MaxConns = 2
	ctx, stop := context.WithTimeout(context.Background(), time.Minute)
	defer stop()
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	proof := os.Getenv("BUTLER_TEST_CONSOLIDATION_BACKUP_RECEIPT")
	anchor := os.Getenv("BUTLER_TEST_CONSOLIDATION_BACKUP_ANCHOR")
	if !filepath.IsAbs(proof) || !filepath.IsAbs(anchor) {
		t.Fatal("absolute private proof paths required")
	}
	if phase == "seed" {
		seedConsolidationBackup(t, ctx, pool, proof, anchor, target)
		return
	}
	raw, err := os.ReadFile(proof)
	var r consolidationBackupReceipt
	if err != nil || len(raw) > 8192 || strict(string(raw), &r) != nil || r.Version != 1 || r.Target != target {
		t.Fatal("invalid private receipt")
	}
	ctx = owneridentity.WithActor(ctx, owneridentity.Actor{OwnerID: "owner", Channel: "web", DeliveryID: "backup"})
	c := catalog.New(pool, catalog.NewFileManifest(anchor))
	switch phase {
	case "delete":
		root := r.Primary
		if target == "ancestor" {
			root = r.Original
		}
		if _, err = c.Delete(ctx, root.ID, "delete", root.Revision); err != nil {
			t.Fatal(err)
		}
	case "restore":
		if err = c.CheckRestore(ctx); !errors.Is(err, catalog.ErrRestore) {
			t.Fatal("old backup ignored newest anchor", err)
		}
		for _, ref := range []catalog.SourceRef{r.Input, r.Result, r.Unrelated} {
			if _, err = c.Get(ctx, ref.ID); !errors.Is(err, catalog.ErrRestore) {
				t.Fatal("restored source readable before reconcile", err)
			}
		}
		if err = c.Reconcile(ctx); err != nil {
			t.Fatal(err)
		}
		if err = c.Reconcile(ctx); err != nil {
			t.Fatal("nonidempotent replay", err)
		}
	default:
		t.Fatal("unknown backup phase")
	}
	assertConsolidationBackupErased(t, ctx, pool, c, r)
}

func seedConsolidationBackup(t *testing.T, ctx context.Context, pool *pgxpool.Pool, proof, anchor, target string) {
	t.Helper()
	if _, err := pool.Exec(ctx, `CREATE EXTENSION IF NOT EXISTS vector`); err != nil {
		t.Fatal(err)
	}
	_, file, _, _ := runtime.Caller(0)
	paths, err := filepath.Glob(filepath.Join(filepath.Dir(file), "..", "..", "..", "migrations", "*.up.sql"))
	if err != nil {
		t.Fatal(err)
	}
	sort.Strings(paths)
	for _, path := range paths {
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if _, err = pool.Exec(ctx, string(raw)); err != nil {
			t.Fatalf("%s: %v", filepath.Base(path), err)
		}
	}
	m := catalog.NewFileManifest(anchor)
	if err = m.Initialize(); err != nil {
		t.Fatal(err)
	}
	f := newWorkflowOnPool(t, pool, m, "PRIVATE_CONSOLIDATION_BACKUP_SOURCE: гипотеза не подтверждена.", "UNRELATED_CONSOLIDATION_BACKUP_RECORD")
	r := consolidationBackupReceipt{Version: 1, Target: target, Original: refsOf(f.sources[:1])[0], Unrelated: refsOf(f.sources[1:])[0]}
	if target == "ancestor" {
		candidate, err := f.store.Catalog.Put(f.ctx, catalog.PutSource{ExternalKey: "copied-candidate", Kind: "model_candidate", ScopeType: "session", ScopeID: "s1", Body: "PRIVATE_CONSOLIDATION_BACKUP_CANDIDATE: повтор гипотезы не подтверждает её.", Parents: []catalog.SourceRef{r.Original}})
		if err != nil {
			t.Fatal(err)
		}
		f.sources[0] = candidate
	}
	r.Primary = refsOf(f.sources[:1])[0]
	for i, src := range f.sources {
		claim, err := f.store.Catalog.Propose(f.ctx, catalog.ClaimRequest{SourceID: src.ID, SourceRevision: 1, Key: "backup-evidence", Value: json.RawMessage(`"не разрешает внешнее действие"`), Summary: "Отдельное решение владельца", Confidence: 1})
		if err != nil {
			t.Fatal(err)
		}
		if _, err = f.store.Catalog.Decide(f.ctx, catalog.Decision{ClaimID: claim.ID}); err != nil {
			t.Fatal(err)
		}
		if _, err = f.store.Catalog.SetTemporal(f.ctx, catalog.TemporalUpdate{ClaimID: claim.ID, ExpectedHeadRevision: 1, Validity: catalog.ValidityUnknown, Class: catalog.FreshnessStablePreference}); err != nil {
			t.Fatal(err)
		}
		if i == 0 {
			r.ClaimID = claim.ID
		} else {
			r.UnrelatedClaimID = claim.ID
		}
	}
	q := f.request()
	q.ClaimIDs = []string{r.ClaimID}
	v := f.generate(t, q)
	r.Head = f.activate(t, v, Head{})
	r.Input = catalog.SourceRef{ID: v.Generation.InputID, Revision: 1}
	r.Result = v.Generation.Source
	input, err := f.store.Catalog.Get(f.ctx, r.Input.ID)
	if err != nil {
		t.Fatal(err)
	}
	r.Input = refsOf([]catalog.Source{input})[0]
	result, err := f.store.Catalog.Get(f.ctx, r.Result.ID)
	if err != nil {
		t.Fatal(err)
	}
	r.Result = refsOf([]catalog.Source{result})[0]
	r.SpendHash = consolidationSpendHash(t, ctx, pool)
	out, err := os.OpenFile(proof, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		t.Fatal(err)
	}
	defer out.Close()
	if err = json.NewEncoder(out).Encode(r); err != nil {
		t.Fatal(err)
	}
	if err = out.Sync(); err != nil {
		t.Fatal(err)
	}
	t.Log("actual worker published and owner activated one exact source/accepted-temporal representation; private receipt has identities/hashes only")
}

func consolidationSpendHash(t *testing.T, ctx context.Context, pool *pgxpool.Pool) string {
	t.Helper()
	var raw []byte
	if err := pool.QueryRow(ctx, `SELECT jsonb_build_object('attempts',(SELECT jsonb_agg(to_jsonb(a) ORDER BY operation_id) FROM memory_model_attempts a),'reservations',(SELECT jsonb_agg(to_jsonb(r) ORDER BY operation_id) FROM execution_budget_reservations r),'scopes',(SELECT jsonb_agg(to_jsonb(s) ORDER BY scope_id) FROM execution_budget_scopes s))`).Scan(&raw); err != nil {
		t.Fatal(err)
	}
	return digest(json.RawMessage(raw))
}

func assertConsolidationBackupErased(t *testing.T, ctx context.Context, pool *pgxpool.Pool, c *catalog.Store, r consolidationBackupReceipt) {
	t.Helper()
	if err := c.CheckRestore(ctx); err != nil {
		t.Fatal(err)
	}
	refs := []catalog.SourceRef{r.Primary, r.Input, r.Result}
	if r.Target == "ancestor" {
		refs = append(refs, r.Original)
	}
	for _, ref := range refs {
		if _, err := c.Get(ctx, ref.ID); err == nil {
			t.Fatal("deleted source/copy accessible", ref.ID)
		}
		var status, hash string
		var bytes int
		if err := pool.QueryRow(ctx, `SELECT s.status,r.content_hash,(SELECT COALESCE(sum(length(body)+length(title)+length(author)),0)::int FROM memory_source_revisions WHERE source_id=s.source_id) FROM memory_sources s JOIN memory_source_revisions r ON r.source_id=s.source_id AND r.revision=$2 WHERE s.source_id=$1`, ref.ID, ref.Revision).Scan(&status, &hash, &bytes); err != nil || status != "deleted" || hash != ref.Hash || bytes != 0 {
			t.Fatal("physical erasure or immutable identity failed", status, bytes, err)
		}
	}
	if consolidationSpendHash(t, ctx, pool) != r.SpendHash {
		t.Fatal("restore/delete mutated original known usage, quote or budget history")
	}
	var claimStatus, value, summary string
	if err := pool.QueryRow(ctx, `SELECT status,value_json::text,summary FROM memory_claims WHERE claim_id=$1`, r.ClaimID).Scan(&claimStatus, &value, &summary); err != nil || claimStatus != "deleted" || value != "{}" || summary != "" {
		t.Fatal("accepted claim copy not erased", err)
	}
	unrelated, err := c.Get(ctx, r.Unrelated.ID)
	if err != nil || unrelated.Body != "UNRELATED_CONSOLIDATION_BACKUP_RECORD" || unrelated.Hash != r.Unrelated.Hash || unrelated.AccessRevision != r.Unrelated.AccessRevision {
		t.Fatal("unrelated source changed", err)
	}
	page, err := c.ReadAcceptedFacts(ctx, []string{r.UnrelatedClaimID}, catalog.TemporalQuery{})
	if err != nil || len(page.Items) != 1 {
		t.Fatal("unrelated accepted decision lost", err)
	}
	s := &Store{Catalog: c, Jobs: jobs.New(pool, c, nil, nil)}
	if _, err = s.Activate(ctx, Activation{SourceID: r.Result.ID, ExpectedActiveID: r.Head.ActiveID, ExpectedRevision: r.Head.Revision}); err == nil {
		t.Fatal("restored selection resurrected deleted evidence")
	}
	t.Log("fresh restore/newest anchor double replay erased input/result/accepted text, preserved unrelated accepted source and exact historical quote/usage")
}
