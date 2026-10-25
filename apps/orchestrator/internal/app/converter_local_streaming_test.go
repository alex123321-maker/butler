package app

import (
	"context"
	"encoding/json"
	"sync/atomic"
	"testing"
	"time"

	"github.com/butler/butler/internal/budget"
	"github.com/butler/butler/internal/generatedtools"
	"github.com/butler/butler/internal/transport"
	"github.com/jackc/pgx/v5/pgxpool"
)

type converterSlowFinalProvider struct {
	transport.ModelProvider
	finals atomic.Int32
	deltas atomic.Int32
}

func (*converterSlowFinalProvider) InferenceLimits() (int64, int64) { return 8192, 128 }
func (*converterSlowFinalProvider) InferenceTimeout() time.Duration { return 120 * time.Second }

func (p *converterSlowFinalProvider) hold(ctx context.Context, stream transport.EventStream, err error) (transport.EventStream, error) {
	if err != nil {
		return nil, err
	}
	out := make(chan transport.TransportEvent)
	go func() {
		defer close(out)
		for event := range stream {
			if event.EventType == transport.EventTypeAssistantFinal && p.finals.Add(1) == 3 {
				// Reproduce C's post-activation long final without HTTP or an
				// owning PG transaction. The unchanged source ticker stays active.
				ticker := time.NewTicker(300 * time.Millisecond)
				deadline := time.NewTimer(time.Minute)
			streaming:
				for i := 0; ; i++ {
					select {
					case <-ctx.Done():
						ticker.Stop()
						deadline.Stop()
						return
					case <-deadline.C:
						break streaming
					case <-ticker.C:
					}
					select {
					case out <- transport.NewAssistantDeltaEvent(event.RunID, p.Name(), transport.AssistantDelta{Content: "SYNTHETIC-HELD-FINAL", SequenceNo: i + 1}):
						p.deltas.Add(1)
					case <-deadline.C:
						break streaming
					case <-ctx.Done():
						ticker.Stop()
						deadline.Stop()
						return
					}
				}
				ticker.Stop()
				deadline.Stop()
			}
			select {
			case out <- event:
			case <-ctx.Done():
				return
			}
		}
	}()
	return out, nil
}
func (p *converterSlowFinalProvider) StartRun(ctx context.Context, q transport.StartRunRequest) (transport.EventStream, error) {
	stream, err := p.ModelProvider.StartRun(ctx, q)
	return p.hold(ctx, stream, err)
}
func (p *converterSlowFinalProvider) SubmitToolResult(ctx context.Context, q transport.SubmitToolResultRequest) (transport.EventStream, error) {
	stream, err := p.ModelProvider.SubmitToolResult(ctx, q)
	return p.hold(ctx, stream, err)
}

func TestConverterDeveloperStreamingLocalContainerE2E(t *testing.T) {
	var f *developerFixture
	var delayed *converterSlowFinalProvider
	observer := &releaseChainSourceObserver{}
	var checks, totalNanos, maxNanos atomic.Int64
	observer.onComplete = func(role string, elapsed time.Duration) {
		if role != "source_check" {
			return
		}
		checks.Add(1)
		totalNanos.Add(int64(elapsed))
		for old := maxNanos.Load(); int64(elapsed) > old; old = maxNanos.Load() {
			if maxNanos.CompareAndSwap(old, int64(elapsed)) {
				break
			}
		}
	}
	cfg := converterLocalNativeConfig()
	cfg.Provider, cfg.Model, cfg.InputTokens, cfg.OutputTokens = "ollama", "converter-worker-fixture", 8192, 128
	cfg.Policy.Models["ollama:converter-worker-fixture"] = budget.ModelPrice{Basis: "local_no_billing"}
	cfg.Policy.OutputTokens = 128
	cfg.FixtureTimeout, cfg.SourceObserver = 4*time.Minute, observer
	cfg.Factory = func(ctx context.Context, pool *pgxpool.Pool, root string) (transport.ModelProvider, error) {
		args := f.args()
		args.Budget = cfg.ChildBudget
		planner := &converterWorkerProvider{need: f.needRef, spec: f.specRef, document: f.document, args: args, admissionInput: f.admissionInput, states: map[string]*converterWorkerState{}}
		checker := &releaseChainEntryChecker{ModelProvider: planner, pool: pool, sources: f.f.sources, root: root, document: f.document, need: f.needRef, spec: f.specRef}
		delayed = &converterSlowFinalProvider{ModelProvider: checker}
		return delayed, nil
	}
	testConverterDeveloperWorkersObserved(t, true, &converterWorkerFaultHooks{factory: func(t *testing.T, p generatedtools.Profile, path string) *developerFixture {
		f = newDeveloperWorkerFixtureWithModel(t, p, path, true, cfg.FixtureTimeout, &cfg)
		t.Cleanup(func() {
			proof, _ := json.Marshal(observer.snapshot())
			t.Log("fixed source-check diagnostics", string(proof))
			t.Logf("source proofs count=%d total_ms=%d max_ms=%d", checks.Load(), time.Duration(totalNanos.Load()).Milliseconds(), time.Duration(maxNanos.Load()).Milliseconds())
			rows, err := f.f.pool.Query(context.Background(), `SELECT funcname,calls,total_time,self_time FROM pg_stat_user_functions WHERE schemaname=current_schema() AND calls>0 ORDER BY total_time DESC LIMIT 12`)
			if err != nil {
				t.Log("optional local function timing unavailable")
				return
			}
			defer rows.Close()
			for rows.Next() {
				var name string
				var calls int64
				var total, self float64
				if rows.Scan(&name, &calls, &total, &self) == nil {
					t.Logf("local function timing name=%s calls=%d total_ms=%.3f self_ms=%.3f", name, calls, total, self)
				}
			}
		})
		return f
	}, after: func(t *testing.T, f *developerFixture, p *converterWorkerProvider) {
		if delayed == nil || delayed.finals.Load() != 4 {
			t.Fatal("long post-activation final and original continuation were not completed")
		}
		t.Logf("60-second wall-bounded post-activation stream, delivered_deltas=%d, current guards, exact original JSON; native calls=0", delayed.deltas.Load())
	}})
}
