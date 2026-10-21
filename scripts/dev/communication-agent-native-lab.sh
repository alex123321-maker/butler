#!/usr/bin/env bash
# One bounded synthetic Core/Broker/native-provider/Telegram acceptance. The
# existing provider-auth utility renews the canonical encrypted row before
# inference. The normal Manager can renew that row during inference through its
# scoped encrypted store; no refresh lineage is copied.
set -Eeuo pipefail
trap 'printf "Native lab failed at line %s; inspect private evidence.\n" "$LINENO" >&2' ERR
base=${1:?owned lab directory}
mode=${2:?prepare, auth, execute, application, branches, corpus or probe}
[[ "$base" == /srv/storage/butler/agents/communication-130-20261021 ]] || exit 2
src=$base/source
umask 077
mkdir -p "$base/private" "$base/reports" "$base/bin" "$base/.cache/go-build" "$base/.cache/go-mod"
exec 9>"$base/.lab.lock"
flock -n 9 || exit 1
trap 'rm -f "$base/private/provider-operator.env"' EXIT INT TERM
if [[ "$mode" == prepare ]]; then
 docker run --rm -v "$src:/workspace:ro" -v "$base/bin:/bin-output" -v "$base/.cache:/cache" -w /workspace \
  -e GOCACHE=/cache/go-build -e GOMODCACHE=/cache/go-mod golang:1.27.2 \
  bash -eu -c 'go test -c -race -o /bin-output/orchestrator.test ./apps/orchestrator/internal/app; go test -c -race -o /bin-output/broker.test ./apps/tool-broker/internal/broker; go test -c -race -o /bin-output/telegramrich.test ./apps/orchestrator/internal/channel/telegramrich; go build -o /bin-output/provider-auth ./cmd/provider-auth; go build -o /bin-output/communication-broker ./apps/tool-broker/cmd/communication-lab; chmod 555 /bin-output/orchestrator.test /bin-output/broker.test /bin-output/telegramrich.test /bin-output/provider-auth /bin-output/communication-broker' \
  > "$base/reports/build.log" 2>&1
 sha256sum "$base/bin/"* > "$base/reports/binaries.sha256"
 printf 'PASS\n' > "$base/reports/prepared"
 printf 'Prepared isolated native acceptance binaries; no provider request.\n'
 exit 0
fi
refresh_provider_auth(){
 # Share the existing operator/native auth lock; do not refresh while another
 # isolated owner or current production run can be using the same lineage.
 exec 8>/srv/storage/butler/agents/native-provider-auth.lock
 flock -n 8 || return 1
 local active
 active=$(docker exec -i butler-main-postgres-1 sh -eu -c 'psql -X -qAt -v ON_ERROR_STOP=1 -U "$POSTGRES_USER" -d "$POSTGRES_DB"' <<'SQL'
SELECT (SELECT count(*) FROM session_ownership WHERE lease_id<>'' AND expires_at>clock_timestamp())+(SELECT count(*) FROM accepted_inputs WHERE status IN('queued','running'));
SQL
 )
 [[ "$active" == 0 ]] || { printf 'Provider renewal deferred: current production work.\n'; return 1; }
 docker inspect butler-main-orchestrator-1 | jq -r '.[0].Config.Env[]|select(startswith("BUTLER_SETTINGS_ENCRYPTION_KEY=") or startswith("BUTLER_POSTGRES_URL=") or startswith("HTTPS_PROXY="))' > "$base/private/provider-operator.env"
 local network
 network=$(docker inspect butler-main-postgres-1 | jq -r '.[0].NetworkSettings.Networks|keys[0]')
 local status=0
 docker run --rm --user "$(id -u):$(id -g)" --network "$network" --cap-drop ALL --security-opt no-new-privileges:true \
  --env-file "$base/private/provider-operator.env" -e NO_PROXY=localhost,127.0.0.1,::1,postgres,communication-agent-130-postgres \
  -v "$base/bin:/bin-output:ro" golang:1.27.2 /bin-output/provider-auth -action diagnose-auth \
  > "$base/reports/auth-refresh.json" 2> "$base/reports/auth-refresh.stderr" || status=$?
 rm -f "$base/private/provider-operator.env"
 [[ "$status" == 0 ]] || return 1
 jq -e '.resolved==true and .unsafe_refresh==false and .after.expired==false' "$base/reports/auth-refresh.json" >/dev/null
}
if [[ "$mode" == auth ]]; then
 test -s "$base/reports/prepared"
 sha256sum --check --status "$base/reports/binaries.sha256"
 refresh_provider_auth
 printf 'Canonical provider auth resolved through normal Manager renewal.\n'
 exit 0
