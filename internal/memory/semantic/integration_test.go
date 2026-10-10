package semantic

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/butler/butler/internal/memory/chunks"
	postgresstore "github.com/butler/butler/internal/storage/postgres"
	"github.com/jackc/pgx/v5"
)

func database(t *testing.T) (context.Context, *Store, *chunks.Store) {
	t.Helper()
	dsn := os.Getenv("BUTLER_TEST_SEMANTIC_POSTGRES_URL")
	if dsn == "" {
		t.Skip("BUTLER_TEST_SEMANTIC_POSTGRES_URL not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	t.Cleanup(cancel)
	pg, err := postgresstore.Open(ctx, postgresstore.Config{URL: dsn, MaxConns: 2, MinConns: 1}, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pg.Close)
	if err := pg.RunMigrations(ctx, filepath.Join("..", "..", "..", "migrations")); err != nil {
		t.Fatal(err)
	}
	if _, err := pg.Pool().Exec(ctx, `TRUNCATE semantic_index_active,semantic_index_entries,semantic_index_generations,memory_chunks CASCADE`); err != nil {
		t.Fatal(err)
	}
	s := NewStore(pg.Pool())
	s.Guard = labSourceGuard{}
	return ctx, s, chunks.NewStore(pg.Pool())
}

type labSourceGuard struct{ deny bool }

func (g labSourceGuard) CheckSources(_ context.Context, sources []Source) error {
	if g.deny {
		return ErrAccess
	}
	for _, src := range sources {
		if src.ID <= 0 || !strings.HasPrefix(src.Scope.ID, "semantic:") {
			return ErrAccess
		}
	}
	return nil
}
func (g labSourceGuard) CheckSourcesTx(ctx context.Context, _ pgx.Tx, sources []Source) error {
	return g.CheckSources(ctx, sources)
}
func qualityProof() Proof {
	return Proof{"fixture://quality", strings.Repeat("1", 64), "fixture-v1", 24, 0.2, 0.8}
}
func TestSemanticIndexPostgresReindexRollbackAndSourceChanges(t *testing.T) {
	ctx, s, cs := database(t)
	scope := Scope{"session", "semantic:fixture"}
	chunk, err := cs.Save(ctx, chunks.Chunk{ScopeType: scope.Type, ScopeID: scope.ID, Title: "Запрет вкладок", Content: "Нельзя переключать вкладки из-за привязки учётных данных.", Summary: "Привязка вкладки", SourceType: "message", SourceID: "msg-1"})
	if err != nil {
		t.Fatal(err)
	}
	sources, err := s.Sources(ctx, scope, 10)
	if err != nil || len(sources) != 1 {
		t.Fatal(err)
	}
	src := sources[0]
	g1, err := s.Create(ctx, testSpace(768), scope)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Verify(ctx, g1.ID, qualityProof()); err == nil {
		t.Fatal("incomplete generation verified")
	}
	if err := s.Put(ctx, g1.ID, src, unitVector(256)); err == nil {
		t.Fatal("mixed dimension accepted")
	}
	if err := s.Put(ctx, g1.ID, src, unitVector(768)); err != nil {
		t.Fatal(err)
	}
	if err := s.Activate(ctx, g1.ID, ""); err == nil {
		t.Fatal("unverified activation")
	}
	bad := qualityProof()
	bad.CandidateTop1 = bad.BaselineTop1
	if err := s.Verify(ctx, g1.ID, bad); err == nil {
		t.Fatal("no improvement accepted")
	}
	if err := s.Verify(ctx, g1.ID, qualityProof()); err != nil {
		t.Fatal(err)
	}
	if err := s.Activate(ctx, g1.ID, ""); err != nil {
		t.Fatal(err)
	}
	g1, _ = s.Generation(ctx, g1.ID)
	hits, err := s.Search(ctx, g1, unitVector(768), 3)
	if err != nil || len(hits) != 1 || hits[0].Source.SourceID != "msg-1" {
		t.Fatalf("source hits %v %v", hits, err)
	}
	g2, err := s.Create(ctx, testSpace(256), scope)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Put(ctx, g2.ID, src, unitVector(256)); err != nil {
		t.Fatal(err)
	}
	if err := s.Verify(ctx, g2.ID, qualityProof()); err != nil {
		t.Fatal(err)
	}
	active, _ := s.Active(ctx, scope)
	if active.ID != g1.ID {
		t.Fatal("building alongside switched active index")
	}
	if err := s.Activate(ctx, g2.ID, "wrong"); err == nil {
		t.Fatal("stale activation overwrote live choice")
	}
	if err := s.Activate(ctx, g2.ID, g1.ID); err != nil {
		t.Fatal(err)
	}
	if err := s.Activate(ctx, g1.ID, g2.ID); err != nil {
		t.Fatal("rollback", err)
	}
	// Editing even without incrementing updated_at cannot reuse an old vector.
	if _, err := s.pool.Exec(ctx, `UPDATE memory_chunks SET content='Новое правило: подготовить действие без исполнения' WHERE id=$1`, chunk.ID); err != nil {
		t.Fatal(err)
	}
	hits, err = s.Search(ctx, g1, unitVector(768), 3)
	if err != nil || len(hits) != 0 {
		t.Fatal("edited rule leaked old embedding", err)
	}
	if err := s.Activate(ctx, g2.ID, g1.ID); err == nil {
		t.Fatal("stale rollback permitted")
	}
	if err := s.Put(ctx, g2.ID, src, unitVector(256)); err == nil {
		t.Fatal("ready generation mutated")
	}
	// Restore the source, then suppression/expiry/withdrawal and cross-scope.
	if _, err := s.pool.Exec(ctx, `UPDATE memory_chunks SET content=$2 WHERE id=$1`, chunk.ID, src.Content); err != nil {
		t.Fatal(err)
	}
	for _, sql := range []string{`suppressed=true`, `confirmation_state='pending'`, `expires_at=clock_timestamp()-interval '1 second'`, `scope_id='different'`} {
		if _, err := s.pool.Exec(ctx, `UPDATE memory_chunks SET `+sql+` WHERE id=$1`, chunk.ID); err != nil {
			t.Fatal(err)
		}
		hits, err = s.Search(ctx, g1, unitVector(768), 3)
		if err != nil || len(hits) != 0 {
			t.Fatal("withdrawn source visible", sql, err)
		}
		if _, err := s.pool.Exec(ctx, `UPDATE memory_chunks SET suppressed=false,confirmation_state='auto_confirmed',expires_at=NULL,scope_id=$2 WHERE id=$1`, chunk.ID, scope.ID); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := s.pool.Exec(ctx, `DELETE FROM memory_chunks WHERE id=$1`, chunk.ID); err != nil {
		t.Fatal(err)
	}
	var count int
	if err := s.pool.QueryRow(ctx, `SELECT count(*) FROM semantic_index_entries`).Scan(&count); err != nil || count != 0 {
		t.Fatal("deletion left derivative", err)
	}
	// A downgrade may not discard reviewed activation history.
	raw, err := os.ReadFile(filepath.Join("..", "..", "..", "migrations", "041_semantic_memory_indexes.down.sql"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.pool.Exec(ctx, string(raw)); err == nil {
		t.Fatal("downgrade discarded retained generations")
	}
}
func TestSemanticIndexResumeReusesPersistedMatchingEntries(t *testing.T) {
	ctx, s, cs := database(t)
	scope := Scope{"user", "semantic:resume"}
	for _, title := range []string{"один", "два", "три"} {
		if _, err := cs.Save(ctx, chunks.Chunk{ScopeType: scope.Type, ScopeID: scope.ID, Title: title, Content: "описание " + title}); err != nil {
			t.Fatal(err)
		}
	}
	g, err := s.Create(ctx, testSpace(768), scope)
	if err != nil {
		t.Fatal(err)
	}
	sources, _ := s.Sources(ctx, scope, 10)
	if err := s.Put(ctx, g.ID, sources[0], unitVector(768)); err != nil {
		t.Fatal(err)
	}
	f := &fakeModel{space: g.Space}
	a := &Adapter{Backend: f, Space: g.Space, Admission: &testAdmission{}}
	if _, err := Rebuild(ctx, s, a, g.ID, 10); err != nil {
		t.Fatal(err)
	}
	if f.calls != 1 {
		t.Fatal("resume count", f.calls)
	}
	// A new caller instance after restart sees all persisted entries and needs no inference.
	restarted := NewStore(s.pool)
	restarted.Guard = s.Guard
	if _, err := Rebuild(ctx, restarted, a, g.ID, 10); err != nil || f.calls != 1 {
		t.Fatal("restart replay reran complete sources", err)
	}
	proof := qualityProof()
	raw, _ := json.Marshal(proof)
	if !json.Valid(raw) {
		t.Fatal("proof")
	}
	if err := s.Verify(ctx, g.ID, proof); err != nil {
		t.Fatal(err)
	}
}
func TestSourceAccessGuardMissingOrDeniedBlocksIndexing(t *testing.T) {
	ctx, s, cs := database(t)
	scope := Scope{"session", "semantic:guard"}
	if _, err := cs.Save(ctx, chunks.Chunk{ScopeType: scope.Type, ScopeID: scope.ID, Title: "source", Content: "fixture", ProvenanceJSON: `{"catalog":{"source_id":"claimed-but-untrusted","revision":7,"access_revision":9}}`}); err != nil {
		t.Fatal(err)
	}
	g, err := s.Create(ctx, testSpace(768), scope)
	if err != nil {
		t.Fatal(err)
	}
	f := &fakeModel{space: g.Space}
	a := &Adapter{Backend: f, Space: g.Space, Admission: &testAdmission{}}
	for _, guard := range []SourceGuard{nil, labSourceGuard{deny: true}} {
		s.Guard = guard
		if _, err := Rebuild(ctx, s, a, g.ID, 10); err != ErrAccess || f.calls != 0 {
			t.Fatal("missing authority inferred from provenance", err)
		}
	}
}
func TestCitationChangeReusesVectorWithoutNewInference(t *testing.T) {
	ctx, s, cs := database(t)
	scope := Scope{"session", "semantic:locator"}
	c, err := cs.Save(ctx, chunks.Chunk{ScopeType: scope.Type, ScopeID: scope.ID, Title: "stable title", Content: "stable text", SourceType: "document", SourceID: "old/path"})
	if err != nil {
		t.Fatal(err)
	}
	f := &fakeModel{space: testSpace(768)}
	a := &Adapter{Backend: f, Space: f.space, Admission: &testAdmission{}}
	g, err := s.Create(ctx, a.Space, scope)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Rebuild(ctx, s, a, g.ID, 10); err != nil {
		t.Fatal(err)
	}
	if f.calls != 1 {
		t.Fatal("initial inference count")
	}
	if _, err := s.pool.Exec(ctx, `UPDATE memory_chunks SET source_id='new/path',updated_at=clock_timestamp() WHERE id=$1`, c.ID); err != nil {
		t.Fatal(err)
	}
	newGeneration, err := s.Create(ctx, a.Space, scope)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Rebuild(ctx, s, a, newGeneration.ID, 10); err != nil || f.calls != 1 {
		t.Fatal("citation-only change re-inferred unchanged text", err)
	}
	if err := s.Verify(ctx, newGeneration.ID, qualityProof()); err != nil {
		t.Fatal(err)
	}
	newGeneration, _ = s.Generation(ctx, newGeneration.ID)
	hits, err := s.Search(ctx, newGeneration, unitVector(768), 1)
	if err != nil || len(hits) != 1 || hits[0].Source.SourceID != "new/path" {
		t.Fatal("reused vector kept old citation", err)
	}
}
func TestSecretSourceExcludedFromIndexAndModelAdmission(t *testing.T) {
	ctx, s, cs := database(t)
	scope := Scope{"session", "semantic:secret"}
	if _, err := cs.Save(ctx, chunks.Chunk{ScopeType: scope.Type, ScopeID: scope.ID, Title: "Secret record", Content: "synthetic secret fixture", ProvenanceJSON: `{"kind":"memory_derived","source_refs":["message:original"],"sensitivity":"secret"}`}); err != nil {
		t.Fatal(err)
	}
	if _, err := cs.Save(ctx, chunks.Chunk{ScopeType: scope.Type, ScopeID: scope.ID, Title: "Private record", Content: "ordinary private fixture"}); err != nil {
		t.Fatal(err)
	}
	g, err := s.Create(ctx, testSpace(768), scope)
	if err != nil {
		t.Fatal(err)
	}
	f := &fakeModel{space: g.Space}
	a := &Adapter{Backend: f, Space: g.Space, Admission: &testAdmission{}}
	if _, err := Rebuild(ctx, s, a, g.ID, 10); err != nil {
		t.Fatal(err)
	}
	var count int
	if err := s.pool.QueryRow(ctx, `SELECT count(*) FROM semantic_index_entries WHERE generation_id=$1`, g.ID).Scan(&count); err != nil || count != 1 || f.calls != 1 {
		t.Fatal("secret indexed", err, count, f.calls)
	}
	if err := s.Verify(ctx, g.ID, qualityProof()); err != nil {
		t.Fatal(err)
	}
	g, _ = s.Generation(ctx, g.ID)
	hits, err := s.Search(ctx, g, unitVector(768), 4)
	if err != nil || len(hits) != 1 || hits[0].Source.Title != "Private record" {
		t.Fatal("secret source retrieved", err)
	}
}
func TestSourceCorrectionCommitsBeforeWaitingVectorPersistence(t *testing.T) {
	ctx, s, cs := database(t)
	scope := Scope{"session", "semantic:correction"}
	c, err := cs.Save(ctx, chunks.Chunk{ScopeType: scope.Type, ScopeID: scope.ID, Title: "Правило владельца", Content: "Разрешена отправка"})
	if err != nil {
		t.Fatal(err)
	}
	sources, _ := s.Sources(ctx, scope, 10)
	g, err := s.Create(ctx, testSpace(768), scope)
	if err != nil {
		t.Fatal(err)
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, `SELECT id FROM memory_chunks WHERE id=$1 FOR UPDATE`, c.ID); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- s.Put(ctx, g.ID, sources[0], unitVector(768)) }()
	deadline := time.Now().Add(3 * time.Second)
	waiting := false
	for time.Now().Before(deadline) {
		if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM pg_locks WHERE pid<>pg_backend_pid() AND NOT granted AND locktype='transactionid')`).Scan(&waiting); err != nil {
			t.Fatal(err)
		}
		if waiting {
			break
		}
		time.Sleep(time.Millisecond)
	}
	if !waiting {
		t.Fatal("vector persistence did not reach source lock barrier")
	}
	if _, err := tx.Exec(ctx, `UPDATE memory_chunks SET content='Только подготовка без отправки',updated_at=clock_timestamp() WHERE id=$1`, c.ID); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	if err := <-done; err != ErrStale {
		t.Fatal("correction lost to stale vector", err)
	}
	var count int
	if err := s.pool.QueryRow(ctx, `SELECT count(*) FROM semantic_index_entries WHERE generation_id=$1`, g.ID).Scan(&count); err != nil || count != 0 {
		t.Fatal("stale vector persisted", err)
	}
}
