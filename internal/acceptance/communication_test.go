package acceptance

import "testing"

func TestCommunicationBehaviorSeparatesStructuralAndSemanticResults(t *testing.T) {
	measured := 3
	valid := CommunicationReview{CaseID: "synthetic", Expected: "result", ContentVerdict: "substantive_result", EvidenceRefs: []string{"fixture:kernel-trace", "fixture:content-review"}, ResultPublications: 1, Obligation: "response_committed", InputStatus: "completed", ModelAttempts: &measured, ObservedAttempts: &measured}
	for _, verdict := range []string{"substantive_result", "premature_promise", "false_result", "", "model_claimed_success"} {
		t.Run(verdict, func(t *testing.T) {
			r := valid
			r.ContentVerdict = verdict
			if (ReviewCommunication(r) == nil) != (verdict == "substantive_result") {
				t.Fatal("structural publication promoted to semantic pass", verdict)
			}
		})
	}
	for _, scenario := range []string{"repeat", "duplicate", "missing_usage", "unobserved", "no_evidence"} {
		t.Run(scenario, func(t *testing.T) {
			r := valid
			switch scenario {
			case "repeat":
				r.RepeatedQuestions = 1
			case "duplicate":
				r.ResultPublications = 2
			case "missing_usage":
				r.ModelAttempts = nil
			case "unobserved":
				zero := 0
				r.ObservedAttempts = &zero
			case "no_evidence":
				r.EvidenceRefs = nil
			}
			if ReviewCommunication(r) == nil {
				t.Fatal("unqualified observation passed", scenario)
			}
		})
	}
	progress := valid
	progress.Expected = "progress"
	progress.ContentVerdict = "useful_progress"
	progress.ResultPublications = 0
	progress.ProgressPublications = 1
	progress.Obligation = "open"
	progress.InputStatus = "running"
	if ReviewCommunication(progress) != nil {
		t.Fatal("useful progress rejected")
	}
	progress.InputStatus = "completed"
	if ReviewCommunication(progress) == nil {
		t.Fatal("progress completed request")
	}
	quiet := valid
	quiet.Expected = "quiet"
	quiet.ContentVerdict = "quiet"
	quiet.ResultPublications = 0
	if ReviewCommunication(quiet) != nil {
		t.Fatal("quiet run rejected")
	}
	quiet.ProgressPublications = 1
	if ReviewCommunication(quiet) == nil {
		t.Fatal("quiet notification accepted")
	}
}
