#!/usr/bin/env bash
# Read-only native accounting projection, never an inference or credential read.
set -euo pipefail
base=${1:?isolated root}
label=${2:?fresh audit label}
[[ "$base" == /srv/storage/butler/agents/issue82-20261015 && "$label" =~ ^audit-[a-zA-Z0-9_-]+$ ]] || exit 2
proof=$base/reports/$label
test ! -e "$proof"
umask 077
mkdir "$proof"
docker exec -i butler-issue82-postgres-1 psql -v ON_ERROR_STOP=1 -U butler -d memory_jobs_test -At <<'SQL' > "$proof/ledger.json"
SELECT jsonb_build_object(
 'attempts',(SELECT jsonb_agg(jsonb_build_object('operation_id',operation_id,'job_id',job_id,'state',state,'usage',usage,'quote',quote,'reason',reason,'provider',provider,'model',model,'locality',locality,'source_fingerprint',source_fingerprint,'reservation_id',reservation_id,'result_source_id',result_source_id,'created_at',created_at)) FROM memory_model_attempts),
 'foreground_attempts',(SELECT count(*) FROM execution_model_attempts),
 'scopes',(SELECT jsonb_agg(jsonb_build_object('scope_id',scope_id,'kind',kind,'used',used_json,'limits',limits_json,'deadline',deadline)) FROM execution_budget_scopes),
 'model_results',(SELECT count(*) FROM memory_sources WHERE source_kind='model_candidate'),
 'control',(SELECT state FROM memory_model_control),
 'run_states',(SELECT jsonb_agg(jsonb_build_object('run_id',run_id,'state',current_state)) FROM runs));
SQL
curl -fsS http://127.0.0.1:8088/api/v1/providers | jq '{active_provider,providers:[.providers[]|{name,model,active,connected}]}' > "$proof/deployed-provider.json"
docker inspect butler-lab-orchestrator-1 --format '{{.Image}} {{index .Config.Labels "com.docker.compose.project.working_dir"}}' > "$proof/main-deployment.txt"
docker inspect butler-issue82-orchestrator-1 --format '{{.State.Status}}' > "$proof/isolated-service.txt"
date -u +%Y-%m-%dT%H:%M:%SZ > "$proof/observed-at.txt"
