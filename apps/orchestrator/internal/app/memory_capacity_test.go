package app

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/butler/butler/internal/memory/catalog"
	"github.com/butler/butler/internal/memory/memorytest"
	"github.com/butler/butler/internal/memory/semantic"
	"github.com/butler/butler/internal/owneridentity"
	"github.com/butler/butler/internal/security/content"
	"github.com/jackc/pgx/v5"
)

// Opt-in synthetic capacity measurements. No credentials or inference backend.
// This measures actual Catalog and current-source-guarded Store.Search, not
// native model latency or full ordinary user task acceptance.
func TestMemoryCapacityScreen(t *testing.T) {
	if os.Getenv("BUTLER_MEMORY_CAPACITY_SCREEN") != "1" {
		t.Skip("explicit isolated capacity screen required")
	}
	root := os.Getenv("BUTLER_MEMORY_CAPACITY_REPORT")
	if !filepath.IsAbs(root) {
		t.Fatal("absolute report directory required")
	}
	for _, p := range []struct{ N, Scopes int }{{10000, 1}, {100000, 1}, {100000, 64}} {
		t.Run(fmt.Sprintf("chunks%d_scopes%d", p.N, p.Scopes), func(t *testing.T) { runMemoryCapacity(t, root, p.N, p.Scopes) })
	}
}

func capacityVector(seed int) []float32 {
	v := make([]float32, 768)
	var norm float64
	for j := range v {
		x := uint64(seed+91)*0x9e3779b97f4a7c15 + uint64(j+1)*0xbf58476d1ce4e5b9
		x ^= x >> 30
		x *= 0xbf58476d1ce4e5b9
		x ^= x >> 27
		v[j] = float32(int64(x%20001) - 10000)
		norm += float64(v[j]) * float64(v[j])
	}
	for j := range v {
		v[j] /= float32(math.Sqrt(norm))
	}
	return v
}
func capacityVectorLiteral(v []float32) string {
	var s strings.Builder
	s.WriteByte('[')
	for i, x := range v {
		if i > 0 {
			s.WriteByte(',')
		}
		s.WriteString(strconv.FormatFloat(float64(x), 'g', -1, 32))
	}
	s.WriteByte(']')
	return s.String()
}

