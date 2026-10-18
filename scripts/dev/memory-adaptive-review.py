#!/usr/bin/env python3
"""Postcollection lab-only packet and accounting, using original private files.
Standard-library JSON/hash/exclusive file operations avoid any runtime, provider,
auth, subprocess or network call. No semantic quality verdict is generated.
"""
import collections
import hashlib
import json
import os
import pathlib
import re
import sys


def sha(raw):
    return hashlib.sha256(raw).hexdigest()


def encoded(value):
    return json.dumps(value, ensure_ascii=False, separators=(",", ":")).encode()


def go_encoded(value):
    # Original Go struct/JSON object order is retained by the JSON decoder.
    return json.dumps(value, ensure_ascii=False, separators=(",", ":")).replace("&", "\\u0026").replace("<", "\\u003c").replace(">", "\\u003e").replace("\u2028", "\\u2028").replace("\u2029", "\\u2029").encode()


def visible_aggregate(trace):
    pieces = []
    def walk(value):
        if isinstance(value, str):
            pieces.append(value+"\n")
        elif isinstance(value, list):
            for child in value:
                walk(child)
        elif isinstance(value, dict):
            for key in sorted(value):
                walk(value[key])
    def append(raw):
        pieces.append(raw+"\n")
        try:
            walk(json.loads(raw))
        except (ValueError, TypeError):
            pass
    for call in trace:
        if "start" in call:
            for item in call["start"]["InputItems"]:
                append(item["Content"])
        if "continuation" in call:
            append(call["continuation"]["ToolResultJSON"])
    return "".join(pieces)


def require(condition, reason):
    if not condition:
        raise SystemExit(reason)


def exclusive(path, raw):
    with os.fdopen(os.open(path, os.O_WRONLY | os.O_CREAT | os.O_EXCL, 0o600), "wb") as out:
        out.write(raw)
        out.flush()
        os.fsync(out.fileno())


