$ErrorActionPreference='Stop'
$script=@'
set -euo pipefail
umask 077
root=/srv/storage/butler/reviews/issue33-v17-collector-pg-20261013
source=/srv/storage/butler/agents/scenario-release-chain-v17-high-20261013/snapshots/20261014-033559-144
[[ -d "$source" && ! -L "$source" ]]
name="context-pg-$(date -u +%Y%m%dT%H%M%SZ)-$$"; net="$name-net"; pg="$name-pg"
proof="$root/reports/$name"; mkdir -p "$proof/private/postgres"
created_net=false; created_pg=false
cleanup(){ $created_pg && docker logs --tail 80 "$pg" > "$proof/pg-startup.log" 2>&1 || true; $created_pg && docker rm -f "$pg" >/dev/null 2>&1 || true; $created_net && docker network rm "$net" >/dev/null 2>&1 || true; }
trap cleanup EXIT INT TERM
(cd "$source"; find . -type f -print0 | sort -z | xargs -0 sha256sum) > "$proof/source.sha256"
docker network create --internal "$net" >/dev/null; created_net=true
docker run -d --runtime runc --name "$pg" --network "$net" --network-alias context-pg --memory 512m --pids-limit 64 --cpus 1 --security-opt no-new-privileges:true --volume "$proof/private/postgres:/var/lib/postgresql/data" --env POSTGRES_USER=fixture --env POSTGRES_PASSWORD=context-fixture --env POSTGRES_DB=butler_context_test sha256:ac08538c6f8b9904c33c8224c5e5706dbe760aca29db1d096972b4052c22a75d -c shared_buffers=32MB -c max_connections=32 -c work_mem=4MB >/dev/null; created_pg=true
for n in $(seq 1 40); do docker exec "$pg" pg_isready -h 127.0.0.1 -U fixture -d butler_context_test >/dev/null 2>&1 && break; [[ "$n" != 40 ]] || exit 1; sleep 1; done
docker exec "$pg" psql -X -q -v ON_ERROR_STOP=1 -U fixture -d butler_context_test -c 'CREATE EXTENSION IF NOT EXISTS vector WITH SCHEMA public' >/dev/null
docker run --rm --runtime runc --pull never --network "$net" --user "$(id -u):$(id -g)" --memory 1536m --pids-limit 160 --cpus 2 --security-opt no-new-privileges:true --volume "$source:/src:ro" --volume "$proof:/proof" --volume /srv/storage/butler/.cache:/cache --workdir /src --env GOPROXY=off --env GOSUMDB=off --env GOMAXPROCS=2 --env GOFLAGS=-p=2 --env GOCACHE=/cache/go-race --env GOMODCACHE=/cache/go-mod --env GOTMPDIR=/cache/go-tmp --env 'BUTLER_TEST_ACTIONAUTH_POSTGRES_URL=postgres://fixture:context-fixture@context-pg:5432/butler_context_test?sslmode=disable' --env 'BUTLER_TEST_MEMORY_POSTGRES_URL=postgres://fixture:context-fixture@context-pg:5432/butler_context_test?sslmode=disable' sha256:5cf287a799e6b94384bad13d16b14904c531f51ba65792237e122ce42b392f61 bash -eu -c '
 go test -race -json -count=1 -run "^(TestReleaseChainV17PendingFaultOrder|TestReleaseChainV17CaseFailureContinuation|TestReleaseChainV17InvalidOriginalStillRetainsOwningEvidenceIntegration)$" ./apps/orchestrator/internal/app > /proof/tests.jsonl
 go vet ./apps/orchestrator/internal/app
' > "$proof/diagnostics.log" 2>&1
! grep -Eq '"Action":"(fail|skip)"' "$proof/tests.jsonl"
[[ "$(awk 'index($0,"\"Action\":\"pass\"") && index($0,"\"Test\":") {n++} END{print n+0}' "$proof/tests.jsonl")" == 5 ]]
printf 'ACTUAL_PG_V17_RETAINED_COLLECTOR=%s\n' "$proof"
sha256sum "$proof/tests.jsonl" "$proof/source.sha256" "$proof/diagnostics.log"
'@
$encoded=[Convert]::ToBase64String([Text.Encoding]::UTF8.GetBytes($script.Replace("`r`n","`n")))
ssh laba "printf '%s' '$encoded' | base64 -d | bash -s"
if($LASTEXITCODE-ne0){throw 'PG current-context proof failed'}
