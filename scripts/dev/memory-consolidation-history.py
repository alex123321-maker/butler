#!/usr/bin/env python3
"""Read-only lab budget export. Standard-library Python handles private JSON and
fixed docker process pipes without shell interpolation, credentials or Go/service
dependencies. It is not a production component or admission authority.
"""
import hashlib
import datetime
import json
import os
import pathlib
import re
import subprocess
import sys

def write(name, raw):
    with os.fdopen(os.open(proof / name, os.O_WRONLY | os.O_CREAT | os.O_EXCL, 0o600), "wb") as out:
        out.write(raw)
        out.flush()
        os.fsync(out.fileno())

def sql(container, database, query, port=5432):
    p = subprocess.run(["docker", "exec", "-i", container, "psql", "-X", "-qAt", "-v", "ON_ERROR_STOP=1", "-U", "butler", "-p", str(port), "-d", database], input="BEGIN TRANSACTION ISOLATION LEVEL REPEATABLE READ READ ONLY;\n" + query + ";\nCOMMIT;\n", text=True, capture_output=True, timeout=30)
    if p.returncode:
        raise SystemExit("read-only historical ledger unavailable")
    return json.loads(p.stdout)

def read_artifact(path):
    p = subprocess.run(["docker", "run", "--rm", "--pull", "never", "--network", "none", "--cpus", ".1", "--memory", "64m", "--pids-limit", "32", "-v", str(path.parent) + ":/from:ro", "golang:1.27.2-bookworm", "cat", "/from/" + path.name], capture_output=True)
    if p.returncode:
        raise SystemExit("immutable historical artifact unavailable")
    return p.stdout


# Fixed retained experimental stores, including the future native target. This
# is an audit inventory, never a provider-auth read. A new UTC day is permitted
# only after an independently visible zero current-day owner population. Any
# existing charged population requires another reviewed carry plan; it is not
# refunded, reset, silently omitted or treated as another owner.
CURRENT_DATABASES = (
    ("issue84-20261016-pg", "issue84_test", 5432),
    ("butler-issue85-postgres", "issue85_native_test", 18585),
    ("butler-issue86-native-postgres-1", "memory_read_native_test", 5432),
    ("butler-issue87-native-v1-pg", "memory_adaptive_test", 5432),
    ("issue86-20261016-pg", "memory_read_test", 5432),
)


def current_inventory(day):
    inventory = {"format": "butler.consolidation.current-owner-day.v1", "day": day, "owner": "owner", "databases": []}
    for container, database, port in CURRENT_DATABASES:
        discovered = sql(container, database, "SELECT jsonb_build_object('day',to_char(clock_timestamp() AT TIME ZONE 'UTC','YYYY-MM-DD'),'schemas',(SELECT jsonb_agg(n.nspname ORDER BY n.nspname) FROM pg_class c JOIN pg_namespace n ON n.oid=c.relnamespace WHERE c.relname='execution_budget_scopes' AND c.relkind='r'))", port)
        schemas = discovered.get("schemas") or []
        if discovered.get("day") != day or not isinstance(schemas, list) or (not schemas and (container, database) != ("issue86-20261016-pg", "memory_read_test")):
            raise SystemExit("complete current UTC-day ledger inventory required")
        windows = []
        for schema in schemas:
            if not re.fullmatch(r"[A-Za-z_][A-Za-z0-9_]*", schema):
                raise SystemExit("bounded current-day schema reference required")
            query = "SET LOCAL search_path=" + schema + ",public; SELECT jsonb_build_object('schema','" + schema + "','day',to_char(clock_timestamp() AT TIME ZONE 'UTC','YYYY-MM-DD'),'scopes',(SELECT coalesce(jsonb_agg(to_jsonb(s) ORDER BY scope_id),'[]') FROM execution_budget_scopes s WHERE kind='owner_day' AND owner_id='owner' AND subject_id='" + day + "'),'reservations',(SELECT coalesce(jsonb_agg(to_jsonb(r) ORDER BY reservation_id),'[]') FROM execution_budget_reservations r WHERE owner_id='owner' AND created_at >= '" + day + "T00:00:00Z'::timestamptz AND created_at < '" + day + "T00:00:00Z'::timestamptz+INTERVAL '1 day'))"
            window = sql(container, database, query, port)
            if window.pop("day", None) != day:
                raise SystemExit("current UTC day changed during read-only snapshot")
            windows.append(window)
        inventory["databases"].append({"container": container, "database": database, "port": port, "schemas": schemas, "windows": windows})
    return inventory


