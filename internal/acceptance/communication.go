package acceptance

import "fmt"

// CommunicationReview combines kernel observations with an explicit content
// review. It does not infer semantic correctness from a purpose tag, a tool ACK,
// a model's self-assessment, or a lexical match against the public response.
// Evidence refs identify retained traces/review records, never credentials/body.
type CommunicationReview struct {
	CaseID, Expected, ContentVerdict                            string
	EvidenceRefs                                                []string
	ResultPublications, ProgressPublications, RepeatedQuestions int
	Obligation, InputStatus                                     string
	// Nil measurements remain unknown and cannot qualify an acceptance result.
	ModelAttempts, ObservedAttempts *int
}

func ReviewCommunication(r CommunicationReview) error {
	if r.CaseID == "" || len(r.EvidenceRefs) == 0 || r.ModelAttempts == nil || r.ObservedAttempts == nil || *r.ModelAttempts < 0 || *r.ObservedAttempts != *r.ModelAttempts || r.ResultPublications < 0 || r.ProgressPublications < 0 || r.RepeatedQuestions < 0 {
		return fmt.Errorf("incomplete communication evidence")
	}
	if r.RepeatedQuestions != 0 {
		return fmt.Errorf("behavioral failure: repeated answered question")
	}
	switch r.Expected {
	case "result":
		if r.ContentVerdict != "substantive_result" || r.ResultPublications != 1 || r.Obligation != "response_committed" || r.InputStatus != "completed" {
			return fmt.Errorf("behavioral failure: result is missing, premature or unsubstantiated")
		}
	case "progress":
		if r.ContentVerdict != "useful_progress" || r.ProgressPublications < 1 || r.ResultPublications != 0 || r.Obligation != "open" || r.InputStatus == "completed" {
			return fmt.Errorf("behavioral failure: progress incorrectly completed the request")
		}
	case "quiet":
		if r.ContentVerdict != "quiet" || r.ResultPublications != 0 || r.ProgressPublications != 0 || r.Obligation != "response_committed" || r.InputStatus != "completed" {
			return fmt.Errorf("behavioral failure: quiet reporting policy violated")
		}
	default:
		return fmt.Errorf("unknown communication review case")
	}
	return nil
}
