package api

import (
	"context"
	"errors"
	"github.com/butler/butler/internal/memory/jobs"
	"strings"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

type candidateQueryStart struct {
	at          time.Time
	shape, hash string
}
type candidateQueryKey struct{}
type candidateQueryObservation struct {
	Shape                 string `json:"shape"`
	Hash                  string `json:"statement_sha256"`
	Millis                int64  `json:"millis"`
	Diagnostic            string `json:"diagnostic"`
	Acquired, Idle, Total int32
}

// Native diagnostics deliberately retain no SQL, args, provider auth or raw
// errors. Only bounded slow/failing statement classes and hashes are retained.
type candidatePGTrace struct {
	mu     sync.Mutex
	pool   *pgxpool.Pool
	slow   []candidateQueryObservation
	counts map[string]int
	probes []jobs.ModelProbe
}

func (t *candidatePGTrace) probe(p jobs.ModelProbe) {
	if p.Millis < 100 && p.Diagnostic == "" {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if len(t.probes) < 512 {
		t.probes = append(t.probes, p)
	}
}

func (t *candidatePGTrace) TraceQueryStart(ctx context.Context, _ *pgx.Conn, d pgx.TraceQueryStartData) context.Context {
	shape := "other"
	for _, s := range []string{"system_settings", "memory_aux_jobs", "memory_model_policies", "memory_model_control", "memory_source_dependencies", "memory_source_revisions", "memory_sources", "memory_restore_state", "memory_deletions", "memory_model_attempts", "execution_budget", "session_ownership", "tasks", "sessions"} {
		if strings.Contains(d.SQL, s) {
			shape = s
			break
		}
	}
	return context.WithValue(ctx, candidateQueryKey{}, candidateQueryStart{time.Now(), shape, candidateHash([]byte(d.SQL))})
}
func (t *candidatePGTrace) TraceQueryEnd(ctx context.Context, _ *pgx.Conn, d pgx.TraceQueryEndData) {
	s, ok := ctx.Value(candidateQueryKey{}).(candidateQueryStart)
	if !ok {
		return
	}
	o := candidateQueryObservation{Shape: s.shape, Hash: s.hash, Millis: time.Since(s.at).Milliseconds()}
	if d.Err != nil {
		var p *pgconn.PgError
		switch {
		case errors.As(d.Err, &p):
			o.Diagnostic = "pg_" + p.Code
		case errors.Is(d.Err, context.DeadlineExceeded):
			o.Diagnostic = "deadline_exceeded"
		case errors.Is(d.Err, context.Canceled):
			o.Diagnostic = "cancelled"
		default:
			o.Diagnostic = "storage_error"
		}
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.counts == nil {
		t.counts = map[string]int{}
	}
	t.counts[s.shape]++
	if o.Millis >= 100 || d.Err != nil {
		if t.pool != nil {
			p := t.pool.Stat()
			o.Acquired = p.AcquiredConns()
			o.Idle = p.IdleConns()
			o.Total = p.TotalConns()
		}
		if len(t.slow) < 512 {
			t.slow = append(t.slow, o)
		}
	}
}
func (t *candidatePGTrace) snapshot() any {
	t.mu.Lock()
	defer t.mu.Unlock()
	return struct {
		Counts map[string]int              `json:"counts"`
		Slow   []candidateQueryObservation `json:"slow_or_failed"`
		Probes []jobs.ModelProbe           `json:"slow_or_failed_probes"`
	}{t.counts, t.slow, t.probes}
}
