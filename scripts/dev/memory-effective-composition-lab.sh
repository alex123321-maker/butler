#!/usr/bin/env bash
# Focused constructor/Doctor/static-prefix and held-publication controls only.
set -euo pipefail
base=${1:?isolated issue92 root}; label=${2:?fresh report label}
[[ "$base" =~ ^/srv/storage/butler/agents/issue92-[A-Za-z0-9_-]+$ && "$label" =~ ^[A-Za-z0-9_-]+$ ]] || exit 2
src=$(readlink -f "$base/current"); [[ "$src" == "$base/snapshots/"* ]] || exit 2
proof=$base/reports/$label; test ! -e "$proof"; umask 077
mkdir -p "$proof/postgres" "$base/.cache/go-build" "$base/.cache/go-tmp"
find "$src" -type f -print0 | sort -z | xargs -0 sha256sum > "$proof/source.sha256"
network=butler-issue92-$label-effective-net; pg=butler-issue92-$label-effective-pg
created_pg=false; created_net=false
cleanup(){
 if $created_pg; then docker logs "$pg" > "$proof/postgres.log" 2>&1 || true; docker rm -f "$pg" >/dev/null 2>&1 || true; created_pg=false; fi
 if $created_net; then docker network rm "$network" >/dev/null 2>&1 || true; created_net=false; fi
}
trap cleanup EXIT
trap 'exit 130' INT
trap 'exit 143' TERM
# The operator checks current Docker subnets and host routes before invocation;
# Docker additionally rejects IPAM overlap. Only this invocation's exact named
# resources are removed; retained reports/PG bytes are preserved.
docker network create --internal --subnet 10.237.214.0/28 "$network" > "$proof/network-id"; created_net=true
docker run --pull never -d --name "$pg" --network "$network" --network-alias postgres --cpus 1 --memory 1g --pids-limit 128 \
 -v "$proof/postgres:/var/lib/postgresql/data" -e POSTGRES_HOST_AUTH_METHOD=trust -e POSTGRES_USER=butler -e POSTGRES_DB=memory_effective_test \
 pgvector/pgvector:pg17 -c shared_buffers=64MB -c max_connections=32 -c max_wal_size=128MB > "$proof/postgres-id"; created_pg=true
for i in $(seq 1 60); do docker exec "$pg" pg_isready -h127.0.0.1 -U butler -d memory_effective_test >/dev/null 2>&1 && break; [[ "$i" != 60 ]] || exit 1; sleep 1; done
runner=(docker run --rm --pull never --network "$network" --cpus 1 --memory 2g --pids-limit 128 --security-opt no-new-privileges
 -v "$src:/src:ro" -v "$base/.cache:/cache" -v /srv/storage/butler/.cache/go-mod:/gomod:ro -v "$proof:/proof" -w /src
 -e GOCACHE=/cache/go-build -e GOTMPDIR=/cache/go-tmp -e GOMODCACHE=/gomod -e GOPROXY=off -e GOSUMDB=off -e GOTOOLCHAIN=local
 -e BUTLER_TEST_MEMORY_POSTGRES_URL='postgres://butler@postgres:5432/memory_effective_test?sslmode=disable'
 -e BUTLER_MEMORY_READ_APP_BINARY=/proof/tool-broker -e BUTLER_BROKER_E2E_BINARY=/proof/tool-broker
 -e BUTLER_EXTENSION_E2E_BINARY=/proof/extension.test -e BUTLER_EXPERIENCE_HTTP_BINARY=/proof/http.test -e BUTLER_EXPERIENCE_RECIPIENT_BROKER=1
 -e BUTLER_MEMORY_COHERENT_PILOT=1 -e BUTLER_MEMORY_COHERENT_PROOF=/proof/timeline
 golang:1.27.2-bookworm)
