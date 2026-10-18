#!/usr/bin/env bash
# One-use offline ABI/fault proof; no provider credentials or model inference.
set -euo pipefail
base=${1:?isolated agent root required}
label=${2:?fresh proof label required}
[[ "$base" =~ ^/srv/storage/butler/agents/issue(89|90)-[a-zA-Z0-9_-]+$ && "$label" =~ ^[a-zA-Z0-9_-]+$ ]] || exit 2
src=$(readlink -f "$base/current")
[[ "$src" == "$base/snapshots/"* ]] || exit 2
proof=$base/reports/$label
test ! -e "$proof"
: "${BUTLER_TEST_MEMORY_POSTGRES_URL:?existing isolated *_test PostgreSQL DSN required}"
umask 077
mkdir -p "$proof" "$base/.cache/go-build" "$base/.cache/go-tmp"
find "$src" -type f -print0 | sort -z | xargs -0 sha256sum > "$proof/source.sha256"
docker run --rm --pull never --network host --cpus 1 --memory 2g --pids-limit 128 \
 --security-opt no-new-privileges -v "$src:/src:ro" -v "$base/.cache:/cache" \
 -v /srv/storage/butler/.cache/go-mod:/gomod:ro -v "$proof:/proof" -w /src \
 -e GOCACHE=/cache/go-build -e GOTMPDIR=/cache/go-tmp -e GOMODCACHE=/gomod -e GOPROXY=off -e GOSUMDB=off -e GOTOOLCHAIN=local \
 -e BUTLER_TEST_MEMORY_POSTGRES_URL \
 -e BUTLER_BROKER_E2E_BINARY=/proof/tool-broker \
 -e BUTLER_EXTENSION_E2E_BINARY=/proof/tool-extension.test \
 -e BUTLER_EXPERIENCE_HTTP_BINARY=/proof/tool-http.test \
 -e BUTLER_EXPERIENCE_RECIPIENT_BROKER=1 \
 golang:1.27.2-bookworm bash -c 'go build -o /proof/tool-broker ./apps/tool-broker && go test -c -o /proof/tool-extension.test ./apps/tool-extension/internal/runtime && go test -c -o /proof/tool-http.test ./apps/tool-http/internal/runtime && go test -json -race -p 1 -count=1 -timeout 5m -run "^TestExperienceRecipient" ./apps/orchestrator/internal/app' \
 > "$proof/events.jsonl" 2>&1
jq -s -e 'all(.[];.Action!="fail" and .Action!="build-fail" and .Action!="skip") and ([.[]|select(.Action=="pass" and .Test!=null)|.Test] as $passed | ["TestExperienceRecipientExactCurrentArtifactAndKnownRejection","TestExperienceRecipientPersistedOutageAndChangedEnvironment","TestExperienceRecipientActualBrokerEffectProtocolIntegration/confirmed_artifact","TestExperienceRecipientActualBrokerEffectProtocolIntegration/rejected_value","TestExperienceRecipientActualBrokerEffectProtocolIntegration/persisted_unknown"] | all(.[]; . as $test | $passed|index($test)!=null))' "$proof/events.jsonl" >/dev/null
jq -s '{passed:map(select(.Action=="pass" and .Test!=null))|length,failures:map(select(.Action=="fail" or .Action=="build-fail"))|length,skipped:map(select(.Action=="skip"))|length,model_calls:0}' "$proof/events.jsonl" > "$proof/summary.json"
sha256sum "$proof/source.sha256" "$proof/events.jsonl" "$proof/tool-broker" "$proof/tool-extension.test" "$proof/tool-http.test" > "$proof/evidence.sha256"
printf 'PASS\n' > "$proof/complete"
cat "$proof/summary.json"
