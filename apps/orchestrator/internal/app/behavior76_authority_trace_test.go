package app

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/butler/butler/internal/transport"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

const behavior76AuthorityTraceLimit = 65536

// This test-only observer keeps bounded value-free metadata in memory. It never
// changes a timeout, SQL statement, argument, pool setting, result or error, and
// performs no database writes. Wiring belongs to the fixture constructor.
type behavior76AuthorityTrace struct {
	mu                sync.Mutex
	events            []behavior76AuthorityTraceEvent
	nextSpan          uint64
	sequence          uint64
	openSpans         uint64
	dropped           uint64
	operationFailures uint64
	observerFailed    bool
}

type behavior76AuthorityTracePoolStats struct {
	Available bool  `json:"available"`
	Maximum   int32 `json:"max_conns"`
	Total     int32 `json:"total_conns"`
	Acquired  int32 `json:"acquired_conns"`
	Idle      int32 `json:"idle_conns"`
}

type behavior76AuthorityTraceEvent struct {
	Sequence      uint64                            `json:"sequence"`
	Span          uint64                            `json:"span"`
	Phase         string                            `json:"phase"`
	Edge          string                            `json:"edge"`
	At            time.Time                         `json:"at_utc"`
	ElapsedMicros int64                             `json:"elapsed_us"`
	Deadline      *time.Time                        `json:"original_context_deadline_utc"`
	TimeLeft      *int64                            `json:"original_context_time_left_us"`
	Family        string                            `json:"query_family"`
	SQLSHA256     string                            `json:"public_sql_template_sha256,omitempty"`
	Outcome       string                            `json:"outcome"`
	SQLState      string                            `json:"sqlstate,omitempty"`
	ContextCause  string                            `json:"context_cause"`
	Pool          behavior76AuthorityTracePoolStats `json:"pool"`
}

type behavior76AuthorityTraceSnapshot struct {
	Version           string                          `json:"version"`
	MaximumEvents     int                             `json:"maximum_events"`
	Events            []behavior76AuthorityTraceEvent `json:"events"`
	OpenSpans         uint64                          `json:"open_spans"`
	Dropped           uint64                          `json:"dropped_events"`
	OperationFailures uint64                          `json:"operation_failures"`
	Failed            bool                            `json:"observer_failed"`
}

type behavior76AuthorityTraceSpan struct {
	owner   *behavior76AuthorityTrace
	id      uint64
	phase   string
	family  string
	hash    string
	started time.Time
	ended   atomic.Bool
}

type behavior76AuthorityQueryTraceKey struct{}
type behavior76AuthorityAcquireTraceKey struct{}

var (
	_ pgx.QueryTracer       = (*behavior76AuthorityTrace)(nil)
	_ pgxpool.AcquireTracer = (*behavior76AuthorityTrace)(nil)
)

func newBehavior76AuthorityTrace() *behavior76AuthorityTrace {
	return &behavior76AuthorityTrace{events: make([]behavior76AuthorityTraceEvent, 0, 256)}
}

func behavior76AuthorityTraceStats(pool *pgxpool.Pool) behavior76AuthorityTracePoolStats {
	if pool == nil {
		return behavior76AuthorityTracePoolStats{}
	}
	s := pool.Stat()
	return behavior76AuthorityTracePoolStats{true, s.MaxConns(), s.TotalConns(), s.AcquiredConns(), s.IdleConns()}
}

func behavior76AuthorityTraceFailure(err error) (category, sqlstate string) {
	if err == nil {
		return "ok", ""
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return "deadline", ""
	}
	if errors.Is(err, context.Canceled) {
		return "cancelled", ""
	}
	var pg *pgconn.PgError
	if errors.As(err, &pg) {
		// Never copy error message/detail/hint/table/constraint or arbitrary codes.
		switch pg.Code {
		case "40001", "40P01", "55P03", "57014", "08000", "08001", "08003", "08004", "08006", "08007", "08P01", "53300", "53400", "53200", "55000", "57P01", "57P02", "57P03", "25P02", "25006", "42P01", "42703", "42501", "23505", "23503", "22023", "22P02":
			return "postgres_error", pg.Code
		}
		return "postgres_error", ""
	}
	switch code := transport.DiagnosticCode(err); code {
	case "execution_authority_check_failed", "execution_ownership_changed", "execution_authority_observation_timeout", "execution_lease_renewal_failed", "memory_source_stale", "memory_source_check_timeout":
		return code, ""
	}
	return "other_error", ""
}

