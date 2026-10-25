#!/usr/bin/env bash
# Dedicated tmpfs lab, ordinary encrypted provider reference, no shared rollout.
set -Eeuo pipefail
umask 077
base=/tmp/butler-issue147-20261025
mode=${1:?setup, baseline or candidate}
window=${2:-$mode}
[[ "$window" =~ ^(baseline|candidate|plumbing)(-v[0-9]+)?$ || "$mode" == setup ]] || exit 2
test -d "$base/bin"
exec 9>"$base/lab.lock"
flock -n 9 || { echo 'This issue147 lab is already in use'; exit 1; }
network=$(docker inspect butler-main-postgres-1 | jq -r '.[0].NetworkSettings.Networks|keys[0]')
if [[ "$mode" == setup ]]; then
  docker inspect butler-issue147-postgres >/dev/null 2>&1 && { echo 'Owned container already exists'; exit 1; }
  docker run -d --name butler-issue147-postgres --network "$network" --tmpfs /var/lib/postgresql/data:rw,size=768m --shm-size=128m \
    -e POSTGRES_USER=issue147 -e POSTGRES_PASSWORD=synthetic-issue147 -e POSTGRES_DB=issue147_test pgvector/pgvector:pg17 >/dev/null
  for n in $(seq 1 30); do docker exec butler-issue147-postgres pg_isready -U issue147 -d issue147_test >/dev/null && break; sleep 1; done
  docker exec butler-issue147-postgres pg_isready -U issue147 -d issue147_test >/dev/null
  echo 'Isolated PostgreSQL ready; no shared schema changed.'
  exit 0
fi
[[ "$mode" == baseline || "$mode" == candidate || "$mode" == plumbing ]] || exit 2
test ! -e "$base/reports/$window/started"
mkdir -p "$base/reports/$window"
if [[ "$mode" == plumbing ]]; then
  docker run --rm --name butler-issue147-plumbing --user "$(id -u):$(id -g)" --network "$network" --cap-drop ALL --security-opt no-new-privileges:true --tmpfs /runbin:rw,exec,size=192m,mode=1777 \
    -e BUTLER_TEST_MEMORY_POSTGRES_URL=postgres://issue147:synthetic-issue147@butler-issue147-postgres:5432/issue147_test?sslmode=disable \
    -e BUTLER_CURRENT_REQUEST_REPORT_DIR="/reports/$window" -e BUTLER_GUARDED_REQUEST_BROKER_BINARY=/runbin/broker.test \
    -v "$base/bin:/bin-output:ro" -v "$base/reports:/reports" -v "$base/migrations:/work/github.com/butler/butler/migrations:ro" -w /work \
    golang:1.27.2 bash -eu -c 'cp /bin-output/*.test /runbin/; exec /runbin/candidate-app.test -test.v -test.timeout=3m -test.run "^TestCurrentRequestScopeCoreBrokerIntegration$"' \
    > "$base/reports/$window/test.log" 2>&1
  echo 'Zero-inference integration window finished.'
  exit 0
fi
# Same lock used by existing native qualification tools. Do not copy or print
# credential values; the normal provider Manager resolves the encrypted row.
exec 8>/srv/storage/butler/agents/native-provider-auth.lock
flock -n 8 || { echo 'Native provider qualification is busy'; exit 1; }
envfile="$base/private-provider.env"
trap 'rm -f "$envfile"' EXIT
docker inspect butler-main-orchestrator-1 | jq -r '.[0].Config.Env[]|select(startswith("BUTLER_SETTINGS_ENCRYPTION_KEY=") or startswith("BUTLER_POSTGRES_URL=") or startswith("HTTPS_PROXY="))|if startswith("BUTLER_POSTGRES_URL=") then sub("BUTLER_POSTGRES_URL=";"BUTLER_COMMUNICATION_AUTH_POSTGRES_URL=") else . end' > "$envfile"
test "$(grep -c '^BUTLER_COMMUNICATION_AUTH_POSTGRES_URL=' "$envfile")" = 1
test "$(grep -c '^HTTPS_PROXY=' "$envfile")" = 1
docker run --rm --name "butler-issue147-$mode" --user "$(id -u):$(id -g)" --network "$network" --cap-drop ALL --security-opt no-new-privileges:true --tmpfs /runbin:rw,exec,size=192m,mode=1777 \
  --env-file "$envfile" -e NO_PROXY=localhost,127.0.0.1,::1,postgres,butler-issue147-postgres \
  -e BUTLER_TEST_MEMORY_POSTGRES_URL=postgres://issue147:synthetic-issue147@butler-issue147-postgres:5432/issue147_test?sslmode=disable \
  -e BUTLER_CURRENT_REQUEST_NATIVE_LAB=1 -e BUTLER_CURRENT_REQUEST_REPORT_DIR="/reports/$window" \
  -e BUTLER_GUARDED_REQUEST_BROKER_BINARY=/runbin/broker.test \
  -v "$base/bin:/bin-output:ro" -v "$base/reports:/reports" \
  -v "$base/migrations:/work/github.com/butler/butler/migrations:ro" -w /work \
  golang:1.27.2 bash -eu -c 'cp /bin-output/*.test /runbin/; exec "/runbin/$1-app.test" -test.v -test.timeout=14m -test.run "^TestNativeCurrentRequestScopeLab$"' _ "$mode" \
  > "$base/reports/$window/test.log" 2>&1
echo "$mode native window finished; evidence retained."