def main():
    base = pathlib.Path(sys.argv[1]).resolve()
    require(re.fullmatch(r"/srv/storage/butler/agents/issue87-native-[A-Za-z0-9_-]+", str(base)), "isolated retained adaptive root required")
    proof = base / "reports" / "native-v5"
    source = pathlib.Path((proof / "source-reference").read_text().strip())
    require(str(source).startswith(str(base / "snapshots")+"/"), "frozen source reference required")
    hashes = {}
    def read(name):
        raw = (proof / name).read_bytes()
        hashes[name] = sha(raw)
        return json.loads(raw)
    pins, schedule = read("frozen-pins.json"), read("scheduled-before-auth.json")
    ledger, calls = read("full-ledger.json"), read("native-calls.json")
    inputs, measurements = read("inputs.json")["inputs"], read("measurements.json")["observations"]
    outcomes, citations, bindings = read("all-original-outcomes.json"), read("citation-observations.json"), read("source-bindings.json")
    capture_digests = {value["run_id"]: value for value in read("native-capture-digests.json")}
    for name in ("history.json", "six-store-ledgers.json", "shared-policy.json", "return-safe.json", "main-auth-safe.json", "main-snapshot-safe.json"):
        read(name)
    corpus_raw = (source / "deploy/memory-adaptive/holdout-v1/corpus.json").read_bytes()
    controls_raw = (source / "deploy/memory-adaptive/holdout-v1/controls.json").read_bytes()
    require(sha(corpus_raw) == pins["corpus_sha256"] and sha(controls_raw) == pins["controls_sha256"], "frozen rubric/import controls differ")
    corpus, controls = json.loads(corpus_raw), json.loads(controls_raw)
    require(ledger["window"]["closed"] is True and ledger["window"]["paid_entries"] == len(calls), "complete closed native window required")
    key = lambda value: (value["case_id"], value["arm"])
    expected = {key(value): value for value in schedule["inputs"]}
    provided, observed = {key(value): value for value in inputs}, {key(value): value for value in measurements}
    require(len(expected) == len(inputs) == len(measurements) == len(provided) == len(observed) == 96 and set(expected) == set(provided) == set(observed), "full96 exact identity population required")
    require(outcomes["inputs"] == inputs and outcomes["observations"] == measurements, "final outcomes differ from actual collector receipts")
    operations = {value["operation_id"]: value for value in ledger["foreground"]}
    reservations = {value["reservation_id"]: value for value in ledger["reservations"]}
    native = {value["entry_id"]: value for value in ledger["native_entries"]}
    require(len(native) == len(calls) and len(operations) == len(calls), "native/product populations differ")
    by_session, by_run, metadata = collections.defaultdict(list), collections.defaultdict(list), []
    seen_ops, seen_rsv = set(), set()
    for call in calls:
        op, rsv = call["operation_id"], call["reservation_id"]
        require(op not in seen_ops and rsv not in seen_rsv, "duplicate owning native operation/reservation")
        seen_ops.add(op)
        seen_rsv.add(rsv)
        product, receipt, durable = operations[op], reservations[rsv], native[call["id"]]
        require(product["run_id"] == durable["run_id"] == call["run_id"] and product["reservation_id"] == durable["reservation_id"] == rsv and durable["operation_id"] == receipt["operation_id"] == op and receipt["owner_id"] == pins["owner"], "owning native receipt correlation failed")
        require(durable["native_usage"] == call["usage"] and durable["full_total_known"] == call["full_total_known"] and durable["usage_invalid"] == call["invalid_usage"], "native original usage differs from SQL")
        usage = call["usage"]
        if usage is None:
            require(product["actual_input_tokens"] is None and product["actual_output_tokens"] is None, "unknown native usage became known product usage")
        else:
            require(product["actual_input_tokens"] == usage["InputTokens"] and product["actual_output_tokens"] == usage["OutputTokens"], "product/native observed counts differ")
            require(all(isinstance(usage[field], int) and usage[field] >= 0 for field in ("InputTokens", "OutputTokens", "TotalTokens")) and usage["TotalTokens"] >= usage["InputTokens"]+usage["OutputTokens"], "native usage invalid")
        item = {field: call[field] for field in ("id", "operation_id", "reservation_id", "run_id", "kind", "usage", "full_total_known", "invalid_usage", "failure", "millis", "model_version")}
        metadata.append(item)
        by_run[call["run_id"]].append(call)
        if "start" in call:
            require(call["start"]["Context"]["RunID"] == call["run_id"], "native start run differs")
            by_session[call["start"]["Context"]["SessionKey"]].append(call["run_id"])
    require(len(by_session) == 96 and all(len(value) == 1 for value in by_session.values()), "one native start per original required")
    owner_scopes = [value for value in ledger["scopes"] if value["kind"] == "owner_day"]
    require(len(owner_scopes) == 1 and owner_scopes[0]["owner_id"] == pins["owner"] and owner_scopes[0]["subject_id"] == pins["utc_day"], "exact current owner-day required")
    charges = collections.Counter()
    cloud, local, read_tools = 0, 0, 0
    for receipt in reservations.values():
        charge = receipt["request_json"]["charge"]
        charges.update(charge)
        if charge["model_turns"]:
            if receipt["operation_id"] in operations:
                cloud += charge["model_turns"]
            else:
                local += charge["model_turns"]
        read_tools += charge["external_calls"]
    require(dict(charges) == owner_scopes[0]["used_json"], "scope charge differs from exact unique reservations")
    known = [call for call in calls if call["full_total_known"]]
    totals = {field: sum(call["usage"][field] for call in known) for field in ("InputTokens", "OutputTokens", "TotalTokens")}
    failures = dict(collections.Counter(call["failure"] for call in calls if call["failure"]))
    status = dict(collections.Counter(value["status"] for value in measurements))
    summary = {"originals": 96, "status": status, "unexecuted": sum(value["reason"] == "not_executed_window_stopped" for value in measurements), "native_entries": len(calls), "native_full_known": len(known), "native_unknown": len(calls)-len(known), "native_invalid": sum(call["invalid_usage"] for call in calls), "known_native_usage": totals, "full_native_total_is_known": len(known) == len(calls), "typed_failures": failures, "conservative_charges": dict(charges), "cloud_model_quotes": cloud, "local_model_quotes": local, "read_tool_quotes": read_tools, "structural_citations": {field: citations[field] for field in ("eligible_original_native_envelopes", "malformed_original_envelopes", "citations_checked", "structural_citation_failures", "released_structural_citation_failures")}, "semantic_quality": "unknown_pending_independent_all96_review", "semantic_entailment": "unknown_pending_independent_review", "qualified": False}
    records = []
    capture_checks = []
    visible_root = proof / "review-visible-inputs-v2"
    visible_root.mkdir(mode=0o700, exist_ok=True)
    arms = ("no_ltm", "retrieval_static", "retrieval_adaptive")
    for position, scheduled in enumerate(schedule["inputs"]):
        case, original_arm = corpus["cases"][position//3], arms[(position%3+position//3)%3]
        identity = key(scheduled)
        actual, result = provided[identity], observed[identity]
        require(scheduled["question"] == case["question"] and scheduled["arm"] == original_arm, "frozen source/rotating order differs")
        require(result["input_sha256"] == sha(go_encoded(actual)), "actual input digest differs")
        for field in ("question", "task", "as_of_utc", "instructions"):
            require(actual[field] == scheduled[field], "original question/task/instructions changed")
        session = "adaptive:"+case["id"]+"-"+original_arm
        run = by_session[session][0]
        trace = by_run[run]
        require(capture_digests[run]["entries"] == len(trace), "actual native capture entry population differs")
        capture_check = {"run_id": run, "expected_capture_sha256": actual["capture_sha256"], "recomputed_retained_capture_sha256": capture_digests[run]["sha256"], "capture_digest_matches": actual["capture_sha256"] == capture_digests[run]["sha256"], "visible_aggregate_matches": actual["memory_prompt"] == visible_aggregate(trace), "input_digest_matches": True}
        capture_checks.append(capture_check)
        visible, opened = [], []
        for call in trace:
            entry = {"native_entry_id": call["id"], "kind": call["kind"]}
            if "start" in call:
                entry["input_items"] = call["start"]["InputItems"]
                entry["tool_definitions"] = call["start"]["ToolDefinitions"]
            if "continuation" in call:
                continuation = call["continuation"]
                entry["tool_call_ref"] = continuation["ToolCallRef"]
                entry["tool_result_json"] = continuation["ToolResultJSON"]
                envelope = json.loads(continuation["ToolResultJSON"])
                for item in envelope.get("result", {}).get("items", []):
                    if item.get("read_ref"):
                        witness = next((value for value in ledger["opened"] if value["read_ref"] == item["read_ref"]), None)
                        require(witness is not None and witness["text_hash"] == sha(item["text"].encode()) and witness["char_count"] == len(item["text"]), "actual opened text differs from durable receipt")
                        opened.append({"native_entry_id": call["id"], "actually_model_provided": item, "durable_opened_receipt": witness, "durable_read_operations": [value for value in ledger["operations"] if value["effect_id"] == witness["effect_id"]]})
            visible.append(entry)
        filename = case["id"]+"-"+original_arm+".json"
        visible_bytes = encoded({"run_id": run, "actual_input_receipt": actual, "actual_logical_visible_turns": visible})+b"\n"
        exclusive(visible_root / filename, visible_bytes)
        literal_final = result["output"] if result.get("output_encoding") == "raw_utf8_json_string" else None
        require(literal_final is None or isinstance(literal_final, str), "public original output encoding differs")
        record = {"position": position+1, "logical_case_id": case["id"], "opaque_case_id": identity[0], "arm": original_arm, "run_id": run, "question": case["question"], "as_of_utc": case["as_of_utc"], "task": case["task"], "rubric_verifier_only": {field: case[field] for field in ("coverage", "answerable", "required_facts", "forbidden_claims", "oracle_source_ids", "repeat_retrieval", "evidence_basis")}, "fixed_original_history_verifier_only": case["history"], "trusted_import_controls_verifier_only": controls["cases"][position//3], "actual_source_bindings": bindings[case["id"]+"/"+original_arm], "actual_status": result["status"], "actual_reason": result["reason"], "actual_final_utf8": literal_final, "actual_final_sha256": sha(literal_final.encode()) if literal_final is not None else None, "original_measurement": result, "native_visible_finals": [{"native_entry_id": call["id"], "visible_final_utf8": call["final"], "visible_final_sha256": sha(call["final"].encode())} for call in trace if call["final"]], "actual_visible_input": {"path": "review-visible-inputs-v2/"+filename, "sha256": sha(visible_bytes), "input_sha256": result["input_sha256"], "capture_sha256": actual["capture_sha256"]}, "capture_correlation": capture_check, "native_usage_and_owning_receipts": [{"native": metadata[call["id"]-1], "product_attempt": operations[call["operation_id"]], "reservation": reservations[call["reservation_id"]]} for call in trace], "actual_opened_witnesses": opened, "original_citation_observations": [value for value in citations["observations"] if value["run_id"] == run], "retrieval_plans": [value for value in ledger["plans"] if value["run_id"] == run], "independent_semantic_verdict": None}
        records.append(record)
    summary["capture_digest_matches"] = sum(value["capture_digest_matches"] for value in capture_checks)
    summary["capture_digest_mismatches"] = sum(not value["capture_digest_matches"] for value in capture_checks)
    summary["visible_aggregate_matches"] = sum(value["visible_aggregate_matches"] for value in capture_checks)
    summary["input_digest_matches"] = 96
    packet_bytes = b"".join(encoded(record)+b"\n" for record in records)
    exclusive(proof / "review-cases.jsonl", packet_bytes)
    witness = {"format": "butler.adaptive.latest-store-metadata.v5", "source_release": pins["release"], "day": pins["utc_day"], "owner": pins["owner"], "store": {"container": "butler-issue87-native-v5-pg", "database": "memory_adaptive_test", "schema": "adaptive_native_v5", "port": 5432, "address": "10.237.203.2", "data": str(base / "postgres")}, "original_full_ledger_sha256": hashes["full-ledger.json"], "original_native_trace_sha256": hashes["native-calls.json"], "window": ledger["window"], "scopes": ledger["scopes"], "reservations": ledger["reservations"], "foreground": ledger["foreground"], "native_entries": ledger["native_entries"], "native_original_metadata": metadata, "summary": summary, "rule": "Distinct adaptive owner population; never add these226 quotes into another owner's49 inclusive carry. NULL usage and original expired/future deadlines remain unchanged. No new reservation/admission is created by this export."}
    witness_bytes = encoded(witness)+b"\n"
    exclusive(proof / "latest-store-metadata.json", witness_bytes)
    manifest = {"format": "butler.adaptive.all96-review-ready.v5", "source_release": pins["release"], "prepared_pins_sha256": sha((proof / "prepared-pins.sha256").read_bytes()), "corpus_sha256": sha(corpus_raw), "controls_sha256": sha(controls_raw), "original_artifact_hashes": hashes, "review_cases": {"path": "review-cases.jsonl", "sha256": sha(packet_bytes), "originals": 96}, "latest_store_metadata": {"path": "latest-store-metadata.json", "sha256": sha(witness_bytes)}, "summary": summary, "exclusions": ["raw provider frames", "hidden reasoning", "auth material", "raw backend error bodies"], "readiness": "ready for independent review of all96; no semantic/cost/promotion qualification granted"}
    exclusive(proof / "review-ready-manifest.json", encoded(manifest)+b"\n")
    print(json.dumps({"manifest_sha256": sha(encoded(manifest)+b"\n"), "packet_sha256": sha(packet_bytes), "latest_store_metadata_sha256": sha(witness_bytes), "summary": summary}))


if __name__ == "__main__":
    main()
