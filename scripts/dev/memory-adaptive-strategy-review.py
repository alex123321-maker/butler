#!/usr/bin/env python3
"""Lab-only read-only accounting of an already closed original96 window.

Inputs are retained metadata exports; no database, auth, network or subprocess
access. These operation counts never constitute a semantic/adoption verdict.
"""
import collections
import hashlib
import json
import os
import pathlib
import sys


def require(ok, reason):
    if not ok:
        raise SystemExit(reason)


def main():
    proof = pathlib.Path(sys.argv[1]).resolve()
    require(str(proof).startswith("/srv/storage/butler/agents/issue87-native-") and proof.name == "native-v5", "retained private proof root required")
    hashes = {}
    def read(name):
        raw = (proof / name).read_bytes()
        hashes[name] = hashlib.sha256(raw).hexdigest()
        return json.loads(raw)
    ledger = read("full-ledger.json")
    live = read("live-seven-store-metadata.json")
    metadata = read("strategy-source-metadata-v2.json")
    pins = read("frozen-pins.json")
    packet_raw = (proof / "review-cases.jsonl").read_bytes()
    hashes["review-cases.jsonl"] = hashlib.sha256(packet_raw).hexdigest()
    cases = [json.loads(row) for row in packet_raw.splitlines()]
    require(len(cases) == 96 and len({row["run_id"] for row in cases}) == 96, "full original96 population required")
    keys = {"foreground": "operation_id", "native_entries": "entry_id", "reservations": "reservation_id", "scopes": "scope_id"}
    for table, key in keys.items():
        require(sorted(live[table], key=lambda value: value[key]) == sorted(ledger[table], key=lambda value: value[key]), "live store changed since immutable export: "+table)
    require(live["window"] == ledger["window"] and ledger["window"]["closed"] is True, "closed window differs")
    runs = {row["run_id"]: row for row in metadata["runs"]}
    require(set(runs) == {row["run_id"] for row in cases}, "live/original run population differs")
    scopes = {row["scope_id"]: row for row in ledger["scopes"]}
    fg = {row["operation_id"] for row in ledger["foreground"]}
    live_local, index_local = collections.defaultdict(list), collections.defaultdict(list)
    seen_local = set()
    for receipt in ledger["reservations"]:
        if not receipt["request_json"]["charge"]["model_turns"] or receipt["operation_id"] in fg:
            continue
        ids = receipt["request_json"]["scope_ids"]
        local = [scopes[key]["subject_id"] for key in ids if scopes[key]["kind"] == "auxiliary"]
        if receipt["operation_id"].startswith("local:"):
            matches = [key[5:] for key in local if key.startswith("live:")]
            require(len(matches) == 1 and matches[0] in runs, "local query must bind one exact live-run scope")
            live_local[matches[0]].append(receipt)
        else:
            matches = [key for key in local if key.startswith("adaptive-index:")]
            require(len(matches) == 1 and receipt["operation_id"].startswith("aux:"+matches[0]+":"), "index attempt must bind exact owned job scope")
            index_local[matches[0]].append(receipt)
        require(receipt["reservation_id"] not in seen_local, "duplicate local receipt")
        seen_local.add(receipt["reservation_id"])
    summaries = {}
    records = []
    assigned_index, assigned_query = set(), set()
    for row in cases:
        run, arm, logical = row["run_id"], row["arm"], row["logical_case_id"]
        require(runs[run]["session_key"] == "adaptive:"+logical+"-"+arm, "exact original/session binding differs")
        plans = [value for value in ledger["plans"] if value["run_id"] == run]
        proposals = [value for value in metadata["proposals"] or [] if value["run_id"] == run]
        reads = [value for value in metadata["confirmed_reads"] or [] if value["run_id"] == run]
        require(all(value["operation_state"] == "observed" and value["effect_state"] == "effect_confirmed" and value["confirmed_runtime_response"] is True for value in reads), "read lacks exact confirmed Broker/runtime receipt")
        queries = live_local[run]
        # Frozen collector's trusted enqueue ID includes logical case, arm and
        # project. The scope and reservation corroboration above is independent.
        prefix = "adaptive-index:"+logical+"-"+arm+":"
        indices = [receipt for key, values in index_local.items() if key.startswith(prefix) for receipt in values]
        for receipt in indices:
            require(receipt["reservation_id"] not in assigned_index, "index receipt assigned twice")
            assigned_index.add(receipt["reservation_id"])
        for receipt in queries:
            require(receipt["reservation_id"] not in assigned_query, "query receipt assigned twice")
            assigned_query.add(receipt["reservation_id"])
        strategies = dict(collections.Counter(value["plan_json"]["strategy"] for value in plans))
        current = {"logical_case_id": logical, "opaque_case_id": row["opaque_case_id"], "arm": arm, "run_id": run, "status": row["actual_status"], "adaptive_plans": len(plans), "selected_strategies": strategies, "plan_reasons": [value["plan_json"]["reasons"] for value in plans], "plan_calls": sum(value["calls"] for value in plans), "plan_searches": sum(value["searches"] for value in plans), "broker_proposals": proposals, "confirmed_reads": reads, "local_query_reservations": queries, "local_index_reservations": indices, "native_entries": len(row["native_usage_and_owning_receipts"]), "adoption_verdict": None}
        records.append(current)
        aggregate = summaries.setdefault(arm, {"originals": 0, "adaptive_plans": 0, "selected_strategies": collections.Counter(), "broker_proposals": 0, "confirmed_reads": 0, "local_queries": 0, "local_index_attempts": 0, "native_entries": 0})
        for field, count in (("originals", 1), ("adaptive_plans", len(plans)), ("broker_proposals", len(proposals)), ("confirmed_reads", len(reads)), ("local_queries", len(queries)), ("local_index_attempts", len(indices)), ("native_entries", current["native_entries"])):
            aggregate[field] += count
        aggregate["selected_strategies"].update(strategies)
    require(assigned_index | assigned_query == seen_local and not assigned_index & assigned_query, "local population not completely assigned")
    require(sum(value["broker_proposals"] for value in summaries.values()) == len(metadata["proposals"] or []) and sum(value["confirmed_reads"] for value in summaries.values()) == len(metadata["confirmed_reads"] or []), "Broker population not completely assigned")
    result = {"format": "butler.adaptive.original96-strategy-accounting.v1", "source_release": pins["release"], "frozen_utc_day": pins["utc_day"], "current_utc_day": live["utc_day"], "observed_at_utc": live["observed_at_utc"], "owner": pins["owner"], "store_live_matches_closed_immutable_export": True, "source_artifact_sha256": hashes, "per_arm": summaries, "originals": records, "qualified": False, "rule": "Counts show selected policy and actual operations only. Frozen Rerank=false means live-local reservations are query embeddings; exact query/run scope and index/job scope IDs are retained. No semantic, adoption, cost or production verdict is granted."}
    raw = json.dumps(result, ensure_ascii=False, separators=(",", ":")).encode()+b"\n"
    with os.fdopen(os.open(proof / "strategy-accounting.json", os.O_CREAT | os.O_EXCL | os.O_WRONLY, 0o600), "wb") as out:
        out.write(raw)
        out.flush()
        os.fsync(out.fileno())
    print(json.dumps({"strategy_accounting_sha256": hashlib.sha256(raw).hexdigest(), "per_arm": summaries, "live_store_matches": True}))


if __name__ == "__main__":
    main()
