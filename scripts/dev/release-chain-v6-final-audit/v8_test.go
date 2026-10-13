package main

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/butler/butler/internal/actionauth"
)

func upgradeV8Witness(t *testing.T, x inputs) (inputs, auditPolicy, map[int64][]byte) {
	t.Helper()
	x = upgradeV7Witness(t, x)
	p := v8AuditPolicy()
	scope, ledger := obj(x.Scope), obj(x.Ledger)
	scope["window_id"], scope["baseline_attempts"], scope["maximum_new_attempts"] = p.WindowID, jsonNumber(541), jsonNumber(74)
	ledger["baseline_attempts"] = jsonNumber(541)
	for _, w := range []object{obj(obj(scope["scope"])["window"]), obj(ledger["window"])} {
		w["native_baseline"], w["attempts"], w["maximum_new_attempts"] = jsonNumber(541), jsonNumber(integer(w["attempts"])+2), jsonNumber(74)
		w["prior_history_hash"], w["prerequisite_hash"], w["protocol_policy"] = p.PriorSHA, p.PrerequisiteSHA, clone(p.WindowPolicy)
	}
	obj(scope["scope"])["total_attempts"] = jsonNumber(integer(obj(scope["scope"])["total_attempts"]) + 2)
	ledger["total_attempts"] = jsonNumber(integer(ledger["total_attempts"]) + 2)
	for _, list := range []any{obj(scope["scope"])["requests"], ledger["requests"]} {
		for _, v := range arr(list) {
			r := obj(v)
			r["ordinal"] = jsonNumber(integer(r["ordinal"]) + 2)
		}
	}
	rawFiles := map[int64][]byte{}
	observations := []any{}
	used := map[int64]bool{}
	for j := range arr(x.Product) {
		tamperProduct(&x, j, func(fin object) {
			pg := obj(fin["postgres"])
			for _, s := range rows(pg, "budget_scopes") {
				if s["kind"] == "owner_day" {
					limit := obj(s["limits"])
					limit["model_turns"], limit["tokens"], limit["active_millis"] = jsonNumber(74), jsonNumber(74*1081344), jsonNumber(74*300000)
				}
			}
			for _, a := range rows(pg, "actions") {
				var match object
				for _, v := range arr(obj(scope["scope"])["requests"]) {
					n := obj(v)
					if n["outcome"] == "completed" && integer(n["final_count"]) == 0 && n["run_id"] == a["run_id"] && !used[integer(n["ordinal"])] && !moment(n["finished_at_utc"]).After(moment(a["created_at"])) {
						if match == nil || moment(n["finished_at_utc"]).After(moment(match["finished_at_utc"])) {
							match = n
						}
					}
				}
				if match == nil {
					t.Fatal("synthetic action has no native packet")
				}
				ordinal := integer(match["ordinal"])
				used[ordinal] = true
				args := `{"synthetic":true,"n":1.0}`
				tool := text(a["tool_name"])
				if tool == "task.delegate_converter" {
					refs := `{"source_id":"need","revision":1,"access_revision":1,"content_hash":"` + strings.Repeat("a", 64) + `"}`
					args = `{"need_source":` + refs + `,"contract_source":` + strings.Replace(refs, `"need"`, `"spec"`, 1) + `,"budget":{"model_turns":2.0,"tokens":4096,"cost_micros":0,"external_calls":4,"children":0,"active_millis":1320000,"egress_bytes":4096,"provider_input_bytes":4096},"deadline":"2026-10-14T00:00:00Z","dependencies":[]}`
				}
				hash, e := actionauth.ArgumentsHash(args)
				if e != nil {
					t.Fatal(e)
				}
				a["args_hash"] = hash
				record := privateProposal{Version: "butler-native-tool-proposal-v1", Ordinal: ordinal, Scenario: text(match["scenario"]), Kind: text(match["kind"]), RunID: text(match["run_id"]), Arguments: []byte(args)}
				record.ToolCall.ToolCallRef = text(a["tool_call_id"])
				record.ToolCall.ToolName = tool
				record.ToolCall.ArgsJSON = args
				raw, e := json.Marshal(record)
				if e != nil {
					t.Fatal(e)
				}
				rawFiles[ordinal] = raw
				meta := object{"developer_decode_status": "not_applicable", "arguments_sha256": digest([]byte(args)), "arguments_bytes": len(args), "strict_arguments_json": true, "arguments_authorization_sha256": hash, "private_proposal_sha256": digest(raw), "private_proposal_bytes": len(raw), "version": "butler-native-tool-proposal-diagnostics-v1", "ordinal": ordinal, "scenario": match["scenario"], "kind": match["kind"]}
				if tool == "task.delegate_converter" {
					meta["developer_decode_status"] = "valid_syntax"
					meta["tool_class"] = "converter_developer_admission"
					meta["top_level_key_count"] = 5
					meta["unexpected_key_count"] = 0
					meta["deadline_rfc3339"] = true
					meta["field_types"] = object{"need_source": "object", "contract_source": "object", "budget": "object", "deadline": "string", "dependencies": "array"}
					for _, k := range []string{"need_source_field_types", "contract_source_field_types"} {
						meta[k] = object{"source_id": "string", "revision": "number", "access_revision": "number", "content_hash": "string"}
					}
					types, spelling := object{}, object{}
					for _, k := range strings.Fields(unitKeys) {
						types[k] = "number"
						spelling[k] = true
					}
					spelling["model_turns"] = false
					meta["budget_field_types"], meta["budget_nonnegative_go_int64"], meta["dependency_count"] = types, spelling, 0
				}
				observations = append(observations, meta)
			}
		})
	}
	// Store source-order metadata, as the actual collector exports it.
	for i := 0; i < len(observations); i++ {
		for j := i + 1; j < len(observations); j++ {
			if obj(observations[i])["ordinal"].(int64) > obj(observations[j])["ordinal"].(int64) {
				observations[i], observations[j] = observations[j], observations[i]
			}
		}
	}
	scope["proposal_observations"] = observations
	x.Prerequisite = frozenObject(v8PrerequisiteSemantic)
	x.SafeTimeoutWaiver = frozenObject(v8HumanSemantic)
	x = normalizedInputs(x)
	p.ReadPrivateProposal = func(m object) []byte { return rawFiles[integer(m["ordinal"])] }
	return x, p, rawFiles
}
func TestV8AuditFiveCasesOriginalProposalBinding(t *testing.T) {
	x, p, _ := upgradeV8Witness(t, syntheticWitness(t))
	r, e := auditWithPolicy(x, p)
	if e != nil || r.Status != "CHAIN_V8_WITNESS_AUDIT_PASS" || r.Prior != 541 || len(r.Cases) != 5 || r.Ready {
		t.Fatal("five cases or original args lost", e, r.Reasons)
	}
}
func TestV8AuditProposalTampering(t *testing.T) {
	for _, name := range []string{"missing_private", "changed_private_bytes", "wrong_ordinal", "wrong_run", "missing_public_observation", "wrong_arguments_hash", "different_core_args", "duplicate_observation", "retention_policy_disabled", "changed_capture_limit", "different_private_arguments", "different_tool_ref", "inconsistent_argument_bytes", "invalid_original_utf8"} {
		t.Run(name, func(t *testing.T) {
			x, p, raw := upgradeV8Witness(t, syntheticWitness(t))
			list := arr(obj(x.Scope)["proposal_observations"])
			m := obj(list[0])
			ordinal := integer(m["ordinal"])
			switch name {
			case "different_private_arguments", "different_tool_ref", "inconsistent_argument_bytes", "invalid_original_utf8":
				var proposal privateProposal
				_ = json.Unmarshal(raw[ordinal], &proposal)
				switch name {
				case "different_private_arguments":
					proposal.ToolCall.ArgsJSON = `{"changed":true}`
					proposal.Arguments = []byte(proposal.ToolCall.ArgsJSON)
				case "different_tool_ref":
					proposal.ToolCall.ToolCallRef = "foreign-ref"
				case "inconsistent_argument_bytes":
					proposal.Arguments = []byte(`{"changed":true}`)
				case "invalid_original_utf8":
					proposal.Arguments = []byte{0xff}
				}
				raw[ordinal], _ = json.Marshal(proposal)
				m["private_proposal_sha256"], m["private_proposal_bytes"] = digest(raw[ordinal]), jsonNumber(int64(len(raw[ordinal])))
			case "missing_private":
				delete(raw, ordinal)
			case "changed_private_bytes":
				raw[ordinal] = append(raw[ordinal], byte(' '))
			case "wrong_ordinal":
				m["ordinal"] = jsonNumber(999)
			case "wrong_run":
				var r privateProposal
				_ = json.Unmarshal(raw[ordinal], &r)
				r.RunID = "foreign-run"
				raw[ordinal], _ = json.Marshal(r)
				m["private_proposal_sha256"], m["private_proposal_bytes"] = digest(raw[ordinal]), jsonNumber(int64(len(raw[ordinal])))
			case "missing_public_observation":
				obj(x.Scope)["proposal_observations"] = list[1:]
			case "wrong_arguments_hash":
				m["arguments_authorization_sha256"] = strings.Repeat("f", 64)
			case "different_core_args":
				tamperProduct(&x, 0, func(fin object) { rows(obj(fin["postgres"]), "actions")[0]["args_hash"] = strings.Repeat("f", 64) })
			case "duplicate_observation":
				obj(x.Scope)["proposal_observations"] = append(list, list[0])
			case "retention_policy_disabled":
				obj(obj(obj(obj(x.Scope)["scope"])["window"])["protocol_policy"])["proposal_retention_required"] = false
			case "changed_capture_limit":
				obj(obj(obj(obj(x.Scope)["scope"])["window"])["protocol_policy"])["maximum_private_proposal_bytes"] = jsonNumber(262145)
			}
			if r, e := auditWithPolicy(x, p); e == nil || r.Status != "NO_GO" {
				t.Fatal("changed original proposal passed")
			}
		})
	}
}
func TestV8AuditRetained541History(t *testing.T) { verifyRetainedFailedV7bSource(candidateRepo(t)) }

