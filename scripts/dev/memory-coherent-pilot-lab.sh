#!/usr/bin/env bash
# One shared controlled timeline. No auth import, internet, or native inference.
set -euo pipefail
base=${1:?isolated agent root}
label=${2:?fresh proof label}
mode=${3:-basic}
case "$mode" in
 basic) test_name=TestMemoryCoherentPilotOrdinaryTimelineIntegration ;;
 withdrawal) test_name=TestMemoryCoherentLateWithdrawalAfterRestartIntegration ;;
 experience) test_name=TestMemoryCoherentVerifiedExperienceAfterRestartIntegration ;;
 *) exit 2 ;;
esac
[[ "$base" =~ ^/srv/storage/butler/agents/issue(89|92)-[a-zA-Z0-9_-]+$ && "$label" =~ ^[a-zA-Z0-9_-]+$ ]] || exit 2
src=$(readlink -f "$base/current")
[[ "$src" == "$base/snapshots/"* ]] || exit 2
proof=$base/reports/$label
test ! -e "$proof"
: "${BUTLER_TEST_MEMORY_POSTGRES_URL:?isolated *_test PostgreSQL DSN required}"
umask 077
mkdir -p "$proof" "$base/.cache/go-build" "$base/.cache/go-tmp"
find "$src" -type f -print0 | sort -z | xargs -0 sha256sum > "$proof/source.sha256"
printf '{"execution_host":"ssh_laba","compiler_image":"golang:1.27.2-bookworm_cached","modules":"server_cached_readonly","GOPROXY":"off","GOSUMDB":"off","GOTOOLCHAIN":"local","cpu_limit":1,"native_model_calls":0,"provider_auth_imported":false}\n' > "$proof/environment.json"
status=0
docker run --rm --pull never --network host --cpus 1 --memory 2g --pids-limit 128 \
 --security-opt no-new-privileges -v "$src:/src:ro" -v "$base/.cache:/cache" \
 -v /srv/storage/butler/.cache/go-mod:/gomod:ro -v "$proof:/proof" -w /src \
 -e GOCACHE=/cache/go-build -e GOTMPDIR=/cache/go-tmp -e GOMODCACHE=/gomod -e GOPROXY=off -e GOSUMDB=off -e GOTOOLCHAIN=local \
 -e BUTLER_TEST_MEMORY_POSTGRES_URL -e BUTLER_MEMORY_READ_APP_BINARY=/proof/tool-broker \
 -e BUTLER_MEMORY_COHERENT_PILOT=1 -e BUTLER_MEMORY_COHERENT_PROOF=/proof/timeline \
 -e BUTLER_COHERENT_TEST_NAME="$test_name" \
 -e BUTLER_COHERENT_MODE="$mode" -e BUTLER_BROKER_E2E_BINARY=/proof/tool-broker \
 -e BUTLER_EXTENSION_E2E_BINARY=/proof/extension.test -e BUTLER_EXPERIENCE_HTTP_BINARY=/proof/http.test -e BUTLER_EXPERIENCE_RECIPIENT_BROKER=1 \
 golang:1.27.2-bookworm bash -eu -c 'go build -o /proof/tool-broker ./apps/tool-broker; if [[ "$BUTLER_COHERENT_MODE" == experience ]]; then go test -c -o /proof/extension.test ./apps/tool-extension/internal/runtime; go test -c -o /proof/http.test ./apps/tool-http/internal/runtime; fi; go test -json -race -p 1 -count=1 -timeout 8m -run "^${BUTLER_COHERENT_TEST_NAME}$" ./apps/orchestrator/internal/app' \
 > "$proof/events.jsonl" 2>&1 || status=$?
docker run --rm --pull never --network none --cpus .1 --memory 64m --pids-limit 32 -v "$proof:/proof" golang:1.27.2-bookworm chown -R "$(id -u):$(id -g)" /proof
[[ "$status" == 0 ]] || exit "$status"
jq -s -e --arg name "$test_name" 'any(.[];.Action=="pass" and .Test==$name) and all(.[];.Action!="fail" and .Action!="build-fail" and .Action!="skip")' "$proof/events.jsonl" >/dev/null
jq -e '.controlled_only and .pass and (.steps|length)==15 and .steps[-1].Name=="owning_source_pool_unavailable_denies_before_release"' "$proof/timeline/timeline.json" >/dev/null
if [[ "$mode" == withdrawal ]]; then
 jq -e '.controlled_only and .pass and .native_calls==0 and (.steps|length)==16 and .known_usage==1 and .remaining_private_copies==0 and .effect_journal_unchanged' "$proof/timeline/timeline-withdrawn.json" >/dev/null
 sha256sum "$proof/timeline/timeline-withdrawn.json" > "$proof/withdrawal.sha256"
fi
if [[ "$mode" == experience ]]; then
 jq -e '.controlled_only and .pass and .native_calls==0 and (.steps|length)==17 and .lesson_source!=""' "$proof/timeline/timeline-experience.json" >/dev/null
 sha256sum "$proof/timeline/timeline-experience.json" "$proof/extension.test" "$proof/http.test" > "$proof/experience.sha256"
fi
sha256sum "$proof/source.sha256" "$proof/tool-broker" "$proof/events.jsonl" "$proof/environment.json" "$proof/timeline/timeline.json" > "$proof/evidence.sha256"
printf 'PASS\n' > "$proof/complete"
printf 'Controlled coherent timeline passed; native inference0, auth0.\n'
