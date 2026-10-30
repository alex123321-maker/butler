#!/usr/bin/env bash
set -euo pipefail
root=${1:?owned browser-owner test root}
[[ "$root" =~ ^/srv/storage/butler/agents/server-browser-owners-[0-9]{8}$ ]] || exit 2
src=${2:-$(readlink -f "$root/current")}
[[ "$src" == "$root/snapshots/"* || "$src" =~ ^/tmp/butler-owner-build-[a-f0-9]{12}/core$ || "$src" == /tmp/butler-owner-read-review-20261029-v2 ]] || exit 2
mkdir -p "$root/reports" "$root/.cache/go-tmp" "$root/.cache/docker"
export DOCKER_CONFIG="$root/.cache/docker"
exec 9>"$root/.cache/validation.lock";flock -n 9
run="$(date -u +%Y%m%dT%H%M%SZ)-$$"
proof="$root/reports/$run";mkdir "$proof"
printf '%s\n' "$src" > "$proof/source.txt"
pg="butler-owner-outcomes-$run"
go_image=$(docker image inspect --format '{{.Id}}' golang:1.27.2-bookworm)
pg_image=$(docker image inspect --format '{{.Id}}' pgvector/pgvector:pg17)
created=false
cleanup(){ local code=$?;if $created;then docker logs "$pg" > "$proof/postgres.log" 2>&1 || true;docker rm -f "$pg" >/dev/null;fi;printf '{"exit_code":%d,"native_model_calls":0}\n' "$code" > "$proof/exit.json"; }
trap cleanup EXIT
docker run -d --name "$pg" --network none --memory 1g --cpus 1 --pids-limit 128 --tmpfs /var/lib/postgresql/data:rw,size=768m -e POSTGRES_HOST_AUTH_METHOD=trust -e POSTGRES_DB=owner_test "$pg_image" -c max_connections=40 -c max_wal_size=64MB -c min_wal_size=32MB -c shared_buffers=32MB >/dev/null
created=true
for i in $(seq 1 30);do docker exec "$pg" pg_isready -U postgres -d owner_test >/dev/null 2>&1 && break;sleep 1;done
docker exec "$pg" pg_isready -U postgres -d owner_test >/dev/null
args=(--rm --user "$(id -u):$(id -g)" --memory 3g --cpus 2 --pids-limit 256 --security-opt no-new-privileges:true
 --network "container:$pg" -v "$src:/src:ro" -v "$root/.cache:/cache" -v /srv/storage/butler/.cache/go-mod:/modules:ro -w /src
 -e GOCACHE=/cache/go-build -e GOMODCACHE=/modules -e GOTMPDIR=/cache/go-tmp -e GOPROXY=off -e GOSUMDB=off
 -e BUTLER_TEST_ACTIONAUTH_POSTGRES_URL='postgres://postgres@127.0.0.1:5432/owner_test?sslmode=disable'
 -e BUTLER_TEST_POSTGRES_URL='postgres://postgres@127.0.0.1:5432/owner_test?sslmode=disable'
 -e BUTLER_TEST_MEMORY_POSTGRES_URL='postgres://postgres@127.0.0.1:5432/owner_test?sslmode=disable'
 -e BUTLER_TEST_COMMUNICATION_POSTGRES_URL='postgres://postgres@127.0.0.1:5432/owner_test?sslmode=disable')
read -r before _ < /proc/uptime
if docker run "${args[@]}" "$go_image" go test -json -race -p 1 -count=1 -timeout 5m ./internal/actionauth ./apps/orchestrator/internal/api ./apps/orchestrator/internal/orchestrator ./apps/orchestrator/internal/app ./apps/orchestrator/internal/communication ./apps/orchestrator/internal/input -run '^TestOwnerOutcome|^TestUnknownBrowser|^TestBrowserOwnerOutcome|^TestStoppedAttempt|^TestStoppedLocalKernel|^TestDurableUnknownEffectAndKnownResultRecoveryIntegration$|^TestDurableApproval|^TestUncertainBrowserObservation' > "$proof/pg-race.jsonl" 2>&1;then code=0;else code=$?;fi
read -r after _ < /proc/uptime
awk -v a="$before" -v b="$after" -v code="$code" 'BEGIN{printf "{\"phase\":\"pg_race\",\"duration_ms\":%.0f,\"exit_code\":%d}\n",(b-a)*1000,code}' > "$proof/timings.jsonl"
if ((code));then grep -E '"Action":"fail"|"Output":".*[.]go:[0-9]+' "$proof/pg-race.jsonl" || tail -50 "$proof/pg-race.jsonl";exit "$code";fi
if grep -q '"Action":"skip"' "$proof/pg-race.jsonl";then grep '"Action":"skip"' "$proof/pg-race.jsonl";exit 1;fi
for required in TestOwnerOutcomeKnownAndUnknownRetainGrantAndEvidenceIntegration TestOwnerOutcomeRejectsForeignStaleApprovalAndUnsettledIntegration TestBrowserOwnerOutcomeSameRunRecoveryIntegration TestUnknownBrowserOwnerWaitNeverResumesOnItsOwn TestOwnerOutcomeHistoricalRequiresExpiredContextAndKeepsTerminalHistoryIntegration;do
 if ! grep -q "\"Action\":\"pass\".*\"Test\":\"$required\"" "$proof/pg-race.jsonl";then printf 'Required test absent: %s\n' "$required";exit 1;fi
done
docker run "${args[@]}" "$go_image" go vet ./internal/actionauth ./apps/orchestrator/internal/api ./apps/orchestrator/internal/orchestrator ./apps/orchestrator/internal/app
printf 'Owner outcome PG/race and vet PASS; proof=%s\n' "$proof"
