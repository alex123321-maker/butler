package app

import (
	"context"
	"encoding/json"
	"os"
	"reflect"
	"strconv"
	"testing"
	"time"

	"github.com/butler/butler/internal/acceptance"
	"github.com/jackc/pgx/v5/pgxpool"
)

const releaseChainV6WindowID = "issue33-high-release-chain-v6-20261013-baseline537"
const releaseChainV6Baseline = 537
const releaseChainV6NewCap = 78
const releaseChainV6Ceiling = 615

// Independently reviewed protocol bytes. Final prepared source/runtime/fault
// admission remains separate; a history count or expense exception is insufficient.
const releaseChainV6ReviewedPrerequisiteSHA = "9ce27e6d9a15f598fadc037e5de377869672937fef84849fc662610ec6379de3"
const releaseChainV6ExceptionSHA = "31b930ac4e39206d65424ac951f65cc4656b71166995650122cbdda3e63b6ab4"
const releaseChainV6FailedAuditSHA = "592d27ee34178a9b336eb0f9cce4784fac1ba214b22f05e1ad0b247a48f97c04"
const releaseChainV6FailedCorrelationSHA = "fb8a430c327c4abec5984cad6fec6e79aa57d36866500dd588979722f282a424"
const releaseChainV6FailedCode = "afa50b225b1341d2ff8a553188a69f6b6b2ac633"
const releaseChainV6FailedSourceSHA = "862fafb7a38a372a288a363fd73260b1efa851837080bdafecf90545d75cf26b"
const releaseChainV6FailedRuntimeSHA = "383729b5493e2038f95fa23924feac880552531537d06f072c3eba8f77768b00"

func releaseChainV6FailedArtifacts() map[string]string {
	return map[string]string{
		"final-audit":         releaseChainV6FailedAuditSHA,
		"high":                "e1101e9f1b8b435e9f420aeea7d1507326cd0f0ee3bc80311c417792c899f709",
		"pg-ledger":           "778b8cd9e204ba074b98ac677d3867522728d25e3f7566473adff6f312160c05",
		"pg-product":          "82a3608fa9e86014140da6ad2e5bb769cfc1747cdab2c98027ccfdfb64557181",
		"pg-initial":          "31a5e87d90569a8a950e76206b1825c818e9ebddec9be2c54e71111da05f78a0",
		"runtime-state":       "4fb2e8ba23eaaf64be7b9967d265fddf2e0ccea07363bbfecfc38d5163716aca",
		"failure-correlation": releaseChainV6FailedCorrelationSHA,
	}
}

type releaseChainV6ExpenseException struct {
	Version          string   `json:"version"`
	Source           string   `json:"source"`
	ReplyTo          string   `json:"reply_to"`
	Reply            string   `json:"user_reply"`
	Ordinal          int      `json:"allowed_closed_ordinal"`
	Fields           []string `json:"allowed_unknown_fields"`
	RetainNULL       bool     `json:"retain_original_null_usage"`
	RetainNO_GO      bool     `json:"retain_closed_chain_no_go"`
	NewKnown         bool     `json:"new_after_537_all_attempts_require_complete_measured_usage"`
	FutureExceptions bool     `json:"future_unknown_expense_exceptions_authorized"`
	ChangedCriteria  bool     `json:"other_quality_release_criteria_changed"`
	OldCause         string   `json:"historical_266_cause"`
	Unknown          int      `json:"prior_total_unknown_dimensions"`
	Subtotal         int64    `json:"prior_known_total_tokens_subtotal"`
	Remaining        int      `json:"remaining_operator_protocol_attempt_cap"`
	Ceiling          int      `json:"absolute_operator_protocol_history_ceiling"`
	ReviewRequired   bool     `json:"independent_native_admission_still_required"`
	Ready            bool     `json:"issue33_ready"`
	Limits           []string `json:"limitations"`
}

