#!/usr/bin/env bash
# Current kernel prompt + safe native batch rejection and owning usage. No auth.
set -euo pipefail
base=${1:?isolated agent root}
label=${2:?fresh controlled label}
[[ "$base" =~ ^/srv/storage/butler/agents/issue89-[a-zA-Z0-9_-]+$ && "$label" =~ ^[a-zA-Z0-9_-]+$ ]] || exit 2
src=$(readlink -f "$base/current")
[[ "$src" == "$base/snapshots/"* ]] || exit 2
proof=$base/reports/$label
test ! -e "$proof"
umask 077
mkdir -p "$proof/postgres" "$base/.cache/go-build" "$base/.cache/go-tmp"
name=butler-single-proposal-$label
network=$name-net
created_pg=false
created_network=false
cleanup(){
 $created_pg && docker rm -f "$name" >/dev/null 2>&1 || true
 $created_network && docker network rm "$network" >/dev/null 2>&1 || true
}
trap cleanup EXIT INT TERM
find "$src" -type f -print0 | sort -z | xargs -0 sha256sum > "$proof/source.sha256"
docker network create --internal --subnet 10.237.189.0/24 "$network" >/dev/null
created_network=true
docker run -d --pull never --name "$name" --network "$network" --network-alias control-postgres \
 --cpus 1 --memory 512m --pids-limit 64 --security-opt no-new-privileges \
 -e POSTGRES_USER=fixture -e POSTGRES_PASSWORD=single-proposal-fixture -e POSTGRES_DB=single_proposal_test \
 -v "$proof/postgres:/var/lib/postgresql/data" pgvector/pgvector:pg17 \
 -c shared_buffers=32MB -c max_connections=24 -c max_wal_size=64MB >/dev/null
created_pg=true
for attempt in $(seq 1 30); do
 docker exec "$name" pg_isready -U fixture -d single_proposal_test >/dev/null 2>&1 && break
 [[ "$attempt" != 30 ]] || exit 1
 sleep 1
done
status=0
docker run --rm --pull never --network "$network" --cpus 1 --memory 2g --pids-limit 128 \
 --security-opt no-new-privileges -v "$src:/src:ro" -v "$base/.cache:/cache" \
 -v /srv/storage/butler/.cache/go-mod:/gomod:ro -w /src \
 -e GOCACHE=/cache/go-build -e GOTMPDIR=/cache/go-tmp -e GOMODCACHE=/gomod -e GOPROXY=off -e GOSUMDB=off -e GOTOOLCHAIN=local \
 -e 'BUTLER_TEST_MEMORY_POSTGRES_URL=postgres://fixture:single-proposal-fixture@control-postgres:5432/single_proposal_test?sslmode=disable' \
 golang:1.27.2-bookworm bash -c 'go test -json -race -p 1 -count=1 -timeout 6m -run "^Test(Trusted|ExecuteIncludesToolSummaryAndStructuredToolDefinitions|ToolAuthorizationFailsClosed|ModelTurnUsageOwnsOnePG|UnsafeOrIncompleteResponsesNeverProposeTools|CompleteInvalidToolProposal|ToolTurnUsage)" ./apps/orchestrator/internal/prompt ./apps/orchestrator/internal/orchestrator ./internal/transport/antigravity && go vet ./apps/orchestrator/internal/prompt ./apps/orchestrator/internal/orchestrator ./internal/transport ./internal/transport/antigravity' \
 > "$proof/events.jsonl" 2> "$proof/compiler-vet.txt" || status=$?
[[ "$status" == 0 ]] || exit "$status"
jq -s -e 'all(.[];.Action!="fail" and .Action!="build-fail" and .Action!="skip") and ([.[]|select(.Action=="pass" and .Test!=null)|.Test] as $passed | ["TestTrustedToolProtocolSurvivesOperatorBaseAndUntrustedData","TestExecuteIncludesToolSummaryAndStructuredToolDefinitions","TestToolAuthorizationFailsClosed","TestModelTurnUsageOwnsOnePGObservationAcrossToolsFinalsAndFailuresIntegration","TestUnsafeOrIncompleteResponsesNeverProposeTools","TestCompleteInvalidToolProposalRetainsUsageWithoutGrantingExecution"] | all(.[]; . as $test | $passed|index($test)!=null))' "$proof/events.jsonl" >/dev/null
jq -s '{passed:map(select(.Action=="pass" and .Test!=null))|length,failed:map(select(.Action=="fail" or .Action=="build-fail"))|length,skipped:map(select(.Action=="skip"))|length,native_model_calls:0,auth_imports:0}' "$proof/events.jsonl" > "$proof/summary.json"
sha256sum "$proof/source.sha256" "$proof/events.jsonl" "$proof/compiler-vet.txt" > "$proof/evidence.sha256"
printf 'PASS\n' > "$proof/complete"
cat "$proof/summary.json"
