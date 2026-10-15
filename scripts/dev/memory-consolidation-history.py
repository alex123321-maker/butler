#!/usr/bin/env python3
"""Read-only lab budget export. Standard-library Python handles private JSON and
fixed docker process pipes without shell interpolation, credentials or Go/service
dependencies. It is not a production component or admission authority.
"""
import hashlib
import json
import os
import pathlib
import re
import subprocess
import sys

base = pathlib.Path(sys.argv[1]).resolve()
label = sys.argv[2]
if not re.fullmatch(r"/srv/storage/butler/agents/issue89-[A-Za-z0-9_-]+", str(base)) or not re.fullmatch(r"[A-Za-z0-9_-]+", label):
    raise SystemExit("isolated history proof path required")
proof = base / "reports" / label
proof.mkdir(mode=0o700)

def write(name, raw):
    with os.fdopen(os.open(proof / name, os.O_WRONLY | os.O_CREAT | os.O_EXCL, 0o600), "wb") as out:
        out.write(raw)
        out.flush()
        os.fsync(out.fileno())

def sql(container, database, query):
    p = subprocess.run(["docker", "exec", "-i", container, "psql", "-X", "-qAt", "-v", "ON_ERROR_STOP=1", "-U", "butler", "-d", database], input="BEGIN TRANSACTION READ ONLY;\n" + query + ";\nCOMMIT;\n", text=True, capture_output=True)
    if p.returncode:
        raise SystemExit("read-only historical ledger unavailable")
    return json.loads(p.stdout)

def read_artifact(path):
    p = subprocess.run(["docker", "run", "--rm", "--pull", "never", "--network", "none", "--cpus", ".1", "--memory", "64m", "--pids-limit", "32", "-v", str(path.parent) + ":/from:ro", "golang:1.27.2-bookworm", "cat", "/from/" + path.name], capture_output=True)
    if p.returncode:
        raise SystemExit("immutable historical artifact unavailable")
    return p.stdout

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
    raw = read_artifact(pathlib.Path("/srv/storage/butler/agents/issue88-20261016/reports/native-v3") / name)
    json.loads(raw)
    target = "issue88-v3-" + name
    write(target, raw)
    retained[name] = {"path": target, "sha256": hashlib.sha256(raw).hexdigest(), "data": json.loads(raw)}

raw = read_artifact(pathlib.Path("/srv/storage/butler/agents/issue84-20261016/reports/native-v7/comparison.json"))
write("issue84-v7-comparison.json", raw)
comparison84 = {"path": "issue84-v7-comparison.json", "sha256": hashlib.sha256(raw).hexdigest()}

# Exact reservation populations and current owner-day scopes retain cross-owner
# roadmap cost without pretending their authority is the prospective owner.
excluded = []
for issue, container, database in [(86, "butler-issue86-native-postgres-1", "memory_read_native_test"), (87, "butler-issue87-native-v1-pg", "memory_adaptive_test")]:
    query = ("SET LOCAL search_path=adaptive_native_v1,public; " if issue == 87 else "") + "SELECT jsonb_build_object('issue'," + str(issue) + ",'database_identity',current_database(),'server_time',clock_timestamp(),'scopes',(SELECT jsonb_agg(to_jsonb(s) ORDER BY scope_id) FROM execution_budget_scopes s WHERE kind='owner_day'),'reservations',(SELECT jsonb_agg(to_jsonb(r) ORDER BY reservation_id) FROM execution_budget_reservations r),'foreground',(SELECT jsonb_agg(to_jsonb(a) ORDER BY operation_id) FROM execution_model_attempts a))"
    data = sql(container, database, query)
    raw = json.dumps(data, sort_keys=True, separators=(",", ":")).encode()
    name = "issue" + str(issue) + "-other-owner.json"
    write(name, raw)
    excluded.append({"path": name, "sha256": hashlib.sha256(raw).hexdigest(), "data": data})

manifest = {"format": "butler.consolidation.native-history.v1", "prospective_owner": "owner", "source84": windows, "comparison84": comparison84, "source88": retained, "other_owner_roadmap": excluded, "issue82": {"owner": "issue82-owner", "reason": "distinct authenticated owner; stopped historical stand, not read/restarted by this exporter", "source": "scripts/dev/memory-jobs-native-lab.sh"}, "rule": "read-only snapshots; derive same-owner/same-UTC-day carry from original reservation identities, count inclusive88v3 once, never sum its predecessor scopes"}
raw = json.dumps(manifest, sort_keys=True, separators=(",", ":")).encode()
write("history.json", raw)
print(json.dumps({"read_only": True, "history_sha256": hashlib.sha256(raw).hexdigest(), "independent84_windows": len(windows), "inclusive88_leaf": True, "other_owner_populations": len(excluded), "auth_or_inference": False}))
