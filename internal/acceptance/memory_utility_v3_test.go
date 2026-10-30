package acceptance

import (
	"encoding/json"
	"testing"
)

func TestMemoryUtilityV3SufficientSupportAndFrozenLegacyExactSets(t *testing.T) {
	for _, index := range []int{4, 15} { // current instruction; historical OR three current documents
		suite := utilitySuite(t)
		c := suite.Cases[index]
		o := utilityObservation(suite, c, "B")
		var final UtilityFinal
		_ = json.Unmarshal(o.Final, &final)
		cited := map[string]bool{}
		for _, ref := range final.Sources {
			cited[ref.ID] = true
		}
		for _, source := range c.Sources {
			if c.SourceAllowed(source, "B") && !cited[source.Source.ID] {
				final.Sources = append(final.Sources, source.Ref())
				o.Reads = append(o.Reads, UtilityRead{source.Ref(), "opened"})
			}
		}
		o.Final, _ = json.Marshal(final)
		if result := EvaluateMemoryUtility(suite, o); !result.TaskSuccess {
			t.Fatal("complete support plus exact opened citations rejected", index, result)
		}
		for _, legacy := range []string{"butler-memory-utility-v1", "butler-memory-utility-v2"} {
			old := suite
			old.Version = legacy
			previous := o
			previous.SuiteSHA256 = MemoryDigest(old)
			if result := EvaluateMemoryUtility(old, previous); result.TaskSuccess || len(result.Reasons) != 1 || result.Reasons[0] != "unsupported_answer" {
				t.Fatal("frozen exact support metric changed", legacy, result)
			}
		}
		// An added source still must actually have been opened, not just found.
		o.Reads = o.Reads[:len(o.Reads)-1]
		if EvaluateMemoryUtility(suite, o).TaskSuccess {
			t.Fatal("additional unread citation accepted")
		}
	}
	old := utilitySuite(t)
	old.Version = "butler-memory-utility-v2"
	c := old.Cases[23]
	if !EvaluateMemoryUtility(old, utilityObservation(old, c, "A")).TaskSuccess {
		t.Fatal("v2 current-receipt artifact semantics changed")
	}
}
