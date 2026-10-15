package app

import (
	"github.com/butler/butler/internal/budget"
	"github.com/butler/butler/internal/memory/catalog"
	"github.com/butler/butler/internal/memory/compaction"
	"github.com/butler/butler/internal/memory/jobs"
	"github.com/butler/butler/internal/providerfactory"
	"os"
)

func configureModelJobs(c *catalog.Store, b *budget.Controller, p providerfactory.Result, contextBudgets ...int) (*jobs.Store, *jobs.ModelWorker) {
	s := jobs.New(c.Pool(), c, b, nil)
	if os.Getenv("BUTLER_MEMORY_MODEL_JOBS_ENABLED") != "true" {
		return s, nil
	}
	handlers := map[string]jobs.ModelHandler{}
	configureCandidateHandler(handlers)
	if os.Getenv("BUTLER_MEMORY_MODEL_QUOTE_PROBE_ENABLED") == "true" {
		handlers["source-quote-v1"] = jobs.QuoteProbe{}
	}
	if os.Getenv("BUTLER_MEMORY_SUMMARY_CONSUMER_ENABLED") == "true" && compaction.ReleaseQualified {
		contextBudget := 65536
		if len(contextBudgets) > 0 {
			contextBudget = contextBudgets[0]
		}
		handlers[compaction.HandlerName] = compaction.Consumer{ContextBudget: contextBudget}
	}
	return s, &jobs.ModelWorker{Jobs: s, Provider: p.Provider, Model: p.ModelName, Handlers: handlers}
}
