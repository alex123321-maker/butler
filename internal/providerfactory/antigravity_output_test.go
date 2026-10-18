package providerfactory

import (
	"context"
	"testing"

	"github.com/butler/butler/internal/providerauth"
	"github.com/butler/butler/internal/transport/antigravity"
)

func TestAntigravityTrustedOutputCeilingReachesNativeAdapter(t *testing.T) {
	for _, tc := range []struct {
		name, model string
		ceiling     int
		want        int64
		bad         bool
	}{
		{"unchanged_default", antigravity.DefaultModel, 0, 8192, false},
		{"explicit_high", antigravity.FlashHighModel, 32768, 32768, false},
		{"tiered_denied", antigravity.DefaultModel, 32768, 0, true},
		{"too_large", antigravity.FlashHighModel, 32769, 0, true},
		{"negative", antigravity.FlashHighModel, -1, 0, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// Construction never resolves credentials or performs HTTP.
			p, err := New(providerauth.NewManager(nil), nil).Build(context.Background(), BuildConfig{ActiveProvider: antigravity.Name, AntigravityModel: tc.model, AntigravityMaximumOutputTokens: tc.ceiling})
			if tc.bad {
				if err == nil {
					t.Fatal("invalid profile accepted")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			_, output := p.Provider.(interface{ InferenceLimits() (int64, int64) }).InferenceLimits()
			if output != tc.want {
				t.Fatal("native ceiling lost", output)
			}
		})
	}
}
