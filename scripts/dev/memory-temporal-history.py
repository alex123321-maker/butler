#!/usr/bin/env python3
"""Lab-only successor history export. Standard-library Python keeps fixed Docker
process pipes, private immutable JSON and checksums out of shell interpolation.
No credential settings, model request or production mutation is accessed.
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


def write(path, raw):
    with os.fdopen(os.open(path, os.O_WRONLY | os.O_CREAT | os.O_EXCL, 0o600), "wb") as out:
        out.write(raw)
        out.flush()
        os.fsync(out.fileno())


def read_artifact(path):
    result = subprocess.run(["docker", "run", "--rm", "--pull", "never", "--network", "none", "--cpus", ".1", "--memory", "64m", "--pids-limit", "32", "-v", str(path.parent) + ":/from:ro", "golang:1.27.2-bookworm", "cat", "/from/" + path.name], capture_output=True, timeout=30)
    if result.returncode or len(result.stdout) > 32*1024*1024:
        raise SystemExit("bounded immutable lab artifact unavailable")
    json.loads(result.stdout)
    return result.stdout


def current_inventory(day):
    script = pathlib.Path(__file__).resolve().parent / "memory-consolidation-history.py"
    result = subprocess.run([sys.executable, str(script), "snapshot-current", day], capture_output=True, timeout=180)
    if result.returncode:
        raise SystemExit("complete read-only six-database current-day inventory unavailable")
    inventory = json.loads(result.stdout)
    if inventory.get("format") != "butler.consolidation.current-owner-day.v2" or inventory.get("day") != day:
        raise SystemExit("current-day inventory contract differs")
    return inventory


def predecessor_metadata():
    query = """BEGIN TRANSACTION ISOLATION LEVEL REPEATABLE READ READ ONLY;
SET LOCAL search_path=consolidation_native_v1,public;
SELECT jsonb_build_object(
 'closed',(SELECT closed FROM consolidation_screen_window WHERE singleton),
 'paid_entries',(SELECT entries FROM consolidation_screen_window WHERE singleton),
 'calls',(SELECT coalesce(jsonb_agg(jsonb_build_object(
   'entry_id',entry_id,'operation_id',operation_id,
   'reservation_id',observation->>'ReservationID',
   'observation_json',jsonb_build_object('EntryID',observation->'EntryID',
     'OperationID',observation->'OperationID','ReservationID',observation->'ReservationID',
     'Usage',observation->'Usage','FinishedAt',observation->'FinishedAt'))
   ORDER BY entry_id),'[]'::jsonb) FROM consolidation_screen_calls));
