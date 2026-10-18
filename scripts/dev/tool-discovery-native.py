#!/usr/bin/env python3
"""Operator-only isolated native comparison. Python stdlib handles Docker/JSON;
all model, policy, discovery and tool execution remains in compiled Go services.
Never prints private environment files or provider credentials.
"""
import datetime
import hashlib
import json
import os
from pathlib import Path
import subprocess
import sys
import time
import urllib.request

ROOT = Path('/srv/storage/butler/agents/tool-discovery-native-20261019')
PROJECT = 'butler-discovery-native-20261019'
COMPOSE = ['docker', 'compose', '-p', PROJECT, '-f', str(ROOT / 'compose.json')]
PG = PROJECT + '-postgres-1'
API = 'http://127.0.0.1:18192'


def command(args, *, stdin=None):
    p = subprocess.run(args, input=stdin, stdout=subprocess.PIPE, stderr=subprocess.PIPE)
    if p.returncode:
        raise RuntimeError('operator command failed: ' + args[0] + ' ' + args[1])
    return p.stdout


def sql(query):
    return command(['docker', 'exec', '-i', PG, 'psql', '-X', '-v', 'ON_ERROR_STOP=1', '-qAt', '-U', 'butler', '-d', 'butler_discovery_native_test'], stdin=query.encode()).decode().strip()


def save(path, value):
    path.write_text(json.dumps(value, ensure_ascii=False, indent=2) + '\n')
    path.chmod(0o600)


def private_env():
    values = {}
    for line in private_read('agent.env').splitlines():
        if line and not line.startswith('#'):
            k, v = line.split('=', 1)
            values[k] = v
    return values


def private_read(name):
    return command(['docker', 'run', '--rm', '--user', '10001:10001', '-v', str(ROOT / 'private') + ':/private:ro',
                    'alpine:3.21', 'cat', '/private/' + name]).decode()


