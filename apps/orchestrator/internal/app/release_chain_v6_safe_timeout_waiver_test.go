package app

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/butler/butler/internal/acceptance"
)

type releaseChainV6SafeTimeoutWaiver struct {
	Version         string   `json:"version"`
	Source          string   `json:"source"`
	Reply           string   `json:"user_reply"`
	Window          string   `json:"window_id"`
	Baseline        int      `json:"baseline_attempts"`
	PriorProtocol   string   `json:"prior_protocol_sha256"`
	PriorException  string   `json:"retained_537_exception_sha256"`
	Policy          string   `json:"expense_exception_policy"`
	Outcomes        []string `json:"allowed_failed_outcomes"`
	Diagnostic      string   `json:"required_diagnostic_code"`
	Stage           string   `json:"required_provider_operation_stage"`
	Usage           string   `json:"failed_usage_shape"`
	VersionShape    string   `json:"failed_model_version_shape"`
	RetainNULL      bool     `json:"retain_original_null_measurements"`
	RetainNO_GO     bool     `json:"retain_historical_no_go"`
	Retries         int      `json:"maximum_model_retries_per_input"`
	Backoff         int      `json:"model_retry_backoff_ms"`
	Kernel          bool     `json:"require_exact_kernel_operation_and_spent_reservations"`
	Fresh           bool     `json:"require_started_retry_and_fresh_completed_start"`
	ScenarioResult  bool     `json:"require_fixed_scenario_result_or_owner_stop_guarantees"`
	KnownSuccess    bool     `json:"all_completed_entries_require_measured_usage_and_selected_version"`
	OtherUnknown    bool     `json:"other_new_unknown_measurements_allowed"`
	ChangedCriteria bool     `json:"other_safety_or_quality_criteria_changed"`
	Maximum         int      `json:"maximum_new_attempts"`
	Ceiling         int      `json:"maximum_total_attempts"`
	AttemptMillis   int      `json:"maximum_attempt_ms"`
	CaseMillis      int      `json:"maximum_case_ms"`
	GlobalMillis    int      `json:"maximum_global_ms"`
	ReviewRequired  bool     `json:"independent_native_admission_still_required"`
	Ready           bool     `json:"issue33_ready"`
	Limitations     []string `json:"limitations"`
}

func validateReleaseChainV6SafeTimeoutWaiver(w releaseChainV6SafeTimeoutWaiver) error {
	if w.Version != "butler-chain-v6-human-safe-timeout-waiver-v1" || w.Source != "explicit_user_reply_in_current_task" || w.Reply != "Хорошо тогда чутьчуть ослабь требования и если прям критичных проблем не останется тогда можно закрывать пр" || w.Window != releaseChainV6WindowID || w.Baseline != 537 || w.PriorProtocol != releaseChainV6PriorProtocolSHA || w.PriorException != releaseChainV6ExceptionSHA || w.Policy != "one_proved_nominal_response_stream_timeout_retry_v1" || !reflect.DeepEqual(w.Outcomes, []string{"timed_out", "transport_error"}) || w.Diagnostic != "provider_timeout" || w.Stage != "response_stream" || w.Usage != "valid_all_three_known_or_all_three_null" || w.VersionShape != "selected_version_or_null" || !w.RetainNULL || !w.RetainNO_GO || w.Retries != 1 || w.Backoff != 2000 || !w.Kernel || !w.Fresh || !w.ScenarioResult || !w.KnownSuccess || w.OtherUnknown || w.ChangedCriteria || w.Maximum != 78 || w.Ceiling != 615 || w.AttemptMillis != 120000 || w.CaseMillis != 1200000 || w.GlobalMillis != 3600000 || !w.ReviewRequired || w.Ready || len(w.Limitations) != 3 {
		return errNativeFixtureDenied
	}
	return nil
}

