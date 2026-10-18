package app

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/butler/butler/internal/transport"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// This boundary control delays scripted model events, never calls a provider,
// and retains the ordinary two-connection pool and all authority/source fences.
// It exercises lease checks, the twenty-second ingress heartbeat and paging
// while the real Flow and Broker consume the exact selected-source corpus.
func TestConsolidationControlledSlowPagingIntegration(t *testing.T) {
	if os.Getenv("BUTLER_CONSOLIDATION_SLOW_CONTROL") != "1" || os.Getenv("BUTLER_MEMORY_READ_APP_BINARY") == "" {
		t.Skip("explicit isolated slow Flow/Broker control required")
	}
	if os.Getenv("BUTLER_CONSOLIDATION_NATIVE_V1") != "" || os.Getenv("BUTLER_CONSOLIDATION_NATIVE_V2") != "" {
		t.Fatal("slow control cannot consume a native window")
	}
	runConsolidationPairedScreen(t, false)
}

type consolidationSlowControlProvider struct{ transport.ModelProvider }

func (p *consolidationSlowControlProvider) InferenceLimits() (int64, int64) {
	return p.ModelProvider.(interface{ InferenceLimits() (int64, int64) }).InferenceLimits()
}
func (p *consolidationSlowControlProvider) InferenceTimeout() time.Duration {
	return p.ModelProvider.(interface{ InferenceTimeout() time.Duration }).InferenceTimeout()
}
func (p *consolidationSlowControlProvider) StartRun(ctx context.Context, r transport.StartRunRequest) (transport.EventStream, error) {
	stream, err := p.ModelProvider.StartRun(ctx, r)
	if err != nil || strings.HasPrefix(r.Context.SessionKey, "memory:") {
		return stream, err
	}
	return consolidationSlowControlStream(ctx, stream), nil
}
func (p *consolidationSlowControlProvider) SubmitToolResult(ctx context.Context, r transport.SubmitToolResultRequest) (transport.EventStream, error) {
	stream, err := p.ModelProvider.SubmitToolResult(ctx, r)
	if err != nil {
		return nil, err
	}
	return consolidationSlowControlStream(ctx, stream), nil
}
func consolidationSlowControlStream(ctx context.Context, stream transport.EventStream) transport.EventStream {
	out := make(chan transport.TransportEvent)
	go func() {
		defer close(out)
		timer := time.NewTimer(10 * time.Second)
		defer timer.Stop()
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
		}
		for event := range stream {
			select {
			case <-ctx.Done():
				return
			case out <- event:
			}
		}
	}()
	return out
}

type consolidationSlowQueryKey struct{}
type consolidationSlowQueryStart struct {
	at    time.Time
	class string
}
type consolidationSlowTrace struct {
	*summaryPGTrace
	mu     sync.Mutex
	checks []map[string]any
}

func (p *consolidationSlowTrace) TraceQueryStart(ctx context.Context, c *pgx.Conn, d pgx.TraceQueryStartData) context.Context {
	ctx = p.summaryPGTrace.TraceQueryStart(ctx, c, d)
	class := "other"
	switch {
	case strings.Contains(d.SQL, "UPDATE accepted_inputs SET claim_expires_at="):
		class = "input_claim_renew"
	case strings.Contains(d.SQL, "WHERE lease_id=$1 AND expires_at>clock_timestamp()"):
		class = "lease_authority_read"
	case strings.Contains(d.SQL, "SELECT t.task_id,t.owner_id FROM tasks t JOIN runs"):
		class = "task_authority_read"
	}
	return context.WithValue(ctx, consolidationSlowQueryKey{}, consolidationSlowQueryStart{at: time.Now(), class: class})
}
func (p *consolidationSlowTrace) TraceQueryEnd(ctx context.Context, c *pgx.Conn, d pgx.TraceQueryEndData) {
	p.summaryPGTrace.TraceQueryEnd(ctx, c, d)
	start, ok := ctx.Value(consolidationSlowQueryKey{}).(consolidationSlowQueryStart)
	if !ok || start.class == "other" || d.Err == nil && start.class != "input_claim_renew" {
		return
	}
	diagnostic := "ok"
	if d.Err != nil {
		diagnostic = "query_error"
		if ctx.Err() != nil {
			diagnostic = ctx.Err().Error()
		}
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if len(p.checks) < 256 {
		p.checks = append(p.checks, map[string]any{"class": start.class, "millis": time.Since(start.at).Milliseconds(), "rows": d.CommandTag.RowsAffected(), "diagnostic": diagnostic})
	}
}

func consolidationSlowControlPool(t *testing.T, original *pgxpool.Pool) *pgxpool.Pool {
	t.Helper()
	if os.Getenv("BUTLER_CONSOLIDATION_SLOW_CONTROL") != "1" {
		return original
	}
	cfg := original.Config()
	if cfg.MaxConns != 2 {
		t.Fatal("slow control requires the original two-connection pool")
	}
	trace := &consolidationSlowTrace{summaryPGTrace: &summaryPGTrace{}}
	cfg.ConnConfig.Tracer = trace
	pool, err := pgxpool.NewWithConfig(t.Context(), cfg)
	if err != nil {
		t.Fatal("slow control diagnostic pool unavailable")
	}
	original.Close()
	trace.pool = pool
	t.Cleanup(func() {
		trace.mu.Lock()
		checks := append([]map[string]any{}, trace.checks...)
		trace.mu.Unlock()
		raw, err := json.MarshalIndent(map[string]any{"native_calls": 0, "auth_imported": false, "pool_max_connections": 2, "fixed_model_delay_seconds": 10, "pg": trace.snapshot(), "claim_and_authority_checks": checks}, "", "  ")
		if err != nil || os.WriteFile(filepath.Join(os.Getenv("BUTLER_CONSOLIDATION_SCREEN_PROOF"), "slow-control-pg.json"), raw, 0600) != nil {
			t.Error("safe slow-control diagnostic receipt unavailable")
		}
		pool.Close()
	})
	return pool
}
