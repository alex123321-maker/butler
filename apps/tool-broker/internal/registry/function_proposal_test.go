package registry

import (
	"errors"
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
		if call.Name == "check_system" {
			// Doctor v2 added a current boundary; the frozen model mastered v1.
			if contract.ContractRevision != "2" || !errors.Is(e, functionproposal.ErrDenied) {
				t.Fatal("changed doctor contract implicitly mastered", e)
			}
			continue
		}
		if call.Name == "read_webpage" {
			// The frozen local catalog mastered v1, not bounded reader v3.
			if contract.ContractRevision != "3" || !errors.Is(e, functionproposal.ErrDenied) {
				t.Fatal("changed reader contract implicitly mastered", e)
			}
			continue
		}
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