def canonical(value):
    return json.dumps(value, sort_keys=True, separators=(",", ":")).encode()


if sys.argv[1] == "check-current":
    path = pathlib.Path(sys.argv[2]).resolve()
    expected = sys.argv[3]
    if not re.fullmatch(r"/srv/storage/butler/agents/issue89-[A-Za-z0-9_-]+/reports/[A-Za-z0-9_-]+/history.json", str(path)):
        raise SystemExit("isolated frozen history reference required")
    raw = path.read_bytes()
    if hashlib.sha256(raw).hexdigest() != expected:
        raise SystemExit("frozen current-day proof hash differs")
    manifest = json.loads(raw)
    ref = manifest["current_owner_day"]
    if ref["path"] != "current-owner-day.json":
        raise SystemExit("bounded current-day artifact reference required")
    data = (path.parent / ref["path"]).read_bytes()
    day = manifest["prospective_day"]
    if datetime.datetime.now(datetime.timezone.utc).strftime("%Y-%m-%d") != day or hashlib.sha256(data).hexdigest() != ref["sha256"] or canonical(current_inventory(day)) != data:
        raise SystemExit("current-day ledger population changed; a new reviewed freeze is required")
    print(json.dumps({"read_only": True, "current_day_unchanged": True, "day": day, "auth_or_inference": False}))
    raise SystemExit(0)


base = pathlib.Path(sys.argv[1]).resolve()
label = sys.argv[2]
temporal_label = sys.argv[3] if len(sys.argv) > 3 else "native-v4"
if not re.fullmatch(r"/srv/storage/butler/agents/issue89-[A-Za-z0-9_-]+", str(base)) or not re.fullmatch(r"[A-Za-z0-9_-]+", label):
    raise SystemExit("isolated history proof path required")
if temporal_label != "native-v4":
    raise SystemExit("new prospective freeze requires complete temporal-v4 history")
