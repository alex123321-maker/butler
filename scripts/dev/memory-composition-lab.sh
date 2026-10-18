#!/usr/bin/env bash
# Controlled composition regression, not native answer-quality acceptance.
set -euo pipefail
base=${1:?isolated issue92 root}
label=${2:?fresh report label}
[[ "$base" =~ ^/srv/storage/butler/agents/issue92-[a-zA-Z0-9_-]+$ && "$label" =~ ^[a-zA-Z0-9_-]+$ ]] || exit 2
src=$(readlink -f "$base/current"); [[ "$src" == "$base/snapshots/"* ]] || exit 2
history=${BUTLER_COMPOSITION_TEMPORAL_V5_HISTORY:?immutable full v5 history fixture required}
history_hash=${BUTLER_COMPOSITION_TEMPORAL_V5_HISTORY_SHA256:?reviewed history sha256 required}
[[ "$history" == /srv/storage/butler/agents/issue88-*/reports/*/history && "$history_hash" =~ ^[a-f0-9]{64}$ ]] || exit 2
test "$(sha256sum "$history/history.json" | cut -d' ' -f1)" = "$history_hash"
proof=$base/reports/$label; test ! -e "$proof"; umask 077
mkdir -p "$proof/postgres" "$base/.cache/go-build" "$base/.cache/go-tmp"
find "$src" -type f -print0 | sort -z | xargs -0 sha256sum > "$proof/source.sha256"
find "$history" -maxdepth 1 -type f -print0 | sort -z | xargs -0 sha256sum > "$proof/history-fixture.sha256"
network=butler-issue92-$label-net; pg=butler-issue92-$label-pg
created_pg=false; created_net=false
cleanup(){
 $created_pg && docker logs "$pg" > "$proof/postgres.log" 2>&1 || true
 $created_pg && docker rm -f "$pg" >/dev/null 2>&1 || true
 $created_net && docker network rm "$network" >/dev/null 2>&1 || true
}
trap cleanup EXIT INT TERM
docker network create --internal --subnet 10.237.192.0/24 "$network" > "$proof/network-id"; created_net=true
docker run --pull never -d --name "$pg" --network "$network" --network-alias postgres --cpus 1 --memory 1g --pids-limit 128 \
 -v "$proof/postgres:/var/lib/postgresql/data" -e POSTGRES_HOST_AUTH_METHOD=trust -e POSTGRES_USER=butler -e POSTGRES_DB=memory_composition_test \
 pgvector/pgvector:pg17 -c shared_buffers=64MB -c max_connections=32 -c max_wal_size=128MB > "$proof/postgres-id"; created_pg=true
for i in $(seq 1 60); do docker exec "$pg" pg_isready -h 127.0.0.1 -U butler -d memory_composition_test >/dev/null 2>&1 && break; [[ "$i" != 60 ]] || exit 1; sleep 1; done
runner=(docker run --rm --pull never --network "$network" --cpus 1 --memory 2g --pids-limit 128 --security-opt no-new-privileges
 -v "$src:/src:ro" -v "$base/.cache:/cache" -v /srv/storage/butler/.cache/go-mod:/gomod:ro -v "$proof:/proof" -v "$history:/history:ro" -w /src
 -e GOCACHE=/cache/go-build -e GOTMPDIR=/cache/go-tmp -e GOMODCACHE=/gomod -e GOPROXY=off -e GOSUMDB=off -e GOTOOLCHAIN=local
 -e BUTLER_TEST_MEMORY_POSTGRES_URL='postgres://butler@postgres:5432/memory_composition_test?sslmode=disable'
 -e BUTLER_TEMPORAL_V5_HISTORY_TEST_ROOT=/history
 -e BUTLER_GUARDED_REQUEST_BROKER_BINARY=/cache/broker-composition.test -e BUTLER_MEMORY_READ_APP_BINARY=/cache/tool-broker-composition
 -e BUTLER_BROKER_E2E_BINARY=/cache/tool-broker-composition -e BUTLER_EXTENSION_E2E_BINARY=/cache/extension-composition.test -e BUTLER_EXPERIENCE_HTTP_BINARY=/cache/http-composition.test -e BUTLER_EXPERIENCE_RECIPIENT_BROKER=1
 golang:1.27.2-bookworm)
"${runner[@]}" bash -eu -c 'go test -c -o /cache/broker-composition.test ./apps/tool-broker/internal/broker; go build -o /cache/tool-broker-composition ./apps/tool-broker; go test -c -o /cache/extension-composition.test ./apps/tool-extension/internal/runtime; go test -c -o /cache/http-composition.test ./apps/tool-http/internal/runtime; sha256sum /cache/broker-composition.test /cache/tool-broker-composition /cache/extension-composition.test /cache/http-composition.test > /proof/binaries.sha256' > "$proof/build.log" 2>&1
"${runner[@]}" bash -eu -c 'go test -json -race -p 1 -count=1 -timeout 12m -run "^Test(ExecuteRetainsOwned|ExecuteParentCancellation|OwnedCancellation|PrepareRetainsEnabled|AdaptiveV5|AdaptiveCollector|AdaptiveNativeCapture|Captured|Reconciled|TrustedToolProtocol|TerminalObservation|SourceGuard|ModelTurnUsageOwns|OwningSourceVerification|OwningCompoundSource|OwningDefaultSource|IdleProviderCannot|RunSourceObserver|ModelPriorityAtDispatchAndDuringInference|ModelPriorityPauseDeadlineAndConsumerGate|Experience|Lesson|Observation|Skill|ReferencedAgentProposal|CandidateAcceptedPreference|MemoryRead|DeletionErasesPinnedTool|ToolPayloadMigration|AssemblerRetainsUntitled|Feature|Reporter|QueueFailure|RequestedUnavailable|DurableMemoryObservations|MemoryDoctor|FreshPublication|Temporal(V5|V6(PositiveCarry|Worker)|Bound|Run|Pin|Distinct|Overlapping|Read|Lifecycle|Fresh|Historical|Unknown|OneOff|EpisodeExpiry|Facts|Qualification|FactValue|BrokerRead|CollectorOwning)|ConsolidationScreen(HistoryRejects|ProspectiveDay)|Consolidation(V2(CurrentDay|LatestLeaf|RetainsNative|RejectsChanged)|V3(Worker|Closure|LatestTemporal|CurrentDay)|Prepared|Temporal|Retention|Concurrent|Owner|Retire|Changed|Lifecycle|Metadata|Deactivate|Ordinary|ReleaseGate)|ExactDuplicates|DifferenceCannot|Grouping|CanonicalGrouping|OwnerRetir|IndexRetir|AtomicIndex|OversizedScope|LateIndex|SlowLocalIndex|LocalIndexObserver|ProjectStructured)" ./apps/orchestrator/internal/orchestrator ./internal/memory/jobs ./internal/memory/experience ./internal/skills ./internal/memory/reader ./internal/memory/factcontrol ./apps/orchestrator/internal/prompt ./internal/memory/doctor ./internal/memory/catalog ./internal/memory/consolidation ./internal/memory/indexing ./internal/memory/semantic ./apps/orchestrator/internal/app ./apps/orchestrator/internal/api ./apps/tool-broker/internal/broker > /proof/tests.jsonl' > "$proof/runner.log" 2>&1
if grep -q '"Action":"skip"\|"Action":"fail"\|"Action":"build-fail"' "$proof/tests.jsonl"; then exit 1; fi
while IFS= read -r name; do
 [[ -z "$name" || "$name" == \#* ]] && continue
 jq -s -e --arg name "$name" 'any(.[];.Action=="pass" and .Test==$name) and all(.[];.Test!=$name or (.Action!="fail" and .Action!="skip"))' "$proof/tests.jsonl" >/dev/null
done < "$src/scripts/dev/memory-composition-required-tests.txt"
"${runner[@]}" go vet ./internal/memory/experience ./internal/memory/factcontrol ./internal/skills ./internal/memory/doctor ./internal/memory/catalog ./internal/memory/jobs ./internal/memory/consolidation ./internal/memory/indexing ./internal/memory/semantic ./apps/orchestrator/internal/app ./apps/orchestrator/internal/api ./apps/tool-broker/internal/broker > "$proof/vet.log" 2>&1
jq -s '{passed:map(select(.Action=="pass" and .Test!=null))|length,failed:map(select(.Action=="fail"))|length,skipped:map(select(.Action=="skip"))|length}' "$proof/tests.jsonl" > "$proof/summary.json"
sha256sum --check --status "$proof/history-fixture.sha256"
sha256sum "$proof/source.sha256" "$proof/history-fixture.sha256" "$proof/binaries.sha256" "$proof/tests.jsonl" "$proof/vet.log" > "$proof/evidence.sha256"
printf 'PASS\n' > "$proof/complete"
cat "$proof/summary.json"
