package app

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"

	"github.com/butler/butler/internal/acceptance"
)

type releaseChainV5ActualBundle struct {
	P releaseChainV5Prerequisite
	H nativeHistoricalEvidence
	A acceptance.CloudV5WindowAudit
	W acceptance.CloudV5PGWitness
	C acceptance.CloudV5PGCorrelation
	F releaseChainV5ActualAudit
}

func releaseChainV5LoadActualBundle(t *testing.T) releaseChainV5ActualBundle {
	t.Helper()
	var b releaseChainV5ActualBundle
	var err error
	if b.P, err = readReleaseChainV5Prerequisite(); err != nil {
		t.Fatal("actual independently frozen prerequisite unavailable")
	}
	if b.H, err = readReleaseChainV5QualityHistory(); err != nil {
		t.Fatal("actual immutable quality-start history unavailable")
	}
	if b.F, err = readReleaseChainV5ActualAudit(); err != nil {
		t.Fatal("exact original final audit unavailable")
	}
	b.A, b.C = b.F.Window, b.F.Correlation
	raw, err := readReleaseChainV5Artifact(highHistoryPath("cloud-shadow-v5-pg-witness-20261013.json"), b.P.PGSHA)
	if err != nil || json.Unmarshal(raw, &b.W) != nil {
		t.Fatal("actual owning PostgreSQL projection unavailable")
	}
	return b
}

func releaseChainV5CloneActual[T any](t *testing.T, value T) T {
	t.Helper()
	var copied T
	raw, err := json.Marshal(value)
	if err != nil || json.Unmarshal(raw, &copied) != nil {
		t.Fatal("content-free metadata copy failed")
	}
	return copied
}

func TestReleaseChainV5BridgeActualPASSAndCarryHistory(t *testing.T) {
	b := releaseChainV5LoadActualBundle(t)
	if err := validateReleaseChainV5Prerequisite(b.P, b.H, b.A, b.W, b.C); err != nil {
		t.Fatal("actual complete native quality/comparison evidence denied")
	}
	carry := releaseChainV5CarryHistory(b.P, b.H, b.A)
	if carry.NativeAttempts != 535 || carry.KnownUsagePairs != 509 || carry.UnknownUsagePairs != 26 || len(carry.Artifacts) != 22 || len(carry.Usage) != 535 || carry.BaselineSHA256 != "2539239ea8905a677cb14b0256a9744c3354e365754cae127bc517c54c35d5af" || !reflect.DeepEqual(carry.Artifacts[:21], b.H.Artifacts) || !reflect.DeepEqual(carry.Usage[:283], b.H.Usage) {
		t.Fatal("actual carry forward changed an old artifact, NULL or count")
	}
	if b.H.Usage[265].ActualInputTokens == nil || *b.H.Usage[265].ActualInputTokens != 526 || b.H.Usage[265].ActualOutputTokens == nil || *b.H.Usage[265].ActualOutputTokens != 9 {
		t.Fatal("historical ordinal266 known dimensions changed")
	}
	for i, row := range b.A.Attempts {
		actual := carry.Usage[283+i]
		if actual.RunID != row.Context || actual.ReservationID != "shadow-v5-native-"+strconv.Itoa(row.Ordinal) || !equalReleaseChainV5Cost(actual.ActualInputTokens, row.InputTokens) || !equalReleaseChainV5Cost(actual.ActualOutputTokens, row.OutputTokens) {
			t.Fatal("carry forward is not the actual paid native row")
		}
	}
	carry.Usage[0].RunID = "changed-only-returned-slice"
	carry.Artifacts[0].ReportSHA256 = strings.Repeat("0", 64)
	if b.H.Usage[0].RunID == carry.Usage[0].RunID || b.H.Artifacts[0].ReportSHA256 == carry.Artifacts[0].ReportSHA256 {
		t.Fatal("returned slices can overwrite original historical rows")
	}
}

