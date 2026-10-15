package compaction

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
	"github.com/butler/butler/internal/memory/summary"
	"github.com/butler/butler/internal/owneridentity"
	"github.com/jackc/pgx/v5/pgxpool"
)

// This receipt crosses processes around real pg_dump/pg_restore. It contains
// only opaque identities/revisions/hashes, never copied transcript/model text.
type summaryBackupReceipt struct {
	Version   int               `json:"version"`
	Target    string            `json:"target"`
	Bootstrap string            `json:"bootstrap_snapshot_id"`
	Primary   catalog.SourceRef `json:"primary"`
	Original  catalog.SourceRef `json:"original"`
	Candidate catalog.SourceRef `json:"candidate"`
	Input     catalog.SourceRef `json:"input"`
	Result    catalog.SourceRef `json:"result"`
	Unrelated catalog.SourceRef `json:"unrelated"`
}

// Run in three separate processes around the external backup driver. An
// ordinary package suite must not substitute a same-database replay for restore.
func TestActualPostgresSummarySnapshotDumpRestoreDeletionIntegration(t *testing.T) {
	phase := os.Getenv("BUTLER_TEST_SUMMARY_BACKUP_PHASE")
	if phase == "" {
		t.Skip("actual external summary pg_dump/restore procedure required")
	}
	target := os.Getenv("BUTLER_TEST_SUMMARY_BACKUP_TARGET")
	if target != "message" && target != "ancestor" {
		t.Fatal("explicit message or ancestor backup case required")
	}
	cfg, err := pgxpool.ParseConfig(os.Getenv("BUTLER_TEST_SUMMARY_BACKUP_POSTGRES_URL"))
	backupDB, restoreDB := "butler_summary_"+target+"_backup_test", "butler_summary_"+target+"_restore_test"
	if err != nil || cfg == nil || (cfg.ConnConfig.Database != backupDB && cfg.ConnConfig.Database != restoreDB) || (phase == "restore") != (cfg.ConnConfig.Database == restoreDB) {
		t.Fatal("dedicated summary backup/restore database required")
	}
	cfg.MaxConns = 2
	ctx, stop := context.WithTimeout(context.Background(), time.Minute)
	defer stop()
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	proof := os.Getenv("BUTLER_TEST_SUMMARY_BACKUP_RECEIPT")
	if !filepath.IsAbs(proof) {
		t.Fatal("private content-free receipt path required")
	}
	if phase == "seed" {
		seedSummaryBackup(t, ctx, pool, proof, target)
		return
	}
	raw, err := os.ReadFile(proof)
	if err != nil || len(raw) > 8192 {
		t.Fatal("private summary receipt unavailable")
	}
	var receipt summaryBackupReceipt
	if decodeStrict(string(raw), &receipt) != nil || receipt.Version != 1 || receipt.Target != target {
		t.Fatal("invalid summary backup receipt")
	}
	manifest := catalog.NewFileManifest(os.Getenv("BUTLER_SUMMARY_NATIVE_MANIFEST"))
	cat := catalog.New(pool, manifest)
	ctx = owneridentity.WithActor(ctx, owneridentity.Actor{OwnerID: "owner", Channel: "web", DeliveryID: "summary-backup-proof"})
	switch phase {
	case "delete":
		ref := receipt.Primary
		if target == "ancestor" {
			ref = receipt.Original
		}
		if _, err = cat.Delete(ctx, ref.ID, "delete", ref.Revision); err != nil {
			t.Fatal(err)
		}
	case "restore":
		if err = cat.CheckRestore(ctx); !errors.Is(err, catalog.ErrRestore) {
			t.Fatal("old restored database accepted the newer independent anchor", err)
		}
		for _, ref := range []catalog.SourceRef{receipt.Input, receipt.Result, receipt.Unrelated} {
			if _, err = cat.Get(ctx, ref.ID); !errors.Is(err, catalog.ErrRestore) {
				t.Fatal("restored copied data readable before anchor reconciliation", err)
			}
		}
		if err = cat.Reconcile(ctx); err != nil {
			t.Fatal(err)
		}
		// Replay is idempotent; it cannot replenish or replace model attempts.
		if err = cat.Reconcile(ctx); err != nil {
			t.Fatal(err)
		}
	default:
		t.Fatal("unknown summary backup phase")
	}
	assertSummaryBackupDeleted(t, ctx, pool, cat, receipt)
}