func behavior76AuthorityTraceCause(ctx context.Context) string {
	if cause := context.Cause(ctx); cause != nil {
		category, _ := behavior76AuthorityTraceFailure(cause)
		return category
	}
	return "none"
}

func behavior76AuthorityTraceDeadline(ctx context.Context) (*time.Time, *int64) {
	deadline, ok := ctx.Deadline()
	if !ok {
		return nil, nil
	}
	left := time.Until(deadline).Microseconds()
	deadline = deadline.UTC()
	return &deadline, &left
}

func (o *behavior76AuthorityTrace) appendLocked(event behavior76AuthorityTraceEvent) {
	o.sequence++
	event.Sequence = o.sequence
	if len(o.events) >= behavior76AuthorityTraceLimit {
		o.dropped++
		o.observerFailed = true
		return
	}
	o.events = append(o.events, event)
}

func (o *behavior76AuthorityTrace) start(ctx context.Context, phase, family, hash string, pool *pgxpool.Pool) *behavior76AuthorityTraceSpan {
	now := time.Now()
	deadline, left := behavior76AuthorityTraceDeadline(ctx)
	stats := behavior76AuthorityTraceStats(pool)
	o.mu.Lock()
	defer o.mu.Unlock()
	o.nextSpan++
	o.openSpans++
	span := &behavior76AuthorityTraceSpan{owner: o, id: o.nextSpan, phase: phase, family: family, hash: hash, started: now}
	o.appendLocked(behavior76AuthorityTraceEvent{Span: span.id, Phase: phase, Edge: "start", At: now.UTC(), Deadline: deadline, TimeLeft: left, Family: family, SQLSHA256: hash, Outcome: "pending", ContextCause: behavior76AuthorityTraceCause(ctx), Pool: stats})
	return span
}

func (o *behavior76AuthorityTrace) end(ctx context.Context, span *behavior76AuthorityTraceSpan, phase string, err error, pool *pgxpool.Pool) {
	now := time.Now()
	deadline, left := behavior76AuthorityTraceDeadline(ctx)
	stats := behavior76AuthorityTraceStats(pool)
	category, sqlstate := behavior76AuthorityTraceFailure(err)
	cause := behavior76AuthorityTraceCause(ctx)
	o.mu.Lock()
	defer o.mu.Unlock()
	if err != nil {
		o.operationFailures++
	}
	if span == nil || span.owner != o || span.phase != phase || span.ended.Swap(true) || o.openSpans == 0 {
		o.observerFailed = true
		o.appendLocked(behavior76AuthorityTraceEvent{Phase: phase, Edge: "unpaired_end", At: now.UTC(), Deadline: deadline, TimeLeft: left, Family: "unavailable", Outcome: category, SQLState: sqlstate, ContextCause: cause, Pool: stats})
		return
	}
	o.openSpans--
	o.appendLocked(behavior76AuthorityTraceEvent{Span: span.id, Phase: phase, Edge: "end", At: now.UTC(), ElapsedMicros: now.Sub(span.started).Microseconds(), Deadline: deadline, TimeLeft: left, Family: span.family, SQLSHA256: span.hash, Outcome: category, SQLState: sqlstate, ContextCause: cause, Pool: stats})
}

func (o *behavior76AuthorityTrace) TraceQueryStart(ctx context.Context, _ *pgx.Conn, data pgx.TraceQueryStartData) context.Context {
	// Args are deliberately never inspected, retained, counted or serialized.
	family, hash := behavior76AuthorityTraceSQL(data.SQL)
	span := o.start(ctx, "query", family, hash, nil)
	return context.WithValue(ctx, behavior76AuthorityQueryTraceKey{}, span)
}

func (o *behavior76AuthorityTrace) TraceQueryEnd(ctx context.Context, _ *pgx.Conn, data pgx.TraceQueryEndData) {
	span, _ := ctx.Value(behavior76AuthorityQueryTraceKey{}).(*behavior76AuthorityTraceSpan)
	o.end(ctx, span, "query", data.Err, nil)
}

