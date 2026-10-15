package acceptance

import (
	"encoding/json"
	"errors"
	"fmt"
)

// MemorySemanticSlot is an entry in the predeclared experiment matrix. It is
// built from the corpus, not from whichever observations survived collection.
type MemorySemanticSlot struct {
	CaseID string
	Arm    string
}

func ValidateMemorySemanticSchedule(inputs []MemoryInput, expected []MemorySemanticSlot) error {
	if len(expected) == 0 || len(inputs) != len(expected) {
		return errors.New("complete nonempty semantic schedule required")
	}
	want := make(map[MemorySemanticSlot]bool, len(expected))
	for _, slot := range expected {
		if slot.CaseID == "" || slot.Arm == "" || want[slot] {
			return errors.New("unique semantic schedule entries required")
		}
		want[slot] = true
	}
	for _, input := range inputs {
		slot := MemorySemanticSlot{input.CaseID, input.Arm}
		if !want[slot] {
			return fmt.Errorf("foreign or duplicate semantic schedule entry: %s/%s", slot.CaseID, slot.Arm)
		}
		delete(want, slot)
	}
	return nil
}

type semanticCitationIdentity struct {
	SourceID string `json:"source_id"`
	Revision int64  `json:"revision"`
	Quote    string `json:"quote"`
}

// Invalid model syntax remains an original semantic observation. It cannot
// silently acquire a successful citation assessment. For parseable citations,
// require an exhaustive multiset of checks, including duplicate/bad citations.
func validateSemanticCitationCoverage(original []byte, checks []MemorySemanticCitation) (bool, error) {
	var output struct {
		Citations []semanticCitationIdentity `json:"citations"`
	}
	if json.Unmarshal(original, &output) != nil {
		if len(checks) != 0 {
			return false, errors.New("citation checks cannot bind to malformed original")
		}
		return false, nil
	}
	if len(output.Citations) != len(checks) {
		return true, errors.New("every original model citation needs exactly one check")
	}
	counts := map[semanticCitationIdentity]int{}
	for _, citation := range output.Citations {
		counts[citation]++
	}
	for _, check := range checks {
		citation := semanticCitationIdentity{check.SourceID, check.Revision, check.Quote}
		if counts[citation] == 0 {
			return true, errors.New("citation check differs from original model citation")
		}
		counts[citation]--
	}
	return true, nil
}
