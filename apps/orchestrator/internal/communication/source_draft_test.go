package communication_test

import (
	"context"
	"strings"
	"testing"
)

func TestCommunicationSourceDraftSelectionAndFencesIntegration(t *testing.T) {
	f := setup(t, fixtureOptions{Authority: true})
	const selected = "document:source-draft-selected"
	if _, err := f.pool.Exec(f.ctx, `INSERT INTO memory_sources(source_id,owner_id,source_kind,scope_type,scope_id,external_key) VALUES($1,$2,'document','user',$2,$1)`, selected, f.p.OwnerID); err != nil {
		t.Fatal(err)
	}
	if _, err := f.pool.Exec(f.ctx, `INSERT INTO memory_run_sources VALUES($1,$2,1,1,$3)`, f.p.RunID, selected, strings.Repeat("a", 64)); err != nil {
		t.Fatal(err)
	}
	for _, refs := range [][]string{{}, {selected}, {"source:" + selected}} {
		if ok, err := f.s.CheckPublicationSources(f.ctx, f.p.RunID, refs); err != nil || !ok {
			t.Fatal("selected draft refused", err)
		}
	}
	for _, refs := range [][]string{{"message:invented-owner-event"}, {"browser:context"}, {selected, "message:invented-owner-event"}} {
		if ok, err := f.s.CheckPublicationSources(f.ctx, f.p.RunID, refs); err != nil || ok {
			t.Fatal("unselected source accepted", err)
		}
	}
	if ok, err := f.s.CheckPublicationSources(context.Background(), f.p.RunID, []string{selected}); err == nil || ok {
		t.Fatal("missing ownership accepted")
	}
	if ok, err := f.s.CheckPublicationSources(f.ctx, "foreign-run", []string{selected}); err == nil || ok {
		t.Fatal("foreign run accepted")
	}
	f.revoked.Store(true)
	if ok, err := f.s.CheckPublicationSources(f.ctx, f.p.RunID, []string{selected}); err == nil || ok {
		t.Fatal("revoked current source policy accepted")
	}
	f.revoked.Store(false)
	if _, err := f.pool.Exec(f.ctx, `UPDATE accepted_inputs SET status='failed' WHERE input_id=$1`, f.p.InputID); err != nil {
		t.Fatal(err)
	}
	if ok, err := f.s.CheckPublicationSources(f.ctx, f.p.RunID, []string{selected}); err == nil || ok {
		t.Fatal("failed Input accepted")
	}
	var n int
	if f.pool.QueryRow(f.ctx, `SELECT count(*) FROM communication_publications`).Scan(&n) != nil || n != 0 {
		t.Fatal("draft published a message")
	}
	// Re-check under a fresh transaction: the checks did not reserve/dispatch
	// anything, change source selection, or manufacture a publication receipt.
	tx, err := f.pool.Begin(f.ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(f.ctx)
	if tx.QueryRow(f.ctx, `SELECT count(*) FROM action_authorizations`).Scan(&n) != nil || n != 0 {
		t.Fatal("draft spent a grant")
	}
}
