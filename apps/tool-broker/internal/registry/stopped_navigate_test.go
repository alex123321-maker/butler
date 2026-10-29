package registry

import (
	"github.com/butler/butler/internal/actionauth"
	toolbrokerv1 "github.com/butler/butler/internal/gen/toolbroker/v1"
	"path/filepath"
	"testing"
)

func TestStoppedNavigateRevision3ContractPin(t *testing.T) {
	reg, err := Load(filepath.Join("..", "..", "..", "..", "configs", "tools.json"), "local")
	if err != nil {
		t.Fatal(err)
	}
	c, ok := reg.Get("browser.navigate")
	if !ok {
		t.Fatal("missing navigation")
	}
	c.ContractRevision = "3"
	c.EffectContract = &toolbrokerv1.EffectContract{RetrySafety: "read_only", VerificationMethod: "read_result"}
	h, err := actionauth.ContractHash(c)
	if err != nil || h != "bf151ae84aaff52f3c3c22c0634e9bc7f48b49c551a4b6777d52d5fdf1c429f2" {
		t.Fatalf("old navigation contract pin drifted: %s %v", h, err)
	}
}
