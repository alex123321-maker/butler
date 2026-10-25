package main

import (
	"github.com/butler/butler/internal/providerauth"
	"github.com/butler/butler/internal/transport/antigravity"
	"testing"
)

func TestFrozenMatrixHasEqualProfilesAndConstructibleSchema(t *testing.T) {
	f := newManifest("offline-fixture", modelInventory{})
	if len(f.Cases) != 12 || len(f.Slots) != 96 {
		t.Fatal(len(f.Cases), len(f.Slots))
	}
	seen := map[string]bool{}
	counts := map[string]int{}
	for _, s := range f.Slots {
		if seen[s.ID()] {
			t.Fatal("duplicate slot")
		}
		seen[s.ID()] = true
		counts[s.Profile]++
	}
	for _, profile := range []string{antigravity.FlashLowModel, antigravity.FlashHighModel} {
		if counts[profile] != 48 {
			t.Fatal(counts)
		}
		if _, e := antigravity.NewProvider(antigravity.Config{Model: profile, AuthSource: providerauth.NewManager(nil), MaximumOutputTokens: f.OutputTokenCap, ThinkingLevel: f.ThinkingLevels[profile], GoogleResponseSchema: f.ResponseSchema}); e != nil {
			t.Fatal(profile, e)
		}
	}
}
