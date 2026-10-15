package compaction

import (
	"github.com/butler/butler/internal/memory/catalog"
	"github.com/butler/butler/internal/memory/jobs"
	"github.com/butler/butler/internal/memory/summary"
	"testing"
)

func TestSamePolicyScannerRecoversToolAndMessageRevisionAfterRestartIntegration(t *testing.T) {
	for _, mode := range []string{"tool", "correction"} {
		t.Run(mode, func(t *testing.T) {
			f := newConsumerFixture(t)
			if _, err := f.pool.Exec(f.ctx, `INSERT INTO tool_calls(tool_call_id,run_id,tool_name,args,status,runtime_target,started_at)VALUES('tool','run','mail.send','{}','unknown','test',clock_timestamp())`); err != nil {
				t.Fatal(err)
			}
			f.poll(t)
			if _, err := f.worker.ProcessOne(f.ctx); err != nil {
				t.Fatal(err)
			}
			first, _ := summary.NewStore(f.pool).GetSummarySnapshot(f.ctx, "session")
			if mode == "tool" {
				if _, err := f.pool.Exec(f.ctx, `UPDATE tool_calls SET status='failed' WHERE tool_call_id='tool'`); err != nil {
					t.Fatal(err)
				}
			} else {
				if _, err := f.cat.Correct(f.ctx, catalog.Correction{SourceID: "message:" + f.firstMessage(t), ExpectedRevision: 1, Body: "Исправление: остановлена только отправка; сбор продолжать; старое условие отменено"}); err != nil {
					t.Fatal(err)
				}
			}
			f.append(t, 9)
			f.js = jobs.New(f.pool, f.cat, f.jsBudget(), nil)
			f.worker = &jobs.ModelWorker{Jobs: f.js, Provider: f.provider, Model: "fixture", Handlers: map[string]jobs.ModelHandler{HandlerName: Consumer{ContextBudget: 4096}}}
			f.poll(t)
			if did, err := f.worker.ProcessOne(f.ctx); err != nil || !did {
				t.Fatal("scanner did not recover exact same finite policy", did, err)
			}
			second, err := summary.NewStore(f.pool).GetSummarySnapshot(f.ctx, "session")
			if err != nil || second.SourceRevision <= first.SourceRevision || second.ThroughSequence <= first.ThroughSequence {
				t.Fatal(first, second, err)
			}
		})
	}
}
