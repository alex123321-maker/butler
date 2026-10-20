package indexing

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/butler/butler/internal/memory/catalog"
	"github.com/butler/butler/internal/memory/jobs"
	"github.com/butler/butler/internal/memory/semantic"
)

func automaticPolicy(r *Runtime) jobs.IndexPolicy {
	proof, _ := json.Marshal(semantic.Proof{ArtifactRef: "controlled-profile-fixture", ArtifactSHA256: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", CorpusVersion: "controlled-v1", Cases: 12, BaselineTop1: .5, CandidateTop1: .75})
	return jobs.IndexPolicy{ID: "fixture-auto-index", SpaceID: r.Space.ID(), MaxChunks: 100, MaxJobsPerDay: 8, Allowance: request(catalog.Source{}).Job.Allowance, ExpiresAt: time.Now().UTC().Add(time.Hour).Truncate(time.Microsecond), Proof: proof}
}
func TestAutomaticIndexPolicyRestartReuseAndDeletionIntegration(t *testing.T) {
	r, owner, src, m := fixture(t)
	p, e := r.CreatePolicy(owner, automaticPolicy(r))
	if e != nil {
		t.Fatal("create policy", e)
	}
	if e = r.Schedule(context.Background()); e != nil {
		t.Fatal("schedule", e)
	}
	if ok, e := r.ProcessOne(context.Background()); !ok || e != nil {
		t.Fatal("automatic index", ok, e)
	}
	store := semantic.NewStore(r.Catalog.Pool())
	g, e := store.Active(owner, semantic.Scope{Type: "user", ID: p.OwnerID})
	if e != nil || g.State != "ready" {
		t.Fatal("not active", e)
	}
	if e = r.Schedule(context.Background()); e != nil {
		t.Fatal(e)
	}
	// Force a fresh scanner check after its quiet window; no redundant job.
	if _, e = r.Catalog.Pool().Exec(owner, `UPDATE memory_index_policies SET next_at=clock_timestamp()`); e != nil {
		t.Fatal(e)
	}
	if e = r.Schedule(context.Background()); e != nil {
		t.Fatal(e)
	}
	var count int
	if e = r.Catalog.Pool().QueryRow(owner, `SELECT count(*) FROM memory_aux_jobs`).Scan(&count); e != nil || count != 1 {
		t.Fatal("unchanged generation was duplicated", count, e)
	}
	var vectors int
	if e = r.Catalog.Pool().QueryRow(owner, `SELECT count(*) FROM semantic_index_entries`).Scan(&vectors); e != nil || vectors != 5 {
		t.Fatal("coverage", vectors, e)
	}
	if m.calls.Load() != 2 {
		t.Fatal("unexpected inference count", m.calls.Load())
	}
	if _, e = r.Catalog.Delete(owner, src.ID, "delete", src.Revision); e != nil {
		t.Fatal(e)
	}
	if e = r.Catalog.Pool().QueryRow(owner, `SELECT count(*) FROM semantic_index_entries`).Scan(&vectors); e != nil || vectors != 0 {
		t.Fatal("deleted vectors survived", vectors, e)
	}
}
func TestAutomaticIndexPolicyRevocationDuringInferenceIntegration(t *testing.T) {
	r, owner, _, m := fixture(t)
	p, e := r.CreatePolicy(owner, automaticPolicy(r))
	if e != nil {
		t.Fatal(e)
	}
	if e = r.Schedule(context.Background()); e != nil {
		t.Fatal(e)
	}
	m.after = func() {
		m.after = nil
		if e := r.Jobs.ControlIndexPolicy(owner, p.ID, p.Revision, false); e != nil {
			t.Fatal(e)
		}
	}
	if _, e = r.ProcessOne(context.Background()); e == nil {
		t.Fatal("revoked policy published a generation")
	}
	var count int
	if e = r.Catalog.Pool().QueryRow(owner, `SELECT count(*) FROM semantic_index_active`).Scan(&count); e != nil || count != 0 {
		t.Fatal("revoked index active", count, e)
	}
}
func TestAutomaticIndexPolicyCannotWidenAllowanceOrOwnerIntegration(t *testing.T) {
	r, owner, _, _ := fixture(t)
	p, e := r.CreatePolicy(owner, automaticPolicy(r))
	if e != nil {
		t.Fatal(e)
	}
	p.OwnerID = "other-owner"
	_ = r.schedule(context.Background(), p)
	p.OwnerID = "owner"
	p.Allowance.Tokens++
	if e = r.schedule(context.Background(), p); e == nil {
		t.Fatal("widened allowance admitted")
	}
	var n int
	if e = r.Catalog.Pool().QueryRow(owner, `SELECT count(*) FROM memory_aux_jobs`).Scan(&n); e != nil || n != 0 {
		t.Fatal("forged policy left work", n, e)
	}
}

func TestAutomaticIndexPolicyReplacementKeepsOldJobsFencedIntegration(t *testing.T) {
	r, owner, _, _ := fixture(t)
	p, e := r.CreatePolicy(owner, automaticPolicy(r))
	if e != nil {
		t.Fatal(e)
	}
	if e = r.Schedule(context.Background()); e != nil {
		t.Fatal(e)
	}
	if e = r.Jobs.ControlIndexPolicy(owner, p.ID, p.Revision, false); e != nil {
		t.Fatal(e)
	}
	replacement := automaticPolicy(r)
	replacement.ID = "replacement-index-policy"
	replacement.Allowance.ModelTurns = 256
	if _, e = r.CreatePolicy(owner, replacement); e != nil {
		t.Fatal("replacement failed", e)
	}
	if _, e = r.ProcessOne(context.Background()); e == nil {
		t.Fatal("old policy job regained permission")
	}
}

func TestAutomaticIndexPolicyRollbackRetainsHistoryIntegration(t *testing.T) {
	r, owner, _, _ := fixture(t)
	down, e := os.ReadFile(filepath.Join("..", "..", "..", "migrations", "087_automatic_memory_index.down.sql"))
	if e != nil {
		t.Fatal(e)
	}
	if _, e = r.CreatePolicy(owner, automaticPolicy(r)); e != nil {
		t.Fatal(e)
	}
	if _, e = r.Catalog.Pool().Exec(owner, string(down)); e == nil || !strings.Contains(e.Error(), "retain automatic index policy") {
		t.Fatal("policy history lost", e)
	}
	var n int
	if e = r.Catalog.Pool().QueryRow(owner, `SELECT count(*) FROM memory_index_policies`).Scan(&n); e != nil || n != 1 {
		t.Fatal(n, e)
	}
}
