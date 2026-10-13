package main

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/butler/butler/internal/actionauth"
	"github.com/butler/butler/internal/security/egress/egresstest"
)

func upgradeV12Witness(t *testing.T, base inputs) (inputs, auditPolicy, map[int64][]byte) {
	t.Helper()
	// Older fixture aliases were never a prospective model catalog contract.
	for i := range arr(base.Product) {
		tamperProduct(&base, i, func(fin object) {
			for _, a := range rows(obj(fin["postgres"]), "actions") {
				if a["tool_name"] == "converter.developer" {
					a["tool_name"] = "task.delegate_converter"
				}
			}
		})
	}
	x, _, oldPrivate := upgradeV11Witness(t, base)
	p := v12AuditPolicy()
	scope, ledger := obj(x.Scope), obj(x.Ledger)
	scope["window_id"], scope["baseline_attempts"], scope["maximum_new_attempts"], scope["maximum_total_attempts"] = p.WindowID, jsonNumber(575), jsonNumber(120), jsonNumber(695)
	ledger["baseline_attempts"] = jsonNumber(575)
	for _, w := range []object{obj(obj(scope["scope"])["window"]), obj(ledger["window"])} {
		w["native_baseline"], w["attempts"], w["maximum_new_attempts"] = jsonNumber(575), jsonNumber(integer(w["attempts"])+18), jsonNumber(120)
		w["prior_history_hash"], w["prerequisite_hash"] = p.PriorSHA, p.PrerequisiteSHA
	}
	obj(scope["scope"])["total_attempts"] = jsonNumber(integer(obj(scope["scope"])["total_attempts"]) + 18)
	ledger["total_attempts"] = jsonNumber(integer(ledger["total_attempts"]) + 18)
	for _, list := range []any{obj(scope["scope"])["requests"], ledger["requests"]} {
		for _, v := range arr(list) {
			n := obj(v)
			n["ordinal"] = jsonNumber(integer(n["ordinal"]) + 18)
		}
	}
	products := witnesses(x.Product)
	private := map[int64][]byte{}
	for _, v := range arr(scope["proposal_observations"]) {
		m := obj(v)
		var rec privateProposal
		if json.Unmarshal(oldPrivate[integer(m["ordinal"])], &rec) != nil {
			t.Fatal("synthetic original proposal invalid")
		}
		fin := obj(products[rec.Scenario]["payload_json"])
		if rec.ToolCall.ToolName == "extension.project.inspect" || rec.ToolCall.ToolName == "extension.project.check" || rec.ToolCall.ToolName == "extension.project.prepare-patch" {
			url := egresstest.Origin + "/project"
			if rec.ToolCall.ToolName == "extension.project.check" {
				url += "/patches/" + text(obj(fin["original_result"])["artifact_id"]) + "/checks"
			}
			argsObject := object{"url": url}
			if rec.ToolCall.ToolName == "extension.project.prepare-patch" {
				argsObject["url"], argsObject["body"] = egresstest.Origin+"/project/patches", object{"project_revision": obj(fin["original_result"])["project_revision"], "changes": []any{object{"path": "README.md", "before": v12Original, "after": v12Prepared}}}
			}
			args := string(canonical(argsObject))
			hash, err := actionauth.ArgumentsHash(args)
			if err != nil {
				t.Fatal(err)
			}
			rec.Arguments, rec.ToolCall.ArgsJSON = []byte(args), args
			m["arguments_sha256"], m["arguments_bytes"], m["arguments_authorization_sha256"] = digest(rec.Arguments), jsonNumber(int64(len(rec.Arguments))), hash
			for _, a := range rows(obj(fin["postgres"]), "actions") {
				if a["run_id"] == rec.RunID && a["tool_call_id"] == rec.ToolCall.ToolCallRef {
					a["args_hash"] = hash
				}
			}
		}
		rec.Ordinal += 18
		raw, _ := json.Marshal(rec)
		private[rec.Ordinal] = raw
		m["ordinal"], m["private_proposal_sha256"], m["private_proposal_bytes"] = jsonNumber(rec.Ordinal), digest(raw), jsonNumber(int64(len(raw)))
	}
	for i := range arr(x.Product) {
		tamperProduct(&x, i, func(fin object) {
			pg := obj(fin["postgres"])
			for _, s := range rows(pg, "budget_scopes") {
				if s["kind"] == "owner_day" {
					limits := obj(s["limits"])
					limits["model_turns"], limits["tokens"], limits["active_millis"] = jsonNumber(120), jsonNumber(120*1081344), jsonNumber(120*300000)
				}
			}
			if fin["original_result"] == nil {
				return
			}
			o := obj(fin["original_result"])
			o["check_receipts"], o["inspect_receipts"] = []any{}, []any{}
			for _, a := range rows(pg, "actions") {
				check := a["tool_name"] == "extension.project.check"
				if !check && a["tool_name"] != "extension.project.inspect" {
					continue
				}
				r := object{"run_id": a["run_id"], "tool_call_id": a["tool_call_id"], "project_revision": o["project_revision"], "checks_passed": check, "applied": nil, "result_sha256": digest([]byte("synthetic-actual-PG-result-" + text(a["tool_call_id"]))), "body_sha256": hashValue(v12Body(text(o["artifact_id"]), check))}
				list := "inspect_receipts"
				if check {
					list = "check_receipts"
					r["artifact_id"], r["applied"] = o["artifact_id"], false
				}
				o[list] = append(arr(o[list]), r)
			}
		})
	}
	x.Prerequisite, x.SafeTimeoutWaiver = frozenObject(v12PrerequisiteSemantic), frozenObject(v12HumanSemantic)
	x = normalizedInputs(x)
	p.ReadPrivateProposal = func(m object) []byte { return private[integer(m["ordinal"])] }
	return x, p, private
}