status=0
"${runner[@]}" bash -eu -c '
 go build -o /proof/tool-broker ./apps/tool-broker
 go test -c -o /proof/extension.test ./apps/tool-extension/internal/runtime
 go test -c -o /proof/http.test ./apps/tool-http/internal/runtime
 go test -json -race -p 1 -count=1 -timeout 8m -run "^Test(MemoryEffectiveConstructorAndDoctorMatrixIntegration|ExperienceDefaultOffRetainsOrdinarySourceGuardIntegration|MemoryDoctorUsesActualHandlerAndMeasuredProfile|TemporalQualificationRemainsSeparateFromProcessFlag|ConsolidationReleaseGateCannotBeEnabledByOperatorFlag|ExperienceProductionGateCannotBeBypassed|ExperienceCompoundSourceBound|CandidateProductionReleaseRequiresProfileAndBothFlags|CandidateProfileRequiresActualMeasuredHighAndMatchingBudget|CandidateNativeProfileUsesActualAdapterTimeout|SummaryConsumerRequiresBothExplicitGates|SummaryProfileRequiresQualifiedNativeCeilingAndBudget|MemoryCoherentPilotOrdinaryTimelineIntegration)$" ./apps/orchestrator/internal/app > /proof/config-tests.jsonl
 go test -json -race -p 1 -count=1 -timeout 8m -run "^TestExperienceCapturedPins(FinalPublicationIntegration|LeaseExpiryDuringFinalWaitIntegration)$/^(accepted|missing_pin|environment)$" ./apps/orchestrator/internal/app > /proof/held-tests.jsonl
 go test -json -race -p 1 -count=1 -timeout 3m -run "^TestFeature" ./internal/memory/doctor > /proof/doctor-tests.jsonl
 go vet ./apps/orchestrator/internal/app ./internal/memory/doctor > /proof/vet.log 2>&1
' > "$proof/runner.log" 2>&1 || status=$?
cleanup
docker run --rm --pull never --network none --cpus .1 --memory 64m --pids-limit 32 -v "$proof:/proof" golang:1.27.2-bookworm \
 bash -c 'find /proof -maxdepth 2 -type f -not -path "/proof/postgres/*" -exec chown "$1:$2" {} +' -- "$(id -u)" "$(id -g)"
find "$proof" -maxdepth 1 -type f ! -name evidence.sha256 ! -name complete -print0 | sort -z | xargs -0 sha256sum > "$proof/evidence.sha256"
[[ "$status" == 0 ]] || exit "$status"
for stream in config held doctor; do
 ! grep -q '"Action":"skip"\|"Action":"fail"\|"Action":"build-fail"' "$proof/$stream-tests.jsonl"
done
for name in MemoryEffectiveConstructorAndDoctorMatrixIntegration ExperienceDefaultOffRetainsOrdinarySourceGuardIntegration MemoryDoctorUsesActualHandlerAndMeasuredProfile TemporalQualificationRemainsSeparateFromProcessFlag ConsolidationReleaseGateCannotBeEnabledByOperatorFlag ExperienceProductionGateCannotBeBypassed ExperienceCompoundSourceBound CandidateProductionReleaseRequiresProfileAndBothFlags CandidateProfileRequiresActualMeasuredHighAndMatchingBudget CandidateNativeProfileUsesActualAdapterTimeout SummaryConsumerRequiresBothExplicitGates SummaryProfileRequiresQualifiedNativeCeilingAndBudget MemoryCoherentPilotOrdinaryTimelineIntegration; do
 jq -s -e --arg name "Test$name" 'any(.[];.Action=="pass" and .Test==$name)' "$proof/config-tests.jsonl" >/dev/null
done
for name in TestExperienceCapturedPinsFinalPublicationIntegration/accepted TestExperienceCapturedPinsFinalPublicationIntegration/missing_pin TestExperienceCapturedPinsFinalPublicationIntegration/environment TestExperienceCapturedPinsLeaseExpiryDuringFinalWaitIntegration; do
 jq -s -e --arg name "$name" 'any(.[];.Action=="pass" and .Test==$name)' "$proof/held-tests.jsonl" >/dev/null
done
jq -e '.controlled_only and .pass and (.steps|length)==15 and .steps[3].Evidence.adaptive_configured==false and .steps[3].Evidence.adaptive_plan_rows==0' "$proof/timeline/timeline.json" >/dev/null
printf '{"native_calls":0,"auth_imported":false,"separate_pg":true,"client_cpu":1,"client_memory_bytes":2147483648,"postgres_cpu":1,"postgres_memory_bytes":1073741824,"qualifications_changed":false,"scope":"focused_effective_configuration_and_static_coherent_prefix"}\n' > "$proof/environment.json"
cat "$proof/config-tests.jsonl" "$proof/held-tests.jsonl" "$proof/doctor-tests.jsonl" | jq -s '{passed:map(select(.Action=="pass" and .Test!=null))|length,failed:map(select(.Action=="fail"))|length,skipped:map(select(.Action=="skip"))|length}' > "$proof/summary.json"
sha256sum "$proof/environment.json" "$proof/summary.json" "$proof/timeline/timeline.json" >> "$proof/evidence.sha256"
printf 'PASS\n' > "$proof/complete"
cat "$proof/summary.json"