func seedSummaryBackup(t *testing.T, ctx context.Context, pool *pgxpool.Pool, proof, target string) {
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
	f := newConsumerFixture(t, pool)
	r := summaryBackupReceipt{Version: 1, Target: target, Bootstrap: f.policy.Job.Sources[0].ID}
	if err = f.js.ControlModel(f.ctx, f.policy.ID, "revoked"); err != nil {
		t.Fatal(err)
	}
	put := func(key, kind, body string, parents []catalog.SourceRef) catalog.SourceRef {
		t.Helper()
		src, err := f.cat.Put(f.ctx, catalog.PutSource{ExternalKey: key, Kind: kind, ScopeType: "user", ScopeID: "owner", Title: key, Body: body, Parents: parents})
		if err != nil {
			t.Fatal(err)
		}
		return catalog.SourceRef{ID: src.ID, Revision: src.Revision, AccessRevision: src.AccessRevision, Hash: src.Hash}
	}
	r.Original = put("summary-backup-original", "document", "Synthetic original for summary privacy removal", nil)
	r.Candidate = put("summary-backup-candidate", "model_candidate", "Unconfirmed candidate derived from the original", []catalog.SourceRef{r.Original})
	r.Unrelated = put("summary-backup-unrelated", "document", "UNRELATED_SUMMARY_BACKUP_RECORD", nil)
	primary, err := f.cat.Get(f.ctx, "message:"+f.firstMessage(t))
	if err != nil {
		t.Fatal(err)
	}
	r.Primary = catalog.SourceRef{ID: primary.ID, Revision: primary.Revision, AccessRevision: primary.AccessRevision, Hash: primary.Hash}
	// A reported message inherits a candidate's original privacy graph. The
	// following owner policy is new; old authority is never silently repinned.
	if _, err = pool.Exec(ctx, `INSERT INTO memory_source_dependencies(child_source_id,parent_source_id,parent_revision) VALUES($1,$2,$3)`, r.Primary.ID, r.Candidate.ID, r.Candidate.Revision); err != nil {
		t.Fatal(err)
	}
	q := f.policy.ModelPolicyRequest
	q.ID, q.Job.Sources = "summary-backup-current-policy", nil
	f.policy, err = (&Policies{Catalog: f.cat, Jobs: f.js}).Create(f.ctx, PolicyRequest{SessionKey: "session", Policy: q})
	if err != nil {
		t.Fatal(err)
	}
	f.poll(t)
	if did, err := f.worker.ProcessOne(f.ctx); err != nil || !did || f.provider.calls != 1 {
		t.Fatal("controlled actual worker did not publish one summary", did, err)
	}
	input, err := f.cat.Get(f.ctx, f.policy.Job.Sources[0].ID)
	if err != nil || input.Body == "" {
		t.Fatal("current copied input unavailable", err)
	}
	r.Input = catalog.SourceRef{ID: input.ID, Revision: input.Revision, AccessRevision: input.AccessRevision, Hash: input.Hash}
	var resultID string
	if err = pool.QueryRow(ctx, `SELECT result_source_id FROM memory_model_attempts WHERE state='published'`).Scan(&resultID); err != nil {
		t.Fatal(err)
	}
	result, err := f.cat.Get(f.ctx, resultID)
	if err != nil || result.Body == "" {
		t.Fatal("published summary derivative unavailable", err)
	}
	r.Result = catalog.SourceRef{ID: result.ID, Revision: result.Revision, AccessRevision: result.AccessRevision, Hash: result.Hash}
	var ordinaryParents, originals, coverage int
	if err = pool.QueryRow(ctx, `SELECT
 (SELECT count(*) FROM memory_source_dependencies WHERE child_source_id=$1),
 (SELECT count(*) FROM memory_summary_dependencies WHERE snapshot_source_id=$1 AND original_source_id=ANY($2)),
 (SELECT count(*) FROM session_summaries WHERE session_key='session' AND result_source_id=$3 AND through_sequence=8 AND summary<>'')`, r.Input.ID, []string{r.Primary.ID, r.Candidate.ID, r.Original.ID}, r.Result.ID).Scan(&ordinaryParents, &originals, &coverage); err != nil || ordinaryParents != 0 || originals != 3 || coverage != 1 {
		t.Fatal("fixture did not exercise unparented copied snapshot dependencies", ordinaryParents, originals, coverage, err)
	}
	out, err := os.OpenFile(proof, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
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
	t.Log("actual controlled worker published unparented copied input, catalog result and covered summary; receipt contains IDs only")
}

func assertSummaryBackupDeleted(t *testing.T, ctx context.Context, pool *pgxpool.Pool, cat *catalog.Store, r summaryBackupReceipt) {
	t.Helper()
	if err := cat.CheckRestore(ctx); err != nil {
		t.Fatal(err)
	}
	refs := []catalog.SourceRef{r.Primary, r.Input, r.Result}
	if r.Target == "ancestor" {
		refs = append(refs, r.Candidate, r.Original)
	}
	for _, ref := range refs {
		if _, err := cat.Get(ctx, ref.ID); !errors.Is(err, catalog.ErrDenied) && !errors.Is(err, catalog.ErrStale) {
			t.Fatal("deleted original/copied snapshot/result remained readable", err)
		}
		var retained int
		var hash, status string
		if err := pool.QueryRow(ctx, `SELECT s.status,r.content_hash,(SELECT COALESCE(sum(length(body)+length(title)+length(author)),0)::int FROM memory_source_revisions WHERE source_id=s.source_id) FROM memory_sources s JOIN memory_source_revisions r ON r.source_id=s.source_id AND r.revision=$2 WHERE s.source_id=$1`, ref.ID, ref.Revision).Scan(&status, &hash, &retained); err != nil || status != "deleted" || hash != ref.Hash || retained != 0 {
			t.Fatal("deleted copy text survived or evidence identity changed", status, retained, err)
		}
	}
	if _, err := cat.Get(ctx, r.Bootstrap); err == nil {
		t.Fatal("earlier copied input regained access")
	}
	snapshot, err := summary.NewStore(pool).GetSummarySnapshot(ctx, "session")
	if err != nil || snapshot.Summary != "" {
		t.Fatal("restored coverage hid primary history", err)
	}
	var coverage, attempts, observed int
	if err = pool.QueryRow(ctx, `SELECT (SELECT count(*) FROM session_summaries),(SELECT count(*) FROM memory_model_attempts WHERE state='published' AND result_source_id=$1),(SELECT count(*) FROM memory_model_attempts WHERE usage IS NOT NULL)`, r.Result.ID).Scan(&coverage, &attempts, &observed); err != nil || coverage != 0 || attempts != 1 || observed != 1 {
		t.Fatal("coverage retained or original paid attempt/evidence erased", coverage, attempts, observed, err)
	}
	unrelated, err := cat.Get(ctx, r.Unrelated.ID)
	if err != nil || unrelated.Body != "UNRELATED_SUMMARY_BACKUP_RECORD" || unrelated.Hash != r.Unrelated.Hash || unrelated.Revision != r.Unrelated.Revision || unrelated.AccessRevision != r.Unrelated.AccessRevision {
		t.Fatal("selective replay changed unrelated current source", err)
	}
	t.Log("deletion anchor replay erased copied input/result revisions and coverage, retained attempt/evidence hashes and unrelated source")
}
