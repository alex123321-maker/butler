package app

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestReleaseChainV17PendingFaultOrder(t *testing.T) {
	fixed := []string{"owner_stop", "explicit_absence", "lost_ack", "restart_sent"}
	got, err := releaseChainHighDeclaredScenarios(releaseChainHighEntry{ScenarioNames: fixed, PreserveInvalidOriginalEvidence: true})
	if err != nil || !reflect.DeepEqual(got, fixed) {
		t.Fatal("fresh fault order differs")
	}
	got[0] = "normal_sent"
	if fixed[0] != "owner_stop" {
		t.Fatal("declaration was aliased")
	}
	for _, bad := range [][]string{{"owner_stop", "explicit_absence", "lost_ack"}, {"normal_sent", "explicit_absence", "lost_ack", "restart_sent"}, {"restart_sent", "explicit_absence", "lost_ack", "owner_stop"}} {
		if _, err = releaseChainHighDeclaredScenarios(releaseChainHighEntry{ScenarioNames: bad, PreserveInvalidOriginalEvidence: true}); err == nil {
			t.Fatal("unreviewed subset/order accepted")
		}
	}
	if _, err = releaseChainHighDeclaredScenarios(releaseChainHighEntry{ScenarioNames: fixed}); err == nil {
		t.Fatal("old entry acquired future ordering")
	}
	legacy := []string{"restart_sent", "explicit_absence", "lost_ack", "owner_stop"}
	if got, err = releaseChainHighDeclaredScenarios(releaseChainHighEntry{ScenarioNames: legacy}); err != nil || !reflect.DeepEqual(got, legacy) {
		t.Fatal("sealed V16 declaration changed")
	}
	if got, err = releaseChainHighDeclaredScenarios(releaseChainHighEntry{PreserveInvalidOriginalEvidence: true}); err != nil || !reflect.DeepEqual(got, releaseChainV5Scenarios()) {
		t.Fatal("legacy five-case default changed")
	}
}

func TestReleaseChainV17InvalidOriginalStillRetainsOwningEvidenceIntegration(t *testing.T) {
	for _, preserve := range []bool{true, false} {
		name := "future_retained"
		if !preserve {
			name = "legacy_denied"
		}
		t.Run(name, func(t *testing.T) {
			f := newDeveloperFixture(t)
			f.channel = &releaseChainChannel{}
			f.projectDirectory = t.TempDir()
			f.modelConfig = &releaseChainModelConfig{PreserveInvalidOriginalEvidence: preserve, AllowRepeatedPatchChecks: true}
			// A real unusable stored artifact exercises capture failure; this is
			// independent of a provider, semantic verdict or fabricated receipt.
			if err := os.WriteFile(filepath.Join(f.projectDirectory, "patches.json"), []byte(`{"unusable":"fixture"}`), 0600); err != nil {
				t.Fatal("fixture write unavailable")
			}
			w, err := createReleaseChainV5Window(f.f.ctx, f.f.pool, strings.Repeat("e", 64), strings.Repeat("f", 64), 7, time.Minute)
			if err != nil {
				t.Fatal(err)
			}
			err = recordReleaseChainV5ProductEvidence(f.f.ctx, w, f, "restart_sent", false, nil)
			var count int
			if scanErr := f.f.pool.QueryRow(f.f.ctx, `SELECT count(*) FROM `+w.table("fixture_chain_v5_high_case_evidence")).Scan(&count); scanErr != nil {
				t.Fatal("owning evidence query failed")
			}
			if !preserve {
				if err == nil || errors.Is(err, errReleaseChainOriginalCaptureInvalid) || count != 0 {
					t.Fatal("legacy capture failure behavior changed")
				}
				return
			}
			if !errors.Is(err, errReleaseChainOriginalCaptureInvalid) || count != 1 {
				t.Fatal("capture rejection lost actual owning SQL")
			}
			var raw []byte
			if scanErr := f.f.pool.QueryRow(f.f.ctx, `SELECT payload_json FROM `+w.table("fixture_chain_v5_high_case_evidence")).Scan(&raw); scanErr != nil {
				t.Fatal("retained witness missing")
			}
			var witness map[string]any
			if json.Unmarshal(raw, &witness) != nil || witness["test_failed"] != true || witness["original_result"] != nil || witness["original_result_capture_status"] != "invalid_or_incomplete" {
				t.Fatal("invalid artifact projection implied success")
			}
			pg, ok := witness["postgres"].(map[string]any)
			if !ok || pg["root_task_id"] != f.f.parent.TaskID {
				t.Fatal("actual owning task identity lost")
			}
			tasks, ok := pg["tasks"].([]any)
			if !ok || len(tasks) == 0 {
				t.Fatal("actual task rows discarded on artifact failure")
			}
		})
	}
}