COMMIT;"""
    result = subprocess.run(["docker", "exec", "-i", "issue86-20261016-pg", "psql", "-X", "-qAt", "-v", "ON_ERROR_STOP=1", "-U", "butler", "-d", "memory_read_test"], input=query, capture_output=True, text=True, timeout=30)
    if result.returncode:
        raise SystemExit("closed current-day native predecessor unavailable")
    data = json.loads(result.stdout)
    if data.get("closed") is not True or not isinstance(data.get("paid_entries"), int) or not 1 <= data["paid_entries"] <= 99 or len(data.get("calls", [])) != data["paid_entries"]:
        raise SystemExit("complete closed predecessor entry population required")
    return data


def bounded_ref(path, root):
    if not isinstance(path, str) or pathlib.Path(path).name != path:
        raise SystemExit("bounded historical artifact basename required")
    return root / path


mode = sys.argv[1]
if mode == "check-current":
    path = pathlib.Path(sys.argv[2]).resolve()
    expected = sys.argv[3]
    if not re.fullmatch(r"/srv/storage/butler/agents/issue88-[A-Za-z0-9_-]+/reports/[A-Za-z0-9_-]+/history/history.json", str(path)):
        raise SystemExit("isolated temporal history reference required")
    raw = path.read_bytes()
    if hashlib.sha256(raw).hexdigest() != expected:
        raise SystemExit("frozen successor history hash differs")
    manifest = json.loads(raw)
    day = manifest["day"]
    if datetime.datetime.now(datetime.timezone.utc).strftime("%Y-%m-%d") != day:
        raise SystemExit("prospective UTC day expired")
    ref = manifest["current"]
    before = bounded_ref(ref["Path"], path.parent).read_bytes()
    if hashlib.sha256(before).hexdigest() != ref["SHA256"] or canonical(current_inventory(day)) != before:
        raise SystemExit("current-day population changed; a new reviewed freeze is required")
    ref = manifest["predecessor89"]
    prior = json.loads(bounded_ref(ref["Path"], path.parent).read_bytes())
    meta = predecessor_metadata()
    for key in ("closed", "paid_entries", "calls"):
        if prior[key] != meta[key]:
            raise SystemExit("closed native predecessor changed after history seal")
    print(json.dumps({"read_only": True, "current_day_unchanged": True, "day": day, "auth_or_inference": False}))
    raise SystemExit(0)

if mode != "prepare":
    raise SystemExit("prepare or check-current required")
base = pathlib.Path(sys.argv[2]).resolve()
label = sys.argv[3]
historical = pathlib.Path(sys.argv[4]).resolve()
native89 = pathlib.Path(sys.argv[5]).resolve()
if not re.fullmatch(r"/srv/storage/butler/agents/issue88-[A-Za-z0-9_-]+", str(base)) or not re.fullmatch(r"[A-Za-z0-9_-]+", label) or not re.fullmatch(r"/srv/storage/butler/agents/issue89-[A-Za-z0-9_-]+/reports/[A-Za-z0-9_-]+", str(historical)) or not re.fullmatch(r"/srv/storage/butler/agents/issue89-[A-Za-z0-9_-]+/reports/[A-Za-z0-9_-]+/screen", str(native89)):
    raise SystemExit("bounded isolated successor paths required")
proof = base / "reports" / label / "history"
proof.mkdir(mode=0o700, parents=True)
now = datetime.datetime.now(datetime.timezone.utc)
day = now.strftime("%Y-%m-%d")
deadline = datetime.datetime.combine(now.date()+datetime.timedelta(days=1), datetime.time(), datetime.timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ")
old_raw = read_artifact(historical / "history.json")
old = json.loads(old_raw)
if old.get("format") != "butler.consolidation.native-history.v2" or old.get("inclusive_temporal_leaf") != "native-v4":
    raise SystemExit("complete immutable temporal-v4 historical lineage required")
refs = list(old["source84"]) + [old["comparison84"], old["current_owner_day"]] + list(old["source88"].values()) + list(old["other_owner_roadmap"])
copied = {}
for ref in refs:
    path = bounded_ref(ref["path"], historical)
    data = read_artifact(path)
    if hashlib.sha256(data).hexdigest() != ref["sha256"]:
        raise SystemExit("historical original bytes changed")
    if path.name in copied:
        if copied[path.name] != data:
            raise SystemExit("historical artifact basename collision")
        continue
    write(proof / path.name, data)
    copied[path.name] = data
write(proof / "historical-v2.json", old_raw)
inventory = current_inventory(day)
matching = [window for database in inventory["databases"] if database["container"] == "issue86-20261016-pg" and database["database"] == "memory_read_test" for window in database["windows"] if window["schema"] == "consolidation_native_v1"]
if len(matching) != 1:
    raise SystemExit("one exact current native89 predecessor required")
window = matching[0]
prior = predecessor_metadata()
prior.update({"format": "butler.temporal.predecessor89.v1", "scopes": window["scopes"], "reservations": window["reservations"], "foreground": window["foreground"], "background": window["background"], "model_jobs": window["background"]})
current_raw, prior_raw = canonical(inventory), canonical(prior)
write(proof / "current-v5-owner-day.json", current_raw)
write(proof / "predecessor89-current.json", prior_raw)
ledger, screen = read_artifact(native89 / "full-ledger.json"), read_artifact(native89 / "screen.json")
write(proof / "native89-full-ledger.json", ledger)
write(proof / "native89-screen.json", screen)
ref = lambda name, raw: {"Path": name, "SHA256": hashlib.sha256(raw).hexdigest()}
manifest = {"format": "butler.temporal.native-history.v1", "owner": "owner", "day": day, "deadline": deadline, "historical": ref("historical-v2.json", old_raw), "current": ref("current-v5-owner-day.json", current_raw), "predecessor89": ref("predecessor89-current.json", prior_raw), "original89_ledger": ref("native89-full-ledger.json", ledger), "original89_screen": ref("native89-screen.json", screen), "rule": "Old immutable UTC-day history and all NULLs remain evidence; current89 exact original quote population is carried once into a new finite successor window. This is an experimental partition, not cross-database production admission."}
raw = canonical(manifest)
write(proof / "history.json", raw)
print(json.dumps({"read_only": True, "history_sha256": hashlib.sha256(raw).hexdigest(), "prospective_day": day, "six_database_inventory": True, "auth_or_inference": False}))
