#!/usr/bin/env bash
set -euo pipefail
base=${1:?isolated root}
[[ "$base" == /srv/storage/butler/agents/issue82-20261015 ]] || exit 2
export ISSUE82_ROOT=$base
compose=(docker compose -p butler-issue82 -f "$base/current/deploy/memory-jobs/compose.yml" -f "$base/current/deploy/memory-jobs/native.compose.yml")
"${compose[@]}" run --rm verify sh -c 'go build -o /proof/migrator ./apps/migrator'
"${compose[@]}" run --rm --no-deps --entrypoint /proof/migrator orchestrator > "$base/reports/native-migrations.log" 2>&1
# Copy only the existing provider's encrypted row via private process pipes.
# No secret is returned to the operator, model or evidence files.
docker exec butler-lab-postgres-1 psql -U butler -d butler -c "COPY (SELECT * FROM system_settings WHERE key='BUTLER_PROVIDER_AUTH_ANTIGRAVITY') TO STDOUT" |
 docker exec -i butler-issue82-postgres-1 psql -v ON_ERROR_STOP=1 -U butler -d memory_jobs_test -c 'COPY system_settings FROM STDIN' > "$base/private/auth-import-receipt"
"${compose[@]}" up -d orchestrator
printf 'Isolated services started; model queue remains administratively disabled.\n'
