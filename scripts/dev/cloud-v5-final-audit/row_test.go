package main

import (
	"strconv"
	"testing"

	"github.com/butler/butler/internal/acceptance"
)

func TestFinalAuditActualBaselineStageAndClosedCategory(t *testing.T) {
	finish, version, stage := "STOP", acceptance.CloudV5ExpectedModelVersion, "response_stream"
	for _, baseline := range []int{280, 283} {
		t.Run(strconv.Itoa(baseline), func(t *testing.T) {
			d := diagnostic{Ordinal: baseline + 1, Outcome: "completed", Stage: "completed", Reason: "json_object", Version: &version, Finish: &finish, ProviderStage: &stage, Finals: 1, Terminals: 1, Clean: true}
			if !validDiagnosticRow(d, baseline, 0) {
				t.Fatal("actual intervening baseline rejected")
			}
			for _, mode := range []string{"old_ordinal", "missing_stage", "private_stage", "private_code", "private_collector", "terminal_count", "wrong_finish"} {
				t.Run(mode, func(t *testing.T) {
					x := d
					switch mode {
					case "old_ordinal":
						x.Ordinal = 269
					case "missing_stage":
						x.ProviderStage = nil
					case "private_stage":
						v := "private upstream stage"
						x.ProviderStage = &v
					case "private_code":
						x.Code = "private upstream code"
					case "private_collector":
						x.Stage = "private collector text"
					case "terminal_count":
						x.Terminals = 2
					case "wrong_finish":
						v := "MAX_TOKENS"
						x.Finish = &v
					}
					if validDiagnosticRow(x, baseline, 0) {
						t.Fatal("unsafe original diagnostic promoted")
					}
				})
			}
			d.Outcome, d.Stage, d.Code, d.ProviderStage = "transport_error", "provider_start", "provider_unavailable", nil
			if !validDiagnosticRow(d, baseline, 0) {
				t.Fatal("safe failed-spend diagnostic lost")
			}
		})
	}
}
