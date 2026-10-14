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
	o, e := normalize(&pb.ClassifyOutput{Scores: completeScores("новое поручение", .51, "уточнение", .49)})
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

func completeScores(best string, score float64, second string, runnerUp float64) []*pb.Score {
	var scores []*pb.Score
	for _, label := range Categories() {
		value := 0.0
		if label == best {
			value = score
		} else if label == second {
			value = runnerUp
		}
		scores = append(scores, &pb.Score{Id: label, Value: value})
	}
	return scores
}

func TestMarginRequiresEveryAlternativeScore(t *testing.T) {
	full := completeScores("уточнение", .9, "изменение условий", .1)
	for size := 2; size < len(full); size++ {
		if _, err := normalize(&pb.ClassifyOutput{Scores: full[:size]}); err == nil {
			t.Fatalf("partial %d-class output can hide a higher alternative", size)
		}
	}
	for _, label := range Categories() {
		o, err := normalize(&pb.ClassifyOutput{Scores: []*pb.Score{{Id: label, Value: 1}}})
		if err != nil || !o.Escalate || !o.MarginUnknown {
			t.Fatal("selected-only score acquired a known margin", o, err)
		}
	}
	o, err := normalize(&pb.ClassifyOutput{Scores: full})
	if err != nil || o.Escalate || o.MarginUnknown || o.Label != "уточнение" {
		t.Fatal("complete separated scores lost their signal", o, err)
	}
}
