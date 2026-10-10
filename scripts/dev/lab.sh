#!/usr/bin/env bash
set -euo pipefail
action=${1:-Status}
root=${2:-/srv/storage/butler}
shift 2
cd "$root/current"
export BUTLER_LAB_ROOT="$root"
export DOCKER_CONFIG="$root/.cache/docker"
export BUTLER_ACTION_AUTHORITY_TOKEN_SOURCE="$root/secrets/action-authority.token"
export BUTLER_OWNER_CONTROL_TOKEN_SOURCE="$root/secrets/owner-control.token"
mkdir -p "$DOCKER_CONFIG" "$root/.cache/go-build" "$root/.cache/go-mod" "$root/.cache/go-tmp" "$root/reports"
# These suites intentionally share disposable PostgreSQL fixtures. Separate
# invocations must not reset approval deliveries while another suite is running.
if [[ "$action" == Integration || "$action" == Race || "$action" == Migrations ]]; then
  exec 9>"$root/.cache/postgres-tests.lock"
  flock 9
fi
compose=(docker compose -p butler-lab --env-file "$root/lab.env" -f deploy/docker-compose.yml -f deploy/docker-compose.lab.yml)
if [[ -f "$root/private-proxy/compose.env" ]]; then
  compose+=(--env-file "$root/private-proxy/compose.env" -f deploy/docker-compose.antigravity-proxy.lab.yml)
fi
if [[ -f "$root/extensions/compose.env" ]]; then
  compose+=(--env-file "$root/extensions/compose.env" -f deploy/docker-compose.extensions.yml)
fi
if [[ -f "$root/local-models/compose.env" ]]; then
  compose+=(--env-file "$root/local-models/compose.env" -f deploy/docker-compose.local-models.lab.yml --profile local-models)
