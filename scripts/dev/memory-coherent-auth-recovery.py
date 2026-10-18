#!/usr/bin/env python3
"""Return only the original owning92 durable auth after proven collector exit.

Reuses the existing90 opaque-file/process fences. The restored old backup is
never an auth source. No encrypted setting appears in args, public proof/output.
"""
import base64
import hashlib
import importlib.util
import json
import pathlib
import re
import subprocess
import sys

sys.dont_write_bytecode = True
spec = importlib.util.spec_from_file_location('coherent_auth_fences', pathlib.Path(__file__).with_name('memory-experience-auth-recovery.py'))
fences = importlib.util.module_from_spec(spec)
spec.loader.exec_module(fences)


def sql(label, query):
    result = subprocess.run(['docker', 'exec', '-i', 'butler-issue92-'+label+'-native-pg', 'psql', '-X', '-qAt', '-v', 'ON_ERROR_STOP=1', '-U', 'butler', '-d', 'coherent_native_test'], input='BEGIN TRANSACTION READ ONLY;'+query+';ROLLBACK;', text=True, capture_output=True, timeout=30)
    if result.returncode:
        raise SystemExit('original native92 durable auth metadata unavailable')
    value = result.stdout.strip()
    return True if value == 't' else False if value == 'f' else json.loads(value)


def recover(base, label, expected):
    if not re.fullmatch('/srv/storage/butler/agents/issue92-native-[A-Za-z0-9_-]+', str(base)) or not re.fullmatch('[a-z0-9-]+', label) or not re.fullmatch('[a-f0-9]{64}', expected):
        raise SystemExit('exact isolated92 recovery identity required')
    proof, private = base/'reports'/label, base/'private'/label
    plan = proof/'preseal.json'
    if plan.is_symlink() or plan.stat().st_size > 16*1024*1024 or hashlib.sha256(plan.read_bytes()).hexdigest() != expected or (private/'auth-return.value').exists():
        raise SystemExit('exact unused92 recovery reference required')
    fences.collector_exited('butler-issue92-'+label+'-native')
    schema = sql(label, "SELECT to_regnamespace('coherent_native_v1') IS NOT NULL")
    table = sql(label, "SELECT to_regclass('public.issue92_native_windows') IS NOT NULL")
    receipt = sql(label, "SELECT coalesce(jsonb_agg(to_jsonb(w)),'[]') FROM public.issue92_native_windows w WHERE window_id='coherent-whole-v1'") if table else []
    if schema and (len(receipt) != 1 or receipt[0]['preseal_sha256'] != expected or receipt[0]['schema_name'] != 'coherent_native_v1' or receipt[0]['owner_id'] != 'owner') or not schema and receipt:
        raise SystemExit('native92 original domain/schema identity differs')
    setting = sql(label, "SELECT to_regclass('coherent_native_v1.system_settings') IS NOT NULL") if schema else False
    value = sql(label, "SELECT jsonb_build_object('value',(SELECT value FROM coherent_native_v1.system_settings WHERE key='BUTLER_PROVIDER_AUTH_ANTIGRAVITY'))")['value'] if setting else None
    source = 'latest_original_owning_durable_encrypted_setting_after_process_exit'
    if value is None:
        for name in ('coherent_native_entries', 'execution_model_attempts', 'memory_model_attempts'):
            exists = sql(label, "SELECT to_regclass('coherent_native_v1.%s') IS NOT NULL" % name) if schema else False
            if exists and sql(label, 'SELECT count(*) FROM coherent_native_v1.'+name) != 0:
                raise SystemExit('native92 admits work without recoverable auth; retain and reconcile')
        if (proof/'provider-auth-imported.json').exists():
            raise SystemExit('native92 imported auth is unavailable; retain and reconcile')
        material = fences.opaque(private/'auth-import.value')
        if material != fences.opaque(private/'expected-main.value'):
            raise SystemExit('unused92 original encrypted clone differs')
        source = 'unused_original_before_provider_import'
    else:
        try:
            material = value.encode()
            if len(material) > 1024*1024 or len(base64.b64decode(material, validate=True)) < 28:
                raise ValueError()
        except (ValueError, AttributeError):
            raise SystemExit('native92 durable encrypted row malformed')
    fences.exclusive(private/'auth-return.value', material)
    fences.exclusive(proof/'auth-recovery-safe.json', json.dumps({'collector_running': False, 'source': source, 'original_database': 'coherent_native_test', 'restored_auth_ignored': True, 'raw_material_printed': False, 'new_native_authority': False}).encode())
    return {'opaque_return_ready': True, 'source': source, 'raw_material_printed': False}


if __name__ == '__main__':
    try:
        print(json.dumps(recover(pathlib.Path(sys.argv[1]).resolve(), sys.argv[2], sys.argv[3])))
    except (OSError, KeyError, TypeError, ValueError, subprocess.TimeoutExpired):
        raise SystemExit('native92 exited-process auth recovery unavailable; retain and reconcile')
