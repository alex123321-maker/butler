package main

import "testing"

func TestSingleHumanClosed537ExpenseExceptionPreservesAllOtherGates(t *testing.T) {
	actual := actualPublic(t, "release-chain-v6-user-expense-exception-20261013.json", expenseExceptionSHA)
	verifyExpenseException(actual)
	for _, name := range []string{"another_ordinal", "any_future_unknown", "omit_nulls", "launder_no_go", "no_complete_new_usage", "change_other_thresholds", "fake_reply", "opaque_signature_claim", "old_unknown_zero", "subtotal_estimate", "refill80", "larger_ceiling", "skip_independent_review", "premature_ready"} {
		t.Run(name, func(t *testing.T) {
			x := obj(clone(actual))
			switch name {
			case "another_ordinal":
				x["allowed_closed_ordinal"] = jsonNumber(538)
			case "any_future_unknown":
				x["future_unknown_expense_exceptions_authorized"] = true
			case "omit_nulls":
				x["retain_original_null_usage"] = false
			case "launder_no_go":
				x["retain_closed_chain_no_go"] = false
			case "no_complete_new_usage":
				x["new_after_537_all_attempts_require_complete_measured_usage"] = false
			case "change_other_thresholds":
				x["other_quality_release_criteria_changed"] = true
			case "fake_reply":
				x["user_reply"] = "Разрешить любые неизвестные расходы"
			case "opaque_signature_claim":
				x["external_signature"] = "unverified"
			case "old_unknown_zero":
				x["prior_total_unknown_dimensions"] = jsonNumber(0)
			case "subtotal_estimate":
				x["prior_known_total_tokens_subtotal"] = jsonNumber(580000)
			case "refill80":
				x["remaining_operator_protocol_attempt_cap"] = jsonNumber(80)
			case "larger_ceiling":
				x["absolute_operator_protocol_history_ceiling"] = jsonNumber(617)
			case "skip_independent_review":
				x["independent_native_admission_still_required"] = false
			case "premature_ready":
				x["issue33_ready"] = true
			}
			denied := false
			func() { defer func() { denied = recover() != nil }(); verifyExpenseException(x) }()
			if !denied {
				t.Fatal("broader expense authority or changed evidence accepted")
			}
		})
	}
}
