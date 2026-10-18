#!/usr/bin/env python3
"""Lab-only opaque auth handoff. Python standard-library process pipes and
exclusive0600 files avoid shell expansion or displaying raw credential material.
The outer reviewed native runner holds the shared flock throughout every stage.
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

mode, base_arg, label = sys.argv[1:4]
base = pathlib.Path(base_arg).resolve()
if mode not in ("clone", "return", "snapshot", "recover") or not re.fullmatch(r"/srv/storage/butler/agents/issue87-native-[A-Za-z0-9_-]+", str(base)) or label != "native-v5":
    raise SystemExit("isolated handoff reference required")
proof, private = base / "reports" / label, base / "private" / label
schedule = proof / "scheduled-before-auth.json"
if not (proof / "prepared").is_file() or not schedule.is_file():
    raise SystemExit("pre-auth complete schedule and immutable build required")

def exclusive(path, value):
    with os.fdopen(os.open(path, os.O_CREAT | os.O_EXCL | os.O_WRONLY, 0o600), "w") as out:
        out.write(value)
        out.flush()
        os.fsync(out.fileno())

def run(args, script=None):
    try:
        result = subprocess.run(args, input=script, capture_output=True, text=True, timeout=40)
    except (OSError, subprocess.TimeoutExpired):
        raise SystemExit("bounded opaque handoff process unavailable")
    if result.returncode:
        # Fixed SQL/process arguments contain no raw secret. Do not display
        # arbitrary provider/SQL stderr even when diagnostic capture is useful.
        raise SystemExit("opaque handoff operation denied")
    return result.stdout.strip()

def sql(query):
    return run(["docker", "exec", "-i", "butler-lab-postgres-1", "psql", "-X", "-qAt", "-v", "ON_ERROR_STOP=1", "-U", "butler", "-d", "butler"], query)

def opaque(value):
    try:
        if len(value) > 1024*1024 or len(base64.b64decode(value, validate=True)) < 28:
            raise ValueError()
    except ValueError:
        raise SystemExit("encrypted opaque settings row required")
    return value

def read_private(path):
    with os.fdopen(os.open(path, os.O_RDONLY | os.O_NOFOLLOW | os.O_NONBLOCK)) as src:
        info = os.fstat(src.fileno())
        if not stat.S_ISREG(info.st_mode) or info.st_mode & 0o077 or info.st_size > 1024*1024:
            raise SystemExit("private opaque settings reference required")
        return opaque(src.read().strip())

if mode == "clone":
    private.mkdir(mode=0o700)
    exclusive(proof / "auth-stage-begun.json", '{"serialized_fresh_main_clone":true}')
    active = int(sql("SELECT (SELECT count(*) FROM session_ownership WHERE lease_id<>'' AND expires_at>clock_timestamp())+(SELECT count(*) FROM accepted_inputs WHERE status IN('queued','running'));"))
    if active:
        raise SystemExit("main active work; serialized clone denied")
    value = opaque(sql("SELECT value FROM system_settings WHERE key='BUTLER_PROVIDER_AUTH_ANTIGRAVITY';"))
    environment = json.loads(run(["docker", "inspect", "butler-lab-orchestrator-1"]))[0]["Config"]["Env"]
    keys = [item for item in environment if item.startswith("BUTLER_SETTINGS_ENCRYPTION_KEY=")]
    if len(keys) != 1 or not keys[0].split("=", 1)[1] or "\n" in keys[0] or "\r" in keys[0]:
        raise SystemExit("one matching private encryption reference required")
    exclusive(private / "expected-main.value", value)
    exclusive(private / "auth-import.value", value)
    exclusive(private / "native.env", keys[0] + "\n")
    main_env = [item for item in environment if item.split("=",1)[0] in ("BUTLER_SETTINGS_ENCRYPTION_KEY","BUTLER_POSTGRES_URL","HTTP_PROXY","HTTPS_PROXY","ALL_PROXY","NO_PROXY")]
    if not any(item.startswith("HTTPS_PROXY=http://") and ":18080" in item for item in main_env):
        raise SystemExit("main configured server proxy required")
    exclusive(private / "main-diagnose.env", "\n".join(main_env)+"\n")
    print(json.dumps({"main_active_work": active, "opaque_clone_saved": True, "ciphertext_sha256": hashlib.sha256(value.encode()).hexdigest(), "raw_material_printed": False}))
elif mode == "return":
    expected = read_private(private / "expected-main.value")
    latest_path = private / "auth-return.value"
    latest = read_private(latest_path)
    script = "BEGIN; CREATE TEMP TABLE consolidation_auth(value TEXT,kind TEXT) ON COMMIT DROP; COPY consolidation_auth FROM STDIN;\n"+latest+"\tnew\n"+expected+"\texpected\n\\.\n"+"""