fi
if [[ "$mode" == probe ]]; then
 test -s "$base/reports/prepared"
 sha256sum --check --status "$base/reports/binaries.sha256"
 network=$(docker inspect butler-main-postgres-1 | jq -r '.[0].NetworkSettings.Networks|keys[0]')
 test -n "$network"
 test "$(docker network inspect "$network" --format '{{.Internal}}')" = false
 trap 'rm -f "$base/private/proxy-probe.env"' EXIT INT TERM
 docker inspect butler-main-orchestrator-1 | jq -r '.[0].Config.Env[]|select(startswith("HTTPS_PROXY="))' > "$base/private/proxy-probe.env"
 test "$(grep -c '^HTTPS_PROXY=.' "$base/private/proxy-probe.env")" = 1
 docker run --rm --user "$(id -u):$(id -g)" --network "$network" --cap-drop ALL --security-opt no-new-privileges:true \
  --env-file "$base/private/proxy-probe.env" -e NO_PROXY=localhost,127.0.0.1,::1,postgres,communication-agent-130-postgres \
  -e BUTLER_TEST_MODEL_PROXY_PROBE=1 -v "$base/bin:/bin-output:ro" \
  golang:1.27.2 /bin-output/orchestrator.test -test.v -test.timeout=45s -test.run '^TestNativeCommunicationModelProxyConnectivity$' \
  > "$base/reports/proxy-probe.log" 2>&1
 printf 'Unauthenticated provider proxy connectivity probe completed.\n'
 exit 0
fi
if [[ "$mode" == corpus ]]; then
 test -s "$base/reports/prepared"
 test ! -e "$base/reports/corpus-started"
 sha256sum --check --status "$base/reports/binaries.sha256"
 network=$(docker inspect butler-main-postgres-1 | jq -r '.[0].NetworkSettings.Networks|keys[0]')
 test "$(docker network inspect "$network" --format '{{.Internal}}')" = false
 trap 'rm -f "$base/private/corpus-proxy"' EXIT INT TERM
 docker inspect butler-main-orchestrator-1 | jq -er '.[0].Config.Env[]|select(startswith("HTTPS_PROXY="))|ltrimstr("HTTPS_PROXY=")|select(length>0)' > "$base/private/corpus-proxy"
 chmod 600 "$base/private/corpus-proxy"
 cp "$base/reports/binaries.sha256" "$base/reports/corpus-binaries.sha256"
 printf 'started\n' > "$base/reports/corpus-started"
 docker run --rm --user "$(id -u):$(id -g)" --network "$network" --cap-drop ALL --security-opt no-new-privileges:true \
  -v "$base/bin:/bin-output:ro" -v "$base/private/bot-token:/private/bot-token:ro" -v "$base/private/corpus-proxy:/private/proxy:ro" \
  -e BUTLER_COMMUNICATION_NATIVE_CORPUS=1 -e BUTLER_TEST_TELEGRAM_RICH_TOKEN_FILE=/private/bot-token -e BUTLER_TEST_TELEGRAM_RICH_PROXY_FILE=/private/proxy \
  golang:1.27.2 /bin-output/telegramrich.test -test.v -test.timeout=3m -test.run '^TestNativeCommunicationTelegramFormatCorpus$' > "$base/reports/native-corpus.log" 2>&1
 sha256sum "$base/reports/native-corpus.log" > "$base/reports/native-corpus.sha256"
 printf 'Native transport corpus completed; zero model requests.\n'
 exit 0
