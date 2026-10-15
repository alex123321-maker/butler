#!/usr/bin/env bash
# Issue77 consumer composition. Offline cached images/modules; synthetic PG only.
set -euo pipefail
base=${1:?isolated issue77 root required}
label=${2:?fresh evidence label required}
[[ "$base" =~ ^/srv/storage/butler/agents/issue77-[a-zA-Z0-9_-]+$ && "$label" =~ ^[a-zA-Z0-9_-]+$ ]] || exit 2
src=$(readlink -f "$base/current")
[[ "$src" == "$base/snapshots/"* ]] || exit 2
proof=$base/reports/$label
test ! -e "$proof"
umask 077
mkdir -p "$proof"
find "$src" -type f -print0 | sort -z | xargs -0 sha256sum > "$proof/source.sha256"
bash "$src/scripts/dev/memory-read-lab.sh" "$base" > "$proof/read-runner.log" 2>&1
cp "$base/reports/read-acceptance.jsonl" "$proof/read.jsonl"
cp "$base/reports/read-vet.log" "$proof/read-vet.log"
docker run --pull never --rm --network issue86-20261016 --cpus 2 --memory 3g \
 -v "$src:/src:ro" -v "$base/.cache:/cache" -v /srv/storage/butler/.cache/go-mod:/gomod:ro -v "$proof:/proof" -w /src \
 -e GOCACHE=/cache/go-build -e GOMODCACHE=/gomod -e GOTMPDIR=/cache/go-tmp -e GOPROXY=off -e GOSUMDB=off -e GOTOOLCHAIN=local \
 -e BUTLER_TEST_MEMORY_POSTGRES_URL='postgres://butler:synthetic-test@issue86-20261016-pg:5432/memory_read_test?sslmode=disable' \
 golang:1.27.2-bookworm bash -eu -c '
go test -json -race -p 1 -count=1 -timeout 10m ./internal/memory/readcontrol ./internal/memory/reader ./internal/memory/sanitize ./internal/memory/compaction ./internal/memory/candidates ./internal/transport/antigravity > /proof/packages.jsonl
go test -json -race -p 1 -count=1 -timeout 10m -run "TestCandidate|TestOrdinaryTaskContinuation|TestSummaryConsumerRequires" ./apps/orchestrator/internal/api ./apps/orchestrator/internal/app > /proof/composition.jsonl
go vet ./internal/memory/compaction ./internal/memory/candidates ./internal/memory/jobs ./internal/transport/antigravity ./apps/orchestrator/internal/api ./apps/orchestrator/internal/app
' > "$proof/runner.log" 2>&1
for name in TestMemoryReadDispatchedBrokerIntegration TestMemoryReadModeZeroDoesNotRequireApprovalIntegration TestMemoryReadLifecycleAttackCorpusIntegration TestEveryLifecycleAttackThroughAtomicSummaryPublicationIntegration TestOrdinaryTaskContinuationAcrossRepeatedCompactionIntegration TestCandidateHTTPWorkerOwnerDecisionCorrectionIntegration TestCandidateAcceptedPreferenceScopedOrdinaryContextIntegration TestCandidateLifecyclePoisonRemainsDataAndDeletionSelectiveIntegration;do
 jq -s -e --arg name "$name" 'any(.[];.Action=="pass" and .Test==$name) and all(.[];.Test!=$name or (.Action!="fail" and .Action!="skip"))' "$proof/"*.jsonl >/dev/null
done
jq -s '{passed:map(select(.Action=="pass" and .Test!=null))|length,failed:map(select(.Action=="fail"))|length,skipped:map(select(.Action=="skip")|{package:.Package,test:.Test})}' "$proof/"*.jsonl > "$proof/summary.json"
jq -e '.failed==0' "$proof/summary.json" >/dev/null
sha256sum "$proof/source.sha256" "$proof/"*.jsonl > "$proof/evidence.sha256"
printf 'PASS\n' > "$proof/complete"
cat "$proof/summary.json"
