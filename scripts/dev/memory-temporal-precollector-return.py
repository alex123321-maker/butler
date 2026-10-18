#!/usr/bin/env python3
"""Narrow lab-only opaque return after a proven pre-collector process failure.
Standard-library process pipes keep encrypted credential bytes off shell/model
arguments. This grants no retry or native authority and cannot return a clone
after a collector, schema, window, budget or model attempt has begun.
"""
import base64
import hashlib
import json
import os
import pathlib
import re
import stat
import subprocess
import sys


def exclusive(path, value):
    with os.fdopen(os.open(path, os.O_WRONLY | os.O_CREAT | os.O_EXCL, 0o600), "wb") as out:
        out.write(value)
        out.flush()
        os.fsync(out.fileno())


def sql(query):
    result = subprocess.run(["docker", "exec", "-i", "butler-issue85-postgres", "psql", "-X", "-qAt", "-h", "127.0.0.1", "-p", "18585", "-U", "butler", "-d", "issue85_native_test", "-v", "ON_ERROR_STOP=1"], input="BEGIN TRANSACTION READ ONLY;" + query + ";ROLLBACK;", text=True, capture_output=True, timeout=30)
    if result.returncode:
        raise SystemExit("pre-collector durable zero proof unavailable")
    return json.loads(result.stdout)


def read_private(path):
    try:
        fd = os.open(path, os.O_RDONLY | os.O_NOFOLLOW | os.O_NONBLOCK)
    except OSError:
        raise SystemExit("private unused opaque import unavailable")
    with os.fdopen(fd, "rb") as src:
        info = os.fstat(src.fileno())
        if not stat.S_ISREG(info.st_mode) or info.st_mode & 0o077 or not 28 <= info.st_size <= 1024*1024:
            raise SystemExit("private unused opaque import unavailable")
        material = src.read(1024*1024+1)
        try:
            if len(material) > 1024*1024 or len(base64.b64decode(material, validate=True)) < 28:
                raise ValueError()
        except ValueError:
            raise SystemExit("bounded encrypted opaque material required")
        return material


base, label = pathlib.Path(sys.argv[1]).resolve(), sys.argv[2]
if not re.fullmatch(r"/srv/storage/butler/agents/issue88-[A-Za-z0-9_-]+", str(base)) or not re.fullmatch(r"[A-Za-z0-9_-]+", label):
    raise SystemExit("bounded temporal pre-collector return required")
proof, private = base / "reports" / label, base / "private" / label
if any((proof / name).exists() for name in ("scheduled-before-setup.json", "scheduled-originals.json", "setup-outcome.json", "full-ledger.json")) or list(proof.glob("entry-*.json")) or (private / "auth-return.value").exists():
    raise SystemExit("collector may have begun; opaque fallback denied")
history = proof / "history" / "history.json"
envelope = json.loads(history.read_bytes())
profiles = {'butler.temporal.native-history.v2':('memory_temporal_native_v6','temporal-explicit-v6','memory-temporal-successor-history.py'), 'butler.temporal.native-history.v3':('memory_temporal_native_v7','temporal-explicit-v7','memory-temporal-v7-history.py')}
if envelope.get('format') not in profiles:
    raise SystemExit('unsupported fixed temporal fallback recipe')
schema, window, script_name = profiles[envelope['format']]
if envelope.get('schema') != schema:
    raise SystemExit('fixed schema/history differs')
catalog = sql("SELECT jsonb_build_object('schema_present',to_regnamespace('"+schema+"') IS NOT NULL,'window_table_present',to_regclass('public.issue88_native_windows') IS NOT NULL)")
if catalog["schema_present"]:
    raise SystemExit("native schema exists; opaque fallback denied")
window_count = sql("SELECT count(*) FROM public.issue88_native_windows WHERE window_id='"+window+"'") if catalog["window_table_present"] else 0
if window_count != 0:
    raise SystemExit("native one-use window consumed; opaque fallback denied")
script = pathlib.Path(__file__).resolve().parent / script_name
current = subprocess.run([sys.executable, str(script), "check-current", str(history), hashlib.sha256(history.read_bytes()).hexdigest()], capture_output=True, timeout=180)
if current.returncode or json.loads(current.stdout).get("current_day_unchanged") is not True:
    raise SystemExit("seven-database attempt population changed; opaque fallback denied")
# The collector creates its durable schema before reading the opaque clone.
# Absence plus unchanged complete current-day inventory proves no decrypt,
# provider setup, quote, native entry or new attempt could have happened.
material = read_private(private / "auth-import.value")
if material != read_private(private / "expected-main.value"):
    raise SystemExit("unused import differs from exact original clone")
exclusive(proof / "pre-collector-return-proof.json", json.dumps({"collector_started": False, "native_entries": 0, "native_schema_present": False, "one_use_window_receipts": 0, "seven_database_current_day_unchanged": True, "opaque_import_never_used_by_collector": True, "new_native_authority": False}).encode())
exclusive(private / "auth-return.value", material)
exclusive(proof / "all-original-outcomes.json", (proof / "scheduled-before-auth.json").read_bytes())
print(json.dumps({"proven_pre_collector_zero_entries": True, "opaque_return_ready": True, "raw_material_printed": False}))
