package generatedtools

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestObservationSurvivesBrokerPrivacyNormalization(t *testing.T) {
	values := []any{
		map[string]any{"need_id": "need", "need_hash": strings.Repeat("a", 64), "discovery": Discovery{Tools: []Capability{{Name: "converters.discover", Revision: "1", ContractHash: strings.Repeat("b", 64)}}, Skills: []SkillIdentity{}, Compatible: []string{}}, "instruction_authority": false},
		Report{PackageHash: strings.Repeat("a", 64), NeedHash: strings.Repeat("b", 64), ProfileHash: strings.Repeat("c", 64), Checker: Checker, Passed: true},
	}
	for _, value := range values {
		wire, err := observationJSON(value)
		if err != nil {
			t.Fatal(err)
		}
		// The real Broker uses a decoded object at its mandatory privacy boundary.
		// Independently re-encoding it must preserve the journal's exact bytes.
		var object any
		if json.Unmarshal([]byte(wire), &object) != nil {
			t.Fatal("invalid observation")
		}
		actual, err := json.Marshal(object)
		expected, hashErr := observationHash(value)
		if err != nil || hashErr != nil || expected != HashBytes(actual) || wire != string(actual) {
			t.Fatal("typed observation changed its proof digest at the Broker boundary")
		}
	}
}