func runMemoryCapacity(t *testing.T, root string, n, scopes int) {
	t.Helper()
	pool := memorytest.Database(t)
	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Minute)
	defer cancel()
	owner := owneridentity.WithActor(ctx, owneridentity.Actor{OwnerID: "capacity-owner", Channel: "web", DeliveryID: "synthetic-capacity"})
	dir := filepath.Join(root, fmt.Sprintf("chunks%d_scopes%d", n, scopes))
	if os.Mkdir(dir, 0700) != nil {
		t.Fatal("fresh report required")
	}
	write := func(name string, v any) {
		raw, e := json.MarshalIndent(v, "", "  ")
		if e != nil || os.WriteFile(filepath.Join(dir, name), append(raw, '\n'), 0644) != nil {
			t.Fatal("capacity report unavailable")
		}
	}
	manifest := catalog.NewFileManifest(filepath.Join(dir, "anchor"))
	if manifest.Initialize() != nil {
		t.Fatal("anchor unavailable")
	}
	cat := catalog.New(pool, manifest)
	store := semantic.NewStore(pool)
	guard, e := semantic.NewCatalogGuard(cat, "memory_read")
	if e != nil {
		t.Fatal(e)
	}
	store.Guard = guard
	space := semantic.Space{ProfileID: "capacity-synthetic-768-v1", Model: "google/embeddinggemma-2", Revision: strings.Repeat("9", 40), Preprocessing: "embeddinggemma2-retrieval-v1", RuntimeVersion: "synthetic-no-inference-v1", ChunkVersion: "synthetic91-v1", Dimensions: 768}
	generations := make([]semantic.Generation, scopes)
	sources := []catalog.Source{}
	corpus := sha256.New()
	started := time.Now()
	for start := 0; start < n; start += 1000 {
		count := min(1000, n-start)
		scope := (start / 1000) % scopes
		// For the64-scope screen, all declared scopes receive rows even when the
		// final source has fewer fragments; split sources into at most1000 rows.
		if scopes > 1 {
			scope = (start / 1000) % scopes
		}
		fragments := make([]catalog.Fragment, count)
		var body strings.Builder
		for j := range fragments {
			text := fmt.Sprintf("Synthetic seed91 record%d. Context group%d. Observation only.", start+j, scope)
			fragments[j] = catalog.Fragment{Key: fmt.Sprintf("row:%d", start+j), Title: fmt.Sprintf("Record%d", start+j), Text: text, Locator: json.RawMessage(`{}`)}
			body.WriteString(text)
			body.WriteByte('\n')
			_, _ = corpus.Write([]byte(text + "\n"))
		}
		src, e := cat.Put(owner, catalog.PutSource{ExternalKey: fmt.Sprintf("capacity:%d", start), Kind: "document", ScopeType: "project", ScopeID: fmt.Sprintf("capacity-scope%d", scope), Title: "Synthetic capacity source", Body: body.String(), Origin: content.Origin{Kind: content.Document, Sensitivity: content.Public}})
		if e != nil {
			t.Fatal("catalog seed", e)
		}
		if _, e = cat.PublishFragments(owner, catalog.SourceRef{ID: src.ID, Revision: src.Revision, AccessRevision: src.AccessRevision, Hash: src.Hash}, fragments); e != nil {
			t.Fatal("catalog fragments", e)
		}
		sources = append(sources, src)
	}
	ingestMS := time.Since(started).Milliseconds()
	started = time.Now()
	// Bulk vectors avoid100000 per-row inference calls. The ordinary exact
	// Verify/Activate routines still check every real current chunk and parent.
	for scope := 0; scope < scopes; scope++ {
		g, e := store.Create(owner, space, semantic.Scope{Type: "project", ID: fmt.Sprintf("capacity-scope%d", scope)})
		if e != nil {
			t.Fatal(e)
		}
		// Page fixture rows so the client retains at most1000 vectors. COPY's
		// text column is explicitly cast on insertion into pgvector. The temporary
		// table and COPY belong to one acquired PostgreSQL session.
		conn, e := pool.Acquire(ctx)
		if e != nil {
			t.Fatal(e)
		}
		if _, e = conn.Exec(ctx, `CREATE TEMP TABLE capacity_vectors(generation_id TEXT,chunk_id BIGINT,dimensions INTEGER,source_revision TIMESTAMPTZ,source_hash TEXT,embedding_hash TEXT,embedding TEXT)`); e != nil {
			conn.Release()
			t.Fatal(e)
		}
		var lastID int64
		counter := 0
		for {
			rows, e := pool.Query(ctx, `SELECT id,scope_type,scope_id,title,content,summary,source_type,source_id,provenance::text,updated_at FROM memory_chunks WHERE scope_type=$1 AND scope_id=$2 AND id>$3 ORDER BY id LIMIT 1000`, g.Scope.Type, g.Scope.ID, lastID)
			if e != nil {
				conn.Release()
				t.Fatal(e)
			}
			batch := [][]any{}
			for rows.Next() {
				var s semantic.Source
				if e = rows.Scan(&s.ID, &s.Scope.Type, &s.Scope.ID, &s.Title, &s.Content, &s.Summary, &s.SourceType, &s.SourceID, &s.ProvenanceJSON, &s.Revision); e != nil {
					rows.Close()
					conn.Release()
					t.Fatal(e)
				}
				v := capacityVector(int(s.ID))
				batch = append(batch, []any{g.ID, s.ID, 768, s.Revision, s.Hash(), s.EmbeddingHash(), capacityVectorLiteral(v)})
				lastID = s.ID
				counter++
			}
			e = rows.Err()
			rows.Close()
			if e != nil {
				conn.Release()
				t.Fatal(e)
			}
			if len(batch) == 0 {
				break
			}
			_, e = conn.Conn().CopyFrom(ctx, pgx.Identifier{"capacity_vectors"}, []string{"generation_id", "chunk_id", "dimensions", "source_revision", "source_hash", "embedding_hash", "embedding"}, pgx.CopyFromRows(batch))
			if e == nil {
				_, e = conn.Exec(ctx, `INSERT INTO semantic_index_entries SELECT generation_id,chunk_id,dimensions,source_revision,source_hash,embedding_hash,embedding::vector,clock_timestamp() FROM capacity_vectors;TRUNCATE capacity_vectors`)
			}
			if e != nil {
				conn.Release()
				t.Fatal("vector fixture", e)
			}
		}
		if _, e = conn.Exec(ctx, `DROP TABLE capacity_vectors`); e != nil {
			conn.Release()
			t.Fatal(e)
		}
		conn.Release()
		if counter == 0 {
			t.Fatal("declared scope empty")
		}
		generations[scope] = g
	}
	vectorMS := time.Since(started).Milliseconds()
	started = time.Now()
	proof := semantic.Proof{ArtifactRef: "fixture://synthetic-capacity-not-quality", ArtifactSHA256: strings.Repeat("1", 64), CorpusVersion: "synthetic91-v1", Cases: 24, BaselineTop1: 0.1, CandidateTop1: 0.2}
	for i, g := range generations {
		if e = store.Verify(owner, g.ID, proof); e != nil {
			t.Fatal("fixture coverage verify", e)
		}
		if e = store.Activate(owner, g.ID, ""); e != nil {
			t.Fatal("fixture activation", e)
		}
		generations[i], e = store.Active(owner, g.Scope)
		if e != nil {
			t.Fatal(e)
		}
	}
	verifyMS := time.Since(started).Milliseconds()
	write("preparation.json", map[string]any{"chunks": n, "scopes": scopes, "source_sha256": hex.EncodeToString(corpus.Sum(nil)), "catalog_ingest_ms": ingestMS, "synthetic_vector_load_ms": vectorMS, "verify_activate_ms": verifyMS})
	type sample struct {
		Index   int     `json:"index"`
		Millis  float64 `json:"millis"`
		Failed  bool    `json:"failed"`
		Hits    int     `json:"hits"`
		Reason  string  `json:"reason,omitempty"`
		StartNS int64   `json:"start_ns"`
		EndNS   int64   `json:"end_ns"`
	}
	measure := func(count, parallel int, firstRead chan<- struct{}) []sample {
		out := make([]sample, count)
		var wg sync.WaitGroup
		for worker := 0; worker < parallel; worker++ {
			wg.Add(1)
			go func(worker int) {
				defer wg.Done()
				for j := worker; j < count; j += parallel {
					g := generations[j%len(generations)]
					q, stop := context.WithTimeout(owner, 2*time.Second)
					start := time.Now()
					if j == 0 && firstRead != nil {
						firstRead <- struct{}{}
					}
					hits, err := store.Search(q, g, capacityVector(91+j), 4)
					reason := ""
					switch {
					case errors.Is(err, context.DeadlineExceeded):
						reason = "deadline"
					case errors.Is(err, catalog.ErrStale):
						reason = "stale"
					case errors.Is(err, catalog.ErrDenied):
						reason = "denied"
					case err != nil:
						reason = "unexpected_backend_failure"
					}
					out[j] = sample{Index: j, Millis: float64(time.Since(start).Microseconds()) / 1000, Failed: err != nil, Hits: len(hits), Reason: reason, StartNS: start.UnixNano(), EndNS: time.Now().UnixNano()}
					stop()
				}
			}(worker)
		}
		wg.Wait()
		return out
	}
	first := measure(1, 1, nil)
	warmup := measure(20, 1, nil)
	warm := measure(100, 2, nil)
	var explain json.RawMessage
	g := generations[0]
	if err := pool.QueryRow(ctx, `EXPLAIN(ANALYZE,BUFFERS,FORMAT JSON) SELECT c.id,c.scope_type,c.scope_id,c.title,c.content,c.summary,c.source_type,c.source_id,c.provenance::text,c.updated_at,e.source_hash,e.embedding <=> $2::vector FROM semantic_index_entries e JOIN memory_chunks c ON c.id=e.chunk_id WHERE e.generation_id=$1 AND e.dimensions=$3 AND c.scope_type=$4 AND c.scope_id=$5 AND c.updated_at=e.source_revision AND c.status='active' AND effective_status='active' AND NOT suppressed AND confirmation_state IN ('confirmed','auto_confirmed') AND (expires_at IS NULL OR expires_at>clock_timestamp()) ORDER BY e.embedding <=> $2::vector,c.id LIMIT $6`, g.ID, capacityVectorLiteral(capacityVector(91)), 768, g.Scope.Type, g.Scope.ID, 16).Scan(&explain); err != nil {
		t.Fatal("exact query plan", err)
	}
	write("exact-plan.json", explain)
	// Exact controls use the nearest vector of known live originals. A generic
	// empty result or a database error cannot substitute for authorization proof.
	controlIDs := make([]int64, 5)
	for i, src := range append(append([]catalog.Source{}, sources[:4]...), sources[len(sources)-1]) {
		if err := pool.QueryRow(ctx, `SELECT min(id) FROM memory_chunks WHERE source_id=$1`, src.ID).Scan(&controlIDs[i]); err != nil {
			t.Fatal(err)
		}
	}
	q := capacityVector(int(controlIDs[4]))
	healthyScope := semantic.Scope{Type: sources[len(sources)-1].ScopeType, ID: sources[len(sources)-1].ScopeID}
	healthy, e := store.Active(owner, healthyScope)
	if e != nil {
		t.Fatal(e)
	}
	hits, e := store.Search(owner, healthy, q, 1)
	if e != nil || len(hits) != 1 || hits[0].Source.ID != controlIDs[4] {
		t.Fatal("owner-positive query control failed", e)
	}
	foreign := owneridentity.WithActor(ctx, owneridentity.Actor{OwnerID: "capacity-foreign", Channel: "web", DeliveryID: "denied-capacity"})
	if hits, err := store.Search(foreign, healthy, q, 1); !errors.Is(err, catalog.ErrDenied) || len(hits) != 0 {
		t.Fatal("foreign-deny control failed", err)
	}
	// Separate mixed catalog writers. Corrections/deletions may deliberately
	// produce an explicit stale result; they may never return withdrawn text.
	writerDone := make(chan error, 1)
	firstRead := make(chan struct{})
	type operation struct{ StartNS, EndNS int64 }
	var operations []operation
	operationStart := func() func() {
		start := time.Now().UnixNano()
		return func() { operations = append(operations, operation{start, time.Now().UnixNano()}) }
	}
	go func() {
		<-firstRead
		for i := 0; i < 2; i++ {
			src := sources[i%len(sources)]
			finish := operationStart()
			if _, err := cat.Delete(owner, src.ID, "delete", src.Revision); err != nil {
				writerDone <- err
				return
			}
			finish()
		}
		for i := 2; i < 4; i++ {
			src := sources[i]
			finish := operationStart()
			if _, err := cat.Put(owner, catalog.PutSource{ExternalKey: src.ExternalKey, ExpectedRevision: src.Revision, Kind: src.Kind, ScopeType: src.ScopeType, ScopeID: src.ScopeID, Title: src.Title, Body: src.Body + "\nCorrection: this source is withdrawn from the old generation.", Origin: src.Origin}); err != nil {
				writerDone <- err
				return
			}
			finish()
		}
		finish := operationStart()
		fresh, err := cat.Put(owner, catalog.PutSource{ExternalKey: "mixed-ingestion", Kind: "document", ScopeType: "project", ScopeID: "capacity-scope0", Title: "New mixed source", Body: "Synthetic new observation.", Origin: content.Origin{Kind: content.Document, Sensitivity: content.Public}})
		if err != nil {
			writerDone <- err
			return
		}
		_, err = cat.PublishFragments(owner, catalog.SourceRef{ID: fresh.ID, Revision: fresh.Revision, AccessRevision: fresh.AccessRevision, Hash: fresh.Hash}, []catalog.Fragment{{Key: "mixed:1", Text: fresh.Body, Locator: json.RawMessage(`{}`)}})
		if err != nil {
			writerDone <- err
			return
		}
		finish()
		writerDone <- nil
	}()
	mixed := measure(100, 2, firstRead)
	if err := <-writerDone; err != nil {
		t.Fatal("mixed catalog mutation", err)
	}
	for _, id := range controlIDs[:4] {
		if _, err := cat.Citation(owner, id); !errors.Is(err, catalog.ErrDenied) && !errors.Is(err, catalog.ErrStale) && !errors.Is(err, catalog.ErrDeleted) {
			t.Fatal("exact withdrawn citation was not denied", err)
		}
	}
	hits, e = store.Search(owner, healthy, q, 1)
	if e != nil || len(hits) != 1 || hits[0].Source.ID != controlIDs[4] {
		t.Fatal("healthy control lost after mutations", e)
	}
	overlap := 0
	for _, r := range mixed {
		for _, op := range operations {
			if r.StartNS < op.EndNS && r.EndNS > op.StartNS {
				overlap++
				break
			}
		}
	}
	write("writer-operations.json", map[string]any{"operations": operations, "overlapping_reads": overlap})
	p95 := func(v []sample) float64 {
		a := make([]float64, len(v))
		for i, s := range v {
			a[i] = s.Millis
		}
		sort.Float64s(a)
		return a[(len(a)*95+99)/100-1]
	}
	var bytes int64
	if e = pool.QueryRow(ctx, `SELECT pg_database_size(current_database())`).Scan(&bytes); e != nil {
		t.Fatal(e)
	}
	countFailures := func(v []sample) int {
		n := 0
		for _, s := range v {
			if s.Failed || s.Hits == 0 {
				n++
			}
		}
		return n
	}
	var relations json.RawMessage
	if err := pool.QueryRow(ctx, `SELECT jsonb_agg(jsonb_build_object('relation',c.relname,'total_bytes',pg_total_relation_size(c.oid))) FROM pg_class c JOIN pg_namespace ns ON ns.oid=c.relnamespace WHERE ns.nspname=current_schema() AND c.relkind='r'`).Scan(&relations); err != nil {
		t.Fatal(err)
	}
	write("relation-bytes.json", relations)
	write("results.json", map[string]any{"chunks": n, "scopes": scopes, "seed": 91, "source_sha256": hex.EncodeToString(corpus.Sum(nil)), "catalog_ingest_ms": ingestMS, "synthetic_vector_load_ms": vectorMS, "verify_activate_ms": verifyMS, "database_bytes_shared_fixture_db": bytes, "first_query": first, "warmup": warmup, "warm": warm, "mixed_catalog": mixed, "warm_p95_ms": p95(warm), "mixed_p95_ms": p95(mixed), "target_warm_p95_ms": 300, "target_profile": n == 10000 && scopes == 1, "store_warm_slo_pass": p95(warm) <= 300 && countFailures(warm) == 0, "warm_fail_or_empty": countFailures(warm), "collection_complete": true, "synthetic_quality_proof": true, "native_calls": 0, "model_queue": "not_measured", "cold_os_cache": "unknown", "mixed_independent_workers": "pending_composed_gate"})
	t.Logf("capacity n=%d scopes=%d warm p95=%.3fms mixed p95=%.3fms", n, scopes, p95(warm), p95(mixed))
	if n == 10000 && scopes == 1 {
		if p95(warm) > 300 || countFailures(warm) != 0 {
			t.Error("declared warm Store SLO failed")
		}
		if p95(mixed) > 300 || overlap < 10 {
			t.Error("declared mixed Store SLO or minimum overlap failed")
		}
		for _, r := range append(append([]sample{}, warm...), mixed...) {
			if r.Millis > 2100 || r.Reason == "unexpected_backend_failure" || r.Reason == "deadline" {
				t.Error("explicit2s deadline or expected mixed limitation violated")
				break
			}
		}
	}
}
