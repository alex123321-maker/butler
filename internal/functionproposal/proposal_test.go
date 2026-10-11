package functionproposal

import (
	pb "github.com/butler/butler/internal/gen/localmodels/v1"
	toolpb "github.com/butler/butler/internal/gen/toolbroker/v1"
	"testing"
)

func TestCurrentBrokerSchemaCannotBeInvented(t *testing.T) {
	c := &toolpb.ToolContract{ToolName: "doctor.check_system", ContractRevision: "1", InputSchemaJson: `{"additionalProperties":false,"type":"object"}`, Status: "active"}
	in := &pb.FunctionCall{Name: "check_system", ArgumentsJson: `{}`}
	proposal, e := ToProposal(in, c)
	if e != nil || proposal.ToolName != c.ToolName {
		t.Fatal(e)
	}
	c.ContractRevision = "2"
	if _, e = ToProposal(in, c); e == nil {
		t.Fatal("stale schema accepted")
	}
	c.ContractRevision = "1"
	c.InputSchemaJson = `{"type":"object"}`
	if _, e = ToProposal(in, c); e == nil {
		t.Fatal("schema replacement accepted")
	}
	c.InputSchemaJson = `{"additionalProperties":false,"type":"object"}`
	c.Extension = &toolpb.ExtensionContract{ExtensionId: "model-created"}
	if _, e = ToProposal(in, c); e == nil {
		t.Fatal("new tool automatically mastered")
	}
}
