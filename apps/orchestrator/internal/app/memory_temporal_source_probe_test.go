package app

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/butler/butler/internal/memory/catalog"
	"github.com/butler/butler/internal/ownership"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type temporalQueryStart struct {
	at          time.Time
	shape, hash string
}
type temporalQueryKey struct{}
type temporalAcquireKey struct{}
type temporalConnectKey struct{}
type temporalQueryObservation struct {
	Shape      string `json:"shape"`
	Hash       string `json:"statement_sha256,omitempty"`
	Millis     int64  `json:"millis"`
	Diagnostic string `json:"diagnostic,omitempty"`
	Acquired   int32  `json:"pool_acquired"`
	Idle       int32  `json:"pool_idle"`
	Total      int32  `json:"pool_total"`
}
type temporalSourceTrace struct {
	mu     sync.Mutex
	pool   *pgxpool.Pool
	counts map[string]int
	slow   []temporalQueryObservation
	phases map[string]int
	probes []catalog.RunSourceProbe
	waits  []json.RawMessage
}

func (t *temporalSourceTrace) probe(p catalog.RunSourceProbe) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.phases == nil {
		t.phases = map[string]int{}
	}
	t.phases[p.Phase]++
	if (p.Millis >= 20 || p.ErrorKind != "") && len(t.probes) < 2048 {
		t.probes = append(t.probes, p)
	}
}
func (t *temporalSourceTrace) TraceQueryStart(ctx context.Context, _ *pgx.Conn, d pgx.TraceQueryStartData) context.Context {
	shape := "other"
	for _, name := range []string{"memory_temporal", "memory_summary", "memory_run_sources", "memory_restore", "memory_deletions", "memory_sources", "session_ownership", "sessions", "tasks", "runs", "system_settings", "commit", "rollback"} {
		if strings.Contains(strings.ToLower(d.SQL), name) {
			shape = name
			break
		}
	}
	h := sha256.Sum256([]byte(d.SQL))
	return context.WithValue(ctx, temporalQueryKey{}, temporalQueryStart{time.Now(), shape, hex.EncodeToString(h[:])})
}
func (t *temporalSourceTrace) TraceQueryEnd(ctx context.Context, _ *pgx.Conn, d pgx.TraceQueryEndData) {
	if s, ok := ctx.Value(temporalQueryKey{}).(temporalQueryStart); ok {
		t.record(s, d.Err)
	}
}
func (t *temporalSourceTrace) TraceAcquireStart(ctx context.Context, _ *pgxpool.Pool, _ pgxpool.TraceAcquireStartData) context.Context {
	return context.WithValue(ctx, temporalAcquireKey{}, time.Now())
}
func (t *temporalSourceTrace) TraceAcquireEnd(ctx context.Context, _ *pgxpool.Pool, d pgxpool.TraceAcquireEndData) {
	if at, ok := ctx.Value(temporalAcquireKey{}).(time.Time); ok {
		t.record(temporalQueryStart{at: at, shape: "pool_acquire"}, d.Err)
	}
}
func (t *temporalSourceTrace) TraceConnectStart(ctx context.Context, _ pgx.TraceConnectStartData) context.Context {
	return context.WithValue(ctx, temporalConnectKey{}, time.Now())
}
func (t *temporalSourceTrace) TraceConnectEnd(ctx context.Context, d pgx.TraceConnectEndData) {
	if at, ok := ctx.Value(temporalConnectKey{}).(time.Time); ok {
		t.record(temporalQueryStart{at: at, shape: "pool_connect"}, d.Err)
	}
}
func (t *temporalSourceTrace) record(s temporalQueryStart, err error) {
	o := temporalQueryObservation{Shape: s.shape, Hash: s.hash, Millis: time.Since(s.at).Milliseconds()}
	if err != nil {
		o.Diagnostic = "storage_error"
		if errors.Is(err, context.DeadlineExceeded) {
			o.Diagnostic = "deadline"
		} else if errors.Is(err, context.Canceled) {
			o.Diagnostic = "cancelled"
		} else {
			var state interface{ SQLState() string }
			if errors.As(err, &state) && len(state.SQLState()) == 5 {
				o.Diagnostic = "pg_" + state.SQLState()
			}
		}
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.counts == nil {
		t.counts = map[string]int{}
	}
	t.counts[s.shape]++
	if (o.Millis >= 20 || err != nil) && len(t.slow) < 2048 {
		if t.pool != nil {
			p := t.pool.Stat()
			o.Acquired, o.Idle, o.Total = p.AcquiredConns(), p.IdleConns(), p.TotalConns()
		}
		t.slow = append(t.slow, o)
	}
}
func (t *temporalSourceTrace) export(proof string) error {
	t.mu.Lock()
	defer t.mu.Unlock()
	raw, err := json.Marshal(map[string]any{"query_counts": t.counts, "slow_or_failed_sql": t.slow, "phase_counts": t.phases, "slow_or_failed_phases": t.probes, "pg_wait_samples": t.waits, "sql_or_parameters_retained": false})
	if err != nil {
		return err
	}
	return temporalWriteExclusive(filepath.Join(proof, "source-probes.json"), raw)
}

func (trace *temporalSourceTrace) monitor(t *testing.T, cfg *pgxpool.Config) func() {
	t.Helper()
	cfg = cfg.Copy()
	cfg.MaxConns, cfg.MinConns = 1, 0
	cfg.ConnConfig.Tracer = nil
	ctx, stop := context.WithCancel(context.Background())
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatal("independent bounded PG wait monitor unavailable")
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		stop()
		t.Fatal("PG wait monitor warmup unavailable")
	}
	done := make(chan struct{})
	go func() {
		defer close(done)
		ticker := time.NewTicker(50 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				bounded, cancel := context.WithTimeout(ctx, 100*time.Millisecond)
				var raw json.RawMessage
				err := pool.QueryRow(bounded, `SELECT COALESCE(jsonb_agg(jsonb_build_object('wait_type',wait_event_type,'wait',wait_event,'state',state,'statement_md5',md5(query),'shape',CASE WHEN query ILIKE '%session_ownership%' THEN 'session_ownership' WHEN query ILIKE '%memory_temporal%' THEN 'memory_temporal' WHEN query ILIKE '%memory_%' THEN 'memory' WHEN query ILIKE '%sessions%' THEN 'sessions' WHEN query ILIKE '%tasks%' THEN 'tasks' WHEN lower(query)='commit' THEN 'commit' ELSE 'other' END)),'[]'::jsonb) FROM (SELECT wait_event_type,wait_event,state,query FROM pg_stat_activity WHERE datname=current_database() AND pid<>pg_backend_pid() AND state='active' AND wait_event_type IS NOT NULL ORDER BY query_start LIMIT 16) s`).Scan(&raw)
				cancel()
				trace.mu.Lock()
				if trace.counts == nil {
					trace.counts = map[string]int{}
				}
				trace.counts["wait_monitor_checks"]++
				if err != nil {
					trace.counts["wait_monitor_errors"]++
				}
				trace.mu.Unlock()
				if err == nil && string(raw) != "[]" {
					trace.mu.Lock()
					if len(trace.waits) < 1024 {
						trace.waits = append(trace.waits, raw)
					}
					trace.mu.Unlock()
				}
			}
		}
	}()
	return func() { stop(); <-done; pool.Close() }
}

