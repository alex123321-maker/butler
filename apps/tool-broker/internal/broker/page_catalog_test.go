package broker

import (
	"context"
	"testing"

	"github.com/butler/butler/apps/tool-broker/internal/registry"
	"github.com/butler/butler/internal/browserpage"
)

func TestPageCatalogRequiresCompositionAndExactContracts(t *testing.T) {
	reg, err := registry.Load("../../../../configs/tools.json", "tool-browser:9090")
	if err != nil {
		t.Fatal(err)
	}
	authority, _ := newAuthFixture().GetActionContext(context.Background(), "run")
	for _, ready := range []bool{false, true} {
		catalog := &TaskCatalog{Registry: reg, Contexts: converterCatalogContext{authority}, PageToolsReady: ready}
		contracts, err := catalog.ListForRun(context.Background(), "run", "", 0)
		if err != nil {
			t.Fatal(err)
		}
		found := 0
		for _, c := range contracts {
			if browserpage.IsTool(c.ToolName) {
				found++
				if !ready || !browserpage.ValidContract(c) {
					t.Fatal("page contract exposed without exact composition")
				}
			}
		}
		if ready && found != 7 || !ready && found != 0 {
			t.Fatalf("unexpected page catalog size: ready=%v found=%d", ready, found)
		}
	}
}
