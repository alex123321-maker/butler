package registry

import (
	"github.com/butler/butler/internal/functionproposal"
	pb "github.com/butler/butler/internal/gen/localmodels/v1"
	toolpb "github.com/butler/butler/internal/gen/toolbroker/v1"
	"path/filepath"
	"testing"
)

func TestFunctionCandidatePassesOrdinaryBrokerSchemaOnly(t *testing.T) {
	r, e := Load(filepath.Join("..", "..", "..", "..", "configs", "tools.json"), "")
	if e != nil {
		t.Fatal(e)
	}
	for _, call := range []*pb.FunctionCall{{Name: "check_system", ArgumentsJson: `{}`}, {Name: "check_database", ArgumentsJson: `{}`}, {Name: "read_webpage", ArgumentsJson: `{"url":"https://example.org/guide"}`}} {
		name := map[string]string{"check_system": "doctor.check_system", "check_database": "doctor.check_database", "read_webpage": "web.fetch"}[call.Name]
		contract := r.toolsByName[name]
		proposal, e := functionproposal.ToProposal(call, contract)
		if e != nil {
			t.Fatal(e)
		}
		valid, _, failure := r.Validate(&toolpb.ToolCall{ToolName: proposal.ToolName, ArgsJson: proposal.ArgsJSON})
		if !valid || failure != nil {
			t.Fatal("ordinary schema rejected fixed proposal")
		}
		contract.ContractRevision = "replacement"
		if _, e = functionproposal.ToProposal(call, contract); e == nil {
			t.Fatal("new schema implicitly mastered")
		}
	}
}
