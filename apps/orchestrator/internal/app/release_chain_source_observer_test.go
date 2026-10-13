package app

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strings"
	"sync"
	"time"

	"github.com/butler/butler/apps/orchestrator/internal/session"
	"github.com/butler/butler/apps/orchestrator/internal/task"
	"github.com/butler/butler/internal/budget"
	"github.com/butler/butler/internal/ownership"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Test-only, failure-only, context-local observations. None of these values is
// authority, a recovered source proof, or permission to retry a failed check.
type releaseChainSourceObserver struct {
	mu      sync.Mutex
	failed  []releaseChainSourceFailure
	dropped int
}

type releaseChainSourcePoolStats struct {
	Max       int32 `json:"max_conns"`
	Total     int32 `json:"total_conns"`
	Acquired  int32 `json:"acquired_conns"`
	Idle      int32 `json:"idle_conns"`
	Empty     int64 `json:"empty_acquire_count"`
	Cancelled int64 `json:"cancelled_acquire_count"`
	Duration  int64 `json:"acquire_duration_ms"`
}

type releaseChainSourceFailure struct {
	Role          string                      `json:"role"`
	RunID         *string                     `json:"run_id"`
	Operation     *string                     `json:"kernel_operation_id"`
	Started       time.Time                   `json:"started_at_utc"`
	Finished      time.Time                   `json:"finished_at_utc"`
	Elapsed       int64                       `json:"elapsed_ms"`
	Maximum       *int64                      `json:"maximum_check_ms"`
	TimeLeft      *int64                      `json:"context_time_left_ms_at_start"`
	Phase         string                      `json:"phase"`
	Family        *string                     `json:"query_family"`
	QuerySHA      *string                     `json:"query_template_sha256"`
	Failure       string                      `json:"failure_category"`
	SQLState      *string                     `json:"sqlstate"`
	Acquisitions  int                         `json:"acquisition_count"`
	AcquireMillis int64                       `json:"acquisition_elapsed_ms"`
	Before        releaseChainSourcePoolStats `json:"pool_before"`
	After         releaseChainSourcePoolStats `json:"pool_after"`
}

type releaseChainSourceSnapshot struct {
	Version string                      `json:"version"`
	Failed  []releaseChainSourceFailure `json:"failed_checks"`
	Dropped int                         `json:"dropped_failed_checks"`
}

type releaseChainSourceSpanKey struct{}
type releaseChainSourceAcquireKey struct{}
type releaseChainSourceSpan struct {
	owner        *releaseChainSourceObserver
	mu           sync.Mutex
	r            releaseChainSourceFailure
	firstFailure bool
}

func releaseChainSourceStats(pool *pgxpool.Pool) releaseChainSourcePoolStats {
	if pool == nil {
		return releaseChainSourcePoolStats{}
	}
	s := pool.Stat()
	return releaseChainSourcePoolStats{s.MaxConns(), s.TotalConns(), s.AcquiredConns(), s.IdleConns(), s.EmptyAcquireCount(), s.CanceledAcquireCount(), s.AcquireDuration().Milliseconds()}
}

func releaseChainSourceError(err error) (string, *string) {
	if errors.Is(err, context.DeadlineExceeded) {
		return "deadline_exceeded", nil
	}
	if errors.Is(err, context.Canceled) {
		return "cancelled", nil
	}
	var pg *pgconn.PgError
	if errors.As(err, &pg) {
		// Never propagate the SQL error message, details, hint, table or values.
		switch pg.Code {
		case "55P03", "57014", "40P01", "40001", "08000", "08003", "08006", "53300":
			code := pg.Code
			return "postgres_error", &code
		default:
			return "postgres_error", nil
		}
	}
	return "source_check_failed", nil
}

func (o *releaseChainSourceObserver) observe(ctx context.Context, pool *pgxpool.Pool, role, run string, maximum time.Duration, call func(context.Context) error) error {
	span := &releaseChainSourceSpan{owner: o, r: releaseChainSourceFailure{Role: role, Started: time.Now().UTC(), Phase: "check_started", Before: releaseChainSourceStats(pool)}}
	if run != "" {
		value := run
		span.r.RunID = &value
	}
	if op, ok := budget.ModelOperationFromContext(ctx); ok {
		span.r.Operation = &op
	}
	if maximum > 0 {
		n := maximum.Milliseconds()
		span.r.Maximum = &n
	}
	if deadline, ok := ctx.Deadline(); ok {
		n := time.Until(deadline).Milliseconds()
		span.r.TimeLeft = &n
	}
	err := call(context.WithValue(ctx, releaseChainSourceSpanKey{}, span))
	if err == nil {
		return nil
	}
	span.mu.Lock()
	span.r.Finished = time.Now().UTC()
	span.r.Elapsed = span.r.Finished.Sub(span.r.Started).Milliseconds()
	span.r.After = releaseChainSourceStats(pool)
	if !span.firstFailure {
		span.r.Failure, span.r.SQLState = releaseChainSourceError(err)
		if ctx.Err() != nil {
			span.r.Failure, span.r.SQLState = releaseChainSourceError(ctx.Err())
		}
	}
	record := span.r
	span.mu.Unlock()
	o.mu.Lock()
	defer o.mu.Unlock()
	if len(o.failed) < 64 {
		o.failed = append(o.failed, record)
	} else {
		o.dropped++
	}
	return err
}

func (o *releaseChainSourceObserver) span(ctx context.Context) *releaseChainSourceSpan {
	s, ok := ctx.Value(releaseChainSourceSpanKey{}).(*releaseChainSourceSpan)
	if !ok || s == nil || s.owner != o {
		return nil
	}
	return s
}

