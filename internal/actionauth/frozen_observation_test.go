package actionauth

import (
	"testing"

	toolbrokerv1 "github.com/butler/butler/internal/gen/toolbroker/v1"
)

func TestFrozenObservationIsOnlyItsOwnRead(t *testing.T) {
	if classifyEffect("browser.freeze_snapshot", nil) != EffectRead {
		t.Fatal("frozen observation is not a read")
	}
	for _, change := range []string{"none", "revision", "policy", "credentials", "contract_credentials"} {
		t.Run(change, func(t *testing.T) {
			b := ActionBinding{ToolName: "browser.freeze_snapshot", ContractRevision: "1", Effect: EffectRead}
			c := &toolbrokerv1.ToolContract{ToolName: b.ToolName, EffectContract: &toolbrokerv1.EffectContract{RetrySafety: "read_only", VerificationMethod: "read_result"}}
			switch change {
			case "revision":
				b.ContractRevision = "0"
			case "policy":
				c.EffectContract.VerificationMethod = "none"
			case "credentials":
				b.CredentialRefs = []CredentialRef{{Alias: "private", Field: "password"}}
			case "contract_credentials":
				c.SupportsCredentialRefs = true
			}
			_, err := ResolveEffectPolicy(b, c)
			if (err == nil) != (change == "none") {
				t.Fatal("incompatible frozen-observation policy", err)
			}
		})
	}
}
