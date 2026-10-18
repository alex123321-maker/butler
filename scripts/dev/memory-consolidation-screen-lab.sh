#!/usr/bin/env bash
# One-use actual Flow/Broker paired collector proof; no provider credentials.
set -euo pipefail
base=${1:?isolated issue89 root required}
label=${2:?fresh proof label required}
[[ "$base" =~ ^/srv/storage/butler/agents/issue89-[a-zA-Z0-9_-]+$ && "$label" =~ ^[a-zA-Z0-9_-]+$ ]] || exit 2
src=$(readlink -f "$base/current")
[[ "$src" == "$base/snapshots/"* ]] || exit 2
proof=$base/reports/$label
test ! -e "$proof"
: "${BUTLER_TEST_MEMORY_POSTGRES_URL:?existing isolated *_test PostgreSQL DSN required}"
umask 077
mkdir -p "$proof" "$base/.cache/go-build" "$base/.cache/go-tmp"
find "$src" -type f -print0 | sort -z | xargs -0 sha256sum > "$proof/source.sha256"
test_status=0
docker run --rm --pull never --network host --cpus 1 --memory 2g --pids-limit 128 \
 --security-opt no-new-privileges -v "$src:/src:ro" -v "$base/.cache:/cache" \
 -v /srv/storage/butler/.cache/go-mod:/gomod:ro -v "$proof:/proof" -w /src \
 -e GOCACHE=/cache/go-build -e GOTMPDIR=/cache/go-tmp -e GOMODCACHE=/gomod -e GOPROXY=off -e GOSUMDB=off -e GOTOOLCHAIN=local \
 -e BUTLER_TEST_MEMORY_POSTGRES_URL -e BUTLER_MEMORY_READ_APP_BINARY=/proof/tool-broker \
 -e BUTLER_CONSOLIDATION_SCREEN_CONTROLLED=1 -e BUTLER_CONSOLIDATION_SCREEN_PROOF=/proof/screen \
 golang:1.27.2-bookworm bash -c 'go build -o /proof/tool-broker ./apps/tool-broker && go test -json -race -p 1 -count=1 -timeout 10m -run "^TestConsolidation(Screen|PairedCollector|OrdinaryTask)" ./apps/orchestrator/internal/app' \
 > "$proof/events.jsonl" 2>&1 || test_status=$?
# The Go container writes private0600/0700 artifacts. Transfer only this exact
# one-use proof directory to its host owner, including on a failed test, without
# making the private corpus/request/auth artifacts world-readable.
docker run --rm --pull never --network none --cpus .1 --memory 64m --pids-limit 32 \
 --security-opt no-new-privileges -v "$proof:/proof" golang:1.27.2-bookworm \
 chown -R "$(id -u):$(id -g)" /proof
[[ "$test_status" == 0 ]] || exit "$test_status"
jq -s -e 'all(.[];.Action!="fail" and .Action!="build-fail" and .Action!="skip") and ([.[]|select(.Action=="pass" and .Test!=null)|.Test] as $passed | ["TestConsolidationPairedCollectorActualFlowBrokerIntegration","TestConsolidationScreenDurableEntryBoundsIntegration","TestConsolidationOrdinaryTaskReadsActivatedDATAIntegration","TestConsolidationScreenCostsPreserveUnknownAndBackground","TestConsolidationScreenAllSlotsSealedBeforeStorage","TestConsolidationScreenRejectsHiddenContextBeforeProvider"] | all(.[]; . as $test | $passed|index($test)!=null))' "$proof/events.jsonl" >/dev/null
jq -e '.mechanics_pass and ([.slots[]|select(.State=="completed")]|length)==9 and .costs.full_unique_experiment.UnknownUsage==0 and .costs.full_unique_experiment.Entries<=99' "$proof/screen/screen.json" >/dev/null
jq -s '{passed:map(select(.Action=="pass" and .Test!=null))|length,failures:map(select(.Action=="fail" or .Action=="build-fail"))|length,skipped:map(select(.Action=="skip"))|length,native_model_calls:0}' "$proof/events.jsonl" > "$proof/summary.json"
sha256sum "$proof/source.sha256" "$proof/events.jsonl" "$proof/tool-broker" "$proof/screen/initial-schedule.json" "$proof/screen/budget-policy.json" "$proof/screen/provider-profile.json" "$proof/screen/screen.json" "$proof/screen/full-ledger.json" > "$proof/evidence.sha256"
printf 'PASS\n' > "$proof/complete"
cat "$proof/summary.json"
