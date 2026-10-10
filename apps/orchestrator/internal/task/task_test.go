package task

import (
	"errors"
	"testing"
)

func TestCompletionRequiresCurrentOwnerEvidenceForEveryCriterion(t *testing.T) {
	record := Record{Revision: 4, CompletionCriteria: []string{"review approved", "message delivered"}}
	valid := Result{TaskRevision: 4, Summary: "Approved and delivered", EvidenceRefs: []string{"approval:1", "delivery:receipt:2"}, VerifiedCriteria: record.CompletionCriteria, Verifier: "owner"}
	if err := ValidateVerification(record, valid); err != nil {
		t.Fatal(err)
	}
	for name, change := range map[string]func(*Result){
		"stale revision": func(r *Result) { r.TaskRevision = 3 }, "no evidence": func(r *Result) { r.EvidenceRefs = nil },
		"model verdict": func(r *Result) { r.Verifier = "model" }, "draft only": func(r *Result) { r.VerifiedCriteria = []string{"review approved"} },
		"invented criterion": func(r *Result) { r.VerifiedCriteria = []string{"review approved", "sounds finished"} },
	} {
		t.Run(name, func(t *testing.T) {
			copy := valid
			change(&copy)
			if !errors.Is(ValidateVerification(record, copy), ErrInvalid) {
				t.Fatal("invalid completion was accepted")
			}
		})
	}
}

func TestTaskStatusDoesNotUseRunStates(t *testing.T) {
	for _, status := range []string{"waiting_for_user", "blocked", "paused", "cancelled", "completed", "active"} {
		from := "active"
		if status == from {
			from = "paused"
		}
		if err := ValidateTransition(from, status); err != nil {
			t.Fatalf("%s: %v", status, err)
		}
	}
	for _, from := range []string{"cancelled", "completed"} {
		if !errors.Is(ValidateTransition(from, "active"), ErrConflict) {
			t.Fatal("terminal task reopened")
		}
	}
	if !errors.Is(ValidateTransition("active", "model_running"), ErrInvalid) {
		t.Fatal("run state accepted as task state")
	}
}