def prepare():
    if (ROOT / 'compose.json').exists():
        raise RuntimeError('existing experiment: preparation cannot reset it')
    env = private_env()
    policy = json.loads(env['BUTLER_EXECUTION_BUDGET_POLICY_JSON'])
    # Fresh finite scopes; never reuse or reset historical reservations.
    for key, turns in [('conversation', 8), ('task', 8), ('mandate', 48), ('day', 48)]:
        policy.setdefault(key, {}).update(model_turns=turns, tokens=67108864, active_millis=14400000,
                                          provider_input_bytes=536870912, external_calls=96, egress_bytes=134217728)
    policy.update(model_millis=300000, output_tokens=32768)
    common = {'BUTLER_POSTGRES_URL': 'postgres://butler:discovery-private-stand@discovery-postgres:5432/butler_discovery_native_test?sslmode=disable',
              'BUTLER_POSTGRES_MAX_CONNS': '4', 'BUTLER_POSTGRES_MIN_CONNS': '0',
              'BUTLER_HTTP_ADDR': ':8080', 'BUTLER_GRPC_ADDR': ':9090',
              'BUTLER_ACTION_AUTHORITY_TOKEN_FILE': '/private/action.token',
              'BUTLER_MEMORY_DELETION_MANIFEST_FILE': '/state/deletions.log',
              'BUTLER_LOG_LEVEL': 'info', 'BUTLER_EXECUTION_BUDGET_POLICY_JSON': json.dumps(policy, separators=(',', ':')),
              'BUTLER_MEMORY_READ_ENABLED': 'true', 'BUTLER_SCHEDULES_ENABLED': 'true',
              'BUTLER_TOOL_DISCOVERY_ENABLED': 'false'}
    core = dict(common, BUTLER_SETTINGS_ENCRYPTION_KEY=env['BUTLER_SETTINGS_ENCRYPTION_KEY'],
                BUTLER_OWNER_CONTROL_OWNER_ID=env['BUTLER_OWNER_CONTROL_OWNER_ID'],
                BUTLER_OWNER_CONTROL_TOKEN_FILE='/private/owner.token', BUTLER_REDIS_URL='redis://discovery-redis:6379/0',
                BUTLER_TOOL_BROKER_ADDR='discovery-broker:9090', BUTLER_MODEL_PROVIDER='antigravity',
                BUTLER_ANTIGRAVITY_MODEL='gemini-3.8-flash-high', BUTLER_ANTIGRAVITY_MAXIMUM_OUTPUT_TOKENS='32768',
                BUTLER_OPENAI_TIMEOUT_SECONDS='300', BUTLER_AUTONOMY_MODE='mode_2', BUTLER_MEMORY_PIPELINE_ENABLED='false',
                HTTPS_PROXY='http://butler-lab-antigravity-proxy-1:18080',
                NO_PROXY='localhost,127.0.0.1,discovery-postgres,discovery-redis,discovery-broker,discovery-core')
    broker = dict(common, BUTLER_TOOL_REGISTRY_PATH='/candidate/tools.json',
                  BUTLER_ACTION_CONTEXT_URL='http://discovery-core:8080/internal/action-context',
                  BUTLER_MEMORY_READ_CONTROL_URL='http://discovery-core:8080/internal/memory-read',
                  BUTLER_SCHEDULES_CONTROL_URL='http://discovery-core:8080/internal/schedules')
    mounts = [str(ROOT / 'bin') + ':/candidate:ro', str(ROOT / 'private') + ':/private:ro',
              str(ROOT / 'state') + ':/state']
    def service(image, environment, **kw):
        return dict(image=image, environment=environment, restart='no', **kw)
    services = {
        'postgres': service('pgvector/pgvector:pg17', {'POSTGRES_USER': 'butler', 'POSTGRES_PASSWORD': 'discovery-private-stand',
                           'POSTGRES_DB': 'butler_discovery_native_test'}, volumes=[str(ROOT / 'pgdata') + ':/var/lib/postgresql/data'],
                           networks={'default': {'aliases': ['discovery-postgres']}}),
        'redis': service('redis:7.4-alpine', {}, networks={'default': {'aliases': ['discovery-redis']}}),
        'broker': service('butler-lab-tool-broker', broker, entrypoint=['/candidate/broker'], volumes=mounts,
                          networks={'default': {'aliases': ['discovery-broker']}}),
        'orchestrator': service('butler-lab-orchestrator', core, entrypoint=['/candidate/orchestrator'], volumes=mounts,
                                ports=['127.0.0.1:18192:8080'], networks={'default': {'aliases': ['discovery-core']}, 'proxy': {}}),
        'doctor': service('butler-lab-tool-doctor', {'BUTLER_HTTP_ADDR': ':8080', 'BUTLER_GRPC_ADDR': ':9090',
                            'BUTLER_DOCTOR_CORE_DIAGNOSTICS_URL': 'http://discovery-core:8080/internal/diagnostics',
                            'BUTLER_DOCTOR_CONTAINER_TARGETS': 'orchestrator=http://discovery-core:8080/health,tool-broker=http://discovery-broker:8080/health'},
                          networks={'default': {'aliases': ['discovery-doctor']}}),
        'migrator': service('golang:1.27.2', {'BUTLER_POSTGRES_URL': common['BUTLER_POSTGRES_URL']},
                            entrypoint=['/candidate/migrator', '-migrations-dir', '/migrations'],
                            volumes=[mounts[0], str(ROOT / 'source/migrations') + ':/migrations:ro'])}
    save(ROOT / 'compose.json', {'services': services, 'networks': {'default': {'ipam': {'config': [{'subnet': '10.237.219.0/24'}]}}, 'proxy': {'external': True, 'name': 'butler-lab_backend'}}})
    (ROOT / 'state').mkdir(mode=0o700)
    # A fresh namespace has no content or deletion history; initialize once.
    with (ROOT / 'state/deletions.log').open('x') as anchor:
        anchor.write('butler-memory-deletions-v1\n')
    (ROOT / 'state/deletions.log').chmod(0o600)
    command(['docker', 'run', '--rm', '-v', str(ROOT / 'state') + ':/state', '-v', str(ROOT / 'private') + ':/private',
             'alpine:3.21', 'sh', '-c', 'chown -R 10001:10001 /state /private; chmod 700 /state /private; chmod 600 /private/*'])
    registry = json.loads((ROOT / 'source/configs/tools.json').read_text())
    for tool in registry['tools']:
        if tool['tool_class'] == 'doctor':
            tool['runtime_target'] = 'discovery-doctor:9090'
    save(ROOT / 'bin/tools.json', registry)
    (ROOT / 'bin/tools.json').chmod(0o644)
    start()


