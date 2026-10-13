package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"reflect"
	"unicode/utf8"

	"github.com/butler/butler/internal/actionauth"
)

type privateProposal struct {
	Version  string `json:"version"`
	Ordinal  int64  `json:"ordinal"`
	Scenario string `json:"scenario"`
	Kind     string `json:"kind"`
	RunID    string `json:"run_id"`
	ToolCall struct {
		ToolCallRef, ToolName, ArgsJSON string
		SequenceNo                      int
	} `json:"tool_call"`
	Arguments []byte `json:"arguments_bytes_base64"`
}

func proposalLeaf(ordinal int64) string { return fmt.Sprintf("tool-proposal-%06d.json", ordinal) }
func parsePrivateProposal(raw []byte) privateProposal {
	require(len(raw) > 0 && len(raw) <= 262144 && utf8.Valid(raw), "v8_private_proposal_bound")
	// Reuse the global strict argument parser solely for duplicate/depth syntax.
	// It never changes the original private bytes or supplies action authority.
	_, err := actionauth.ArgumentsHash(string(raw))
	require(err == nil, "v8_private_proposal_strict_json")
	d := json.NewDecoder(bytes.NewReader(raw))
	d.DisallowUnknownFields()
	var p privateProposal
	require(d.Decode(&p) == nil && d.Decode(new(any)) == io.EOF, "v8_private_proposal_typed_json")
	require(p.Version == "butler-native-tool-proposal-v1" && p.Ordinal > 0 && p.RunID != "" && p.ToolCall.ToolCallRef != "" && p.ToolCall.ToolName != "", "v8_private_proposal_identity")
	require(utf8.Valid(p.Arguments) && json.Valid(p.Arguments) && string(p.Arguments) == p.ToolCall.ArgsJSON, "v8_original_argument_bytes")
	return p
}
func verifyV8Proposals(in inputs, policy auditPolicy) {
	require(policy.ReadPrivateProposal != nil, "v8_private_proposal_reader_required")
	sc := obj(in.Scope)
	list := arr(sc["proposal_observations"])
	native := arr(obj(sc["scope"])["requests"])
	byOrdinal := map[int64]object{}
	for _, v := range native {
		r := obj(v)
		if r["outcome"] == "completed" && integer(r["final_count"]) == 0 {
			byOrdinal[integer(r["ordinal"])] = r
		}
	}
	require(len(list) == len(byOrdinal), "v8_proposal_native_inventory")
	products := witnesses(in.Product)
	matched := map[string]bool{}
	var previous int64
	for _, v := range list {
		m := obj(v)
		ordinal := integer(m["ordinal"])
		r := byOrdinal[ordinal]
		require(r != nil && ordinal > previous, "v8_proposal_ordinal_duplicate_or_foreign")
		previous = ordinal
		require(m["version"] == "butler-native-tool-proposal-diagnostics-v1" && m["scenario"] == r["scenario"] && m["kind"] == r["kind"] && m["strict_arguments_json"] == true, "v8_proposal_metadata_identity")
		require(integer(m["private_proposal_bytes"]) > 0 && integer(m["private_proposal_bytes"]) <= 262144, "v8_proposal_declared_size")
		raw := policy.ReadPrivateProposal(m)
		require(int64(len(raw)) == integer(m["private_proposal_bytes"]) && digest(raw) == sha(m["private_proposal_sha256"]), "v8_private_proposal_digest")
		p := parsePrivateProposal(raw)
		require(p.Ordinal == ordinal && p.Scenario == r["scenario"] && p.Kind == r["kind"] && p.RunID == r["run_id"], "v8_private_proposal_native_lineage")
		authHash, err := actionauth.ArgumentsHash(p.ToolCall.ArgsJSON)
		require(err == nil && authHash == sha(m["arguments_authorization_sha256"]) && digest(p.Arguments) == sha(m["arguments_sha256"]) && int64(len(p.Arguments)) == integer(m["arguments_bytes"]), "v8_original_arguments_hash")
		fin := products[p.Scenario]
		require(fin != nil, "v8_proposal_product_missing")
		pg := obj(obj(fin["payload_json"])["postgres"])
		matches := 0
		for _, a := range rows(pg, "actions") {
			if a["run_id"] == p.RunID && a["tool_call_id"] == p.ToolCall.ToolCallRef {
				key := text(a["action_id"])
				require(!matched[key] && a["tool_name"] == p.ToolCall.ToolName && sha(a["args_hash"]) == authHash, "v8_original_core_arguments_binding")
				matched[key] = true
				matches++
			}
		}
		require(matches == 1, "v8_proposal_owned_action_exact_once")
		// Public diagnostics are not authority and cannot introduce secret values.
		verifyV8DiagnosticInventory(m, p.ToolCall.ToolName)
	}
	expected := 0
	for _, f := range products {
		expected += len(rows(obj(obj(f["payload_json"])["postgres"]), "actions"))
	}
	require(len(matched) == expected, "v8_action_without_retained_proposal")
}
func verifyV8DiagnosticInventory(m object, tool string) {
	common := "developer_decode_status arguments_sha256 arguments_bytes strict_arguments_json arguments_authorization_sha256 private_proposal_sha256 private_proposal_bytes version ordinal scenario kind"
	if tool != "task.delegate_converter" {
		keys(m, common)
		require(m["developer_decode_status"] == "not_applicable", "v8_non_developer_diagnostic")
		return
	}
	keys(m, common+" tool_class top_level_key_count field_types unexpected_key_count deadline_rfc3339 need_source_field_types contract_source_field_types budget_field_types budget_nonnegative_go_int64 dependency_count")
	require(m["tool_class"] == "converter_developer_admission" && m["developer_decode_status"] == "valid_syntax" && integer(m["top_level_key_count"]) == 5 && integer(m["unexpected_key_count"]) == 0 && m["deadline_rfc3339"] == true, "v8_developer_diagnostic_shape")
	require(reflect.DeepEqual(obj(m["field_types"]), object{"need_source": "object", "contract_source": "object", "budget": "object", "deadline": "string", "dependencies": "array"}), "v8_developer_field_types")
	for _, name := range []string{"need_source_field_types", "contract_source_field_types"} {
		require(reflect.DeepEqual(obj(m[name]), object{"source_id": "string", "revision": "number", "access_revision": "number", "content_hash": "string"}), "v8_reference_field_types")
	}
	for _, name := range []string{"budget_field_types", "budget_nonnegative_go_int64"} {
		o := obj(m[name])
		keys(o, unitKeys)
		for _, value := range o {
			if name == "budget_field_types" {
				require(value == "number", "v8_budget_field_types")
			} else {
				_, ok := value.(bool)
				require(ok, "v8_budget_spelling_diagnostic_boolean")
			}
		}
	}
	require(integer(m["dependency_count"]) <= 8, "v8_dependency_count")
}

func verifyV8PrivateDirectory(scope object) {
	info, err := os.Lstat("/proposals")
	require(err == nil && info.IsDir() && info.Mode()&os.ModeSymlink == 0, "v8_named_proposal_directory")
	allowed := map[string]bool{}
	for _, v := range arr(scope["proposal_observations"]) {
		m := obj(v)
		leaf := proposalLeaf(integer(m["ordinal"]))
		require(!allowed[leaf], "v8_private_leaf_duplicate")
		allowed[leaf] = true
	}
	entries, err := os.ReadDir("/proposals")
	require(err == nil && len(entries) == len(allowed), "v8_private_directory_inventory")
	for _, entry := range entries {
		require(allowed[entry.Name()] && entry.Type().IsRegular(), "v8_private_directory_unexpected_leaf")
	}
}
