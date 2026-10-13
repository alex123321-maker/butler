package main

import (
	"os"
	"path/filepath"
	"testing"
)

func candidateRepo(t *testing.T) string {
	t.Helper()
	root, e := os.Getwd()
	if e != nil {
		t.Fatal(e)
	}
	for {
		if _, e = os.Stat(filepath.Join(root, "go.mod")); e == nil {
			return root
		}
		parent := filepath.Dir(root)
		if parent == root {
			t.Fatal("repository unavailable")
		}
		root = parent
	}
}
func candidatePrerequisite(t *testing.T) any {
	t.Helper()
	root := candidateRepo(t)
	path := filepath.Join(root, "deploy", "acceptance", "release-chain-v6-high-prerequisite-20261013.json")
	raw, e := readPinnedPublic(path, prerequisiteSHA)
	if e != nil {
		t.Fatal("exact candidate prerequisite unavailable", e)
	}
	v, e := decode(raw)
	if e != nil {
		t.Fatal(e)
	}
	return v
}
func TestActualSealedFailedV5HistoryAndMultiEntryRunRemainUnchanged(t *testing.T) {
	root := candidateRepo(t)
	verifyRetainedFailedV5Source(root)
	objects := map[string]any{}
	for k, pin := range retainedFailedV5Pins() {
		objects[k] = actualPublic(t, "release-chain-v5-"+k+"-20261013.json", pin)
	}
	for _, name := range []string{"old_no_go_promoted", "lost_unknown_refunded", "old537_estimated", "old536_cost_changed", "old_witness_hash", "old_unknown266_recovered"} {
		t.Run(name, func(t *testing.T) {
			x := obj(clone(objects))
			switch name {
			case "old_no_go_promoted":
				obj(obj(x["final-audit"])["chain_audit"])["status"] = "PASS"
			case "lost_unknown_refunded":
				obj(obj(obj(x["final-audit"])["chain_audit"])["all_new_measured_cost"])["unknown_total_attempts"] = jsonNumber(0)
			case "old537_estimated":
				obj(arr(obj(x["pg-ledger"])["requests"])[1])["total_tokens"] = jsonNumber(8462)
			case "old536_cost_changed":
				obj(arr(obj(x["pg-ledger"])["requests"])[0])["total_tokens"] = jsonNumber(1)
			case "old_witness_hash":
				obj(arr(x["pg-product"])[0])["payload_sha256"] = expenseExceptionSHA
			case "old_unknown266_recovered":
				obj(obj(x["failure-correlation"])["carry_forward"])["old_unknown_266_cause"] = "timeout"
			}
			denied := false
			func() { defer func() { denied = recover() != nil }(); verifyRetainedFailedV5(x) }()
			if !denied {
				t.Fatal("old failed native history rewritten")
			}
		})
	}
}
func TestV6PrerequisiteRemainingBudgetStagesAndNewUsageAreFrozen(t *testing.T) {
	actual := candidatePrerequisite(t)
	verifyV6Prerequisite(obj(actual))
	for _, name := range []string{"reset_baseline535", "restore80", "larger617", "wrong_history", "unknowns_erased", "estimated_subtotal", "exception_missing", "new_unknown_allowed", "stages_disabled", "model_changed", "generation_relaxed", "retry_policy_changed", "second_model_retry", "shortened_retry_backoff", "kernel_binding_disabled", "original_protocol_removed", "waiver_pin_replaced", "waiver_disabled", "unamended_expense_claim"} {
		t.Run(name, func(t *testing.T) {
			x := obj(clone(actual))
			switch name {
			case "reset_baseline535":
				x["baseline_attempts"] = jsonNumber(535)
			case "restore80":
				x["maximum_new_attempts"] = jsonNumber(80)
			case "larger617":
				x["maximum_total_attempts"] = jsonNumber(617)
			case "wrong_history":
				x["prior_history_sha256"] = expenseExceptionSHA
			case "unknowns_erased":
				x["historical_unknown_total_attempts"] = jsonNumber(0)
			case "estimated_subtotal":
				x["historical_known_total_tokens_subtotal"] = jsonNumber(580000)
			case "exception_missing":
				x["human_expense_exception_sha256"] = nil
			case "new_unknown_allowed":
				x["all_successful_expense_must_be_measured"] = false
			case "stages_disabled":
				x["provider_stage_observation"] = false
			case "model_changed":
				x["requested_model"] = "other"
			case "generation_relaxed":
				x["maximum_generation_tokens"] = jsonNumber(65536)
			case "retry_policy_changed":
				x["retry_policy"] = "provider_retryable_grants_authority"
			case "second_model_retry":
				x["maximum_model_retries_per_input"] = jsonNumber(2)
			case "shortened_retry_backoff":
				x["model_retry_backoff_ms"] = jsonNumber(1999)
			case "kernel_binding_disabled":
				x["capture_kernel_operation_id"] = false
			case "original_protocol_removed":
				x["prior_protocol_sha256"] = nil
			case "waiver_pin_replaced":
				x["human_safe_timeout_waiver_sha256"] = expenseExceptionSHA
			case "waiver_disabled":
				x["allow_unmeasured_safe_timeout_retry"] = false
			case "unamended_expense_claim":
				x["all_new_expense_must_be_measured"] = true
			}
			denied := false
			func() { defer func() { denied = recover() != nil }(); verifyV6Prerequisite(x) }()
			if !denied {
				t.Fatal("prospective resources or expense gates changed")
			}
		})
	}
}
func TestV6CurrentSQLStagesAndExceptionCannotLaunderNewUnknownSpend(t *testing.T) {
	for _, name := range []string{"stage_missing", "stage_http_only", "stage_private", "window_refill80", "window_observer_false", "window_kernel_binding_false", "new_unknown_expense"} {
		t.Run(name, func(t *testing.T) {
			x := syntheticWitness(t)
			a := obj(arr(obj(obj(x.Scope)["scope"])["requests"])[0])
			b := obj(arr(obj(x.Ledger)["requests"])[0])
			switch name {
			case "stage_missing":
				a["provider_operation_stage"] = nil
				b["provider_operation_stage"] = nil
			case "stage_http_only":
				a["provider_operation_stage"] = "http_round_trip"
				b["provider_operation_stage"] = "http_round_trip"
			case "stage_private":
				a["provider_operation_stage"] = "PRIVATE_STAGE"
				b["provider_operation_stage"] = "PRIVATE_STAGE"
			case "window_kernel_binding_false":
				obj(obj(obj(x.Scope)["scope"])["window"])["kernel_operation_binding"] = false
				obj(obj(x.Ledger)["window"])["kernel_operation_binding"] = false
			case "window_refill80":
				obj(obj(obj(x.Scope)["scope"])["window"])["maximum_new_attempts"] = jsonNumber(80)
				obj(obj(x.Ledger)["window"])["maximum_new_attempts"] = jsonNumber(80)
			case "window_observer_false":
				obj(obj(obj(x.Scope)["scope"])["window"])["provider_stage_observation"] = false
				obj(obj(x.Ledger)["window"])["provider_stage_observation"] = false
			case "new_unknown_expense":
				a["actual_total_tokens"] = nil
				b["total_tokens"] = nil
			}
			r, e := audit(x)
			if e == nil || r.Status != "NO_GO" || r.Ready || r.Cost.Attempts != 70 {
				t.Fatal("new scope bypassed frozen gate")
			}
			if name == "new_unknown_expense" && r.Cost.Unknown != 1 {
				t.Fatal("new unknown covered by closed537 exception")
			}
		})
	}
}
