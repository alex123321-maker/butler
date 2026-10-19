package actionauth

import (
	"strings"
	"testing"

	"github.com/butler/butler/internal/delegation"
	toolbrokerv1 "github.com/butler/butler/internal/gen/toolbroker/v1"
	"github.com/butler/butler/internal/toolcatalog"
)

func TestToolCatalogStandardReadRequiresExactLocalContractAndClaim(t *testing.T) {
	c := toolcatalog.Contract(toolcatalog.Search)
	call := &toolbrokerv1.ToolCall{RunId: "run", ToolCallId: "search", ToolName: toolcatalog.Search, ArgsJson: `{"query":"http"}`}
	context := ContextSnapshot{RunID: "run", OwnerID: "owner", InputClaimHash: strings.Repeat("a", 64)}
	b, _, err := BuildBinding("action", call, c, context)
	if err != nil || !standardCatalogAction(b) || !delegationAllows(&delegation.Contract{}, b) {
		t.Fatal("fixed read unavailable", err)
	}
	for _, change := range []string{"hash", "resource", "effect", "credential"} {
		forged := b
		switch change {
		case "hash":
			forged.ContractHash = strings.Repeat("b", 64)
		case "resource":
			forged.Resource = "https://external.invalid"
		case "effect":
			forged.Effect = EffectCommit
		case "credential":
			forged.CredentialRefs = []CredentialRef{{Alias: "x", Field: "token"}}
		}
		if standardCatalogAction(forged) || delegationAllows(&delegation.Contract{}, forged) {
			t.Fatal("forged catalog read escaped child scope", change)
		}
	}
	context.InputClaimHash = ""
	if _, _, err = BuildBinding("action", call, c, context); err == nil {
		t.Fatal("selection without input claim accepted")
	}
}

func TestToolCatalogConversationSlotsCannotLinkEffectsAcrossRuns(t *testing.T) {
	a := ActionBinding{ActionID: StableActionID("run-one", "call"), RunID: "run-one", ToolCallID: "call", OwnerID: "owner", Effect: EffectRead}
	b := a
	b.RunID = "run-two"
	b.ActionID = StableActionID(b.RunID, b.ToolCallID)
	first, _, err := EffectIdentity(a, "same-slot")
	if err != nil {
		t.Fatal(err)
	}
	second, _, err := EffectIdentity(b, "same-slot")
	if err != nil {
		t.Fatal(err)
	}
	if first != a.ActionID || second != b.ActionID || first == second {
		t.Fatal("taskless effects linked across runs")
	}
}
