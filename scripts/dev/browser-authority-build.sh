#!/usr/bin/env bash
# Minimal deployed-Core base + GET reader + exact authority-isolation delta.
set -euo pipefail
root=${1:?owned authority evaluation root}
revision=${2:?committed patch revision}
patch_sha=${3:?patch SHA256}
[[ "$root" =~ ^/srv/storage/butler/agents/browser-authority-isolation-[0-9]{8}$ && "$revision" =~ ^[a-f0-9]{40}$ && "$patch_sha" =~ ^[a-f0-9]{64}$ ]] || exit 2
bundle="$root/build-${revision:0:8}-${patch_sha:0:8}"
test ! -e "$bundle"
mkdir -p "$bundle/src" "$bundle/bin" "$bundle/reports"
timings="$bundle/reports/phase-timings.jsonl"
phase() {
 local label=$1; shift
 local started finished before after code
 started=$(date -u +%FT%TZ)
 before=$(awk '{printf "%.0f",$1*1000}' /proc/uptime)
 if "$@"; then code=0; else code=$?; fi
 after=$(awk '{printf "%.0f",$1*1000}' /proc/uptime)
 finished=$(date -u +%FT%TZ)
 jq -cn --arg phase "$label" --arg start "$started" --arg end "$finished" --argjson ms "$((after-before))" --argjson code "$code" \
  '{phase:$phase,started_at:$start,finished_at:$end,duration_ms:$ms,exit_code:$code,duration_clock:"Linux monotonic uptime (10ms resolution)"}' >> "$timings" || return 1
 return "$code"
}
verify() {
 printf '%s  %s\n' cc9da7d6326bfa60a2aa0ee21f3fddb7fd03809e32c37f4792703a47bbc4b3e3 "$root/base-47f9429c.tar.gz" \
  6cb3cff0a85b656ce273541c4e02026ca6beebca3d9f687c92371bd95195e835 "$root/guarded-get-evidence.patch" \
  "$patch_sha" "$root/authority-isolation.patch" | sha256sum -c -
}
reconstruct() {
 tar -xzf "$root/base-47f9429c.tar.gz" -C "$bundle/src" || return
 git -C "$bundle/src" apply --check "$root/guarded-get-evidence.patch" || return
 git -C "$bundle/src" apply "$root/guarded-get-evidence.patch" || return
 git -C "$bundle/src" apply --check "$root/authority-isolation.patch" || return
 git -C "$bundle/src" apply "$root/authority-isolation.patch" || return
}
phase source_verification verify
phase source_reconstruction reconstruct
src="$bundle/src"
go_image=$(docker image inspect --format '{{.Id}}' golang:1.27.2-bookworm)
pg_image=$(docker image inspect --format '{{.Id}}' pgvector/pgvector:pg17)
run="$(date -u +%Y%m%dT%H%M%SZ)-$$"
pg="butler-authority-build-pg-$run"
created=false
cleanup() {
 if $created; then docker logs "$pg" > "$bundle/reports/postgres.log" 2>&1 || true; docker rm -f "$pg" >/dev/null; fi
}
trap cleanup EXIT
start_pg() {
 docker run -d --name "$pg" --network none --memory 1g --cpus 1 --pids-limit 128 \
  --tmpfs /var/lib/postgresql/data:rw,size=768m -e POSTGRES_HOST_AUTH_METHOD=trust -e POSTGRES_DB=authority_test "$pg_image" \
  -c max_connections=40 -c max_wal_size=64MB -c min_wal_size=32MB -c shared_buffers=32MB >/dev/null
 created=true
 for i in $(seq 1 30); do docker exec "$pg" pg_isready -U postgres -d authority_test >/dev/null 2>&1 && break; sleep 1; done
 docker exec "$pg" pg_isready -U postgres -d authority_test
}
phase disposable_postgres_startup start_pg
args=(--rm --user "$(id -u):$(id -g)" --memory 3g --cpus 2 --pids-limit 256 --security-opt no-new-privileges:true
 --network "container:$pg" -v "$src:/src:ro" -v "$root/.cache:/cache" -v "$bundle/bin:/out"
 -v /srv/storage/butler/.cache/go-mod:/modules:ro -w /src -e GOCACHE=/cache/go-build -e GOMODCACHE=/modules -e GOTMPDIR=/cache/go-tmp -e GOPROXY=off -e GOSUMDB=off
 -e BUTLER_TEST_COMMUNICATION_POSTGRES_URL='postgres://postgres@127.0.0.1:5432/authority_test?sslmode=disable'
 -e BUTLER_TEST_POSTGRES_URL='postgres://postgres@127.0.0.1:5432/authority_test?sslmode=disable'
 -e BUTLER_TEST_MEMORY_POSTGRES_URL='postgres://postgres@127.0.0.1:5432/authority_test?sslmode=disable'
 -e BUTLER_TEST_ACTIONAUTH_POSTGRES_URL='postgres://postgres@127.0.0.1:5432/authority_test?sslmode=disable')
