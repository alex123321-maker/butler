package main

import "github.com/butler/butler/internal/security/egress/egresstest"

const v12ProjectRef = "extension-project-fixture"
const v12Original = "Butler reviewed fixture\nready=false\n"
const v12Prepared = "Butler reviewed fixture\nready=true\n"

func v12Body(artifact string, check bool) object {
	revision := digest([]byte(v12ProjectRef + "\x00" + v12Original))
	status, kind := "failed", "ready_flag_false"
	if check {
		status, kind = "passed", ""
	}
	body := object{"observation_format": "project_check_v1", "project_ref": v12ProjectRef, "revision_sha256": revision, "project_revision": revision, "checks_passed": check, "produced_by_butler": check, "error_kind": kind, "status": status, "check_id": digest([]byte(v12ProjectRef + "\x00" + revision + "\x00ready_flag\x00" + status))}
	if check {
		body["artifact_id"], body["applied"], body["checks"] = artifact, false, []any{object{"name": "ready_flag", "status": "passed"}}
	} else {
		body["files"] = []any{object{"path": "README.md", "content": v12Original}}
	}
	return body
}

// All read counts remain finite through the existing eight-dimensional budget
// verifier. This adds no special per-GET count or global tool-call ID constraint.
func verifyV12OriginalResult(fin object, actions []object, runs map[string]object, root, child, converter string) {
	sha(fin["patch_artifact_file_sha256"])
	o := obj(fin["original_result"])
	keys(o, "patch_count artifact_id project_revision changes patch_tool_call_id check_tool_call_id prepare_receipt_count check_receipt_count prepare_receipt_artifact_id check_receipt_artifact_id checks_passed applied check_receipts inspect_receipts")
	artifact := sha(o["artifact_id"])
	require(integer(o["patch_count"]) == 1 && integer(o["prepare_receipt_count"]) == 1 && o["checks_passed"] == true && o["applied"] == false && o["prepare_receipt_artifact_id"] == artifact && o["check_receipt_artifact_id"] == artifact, "v12_actual_original_patch_receipt")
	revision := digest([]byte(v12ProjectRef + "\x00" + v12Original))
	require(o["project_revision"] == revision, "original_project_revision")
	changes := arr(o["changes"])
	require(len(changes) == 1, "one_original_patch")
	c := obj(changes[0])
	keys(c, "path before_sha256 after_sha256")
	require(c["path"] == "README.md" && c["before_sha256"] == digest([]byte(v12Original)) && c["after_sha256"] == digest([]byte(v12Prepared)), "exact_prepared_original_bytes")
	counts := map[string]int{}
	byCall := map[string]object{}
	prepare := 0
	for _, a := range actions {
		run := runs[text(a["run_id"])]
		require(run != nil, "v12_action_run")
		key := text(a["run_id"]) + "\x00" + text(a["tool_call_id"])
		require(byCall[key] == nil, "v12_qualified_action_duplicate")
		byCall[key] = a
		prefix := "child:"
		if run["task_id"] == root {
			prefix = "root:"
		} else {
			require(run["task_id"] == child, "v12_foreign_action_task")
		}
		counts[prefix+text(a["tool_name"])]++
		if a["tool_name"] == "extension.project.prepare-patch" {
			require(prefix == "root:" && a["tool_call_id"] == o["patch_tool_call_id"], "v12_patch_actual_broker_action")
			prepare++
		}
	}
	for _, name := range []string{"converters.discover", "task.delegate_converter", "converters.collect", "converters.test", "converters.trial", "converters.activate", "converter." + converter + ".convert", "extension.project.prepare-patch"} {
		require(counts["root:"+name] == 1, "v12_mandatory_effect_once")
		delete(counts, "root:"+name)
	}
	for _, name := range []string{"converters.discover", "converters.develop", "converters.test"} {
		require(counts["child:"+name] == 1, "v12_mandatory_child_effect_once")
		delete(counts, "child:"+name)
	}
	require(prepare == 1, "v12_single_prepared_write")
	for _, pair := range []struct {
		list, tool string
		check      bool
	}{{"inspect_receipts", "extension.project.inspect", false}, {"check_receipts", "extension.project.check", true}} {
		reads := rows(o, pair.list)
		require(len(reads) >= 1 && len(reads) == counts["root:"+pair.tool], "v12_read_receipt_inventory")
		seen := map[string]bool{}
		for i, r := range reads {
			fields := "run_id tool_call_id project_revision checks_passed applied result_sha256 body_sha256"
			if pair.check {
				fields += " artifact_id"
			}
			keys(r, fields)
			key := text(r["run_id"]) + "\x00" + text(r["tool_call_id"])
			a := byCall[key]
			require(!seen[key] && a != nil && a["tool_name"] == pair.tool && runs[text(a["run_id"])]["task_id"] == root, "v12_read_qualified_action_binding")
			seen[key] = true
			sha(r["result_sha256"])
			require(r["project_revision"] == revision && r["body_sha256"] == hashValue(v12Body(artifact, pair.check)) && r["checks_passed"] == pair.check, "v12_actual_read_body_agreement")
			if pair.check {
				require(r["artifact_id"] == artifact && r["applied"] == false, "v12_same_unapplied_patch_read")
				if i == 0 {
					require(r["tool_call_id"] == o["check_tool_call_id"], "v12_canonical_check_receipt")
				}
			} else {
				require(r["applied"] == nil, "v12_inspection_no_applied_claim")
			}
		}
		delete(counts, "root:"+pair.tool)
	}
	require(len(counts) == 0 && integer(o["check_receipt_count"]) == int64(len(arr(o["check_receipts"]))), "v12_unknown_or_missing_logical_action")
}

// The retained original proposal proves the fixed target and unchanged raw
// authorization fingerprint. Receipt digests are actual PG capture evidence,
// not a claim that private HTTP/SSE bodies were independently imported.
func verifyV12ReadProposal(fin object, p privateProposal) {
	if p.ToolCall.ToolName != "extension.project.check" && p.ToolCall.ToolName != "extension.project.inspect" && p.ToolCall.ToolName != "extension.project.prepare-patch" {
		return
	}
	o := obj(fin["original_result"])
	value, err := decode(p.Arguments)
	require(err == nil, "v12_read_proposal_json")
	args := obj(value)
	if p.ToolCall.ToolName == "extension.project.prepare-patch" {
		keys(args, "url body")
		require(args["url"] == egresstest.Origin+"/project/patches", "v12_prepare_fixed_owned_target")
		body := obj(args["body"])
		keys(body, "project_revision changes")
		require(body["project_revision"] == o["project_revision"], "v12_prepare_current_revision")
		changes := arr(body["changes"])
		require(len(changes) == 1, "v12_one_prepared_change")
		c := obj(changes[0])
		keys(c, "path before after")
		require(c["path"] == "README.md" && c["before"] == v12Original && c["after"] == v12Prepared, "v12_original_prepare_argument_bytes")
		return
	}
	keys(args, "url")
	want := egresstest.Origin + "/project"
	if p.ToolCall.ToolName == "extension.project.check" {
		want += "/patches/" + sha(o["artifact_id"]) + "/checks"
	}
	require(args["url"] == want, "v12_read_fixed_owned_target")
}
