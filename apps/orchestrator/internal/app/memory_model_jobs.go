package app

import (
	"github.com/butler/butler/internal/budget"
	"github.com/butler/butler/internal/memory/catalog"
	"github.com/butler/butler/internal/memory/jobs"
	"github.com/butler/butler/internal/memory/summary"
	"github.com/butler/butler/internal/providerfactory"
	"os"
	"strconv"
)

func configureModelJobs(c *catalog.Store, b *budget.Controller, p providerfactory.Result) (*jobs.Store, *jobs.ModelWorker) {
	s := jobs.New(c.Pool(), c, b, nil)
	if os.Getenv("BUTLER_MEMORY_MODEL_JOBS_ENABLED") != "true" {
		return s, nil
	}
	handlers := map[string]jobs.ModelHandler{}
	configureCandidateHandler(handlers)
	if os.Getenv("BUTLER_MEMORY_MODEL_QUOTE_PROBE_ENABLED") == "true" {
		handlers["source-quote-v1"] = jobs.QuoteProbe{}
	}
	if os.Getenv("BUTLER_MEMORY_SUMMARY_CONSUMER_ENABLED") == "true" {
		contextBudget, _ := strconv.Atoi(os.Getenv("BUTLER_CONTEXT_BUDGET_TOKENS"))
		handlers[summary.HandlerName] = summary.Consumer{ContextBudget: contextBudget}
	}
	return s, &jobs.ModelWorker{Jobs: s, Provider: p.Provider, Model: p.ModelName, Handlers: handlers}
}
