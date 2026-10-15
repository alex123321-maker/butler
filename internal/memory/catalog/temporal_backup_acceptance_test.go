package catalog_test

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
	"github.com/butler/butler/internal/owneridentity"
	"github.com/jackc/pgx/v5/pgxpool"
)

type temporalBackupReceipt struct {
	Source, Derived, Unrelated catalog.SourceRef
	ClaimID, UnrelatedClaim    string
	Past                       time.Time
}

// Three independent processes run around a real old pg_dump/fresh pg_restore.
// The external deletion anchor is newer than the dump and is never rolled back.
func TestActualPostgresTemporalDumpRestoreDeletionIntegration(t *testing.T) {
	phase := os.Getenv("BUTLER_TEST_TEMPORAL_BACKUP_PHASE")
	if phase == "" {
		t.Skip("external temporal pg_dump/restore driver required")
	}
	cfg, err := pgxpool.ParseConfig(os.Getenv("BUTLER_TEST_TEMPORAL_BACKUP_POSTGRES_URL"))
	if err != nil || cfg == nil || (cfg.ConnConfig.Database != "butler_temporal_backup_test" && cfg.ConnConfig.Database != "butler_temporal_restore_test") || (phase == "restore") != (cfg.ConnConfig.Database == "butler_temporal_restore_test") {
		t.Fatal("dedicated temporal backup/restore DB required")
	}
	cfg.MaxConns = 2
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	anchor, receipt := os.Getenv("BUTLER_TEST_TEMPORAL_BACKUP_MANIFEST"), os.Getenv("BUTLER_TEST_TEMPORAL_BACKUP_RECEIPT")
	if !filepath.IsAbs(anchor) || !filepath.IsAbs(receipt) {
		t.Fatal("absolute independent anchor/receipt required")
	}
	m := catalog.NewFileManifest(anchor)
	s := catalog.New(pool, m)
	ctx = owneridentity.WithActor(ctx, owneridentity.Actor{OwnerID: "temporal-owner", Channel: "web", DeliveryID: "backup"})
	if phase == "seed" {
		if _, err = pool.Exec(ctx, `CREATE EXTENSION IF NOT EXISTS vector`); err != nil {
			t.Fatal(err)
		}
		_, file, _, _ := runtime.Caller(0)
		paths, err := filepath.Glob(filepath.Join(filepath.Dir(file), "..", "..", "..", "migrations", "*.up.sql"))
		if err != nil {
			t.Fatal(err)
		}
		sort.Strings(paths)
		for _, p := range paths {
			raw, err := os.ReadFile(p)
			if err != nil {
				t.Fatal(err)
			}
			if _, err = pool.Exec(ctx, string(raw)); err != nil {
				t.Fatal(filepath.Base(p), err)
			}
		}
		if err = m.Initialize(); err != nil {
			t.Fatal(err)
		}
		create := func(key, body string) (catalog.Source, catalog.Claim) {
			t.Helper()
			src, err := s.Put(ctx, catalog.PutSource{ExternalKey: key, Kind: "owner_statement", ScopeType: "user", ScopeID: "temporal-owner", Body: body})
			if err != nil {
				t.Fatal(err)
			}
			c, err := s.Propose(ctx, catalog.ClaimRequest{SourceID: src.ID, SourceRevision: 1, Key: key, Value: json.RawMessage(`"accepted"`), Summary: body})
			if err != nil {
				t.Fatal(err)
			}
			c, err = s.Decide(ctx, catalog.Decision{ClaimID: c.ID})
			if err != nil {
				t.Fatal(err)
			}
			return src, c
		}
		src, c := create("temporal-old", "Private historical technical assertion")
		past := time.Now().Add(-2 * time.Hour).UTC()
		until := past.Add(time.Hour)
		if _, err = s.SetTemporal(ctx, catalog.TemporalUpdate{ClaimID: c.ID, ExpectedHeadRevision: 1, Validity: catalog.ValidityInterval, ValidFrom: &past, ValidTo: &until, Class: catalog.FreshnessTechnicalFact}); err != nil {
			t.Fatal(err)
		}
		other, oc := create("temporal-unrelated", "Unrelated permanent owner preference")
		if _, err = s.SetTemporal(ctx, catalog.TemporalUpdate{ClaimID: oc.ID, ExpectedHeadRevision: 1, Validity: catalog.ValidityUnknown, Class: catalog.FreshnessStablePreference}); err != nil {
			t.Fatal(err)
		}
		tx, err := pool.Begin(ctx)
		if err != nil {
			t.Fatal(err)
		}
		defer tx.Rollback(ctx)
		q := catalog.TemporalQuery{ValidAt: &past}
		page, err := s.ReadAcceptedFactsTx(ctx, tx, []string{c.ID}, q)
		if err != nil || len(page.Items) != 1 || page.Items[0].Freshness.Status != "expired" {
			t.Fatal(page, err)
		}
		ref := catalog.SourceRef{ID: src.ID, Revision: src.Revision, AccessRevision: src.AccessRevision, Hash: src.Hash}
		d, err := s.CreateDerivedTx(ctx, tx, []catalog.SourceRef{ref}, "memory_read", "historical view", "Private derived historical assertion", json.RawMessage(`{}`))
		if err != nil {
			t.Fatal(err)
		}
		if err = s.BindNewAcceptedFactPinsTx(ctx, tx, d, []catalog.SourceRef{ref}, []catalog.AcceptedFactPin{page.Items[0].Pin}, q); err != nil {
			t.Fatal(err)
		}
		if err = tx.Commit(ctx); err != nil {
			t.Fatal(err)
		}
		r := temporalBackupReceipt{Source: ref, Derived: catalog.SourceRef{ID: d.ID, Revision: d.Revision, AccessRevision: d.AccessRevision, Hash: d.Hash}, ClaimID: c.ID, Unrelated: catalog.SourceRef{ID: other.ID, Revision: 1, AccessRevision: other.AccessRevision, Hash: other.Hash}, UnrelatedClaim: oc.ID, Past: past}
		raw, _ := json.Marshal(r)
		if err = os.WriteFile(receipt, raw, 0600); err != nil {
			t.Fatal(err)
		}
		return
	}
	raw, err := os.ReadFile(receipt)
	if err != nil || len(raw) > 8192 {
		t.Fatal("content-free backup receipt unavailable")
	}
	var r temporalBackupReceipt
	if json.Unmarshal(raw, &r) != nil || r.Source.ID == "" || r.ClaimID == "" {
		t.Fatal("invalid backup receipt")
	}
	if phase == "delete" {
		if _, err = s.Delete(ctx, r.Source.ID, "delete", r.Source.Revision); err != nil {
			t.Fatal(err)
		}
		return
	}
	if phase != "restore" {
		t.Fatal("unknown temporal backup phase")
	}
	var metadata, pins int
	if err = pool.QueryRow(ctx, `SELECT (SELECT count(*) FROM memory_claim_temporal_revisions WHERE claim_id=$1),(SELECT count(*) FROM memory_temporal_source_pins WHERE claim_id=$1)`, r.ClaimID).Scan(&metadata, &pins); err != nil || metadata != 1 || pins != 1 {
		t.Fatal("old dump did not retain historical semantic state", metadata, pins, err)
	}
	if _, err = s.ReadAcceptedFacts(ctx, []string{r.ClaimID}, catalog.TemporalQuery{ValidAt: &r.Past}); !errors.Is(err, catalog.ErrRestore) {
		t.Fatal("old DB bypassed newest anchor", err)
	}
	for i := 0; i < 2; i++ {
		if err = s.Reconcile(ctx); err != nil {
			t.Fatal(err)
		}
	}
	if err = s.CheckRestore(ctx); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{r.Source.ID, r.Derived.ID} {
		if _, err = s.Get(ctx, id); err == nil {
			t.Fatal("restored private history released", id)
		}
	}
	if err = pool.QueryRow(ctx, `SELECT (SELECT count(*) FROM memory_claim_temporal_revisions WHERE claim_id=$1)+(SELECT count(*) FROM memory_temporal_source_pins WHERE claim_id=$1)+(SELECT count(*) FROM memory_claim_temporal_heads WHERE claim_id=$1)`, r.ClaimID).Scan(&metadata); err != nil || metadata != 0 {
		t.Fatal("temporal metadata survived erasure/replay", metadata, err)
	}
	if err = pool.QueryRow(ctx, `SELECT count(*) FROM memory_source_revisions WHERE source_id=ANY($1) AND (body<>'' OR title<>'')`, []string{r.Source.ID, r.Derived.ID}).Scan(&metadata); err != nil || metadata != 0 {
		t.Fatal("source/derivative bytes survived", metadata, err)
	}
	page, err := s.ReadAcceptedFacts(ctx, []string{r.UnrelatedClaim}, catalog.TemporalQuery{})
	if err != nil || len(page.Items) != 1 || page.Items[0].Pin.Source != r.Unrelated || page.Items[0].Freshness.Status != "stable" {
		t.Fatal("replay changed unrelated accepted preference", page, err)
	}
}