fi
[[ "$mode" == execute || "$mode" == application || "$mode" == branches ]] || exit 2
test -s "$base/reports/prepared"
marker=execution-started
[[ "$mode" != application ]] || marker=application-started
[[ "$mode" != branches ]] || marker=branches-started
test ! -e "$base/reports/$marker"
test -s "$base/private/bot-token"
sha256sum --check --status "$base/reports/binaries.sha256"
refresh_provider_auth
# Select the private network shared with the actual provider-auth PostgreSQL.
# The selected existing backend must already support outbound HTTPS. Only the
# new test containers attach; no production container/network is reconfigured.
network=$(docker inspect butler-main-postgres-1 | jq -r '.[0].NetworkSettings.Networks|keys[0]')
test -n "$network"
test "$(docker network inspect "$network" --format '{{.Internal}}')" = false
pg=butler-communication-agent-130-pg
redis=butler-communication-application-130-redis
test -z "$(docker ps -aq --filter name="^${pg}$")"
cleanup(){ docker logs "$pg" > "$base/reports/postgres-startup.log" 2>&1 || true; docker rm -fv "$pg" >/dev/null 2>&1 || true; if [[ "$mode" == application ]];then docker rm -fv "$redis" >/dev/null 2>&1 || true;fi; rm -f "$base/private/provider-runtime.env" "$base/private/telegram-proxy"; }
trap cleanup EXIT INT TERM
mkdir -p "$base/data"
pgdata=$(mktemp -d "$base/data/postgres.XXXXXX")
printf '%s\n' "$pgdata" > "$base/reports/postgres-data.path"
docker run -d --name "$pg" --network "$network" --network-alias communication-agent-130-postgres \
 -v "$pgdata:/var/lib/postgresql/data" \
 -e POSTGRES_HOST_AUTH_METHOD=trust -e POSTGRES_DB=butler_communication_agent_test \
 pgvector/pgvector:pg17 > "$base/reports/postgres.id"
for n in $(seq 1 60); do
 if docker exec "$pg" pg_isready -U postgres -d butler_communication_agent_test >/dev/null 2>&1;then break;fi
 [[ "$n" != 60 ]] || exit 1
 sleep 1