func (o *releaseChainSourceObserver) TraceAcquireStart(ctx context.Context, _ *pgxpool.Pool, _ pgxpool.TraceAcquireStartData) context.Context {
	if s := o.span(ctx); s != nil {
		s.mu.Lock()
		if !s.firstFailure {
			s.r.Phase, s.r.Family, s.r.QuerySHA = "pool_acquire", nil, nil
		}
		s.r.Acquisitions++
		s.mu.Unlock()
		return context.WithValue(ctx, releaseChainSourceAcquireKey{}, time.Now())
	}
	return ctx
}

func (o *releaseChainSourceObserver) TraceAcquireEnd(ctx context.Context, _ *pgxpool.Pool, data pgxpool.TraceAcquireEndData) {
	if s := o.span(ctx); s != nil {
		s.mu.Lock()
		defer s.mu.Unlock()
		if at, ok := ctx.Value(releaseChainSourceAcquireKey{}).(time.Time); ok {
			s.r.AcquireMillis += time.Since(at).Milliseconds()
		}
		if s.firstFailure {
			return
		}
		if data.Err != nil {
			s.r.Failure, s.r.SQLState = releaseChainSourceError(data.Err)
			s.firstFailure = true
		} else {
			s.r.Phase = "connection_acquired"
		}
	}
}

func releaseChainSourceQueryFamily(sql string) string {
	// SQL and Args never enter the record. Only known public template families
	// receive a digest; arbitrary statements are represented as other_query.
	for _, name := range []string{"memory_restore_state", "memory_deletions", "session_ownership", "butler_memory_check_run_sources", "memory_run_sources", "memory_sources", "task_authority_bindings", "action_mandates", "owner_goals", "owner_purpose_policies", "converter_", "task_delegations", "transcript_", "tasks", "runs"} {
		if strings.Contains(sql, name) {
			return name
		}
	}
	switch strings.ToLower(strings.TrimSpace(sql)) {
	case "begin", "commit", "rollback":
		return "transaction_control"
	}
	return "other_query"
}

func (o *releaseChainSourceObserver) TraceQueryStart(ctx context.Context, _ *pgx.Conn, data pgx.TraceQueryStartData) context.Context {
	if s := o.span(ctx); s != nil {
		s.mu.Lock()
		defer s.mu.Unlock()
		if !s.firstFailure {
			family := releaseChainSourceQueryFamily(data.SQL)
			s.r.Phase, s.r.Family, s.r.QuerySHA = "query", &family, nil
			if family != "other_query" {
				h := sha256.Sum256([]byte(data.SQL))
				digest := hex.EncodeToString(h[:])
				s.r.QuerySHA = &digest
			}
		}
	}
	return ctx
}

func (o *releaseChainSourceObserver) TraceQueryEnd(ctx context.Context, _ *pgx.Conn, data pgx.TraceQueryEndData) {
	if s := o.span(ctx); s != nil {
		s.mu.Lock()
		defer s.mu.Unlock()
		if s.firstFailure {
			return
		}
		if data.Err != nil {
			s.r.Failure, s.r.SQLState = releaseChainSourceError(data.Err)
			s.firstFailure = true
		} else {
			s.r.Phase = "query_complete"
		}
	}
}

func (o *releaseChainSourceObserver) snapshot() releaseChainSourceSnapshot {
	o.mu.Lock()
	defer o.mu.Unlock()
	return releaseChainSourceSnapshot{Version: "butler-source-check-observation-v1", Failed: append([]releaseChainSourceFailure{}, o.failed...), Dropped: o.dropped}
}

// All wrappers delegate to the unchanged production implementation and preserve
// its interfaces, context, return values, cancellation, fencing and bounds.
type releaseChainObservedSources struct {
	converterSources
	observer *releaseChainSourceObserver
}

func (s releaseChainObservedSources) CheckRunSources(ctx context.Context, run string) error {
	return s.observer.observe(ctx, s.pool, "source_check", run, s.MemorySourceCheckTimeout(), func(c context.Context) error { return s.converterSources.CheckRunSources(c, run) })
}
func (s releaseChainObservedSources) PinTranscript(ctx context.Context, key string, revision int64) error {
	run := ""
	if token, ok := ownership.TokenFromContext(ctx); ok {
		run = token.RunID
	}
	return s.observer.observe(ctx, s.pool, "pin_transcript", run, 0, func(c context.Context) error { return s.converterSources.PinTranscript(c, key, revision) })
}

type releaseChainObservedLeases struct {
	*session.PostgresLeaseManager
	pool     *pgxpool.Pool
	observer *releaseChainSourceObserver
}

func (s releaseChainObservedLeases) GetLease(ctx context.Context, id string) (record session.LeaseRecord, err error) {
	run := ""
	if token, ok := ownership.TokenFromContext(ctx); ok {
		run = token.RunID
	}
	err = s.observer.observe(ctx, s.pool, "lease_watchdog", run, 750*time.Millisecond, func(c context.Context) error { record, err = s.PostgresLeaseManager.GetLease(c, id); return err })
	return record, err
}

type releaseChainObservedTasks struct {
	*task.Store
	pool     *pgxpool.Pool
	observer *releaseChainSourceObserver
}

func (s releaseChainObservedTasks) ValidateRunAuthority(ctx context.Context, run string) error {
	return s.observer.observe(ctx, s.pool, "task_authority_watchdog", run, 750*time.Millisecond, func(c context.Context) error { return s.Store.ValidateRunAuthority(c, run) })
}
