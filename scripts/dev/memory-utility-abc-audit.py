#!/usr/bin/env python3
"""Offline stdlib audit of frozen ABC artifacts; no inference, SQL, or auth access.

Python is host-side evidence/report glue only. This does not re-score a window or
change Core, Broker, transport, source eligibility, or primary denominators.
"""
import argparse
import collections
import hashlib
import json
import pathlib


def read(path):
    return json.loads(path.read_text())


def digest(path):
    return hashlib.sha256(path.read_bytes()).hexdigest()


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("root", type=pathlib.Path)
    parser.add_argument("output", type=pathlib.Path)
    args = parser.parse_args()
    report_path = args.root / "public-report.json"
    report = read(report_path)
    assert report["scheduled"] == 72 and report["all_slots_observed"]
    assert report["raw_pg_entries_correlated"] == report["native_methods_dispatched"]
    rows = []
    for row in report["rows"]:
        slot = args.root / "out/actual" / ("slot-%03d" % row["sequence"])
        for name, expected in row["artifact_hashes"].items():
            assert digest(slot / name) == expected, (row["sequence"], name)
        snapshot = read(slot / "snapshot.private.json")
        binding = read(slot / "binding.json")
        tables = snapshot["Tables"]
        finals = []
        for final in tables["finals"]:
            checks = {
                "run": final["run_id"] == snapshot["RunID"],
                "task": final["task_id"] == binding["TaskID"],
                "session": final["session_key"] == binding["SessionKey"],
                "task_revision": final["task_revision"] == binding["TaskRevision"],
                "constraints": final["constraints_revision"] == binding["ConstraintsRevision"],
                "content_hash": hashlib.sha256(final["content"].encode()).hexdigest() == final["content_hash"],
                "answer_state": final["answer_state"] == "final",
            }
            try:
                content = json.loads(final["content"])
            except ValueError:
                content = None
            cited = content.get("sources") if isinstance(content, dict) else None
            finals.append({"checks": checks, "answer_state": final["answer_state"],
                           "json_object": isinstance(content, dict),
                           "cited_sources": len(cited) if isinstance(cited, list) else None})
        native = [read(path) for path in sorted(slot.glob("native-*.private.json"))]
        finishes = [event["AssistantFinal"]["FinishReason"]
                    for entry in native for event in entry["events"]
                    if event.get("AssistantFinal")]
        inputs = [{key: value.get(key) for key in ("status", "checkpoint", "error_code")}
                  for value in tables["inputs"]]
        failed = [{"tool": call["tool_name"], "status": call["status"]}
                  for call in tables["calls"] if call["status"] != "completed"]
        rows.append({"sequence": row["sequence"], "arm": row["arm"],
                     "family": row["family"], "condition": row["condition"],
                     "frozen_success": row["task_success"], "frozen_reasons": row["reasons"],
                     "projection_error": row["projection_error"], "finals": finals,
                     "provider_final_finish_reasons": finishes, "inputs": inputs,
                     "failed_tools": failed})
    assert len(rows) == 72
    errors = collections.Counter(value["error_code"] for row in rows for value in row["inputs"]
                                 if value["error_code"])
    projections = collections.Counter(row["projection_error"] for row in rows)
    final_checks = collections.Counter(key for row in rows for final in row["finals"]
                                      for key, value in final["checks"].items() if not value)
    slices = {}
    for family in sorted({row["family"] for row in rows}):
        slices[family] = {arm: {"scheduled": sum(row["family"] == family and row["arm"] == arm for row in rows),
                               "success": sum(row["family"] == family and row["arm"] == arm and row["frozen_success"] for row in rows)}
                          for arm in "ABC"}
    result = {"report_sha256": digest(report_path), "source": report["source"],
              "no_new_native_requests": True, "primary_scores_unchanged": True,
              "projection_errors": dict(projections), "input_error_codes": dict(errors),
              "final_check_failures": dict(final_checks), "family_slices": slices,
              "provider_final_finish_reasons": dict(collections.Counter(reason for row in rows for reason in row["provider_final_finish_reasons"])),
              "unsupported_answer_slots": [row["sequence"] for row in rows if row["frozen_reasons"] == ["unsupported_answer"]],
              "rows": rows}
    with args.output.open("x") as target:
        json.dump(result, target, indent=2)
    print(json.dumps({key: value for key, value in result.items() if key != "rows"}))


if __name__ == "__main__":
    main()