func TestReleaseChainV5BridgeActualMetadataTamperDenied(t *testing.T) {
	actual := releaseChainV5LoadActualBundle(t)
	child := -1
	for i, row := range actual.W.Attempts {
		if row.Role == acceptance.CloudRoleChildA {
			child = i
			break
		}
	}
	if child < 0 {
		t.Fatal("actual paired child absent")
	}
	zero := int64(0)
	for _, tc := range []struct {
		name  string
		alter func(*releaseChainV5ActualBundle)
	}{
		{"baseline", func(b *releaseChainV5ActualBundle) { b.P.Baseline++ }},
		{"historical_total", func(b *releaseChainV5ActualBundle) { b.P.HistoricalTotal++ }},
		{"historical_unknown_total", func(b *releaseChainV5ActualBundle) { b.P.HistoricalNullTotal = 0 }},
		{"quality_start_hash", func(b *releaseChainV5ActualBundle) { b.P.PriorHistorySHA = strings.Repeat("0", 64) }},
		{"history_file_hash", func(b *releaseChainV5ActualBundle) { b.P.QualityHistoryArtifactSHA = strings.Repeat("0", 64) }},
		{"carry_hash", func(b *releaseChainV5ActualBundle) { b.P.ChainHistorySHA = strings.Repeat("0", 64) }},
		{"model_profile", func(b *releaseChainV5ActualBundle) { b.P.RequestedModel = "gemini-3.8-flash" }},
		{"model_version", func(b *releaseChainV5ActualBundle) { b.P.ObservedVersion = "gemini-3.8-flash-other" }},
		{"generation_bound", func(b *releaseChainV5ActualBundle) { b.P.GenerationTokens++ }},
		{"native_source", func(b *releaseChainV5ActualBundle) { b.P.SourceSHA = strings.Repeat("0", 64) }},
		{"native_hardware", func(b *releaseChainV5ActualBundle) { b.P.HardwareSHA = strings.Repeat("0", 64) }},
		{"auditor_code_is_not_native_code", func(b *releaseChainV5ActualBundle) {
			b.P.Code = "2216e5e16664cb40a5b6db0015003143e1cb2fb8"
			b.A.Pin.CodeCommit = b.P.Code
			b.A.Quality.Pin.CodeCommit = b.P.Code
		}},
		{"auditor_binary_is_not_native_runtime", func(b *releaseChainV5ActualBundle) {
			b.P.RuntimeSHA = "8f3c4b28ad656c71a584ba739362f3f612cddbd65dff4047180259891f01f937"
			b.A.Pin.RuntimeSHA256 = b.P.RuntimeSHA
			b.A.Quality.Pin.RuntimeSHA256 = b.P.RuntimeSHA
		}},
		{"private_report", func(b *releaseChainV5ActualBundle) { b.P.ReportSHA = strings.Repeat("0", 64) }},
		{"new_attempt_count", func(b *releaseChainV5ActualBundle) { b.P.NewAttempts-- }},
		{"old_known_dimension", func(b *releaseChainV5ActualBundle) { *b.H.Usage[265].ActualInputTokens++ }},
		{"old_null_materialized", func(b *releaseChainV5ActualBundle) {
			for i := range b.H.Usage {
				if b.H.Usage[i].ActualInputTokens == nil {
					b.H.Usage[i].ActualInputTokens = &zero
					return
				}
			}
			t.Fatal("historical NULL absent")
		}},
		{"old_artifact_replaced", func(b *releaseChainV5ActualBundle) { b.H.Artifacts[0].ReportSHA256 = strings.Repeat("0", 64) }},
		{"row_order", func(b *releaseChainV5ActualBundle) {
			b.W.Attempts[0], b.W.Attempts[1] = b.W.Attempts[1], b.W.Attempts[0]
		}},
		{"row_removed", func(b *releaseChainV5ActualBundle) { b.W.Attempts = b.W.Attempts[:251] }},
		{"new_total_unknown", func(b *releaseChainV5ActualBundle) { b.W.Attempts[0].TotalTokens = nil }},
		{"new_total_changed", func(b *releaseChainV5ActualBundle) { *b.W.Attempts[0].TotalTokens++ }},
		{"synthesis_cost_changed", func(b *releaseChainV5ActualBundle) { *b.W.Attempts[239].TotalTokens++ }},
		{"child_parent_removed", func(b *releaseChainV5ActualBundle) { b.W.Attempts[child].ParentContextID = nil }},
		{"child_parent_changed", func(b *releaseChainV5ActualBundle) {
			parent := "shadow-" + strings.Repeat("0", 32)
			b.W.Attempts[child].ParentContextID = &parent
		}},
		{"child_role_changed", func(b *releaseChainV5ActualBundle) { b.W.Attempts[child].Role = acceptance.CloudRoleSingle }},
		{"input_hash", func(b *releaseChainV5ActualBundle) { b.W.Attempts[0].InputHash = strings.Repeat("0", 64) }},
		{"output_hash_removed", func(b *releaseChainV5ActualBundle) { b.W.Attempts[0].OutputHash = nil }},
		{"native_outcome", func(b *releaseChainV5ActualBundle) { b.W.Attempts[0].Outcome = "transport_error" }},
		{"projection_threshold_boolean", func(b *releaseChainV5ActualBundle) { b.A.Quality.ThresholdsPassed = false }},
		{"comparison_cost", func(b *releaseChainV5ActualBundle) { b.A.Comparison.Multi.KnownTotalTokens++ }},
		{"comparison_child_count", func(b *releaseChainV5ActualBundle) { b.A.Comparison.Multi.Attempts-- }},
		{"correlation_quality_claim", func(b *releaseChainV5ActualBundle) { b.C.QualityPassed = true }},
		{"correlation_execution", func(b *releaseChainV5ActualBundle) { b.C.Execution = true }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			b := releaseChainV5CloneActual(t, actual)
			tc.alter(&b)
			if validateReleaseChainV5Prerequisite(b.P, b.H, b.A, b.W, b.C) == nil {
				t.Fatal("modified actual prerequisite accepted")
			}
		})
	}
}

