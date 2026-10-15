package jobs

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/butler/butler/internal/memory/catalog"
)

func TestNilModelContextsPreserveLegacyPolicyAndSourceFingerprint(t *testing.T) {
	refs := []catalog.SourceRef{{ID: "source", Revision: 1, AccessRevision: 1, Hash: strings.Repeat("a", 64)}}
	_, old, err := canonical(refs)
	if err != nil {
		t.Fatal(err)
	}
	for _, pins := range [][]ModelContextPin{nil, {}} {
		got, err := modelFingerprint(refs, pins)
		if err != nil || got != old {
			t.Fatal("legacy fingerprint changed", got, err)
		}
	}
	raw, _ := json.Marshal(ModelPolicy{})
	if strings.Contains(string(raw), "context") {
		t.Fatal("legacy immutable policy gained a field", string(raw))
	}
	if string(contextManifestJSON(nil)) != "[]" {
		t.Fatal("invalid legacy PG manifest")
	}
	pins := []ModelContextPin{{ModelContextRef: ModelContextRef{SessionKey: "s", OriginalRunID: "r"}, OwnerID: "owner", Privacy: "cloud_allowed", QuietSeconds: 60}}
	one, err := modelFingerprint(refs, pins)
	if err != nil || one == old {
		t.Fatal("context not fingerprinted", err)
	}
	pins[0].ConstraintsRevision++
	two, err := modelFingerprint(refs, pins)
	if err != nil || one == two {
		t.Fatal("changed context not fingerprinted", err)
	}
}
