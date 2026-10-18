package experience

import (
	"context"
	"strings"
	"testing"

	"github.com/butler/butler/internal/actionauth"
	toolbrokerv1 "github.com/butler/butler/internal/gen/toolbroker/v1"
	"github.com/butler/butler/internal/skills"
)

func TestPrepareRetainsEnabledBrokerContractWithoutNormalizingIdentity(t *testing.T) {
	hashes := map[string]string{}
	for _, status := range []string{"active", "enabled", "disabled", "", "unavailable"} {
		t.Run("status_"+status, func(t *testing.T) {
			contract := &toolbrokerv1.ToolContract{ToolName: "extension.project.inspect", ToolClass: "extension", ContractRevision: "reviewed-v1:1", Status: status, RuntimeTarget: "extension:50051", InputSchemaJson: `{"type":"object"}`, OutputSchemaJson: `{"type":"object"}`}
			store := &Store{EnvironmentHash: strings.Repeat("a", 64), Tools: toolsFixture{contract}}
			prepared, err := store.Prepare(context.Background(), "current-owning-run")
			if err != nil {
				t.Fatal(err)
			}
			dep, present := prepared.tools[contract.ToolName]
			if status != "active" && status != "enabled" {
				if present {
					t.Fatal("unavailable contract became a current lesson dependency")
				}
				return
			}
			hash, err := actionauth.ContractHash(contract)
			if err != nil || !present || dep != (skills.Dependency{Tool: contract.ToolName, ContractHash: hash, ContractRevision: contract.ContractRevision}) {
				t.Fatal("current Broker contract status lost its exact dependency identity", err)
			}
			hashes[status] = hash
		})
	}
	if hashes["active"] == "" || hashes["enabled"] == "" || hashes["active"] == hashes["enabled"] {
		t.Fatal("status was normalized instead of preserving the Broker's exact hash")
	}
}
