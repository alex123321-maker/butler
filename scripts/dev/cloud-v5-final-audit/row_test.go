package main

import (
	"strconv"
	"testing"

	"github.com/butler/butler/internal/acceptance"
	"github.com/butler/butler/internal/transport"
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

func TestFinalAuditRecomputesCriticalCategoryWithoutRepair(t *testing.T) {
	for _, stage := range []string{"critical_schema_contract", "critical_source_contract", "critical_authority_contract", "critical_tool_contract"} {
		d := diagnostic{Ordinal: 284, Outcome: "transport_error", Stage: stage, Code: string(transport.ErrorTypeProviderProtocolError)}
		if !validDiagnosticRow(d, 283, 0) {
			t.Fatal("critical failed spend lost")
		}
		d.Code = ""
		if validDiagnosticRow(d, 283, 0) {
			t.Fatal("unclassified critical failure accepted")
		}
	}
	for _, tc := range []struct {
		check acceptance.CloudResponseCheck
		stage string
	}{
		{acceptance.CloudResponseCheck{}, ""},
		{acceptance.CloudResponseCheck{SchemaError: true}, "critical_schema_contract"},
		{acceptance.CloudResponseCheck{SchemaError: true, SourceFabrication: true}, "critical_source_contract"},
		{acceptance.CloudResponseCheck{SchemaError: true, SourceFabrication: true, UnsafeAuthority: true}, "critical_authority_contract"},
	} {
		if criticalOriginalStage(tc.check) != tc.stage {
			t.Fatal("original critical priority changed")
		}
	}
}