done
# Secret material stays in a private process/file pipe. Only the exact provider
# provider key is exposed to the normal Manager through a refresh-only adapter.
docker inspect butler-main-orchestrator-1 | jq -r '.[0].Config.Env[]|select(startswith("BUTLER_SETTINGS_ENCRYPTION_KEY=") or startswith("BUTLER_POSTGRES_URL=") or startswith("HTTPS_PROXY=") or startswith("NO_PROXY="))|sub("^BUTLER_POSTGRES_URL=";"BUTLER_COMMUNICATION_AUTH_POSTGRES_URL=")' > "$base/private/provider-runtime.env"
test "$(grep -c '^BUTLER_COMMUNICATION_AUTH_POSTGRES_URL=' "$base/private/provider-runtime.env")" = 1
test "$(grep -c '^BUTLER_SETTINGS_ENCRYPTION_KEY=' "$base/private/provider-runtime.env")" = 1
test "$(grep -c '^HTTPS_PROXY=.' "$base/private/provider-runtime.env")" = 1
docker inspect butler-main-orchestrator-1 | jq -er '.[0].Config.Env[]|select(startswith("HTTPS_PROXY="))|ltrimstr("HTTPS_PROXY=")|select(length>0)' > "$base/private/telegram-proxy"
test -s "$base/private/telegram-proxy"
find "$base/private" -maxdepth 1 -type f -exec chmod 600 {} +
cp "$base/reports/binaries.sha256" "$base/reports/$marker-binaries.sha256"
printf 'started\n' > "$base/reports/$marker"
if [[ "$mode" == application ]];then
 test -z "$(docker ps -aq --filter name="^${redis}$")"
 test ! -e "$base/private/application/ready"
 test ! -e "$base/private/application/stop"
 mkdir -p "$base/private/application"
 chmod 700 "$base/private/application"
 redisdata=$(mktemp -d "$base/data/redis.XXXXXX")
 docker run --rm -d --name "$redis" --network "$network" --network-alias communication-application-130-redis -v "$redisdata:/data" redis:7.4-alpine > "$base/reports/application-redis.id"
 docker run --rm --name butler-communication-application-130 --user "$(id -u):$(id -g)" --network "$network" --cap-drop ALL --security-opt no-new-privileges:true \
  -p 127.0.0.1:18470:18470 --env-file "$base/private/provider-runtime.env" \
  -e NO_PROXY=localhost,127.0.0.1,::1,postgres,communication-agent-130-postgres,communication-application-130-redis \
  -v "$src:/workspace:ro" -v "$base/bin:/bin-output:ro" -v "$base/private/bot-token:/private/bot-token:ro" -v "$base/private/telegram-proxy:/private/telegram-proxy:ro" -v "$base/private/application:/private/application" -w /workspace \
  -e BUTLER_TEST_MEMORY_POSTGRES_URL=postgres://postgres@communication-agent-130-postgres:5432/butler_communication_agent_test?sslmode=disable \
  -e BUTLER_REDIS_URL=redis://communication-application-130-redis:6379/0 \
  -e BUTLER_COMMUNICATION_APPLICATION_LAB=1 -e BUTLER_COMMUNICATION_APPLICATION_PRIVATE=/private/application \
  -e BUTLER_COMMUNICATION_BROKER_COMMAND=/bin-output/communication-broker \
  -e BUTLER_TEST_TELEGRAM_RICH_TOKEN_FILE=/private/bot-token -e BUTLER_TEST_TELEGRAM_RICH_PROXY_FILE=/private/telegram-proxy \
  -e BUTLER_TEST_TELEGRAM_RICH_CHAT_ID=1893384316 -e BUTLER_TEST_TELEGRAM_RICH_BOT=butler_rich_lab_20261021_bot \
  golang:1.27.2 /bin-output/orchestrator.test -test.v -test.timeout=15m -test.run '^TestNativeCommunicationApplicationLab$' > "$base/reports/application.log" 2>&1
 sha256sum "$base/reports/application.log" > "$base/reports/application.sha256"
 printf 'Actual application lab completed; private evidence retained.\n'
 exit 0
fi
native_test='^TestNativeCommunicationTelegramAgentIntegration$'
native_log=native
if [[ "$mode" == branches ]];then native_test='^TestNativeCommunicationQuietAndMissingIntegration$';native_log=native-branches;fi
docker run --rm --user "$(id -u):$(id -g)" --network "$network" --cap-drop ALL --security-opt no-new-privileges:true \
 --env-file "$base/private/provider-runtime.env" \
 -e NO_PROXY=localhost,127.0.0.1,::1,postgres,communication-agent-130-postgres \
 -v "$src:/workspace:ro" -v "$base/bin:/bin-output:ro" -v "$base/private/bot-token:/private/bot-token:ro" -v "$base/private/telegram-proxy:/private/telegram-proxy:ro" -w /workspace \
 -e BUTLER_TEST_MEMORY_POSTGRES_URL=postgres://postgres@communication-agent-130-postgres:5432/butler_communication_agent_test?sslmode=disable \
 -e BUTLER_GUARDED_REQUEST_BROKER_BINARY=/bin-output/broker.test \
 -e BUTLER_COMMUNICATION_NATIVE_HARNESS=1 \
 -e BUTLER_TEST_TELEGRAM_RICH_TOKEN_FILE=/private/bot-token \
 -e BUTLER_TEST_TELEGRAM_RICH_PROXY_FILE=/private/telegram-proxy \
 -e BUTLER_TEST_TELEGRAM_RICH_CHAT_ID=1893384316 -e BUTLER_TEST_TELEGRAM_RICH_BOT=butler_rich_lab_20261021_bot \
 golang:1.27.2 /bin-output/orchestrator.test -test.v -test.timeout=8m -test.run "$native_test" \
 > "$base/reports/$native_log.log" 2>&1
sha256sum "$base/reports/$native_log.log" > "$base/reports/$native_log.sha256"
printf 'Native acceptance completed; private structured evidence retained.\n'