DO $handoff$ BEGIN
 IF EXISTS(SELECT 1 FROM session_ownership WHERE lease_id<>'' AND expires_at>clock_timestamp()) OR EXISTS(SELECT 1 FROM accepted_inputs WHERE status IN('queued','running')) THEN RAISE EXCEPTION 'main active work';END IF;
 UPDATE system_settings SET value=(SELECT value FROM consolidation_auth WHERE kind='new'),updated_by='memory-serialized-auth-handback',updated_at=clock_timestamp() WHERE key='BUTLER_PROVIDER_AUTH_ANTIGRAVITY' AND value=(SELECT value FROM consolidation_auth WHERE kind='expected');
 IF NOT FOUND THEN RAISE EXCEPTION 'opaque main CAS denied';END IF;
END $handoff$;COMMIT;
"""
    sql(script)
    print(json.dumps({"opaque_main_cas":True,"active_work_checked":True,"raw_material_printed":False}))
elif mode == "recover":
    # Fixed isolated target only. The collector imports this row before provider
    # construction; its latest ciphertext is authoritative even on fatal exit.
    def isolated(query):
        return run(["docker", "exec", "-i", "butler-issue87-native-v5-pg", "psql", "-X", "-qAt", "-v", "ON_ERROR_STOP=1", "-U", "butler", "-d", "memory_adaptive_test"],
                   "BEGIN TRANSACTION READ ONLY;" + query + ";ROLLBACK;")
    present = json.loads(isolated("SELECT jsonb_build_object('settings',to_regclass('adaptive_native_v5.system_settings') IS NOT NULL,'entries',to_regclass('adaptive_native_v5.adaptive_native_entries') IS NOT NULL,'reservations',to_regclass('adaptive_native_v5.execution_budget_reservations') IS NOT NULL)"))
    latest = isolated("SELECT value FROM adaptive_native_v5.system_settings WHERE key='BUTLER_PROVIDER_AUTH_ANTIGRAVITY'") if present["settings"] else ""
    if latest:
        latest = opaque(latest)
        unused = False
    else:
        for key, table in (("entries", "adaptive_native_entries"), ("reservations", "execution_budget_reservations")):
            if present[key] and isolated("SELECT count(*) FROM adaptive_native_v5." + table) != "0":
                raise SystemExit("collector may have used auth; opaque fallback denied")
        latest = read_private(private / "auth-import.value")
        if latest != read_private(private / "expected-main.value"):
            raise SystemExit("unused opaque clone differs")
        unused = True
    exclusive(private / "auth-return.value", latest)
    print(json.dumps({"isolated_latest_exported": not unused, "proven_unused_clone": unused, "raw_material_printed": False}))
else:
    value = opaque(sql("SELECT value FROM system_settings WHERE key='BUTLER_PROVIDER_AUTH_ANTIGRAVITY';"))
    exclusive(private / "main-after-native.value", value)
    print(json.dumps({"fresh_main_snapshot_saved":True,"ciphertext_sha256":hashlib.sha256(value.encode()).hexdigest(),"raw_material_printed":False}))
