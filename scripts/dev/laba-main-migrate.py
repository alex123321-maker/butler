#!/usr/bin/env python3
"""One-use operator deployment assembly. Stdlib Python handles Compose JSON and
private file copying; production services and ingress remain Go/Node. Does not
reset databases, deletion anchors, authority, budgets, or activate owner policies.
"""
import hashlib
import json
import os
from pathlib import Path
import secrets
import subprocess
import sys

ROOT = Path('/srv/storage/butler/main')
NATIVE = Path('/srv/storage/butler/agents/tool-discovery-native-20261019')


def command(args, data=None):
    r = subprocess.run(args, input=data, stdout=subprocess.PIPE, stderr=subprocess.PIPE)
    if r.returncode:
        raise RuntimeError('private operator command failed: ' + args[0])
    return r.stdout


def write(path, data, mode=0o600):
    if isinstance(data, str):
        data = data.encode()
    path.write_bytes(data)
    path.chmod(mode)


def inspect(name):
    return json.loads(command(['docker', 'inspect', name]))[0]


def prepare():
    if (ROOT / 'compose.json').exists():
        raise RuntimeError('existing deployment assembly is preserved')
    for name in ('agent.env', 'owner.token', 'action.token'):
        raw = command(['docker', 'run', '--rm', '--user', '10001:10001', '-v', str(NATIVE / 'private') + ':/private:ro',
                       'alpine:3.21', 'cat', '/private/' + name])
        write(ROOT / 'private' / name, raw)
    source = dict(line.split('=', 1) for line in (ROOT / 'private/agent.env').read_text().splitlines() if line and not line.startswith('#'))
    old = dict(line.split('=', 1) for line in inspect('butler-lab-orchestrator-1')['Config']['Env'] if '=' in line)
    ext = dict(line.split('=', 1) for line in inspect('butler-lab-tool-broker-1')['Config']['Env'] if '=' in line)
    policy = json.loads(source['BUTLER_EXECUTION_BUDGET_POLICY_JSON'])
    policy.update(output_tokens=32768, model_millis=300000)
    policy['models']['local:google/embeddinggemma-2'] = {'basis': 'local_no_billing'}
    password = secrets.token_hex(32)
    write(ROOT / 'private/postgres.pass', password)
    for name in ('converter', 'workspace'):
        write(ROOT / ('private/' + name + '.token'), secrets.token_hex(48))
    ext_token = next(m['Source'] for m in inspect('butler-lab-tool-broker-1')['Mounts'] if 'extension-runtime' in m['Destination'])
    command(['docker', 'run', '--rm', '-v', ext_token + ':/input:ro', '-v', str(ROOT / 'private') + ':/output',
             'alpine:3.21', 'cp', '/input', '/output/extension.token'])
    dsn = 'postgres://butler:' + password + '@postgres:5432/butler?sslmode=disable'
    common = {'BUTLER_POSTGRES_URL': dsn, 'BUTLER_POSTGRES_MAX_CONNS': '4', 'BUTLER_POSTGRES_MIN_CONNS': '0',
              'BUTLER_HTTP_ADDR': ':8080', 'BUTLER_GRPC_ADDR': ':9090',
              'BUTLER_ACTION_AUTHORITY_TOKEN_FILE': '/run/secrets/action',
              'BUTLER_MEMORY_DELETION_MANIFEST_FILE': '/state/deletions.log',
              'BUTLER_EXECUTION_BUDGET_POLICY_JSON': json.dumps(policy, separators=(',', ':')),
              'BUTLER_OWNER_CONTROL_OWNER_ID': source['BUTLER_OWNER_CONTROL_OWNER_ID'],
              'BUTLER_LOG_LEVEL': 'info'}
    for feature in ('MEMORY_READ', 'MEMORY_CANDIDATE', 'MEMORY_TEMPORAL', 'DELEGATION', 'CONVERTER_DELEGATION', 'WORKSPACE', 'SCHEDULES', 'TOOL_DISCOVERY'):
        common['BUTLER_' + feature + '_ENABLED'] = 'true'
    profile = json.dumps({'dependency': {'id': 'butler-converter-ir', 'version': '1.0.0',
                         'sha256': hashlib.sha256((ROOT / 'bin/tool-converter').read_bytes()).hexdigest()},
                         'checker': 'butler.delimited-json.checks.v1'}, separators=(',', ':'))
    core = dict(source, **common)
    core.update(BUTLER_POSTGRES_URL=dsn.replace('@postgres:', '@butler-main-postgres-1:'),
                BUTLER_SETTINGS_ENCRYPTION_KEY=source['BUTLER_SETTINGS_ENCRYPTION_KEY'], BUTLER_REDIS_URL='redis://butler-main-redis-1:6379/0',
                BUTLER_OWNER_CONTROL_TOKEN_FILE='/run/secrets/owner', BUTLER_TOOL_BROKER_ADDR='tool-broker:9090',
                BUTLER_ANTIGRAVITY_MAXIMUM_OUTPUT_TOKENS='32768', BUTLER_OPENAI_TIMEOUT_SECONDS='300',
                BUTLER_MEMORY_PIPELINE_ENABLED='false', BUTLER_MEMORY_ADAPTIVE_RETRIEVAL_ENABLED='false', BUTLER_MEMORY_EXPERIENCE_ENABLED='false',
                BUTLER_MEMORY_SEMANTIC_ENABLED='true', BUTLER_MEMORY_PDF_PARSER_ENABLED='true',
                BUTLER_MEMORY_MODEL_JOBS_ENABLED='true', BUTLER_MEMORY_MODEL_CANDIDATES_ENABLED='true',
                BUTLER_MEMORY_SUMMARY_CONSUMER_ENABLED='true', BUTLER_MEMORY_CONSOLIDATION_CONSUMER_ENABLED='true',
                BUTLER_MEMORY_LOCAL_SERVICE_ID=old['BUTLER_MEMORY_LOCAL_SERVICE_ID'], BUTLER_MEMORY_LOCAL_TOKEN_FILE='/run/secrets/local-model',
                BUTLER_SKILL_CONTROL_URL='http://tool-broker:8080/internal/skills', BUTLER_EXTENSION_CONTROL_URL='http://tool-broker:8080/internal/extensions',
                BUTLER_CONVERTER_CONTROL_URL='http://tool-broker:8080/internal/converters', BUTLER_CONVERTER_PROFILE_JSON=profile,
                HTTPS_PROXY='http://butler-lab-antigravity-proxy-1:18080',
                NO_PROXY='localhost,127.0.0.1,postgres,redis,tool-broker,orchestrator,local-models,memory-parser,oauth2.googleapis.com')
    for key, value in old.items():
        if key.startswith('BUTLER_VOICE_'):
            core[key] = value
    broker = dict(common, BUTLER_TOOL_REGISTRY_PATH='/candidate/tools.json', BUTLER_SKILLS_ENABLED='true', BUTLER_CONVERTERS_ENABLED='true',
                  BUTLER_ACTION_CONTEXT_URL='http://orchestrator:8080/internal/action-context',
                  BUTLER_MEMORY_READ_CONTROL_URL='http://orchestrator:8080/internal/memory-read',
                  BUTLER_MEMORY_CANDIDATE_CONTROL_URL='http://orchestrator:8080/internal/memory-candidate',
                  BUTLER_MEMORY_FACT_CONTROL_URL='http://orchestrator:8080/internal/memory-facts',
                  BUTLER_SCHEDULES_CONTROL_URL='http://orchestrator:8080/internal/schedules',
                  BUTLER_DELEGATION_CONTROL_URL='http://orchestrator:8080/internal/delegation',
                  BUTLER_CONVERTER_DELEGATION_CONTROL_URL='http://orchestrator:8080/internal/converter-delegation',
                  BUTLER_EXTENSION_PROFILES_JSON=ext['BUTLER_EXTENSION_PROFILES_JSON'], BUTLER_EXTENSION_RUNTIME_TOKEN_FILE='/run/secrets/extension',
                  BUTLER_CONVERTER_PROFILE_JSON=profile, BUTLER_CONVERTER_RUNTIME_TARGET='unix:///run/butler-converter/runtime.sock',
                  BUTLER_CONVERTER_RUNTIME_TOKEN_FILE='/run/secrets/converter', BUTLER_WORKSPACE_PROFILE_ID='main-workspace-v1',
                  BUTLER_WORKSPACE_RUNTIME_TARGET='unix:///run/butler-workspace/runtime.sock', BUTLER_WORKSPACE_RUNTIME_TOKEN_FILE='/run/secrets/workspace')
    bins = str(ROOT / 'bin') + ':/candidate:ro'
    state = str(ROOT / 'state') + ':/state'
    def secret(name):
        return str(ROOT / ('private/' + name + '.token')) + ':/run/secrets/' + name + ':ro'
    def service(image, environment, networks, binary=None, **kwargs):
        value = dict(image=image, restart='unless-stopped', environment=environment, networks=networks, **kwargs)
        if binary:
            value.update(entrypoint=['/candidate/' + binary], read_only=True, cap_drop=['ALL'], security_opt=['no-new-privileges:true'],
                         pids_limit=128, mem_limit='512m')
            value.setdefault('volumes', []).append(bins)
        return value
    services = {
        'postgres': service('pgvector/pgvector:pg17', {'POSTGRES_USER': 'butler', 'POSTGRES_DB': 'butler', 'POSTGRES_PASSWORD': password},
                            ['backend'], volumes=[str(ROOT / 'data/postgres') + ':/var/lib/postgresql/data']),
        'redis': service('redis:7.4-alpine', {}, ['backend'], command=['redis-server', '--appendonly', 'yes'],
                         volumes=[str(ROOT / 'data/redis') + ':/data']),
        'orchestrator': service('butler-lab-orchestrator', core, {'backend': {'aliases': ['agent']}, 'diagnostics': {}, 'proxy': {}, 'local-models': {}, 'parser': {}},
                                'orchestrator', ports=['192.168.0.5:8088:8080'],
                                volumes=[state, secret('action'), secret('owner')]),
        'tool-broker': service('butler-lab-tool-broker', broker, ['backend', 'tool-runtime'], 'broker',
                               volumes=[state + ':ro', secret('action'), secret('extension'), secret('converter'), secret('workspace'),
                                        str(ROOT / 'converter-socket') + ':/run/butler-converter:ro', str(ROOT / 'workspace-socket') + ':/run/butler-workspace:ro']),
        'migrator': service('butler-lab-migrator', {'BUTLER_POSTGRES_URL': dsn}, ['backend'], 'migrator',
                            command=['-migrations-dir', '/migrations'], volumes=[str(NATIVE / 'source/migrations') + ':/migrations:ro']),
        'web': service('butler-lab-web', {'NUXT_PUBLIC_API_BASE': 'http://192.168.0.5:8088', 'HOST': '0.0.0.0', 'PORT': '3000'}, ['backend'],
                        ports=['192.168.0.5:3080:3000'], volumes=[str(ROOT / 'web-output') + ':/app/.output:ro'],
                        read_only=True, cap_drop=['ALL'], security_opt=['no-new-privileges:true'], mem_limit='512m'),
        'mobile-ingress': service('butler-lab-orchestrator', {}, ['backend'], 'wifi-proxy', ports=['192.168.0.5:18061:8081']),
        'tool-doctor': service('butler-lab-tool-doctor', {'BUTLER_HTTP_ADDR': ':8080', 'BUTLER_GRPC_ADDR': ':9090',
                                'BUTLER_DOCTOR_CORE_DIAGNOSTICS_URL': 'http://orchestrator:8080/internal/diagnostics',
                                'BUTLER_DOCTOR_CONTAINER_TARGETS': 'orchestrator=http://orchestrator:8080/health,tool-broker=http://tool-broker:8080/health,tool-browser=http://tool-browser:8080/health,tool-http=http://tool-http:8080/health,tool-webfetch=http://tool-webfetch:8080/health,tool-doctor=http://tool-doctor:8080/health'},
                                ['tool-runtime', 'diagnostics'], 'tool-doctor'),
        'tool-converter': service('butler-lab-tool-extension', {'BUTLER_HTTP_ADDR': '127.0.0.1:8080', 'BUTLER_CONVERTER_UNIX_SOCKET': '/run/butler-converter/runtime.sock',
                                   'BUTLER_CONVERTER_PROFILE_JSON': profile, 'BUTLER_CONVERTER_RUNTIME_TOKEN_FILE': '/run/secrets/converter'}, [], 'tool-converter',
                                  network_mode='none', volumes=[secret('converter'), str(ROOT / 'converter-socket') + ':/run/butler-converter'],
                                  tmpfs=['/tmp:size=4m,noexec,nosuid,nodev,uid=10001,gid=10001,mode=0700']),
        'tool-workspace': service('butler-lab-tool-extension', {'BUTLER_POSTGRES_URL': dsn, 'BUTLER_POSTGRES_MAX_CONNS': '2',
                                  'BUTLER_HTTP_ADDR': '127.0.0.1:8086', 'BUTLER_GRPC_ADDR': 'unix:///run/butler-workspace/runtime.sock',
                                  'BUTLER_WORKSPACE_PROFILE_ID': 'main-workspace-v1', 'BUTLER_WORKSPACE_ROOT': '/workspaces',
                                  'BUTLER_WORKSPACE_RUNTIME_TOKEN_FILE': '/run/secrets/workspace', 'BUTLER_MEMORY_DELETION_MANIFEST_FILE': '/state/deletions.log'},
                                  ['backend'], 'tool-workspace', volumes=[state + ':ro', secret('workspace'), str(ROOT / 'workspaces') + ':/workspaces', str(ROOT / 'workspace-socket') + ':/run/butler-workspace']),
        'tool-extension': service('butler-lab-tool-extension', {'BUTLER_HTTP_ADDR': ':8080', 'BUTLER_GRPC_ADDR': ':9090',
                                   'BUTLER_EXTENSION_PROFILES_JSON': ext['BUTLER_EXTENSION_PROFILES_JSON'], 'BUTLER_EXTENSION_RUNTIME_TOKEN_FILE': '/run/secrets/extension'},
                                  ['tool-runtime', 'tool-egress'], 'tool-extension', volumes=[secret('extension')])}
    services['tool-converter'].pop('networks')
    for name in ('tool-http', 'tool-webfetch', 'tool-browser', 'tool-browser-local'):
        env = {'BUTLER_HTTP_ADDR': ':8080', 'BUTLER_GRPC_ADDR': ':9090'}
        nets = ['tool-runtime', 'tool-egress']
        if name == 'tool-browser-local':
            nets = ['backend', 'tool-runtime']
            env.update(BUTLER_TOOL_BROWSER_LOCAL_ORCHESTRATOR_URL='http://orchestrator:8080', BUTLER_TOOL_BROWSER_LOCAL_DISPATCH_MODE='orchestrator_relay', BUTLER_TOOL_BROWSER_LOCAL_ROLLOUT_MODE='remote_preferred')
        services[name] = service('butler-lab-' + name, env, nets, name)
        if name == 'tool-browser':
            services[name].update(mem_limit='1g', pids_limit=256, tmpfs=['/tmp:rw,noexec,nosuid,size=256m,mode=1777', '/home/butler:rw,noexec,nosuid,size=64m,uid=10001,gid=10001,mode=700'])
    model_token = next(m['Source'] for m in inspect('butler-lab-orchestrator-1')['Mounts'] if m['Destination'] == '/private-memory-models/token')
    services['orchestrator']['volumes'].append(model_token + ':/run/secrets/local-model:ro')
    networks = {name: {'ipam': {'config': [{'subnet': '10.237.' + str(220 + i) + '.0/24'}]}, 'internal': name in ('tool-runtime', 'diagnostics')}
                for i, name in enumerate(('backend', 'tool-runtime', 'tool-egress', 'diagnostics'))}
    for name, actual in [('proxy', 'backend'), ('local-models', 'local-inference'), ('parser', 'memory-parsing')]:
        networks[name] = {'external': True, 'name': 'butler-lab_' + actual}
    write(ROOT / 'compose.json', json.dumps({'name': 'butler-main', 'services': services, 'networks': networks}, indent=2))
    # Use the unchanged canonical registry, not the stand's redirected Doctor.
    write(ROOT / 'bin/tools.json', (NATIVE / 'source/configs/tools.json').read_bytes(), 0o644)
    command(['docker', 'run', '--rm', '-v', str(ROOT) + ':/main', 'alpine:3.21', 'sh', '-c',
             'chown -R 10001:10001 /main/private /main/state /main/workspaces /main/workspace-socket /main/converter-socket; chmod 700 /main/private /main/state /main/workspaces /main/workspace-socket /main/converter-socket; chmod 600 /main/private/*'])
    command(['docker', 'compose', '-f', str(ROOT / 'compose.json'), 'config', '--quiet'])
    print(json.dumps({'assembled': True, 'started': False, 'private_values_emitted': False}))


if __name__ == '__main__':
    try:
        if sys.argv[1] == 'prepare':
            prepare()
        elif sys.argv[1] == 'repair-assembly':
            config = json.loads((ROOT / 'compose.json').read_text())
            core = config['services']['orchestrator']['environment']
            core['BUTLER_POSTGRES_URL'] = core['BUTLER_POSTGRES_URL'].replace('@postgres:', '@butler-main-postgres-1:')
            core['BUTLER_REDIS_URL'] = 'redis://butler-main-redis-1:6379/0'
            (ROOT / 'bin/wifi-proxy').chmod(0o755)
            write(ROOT / 'compose.json', json.dumps(config, indent=2))
        else:
            raise RuntimeError('unsupported migration action')
    except Exception as exc:
        print(json.dumps({'status': 'assembly_failed', 'type': type(exc).__name__}))
        sys.exit(1)
