package app

import (
	"github.com/butler/butler/internal/config"
	"testing"
)

func TestAdaptiveUnqualifiedCannotBeEnabledByOperatorFlag(t *testing.T) {
	if adaptiveRetrievalQualified {
		t.Fatal("update this gate test only with frozen qualification evidence")
	}
	if store, err := configureAdaptiveRetrieval(config.OrchestratorConfig{}, nil); store != nil || err != nil {
		t.Fatal(store, err)
	}
	if store, err := configureAdaptiveRetrieval(config.OrchestratorConfig{MemoryReadEnabled: true, MemoryAdaptiveRetrievalEnabled: true}, nil); store != nil || err == nil {
		t.Fatal("unqualified enabled", err)
	}
}