func validateReleaseChainV6ExpenseException(e releaseChainV6ExpenseException) error {
	if e.Version != "butler-chain-v6-human-expense-exception-v1" || e.Source != "explicit_user_reply_in_current_task" || e.ReplyTo != "request_user_input_async:call_48746905f5834b09be7b567fd2531f0d" || e.Reply != "Разрешить UNKNOWN только для закрытого сбоя №537; новый прогон должен иметь полный измеренный расход." || e.Ordinal != 537 || !reflect.DeepEqual(e.Fields, []string{"input_tokens", "output_tokens", "total_tokens"}) || !e.RetainNULL || !e.RetainNO_GO || !e.NewKnown || e.FutureExceptions || e.ChangedCriteria || e.OldCause != "UNKNOWN" || e.Unknown != 47 || e.Subtotal != 572459 || e.Remaining != 78 || e.Ceiling != 615 || !e.ReviewRequired || e.Ready || len(e.Limits) != 3 {
		return errNativeFixtureDenied
	}
	return nil
}

func readReleaseChainV6ExpenseException() (releaseChainV6ExpenseException, error) {
	var e releaseChainV6ExpenseException
	raw, err := readReleaseChainV5Artifact(highHistoryPath("release-chain-v6-user-expense-exception-20261013.json"), releaseChainV6ExceptionSHA)
	if err != nil || acceptance.DecodeStrict(raw, &e, "version", "source", "reply_to", "user_reply", "allowed_closed_ordinal", "allowed_unknown_fields", "retain_original_null_usage", "retain_closed_chain_no_go", "new_after_537_all_attempts_require_complete_measured_usage", "future_unknown_expense_exceptions_authorized", "other_quality_release_criteria_changed", "historical_266_cause", "prior_total_unknown_dimensions", "prior_known_total_tokens_subtotal", "remaining_operator_protocol_attempt_cap", "absolute_operator_protocol_history_ceiling", "independent_native_admission_still_required", "issue33_ready", "limitations") != nil || validateReleaseChainV6ExpenseException(e) != nil {
		return e, errNativeFixtureDenied
	}
	return e, nil
}

type releaseChainV6Prerequisite struct {
	Version              string `json:"version"`
	Baseline             int    `json:"baseline_attempts"`
	Maximum              int    `json:"maximum_new_attempts"`
	Ceiling              int    `json:"maximum_total_attempts"`
	HistorySHA           string `json:"prior_history_sha256"`
	QualitySHA           string `json:"retained_quality_prerequisite_sha256"`
	OriginalQualitySHA   string `json:"retained_quality_final_audit_sha256"`
	FailedAuditSHA       string `json:"failed_v5_audit_sha256"`
	FailedCorrelationSHA string `json:"failed_v5_correlation_sha256"`
	ExpenseExceptionSHA  string `json:"human_expense_exception_sha256"`
	KnownInput           int64  `json:"historical_known_input_tokens_subtotal"`
	KnownOutput          int64  `json:"historical_known_output_tokens_subtotal"`
	KnownTotal           int64  `json:"historical_known_total_tokens_subtotal"`
	UnknownTotal         int    `json:"historical_unknown_total_attempts"`
	KnownPairs           int    `json:"historical_known_usage_pairs"`
	UnknownPairs         int    `json:"historical_unknown_usage_pairs"`
	ProviderStages       bool   `json:"provider_stage_observation"`
	RequireKnown         bool   `json:"all_new_expense_must_be_measured"`
	Generation           int    `json:"maximum_generation_tokens"`
	Model                string `json:"requested_model"`
	RetryPolicy          string `json:"retry_policy"`
	MaximumRetries       int    `json:"maximum_model_retries_per_input"`
	RetryBackoffMillis   int    `json:"model_retry_backoff_ms"`
	KernelOperations     bool   `json:"capture_kernel_operation_id"`
}

func validateReleaseChainV6Prerequisite(p releaseChainV6Prerequisite, h nativeHistoricalEvidence) error {
	if p.RetryPolicy != "one_nominal_timeout_per_accepted_input_v1" || p.MaximumRetries != 1 || p.RetryBackoffMillis != 2000 || !p.KernelOperations {
		return errNativeFixtureDenied
	}
	if p.Version != "butler-chain-high-reviewed-v6-prerequisite-v1" || p.Baseline != 537 || p.Maximum != 78 || p.Ceiling != 615 || p.Baseline+p.Maximum != p.Ceiling || p.HistorySHA != h.BaselineSHA256 || h.NativeAttempts != 537 || h.KnownUsagePairs != 510 || h.UnknownUsagePairs != 27 || p.QualitySHA != releaseChainV5ReviewedPrerequisiteSHA || p.OriginalQualitySHA != releaseChainV5ReviewedOriginalAuditSHA || p.FailedAuditSHA != releaseChainV6FailedAuditSHA || p.FailedCorrelationSHA != releaseChainV6FailedCorrelationSHA || p.ExpenseExceptionSHA != releaseChainV6ExceptionSHA || p.KnownInput != 294598 || p.KnownOutput != 11827 || p.KnownTotal != 572459 || p.UnknownTotal != 47 || p.KnownPairs != 510 || p.UnknownPairs != 27 || !p.ProviderStages || !p.RequireKnown || p.Generation != 32768 || p.Model != acceptance.CloudRequestedModel {
		return errNativeFixtureDenied
	}
	return nil
}