func TestV12AuditFiveCasesQualifiedReadReceiptsAnd695(t *testing.T) {
	x, p, _ := upgradeV12Witness(t, syntheticWitness(t))
	r, err := auditWithPolicy(x, p)
	if err != nil || r.Status != "CHAIN_V12_WITNESS_AUDIT_PASS" || r.Prior != 575 || r.Total != 645 || r.KnownHistoricalTotal != 1039921 || r.UnknownHistoricalTotal != 49 || len(r.Cases) != 5 || r.Ready || r.AuditorCalls != 0 {
		t.Fatal("V12 read receipt five-case proof rejected/overclaimed", err, r.Reasons)
	}
	if selectedAuditPolicy("v12").Baseline+selectedAuditPolicy("v12").Maximum != 695 || v11AuditPolicy().AllowRepeatedPatchChecks || v11AuditPolicy().Baseline+v11AuditPolicy().Maximum != 628 {
		t.Fatal("old scope/default modified")
	}
}

func TestV12AuditRepeatedReadsNoSeparateCountLimit(t *testing.T) {
	x, _, _ := upgradeV12Witness(t, syntheticWitness(t))
	fin := obj(witnesses(x.Product)["normal_sent"]["payload_json"])
	pg := obj(fin["postgres"])
	actions := rows(pg, "actions")
	runs := byID(rows(pg, "runs"), "run_id")
	root, child := "root-normal_sent", "child-normal_sent"
	o := obj(fin["original_result"])
	for i := 0; i < 8; i++ {
		a := obj(clone(actions[len(actions)-1]))
		a["action_id"], a["effect_id"], a["tool_call_id"] = "additional-read-action-"+string(rune('a'+i)), "additional-read-effect-"+string(rune('a'+i)), "additional-read-call-"+string(rune('a'+i))
		if i%2 == 0 {
			a["tool_name"] = "extension.project.inspect"
		} else {
			a["tool_name"] = "extension.project.check"
		}
		actions = append(actions, a)
		check := a["tool_name"] == "extension.project.check"
		r := obj(clone(arr(o["inspect_receipts"])[0]))
		list := "inspect_receipts"
		if check {
			r = obj(clone(arr(o["check_receipts"])[0]))
			list = "check_receipts"
		}
		r["tool_call_id"] = a["tool_call_id"]
		o[list] = append(arr(o[list]), r)
	}
	o["check_receipt_count"] = jsonNumber(int64(len(arr(o["check_receipts"]))))
	// Pure read predicate only; no claim that these additional actions acquired
	// normal Model/Tool reservations. The full audit independently enforces them.
	verifyV12OriginalResult(fin, actions, runs, root, child, "developer-inventory")
}