func (o *behavior76AuthorityTrace) TraceAcquireStart(ctx context.Context, pool *pgxpool.Pool, _ pgxpool.TraceAcquireStartData) context.Context {
	span := o.start(ctx, "pool_acquire", "pool_acquire", "", pool)
	return context.WithValue(ctx, behavior76AuthorityAcquireTraceKey{}, span)
}

func (o *behavior76AuthorityTrace) TraceAcquireEnd(ctx context.Context, pool *pgxpool.Pool, data pgxpool.TraceAcquireEndData) {
	span, _ := ctx.Value(behavior76AuthorityAcquireTraceKey{}).(*behavior76AuthorityTraceSpan)
	o.end(ctx, span, "pool_acquire", data.Err, pool)
}

func (o *behavior76AuthorityTrace) snapshot() behavior76AuthorityTraceSnapshot {
	o.mu.Lock()
	defer o.mu.Unlock()
	events := append([]behavior76AuthorityTraceEvent{}, o.events...)
	for i := range events {
		if events[i].Deadline != nil {
			value := *events[i].Deadline
			events[i].Deadline = &value
		}
		if events[i].TimeLeft != nil {
			value := *events[i].TimeLeft
			events[i].TimeLeft = &value
		}
	}
	return behavior76AuthorityTraceSnapshot{"behavior76-authority-pg-trace-v1", behavior76AuthorityTraceLimit, events, o.openSpans, o.dropped, o.operationFailures, o.observerFailed}
}

// failed means this observer lost metadata/correlation, including cap overflow.
// Actual SQL/acquisition errors are separately retained as operation_failures;
// the caller's ordinary authority/error handling remains authoritative.
func (o *behavior76AuthorityTrace) failed() bool {
	o.mu.Lock()
	defer o.mu.Unlock()
	return o.observerFailed
}

func behavior76AuthorityTraceSQL(sql string) (family, hash string) {
	q := strings.ToLower(strings.Join(strings.Fields(sql), " "))
	family = "other_query"
	for _, item := range []struct{ token, family string }{
		{"butler_task_check_schedule", "schedule_check"},
		{"butler_task_check_delegation", "delegation_check"},
		{"scheduled_occurrences", "schedule_probe"},
		{"task_delegations", "delegation_metadata"},
		{"task_authority_bindings", "owner_assignment"},
		{"action_mandates", "owner_mandate"},
		{"owner_purpose_policies", "owner_purpose_policy"},
		{"owner_goals", "owner_goal"},
		{"session_ownership", "lease"},
		{"butler_memory_check_run_sources", "source_guard"},
		{"memory_restore_state", "restore_watermark"},
		{"memory_deletions", "deletion_manifest"},
		{"memory_run_sources", "source_pins"},
		{"execution_", "budget"},
		{"tasks", "task_current"},
		{"runs", "run_current"},
	} {
		if strings.Contains(q, item.token) {
			family = item.family
			break
		}
	}
	// Hash only exact normalized, public parameterized templates. Unknown SQL
	// can contain literal values: retain its fixed family/timing, never its text
	// or a value-dependent hash. No argument or result enters these templates.
	switch q {
	case "begin", "commit", "rollback":
		family = "transaction_control"
	case "select lease_id,session_key,run_id,owner_id,fence_epoch,acquired_at,expires_at from session_ownership where lease_id=$1 and expires_at>clock_timestamp()",
		"select t.task_id,t.owner_id from tasks t join runs r on r.task_id=t.task_id where r.run_id=$1 and t.revision=r.task_revision and t.status='active'",
		"select task_id is not null from runs where run_id=$1",
		"select to_regclass('scheduled_occurrences') is not null",
		"select butler_task_check_schedule($1)",
		"select to_regclass('task_delegations') is not null",
		"select butler_task_check_delegation($1)",
		"select task_id,owner_id,goal_id,goal_revision,mandate_id,mandate_revision,purpose,initiative_reason,observation_ref,policy_revision from task_authority_bindings where task_id=$1",
		"select coalesce(task_id,'') from runs where run_id=$1 for share",
		"select watermark from memory_restore_state where singleton",
		"select deletion_id,source_id,owner_id,action,watermark from memory_deletions order by watermark",
		"select butler_memory_check_run_sources($1)",
		"select coalesce(array_agg(source_id),'{}'::text[]) from memory_run_sources where run_id=$1":
	default:
		return family, ""
	}
	sum := sha256.Sum256([]byte(q))
	return family, hex.EncodeToString(sum[:])
}
