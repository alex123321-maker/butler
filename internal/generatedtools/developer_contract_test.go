package generatedtools

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/butler/butler/internal/actionauth"
	"github.com/butler/butler/internal/budget"
	"github.com/butler/butler/internal/memory/catalog"
)

func TestDeveloperProposalContainsOnlySelectedRefsAndFiniteAllocation(t *testing.T) {
	need := catalog.SourceRef{ID: "need", Revision: 1, AccessRevision: 1, Hash: strings.Repeat("a", 64)}
	contract := need
	contract.ID = "synthetic-contract"
	args := DeveloperArgs{NeedSource: need, ContractSource: contract, Budget: budget.Units{ModelTurns: 2, Tokens: 4096, ExternalCalls: 3, ActiveMillis: 60000, EgressBytes: 4096, ProviderInputBytes: 4096}, Deadline: time.Now().UTC().Truncate(time.Microsecond).Add(time.Hour), Dependencies: []string{}}
	raw, err := json.Marshal(args)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := DecodeDeveloperArgs(string(raw))
	if err != nil || decoded.NeedSource != need || decoded.ContractSource != contract || decoded.Budget != args.Budget || !decoded.Deadline.Equal(args.Deadline) {
		t.Fatal("valid exact proposal rejected", err)
	}
	for _, field := range []string{"permissions", "converter_permission", "owner_id", "task_id", "body", "code", "input_ref", "credentials", "source_refs"} {
		bad := strings.TrimSuffix(string(raw), "}") + `,"` + field + `":"untrusted"}`
		if _, err := DecodeDeveloperArgs(bad); err == nil {
			t.Fatalf("model field %s entered private developer capability", field)
		}
	}
	for _, bad := range []string{
		strings.TrimSuffix(string(raw), "}") + `,"need_source":` + string(mustDeveloperJSON(need)) + `}`,
		strings.Replace(string(raw), `"children":0,`, "", 1),
		strings.Replace(string(raw), `"children":0`, `"children":null`, 1),
		strings.Replace(string(raw), `"model_turns":2`, `"model_turns":-1`, 1),
		strings.Replace(string(raw), `"dependencies":[]`, `"dependencies":["same","same"]`, 1),
		strings.Replace(string(raw), `"dependencies":[]`, `"dependencies":null`, 1),
		string(raw) + `{}`,
	} {
		if _, err := DecodeDeveloperArgs(bad); err == nil {
			t.Fatal("ambiguous or unbounded proposal admitted")
		}
	}
	if _, err := actionauth.ContractHash(DeveloperAdmissionContract()); err != nil {
		t.Fatal(err)
	}
	// Merely knowing this reviewed contract does not change the six converter
	// lifecycle contracts or register a generic child-capability request.
	for _, c := range Contracts() {
		if c.ToolName == DeveloperAdmissionTool {
			t.Fatal("developer admission enabled implicitly")
		}
	}
}

func mustDeveloperJSON(v any) []byte { raw, _ := json.Marshal(v); return raw }
