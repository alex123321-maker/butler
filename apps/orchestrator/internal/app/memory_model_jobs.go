package app

import (
	"github.com/butler/butler/internal/budget"
	"github.com/butler/butler/internal/memory/catalog"
	"github.com/butler/butler/internal/memory/compaction"
	"github.com/butler/butler/internal/memory/jobs"
	"github.com/butler/butler/internal/providerfactory"
	"github.com/butler/butler/internal/transport/antigravity"
	"os"
	"time"
)

func configureModelJobs(c *catalog.Store, b *budget.Controller, p providerfactory.Result, contextBudgets ...int) (*jobs.Store, *jobs.ModelWorker) {
	s := jobs.New(c.Pool(), c, b, nil)
	if os.Getenv("BUTLER_MEMORY_MODEL_JOBS_ENABLED") != "true" {
		return s, nil
	}
	handlers := map[string]jobs.ModelHandler{}
	configureCandidateHandler(handlers, p, b)
	if os.Getenv("BUTLER_MEMORY_MODEL_QUOTE_PROBE_ENABLED") == "true" {
		handlers["source-quote-v1"] = jobs.QuoteProbe{}
	}
	if os.Getenv("BUTLER_MEMORY_SUMMARY_CONSUMER_ENABLED") == "true" && compaction.ReleaseQualified && summaryProfileReady(p, b) {
		contextBudget := 65536
		if len(contextBudgets) > 0 {
			contextBudget = contextBudgets[0]
		}
		handlers[compaction.HandlerName] = compaction.Consumer{ContextBudget: contextBudget}
	}
	return s, &jobs.ModelWorker{Jobs: s, Provider: p.Provider, Model: p.ModelName, Handlers: handlers}
}

func summaryProfileReady(p providerfactory.Result, b *budget.Controller) bool {
	if p.Provider == nil || p.Provider.Name() != antigravity.Name || p.ModelName != antigravity.FlashHighModel || b == nil {
		return false
	}
	limits, ok := p.Provider.(interface{ InferenceLimits() (int64, int64) })
	if !ok {
		return false
	}
	_, output := limits.InferenceLimits()
	timed, ok := p.Provider.(interface{ InferenceTimeout() time.Duration })
	return ok && timed.InferenceTimeout() >= 300*time.Second && b.Policy.ModelMillis >= 300000 && output == 32768 && b.Policy.OutputTokens >= output
}

func summaryConsumerConfigured(w *jobs.ModelWorker) bool {
	if w == nil {
		return false
	}
	_, ok := w.Handlers[compaction.HandlerName]
	return ok
}
