package app

import "testing"

func TestReleaseChainV17CaseFailureContinuation(t *testing.T) {
	if !releaseChainHighStopsAfterCase(releaseChainHighEntry{}, true) {
		t.Fatal("legacy failure no longer seals loop")
	}
	if releaseChainHighStopsAfterCase(releaseChainHighEntry{ContinueAfterCaseFailure: true}, true) {
		t.Fatal("independent remaining faults skipped after a failed case")
	}
	for _, entry := range []releaseChainHighEntry{{}, {ContinueAfterCaseFailure: true}} {
		if releaseChainHighStopsAfterCase(entry, false) {
			t.Fatal("healthy case stopped remaining faults")
		}
	}
}
