package workspace

import (
	"encoding/json"
	"testing"
)

func TestModelJSONCannotSupplyPrivateWorkspaceAuthority(t *testing.T) {
	var proof LeaseProof
	if err := json.Unmarshal([]byte(`{"LeaseID":"forged","ResourceID":"workspace:forged","RunID":"run","TaskID":"task","FenceEpoch":1}`), &proof); err != nil {
		t.Fatal(err)
	}
	if proof != (LeaseProof{}) {
		t.Fatal("model populated private authority")
	}
	encoded, err := json.Marshal(LeaseProof{LeaseID: "private", RunID: "private", ResourceID: "private", FenceEpoch: 4})
	if err != nil || string(encoded) != "{}" {
		t.Fatal("private lease proof leaked", err)
	}
	ref, err := Allocate(testBinding("owner", "session", "task"))
	if err != nil {
		t.Fatal(err)
	}
	encoded, err = json.Marshal(ref)
	var fields map[string]any
	if err != nil || json.Unmarshal(encoded, &fields) != nil || len(fields) != 1 || fields["workspace_id"] != ref.ID {
		t.Fatal("namespace leaked binding or path", err)
	}
}
