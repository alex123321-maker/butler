package indexing

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/butler/butler/internal/budget"
	pb "github.com/butler/butler/internal/gen/localmodels/v1"
	"github.com/butler/butler/internal/memory/catalog"
	"github.com/butler/butler/internal/memory/jobs"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// This trace retains only fixed SQL classes. It never copies SQL, parameters,
// source bodies or raw PostgreSQL/provider errors into diagnostic output.
type indexObserverTrace struct {
	armed                  atomic.Bool
	delayCommit            bool
	mu                     sync.Mutex
	proofs, commits, rolls int
	chunkReads             int
}

func (p *indexObserverTrace) TraceQueryStart(ctx context.Context, _ *pgx.Conn, d pgx.TraceQueryStartData) context.Context {
	deadline, bounded := ctx.Deadline()
	// The local source observers have a 200ms child deadline. The owning
	// admission, result and publication transactions have the larger call bound.
	if !p.armed.Load() || !bounded || time.Until(deadline) > 210*time.Millisecond {
		return ctx
	}
	query := strings.ToUpper(strings.TrimSpace(d.SQL))
	p.mu.Lock()
	switch query {
	case "BEGIN":
		p.proofs++
	case "COMMIT":
		p.commits++
	case "ROLLBACK":
		p.rolls++
	default:
		if strings.Contains(query, "FROM MEMORY_CHUNKS WHERE ID=") {
			p.chunkReads++
		}
	}
	delay := p.delayCommit && query == "COMMIT"
	p.mu.Unlock()
	if delay {
		// A deterministic client-side counterfactual, not a reproduction of
		// the historical e04 rejection or a claim about server WAL latency.
		timer := time.NewTimer(300 * time.Millisecond)
		defer timer.Stop()
		select {
		case <-ctx.Done():
		case <-timer.C:
		}
	}
	return ctx
}

func (*indexObserverTrace) TraceQueryEnd(context.Context, *pgx.Conn, pgx.TraceQueryEndData) {}

func (p *indexObserverTrace) counts() (int, int, int, int) {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.proofs, p.commits, p.rolls, p.chunkReads
}

type slowIndexModel struct {
	*model
	trace        *indexObserverTrace
	started      atomic.Int32
	cancelled    atomic.Int32
	hold         time.Duration
	whileActive  func()
	ignoreCancel bool
}

func (m *slowIndexModel) Infer(ctx context.Context, q *pb.InferRequest) (*pb.InferResponse, error) {
	m.started.Add(1)
	m.trace.armed.Store(true)
	if m.whileActive != nil {
		m.whileActive()
	}
	timer := time.NewTimer(m.hold)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		m.cancelled.Add(1)
		if !m.ignoreCancel {
			return nil, ctx.Err()
		}
		// Simulate an already-running backend returning a valid late result.
		// Adapter/current publication proof must still discard every vector.
		<-timer.C
		return m.model.Infer(ctx, q)
	case <-timer.C:
		return m.model.Infer(ctx, q)
	}
}

func tracedIndexFixture(t *testing.T, trace *indexObserverTrace) (*Runtime, context.Context, catalog.Source, *slowIndexModel) {
	t.Helper()
	r, owner, src, base := fixture(t)
	cfg := r.Catalog.Pool().Config()
	cfg.ConnConfig.Tracer = trace
	pool, err := pgxpool.NewWithConfig(context.Background(), cfg)
	if err != nil {
		t.Fatal("controlled tracing pool unavailable")
	}
	t.Cleanup(pool.Close)
	// The fixture has no deletions; a second initialized empty manifest observes
	// the same zero deletion watermark without modifying the primary manifest.
	manifest := catalog.NewFileManifest(filepath.Join(t.TempDir(), "observer-anchor"))
	if err = manifest.Initialize(); err != nil {
		t.Fatal("controlled observer manifest unavailable")
	}
	r.Catalog = catalog.New(pool, manifest)
	b := budget.NewController(pool, budget.DefaultPolicy())
	r.Jobs = jobs.New(pool, r.Catalog, b, &jobs.LocalTarget{Endpoint: r.Target.Endpoint(), Identity: r.Target.Identity()})
	m := &slowIndexModel{model: base, trace: trace, hold: 1200 * time.Millisecond}
	r.Backend = m
	return r, owner, src, m
}

func indexObserverState(t *testing.T, r *Runtime) (string, int, int) {
	t.Helper()
	var sourceDigest string
	var reserved, vectors int
	err := r.Catalog.Pool().QueryRow(context.Background(), `SELECT
 md5(jsonb_build_object(
 'sources',(SELECT jsonb_agg(to_jsonb(x) ORDER BY source_id) FROM memory_sources x),
 'revisions',(SELECT jsonb_agg(to_jsonb(x) ORDER BY source_id,revision) FROM memory_source_revisions x),
 'chunks',(SELECT jsonb_agg(to_jsonb(x) ORDER BY id) FROM memory_chunks x)
 )::text),
 (SELECT count(*) FROM execution_budget_reservations),
 (SELECT count(*) FROM semantic_index_entries)`).Scan(&sourceDigest, &reserved, &vectors)
	if err != nil {
		t.Fatal("controlled observer state unavailable")
	}
	return sourceDigest, reserved, vectors
}