// The common scenario implementation needs only the retained measured model and
// the new actual history. Every other gate is checked before auth or window DDL.
func readReleaseChainV6Prerequisite() (releaseChainV5Prerequisite, error) {
	var result releaseChainV5Prerequisite
	if !nativeAuditHash.MatchString(releaseChainV6ReviewedPrerequisiteSHA) {
		return result, errNativeFixtureDenied
	}
	raw, err := readReleaseChainV5Artifact(highHistoryPath("release-chain-v6-high-prerequisite-20261013.json"), releaseChainV6ReviewedPrerequisiteSHA)
	var p releaseChainV6Prerequisite
	if err != nil || acceptance.DecodeStrict(raw, &p, "version", "baseline_attempts", "maximum_new_attempts", "maximum_total_attempts", "prior_history_sha256", "retained_quality_prerequisite_sha256", "retained_quality_final_audit_sha256", "failed_v5_audit_sha256", "failed_v5_correlation_sha256", "human_expense_exception_sha256", "historical_known_input_tokens_subtotal", "historical_known_output_tokens_subtotal", "historical_known_total_tokens_subtotal", "historical_unknown_total_attempts", "historical_known_usage_pairs", "historical_unknown_usage_pairs", "provider_stage_observation", "all_new_expense_must_be_measured", "maximum_generation_tokens", "requested_model", "retry_policy", "maximum_model_retries_per_input", "model_retry_backoff_ms", "capture_kernel_operation_id") != nil {
		return result, errNativeFixtureDenied
	}
	h, err := readReleaseChainV6ActualHistory()
	if err != nil || validateReleaseChainV6Prerequisite(p, h) != nil {
		return result, errNativeFixtureDenied
	}
	if _, err = readReleaseChainV6ExpenseException(); err != nil {
		return result, errNativeFixtureDenied
	}
	result, err = readReleaseChainV5Prerequisite()
	if err != nil {
		return result, errNativeFixtureDenied
	}
	result.Baseline, result.ChainHistorySHA = p.Baseline, p.HistorySHA
	return result, nil
}

func newReleaseChainV6Window(ctx context.Context, pool *pgxpool.Pool) (*releaseChainV5Window, error) {
	p, err := readReleaseChainV6Prerequisite()
	if err != nil {
		return nil, errNativeFixtureDenied
	}
	deadline, err := time.Parse(time.RFC3339, os.Getenv("BUTLER_SCENARIO_CHAIN_V6_HIGH_DEADLINE"))
	if err != nil {
		return nil, errNativeFixtureDenied
	}
	return createReleaseChainHighWindowInSchemaAt(ctx, pool, releaseChainV6ReviewedPrerequisiteSHA, p.ChainHistorySHA, p.Baseline, "fixture_chain_v6_high", deadline, releaseChainV6NewCap, true, true)
}

