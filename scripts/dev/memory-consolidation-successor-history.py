#!/usr/bin/env python3
"""Read-only experimental successor inventory, using only standard-library pipes.

The previous temporal manifest retains all old original evidence. Its inclusive
current-day owner scope is carried once; older predecessor scopes are witnesses,
not another charge. This exporter never reads settings or authentication.
"""
import datetime
import hashlib
import json
import os
import pathlib
import re
import subprocess
import sys


def canonical(value):
    return json.dumps(value, sort_keys=True, separators=(",", ":")).encode()


def write(path, data):
    with os.fdopen(os.open(path, os.O_CREAT | os.O_EXCL | os.O_WRONLY, 0o600), "wb") as out:
        out.write(data)
        out.flush()
        os.fsync(out.fileno())


def artifact(path):
    result = subprocess.run(["docker", "run", "--rm", "--pull", "never", "--network", "none", "--cpus", ".1", "--memory", "64m", "--pids-limit", "32", "-v", str(path.parent) + ":/from:ro", "golang:1.27.2-bookworm", "cat", "/from/" + path.name], capture_output=True, timeout=30)
    if result.returncode or len(result.stdout) > 32 * 1024 * 1024:
        raise SystemExit("bounded immutable successor artifact unavailable")
    json.loads(result.stdout)
    return result.stdout


def current(day):
    command = pathlib.Path(__file__).resolve().parent / "memory-consolidation-history.py"
    result = subprocess.run([sys.executable, str(command), "snapshot-current", day], capture_output=True, timeout=180)
    if result.returncode:
        raise SystemExit("complete six-database owner-day inventory unavailable")
    value = json.loads(result.stdout)
    if value.get("format") != "butler.consolidation.current-owner-day.v2" or value.get("day") != day:
        raise SystemExit("current owner-day contract differs")
    return value


def metadata():
    query = """BEGIN TRANSACTION ISOLATION LEVEL REPEATABLE READ READ ONLY;
SET LOCAL search_path=memory_temporal_native_v5,public;
SELECT jsonb_build_object(
 'window',(SELECT to_jsonb(w) FROM temporal_native_window w),
 'entries',(SELECT coalesce(jsonb_agg(jsonb_build_object(
   'id',id,'run_id',run_id,'kind',kind,'finished_at',finished_at,
   'observation_json',jsonb_build_object('id',observation_json->'id',
     'run_id',observation_json->'run_id','kind',observation_json->'kind',
     'operation_id',observation_json->'operation_id',
     'reservation_id',observation_json->'reservation_id',
     'usage',observation_json->'usage')) ORDER BY id),'[]'::jsonb)
   FROM temporal_native_entries));
COMMIT;"""
    result = subprocess.run(["docker", "exec", "-i", "butler-issue85-postgres", "psql", "-X", "-qAt", "-v", "ON_ERROR_STOP=1", "-U", "butler", "-p", "18585", "-d", "issue85_native_test"], input=query, text=True, capture_output=True, timeout=30)
    if result.returncode:
        raise SystemExit("closed temporal successor metadata unavailable")
    value = json.loads(result.stdout)
    window = value.get("window", {})
    if window.get("closed") is not True or window.get("sealed") is not True or not isinstance(window.get("paid_entries"), int) or not 1 <= window["paid_entries"] <= 144 or len(value.get("entries", [])) != window["paid_entries"]:
        raise SystemExit("complete closed temporal entry population required")
    return value


def ref(name, data):
    return {"Path": name, "SHA256": hashlib.sha256(data).hexdigest()}


def referenced(root, reference):
    name = reference.get("Path")
    if not isinstance(name, str) or pathlib.Path(name).name != name:
        raise SystemExit("bounded successor artifact basename required")
    data = artifact(root / name)
    if hashlib.sha256(data).hexdigest() != reference.get("SHA256"):
        raise SystemExit("immutable predecessor artifact changed")
    return name, data


mode = sys.argv[1]
if mode == "check-current":
    path = pathlib.Path(sys.argv[2]).resolve()
    if not re.fullmatch(r"/srv/storage/butler/agents/issue89-[A-Za-z0-9_-]+/reports/[A-Za-z0-9_-]+/history/history.json", str(path)):
        raise SystemExit("isolated successor history path required")
    raw = path.read_bytes()
    if hashlib.sha256(raw).hexdigest() != sys.argv[3]:
        raise SystemExit("frozen successor history changed")
    manifest = json.loads(raw)
    day = manifest["day"]
    if datetime.datetime.now(datetime.timezone.utc).strftime("%Y-%m-%d") != day:
        raise SystemExit("prospective UTC day expired")
    _, before = referenced(path.parent, manifest["current"])
    if canonical(current(day)) != before:
        raise SystemExit("current owner-day population changed; new review required")
    _, prior_raw = referenced(path.parent, manifest["predecessor88"])
    prior = json.loads(prior_raw)
    live = metadata()
    for key in ("window", "entries"):
        if prior[key] != live[key]:
            raise SystemExit("closed temporal predecessor changed")
    print(json.dumps({"read_only": True, "current_day_unchanged": True, "day": day, "auth_or_inference": False}))
    raise SystemExit(0)

