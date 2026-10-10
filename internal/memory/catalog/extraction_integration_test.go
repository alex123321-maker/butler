package catalog

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestExtractionDedupKeyBodyMismatchAndDowngradeIntegration(t *testing.T) {
	s, owner := fixture(t)
	src := put(t, s, owner, "source", "source")
	p := ClaimRequest{SourceID: src.ID, SourceRevision: src.Revision, Key: "candidate", Value: json.RawMessage(`{"raw":"source"}`), Summary: "source", Confidence: .5, DedupKey: "trusted-extraction-fingerprint"}
	tx, e := s.pool.Begin(owner)
	if e != nil {
		t.Fatal(e)
	}
	a, e := s.ProposeTx(owner, tx, p)
	if e != nil {
		t.Fatal(e)
	}
	if e = tx.Commit(owner); e != nil {
		t.Fatal(e)
	}
	tx, e = s.pool.Begin(owner)
	if e != nil {
		t.Fatal(e)
	}
	b, e := s.ProposeTx(owner, tx, p)
	if e != nil || a.ID != b.ID {
		t.Fatal(e)
	}
	if e = tx.Commit(owner); e != nil {
		t.Fatal(e)
	}
	tx, e = s.pool.Begin(owner)
	if e != nil {
		t.Fatal(e)
	}
	p.Value = json.RawMessage(`{"raw":"different"}`)
	if _, e = s.ProposeTx(owner, tx, p); !errors.Is(e, ErrStale) {
		t.Fatal("changed retry body accepted", e)
	}
	tx.Rollback(context.Background())
	data, err := os.ReadFile(filepath.Join("..", "..", "..", "migrations", "043_memory_extraction_dedup.down.sql"))
	if err != nil {
		t.Fatal(err)
	}
	if _, e = s.pool.Exec(owner, string(data)); e == nil {
		t.Fatal("populated downgrade allowed")
	}
}
