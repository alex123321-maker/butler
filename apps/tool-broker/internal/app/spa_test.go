package app

import (
	"context"
	"testing"

	"github.com/butler/butler/internal/config"
)

func TestSPACompositionDefaultOffAndMissingGuards(t *testing.T) {
	store, handler, err := configureServerSPA(context.Background(), config.ToolBrokerConfig{}, nil, nil, nil, nil, nil)
	if err != nil || store != nil || handler != nil {
		t.Fatal("disabled SPA exposed composition")
	}
	if _, _, err = configureServerSPA(context.Background(), config.ToolBrokerConfig{BrowserSPAEnabled: true}, nil, nil, nil, nil, nil); err == nil {
		t.Fatal("unguarded SPA enabled")
	}
}