if mode != "prepare":
    raise SystemExit("prepare or check-current required")
base, label = pathlib.Path(sys.argv[2]).resolve(), sys.argv[3]
predecessor = pathlib.Path(sys.argv[4]).resolve()
if not re.fullmatch(r"/srv/storage/butler/agents/issue89-[A-Za-z0-9_-]+", str(base)) or not re.fullmatch(r"[A-Za-z0-9_-]+", label) or not re.fullmatch(r"/srv/storage/butler/agents/issue88-[A-Za-z0-9_-]+/reports/[A-Za-z0-9_-]+", str(predecessor)):
    raise SystemExit("bounded isolated successor references required")
proof = base / "reports" / label / "history"
proof.mkdir(mode=0o700, parents=True)
now = datetime.datetime.now(datetime.timezone.utc)
day = now.strftime("%Y-%m-%d")
deadline = datetime.datetime.combine(now.date() + datetime.timedelta(days=1), datetime.time(), datetime.timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ")
old_raw = artifact(predecessor / "history" / "history.json")
old = json.loads(old_raw)
if old.get("format") != "butler.temporal.native-history.v1" or old.get("day") != day or old.get("deadline") != deadline:
    raise SystemExit("same-day validated temporal predecessor required")
copied = {}
for key in ("historical", "current", "predecessor89", "original89_ledger", "original89_screen"):
    name, data = referenced(predecessor / "history", old[key])
    if name not in copied:
        write(proof / name, data)
        copied[name] = data
historical = json.loads(copied[old["historical"]["Path"]])
historical_refs = list(historical["source84"]) + [historical["comparison84"], historical["current_owner_day"]] + list(historical["source88"].values()) + list(historical["other_owner_roadmap"])
for reference in historical_refs:
    name = reference["path"]
    if pathlib.Path(name).name != name:
        raise SystemExit("bounded historical basename required")
    data = artifact(predecessor / "history" / name)
    if hashlib.sha256(data).hexdigest() != reference["sha256"] or (name in copied and copied[name] != data):
        raise SystemExit("historical evidence changed or collided")
    if name not in copied:
        write(proof / name, data)
        copied[name] = data
write(proof / "temporal-v5-history.json", old_raw)
inventory = current(day)
matching = [window for database in inventory["databases"] if database["container"] == "butler-issue85-postgres" and database["database"] == "issue85_native_test" for window in database["windows"] if window["schema"] == "memory_temporal_native_v5"]
if len(matching) != 1:
    raise SystemExit("one exact latest current-day leaf required")
prior = metadata()
prior.update({"format": "butler.consolidation.predecessor88.v1", **{key: matching[0][key] for key in ("scopes", "reservations", "foreground", "background")}})
current_raw, prior_raw = canonical(inventory), canonical(prior)
write(proof / "current-v2-owner-day.json", current_raw)
write(proof / "predecessor88-current.json", prior_raw)
ledger, outcomes = artifact(predecessor / "full-ledger.json"), artifact(predecessor / "all-original-outcomes.json")
write(proof / "native88-full-ledger.json", ledger)
write(proof / "native88-outcomes.json", outcomes)
entries = []
for entry in prior["entries"]:
    number = entry["id"]
    if not isinstance(number, int) or not 1 <= number <= 144:
        raise SystemExit("bounded temporal native entry identity required")
    name = "entry-%03d.json" % number
    data = artifact(predecessor / name)
    write(proof / name, data)
    entries.append(ref(name, data))
manifest = {"format": "butler.consolidation.native-history.v3", "owner": "owner", "day": day, "deadline": deadline, "schema": "consolidation_native_v2", "predecessor88_history": ref("temporal-v5-history.json", old_raw), "current": ref("current-v2-owner-day.json", current_raw), "predecessor88": ref("predecessor88-current.json", prior_raw), "original88_ledger": ref("native88-full-ledger.json", ledger), "original88_outcomes": ref("native88-outcomes.json", outcomes), "original88_entries": entries, "rule": "Only the latest inclusive owner-day scope is imported once; retain all earlier original attempts, NULLs and deadlines separately, never sum predecessor scopes again."}
raw = canonical(manifest)
write(proof / "history.json", raw)
print(json.dumps({"read_only": True, "history_sha256": hashlib.sha256(raw).hexdigest(), "prospective_day": day, "six_database_inventory": True, "auth_or_inference": False}))