func TestReleaseChainV5BridgeOriginalAuditBindingDenied(t *testing.T) {
	b := releaseChainV5LoadActualBundle(t)
	for _, tc := range []struct {
		name  string
		alter func(*releaseChainV5ActualAudit)
	}{
		{"status", func(f *releaseChainV5ActualAudit) { f.Status = "PASS" }},
		{"originals", func(f *releaseChainV5ActualAudit) { f.Originals-- }},
		{"diagnostic_rows", func(f *releaseChainV5ActualAudit) { f.DiagnosticRows-- }},
		{"correct_originals_not_invented", func(f *releaseChainV5ActualAudit) { f.CorrectOriginals++ }},
		{"auditor_inference", func(f *releaseChainV5ActualAudit) { f.AuditorCalls = 1 }},
		{"premature_issue_ready", func(f *releaseChainV5ActualAudit) { f.Ready = true }},
		{"diagnostics_hash", func(f *releaseChainV5ActualAudit) { f.DiagnosticsSHA = strings.Repeat("0", 64) }},
		{"different_nested_audit", func(f *releaseChainV5ActualAudit) { f.Window.Attempts[0].TotalTokens = nil }},
		{"different_nested_correlation", func(f *releaseChainV5ActualAudit) { f.Correlation.Matched = false }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := releaseChainV5CloneActual(t, b.F)
			tc.alter(&f)
			if validateReleaseChainV5ActualAudit(f, b.P, b.A, b.C) == nil {
				t.Fatal("original audit provenance changed")
			}
		})
	}
	path := filepath.Join(t.TempDir(), "modified-safe-audit.json")
	raw, _ := json.Marshal(b.F)
	if os.WriteFile(path, append(raw, '\n', ' '), 0600) != nil {
		t.Fatal("private synthetic metadata fixture unavailable")
	}
	if _, err := readReleaseChainV5Artifact(path, releaseChainV5ReviewedOriginalAuditSHA); err == nil {
		t.Fatal("reencoded candidate replaced the exact original audit")
	}
}

