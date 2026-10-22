package acceptance

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"testing"
)

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

// An operator supplies separate content reviews after inspecting retained native
// transcripts. The counters in this file must come from the kernel ledger; this
// gate makes no model call and does not turn the reviewer into an authority.
func TestNativeCommunicationRetainedContentReviews(t *testing.T) {
	path := os.Getenv("BUTLER_COMMUNICATION_REVIEW_FILE")
	if path == "" {
		t.Skip("explicit retained native reviews required")
	}
	if path != "/private/communication-review.json" {
		t.Fatal("fixed private review path required")
	}
	raw, err := os.ReadFile(path)
	if err != nil || len(raw) > 64*1024 {
		t.Fatal("bounded review evidence unavailable")
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	d.DisallowUnknownFields()
	var rows []CommunicationReview
	if d.Decode(&rows) != nil || len(rows) == 0 || len(rows) > 32 {
		t.Fatal("review evidence invalid")
	}
	var tail any
	if d.Decode(&tail) != io.EOF {
		t.Fatal("review evidence trailing data")
	}
	seen := map[string]bool{}
	for _, r := range rows {
		if seen[r.CaseID] {
			t.Fatal("duplicate review case")
		}
		seen[r.CaseID] = true
		t.Run(r.CaseID, func(t *testing.T) {
			if err := ReviewCommunication(r); err != nil {
				t.Error(err)
			}
		})
	}
}
