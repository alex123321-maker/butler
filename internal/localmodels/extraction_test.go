package localmodels

import (
	pb "github.com/butler/butler/internal/gen/localmodels/v1"
	"testing"
)

func TestStructuredExtractionPreservesUnicodeAndRejectsFabricatedFields(t *testing.T) {
	text := "🛠 Не обновляй сервер"
	field := &pb.Span{Type: "action", Text: "обновляй", Start: 5, End: 13, Confidence: .8}
	out := &pb.ExtractOutput{SchemaVersion: MemorySchema, Records: []*pb.ExtractRecord{{Type: "constraint", Fields: []*pb.Span{field}, UnresolvedFields: []string{"resource"}}}}
	if e := ValidateMemoryExtraction(text, out); e != nil {
		t.Fatal(e)
	}
	field.Start = 4
	if ValidateMemoryExtraction(text, out) == nil {
		t.Fatal("fabricated position accepted")
	}
	field.Start = 5
	field.Text = "обнови"
	if ValidateMemoryExtraction(text, out) == nil {
		t.Fatal("generated text accepted")
	}
	field.Text = "обновляй"
	out.Records[0].UnresolvedFields = []string{"permission"}
	if ValidateMemoryExtraction(text, out) == nil {
		t.Fatal("arbitrary schema field accepted")
	}
}
func TestStructuredSchemaRejectsCallerLabels(t *testing.T) {
	p := &pb.ModelProfile{MaxBatch: 1, MaxInputBytes: 2048, Capabilities: []pb.Capability{pb.Capability_CAPABILITY_EXTRACT}}
	in := &pb.ExtractInput{Text: "text", SchemaVersion: MemorySchema}
	r := &pb.InferRequest{RequestId: "id", ProfileId: "profile", Input: &pb.InferRequest_Extract{Extract: in}}
	if validateRequest(p, r) != nil {
		t.Fatal("fixed schema denied")
	}
	in.EntityTypes = []string{"grant_permission"}
	if validateRequest(p, r) == nil {
		t.Fatal("model-selected schema accepted")
	}
}