proof = base / "reports" / label
proof.mkdir(mode=0o700)
now = datetime.datetime.now(datetime.timezone.utc)
day = now.strftime("%Y-%m-%d")
deadline = datetime.datetime.combine(now.date()+datetime.timedelta(days=1), datetime.time(), datetime.timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ")
current_raw = canonical(current_inventory(day))
write("current-owner-day.json", current_raw)
current_ref = {"path": "current-owner-day.json", "sha256": hashlib.sha256(current_raw).hexdigest()}

windows = []
for number in range(1, 8):
    schema = "summary_native_v" + str(number)
    query = "SET LOCAL search_path=" + schema + ",public; SELECT jsonb_build_object('schema','" + schema + "','database_identity',current_database(),'server_time',clock_timestamp(),'scopes',(SELECT jsonb_agg(to_jsonb(s) ORDER BY scope_id) FROM execution_budget_scopes s WHERE kind='owner_day'),'reservations',(SELECT jsonb_agg(to_jsonb(r) ORDER BY reservation_id) FROM execution_budget_reservations r),'background',(SELECT jsonb_agg(to_jsonb(a) ORDER BY operation_id) FROM memory_model_attempts a),'foreground',(SELECT jsonb_agg(to_jsonb(a) ORDER BY operation_id) FROM execution_model_attempts a))"
    window = sql("issue84-20261016-pg", "issue84_test", query)
    raw = json.dumps(window, sort_keys=True, separators=(",", ":")).encode()
    write(schema + ".json", raw)
    windows.append({"path": schema + ".json", "sha256": hashlib.sha256(raw).hexdigest(), "data": window})

retained = {}
for name in ["full-ledger.json", "retained85-ledger.json"]:
    raw = read_artifact(pathlib.Path("/srv/storage/butler/agents/issue88-20261016/reports") / temporal_label / name)
    json.loads(raw)
    target = "issue88-" + temporal_label + "-" + name
    write(target, raw)
    retained[name] = {"path": target, "sha256": hashlib.sha256(raw).hexdigest(), "data": json.loads(raw)}

raw = read_artifact(pathlib.Path("/srv/storage/butler/agents/issue84-20261016/reports/native-v7/comparison.json"))
write("issue84-v7-comparison.json", raw)
comparison84 = {"path": "issue84-v7-comparison.json", "sha256": hashlib.sha256(raw).hexdigest()}

# Exact reservation populations and current owner-day scopes retain cross-owner
# roadmap cost without pretending their authority is the prospective owner.
excluded = []
for issue, container, database in [(86, "butler-issue86-native-postgres-1", "memory_read_native_test"), (87, "butler-issue87-native-v1-pg", "memory_adaptive_test")]:
    schemas = sql(container, database, "SELECT jsonb_agg(n.nspname ORDER BY n.nspname) FROM pg_class c JOIN pg_namespace n ON n.oid=c.relnamespace WHERE c.relname='execution_budget_scopes' AND c.relkind='r'")
    if not isinstance(schemas, list) or not schemas:
        raise SystemExit("complete other-owner roadmap inventory required")
    populations = []
    for schema in schemas:
        if not re.fullmatch(r"[A-Za-z_][A-Za-z0-9_]*", schema):
            raise SystemExit("bounded other-owner schema reference required")
        query = "SET LOCAL search_path=" + schema + ",public; SELECT jsonb_build_object('schema','" + schema + "','scopes',(SELECT jsonb_agg(to_jsonb(s) ORDER BY scope_id) FROM execution_budget_scopes s WHERE kind='owner_day'),'reservations',(SELECT jsonb_agg(to_jsonb(r) ORDER BY reservation_id) FROM execution_budget_reservations r),'foreground',(SELECT jsonb_agg(to_jsonb(a) ORDER BY operation_id) FROM execution_model_attempts a),'background',(SELECT jsonb_agg(to_jsonb(a) ORDER BY operation_id) FROM memory_model_attempts a))"
        populations.append(sql(container, database, query))
    data = {"format": "butler.memory-other-owner-ledgers.v1", "issue": issue, "database_identity": database, "schemas": schemas, "populations": populations, "rule": "retain every schema/window; inclusive predecessor scope vectors are never summed as additional model calls"}
    raw = json.dumps(data, sort_keys=True, separators=(",", ":")).encode()
    name = "issue" + str(issue) + "-other-owner.json"
    write(name, raw)
    excluded.append({"path": name, "sha256": hashlib.sha256(raw).hexdigest(), "data": data})

manifest = {"format": "butler.consolidation.native-history.v2", "prospective_owner": "owner", "prospective_day": day, "prospective_deadline": deadline, "current_owner_day": current_ref, "historical_day": "2026-10-15", "inclusive_temporal_leaf": temporal_label, "source84": windows, "comparison84": comparison84, "source88": retained, "other_owner_roadmap": excluded, "issue82": {"owner": "issue82-owner", "reason": "distinct authenticated owner; stopped historical stand, not read/restarted by this exporter", "source": "scripts/dev/memory-jobs-native-lab.sh"}, "rule": "read-only original history remains charged with its original expired deadlines; count latest inclusive temporal leaf once; a distinct prospective UTC day requires independently proved zero current-day spend and a newly frozen finite window; never renew historical authority"}
raw = json.dumps(manifest, sort_keys=True, separators=(",", ":")).encode()
write("history.json", raw)
print(json.dumps({"read_only": True, "history_sha256": hashlib.sha256(raw).hexdigest(), "independent84_windows": len(windows), "inclusive88_leaf": True, "other_owner_populations": len(excluded), "auth_or_inference": False}))