// A fresh history fold binds the exact failed owning SQL and safe correlation;
// it does not contain projected admissions or convert NO_GO into a PASS.
func releaseChainV6CarryHistory(prior nativeHistoricalEvidence, rows []map[string]any) nativeHistoricalEvidence {
	h := prior
	h.Usage = append([]nativeUsageEvidence(nil), prior.Usage...)
	h.Artifacts = append([]nativePriorArtifact(nil), prior.Artifacts...)
	for _, row := range rows {
		u := nativeUsageEvidence{RunID: row["run_id"].(string), ReservationID: "release-chain-v5-native-" + strconv.Itoa(int(row["ordinal"].(float64)))}
		if row["actual_input_tokens"] != nil {
			in, out := int64(row["actual_input_tokens"].(float64)), int64(row["actual_output_tokens"].(float64))
			u.ActualInputTokens, u.ActualOutputTokens = &in, &out
			h.KnownUsagePairs++
		} else {
			h.UnknownUsagePairs++
		}
		h.Usage = append(h.Usage, u)
	}
	h.NativeAttempts += len(rows)
	h.Artifacts = append(h.Artifacts, nativePriorArtifact{ReportSHA256: releaseChainV6FailedAuditSHA, ReleaseRef: releaseChainV6FailedCode, NewAttempts: 2})
	pins := releaseChainV6FailedArtifacts()
	h.BaselineSHA256 = shadowV4Hash([]string{prior.BaselineSHA256, pins["final-audit"], pins["high"], pins["pg-ledger"], pins["pg-product"], pins["pg-initial"], pins["runtime-state"], pins["failure-correlation"], releaseChainV6FailedCode, releaseChainV6FailedSourceSHA, releaseChainV6FailedRuntimeSHA})
	return h
}

func TestReleaseChainV6OperationalPrerequisiteClosed(t *testing.T) {
	if releaseChainV6ReviewedPrerequisiteSHA == "" {
		if _, err := readReleaseChainV6Prerequisite(); err == nil {
			t.Fatal("actual count or human exception authorized an unfrozen scope")
		}
	} else if _, err := readReleaseChainV6Prerequisite(); err != nil {
		t.Fatal("separately reviewed V6 prerequisite unavailable")
	}
}

func TestReleaseChainV6OperationalGate(t *testing.T) {
	if os.Getenv("BUTLER_SCENARIO_CHAIN_V6_HIGH_GATE") != "1" {
		t.Skip("explicit separately frozen operational gate required")
	}
	if _, err := readReleaseChainV6Prerequisite(); err != nil {
		t.Fatal("V6 independent native admission has not been frozen")
	}
}

// Only exact safe artifacts are decoded. Neither inference nor provider
// credentials are used by this history reader or its positive/tamper tests.
func readReleaseChainV6ActualHistory() (nativeHistoricalEvidence, error) {
	priorP, err := readReleaseChainV5Prerequisite()
	if err != nil {
		return nativeHistoricalEvidence{}, errNativeFixtureDenied
	}
	prior, err := readReleaseChainV5QualityHistory()
	if err != nil {
		return nativeHistoricalEvidence{}, errNativeFixtureDenied
	}
	var qa acceptance.CloudV5WindowAudit
	raw, err := readReleaseChainV5Artifact(highHistoryPath("cloud-shadow-v5-high-20261013.json"), priorP.AuditSHA)
	if err != nil || json.Unmarshal(raw, &qa) != nil {
		return nativeHistoricalEvidence{}, errNativeFixtureDenied
	}
	prior = releaseChainV5CarryHistory(priorP, prior, qa)
	artifacts, err := readReleaseChainV6FailureObjects()
	if err != nil {
		return nativeHistoricalEvidence{}, errNativeFixtureDenied
	}
	if validateReleaseChainV6FailedArtifacts(artifacts) != nil {
		return nativeHistoricalEvidence{}, errNativeFixtureDenied
	}
	rows := artifacts["high"]["scope"].(map[string]any)["requests"].([]any)
	actual := []map[string]any{rows[0].(map[string]any), rows[1].(map[string]any)}
	h := releaseChainV6CarryHistory(prior, actual)
	if h.NativeAttempts != 537 || len(h.Usage) != 537 || h.KnownUsagePairs != 510 || h.UnknownUsagePairs != 27 || !reflect.DeepEqual(h.Usage[:535], prior.Usage) || !reflect.DeepEqual(h.Artifacts[:len(prior.Artifacts)], prior.Artifacts) {
		return nativeHistoricalEvidence{}, errNativeFixtureDenied
	}
	var in, out int64
	for _, u := range h.Usage {
		if u.ActualInputTokens != nil {
			in += *u.ActualInputTokens
		}
		if u.ActualOutputTokens != nil {
			out += *u.ActualOutputTokens
		}
	}
	if in != 294598 || out != 11827 {
		return nativeHistoricalEvidence{}, errNativeFixtureDenied
	}
	return h, nil
}
