package main

import "time"

// This rule is fixed before native execution. A success never supplies a failed
// request's usage: verifyRequest and exact owning budget correlation run first.
func verifyModelRetries(scenario string, pg object, native []object) int {
	byOperation := map[string]object{}
	byRun := map[string][]object{}
	failed := map[string]bool{}
	for _, n := range native {
		op := text(n["kernel_operation_id"])
		require(op != "" && byOperation[op] == nil, "retry_operation_inventory")
		byOperation[op] = n
		byRun[text(n["run_id"])] = append(byRun[text(n["run_id"])], n)
		if n["outcome"] != "completed" {
			failed[op] = false
		}
	}
	kernel := byID(rows(pg, "model_budget_attempts"), "operation_id")
	runs := byID(rows(pg, "runs"), "run_id")
	boundRuns := map[string]bool{}
	count := 0
	for _, input := range rows(pg, "accepted_inputs") {
		raw := input["model_retry"]
		if raw == nil {
			continue
		}
		retry := obj(raw)
		keys(retry, "count reason prior_operation_id state claimed_at not_before retry_operation_id started_at")
		require(integer(retry["count"]) == 1 && retry["reason"] == "model_turn_timeout" && retry["state"] == "started" && integer(input["attempts"]) == 2, "durable_retry_contract")
		run := text(input["run_id"])
		require(run != "" && !boundRuns[run], "one_retry_per_input_run")
		if scenario == "owner_stop" {
			require(input["status"] == "cancelled" && input["error_code"] == "owner_control" && runs[run] != nil && runs[run]["status"] == "cancelled" && runs[run]["current_state"] == "cancelled", "stopped_retry_exact_owner_control")
		} else {
			require(input["status"] == "completed" && input["error_code"] == "", "durable_retry_contract")
		}
		boundRuns[run] = true
		priorID, freshID := text(retry["prior_operation_id"]), text(retry["retry_operation_id"])
		prior, fresh := byOperation[priorID], byOperation[freshID]
		wasBound, exists := failed[priorID]
		require(exists && !wasBound && prior != nil && fresh != nil && priorID != freshID && prior["run_id"] == run && fresh["run_id"] == run && fresh["kind"] == "start" && fresh["outcome"] == "completed", "retry_exact_failed_to_fresh_start")
		claimed, ready, started := moment(retry["claimed_at"]), moment(retry["not_before"]), moment(retry["started_at"])
		require(ready.Sub(claimed) == 2*time.Second && !started.Before(ready) && !started.After(moment(fresh["admitted_at_utc"])) && !moment(input["updated_at"]).Before(started) && !moment(input["not_before"]).Before(ready), "retry_durable_backoff")
		require(!claimed.Before(moment(prior["admitted_at_utc"])) && !moment(prior["finished_at_utc"]).After(moment(fresh["admitted_at_utc"])) && moment(input["expires_at"]).After(moment(fresh["finished_at_utc"])), "retry_current_input_deadline")
		priorBudget, freshBudget := kernel[priorID], kernel[freshID]
		require(priorBudget != nil && freshBudget != nil && priorBudget["reservation_id"] != freshBudget["reservation_id"] && !moment(freshBudget["created_at"]).Before(ready) && !moment(freshBudget["created_at"]).After(started), "retry_fresh_spent_reservation")
		sequence := byRun[run]
		position := -1
		for j, n := range sequence {
			if n["kernel_operation_id"] == priorID {
				position = j
				break
			}
			require(integer(n["final_count"]) == 0, "retry_after_completed_native_final")
		}
		require(position >= 0 && position+1 < len(sequence) && sequence[position+1]["kernel_operation_id"] == freshID, "retry_next_request_same_run")
		failed[priorID] = true
		count++
	}
	for _, bound := range failed {
		require(bound, "unbound_failed_request")
	}
	return count
}