func readReleaseChainV6SafeTimeoutWaiver() (releaseChainV6SafeTimeoutWaiver, error) {
	var w releaseChainV6SafeTimeoutWaiver
	raw, err := readReleaseChainV5Artifact(highHistoryPath("release-chain-v6-user-safe-timeout-waiver-20261013.json"), releaseChainV6SafeTimeoutWaiverSHA)
	if err != nil || acceptance.DecodeStrict(raw, &w, "version", "source", "user_reply", "window_id", "baseline_attempts", "prior_protocol_sha256", "retained_537_exception_sha256", "expense_exception_policy", "allowed_failed_outcomes", "required_diagnostic_code", "required_provider_operation_stage", "failed_usage_shape", "failed_model_version_shape", "retain_original_null_measurements", "retain_historical_no_go", "maximum_model_retries_per_input", "model_retry_backoff_ms", "require_exact_kernel_operation_and_spent_reservations", "require_started_retry_and_fresh_completed_start", "require_fixed_scenario_result_or_owner_stop_guarantees", "all_completed_entries_require_measured_usage_and_selected_version", "other_new_unknown_measurements_allowed", "other_safety_or_quality_criteria_changed", "maximum_new_attempts", "maximum_total_attempts", "maximum_attempt_ms", "maximum_case_ms", "maximum_global_ms", "independent_native_admission_still_required", "issue33_ready", "limitations") != nil || validateReleaseChainV6SafeTimeoutWaiver(w) != nil {
		return w, errNativeFixtureDenied
	}
	return w, nil
}

func TestReleaseChainV6HumanSafeTimeoutWaiverTamperMatrix(t *testing.T) {
	w, err := readReleaseChainV6SafeTimeoutWaiver()
	if err != nil {
		t.Fatal("trusted prospective waiver did not bind", err)
	}
	for _, tc := range []struct {
		name   string
		change func(*releaseChainV6SafeTimeoutWaiver)
	}{
		{"other_window", func(w *releaseChainV6SafeTimeoutWaiver) { w.Window = "old-v5" }},
		{"prior_protocol_unbound", func(w *releaseChainV6SafeTimeoutWaiver) { w.PriorProtocol = "UNSET" }},
		{"old_exception_unbound", func(w *releaseChainV6SafeTimeoutWaiver) { w.PriorException = "UNSET" }},
		{"http_phase_widened", func(w *releaseChainV6SafeTimeoutWaiver) { w.Stage = "http_round_trip" }},
		{"other_provider_failure", func(w *releaseChainV6SafeTimeoutWaiver) { w.Diagnostic = "provider_unavailable" }},
		{"partial_usage_allowed", func(w *releaseChainV6SafeTimeoutWaiver) { w.Usage = "any_nullable_dimensions" }},
		{"wrong_failed_version_allowed", func(w *releaseChainV6SafeTimeoutWaiver) { w.VersionShape = "any_version" }},
		{"unknown_success_allowed", func(w *releaseChainV6SafeTimeoutWaiver) { w.KnownSuccess = false }},
		{"other_unknown_allowed", func(w *releaseChainV6SafeTimeoutWaiver) { w.OtherUnknown = true }},
		{"multiple_retries", func(w *releaseChainV6SafeTimeoutWaiver) { w.Retries = 2 }},
		{"backoff_removed", func(w *releaseChainV6SafeTimeoutWaiver) { w.Backoff = 0 }},
		{"kernel_binding_removed", func(w *releaseChainV6SafeTimeoutWaiver) { w.Kernel = false }},
		{"fresh_completion_removed", func(w *releaseChainV6SafeTimeoutWaiver) { w.Fresh = false }},
		{"owner_stop_or_result_relaxed", func(w *releaseChainV6SafeTimeoutWaiver) { w.ScenarioResult = false }},
		{"old_unknown_filled", func(w *releaseChainV6SafeTimeoutWaiver) { w.RetainNULL = false }},
		{"old_no_go_changed", func(w *releaseChainV6SafeTimeoutWaiver) { w.RetainNO_GO = false }},
		{"critical_gates_changed", func(w *releaseChainV6SafeTimeoutWaiver) { w.ChangedCriteria = true }},
		{"refilled_cap", func(w *releaseChainV6SafeTimeoutWaiver) { w.Maximum = 80 }},
		{"ceiling_extended", func(w *releaseChainV6SafeTimeoutWaiver) { w.Ceiling = 617 }},
		{"attempt_extended", func(w *releaseChainV6SafeTimeoutWaiver) { w.AttemptMillis = 121000 }},
		{"global_extended", func(w *releaseChainV6SafeTimeoutWaiver) { w.GlobalMillis = 3600001 }},
		{"review_removed", func(w *releaseChainV6SafeTimeoutWaiver) { w.ReviewRequired = false }},
		{"premature_release", func(w *releaseChainV6SafeTimeoutWaiver) { w.Ready = true }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			raw, _ := json.Marshal(w)
			var copy releaseChainV6SafeTimeoutWaiver
			_ = json.Unmarshal(raw, &copy)
			tc.change(&copy)
			if validateReleaseChainV6SafeTimeoutWaiver(copy) == nil {
				t.Fatal("waiver widened safety or measured-success requirements")
			}
		})
	}
}
