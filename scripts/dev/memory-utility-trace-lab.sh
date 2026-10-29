#!/bin/sh
# Fresh isolated real Core/Broker/PG, scripted transport only; no native provider.
set -eu
umask 077
r=${1:?fresh utility trace root required}
case "$r" in /srv/gitea/butler-acceptance/issue115-utility-trace-*) ;; *) exit 2;; esac
test -f "$r/source/go.mod"
mkdir "$r/reports" "$r/reports/cases" "$r/tmp" "$r/pgdata"
name=$(basename "$r")
net=$name-net
pg=$name-pg
made_net=0
made_pg=0
cleanup() {
 status=$?
 trap - EXIT
 if [ "$made_pg" = 1 ]; then docker stop -t 15 "$pg" > "$r/reports/pg-stop.txt"; docker network disconnect "$net" "$pg"; fi
 if [ "$made_net" = 1 ]; then docker network rm "$net" > "$r/reports/network-removed.txt"; fi
 printf '{"exit_code":%s,"native_requests":0,"controlled_transport":true}\n' "$status" > "$r/reports/exit.json"
 exit "$status"
}
trap cleanup EXIT
image=sha256:5cf287a799e6b94384bad13d16b14904c531f51ba65792237e122ce42b392f61
docker run --rm --network none --cpus 1 --memory 3g --memory-swap 3g --pids-limit 256 \
 --read-only --cap-drop ALL --security-opt no-new-privileges --log-driver none --user 1000:1000 \
 --tmpfs /tmp:rw,nosuid,nodev,size=128m -v "$r/source:/source:ro" -v "$r/reports:/reports" \
 -v "$r/cache:/cache" -v "$r/tmp:/build-tmp" -v /srv/storage/butler/.cache/go-mod:/modules:ro \
 -e GOCACHE=/cache -e GOTMPDIR=/build-tmp -e GOMODCACHE=/modules -e GOPROXY=off -e GOSUMDB=off \
 -e GOTOOLCHAIN=local -e GOMAXPROCS=1 -e GOFLAGS='-p=1 -buildvcs=false -mod=readonly' -w /source "$image" sh -eu -c '
 go version
 go test -race -count=1 -run "^TestMemoryUtility" ./internal/acceptance
 go vet ./internal/acceptance/utilitytrace ./apps/orchestrator/internal/app
 go build -o /reports/broker ./apps/tool-broker
 go test -c -o /reports/extension.test ./apps/tool-extension/internal/runtime
 go test -c -o /reports/http.test ./apps/tool-http/internal/runtime
 go test -race -c -o /reports/app.test ./apps/orchestrator/internal/app
 go build -o /reports/test2json cmd/test2json
 sha256sum /reports/broker /reports/extension.test /reports/http.test /reports/app.test
 ' > "$r/reports/build.log" 2>&1
if [ -n "${UTILITY_TRACE_SUBNET:-}" ]; then
 docker network create --internal --subnet "$UTILITY_TRACE_SUBNET" "$net" > "$r/reports/network.id"
else
 docker network create --internal "$net" > "$r/reports/network.id"
fi
made_net=1
docker run -d --name "$pg" --network "$net" --cpus 1 --memory 768m --memory-swap 768m \
 -e POSTGRES_USER=utility_fixture -e POSTGRES_PASSWORD=utility_fixture_only -e POSTGRES_DB=utility_trace_test \
 -v "$r/pgdata:/var/lib/postgresql/data" \
 sha256:ac08538c6f8b9904c33c8224c5e5706dbe760aca29db1d096972b4052c22a75d > "$r/reports/pg.id"
made_pg=1
ready=0
for n in $(seq 1 40); do if docker exec "$pg" pg_isready -U utility_fixture -d utility_trace_test >/dev/null 2>&1; then ready=1; break; fi; sleep 1; done
test "$ready" = 1
docker run --rm --network "container:$pg" --cpus 1 --memory 3g --memory-swap 3g --pids-limit 256 \
 --read-only --cap-drop ALL --security-opt no-new-privileges --log-driver none --user 1000:1000 \
 --tmpfs /tmp:rw,nosuid,nodev,size=512m -v "$r/source:/source:ro" -v "$r/reports:/reports" \
 -e BUTLER_TEST_MEMORY_POSTGRES_URL='postgres://utility_fixture:utility_fixture_only@127.0.0.1:5432/utility_trace_test?sslmode=disable' \
 -e BUTLER_TEST_ACTIONAUTH_POSTGRES_URL='postgres://utility_fixture:utility_fixture_only@127.0.0.1:5432/utility_trace_test?sslmode=disable' -e BUTLER_EXTENSION_E2E=1 \
 -e BUTLER_BROKER_E2E_BINARY=/reports/broker -e BUTLER_EXTENSION_E2E_BINARY=/reports/extension.test \
 -e BUTLER_EXPERIENCE_HTTP_BINARY=/reports/http.test -e BUTLER_UTILITY_TRACE_OFFLINE=1 \
 -e BUTLER_UTILITY_TRACE_OUTPUT=/reports/cases -e GOMAXPROCS=1 -w /source \
 "$image" /reports/test2json -t -p github.com/butler/butler/apps/orchestrator/internal/app \
 /reports/app.test -test.v -test.count=1 -test.timeout=12m -test.run '^(TestMemoryUtilityActualCoreBrokerTraceIntegration|TestReviewedSkillRealBrokerDurableApprovalE2E)$' \
 > "$r/reports/tests.jsonl" 2>&1
sha256sum "$r/reports/tests.jsonl" > "$r/reports/tests.sha256"
