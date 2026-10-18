package app

import (
	"github.com/butler/butler/internal/config"
	"github.com/butler/butler/internal/memory/adaptive"
	"github.com/butler/butler/internal/memory/catalog"
)

// Code-owned qualification cannot be bypassed by flags or stored settings.
// Component tests and the isolated evaluation compose the real production
// services explicitly; main activation remains a separate #92 decision.
const adaptiveRetrievalQualified = false

func configureAdaptiveRetrieval(cfg config.OrchestratorConfig, sources *catalog.Store) (*adaptive.Store, error) {
	if !cfg.MemoryAdaptiveRetrievalEnabled {
		return nil, nil
	}
	if !adaptiveRetrievalQualified || !cfg.MemoryReadEnabled {
		return nil, adaptive.ErrDenied
	}
	return adaptive.NewStore(sources.Pool(), sources)
}
