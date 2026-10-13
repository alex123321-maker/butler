package main

import (
	"encoding/json"
	"reflect"
	"sort"
	"strings"
	"testing"
	"time"
)

func shiftV20Times(v any, delta time.Duration) {
	switch x := v.(type) {
	case map[string]any:
		for k, value := range x {
			if s, ok := value.(string); ok {
				if at, e := time.Parse(time.RFC3339Nano, s); e == nil {
					x[k] = at.Add(delta).Format(time.RFC3339Nano)
					continue
				}
			}
			shiftV20Times(value, delta)
		}
	case []any:
		for _, value := range x {
			shiftV20Times(value, delta)
		}
	}
}
func upgradeV20Witness(t *testing.T, base inputs) (inputs, auditPolicy) {
	t.Helper()
	x, old := upgradeV16Witness(t, base)
	p := v20AuditPolicy()
	scope, ledger := obj(x.Scope), obj(x.Ledger)
	keep := map[string]bool{"lost_ack": true, "restart_sent": true}
	delta := map[string]time.Duration{"lost_ack": -15 * time.Second, "restart_sent": 0}
	initial, products := []any{}, []any{}
	for _, value := range arr(x.Initial) {
		w := obj(value)
		name := text(w["scenario"])
		if !keep[name] {
			continue
		}
		shiftV20Times(w, delta[name])
		setup := obj(obj(w["payload_json"])["postgres"])
		for _, a := range rows(setup, "setup_actions") {
			a["requires_approval"], a["tool_name"] = false, "extension.project.inspect"
		}
		doc := obj(obj(obj(w["payload_json"])["original_refs"])["document"])
		doc["content_hash"] = v18DocumentRevisionHash(text(doc["source_id"]))
		w["payload_sha256"] = hashValue(w["payload_json"])
		initial = append(initial, w)
	}
	x.Initial = initial
	for _, value := range arr(x.Product) {
		w := obj(value)
		name := text(w["scenario"])
		if !keep[name] {
			continue
		}
		shiftV20Times(w, delta[name])
		fin := obj(w["payload_json"])
		fin["original_result_capture_status"] = "captured"
		db := obj(fin["postgres"])
		var refs object
		for _, in := range initial {
			if obj(in)["scenario"] == name {
				refs = obj(obj(obj(in)["payload_json"])["original_refs"])
			}
		}
		sources := byID(rows(db, "catalog_sources"), "source_id")
		doc := obj(refs["document"])
		fin["original_document"] = clone(doc)
		sources[text(doc["source_id"])]["content_hash"] = doc["content_hash"]
		version := rows(db, "converter_versions")[0]
		for _, table := range []string{"converter_trials", "converter_usages"} {
			for _, conversion := range rows(db, table) {
				if table == "converter_trials" {
					conversion["input_ref_json"] = clone(doc)
				}
				ref := obj(conversion["output_ref_json"])
				hash := v18DerivedConversionHash(ref, version, refs)
				ref["content_hash"], sources[text(ref["source_id"])]["content_hash"] = hash, hash
			}
		}
		w["payload_sha256"] = hashValue(fin)
		products = append(products, w)
	}
	rank := map[string]int{"lost_ack": 0, "restart_sent": 1}
	sort.Slice(initial, func(i, j int) bool {
		return rank[text(obj(initial[i])["scenario"])] < rank[text(obj(initial[j])["scenario"])]
	})
	sort.Slice(products, func(i, j int) bool {
		return rank[text(obj(products[i])["scenario"])] < rank[text(obj(products[j])["scenario"])]
	})
	x.Initial = initial
	x.Product = products
	oldNative, oldSQL := arr(obj(scope["scope"])["requests"]), arr(ledger["requests"])
	native, sql := []any{}, []any{}
	ordinals := map[int64]int64{}
	for _, name := range p.ScenarioNames {
		for i, value := range oldNative {
			n := obj(value)
			if n["scenario"] != name {
				continue
			}
			r := obj(oldSQL[i])
			shiftV20Times(n, delta[name])
			shiftV20Times(r, delta[name])
			oldOrdinal := integer(n["ordinal"])
			next := int64(p.Baseline + 1 + len(native))
			ordinals[oldOrdinal] = next
			n["ordinal"], r["ordinal"] = jsonNumber(next), jsonNumber(next)
			n["provider_unavailable_class"], r["provider_unavailable_class"] = nil, nil
			native = append(native, n)
			sql = append(sql, r)
		}
	}
	obj(scope["scope"])["requests"], ledger["requests"] = native, sql
	for _, w := range []object{obj(obj(scope["scope"])["window"]), obj(ledger["window"])} {
		w["native_baseline"], w["attempts"] = jsonNumber(int64(p.Baseline)), jsonNumber(int64(p.Baseline+len(native)))
		w["prior_history_hash"], w["prerequisite_hash"] = p.PriorSHA, p.PrerequisiteSHA
		w["protocol_policy"] = clone(p.WindowPolicy)
	}
	for _, o := range []object{obj(scope["scope"]), ledger} {
		o["total_attempts"], o["new_attempts"] = jsonNumber(int64(p.Baseline+len(native))), jsonNumber(int64(len(native)))
	}
	scope["window_id"], scope["baseline_attempts"], scope["maximum_total_attempts"] = p.WindowID, jsonNumber(int64(p.Baseline)), jsonNumber(int64(p.Baseline+p.Maximum))
	ledger["baseline_attempts"] = jsonNumber(int64(p.Baseline))
	scope["scenarios"] = object{"lost_ack": true, "restart_sent": true}
	metadata := []any{}
	private := map[int64][]byte{}
	for _, value := range arr(scope["proposal_observations"]) {
		m := obj(value)
		if !keep[text(m["scenario"])] {
			continue
		}
		var rec privateProposal
		if json.Unmarshal(old.ReadPrivateProposal(m), &rec) != nil {
			t.Fatal("synthetic original packet invalid")
		}
		rec.Ordinal = ordinals[rec.Ordinal]
		raw, _ := json.Marshal(rec)
		private[rec.Ordinal] = raw
		m["ordinal"], m["private_proposal_sha256"], m["private_proposal_bytes"] = jsonNumber(rec.Ordinal), digest(raw), jsonNumber(int64(len(raw)))
		metadata = append(metadata, m)
	}
	sort.Slice(metadata, func(i, j int) bool {
		return integer(obj(metadata[i])["ordinal"]) < integer(obj(metadata[j])["ordinal"])
	})
	scope["proposal_observations"] = metadata
	x.Prerequisite, x.SafeTimeoutWaiver = frozenObject(v20PrerequisiteSemantic), frozenObject(v20HumanSemantic)
	x = normalizedInputs(x)
	p.ReadPrivateProposal = func(m object) []byte { return private[integer(m["ordinal"])] }
	return x, p
}
func TestV20AuditPendingTwoAndRetainedComposite(t *testing.T) {
	x, p := upgradeV20Witness(t, syntheticWitness(t))
	r, err := auditWithPolicy(x, p)
	if err != nil || r.Status != p.PassStatus || !reflect.DeepEqual(r.Cases, []string{"lost_ack", "restart_sent"}) || !reflect.DeepEqual(r.RetainedCases, []string{"owner_stop", "explicit_absence"}) || r.CommonCompletionEvidence != "restart_sent_current_owner_result_and_single_sent_notice" || r.Ready || r.AuditorCalls != 0 {
		t.Fatal("current final composite invalid", err, r.Reasons)
	}
	if selectedAuditPolicy("v16").RequireUnavailableClass || strings.Contains(strings.Join(r.Cases, ","), "normal_sent") {
		t.Fatal("old protocol or normal result rewritten")
	}
}
func TestV20AuditRawPinsAndHistorical706Unknown(t *testing.T) {
	p := v20AuditPolicy()
	p.VerifyPrerequisite(obj(actualPublic(t, "release-chain-v20-high-prerequisite-20261013.json", v20PrerequisiteSHA)))
	p.VerifyHuman(inputs{ExpenseException: syntheticWitness(t).ExpenseException, SafeTimeoutWaiver: actualPublic(t, "release-chain-v20-user-policy-amendment-20261013.json", v20HumanSHA)})
	root := candidateRepo(t)
	verifyV20Closed706(obj(pinnedObject(root, "release-chain-v18-history705-20261013.json", v19ClosedV18HistorySHA)), obj(pinnedObject(root, "release-chain-v19-history706-20261013.json", v20ClosedHistorySHA)), obj(pinnedObject(root, "release-chain-v19-carry706-independent-20261013.json", v20ClosedBridgeSHA)), obj(pinnedObject(root, "release-chain-v19-final-audit-20261013.json", v20ClosedFinalSHA)), obj(pinnedObject(root, "release-chain-v19-failure-correlation-20261013.json", v20ClosedCorrelationSHA)))
}
func TestV20AuditCriticalAndDiagnosticTamper(t *testing.T) {
	for _, kind := range []string{"missing_restart", "invented_normal", "success_class", "raw_message_class", "backend_failure_is_not_retry", "window_flag_removed", "invalid_capture", "duplicate_prepare", "lost_ack_sent", "original_source_raw_hash"} {
		t.Run(kind, func(t *testing.T) {
			x, p := upgradeV20Witness(t, syntheticWitness(t))
			scope := obj(x.Scope)
			switch kind {
			case "missing_restart":
				delete(obj(scope["scenarios"]), "restart_sent")
			case "invented_normal":
				obj(scope["scenarios"])["normal_sent"] = true
			case "window_flag_removed":
				delete(obj(obj(obj(scope["scope"])["window"])["protocol_policy"]), "allow_independent_closed_cases")
			case "success_class", "raw_message_class", "backend_failure_is_not_retry":
				n := obj(arr(obj(scope["scope"])["requests"])[0])
				r := obj(arr(obj(x.Ledger)["requests"])[0])
				for _, row := range []object{n, r} {
					if kind == "success_class" {
						row["provider_unavailable_class"] = "backend_unavailable"
					} else if kind == "raw_message_class" {
						row["provider_unavailable_class"] = "PRIVATE_MESSAGE_TOKEN"
					} else {
						row["provider_unavailable_class"], row["diagnostic_code"], row["outcome"] = "backend_unavailable", "provider_unavailable", "transport_error"
						row["provider_operation_stage"] = "http_round_trip"
					}
				}
			default:
				tamperProduct(&x, 0, func(fin object) {
					db := obj(fin["postgres"])
					switch kind {
					case "invalid_capture":
						fin["original_result_capture_status"] = "invalid_or_incomplete"
					case "duplicate_prepare":
						for _, a := range rows(db, "actions") {
							if a["tool_name"] == "extension.project.prepare-patch" {
								extra := obj(clone(a))
								extra["action_id"] = "extra-effect"
								db["actions"] = append(arr(db["actions"]), extra)
								break
							}
						}
					case "lost_ack_sent":
						rows(db, "notices")[0]["status"] = "sent"
					case "original_source_raw_hash":
						obj(fin["original_document"])["content_hash"] = digest([]byte(v18OriginalCSV))
					}
				})
			}
			r, err := auditWithPolicy(x, p)
			if err == nil || r.Status != "NO_GO" || r.CommonCompletionEvidence != "" || len(r.RetainedCases) != 0 || r.AcceptedRetryFailures != 0 || r.AcceptedUnknownCost != 0 {
				t.Fatal("critical denial or global result rewritten", kind, err, r.Reasons)
			}
		})
	}
}
