package main

import (
	"encoding/json"
	"github.com/butler/butler/internal/security/content"
	"reflect"
	"sort"
)

const v18OriginalCSV = "name,count\nprivate-original-document,2\n"
const v18GoldenOutput = `[{"label":"private-original-document","quantity":2}]`

// Catalog revisions bind source identity, provenance and descriptive metadata;
// their hash is distinct from the immutable raw source/result body digest.
func v18DocumentRevisionHash(id string) string {
	origin := content.Origin{Kind: content.Document, SourceRefs: []string{id}, ParentRefs: []string{id}, Sensitivity: content.Private}
	return hashValue([]any{"Owner selected synthetic project asset", v18OriginalCSV, object{}, "", nil, origin})
}
func verifyV18DocumentRevision(ref object, sources map[string]object) {
	id := text(ref["source_id"])
	expected := v18DocumentRevisionHash(id)
	row := sources[id]
	require(row != nil && ref["content_hash"] == expected && row["content_hash"] == expected && row["revision"] == ref["revision"] && row["access_revision"] == ref["access_revision"] && row["status"] == "active", "v18_original_catalog_revision")
}
func verifyV18StoppedWaitingIntents(pg, initial object, tasks map[string]object, root, child string) {
	intents := rows(pg, "continuations")
	require(len(intents) == 2, "v18_stopped_waiting_intent_inventory")
	purposes := map[string]bool{}
	for _, c := range intents {
		purpose := text(c["purpose"])
		require((purpose == "candidate_return" || purpose == "activated_catalog") && !purposes[purpose] && c["parent_task_id"] == root && c["child_task_id"] == child && c["state"] == "waiting" && c["input_id"] == nil && c["ready_hash"] == "" && c["parent_revision"] == initial["revision"] && c["constraints_revision"] == initial["constraints_revision"] && integer(tasks[root]["revision"]) == integer(c["parent_revision"])+1 && integer(tasks[root]["constraints_revision"]) == integer(c["constraints_revision"])+1, "v18_stopped_unarmed_intent")
		purposes[purpose] = true
		sha(c["intent_hash"])
		require(text(c["continuation_id"]) != "", "v18_stopped_intent_identity")
	}
}
func verifyV18ConvertedReference(ref, row object) {
	keys(ref, "source_id revision access_revision content_hash")
	require(row != nil && row["source_id"] == ref["source_id"] && row["content_hash"] == ref["content_hash"] && row["revision"] == ref["revision"] && row["access_revision"] == ref["access_revision"] && row["status"] == "active", "v18_converted_catalog_reference")
	sha(ref["content_hash"])
}
func verifyV18DerivedConversionRevision(ref, row, version, refs object) {
	verifyV18ConvertedReference(ref, row)
	require(ref["content_hash"] == v18DerivedConversionHash(ref, version, refs), "v18_converted_catalog_content")
}
func v18DerivedConversionHash(ref, version, refs object) string {
	id := text(ref["source_id"])
	parents := []string{id, text(obj(refs["document"])["source_id"]), text(obj(refs["need"])["source_id"]), text(obj(refs["spec"])["source_id"]), text(obj(version["source_ref_json"])["source_id"])}
	sort.Strings(parents)
	unique := parents[:0]
	for _, p := range parents {
		if len(unique) == 0 || unique[len(unique)-1] != p {
			unique = append(unique, p)
		}
	}
	origin := content.Origin{Kind: content.MemoryDerived, SourceRefs: []string{id}, ParentRefs: unique, Sensitivity: content.Private}
	locator := object{"kind": "converter_output_v1", "package_hash": version["digest"], "need_hash": version["need_hash"], "sha256": digest([]byte(v18GoldenOutput))}
	expected := hashValue([]any{text(version["converter_id"]) + " conversion", v18GoldenOutput, locator, "derived", nil, origin})
	return expected
}
func verifyV18OwnerStopComponent(root string) {
	component := obj(pinnedObject(root, "v17-owner-stop-component-independent-20261013.json", v18StopComponentSHA))
	boundary := obj(pinnedObject(root, "v17-owner-stop-checker-alignment-boundary-20261013.json", v18StopBoundarySHA))
	require(component["version"] == "butler-independent-v17-owner-stop-component-audit-v1" && component["status"] == "OWNER_STOP_COMPONENT_PASS" && component["overall_chain_status"] == "NO_GO" && component["all_release_cases_passed"] == false && component["original_measurements_changed"] == false && component["issue33_ready"] == false && integer(component["native_auth_or_model_calls_by_auditor"]) == 0 && reflect.DeepEqual(arr(component["verified_scenarios"]), []any{"owner_stop"}), "v18_retained_stop_component_only")
	for _, k := range []string{"root_and_child_cancelled", "stop_source_and_broker_denials", "two_measured_kernel_operations_correlated", "all_eight_budget_dimensions_recomputed"} {
		require(component[k] == true, "v18_retained_stop_critical_proof")
	}
	for _, k := range []string{"final_results_or_notices", "live_leases_or_outbox_or_channel_sends", "stopped_descendant_actions", "stopped_descendant_native_entries"} {
		require(integer(component[k]) == 0, "v18_retained_stop_no_effect_or_notice")
	}
	require(boundary["component_report_sha256"] == v18StopComponentSHA && boundary["overall_chain_status"] == "NO_GO" && boundary["critical_stop_component_predicates_unchanged"] != nil && boundary["old_evidence_or_thresholds_rewritten"] == false && boundary["unchanged_full_canonical_checker_passed"] == false, "v18_retained_stop_no_retroactive_pass")
	// These are exact independently frozen input/SQL/packet correlations, not a
	// rewrite of the closed V17 full checker. Every current selected case still
	// needs the common complete product, action, source and delivery checks.
	cost := obj(component["native_measured_cost"])
	require(integer(cost["input_tokens"]) == 16960 && integer(cost["output_tokens"]) == 569 && integer(cost["total_tokens"]) == 20550 && integer(cost["unknown"]) == 0, "v18_retained_stop_cost")
	packets := arr(component["original_two_named_packets"])
	require(len(packets) == 2 && reflect.DeepEqual(arr(component["native_requests"]), []any{json.Number("637"), json.Number("638")}), "v18_retained_stop_packet_inventory")
	for i, value := range packets {
		m := obj(value)
		require(integer(m["ordinal"]) == int64(637+i) && m["actual_core_arguments_hash_matches_original"] == true && m["qualified_action_authorization_dispatch_confirmed"] == true, "v18_retained_stop_packet_binding")
		sha(m["private_proposal_sha256"])
		sha(m["private_metadata_sha256"])
	}
}