def start():
    if list((ROOT / 'reports').glob('*.admitted')):
        raise RuntimeError('preparation is sealed after first admission')
    config = json.loads((ROOT / 'compose.json').read_text())
    config['networks']['default']['ipam'] = {'config': [{'subnet': '10.237.219.0/24'}]}
    save(ROOT / 'compose.json', config)
    (ROOT / 'bin/tools.json').chmod(0o644)
    command(COMPOSE + ['up', '-d', 'postgres', 'redis', 'doctor'])
    for _ in range(60):
        try:
            if sql('select 1') == '1':
                break
        except RuntimeError:
            time.sleep(1)
    command(COMPOSE + ['run', '--rm', 'migrator'])
    sql(private_read('provider.sql'))
    print(json.dumps({'prepared': True, 'production_data_copied': False, 'native_calls': 0}))


CASES = [
    {'id': 'database', 'text': 'Проверь доступность PostgreSQL и Redis через инструменты диагностики. Назови отдельный статус каждого. Ничего не меняй.', 'tools': ['doctor.check_database']},
    {'id': 'containers', 'text': 'Проверь через диагностику здоровье контейнера orchestrator. Сообщи фактически полученный статус. Ничего не меняй.', 'tools': ['doctor.check_container']},
    {'id': 'provider', 'text': 'Проверь инструментом диагностики состояние конфигурации провайдера antigravity. Отдельно отметь, доказывает ли эта проверка реальную генерацию. Ничего не меняй.', 'tools': ['doctor.check_provider']},
    {'id': 'no-tool', 'text': 'Вычисли 17 * 23. Ответь только целым числом. Не используй инструменты.', 'tools': [], 'answer': '391'},
]


def request(path, body=None):
    token = private_read('owner.token').strip()
    data = None if body is None else json.dumps(body).encode()
    req = urllib.request.Request(API + path, data=data, headers={'Authorization': 'Bearer ' + token, 'Content-Type': 'application/json'})
    with urllib.request.urlopen(req, timeout=12) as response:
        return json.load(response)


