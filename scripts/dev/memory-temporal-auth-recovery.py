#!/usr/bin/env python3
"""Narrow lab-only opaque recovery after the exact native collector has exited.

Python stdlib keeps encrypted settings in process pipes/private files. This
returns a durable transport refresh or a proven unused original; it grants no
model request, task retry, cohort reopen, refund or qualification.
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


def opaque(path):
    with os.fdopen(os.open(path, os.O_RDONLY | os.O_NOFOLLOW | os.O_NONBLOCK), "rb") as src:
        info = os.fstat(src.fileno())
        if not stat.S_ISREG(info.st_mode) or info.st_mode & 0o077 or not 28 <= info.st_size <= 1024*1024:
            raise SystemExit("private bounded encrypted setting required")
        value = src.read(1024*1024+1).strip()
    try:
        if len(base64.b64decode(value, validate=True)) < 28:
            raise ValueError()
    except ValueError:
        raise SystemExit("opaque encrypted setting required")
    return value


def sql(query):
    result = subprocess.run(["docker", "exec", "-i", "butler-issue85-postgres", "psql", "-X", "-qAt", "-h", "127.0.0.1", "-p", "18585", "-v", "ON_ERROR_STOP=1", "-U", "butler", "-d", "issue85_native_test"], input="BEGIN TRANSACTION READ ONLY;"+query+";ROLLBACK;", text=True, capture_output=True, timeout=30)
    if result.returncode:
        raise SystemExit("native opaque recovery metadata unavailable")
    raw = result.stdout.strip()
    return True if raw == 't' else False if raw == 'f' else json.loads(raw)


def collector_exited(name):
    population = subprocess.run(["docker", "ps", "-a", "--format", "{{.Names}}"], capture_output=True, text=True, timeout=15)
    if population.returncode:
        raise SystemExit("collector exit proof unavailable; auth recovery denied")
    if name in population.stdout.splitlines():
        process = subprocess.run(["docker", "inspect", name], capture_output=True, text=True, timeout=15)
        if process.returncode:
            raise SystemExit("collector is running or its exit is unproven; auth recovery denied")
        try:
            states = json.loads(process.stdout)
            if len(states) != 1 or states[0]['State']['Running'] is not False or states[0]['State']['Status'] not in ('exited', 'dead', 'created'):
                raise ValueError()
        except (ValueError, KeyError, TypeError):
            raise SystemExit("collector is running or its exit is unproven; auth recovery denied")


def main():
    base, label, expected = pathlib.Path(sys.argv[1]).resolve(), sys.argv[2], sys.argv[3]
    if not re.fullmatch(r"/srv/storage/butler/agents/issue88-[A-Za-z0-9_-]+", str(base)) or not re.fullmatch(r"[a-z0-9-]+", label) or not re.fullmatch(r"[a-f0-9]{64}", expected):
        raise SystemExit("exact reviewed isolated native recovery reference required")
    proof, private = base/"reports"/label, base/"private"/label
    plan = proof/"prepared-pins.sha256"
    if hashlib.sha256(plan.read_bytes()).hexdigest() != expected or (private/"auth-return.value").exists():
        raise SystemExit("exact unused recovery identity required")
    check = subprocess.run(["sha256sum", "--check", "--status", str(plan)], capture_output=True, timeout=30)
    if check.returncode:
        raise SystemExit("original prepared artifact digests differ; auth recovery denied")
    pins = json.loads((proof/"frozen-pins.json").read_bytes())
    if pins.get("window_revision") != 9 or pins.get("facts_contract_revision") != "5":
        raise SystemExit("exact prospective V9 recovery pins required")
    collector_exited("butler-issue88-"+label+"-native-v9")
    schema = sql("SELECT to_regnamespace('memory_temporal_native_v9') IS NOT NULL")
    table = sql("SELECT to_regclass('public.issue88_native_windows') IS NOT NULL")
    receipt = sql("SELECT coalesce(jsonb_agg(to_jsonb(w)),'[]') FROM public.issue88_native_windows w WHERE window_id='temporal-explicit-v9'") if table else []
    if schema and (len(receipt) != 1 or receipt[0]['window_id'] != 'temporal-explicit-v9'):
        raise SystemExit("owning native schema receipt differs; retain auth for reconciliation")
    if not schema and receipt:
        raise SystemExit("native claim/schema state is inconsistent")
    setting = sql("SELECT to_regclass('memory_temporal_native_v9.system_settings') IS NOT NULL") if schema else False
    value = sql("SELECT jsonb_build_object('value',(SELECT value FROM memory_temporal_native_v9.system_settings WHERE key='BUTLER_PROVIDER_AUTH_ANTIGRAVITY'))")['value'] if setting else None
    source = "latest_durable_encrypted_setting"
    if value is None:
        entries_table = sql("SELECT to_regclass('memory_temporal_native_v9.temporal_native_entries') IS NOT NULL") if schema else False
        entries = sql("SELECT count(*) FROM memory_temporal_native_v9.temporal_native_entries") if entries_table else 0
        if entries != 0 or (proof/"provider-auth-imported.json").exists():
            raise SystemExit("provider may have started without recoverable auth; retain and reconcile")
        material = opaque(private/"auth-import.value")
        if material != opaque(private/"expected-main.value"):
            raise SystemExit("unused original encrypted clone differs")
        source = "unused_original_before_provider_import"
    else:
        try:
            material = value.encode()
            if len(material) > 1024*1024 or len(base64.b64decode(material, validate=True)) < 28:
                raise ValueError()
        except (ValueError, AttributeError):
            raise SystemExit("durable encrypted auth row is unavailable")
    exclusive(private/"auth-return.value", material)
    exclusive(proof/"auth-recovery-safe.json", json.dumps({"collector_running": False, "native_schema_present": schema, "source": source, "raw_material_printed": False, "new_native_authority": False}).encode())
    print(json.dumps({"opaque_return_ready": True, "source": source, "raw_material_printed": False}))


if __name__ == '__main__':
    main()
