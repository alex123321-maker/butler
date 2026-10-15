package catalog

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/butler/butler/internal/owneridentity"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type observerSQLTrace struct {
	mu                     sync.Mutex
	commits, rolls, writes int
	delayCommit            bool
}

func (p *observerSQLTrace) TraceQueryStart(ctx context.Context, _ *pgx.Conn, d pgx.TraceQueryStartData) context.Context {
	if ctx.Value(runSourceProbeKey{}) == nil {
		return ctx
	}
	query := strings.ToUpper(strings.TrimSpace(d.SQL))
	p.mu.Lock()
	if query == "COMMIT" {
		p.commits++
	} else if query == "ROLLBACK" {
		p.rolls++
	} else if query != "BEGIN" && !strings.HasPrefix(query, "SELECT ") && !strings.HasPrefix(query, "WITH ") {
		p.writes++
	}
	delay := query == "COMMIT" && p.delayCommit
	p.mu.Unlock()
	if delay {
		// A deterministic client-side delay is a counterfactual commit control,
		// not a claim to reproduce a historical PostgreSQL disk/WAL fault.
		timer := time.NewTimer(300 * time.Millisecond)
		defer timer.Stop()
		select {
		case <-timer.C:
		case <-ctx.Done():
		}
	}
	return ctx
}
func (*observerSQLTrace) TraceQueryEnd(context.Context, *pgx.Conn, pgx.TraceQueryEndData) {}

func TestRunSourceObserverRollbackHasNoBusinessMutationIntegration(t *testing.T) {
	s, owner := fixture(t)
	live := seedRun(t, s)
	if _, err := s.pool.Exec(owner, `INSERT INTO tasks(task_id,session_key,owner_id,goal)VALUES('task','web:owner','owner','read selected DATA');UPDATE runs SET task_id='task',task_revision=1 WHERE run_id='run'`); err != nil {
		t.Fatal(err)
	}
	c, err := s.Decide(owner, Decision{ClaimID: propose(t, s, owner, put(t, s, owner, "rule", "Сохранять неизвестность"), `"ru"`).ID})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.SetTemporal(owner, TemporalUpdate{ClaimID: c.ID, ExpectedHeadRevision: 1, Validity: ValidityUnknown, Class: FreshnessStablePreference}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.ReadAcceptedFacts(live, []string{c.ID}, TemporalQuery{}); err != nil {
		t.Fatal(err)
	}
	trace := &observerSQLTrace{delayCommit: true}
	cfg := s.pool.Config()
	cfg.ConnConfig.Tracer = trace
	pool, err := pgxpool.NewWithConfig(live, cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	observed := New(pool, s.manifest)
	live = WithRunSourceObserver(live, func(RunSourceProbe) {})
	state := func() string {
		t.Helper()
		var digest string
		err := s.pool.QueryRow(owner, `SELECT md5(jsonb_build_object('sources',(SELECT jsonb_agg(to_jsonb(x)ORDER BY source_id)FROM memory_sources x),'claims',(SELECT jsonb_agg(to_jsonb(x)ORDER BY claim_id)FROM memory_claims x),'heads',(SELECT jsonb_agg(to_jsonb(x)ORDER BY owner_id,scope_type,scope_id,fact_key)FROM memory_fact_heads x),'metadata',(SELECT jsonb_agg(to_jsonb(x)ORDER BY claim_id,revision)FROM memory_claim_temporal_revisions x),'run_sources',(SELECT jsonb_agg(to_jsonb(x)ORDER BY run_id,source_id)FROM memory_run_sources x),'run_pins',(SELECT jsonb_agg(to_jsonb(x)ORDER BY run_id,claim_id,query_key)FROM memory_temporal_run_pins x),'tasks',(SELECT jsonb_agg(to_jsonb(x)ORDER BY task_id)FROM tasks x),'runs',(SELECT jsonb_agg(to_jsonb(x)ORDER BY run_id)FROM runs x),'budget',(SELECT jsonb_agg(to_jsonb(x)ORDER BY reservation_id)FROM execution_budget_reservations x),'deletions',(SELECT jsonb_agg(to_jsonb(x)ORDER BY deletion_id)FROM memory_deletions x))::text)`).Scan(&digest)
		if err != nil {
			t.Fatal(err)
		}
		return digest
	}
	before := state()
	for range 20 {
		bounded, stop := context.WithTimeout(live, 200*time.Millisecond)
		err := observed.CheckRunSources(bounded, "run")
		stop()
		if err != nil {
			t.Fatal("read-only current observer failed", err)
		}
	}
	if state() != before || trace.commits != 0 || trace.rolls != 20 || trace.writes != 0 {
		t.Fatal("observation committed, mutated business state or skipped rollback", trace.commits, trace.rolls, trace.writes)
	}
	// The same injected COMMIT delay still exhausts200ms in the preserved
	// compound-owner path, proving the negative control is actually armed.
	compound := owneridentity.WithActor(live, owneridentity.Actor{OwnerID: "owner", Channel: "web", DeliveryID: "commit-control"})
	bounded, stop := context.WithTimeout(compound, 200*time.Millisecond)
	err = observed.CheckRunSources(bounded, "run")
	stop()
	if err == nil || trace.commits != 1 {
		t.Fatal("commit delay negative control was not exercised", err, trace.commits)
	}
	// Current metadata changes still invalidate the next proof and SQL final
	// publication, even though no observer history needs a durable COMMIT.
	if _, err = s.SetTemporal(owner, TemporalUpdate{ClaimID: c.ID, ExpectedHeadRevision: 1, ExpectedRevision: 1, Validity: ValidityUnknown, Class: FreshnessTechnicalFact}); err != nil {
		t.Fatal(err)
	}
	if err = observed.CheckRunSources(live, "run"); !errors.Is(err, ErrStale) {
		t.Fatal("metadata mutation escaped current observer", err)
	}
	if _, err = s.pool.Exec(owner, `INSERT INTO messages(message_id,session_key,run_id,role,content)VALUES('late','web:owner','run','assistant','obsolete')`); err == nil {
		t.Fatal("late publication escaped SQL temporal proof")
	}
}