func TestSlowLocalIndexRetainsBothCurrentnessObserversIntegration(t *testing.T) {
	trace := &indexObserverTrace{}
	r, owner, src, m := tracedIndexFixture(t, trace)
	before, _, _ := indexObserverState(t, r)
	if _, err := r.Enqueue(owner, request(src)); err != nil {
		t.Fatal("controlled index enqueue failed")
	}
	ctx, stop := context.WithTimeout(context.Background(), 15*time.Second)
	defer stop()
	worked, err := r.ProcessOne(ctx)
	if !worked || err != nil {
		t.Fatal("unchanged slow local index failed", worked)
	}
	proofs, commits, rolls, chunks := trace.counts()
	after, reserved, vectors := indexObserverState(t, r)
	if m.started.Load() != 2 || m.cancelled.Load() != 0 || proofs < 4 || chunks < 2 || before != after || reserved != 2 || vectors != 5 {
		t.Fatal("slow local observer coverage incomplete", m.started.Load(), m.cancelled.Load(), proofs, chunks, before == after, reserved, vectors)
	}
	t.Logf("controlled slow backend: calls=%d bounded_proofs=%d commits=%d rollbacks=%d chunk_checks=%d; sources unchanged; reservations=%d vectors=%d", m.started.Load(), proofs, commits, rolls, chunks, reserved, vectors)
}

func TestLocalIndexObserverCommitDelayCancelsWithoutRefundIntegration(t *testing.T) {
	trace := &indexObserverTrace{delayCommit: true}
	r, owner, src, m := tracedIndexFixture(t, trace)
	m.hold = 3 * time.Second
	before, _, _ := indexObserverState(t, r)
	if _, err := r.Enqueue(owner, request(src)); err != nil {
		t.Fatal("controlled index enqueue failed")
	}
	ctx, stop := context.WithTimeout(context.Background(), 10*time.Second)
	defer stop()
	worked, err := r.ProcessOne(ctx)
	proofs, commits, rolls, chunks := trace.counts()
	after, reserved, vectors := indexObserverState(t, r)
	if !worked || !errors.Is(err, context.Canceled) || commits == 0 || m.started.Load() != 1 || m.cancelled.Load() != 1 || before != after || reserved != 1 || vectors != 0 {
		t.Fatal("controlled commit fault did not retain denied attempt", worked, errors.Is(err, context.Canceled), commits, m.started.Load(), m.cancelled.Load(), before == after, reserved, vectors)
	}
	var status, code string
	if err = r.Catalog.Pool().QueryRow(context.Background(), `SELECT status,error_code FROM memory_aux_jobs WHERE job_id='index-job'`).Scan(&status, &code); err != nil || status != "failed" || code != "index_rejected" {
		t.Fatal("controlled observer failure changed fixed job receipt", status, code)
	}
	t.Logf("injected observer COMMIT300ms against unchanged200ms proof: bounded_proofs=%d commits=%d rollbacks=%d chunk_checks=%d; one attempt remains charged, no vectors; historical e04 cause remains unproven", proofs, commits, rolls, chunks)
}

func TestSlowLocalIndexCurrentnessCancellationRejectsLateBackendIntegration(t *testing.T) {
	for _, change := range []string{"source_revision", "source_revoke", "claim_takeover", "chunk_revision"} {
		t.Run(change, func(t *testing.T) {
			trace := &indexObserverTrace{}
			r, owner, src, m := tracedIndexFixture(t, trace)
			m.ignoreCancel = true
			if _, err := r.Enqueue(owner, request(src)); err != nil {
				t.Fatal("controlled index enqueue failed")
			}
			m.whileActive = func() {
				var err error
				switch change {
				case "source_revision":
					_, err = r.Catalog.Correct(owner, catalog.Correction{SourceID: src.ID, ExpectedRevision: src.Revision, Body: "Owner correction while local inference is active"})
				case "source_revoke":
					_, err = r.Catalog.Delete(owner, src.ID, "revoke", src.Revision)
				case "claim_takeover":
					_, err = r.Catalog.Pool().Exec(owner, `UPDATE memory_aux_jobs SET claim_epoch=claim_epoch+1 WHERE job_id='index-job'`)
				case "chunk_revision":
					_, err = r.Catalog.Pool().Exec(owner, `UPDATE memory_chunks SET content=content||' changed',updated_at=clock_timestamp()`)
				}
				if err != nil {
					t.Error("controlled currentness mutation unavailable")
				}
			}
			ctx, stop := context.WithTimeout(context.Background(), 10*time.Second)
			defer stop()
			worked, err := r.ProcessOne(ctx)
			_, reserved, vectors := indexObserverState(t, r)
			if !worked || !errors.Is(err, context.Canceled) || m.started.Load() != 1 || m.cancelled.Load() != 1 || m.calls.Load() != 1 || reserved != 1 || vectors != 0 {
				t.Fatal("currentness mutation accepted late vectors or refunded local attempt", worked, errors.Is(err, context.Canceled), m.started.Load(), m.cancelled.Load(), m.calls.Load(), reserved, vectors)
			}
			var active int
			if err = r.Catalog.Pool().QueryRow(context.Background(), `SELECT count(*) FROM semantic_index_active`).Scan(&active); err != nil || active != 0 {
				t.Fatal("currentness failure activated unreviewed generation")
			}
		})
	}
}
