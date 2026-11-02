#!/usr/bin/env bash
# Rebuild the known running Core source with only the guarded-GET reader delta.
# No live service, production database, provider or channel is contacted here.
set -euo pipefail
root=${1:?owned browser evaluation root}
[[ "$root" =~ ^/srv/storage/butler/agents/server-browser-get-form-[0-9]{8}$ ]] || exit 2
base=47f9429cc3a712060eaf01d329e371552cd58bcf
patch_source=cb7c937eb03d195154a650e64203763a3184316c
archive_sha=cc9da7d6326bfa60a2aa0ee21f3fddb7fd03809e32c37f4792703a47bbc4b3e3
patch_sha=6cb3cff0a85b656ce273541c4e02026ca6beebca3d9f687c92371bd95195e835
bundle="$root/core-evidence-47f9429c-cb7c937e"
test ! -e "$bundle"
printf '%s  %s\n' "$archive_sha" "$root/base-47f9429c.tar.gz" "$patch_sha" "$root/guarded-get-evidence.patch" | sha256sum -c -
mkdir -p "$bundle/src" "$bundle/reports" "$bundle/bin" "$root/.cache/go-tmp"
tar -xzf "$root/base-47f9429c.tar.gz" -C "$bundle/src"
git -C "$bundle/src" apply --check "$root/guarded-get-evidence.patch"
git -C "$bundle/src" apply "$root/guarded-get-evidence.patch"
src="$bundle/src"
go_image=$(docker image inspect --format '{{.Id}}' golang:1.27.2-bookworm)
pg_image=$(docker image inspect --format '{{.Id}}' pgvector/pgvector:pg17)
run="$(date -u +%Y%m%dT%H%M%SZ)-$$"
pg="butler-core-evidence-pg-$run"
created=false
cleanup() {
 if $created; then
  docker logs "$pg" > "$bundle/reports/postgres.log" 2>&1 || true
  docker rm -f "$pg" >/dev/null
 fi
}
trap cleanup EXIT
docker run -d --name "$pg" --network none --memory 1g --cpus 1 --pids-limit 128 \
 --tmpfs /var/lib/postgresql/data:rw,size=768m -e POSTGRES_HOST_AUTH_METHOD=trust -e POSTGRES_DB=core_evidence_test "$pg_image" \
 -c max_connections=40 -c max_wal_size=64MB -c min_wal_size=32MB -c shared_buffers=32MB >/dev/null
created=true
for i in $(seq 1 30); do docker exec "$pg" pg_isready -U postgres -d core_evidence_test >/dev/null 2>&1 && break; sleep 1; done
docker exec "$pg" pg_isready -U postgres -d core_evidence_test
args=(--rm --user "$(id -u):$(id -g)" --memory 3g --cpus 2 --pids-limit 256 --security-opt no-new-privileges:true
 --network "container:$pg" -v "$src:/src:ro" -v "$root/.cache:/cache" -v "$bundle/bin:/out"
 -v /srv/storage/butler/.cache/go-mod:/modules:ro -w /src
 -e GOCACHE=/cache/go-build -e GOMODCACHE=/modules -e GOTMPDIR=/cache/go-tmp -e GOPROXY=off -e GOSUMDB=off
 -e BUTLER_TEST_COMMUNICATION_POSTGRES_URL='postgres://postgres@127.0.0.1:5432/core_evidence_test?sslmode=disable'
 -e BUTLER_TEST_ACTIONAUTH_POSTGRES_URL='postgres://postgres@127.0.0.1:5432/core_evidence_test?sslmode=disable'
 -e BUTLER_TEST_POSTGRES_URL='postgres://postgres@127.0.0.1:5432/core_evidence_test?sslmode=disable'
 -e BUTLER_TEST_MEMORY_POSTGRES_URL='postgres://postgres@127.0.0.1:5432/core_evidence_test?sslmode=disable')
docker run "${args[@]}" "$go_image" go test -c -race -o /out/broker.test ./apps/tool-broker/internal/broker
report="$bundle/reports/required.jsonl"
if ! docker run "${args[@]}" -e BUTLER_GUARDED_REQUEST_BROKER_BINARY=/out/broker.test "$go_image" \
 go test -race -p 1 -count=1 -json -timeout 8m ./internal/research ./internal/budget ./apps/orchestrator/internal/orchestrator ./apps/orchestrator/internal/app \
 -run '^TestResearch|^TestCurrentRequestScopeCoreBrokerIntegration$|^TestStoppedAttempt' > "$report"; then
 grep -E '"Action":"fail"|"Output":".*[.]go:[0-9]+' "$report" || tail -30 "$report"; exit 1
fi
if grep -q '"Action":"skip"' "$report"; then grep '"Action":"skip"' "$report"; exit 1; fi
grep -q '"Test":"TestCurrentRequestScopeCoreBrokerIntegration"' "$report"
grep -q '"Test":"TestResearchGuardedGETPublicationUsesCurrentObservation"' "$report"
docker run "${args[@]}" "$go_image" go vet ./internal/research ./apps/orchestrator/internal/orchestrator
docker run "${args[@]}" -e CGO_ENABLED=0 "$go_image" go build -o /out/orchestrator ./apps/orchestrator
docker run "${args[@]}" "$go_image" go version -m /out/orchestrator > "$bundle/reports/build-info.txt"
sha256sum "$bundle/bin/orchestrator" > "$bundle/reports/binary-sha256.txt"
jq -n --arg base "$base" --arg patch_source "$patch_source" --arg archive "$archive_sha" --arg patch "$patch_sha" \
 --arg go "$go_image" --arg pg "$pg_image" '{base_commit:$base,patch_source_commit:$patch_source,archive_sha256:$archive,patch_sha256:$patch,go_image:$go,postgres_image:$pg,production_files_changed:["internal/research/evidence.go"],new_migrations:0,model_calls:0,live_services_changed:0}' > "$bundle/reports/source-manifest.json"
printf 'Pinned Core + three-file patch: required PG/race tests (no skips), vet and build PASS.\n'
cat "$bundle/reports/binary-sha256.txt"