def run(arm, index):
    if arm not in ('eager', 'deferred') or index not in range(len(CASES)):
        raise RuntimeError('invalid frozen label')
    label = 'v5-' + arm + '-' + str(index)
    marker = ROOT / ('reports/' + label + '.admitted')
    # Atomic owner-authorized single admission; do not unlink after failure.
    with marker.open('x') as handle:
        handle.write(datetime.datetime.now(datetime.timezone.utc).isoformat())
    config = json.loads((ROOT / 'compose.json').read_text())
    config['services']['orchestrator']['networks']['default']['aliases'] = ['discovery-core', 'orchestrator']
    config['services']['orchestrator']['environment']['NO_PROXY'] = 'localhost,127.0.0.1,discovery-postgres,discovery-redis,discovery-broker,discovery-core,orchestrator,oauth2.googleapis.com'
    for name in ('BUTLER_ACTION_CONTEXT_URL', 'BUTLER_MEMORY_READ_CONTROL_URL', 'BUTLER_SCHEDULES_CONTROL_URL'):
        config['services']['broker']['environment'][name] = config['services']['broker']['environment'][name].replace('discovery-core:8080', 'orchestrator:8080')
    for name in ('broker', 'orchestrator'):
        config['services'][name]['environment']['BUTLER_TOOL_DISCOVERY_ENABLED'] = str(arm == 'deferred').lower()
    save(ROOT / 'compose.json', config)
    capture()
    command(COMPOSE + ['up', '-d', 'broker', 'orchestrator'])
    healthy = False
    for _ in range(60):
        try:
            with urllib.request.urlopen(API + '/health', timeout=2) as response:
                if response.status == 200:
                    healthy = True
                    break
        except Exception:
            time.sleep(1)
    if not healthy:
        raise RuntimeError('stand not healthy; no input admitted')
    command(['docker', 'exec', PROJECT + '-broker-1', 'wget', '-qO-', 'http://127.0.0.1:8080/health'])
    case = CASES[index]
    key = 'discovery-native-20261019-' + label
    payload = {'event_id': key, 'idempotency_key': key, 'event_type': 'user_message', 'source': 'web',
               'session_key': key, 'payload': {'text': case['text'], 'intent': 'conversation'}}
    start = time.monotonic()
    receipt = request('/api/v1/events', payload)
    save(ROOT / ('reports/' + label + '-receipt.json'), receipt)
    final = None
    for _ in range(900):
        row = sql("select coalesce(json_agg(row_to_json(x)),'[]') from (select input_id,status,run_id,error_code from accepted_inputs where session_key='" + key + "') x")
        rows = json.loads(row)
        if rows and rows[0]['status'] in ('completed', 'failed', 'blocked', 'expired', 'awaiting_approval'):
            final = rows[0]
            break
        time.sleep(1)
    elapsed = time.monotonic() - start
    capture()
    save(ROOT / ('reports/' + label + '-result.json'), {'arm': arm, 'case': case, 'elapsed_seconds': elapsed, 'input': final,
                                                        'source_sha256': hashlib.sha256((ROOT / 'source.tar.gz').read_bytes()).hexdigest()})
    print(json.dumps({'arm': arm, 'case': index, 'elapsed_seconds': round(elapsed, 3), 'input': final}))


def capture():
    log = subprocess.run(['docker', 'logs', PROJECT + '-orchestrator-1'], stdout=subprocess.PIPE, stderr=subprocess.PIPE)
    records = []
    for line in log.stdout.splitlines():
        try:
            record = json.loads(line)
            if record.get('msg') == 'model turn usage observed':
                records.append({k: record[k] for k in ('time', 'run_id', 'operation_id', 'input_tokens', 'output_tokens', 'total_tokens')})
        except (ValueError, KeyError):
            pass
    path = ROOT / 'reports/complete-usage.json'
    prior = json.loads(path.read_text()) if path.exists() else []
    by_id = {r['operation_id']: r for r in prior + records}
    save(path, list(by_id.values()))


if __name__ == '__main__':
    try:
        if sys.argv[1] == 'prepare':
            prepare()
        elif sys.argv[1] == 'start':
            start()
        elif sys.argv[1] == 'repair-empty-preparation':
            if sql('select count(*) from execution_model_attempts') != '0' or sql('select count(*) from memory_sources') != '0':
                raise RuntimeError('nonempty preparation cannot be repaired')
            command(['docker', 'run', '--rm', '-i', '-v', str(ROOT / 'state') + ':/state', 'alpine:3.21',
                     'sh', '-c', 'cat > /state/deletions.log; chown 10001:10001 /state/deletions.log; chmod 600 /state/deletions.log'],
                    stdin=b'butler-memory-deletions-v1\n')
            save(ROOT / 'reports/preparation-v1.json', {'native_calls': 0, 'cause': 'glibc_binary_in_alpine_and_empty_anchor',
                                                       'change': 'static_go_build_and_explicit_empty_namespace_anchor_initialization'})
        elif sys.argv[1] == 'run':
            run(sys.argv[2], int(sys.argv[3]))
        elif sys.argv[1] == 'capture':
            capture()
        else:
            raise RuntimeError('unknown operator action')
    except Exception as exc:
        # Never expose arbitrary provider errors or subprocess private output.
        print(json.dumps({'status': 'operator_action_failed', 'error_type': type(exc).__name__,
                          'phase': str(exc) if isinstance(exc, RuntimeError) else 'private_operation'}))
        sys.exit(1)