fi
test_options=()
if [[ "$action" == Integration || "$action" == Race ]]; then
  for database in butler_lab_core_test butler_lab_actionauth_test butler_lab_completion_test; do
    if [[ "$(docker exec butler-lab-postgres-1 psql -U butler -d postgres -Atc "SELECT 1 FROM pg_database WHERE datname = '$database'")" != 1 ]]; then
      docker exec butler-lab-postgres-1 createdb -U butler "$database"
    fi
  done
  test_options=(--network butler-lab_backend
    --env BUTLER_TEST_POSTGRES_URL=postgres://butler:butler-dev-password@postgres:5432/butler_lab_core_test?sslmode=disable
    --env BUTLER_TEST_GOALS_POSTGRES_URL=postgres://butler:butler-dev-password@postgres:5432/butler_lab_core_test?sslmode=disable
    --env BUTLER_TEST_INITIATIVE_POSTGRES_URL=postgres://butler:butler-dev-password@postgres:5432/butler_lab_core_test?sslmode=disable
    --env BUTLER_TEST_MEMORY_POSTGRES_URL=postgres://butler:butler-dev-password@postgres:5432/butler_lab_core_test?sslmode=disable
    --env BUTLER_TEST_SEMANTIC_POSTGRES_URL=postgres://butler:butler-dev-password@postgres:5432/butler_lab_core_test?sslmode=disable
    --env BUTLER_TEST_ACTIONAUTH_POSTGRES_URL=postgres://butler:butler-dev-password@postgres:5432/butler_lab_actionauth_test?sslmode=disable
    --env BUTLER_TEST_COMPLETION_POSTGRES_URL=postgres://butler:butler-dev-password@postgres:5432/butler_lab_completion_test?sslmode=disable
    --env BUTLER_TEST_REDIS_URL=redis://redis:6379/1)
fi
case "$action" in
  Up)
    BUILDX_BUILDER=$(bash scripts/dev/lab-builder.sh "$root")
    export BUILDX_BUILDER
    if [[ ! -f "$BUTLER_ACTION_AUTHORITY_TOKEN_SOURCE" ]]; then
      install -d -m 700 "$root/secrets"
      (umask 077; openssl rand -hex 48 > "$BUTLER_ACTION_AUTHORITY_TOKEN_SOURCE")
    fi
    if [[ ! -f "$BUTLER_OWNER_CONTROL_TOKEN_SOURCE" ]]; then
      install -d -m 700 "$root/secrets"
      (umask 077; openssl rand -hex 48 > "$BUTLER_OWNER_CONTROL_TOKEN_SOURCE")
    fi
    # Compose file-backed secrets preserve host ownership. Both core services
    # run as UID10001 and need to read this private file without widening mode.
    docker run --rm --user 0:0 --volume "$root/secrets:/private" \
      --entrypoint sh alpine:3.21 -c \
      'chown 10001:10001 /private/action-authority.token /private/owner-control.token; chmod 600 /private/action-authority.token /private/owner-control.token'
    if [[ ! -f "$root/lab.env" ]]; then
      umask 077
      {
        printf 'BUTLER_ENVIRONMENT=test\nBUTLER_LOG_LEVEL=info\n'
        printf 'BUTLER_LAB_API_PUBLIC_URL=http://%s:8088\n' "$(hostname -I | awk '{print $1}')"
        printf 'BUTLER_SETTINGS_ENCRYPTION_KEY=%s\n' "$(openssl rand -hex 32)"
        printf 'BUTLER_EXTENSION_API_TOKENS=%s\n' "$(openssl rand -hex 32)"
      } > "$root/lab.env"
    fi
    "${compose[@]}" config --quiet
    report="$root/reports/deploy-$(date -u +%Y%m%dT%H%M%SZ).log"
    printf 'Building and starting lab; detailed log: %s\n' "$report"
    if ! "${compose[@]}" --progress plain up -d --build --wait --wait-timeout 240 > "$report" 2>&1; then
      tail -n 80 "$report"
      exit 1
    fi
    "${compose[@]}" ps
    ;;
  Test|Integration)
    (( $# > 0 )) || set -- ./apps/orchestrator/...
    report="$root/reports/go-$(date -u +%Y%m%dT%H%M%SZ).log"
    docker run --rm --user "$(id -u):$(id -g)" \
      --volume "$PWD:/src:ro" --volume "$root/.cache:/cache" --workdir /src \
      --env GOCACHE=/cache/go-build --env GOMODCACHE=/cache/go-mod \
      --env GOTMPDIR=/cache/go-tmp \
      "${test_options[@]}" golang:1.27.2-alpine go test -p 1 -count=1 "$@" 2>&1 | tee "$report"
    ;;
  Migrations)
    # Upgrade/rollback fixtures must never share the app or integration database.
    for database in butler_lab_task_migrations_test butler_lab_summary_migrations_test; do
      if [[ "$(docker exec butler-lab-postgres-1 psql -U butler -d postgres -Atc "SELECT 1 FROM pg_database WHERE datname = '$database'")" != 1 ]]; then
        docker exec butler-lab-postgres-1 createdb -U butler "$database"
      fi
    done
    report="$root/reports/migrations-$(date -u +%Y%m%dT%H%M%SZ).log"
    docker run --rm --user "$(id -u):$(id -g)" --network butler-lab_backend \
      --volume "$PWD:/src:ro" --volume "$root/.cache:/cache" --workdir /src \
      --env GOCACHE=/cache/go-build --env GOMODCACHE=/cache/go-mod \
      --env GOTMPDIR=/cache/go-tmp \
      --env BUTLER_TASK_MIGRATION_POSTGRES_URL=postgres://butler:butler-dev-password@postgres:5432/butler_lab_task_migrations_test?sslmode=disable \
      --env BUTLER_SUMMARY_MIGRATION_POSTGRES_URL=postgres://butler:butler-dev-password@postgres:5432/butler_lab_summary_migrations_test?sslmode=disable \
      golang:1.27.2-alpine go test -p 1 -count=1 -run MigrationIntegration \
      ./apps/orchestrator/internal/task ./internal/memory/summary 2>&1 | tee "$report"
    bash scripts/smoke/durable-migration-audit.sh "$PWD" butler-lab-postgres-1 \
      "butler_durable_migrations_$(date -u +%Y%m%d%H%M%S)_$$" 2>&1 | tee -a "$report"
    ;;
  Status)
    "${compose[@]}" ps
    df -h "$root" /
    ;;
  Race)
    (( $# > 0 )) || set -- ./apps/orchestrator/internal/orchestrator
    mkdir -p "$root/.cache/go-race"
    report="$root/reports/race-$(date -u +%Y%m%dT%H%M%SZ).log"
    docker run --rm --user "$(id -u):$(id -g)" \
      --volume "$PWD:/src:ro" --volume "$root/.cache:/cache" --workdir /src \
      --env GOCACHE=/cache/go-race --env GOMODCACHE=/cache/go-mod \
      --env GOTMPDIR=/cache/go-tmp \
      "${test_options[@]}" golang:1.27.2-bookworm go test -race -p 1 -count=1 "$@" 2>&1 | tee "$report"
    ;;
  Web)
    mkdir -p "$root/.cache/npm" "$root/.cache/ms-playwright"
    report="$root/reports/web-$(date -u +%Y%m%dT%H%M%SZ).log"
    docker run --rm --user "$(id -u):$(id -g)" --network butler-lab_backend \
      --volume "$PWD:/src" --volume "$root/.cache:/cache" --workdir /src/web \
      --env NPM_CONFIG_CACHE=/cache/npm --env PLAYWRIGHT_BROWSERS_PATH=/cache/ms-playwright \
      --env BUTLER_LAB_SMOKE_BASE_URL=http://web:3000 --env BUTLER_LAB_SMOKE_SCREENSHOT=/cache/lab-ui.png \
      --entrypoint bash butler-lab-tool-browser -c \
      'set -eu; npm ci; npm run typecheck; npm run lint; npx --no-install playwright install chromium; npm run test:e2e -- --grep-invert screenshots; node /src/scripts/dev/lab-web-smoke.mjs' \
      2>&1 | tee "$report"
    ;;
  *) printf 'Unknown action: %s\n' "$action" >&2; exit 2 ;;
esac
