package app

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// This local entrypoint invokes the existing closed90 consumer unchanged. It
// neither creates a new rubric/harness nor authenticates, opens a database,
// resumes an old window or starts inference. Copied private bytes stay private.
func TestLocal115ClosedHistoryImport(t *testing.T) {
	if os.Getenv("BUTLER_ISSUE115_LOCAL_IMPORT") != "copied-exact-evidence-only" {
		t.Skip("explicit local copied-evidence validation required")
	}
	history := coherentNativeConfiguredHistory(t)
	if history.Hash != "8d9329e13da9e51e620c5fa66d6625ae11c938006b18369d2e09f068f1517b3f" || history.Usage.NativeEntries != 213 || history.Usage.NativeKnownUsage != 212 || history.Usage.NativeUnknownUsage != 1 || history.Usage.KnownFullTokenLowerBound != 1605455 || history.Usage.ProductNULL != 1 {
		t.Fatal("published immutable closed90 population was replaced or lost uncertainty")
	}
	output := os.Getenv("BUTLER_ISSUE115_LOCAL_IMPORT_RECEIPT")
	if !filepath.IsAbs(output) {
		t.Fatal("absolute local receipt path required")
	}
	receipt := map[string]any{
		"version":            "butler115-existing-closed90-consumer-local-v1",
		"result":             "EXACT_CLOSED_HISTORY_CONSUMED_NOT_NATIVE_ACCEPTANCE",
		"closed_leaf_sha256": history.Hash, "historical_carry": history.Carry,
		"historical_usage": history.Usage, "new_native_admissions": 0,
		"auth_sources_constructed": 0, "new_window_opened": false,
		"release_qualified": false, "issue115_closed": false,
	}
	raw, err := json.MarshalIndent(receipt, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(output, append(raw, '\n'), 0600); err != nil {
		t.Fatal("local history receipt unavailable")
	}
	t.Logf("original closed90 consumer imported213 admissions/212 measured/1 unknown, known full tokens1605455; no native admission or qualification")
}
