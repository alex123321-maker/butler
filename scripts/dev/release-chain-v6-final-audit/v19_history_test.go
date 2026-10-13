package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func v19HistoricalTestEvidence(t *testing.T) v19ClosedV18Evidence {
	t.Helper()
	root := candidateRepo(t)
	e := v19ClosedV18Evidence{
		Prior:       obj(pinnedObject(root, "release-chain-v17-history658-20261013.json", v17HistorySHA)),
		History:     obj(pinnedObject(root, "release-chain-v18-history705-20261013.json", v19ClosedV18HistorySHA)),
		Bridge:      obj(pinnedObject(root, "release-chain-v18-carry705-independent-20261013.json", v19ClosedV18BridgeSHA)),
		Correlation: obj(pinnedObject(root, "release-chain-v18-failure-correlation-20261013.json", v19ClosedV18CorrelationSHA)),
		Final:       obj(pinnedObject(root, "release-chain-v18-final-audit-20261013.json", v19ClosedV18FinalSHA)),
		Component:   obj(pinnedObject(root, "v18-explicit-absence-component-independent-20261013.json", v19RetainedAbsenceSHA)),
	}
	fixture := os.Getenv("BUTLER_V19_HISTORICAL_FIXTURE_DIR")
	if fixture == "" {
		t.Fatal("five exact named safe historical leaves required; set BUTLER_V19_HISTORICAL_FIXTURE_DIR")
	}
	values := map[string]any{}
	for _, name := range []string{"scope", "ledger", "initial", "product", "runtime"} {
		raw, err := readPinnedPublic(filepath.Join(fixture, name+".json"), v19ClosedV18RawPins[name])
		if err != nil {
			t.Fatal("historical fixture identity denied", name)
		}
		values[name], err = decode(raw)
		if err != nil {
			t.Fatal("historical fixture decoding denied", name)
		}
	}
	e.Scope, e.Ledger, e.Runtime = obj(values["scope"]), obj(values["ledger"]), obj(values["runtime"])
	e.Initial, e.Product = arr(values["initial"]), arr(values["product"])
	return e
}

func TestV19RetainedActual705Semantic(t *testing.T) {
	e := v19HistoricalTestEvidence(t)
	verifyV19ClosedV18Evidence(e)
}

func TestV19RetainedActual705TamperBoundaries(t *testing.T) {
	for _, kind := range []string{"prefix658_changed", "cancel705_zero_cost", "cancel705_accepted_retry", "failed_lost_ack_promoted", "watchdog_foreign_run", "watchdog_reported_model_timeout", "absence_second_prepare", "absence_missing_notice", "absence_catalog_raw_body_hash", "absence_private_packet_digest_changed"} {
		t.Run(kind, func(t *testing.T) {
			e := v19HistoricalTestEvidence(t)
			switch kind {
			case "prefix658_changed":
				obj(arr(e.History["usage"])[0])["actual_input_tokens"] = jsonNumber(0)
			case "cancel705_zero_cost":
				r := rows(e.Ledger, "requests")[46]
				r["input_tokens"], r["output_tokens"], r["total_tokens"] = jsonNumber(0), jsonNumber(0), jsonNumber(0)
				n := rows(obj(e.Scope["scope"]), "requests")[46]
				n["actual_input_tokens"], n["actual_output_tokens"], n["actual_total_tokens"] = jsonNumber(0), jsonNumber(0), jsonNumber(0)
			case "cancel705_accepted_retry":
				obj(e.Final["chain_audit"])["accepted_model_retry_failures"] = jsonNumber(1)
			case "failed_lost_ack_promoted":
				obj(e.Scope["scenarios"])["lost_ack"] = true
			case "watchdog_foreign_run", "watchdog_reported_model_timeout":
				fin := obj(obj(e.Product[1])["payload_json"])
				for _, v := range arr(obj(fin["source_check_observation"])["failed_checks"]) {
					x := obj(v)
					if x["role"] == "task_authority_watchdog" {
						if kind == "watchdog_foreign_run" {
							x["run_id"] = "foreign-run"
						} else {
							x["failure_category"] = "provider_timeout"
						}
					}
				}
				obj(e.Product[1])["payload_sha256"] = hashValue(fin)
				obj(obj(e.Correlation["case_failure_correlations"])["lost_ack"])["source_check_observation"] = clone(fin["source_check_observation"])
				e.Correlation["original_pg_product_witness"] = clone(e.Product)
			case "absence_second_prepare":
				pg := obj(obj(obj(e.Product[0])["payload_json"])["postgres"])
				for _, a := range rows(pg, "actions") {
					if a["tool_name"] == "extension.project.prepare-patch" {
						second := obj(clone(a))
						second["action_id"] = "second-real-write"
						second["effect_id"] = "second-effect"
						pg["actions"] = append(arr(pg["actions"]), second)
						break
					}
				}
			case "absence_missing_notice":
				obj(obj(obj(e.Product[0])["payload_json"])["postgres"])["notices"] = []any{}
			case "absence_catalog_raw_body_hash":
				f := obj(obj(e.Product[0])["payload_json"])
				id := obj(f["original_document"])["source_id"]
				for _, s := range rows(obj(f["postgres"]), "catalog_sources") {
					if s["source_id"] == id {
						s["content_hash"] = digest([]byte(v18OriginalCSV))
					}
				}
			case "absence_private_packet_digest_changed":
				obj(arr(e.Component["all13_original_named_packets"])[0])["private_proposal_sha256"] = strings.Repeat("b", 64)
			}
			if strings.HasPrefix(kind, "absence_") && kind != "absence_private_packet_digest_changed" {
				// Keep the wrapper/correlation/component projection coherent so
				// rejection must reach actual product/source/receipt predicates.
				wrapper := obj(e.Product[0])
				wrapper["payload_sha256"] = hashValue(wrapper["payload_json"])
				e.Component["current_case_payload_sha256"] = wrapper["payload_sha256"]
				e.Correlation["original_pg_product_witness"] = clone(e.Product)
			}
			defer func() {
				if value := recover(); value == nil {
					t.Fatal("historical factual boundary tamper accepted", kind)
				} else if value == "v19_closed_v18_wrapper_hash_order" || value == "v19_closed_v18_raw_projection_identity" || value == "v19_retained_absence_original_source_binding" {
					t.Fatal("tamper rejected only by projection coherence, not critical predicate", kind, value)
				}
			}()
			verifyV19ClosedV18Evidence(e)
		})
	}
}
