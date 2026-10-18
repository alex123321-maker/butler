#!/usr/bin/env python3
"""Lab-only six-store resource inventory. Standard-library subprocess pipes and
exclusive private files preserve NULLs and avoid shell interpolation. This is
not a Butler core service, admission authority or provider/auth reader.
"""
import datetime
import hashlib
import json
import os
import pathlib
import re
import subprocess
import sys

OWNER = "adaptive-prospective-owner-v1"
OLD_SHA = "ef3b23faaa5668a74a0aefc217c127a957aec71c3c491cd805b0793b260a8771"
OLD_NATIVE_SHA = "613652b6fa6c0cb549aeb62808d110143e26411494174c0f2d5174d603a0cf98"
DATABASES = (("butler-lab-postgres-1", "butler", 5432),
             ("issue84-20261016-pg", "issue84_test", 5432),
             ("butler-issue85-postgres", "issue85_native_test", 18585),
             ("butler-issue86-native-postgres-1", "memory_read_native_test", 5432),
             ("butler-issue87-native-v1-pg", "memory_adaptive_test", 5432),
             ("issue86-20261016-pg", "memory_read_test", 5432))


def canonical(value):
    return json.dumps(value, sort_keys=True, separators=(",", ":")).encode()


def utc_day(timestamp):
    parsed = datetime.datetime.fromisoformat(timestamp.replace("Z", "+00:00"))
    if parsed.tzinfo is None:
        raise SystemExit("explicit timestamp offset required")
    return parsed.astimezone(datetime.timezone.utc).strftime("%Y-%m-%d")


def run(args, script=None):
    result = subprocess.run(args, input=script, capture_output=True, text=True, timeout=40)
    if result.returncode:
        raise SystemExit("bounded read-only inventory unavailable")
    return result.stdout.strip()


def sql(container, database, port, query):
    return json.loads(run(["docker", "exec", "-i", container, "psql", "-X", "-qAt", "-v", "ON_ERROR_STOP=1", "-U", "butler", "-p", str(port), "-d", database],
                          "BEGIN TRANSACTION ISOLATION LEVEL REPEATABLE READ READ ONLY;" + query + ";COMMIT;"))


def inventory(day):
    result = {"format": "butler.adaptive.six-store-ledgers.v5", "day": day, "prospective_owner": OWNER, "databases": []}
    for container, database, port in DATABASES:
        found = sql(container, database, port, "SELECT jsonb_build_object('day',to_char(clock_timestamp() AT TIME ZONE 'UTC','YYYY-MM-DD'),'schemas',(SELECT coalesce(jsonb_agg(n.nspname ORDER BY n.nspname),'[]') FROM pg_class c JOIN pg_namespace n ON n.oid=c.relnamespace WHERE c.relname='execution_budget_scopes' AND c.relkind='r'))")
        if found["day"] != day or not found["schemas"]:
            raise SystemExit("complete current UTC six-store inventory required")
        windows = []
        for schema in found["schemas"]:
            if not re.fullmatch(r"[A-Za-z_][A-Za-z0-9_]*", schema):
                raise SystemExit("bounded ledger schema required")
            # Whole history and every distinct owner remain visible. Only
            # resource metadata is read; never settings/source/error payloads.
            window = sql(container, database, port, "SET LOCAL search_path=" + schema + ",public;SELECT jsonb_build_object('schema','" + schema + "','scopes',(SELECT coalesce(jsonb_agg(to_jsonb(s) ORDER BY scope_id),'[]') FROM execution_budget_scopes s),'reservations',(SELECT coalesce(jsonb_agg(to_jsonb(r) ORDER BY reservation_id),'[]') FROM execution_budget_reservations r),'foreground_present',to_regclass('" + schema + ".execution_model_attempts') IS NOT NULL,'background_present',to_regclass('" + schema + ".memory_model_attempts') IS NOT NULL)")
            window["foreground"], window["background"] = [], []
            if window["foreground_present"]:
                window["foreground"] = sql(container, database, port, "SET LOCAL search_path=" + schema + ",public;SELECT coalesce(jsonb_agg(jsonb_build_object('operation_id',a.operation_id,'run_id',a.run_id,'reservation_id',a.reservation_id,'provider',a.provider,'model',a.model,'input_allowance',a.input_allowance,'output_allowance',a.output_allowance,'actual_input_tokens',a.actual_input_tokens,'actual_output_tokens',a.actual_output_tokens,'created_at',a.created_at,'observed_at',a.observed_at) ORDER BY operation_id),'[]') FROM execution_model_attempts a")
            if window["background_present"]:
                window["background"] = sql(container, database, port, "SET LOCAL search_path=" + schema + ",public;SELECT coalesce(jsonb_agg(jsonb_build_object('operation_id',a.operation_id,'job_id',a.job_id,'reservation_id',a.reservation_id,'provider',a.provider,'model',a.model,'quote',a.quote,'state',a.state,'usage',a.usage,'created_at',a.created_at,'observed_at',a.observed_at) ORDER BY operation_id),'[]') FROM memory_model_attempts a")
            present = sql(container, database, port, "SELECT jsonb_build_object('temporal',to_regclass('" + schema + ".temporal_native_entries') IS NOT NULL,'consolidation',to_regclass('" + schema + ".consolidation_screen_calls') IS NOT NULL)")
            window["native_observers_present"] = present
            window["native_temporal"], window["native_consolidation"] = [], []
            if present["temporal"]:
                window["native_temporal"] = sql(container, database, port, "SET LOCAL search_path=" + schema + ",public;SELECT coalesce(jsonb_agg(jsonb_build_object('entry_id',id,'run_id',run_id,'operation_id',operation_id,'reservation_id',reservation_id,'usage',observation_json->'usage','created_at',created_at,'finished_at',finished_at) ORDER BY id),'[]') FROM temporal_native_entries")
            if present["consolidation"]:
                window["native_consolidation"] = sql(container, database, port, "SET LOCAL search_path=" + schema + ",public;SELECT coalesce(jsonb_agg(jsonb_build_object('entry_id',entry_id,'run_id',run_id,'operation_id',operation_id,'reservation_id',observation->'ReservationID','usage',observation->'Usage','created_at',created_at,'finished_at',finished_at) ORDER BY entry_id),'[]') FROM consolidation_screen_calls")
            windows.append(window)
        result["databases"].append({"container": container, "database": database, "port": port, "windows": windows})
    return result