// Exact read-only replay of the retained current t02 state: the run is now
// terminal, so denial is expected. It does not pretend to recreate its live
// historical state or explain a transient timeout which was not instrumented.
func TestTemporalRetainedSourceGuardReadOnly(t *testing.T) {
	if os.Getenv("BUTLER_TEMPORAL_RETAINED_PROBE") != "1" {
		t.Skip("explicit retained source diagnostic only")
	}
	ctx, stop := context.WithTimeout(context.Background(), 30*time.Second)
	defer stop()
	cfg, err := pgxpool.ParseConfig(os.Getenv("BUTLER_TEST_MEMORY_POSTGRES_URL"))
	if err != nil || cfg.ConnConfig.Database != "issue85_native_test" {
		t.Fatal("retained isolated database required")
	}
	cfg.ConnConfig.RuntimeParams["search_path"] = "memory_temporal_native_v3,public"
	trace := &temporalSourceTrace{}
	cfg.ConnConfig.Tracer = trace
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatal("diagnostic pool unavailable")
	}
	defer pool.Close()
	trace.pool = pool
	defer func() {
		if err := trace.export(os.Getenv("BUTLER_TEMPORAL_NATIVE_PROOF")); err != nil {
			t.Error("safe diagnostic export failed")
		}
	}()
	monitorStop := trace.monitor(t, cfg)
	defer monitorStop()
	var token ownership.Token
	if err := pool.QueryRow(ctx, `SELECT r.session_key,r.run_id,r.lease_id,o.fence_epoch FROM temporal_native_cases c JOIN runs r ON r.run_id=c.run_id JOIN session_ownership o ON o.session_key=c.session_key WHERE c.case_id='t02_current_head'`).Scan(&token.SessionKey, &token.RunID, &token.LeaseID, &token.FenceEpoch); err != nil {
		t.Fatal("retained owning identity missing")
	}
	var before, after string
	hash := func(target *string) {
		t.Helper()
		if err := pool.QueryRow(ctx, `SELECT md5(jsonb_build_object('history',(`+temporalNativeHistorySQL+`),'runs',(SELECT jsonb_agg(to_jsonb(r)ORDER BY run_id)FROM runs r),'pins',(SELECT jsonb_agg(to_jsonb(p)ORDER BY run_id,claim_id,query_key)FROM memory_temporal_run_pins p))::text)`).Scan(target); err != nil {
			t.Fatal("retained state hash failed")
		}
	}
	hash(&before)
	cat := catalog.New(pool, catalog.NewFileManifest("/native-private/temporal-deletion-anchor"))
	live := ownership.WithToken(ctx, ownership.NewAuthority(pool), token)
	live = catalog.WithRunSourceObserver(live, trace.probe)
	for range 50 {
		check, cancel := context.WithTimeout(live, 200*time.Millisecond)
		err := cat.CheckRunSources(check, token.RunID)
		cancel()
		if err == nil {
			t.Fatal("terminal retained run unexpectedly released")
		}
	}
	hash(&after)
	if before != after {
		t.Fatal("read-only diagnostic mutated retained business or spend state")
	}
}
