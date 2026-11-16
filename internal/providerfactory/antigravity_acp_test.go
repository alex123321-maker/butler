package providerfactory

import (
	"context"
	"testing"
	"time"

	"github.com/butler/butler/internal/modelprovider"
	"github.com/butler/butler/internal/transport/antigravityacp"
)

func TestACPRequiresDedicatedOptInAndRuntime(test *testing.T) {
	factory := New(nil, nil)
	for _, enabled := range []bool{false, true} {
		result, err := factory.Build(context.Background(), BuildConfig{
			ActiveProvider: modelprovider.ProviderAntigravityACP,
			OpenAIAPIKey:   "must-not-fallback",
			AntigravityACP: antigravityacp.Config{Enabled: enabled, Model: "gemini-fixture", Timeout: time.Second},
		})
		if err == nil || result.Provider != nil {
			test.Fatal("ACP started without dedicated runtime or fell back")
		}
	}
}