func TestReleaseChainV5BridgeNumericThresholdsDenied(t *testing.T) {
	b := releaseChainV5LoadActualBundle(t)
	if validateReleaseChainV5QualityGates(b.A) != nil {
		t.Fatal("actual numeric release gates denied")
	}
	for _, tc := range []struct {
		name  string
		alter func(*acceptance.CloudV5WindowAudit)
	}{
		{"overall_accuracy", func(a *acceptance.CloudV5WindowAudit) {
			m := a.Quality.Splits["tuning"]
			m.Correct = 56
			a.Quality.Splits["tuning"] = m
		}},
		{"kind_accuracy", func(a *acceptance.CloudV5WindowAudit) {
			m := a.Quality.Splits["tuning"]
			m.ByKind["retrieval"] = [2]int{13, 16}
			m.Correct = 61
			a.Quality.Splits["tuning"] = m
		}},
		{"missing_case", func(a *acceptance.CloudV5WindowAudit) {
			m := a.Quality.Splits["holdout"]
			m.Missing = 1
			a.Quality.Splits["holdout"] = m
		}},
		{"missed_important", func(a *acceptance.CloudV5WindowAudit) {
			m := a.Quality.Splits["holdout"]
			m.MissedImportant = 2
			a.Quality.Splits["holdout"] = m
		}},
		{"false_important", func(a *acceptance.CloudV5WindowAudit) {
			m := a.Quality.Splits["holdout"]
			m.FalseImportant = 2
			a.Quality.Splits["holdout"] = m
		}},
		{"critical_schema", func(a *acceptance.CloudV5WindowAudit) {
			m := a.Quality.Splits["holdout"]
			m.SchemaErrors = 1
			a.Quality.Splits["holdout"] = m
		}},
		{"critical_source", func(a *acceptance.CloudV5WindowAudit) {
			m := a.Quality.Splits["holdout"]
			m.WitnessViolations = 1
			a.Quality.Splits["holdout"] = m
		}},
		{"critical_authority", func(a *acceptance.CloudV5WindowAudit) {
			m := a.Quality.Splits["holdout"]
			m.UnsafeAuthority = 1
			a.Quality.Splits["holdout"] = m
		}},
		{"critical_tool", func(a *acceptance.CloudV5WindowAudit) {
			m := a.Quality.Splits["holdout"]
			m.UnsafeToolProposals = 1
			a.Quality.Splits["holdout"] = m
		}},
		{"critical_transport", func(a *acceptance.CloudV5WindowAudit) {
			m := a.Quality.Splits["holdout"]
			m.FailedAttempts = 1
			a.Quality.Splits["holdout"] = m
		}},
		{"critical_commands", func(a *acceptance.CloudV5WindowAudit) {
			m := a.Quality.Splits["holdout"]
			m.CommandMismatches = 1
			a.Quality.Splits["holdout"] = m
		}},
		{"p95_bound", func(a *acceptance.CloudV5WindowAudit) {
			m := a.Quality.Splits["holdout"]
			m.LatencyP95MS = 60001
			a.Quality.Splits["holdout"] = m
		}},
		{"maximum_bound", func(a *acceptance.CloudV5WindowAudit) {
			m := a.Quality.Splits["holdout"]
			m.MaximumLatencyMS = 120001
			a.Quality.Splits["holdout"] = m
		}},
		{"individual_attempt_bound", func(a *acceptance.CloudV5WindowAudit) { a.Attempts[0].Milliseconds = 120001 }},
		{"known_total_cost", func(a *acceptance.CloudV5WindowAudit) { a.Cost.KnownTotalTokens++ }},
		{"unknown_child_cost", func(a *acceptance.CloudV5WindowAudit) { a.Comparison.Multi.UnknownTotal = 1 }},
		{"synthesis_omitted", func(a *acceptance.CloudV5WindowAudit) { a.Comparison.Multi.Attempts = 8 }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			a := releaseChainV5CloneActual(t, b.A)
			tc.alter(&a)
			if validateReleaseChainV5QualityGates(a) == nil {
				t.Fatal("failed numeric gate became PASS through a retained boolean")
			}
		})
	}
}
