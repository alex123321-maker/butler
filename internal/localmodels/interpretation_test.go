package localmodels

import (
	"context"
	"testing"

	pb "github.com/butler/butler/internal/gen/localmodels/v1"
)

func TestInterpretationProfileLabelsDenyBeforeWorker(t *testing.T) {
	for _, variant := range []string{"missing", "foreign", "duplicate"} {
		t.Run(variant, func(t *testing.T) {
			labels := InterpretationLabels()
			switch variant {
			case "missing":
				labels = labels[:15]
			case "foreign":
				labels[0] = "permission"
			case "duplicate":
				labels[0] = labels[1]
			}
			b := &Process{}
			p := &pb.ModelProfile{Model: "fastino/gliner2.5-multi-v1", PreprocessingVersion: InterpretationSchemaV3}
			r := &pb.InferRequest{Input: &pb.InferRequest_Classify{Classify: &pb.ClassifyInput{Text: "event", Labels: labels}}}
			if _, err := b.extract(context.Background(), p, r, "cpu"); !isFault(err, Invalid) || b.cmd != nil {
				t.Fatal("unknown interpretation labels started worker", err)
			}
		})
	}
}
