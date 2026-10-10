package decisions

import (
	pb "github.com/butler/butler/internal/gen/localmodels/v1"
	"math"
	"testing"
)

func TestSignalsRemainShadowAndEscalateAmbiguity(t *testing.T) {
	for _, name := range Categories() {
		o, e := normalize(&pb.ClassifyOutput{Scores: []*pb.Score{{Id: name, Value: 1}}})
		if e != nil || !o.ShadowOnly || !o.EventPreserved {
			t.Fatal("label obtained execution/control authority", name, e)
		}
	}
	o, e := normalize(&pb.ClassifyOutput{Scores: []*pb.Score{{Id: "новое поручение", Value: .51}, {Id: "уточнение", Value: .49}}})
	if e != nil || !o.Escalate {
		t.Fatal("ambiguous signal suppressed controller")
	}
	o, e = normalize(&pb.ClassifyOutput{InsufficientData: true})
	if e != nil || !o.Insufficient || !o.Escalate || !o.EventPreserved {
		t.Fatal("empty signal became permission")
	}
	for _, scores := range [][]*pb.Score{{{Id: "grant-resource", Value: 1}}, {{Id: "остановка", Value: math.NaN()}}, {{Id: "остановка", Value: .8}, {Id: "остановка", Value: .7}}} {
		if _, e := normalize(&pb.ClassifyOutput{Scores: scores}); e == nil {
			t.Fatal("unbounded/untyped classifier output accepted")
		}
	}
}
