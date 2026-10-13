package main

import "encoding/json"

func v11OneOf(value any, allowed ...string) bool {
	for _, v := range allowed {
		if value == v {
			return true
		}
	}
	return false
}

func verifyV11PoolStats(value any) object {
	p := obj(value)
	keys(p, "max_conns total_conns acquired_conns idle_conns empty_acquire_count cancelled_acquire_count acquire_duration_ms")
	for _, n := range []string{"max_conns", "total_conns", "acquired_conns", "idle_conns", "empty_acquire_count", "cancelled_acquire_count", "acquire_duration_ms"} {
		require(integer(p[n]) >= 0, "v11_pool_counter_nonnegative")
	}
	require(integer(p["max_conns"]) == 2 && integer(p["total_conns"]) <= 2 && integer(p["acquired_conns"])+integer(p["idle_conns"]) <= integer(p["total_conns"]), "v11_owning_pool_bound")
	return p
}

// A diagnostic is data, never a missing current source proof or retry grant.
// Existing product/entry/current-authority predicates independently decide PASS.
func verifyV11SourceObservation(value any, pg object) {
	s := obj(value)
	keys(s, "version failed_checks dropped_failed_checks")
	require(s["version"] == "butler-source-check-observation-v1", "v11_source_projection_version")
	failures := arr(s["failed_checks"])
	require(len(failures) <= 64, "v11_source_projection_bound")
	dropped := integer(s["dropped_failed_checks"])
	require(dropped == 0 || len(failures) == 64, "v11_source_projection_drop_inventory")
	runs := byID(rows(pg, "runs"), "run_id")
	ops := byID(rows(pg, "model_budget_attempts"), "operation_id")
	for _, value := range failures {
		r := obj(value)
		keys(r, "role run_id kernel_operation_id started_at_utc finished_at_utc elapsed_ms maximum_check_ms context_time_left_ms_at_start phase query_family query_template_sha256 failure_category sqlstate acquisition_count acquisition_elapsed_ms pool_before pool_after")
		require(v11OneOf(r["role"], "source_check", "pin_transcript", "lease_watchdog", "task_authority_watchdog"), "v11_source_projection_role")
		require(v11OneOf(r["phase"], "check_started", "pool_acquire", "connection_acquired", "query", "query_complete"), "v11_source_projection_phase")
		require(v11OneOf(r["failure_category"], "deadline_exceeded", "cancelled", "postgres_error", "source_check_failed"), "v11_source_projection_failure")
		if r["sqlstate"] != nil {
			require(r["failure_category"] == "postgres_error" && v11OneOf(r["sqlstate"], "55P03", "57014", "40P01", "40001", "08000", "08003", "08006", "53300"), "v11_source_projection_sqlstate")
		}
		if r["run_id"] != nil {
			require(runs[text(r["run_id"])] != nil, "v11_source_projection_run")
		}
		if r["kernel_operation_id"] != nil {
			op := ops[text(r["kernel_operation_id"])]
			require(op != nil && r["run_id"] != nil && op["run_id"] == r["run_id"], "v11_source_projection_operation")
		}
		maximum := int64(750)
		if r["role"] == "source_check" {
			maximum = 2000
		}
		if r["role"] == "pin_transcript" {
			require(r["maximum_check_ms"] == nil, "v11_source_pin_bound_not_invented")
		} else {
			require(integer(r["maximum_check_ms"]) == maximum, "v11_source_projection_declared_bound")
		}
		if r["context_time_left_ms_at_start"] != nil {
			// Observed parent remaining time, not an enforced or invented bound.
			// An already cancelled/expired caller can supply a negative value.
			number, ok := r["context_time_left_ms_at_start"].(json.Number)
			require(ok, "v11_source_remaining_time_integer")
			_, err := number.Int64()
			require(err == nil, "v11_source_remaining_time_integer")
		}
		start, end := moment(r["started_at_utc"]), moment(r["finished_at_utc"])
		elapsed := integer(r["elapsed_ms"])
		require(!end.Before(start) && elapsed >= 0 && end.Sub(start).Milliseconds() == elapsed, "v11_source_projection_time")
		count, acquired := integer(r["acquisition_count"]), integer(r["acquisition_elapsed_ms"])
		require(count >= 0 && acquired >= 0 && acquired <= elapsed+2, "v11_source_projection_acquisition")
		if v11OneOf(r["phase"], "check_started", "pool_acquire", "connection_acquired") {
			require(r["query_family"] == nil && r["query_template_sha256"] == nil, "v11_prequery_values_absent")
			if r["phase"] == "check_started" {
				require(count == 0, "v11_unobserved_acquisition_not_invented")
			} else {
				require(count > 0, "v11_acquisition_trace_present")
			}
		} else {
			require(v11OneOf(r["query_family"], "memory_restore_state", "memory_deletions", "session_ownership", "butler_memory_check_run_sources", "memory_run_sources", "memory_sources", "task_authority_bindings", "action_mandates", "owner_goals", "owner_purpose_policies", "converter_", "task_delegations", "transcript_", "tasks", "runs", "transaction_control", "other_query"), "v11_source_query_family")
			if r["query_family"] == "other_query" {
				require(r["query_template_sha256"] == nil, "v11_unknown_query_not_hashed")
			} else {
				sha(r["query_template_sha256"])
			}
		}
		before, after := verifyV11PoolStats(r["pool_before"]), verifyV11PoolStats(r["pool_after"])
		for _, name := range []string{"empty_acquire_count", "cancelled_acquire_count", "acquire_duration_ms"} {
			require(integer(after[name]) >= integer(before[name]), "v11_cumulative_pool_counters")
		}
	}
}