def exclusive(path, raw):
    with os.fdopen(os.open(path, os.O_WRONLY | os.O_CREAT | os.O_EXCL, 0o600), "wb") as target:
        target.write(raw)
        target.flush()
        os.fsync(target.fileno())


def retained_original(name, expected):
    root = pathlib.Path("/srv/storage/butler/agents/issue87-native-v4-20261016/reports/native-v1")
    result = subprocess.run(["docker", "run", "--rm", "--pull", "never", "--network", "none", "--cpus", ".1", "--memory", "128m", "-v", str(root)+":/from:ro", "golang:1.27.2-bookworm", "cat", "/from/"+name], capture_output=True, timeout=40)
    if result.returncode or hashlib.sha256(result.stdout).hexdigest() != expected:
        raise SystemExit("immutable historical adaptive original differs")
    json.loads(result.stdout)
    return result.stdout


def main():
    mode, base_arg = sys.argv[1:3]
    base = pathlib.Path(base_arg).resolve()
    if mode not in ("prepare", "check-current") or not re.fullmatch(r"/srv/storage/butler/agents/issue87-native-[A-Za-z0-9_-]+", str(base)):
        raise SystemExit("bounded adaptive proof reference required")
    proof = base / "reports" / "native-v5"
    now = datetime.datetime.now(datetime.timezone.utc)
    day = now.strftime("%Y-%m-%d")
    if mode == "check-current":
        raw = (proof / "history.json").read_bytes()
        if hashlib.sha256(raw).hexdigest() != sys.argv[3]:
            raise SystemExit("immutable history hash differs")
        manifest = json.loads(raw)
        saved = (proof / "six-store-ledgers.json").read_bytes()
        for name, expected in (("historical-v4-full-ledger.json", OLD_SHA), ("historical-v4-native-calls.json", OLD_NATIVE_SHA)):
            if hashlib.sha256((proof / name).read_bytes()).hexdigest() != expected:
                raise SystemExit("historical original changed after freeze")
        if manifest["day"] != day or hashlib.sha256(saved).hexdigest() != manifest["inventory_sha256"] or canonical(inventory(day)) != saved:
            raise SystemExit("ledger population or UTC day changed; new independent freeze required")
        print(json.dumps({"read_only": True, "current_day_unchanged": True, "six_databases": 6, "day": day, "auth_or_inference": False}))
        raise SystemExit(0)

    proof.mkdir(parents=True, mode=0o700)
    data = inventory(day)
    raw = canonical(data)
    reservations = [r for database in data["databases"] for window in database["windows"] for r in window["reservations"] if r["owner_id"] == OWNER and utc_day(r["created_at"]) == day]
    scopes = [s for database in data["databases"] for window in database["windows"] for s in window["scopes"] if s["owner_id"] == OWNER and s["kind"] == "owner_day" and s["subject_id"] == day]
    if reservations or scopes:
        raise SystemExit("current adaptive owner population requires a separately reviewed carry plan")
    old_raw = retained_original("full-ledger.json", OLD_SHA)
    native_raw = retained_original("native-calls.json", OLD_NATIVE_SHA)
    exclusive(proof / "six-store-ledgers.json", raw)
    exclusive(proof / "historical-v4-full-ledger.json", old_raw)
    exclusive(proof / "historical-v4-native-calls.json", native_raw)
    deadline = datetime.datetime.combine(now.date()+datetime.timedelta(days=1), datetime.time(), datetime.timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ")
    manifest = {"format": "butler.adaptive.native-history.v5", "owner": OWNER, "day": day, "deadline": deadline, "current_owner_reservations": 0, "current_owner_scopes": 0, "inventory_sha256": hashlib.sha256(raw).hexdigest(), "historical_ledger_sha256": OLD_SHA, "historical_native_sha256": OLD_NATIVE_SHA, "rule": "All six original stores, distinct owners, original deadlines and nullable usage remain visible. Native observer usage is separate from product input/output; original v4 trace remains unchanged. Inclusive prior scope vectors are never added again as fresh calls. No reset, refund, replay or historical authority renewal. Zero current adaptive owner is a lab partition control, not cross-database production admission."}
    raw = canonical(manifest)
    exclusive(proof / "history.json", raw)
    print(json.dumps({"read_only": True, "history_sha256": hashlib.sha256(raw).hexdigest(), "six_databases": 6, "day": day, "auth_or_inference": False}))


if __name__ == "__main__":
    main()
