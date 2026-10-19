#!/usr/bin/env python3
"""Read-only content-free deployment evidence. Stdlib operator tooling only."""
import datetime
import hashlib
import json
from pathlib import Path
import subprocess

ROOT = Path('/srv/storage/butler/main')


def command(args, data=None):
    return subprocess.run(args, input=data, stdout=subprocess.PIPE, stderr=subprocess.PIPE, check=True).stdout


def query(sql):
    raw = command(['docker', 'exec', '-i', 'butler-main-postgres-1', 'psql', '-X', '-qAt', '-v', 'ON_ERROR_STOP=1', '-U', 'butler', '-d', 'butler'],
                  ('select coalesce(json_agg(row_to_json(x)),\'[]\') from (' + sql + ') x').encode())
    return json.loads(raw)


config = json.loads((ROOT / 'compose.json').read_text())
ids = command(['docker', 'ps', '-aq', '--filter', 'label=com.docker.compose.project=butler-main']).decode().split()
containers = json.loads(command(['docker', 'inspect', *ids]))
states = []
for item in containers:
    states.append({'service': item['Config']['Labels']['com.docker.compose.service'], 'state': item['State']['Status'],
                   'health': item['State'].get('Health', {}).get('Status'), 'image_id': item['Image'],
                   'restart_policy': item['HostConfig']['RestartPolicy']['Name']})
core = config['services']['orchestrator']['environment']
broker = config['services']['tool-broker']['environment']
report = {'checked_at': datetime.datetime.now(datetime.timezone.utc).isoformat(), 'project': 'butler-main',
          'service_source': {'orchestrator': '1d345ff11bd892663ab02a4272ab041640d74951', 'broker': 'f1753cd0b0ebf09beb25056fb5c65dfc03893f04'},
          'compose_sha256': hashlib.sha256((ROOT / 'compose.json').read_bytes()).hexdigest(),
          'binary_sha256': {p.name: hashlib.sha256(p.read_bytes()).hexdigest() for p in sorted((ROOT / 'bin').iterdir()) if p.is_file()},
          'flags': {name: {k: v for k, v in env.items() if k.endswith('_ENABLED') or k in ('BUTLER_AUTONOMY_MODE', 'BUTLER_MODEL_PROVIDER', 'BUTLER_ANTIGRAVITY_MODEL', 'BUTLER_ANTIGRAVITY_MAXIMUM_OUTPUT_TOKENS')}
                    for name, env in [('orchestrator', core), ('tool-broker', broker)]}, 'services': states,
          'migrations': query('select count(*) as applied from schema_migrations'),
          'mobile_devices': query('select status,accepted_count,duplicate_count,rejected_count,last_seen_at from mobile_devices order by created_at'),
          'mobile_messages': query('select state,count(*) as count from mobile_messages group by state'),
          'mobile_ingress': query('select count(*) as count,max(received_at) as newest from mobile_events'),
          'mobile_source': query("select enabled,revision from initiative_sources where source_id='phone-attention'"),
          'mobile_batches': query('select (finished_at is not null) as finished,count(*) as count from mobile_attention_batches group by (finished_at is not null)'),
          'recent_input_states': query("select source,status,error_code,count(*) as count from accepted_inputs where accepted_at>'2026-10-18T18:54:00Z' group by source,status,error_code"),
          'recent_tool_states': query("select t.tool_name,t.status,count(*) as count from tool_calls t join runs r using(run_id) where r.started_at>'2026-10-18T18:54:00Z' group by t.tool_name,t.status"),
          'unknown_effects_retained': query("select state,count(*) as count from action_effects where state='effect_unknown' group by state"),
          'model_worker_control': query('select state from memory_model_control'),
          'active_phone_runs': query("select r.status,r.current_state,r.error_type from runs r join tasks t using(task_id) where t.source_json ? 'mobile_batch_id' order by r.started_at desc limit 3"),
          'owner_policy_required': True, 'quality_noninferiority': 'not_established',
          'blocked_features': {'adaptive_retrieval': 'code_release_qualification_false', 'episodic_experience': 'code_release_qualification_false',
                               'guarded_user_tab': 'host_metadata_credentials_not_provisioned', 'russian_voice': 'experimental_quality_and_composition_gate_not_passed',
                               'legacy_memory_pipeline': 'superseded_pipeline_not_reactivated'}}
path = ROOT / 'reports/deployment-verification.json'
path.write_text(json.dumps(report, ensure_ascii=False, indent=2) + '\n')
path.chmod(0o600)
print(json.dumps({'services': len(states), 'healthy': sum(s['health'] == 'healthy' for s in states),
                  'mobile_messages': report['mobile_messages'], 'active_phone_runs': report['active_phone_runs']}))