phase broker_test_compile docker run "${args[@]}" "$go_image" go test -c -race -o /out/broker.test ./apps/tool-broker/internal/broker
report="$bundle/reports/required.jsonl"
if ! phase required_pg_race docker run "${args[@]}" -e BUTLER_GUARDED_REQUEST_BROKER_BINARY=/out/broker.test "$go_image" \
 go test -race -p 1 -count=1 -json -timeout 8m ./internal/storage/postgres ./internal/research ./apps/orchestrator/internal/session ./apps/orchestrator/internal/orchestrator ./apps/orchestrator/internal/app \
 -run '^TestCommunicationAuthority|^TestResearch|^TestAuthorityRetry|^TestDurableLeaseFence|^TestExecute(RetainsOwned|StopsInFlight|RenewsLease)|^TestCurrentRequestScopeCoreBrokerIntegration$' > "$report"; then
 grep -E '"Action":"fail"|"Output":".*[.]go:[0-9]+' "$report" || tail -30 "$report"; exit 1
fi
if grep -q '"Action":"skip"' "$report"; then grep '"Action":"skip"' "$report"; exit 1; fi
for required in TestCommunicationAuthorityReserveWhileBusinessPoolSaturatedIntegration TestCommunicationAuthorityPoolAllocationIntegration TestResearchGuardedGETPublicationUsesCurrentObservation TestCurrentRequestScopeCoreBrokerIntegration; do grep -q "\"Test\":\"$required\"" "$report"; done
phase vet docker run "${args[@]}" "$go_image" go vet ./internal/storage/postgres ./internal/research ./apps/orchestrator/internal/session ./apps/orchestrator/internal/orchestrator ./apps/orchestrator/internal/app
phase core_compile docker run "${args[@]}" -e CGO_ENABLED=0 "$go_image" go build -o /out/orchestrator ./apps/orchestrator
sha=$(sha256sum "$bundle/bin/orchestrator" | cut -d ' ' -f 1)
jq -n --arg revision "$revision" --arg patch "$patch_sha" --arg binary "$sha" --arg go "$go_image" --arg pg "$pg_image" \
 '{base_commit:"47f9429cc3a712060eaf01d329e371552cd58bcf",get_evidence_patch_sha256:"6cb3cff0a85b656ce273541c4e02026ca6beebca3d9f687c92371bd95195e835",authority_patch_source_commit:$revision,authority_patch_sha256:$patch,executable_sha256:$binary,go_image:$go,postgres_image:$pg,new_migrations:0,real_provider_calls:0,live_services_changed:0}' > "$bundle/reports/source-manifest.json"
printf 'Minimal authority build: required PG/race, vet and compile PASS.\n'
cat "$timings"
printf 'sha256=%s\n' "$sha"