func TestV12AuditSafeRecoveredUnknownKeepsMeasuredSuccess(t *testing.T) {
	x, p, _ := upgradeV12Witness(t, waivedStreamTimeout(t, 0, true, true))
	r, err := auditWithPolicy(x, p)
	if err != nil || r.Status != "CHAIN_V12_WITNESS_AUDIT_PASS" || r.AcceptedRetryFailures != 1 || r.AcceptedUnknownCost != 1 || r.AcceptedUnknownVersion != 1 || r.AcceptedUnmeasured != 1 || r.Cost.Unknown != 1 || r.MeasuredComplete || r.UnknownHistoricalTotal != 49 || r.Ready {
		t.Fatal("safe recovered UNKNOWN erased or widened", err, r.Reasons)
	}
}

func TestV12AuditReadReceiptAndCriticalTampering(t *testing.T) {
	for _, kind := range []string{"missing_receipt", "duplicate_receipt", "wrong_run", "wrong_call", "foreign_artifact", "body_conflict", "applied_true", "inspection_applied_claim", "false_check", "unknown_field", "double_prepare", "extra_other_tool", "unknown_effect", "stop_result", "successful_usage_unknown", "unknown_retry_bad_receipt", "foreign_original_url", "wrong_original_prepare"} {
		t.Run(kind, func(t *testing.T) {
			base := syntheticWitness(t)
			if kind == "unknown_retry_bad_receipt" {
				base = waivedStreamTimeout(t, 0, true, true)
			}
			x, p, private := upgradeV12Witness(t, base)
			if kind == "successful_usage_unknown" {
				changeNative(&x, 0, func(n object) { n["actual_input_tokens"] = nil })
			} else {
				index := 0
				if kind == "stop_result" {
					index = 2
				}
				tamperProduct(&x, index, func(fin object) {
					if kind == "stop_result" {
						fin["original_result"] = object{"checks_passed": true}
						return
					}
					pg := obj(fin["postgres"])
					o := obj(fin["original_result"])
					r := obj(arr(o["check_receipts"])[0])
					switch kind {
					case "missing_receipt":
						o["check_receipts"] = []any{}
					case "duplicate_receipt":
						o["check_receipts"] = append(arr(o["check_receipts"]), clone(r))
					case "wrong_run":
						r["run_id"] = "foreign-run"
					case "wrong_call":
						r["tool_call_id"] = "foreign-call"
					case "foreign_artifact":
						r["artifact_id"] = strings.Repeat("c", 64)
					case "body_conflict", "unknown_retry_bad_receipt":
						r["body_sha256"] = strings.Repeat("d", 64)
					case "applied_true":
						r["applied"] = true
					case "inspection_applied_claim":
						obj(arr(o["inspect_receipts"])[0])["applied"] = false
					case "false_check":
						r["checks_passed"] = false
					case "unknown_field":
						r["private_body"] = "PRIVATE_VALUE"
					case "double_prepare", "extra_other_tool":
						a := obj(clone(rows(pg, "actions")[0]))
						a["action_id"], a["effect_id"], a["tool_call_id"] = "another-action", "another-effect", "another-call"
						a["tool_name"] = "extension.project.prepare-patch"
						if kind == "extra_other_tool" {
							a["tool_name"] = "extension.project.arbitrary"
						}
						pg["actions"] = append(arr(pg["actions"]), a)
					case "unknown_effect":
						rows(pg, "actions")[0]["state"] = "effect_unknown"
					}
				})
			}
			if kind == "foreign_original_url" || kind == "wrong_original_prepare" {
				for _, m := range arr(obj(x.Scope)["proposal_observations"]) {
					meta := obj(m)
					var rec privateProposal
					_ = json.Unmarshal(private[integer(meta["ordinal"])], &rec)
					tool := "extension.project.check"
					if kind == "wrong_original_prepare" {
						tool = "extension.project.prepare-patch"
					}
					if rec.ToolCall.ToolName != tool {
						continue
					}
					args := `{"url":"https://foreign.invalid/project"}`
					if kind == "wrong_original_prepare" {
						argsObject := frozenObject(rec.ToolCall.ArgsJSON)
						obj(arr(obj(argsObject["body"])["changes"])[0])["after"] = "PRIVATE_VALUE"
						args = string(canonical(argsObject))
					}
					hash, _ := actionauth.ArgumentsHash(args)
					rec.Arguments, rec.ToolCall.ArgsJSON = []byte(args), args
					raw, _ := json.Marshal(rec)
					private[rec.Ordinal] = raw
					meta["private_proposal_sha256"], meta["private_proposal_bytes"], meta["arguments_authorization_sha256"], meta["arguments_sha256"], meta["arguments_bytes"] = digest(raw), jsonNumber(int64(len(raw))), hash, digest([]byte(args)), jsonNumber(int64(len(args)))
					for i := range arr(x.Product) {
						tamperProduct(&x, i, func(fin object) {
							for _, a := range rows(obj(fin["postgres"]), "actions") {
								if a["run_id"] == rec.RunID && a["tool_call_id"] == rec.ToolCall.ToolCallRef {
									a["args_hash"] = hash
								}
							}
						})
					}
					break
				}
			}
			r, err := auditWithPolicy(x, p)
			if err == nil || r.Status != "NO_GO" || r.Ready || r.AcceptedRetryFailures != 0 || r.AcceptedUnknownCost != 0 || r.AcceptedUnknownVersion != 0 || r.AcceptedUnmeasured != 0 {
				t.Fatal("V12 unsafe read/effect accepted", kind, err, r.Reasons)
			}
			if kind == "unknown_retry_bad_receipt" && r.Cost.Unknown != 1 {
				t.Fatal("factual unknown cost erased on deny")
			}
			if strings.Contains(strings.Join(r.Reasons, ","), "PRIVATE_VALUE") {
				t.Fatal("private field echoed")
			}
		})
	}
}