func TestV8AuditDeniedProposalClearsAcceptedTimeoutSubsets(t *testing.T) {
	x, p, raw := upgradeV8Witness(t, waivedStreamTimeout(t, 0, true, true))
	for j, v := range arr(obj(obj(x.Scope)["scope"])["requests"]) {
		if obj(v)["outcome"] != "completed" {
			changeNative(&x, j, func(n object) { n["provider_operation_stage"] = "http_round_trip" })
			break
		}
	}
	r, err := auditWithPolicy(x, p)
	if err != nil || r.Status != "CHAIN_V8_WITNESS_AUDIT_PASS" || r.AcceptedRetryFailures != 1 || r.AcceptedUnknownCost != 1 || r.AcceptedUnknownVersion != 1 || r.AcceptedUnmeasured != 1 || r.Cost.Unknown != 1 {
		t.Fatal("safe recovered timeout prerequisite failed", err, r.Reasons)
	}
	m := obj(arr(obj(x.Scope)["proposal_observations"])[0])
	ordinal := integer(m["ordinal"])
	raw[ordinal] = append(raw[ordinal], byte(' '))
	r, err = auditWithPolicy(x, p)
	if err == nil || r.Status != "NO_GO" || r.AcceptedRetryFailures != 0 || r.AcceptedUnknownCost != 0 || r.AcceptedUnknownVersion != 0 || r.AcceptedUnmeasured != 0 || r.Cost.Unknown != 1 || r.Cost.UnknownInput != 1 || r.Cost.UnknownOutput != 1 || r.MeasuredComplete || r.Ready {
		t.Fatal("denied proposal retained accepted subset or lost factual unknown cost", err, r.Reasons)
	}
}