func TestV12AuditRetained575AndUnknownLegacyReads(t *testing.T) {
	verifyRetainedFailedV11Source(candidateRepo(t))
}

func TestV12AuditHistorical575Tampering(t *testing.T) {
	for _, kind := range []string{"old_usage", "new_cost", "read_unknown_filled", "old_no_go_pass", "old_ceiling_refunded", "old_source_cause", "original_proposal_match"} {
		t.Run(kind, func(t *testing.T) {
			all := readRetainedFailedV11(candidateRepo(t))
			switch kind {
			case "old_usage":
				obj(arr(obj(all["history"])["usage"])[536])["actual_input_tokens"] = jsonNumber(1)
			case "new_cost":
				obj(arr(obj(obj(all["high"])["scope"])["requests"])[0])["actual_total_tokens"] = jsonNumber(1)
			case "read_unknown_filled":
				obj(obj(witnesses(all["pg-product"])["normal_sent"]["payload_json"])["original_result"])["checks_passed"] = true
			case "old_no_go_pass":
				obj(obj(all["final-audit"])["chain_audit"])["status"] = "PASS"
			case "old_ceiling_refunded":
				obj(all["bridge"])["remaining_original_absolute628_attempts"] = jsonNumber(71)
			case "old_source_cause":
				obj(obj(all["failure-correlation"])["failure_classification"])["specific_prior557_source_cause"] = "CONFIRMED"
			case "original_proposal_match":
				obj(all["proposals"])["all_fourteen_core_action_fingerprints_match"] = false
			}
			defer func() {
				if recover() == nil {
					t.Fatal("historical V11 failure changed", kind)
				}
			}()
			verifyRetainedFailedV11(all)
		})
	}
}

func TestV12AuditRawProtocolPins(t *testing.T) {
	for _, item := range []struct{ file, pin, semantic string }{{"release-chain-v12-high-prerequisite-20261013.json", v12PrerequisiteSHA, v12PrerequisiteSemantic}, {"release-chain-v12-user-policy-amendment-20261013.json", v12HumanSHA, v12HumanSemantic}} {
		v := actualPublic(t, item.file, item.pin)
		if !reflect.DeepEqual(obj(v), frozenObject(item.semantic)) {
			t.Fatal("raw/semantic prospective protocol differs")
		}
	}
}
