#!/usr/bin/env bash
# Prospective separate High scope. Preparation uses only private PG/synthetic
# providers; unknown V5 review bytes deny execution before auth/network/markers.
set -euo pipefail
root=${1:?isolated root required}; phase=${2:-prepare}; release=${3:?exact clean release required}
protocol=${4:-v5}
[[ "$phase" =~ ^(prepare|execute)$ && "$release" =~ ^[a-f0-9]{40}$ ]] || exit 2
global_minutes=60
if [[ "$protocol" == v7 ]]; then
  [[ "$root" == /srv/storage/butler/agents/scenario-release-chain-v7b-high-20261013 ]] || exit 2
  chain_baseline=539; chain_cap=76; chain_env=BUTLER_SCENARIO_CHAIN_V7_HIGH
  gate_test=TestReleaseChainV7OperationalGate; window_id=issue33-high-release-chain-v7b-20261013-baseline539
  audit_package=./scripts/dev/release-chain-v6-final-audit
  offline_timeout=3m; global_minutes=120
  # Unchanged Core, transport and broad fault suites have independently retained
  # proofs. V7 checks only the new history, frozen timing policy and retry guards.
  expected_audit_passes=0
elif [[ "$protocol" == v6 ]]; then
  [[ "$root" == /srv/storage/butler/agents/scenario-release-chain-v6-high-20261013 ]] || exit 2
  chain_baseline=537; chain_cap=78; chain_env=BUTLER_SCENARIO_CHAIN_V6_HIGH
  gate_test=TestReleaseChainV6OperationalGate; window_id=issue33-high-release-chain-v6-20261013-baseline537
  audit_package=./scripts/dev/release-chain-v6-final-audit
  offline_timeout=8m
  # Independently reviewed exact manifest; runtime/source admission remains a
  # separate gate after zero-inference preparation and final composition checks.
  expected_audit_passes=231
else
  [[ "$protocol" == v5 && "$root" == /srv/storage/butler/agents/scenario-release-chain-v5-high-20261013 ]] || exit 2
  chain_baseline=535; chain_cap=80; chain_env=BUTLER_SCENARIO_CHAIN_V5_HIGH
  gate_test=TestReleaseChainV5OperationalGate; window_id=issue33-high-release-chain-v5-20261013-baseline535
  audit_package=./scripts/dev/release-chain-v5-final-audit; expected_audit_passes=67
  offline_timeout=3m
fi
source_dir=$(readlink -f "$root/current")
[[ "$source_dir" == "$root/snapshots/"* ]] || exit 2
umask 077
export DOCKER_CONFIG="$root/.cache/docker"
proof="$root/reports/chain-$protocol-high-$phase-$(date -u +%Y%m%dT%H%M%SZ)-$$"
mkdir -p "$proof/bin" "$proof/postgres" /srv/storage/butler/.cache/go-tmp
printf '%s\n' "$release" > "$proof/source-commit.txt"
manifest() { (cd "$source_dir"; [[ -z "$(find . ! -type f ! -type d -print -quit)" ]]; find . -type f -print0 | sort -z | xargs -0 sha256sum); }
manifest > "$proof/source.sha256"
printf 'Private prospective High-chain %s proof: %s\n' "$phase" "$proof"
go_image=$(docker image inspect golang:1.27.2-bookworm --format '{{.Id}}')
pg_image=$(docker image inspect pgvector/pgvector:pg17 --format '{{.Id}}')
runtime_image=$(docker image inspect alpine:3.21 --format '{{.Id}}')
lscpu --json --extended=CPU,CORE,SOCKET,NODE,ONLINE > "$proof/hardware-cpu.json"
cpu_hash=$(sha256sum "$proof/hardware-cpu.json" | cut -d' ' -f1)
printf '{"go_image":"%s","postgres_image":"%s","runtime_image":"%s","kernel":"%s","cpu_sha256":"%s","host_ram_kib":%s,"fixture_ram_bytes":1610612736,"fixture_cpus":2,"remote_model_ram":null,"cloud_weights_sha256":null}\n' \
  "$go_image" "$pg_image" "$runtime_image" "$(uname -r)" "$cpu_hash" "$(awk '/^MemTotal:/ {print $2}' /proc/meminfo)" > "$proof/hardware.json"
if [[ "$phase" == execute ]]; then
  [[ -f "$root/prepared-release" && "$(cat "$root/prepared-release")" == "$release" && "$(cat "$root/prepared-source")" == "$source_dir" ]]
  manifest | cmp -s "$root/prepared-source.sha256" -
  sha256sum -c "$root/prepared-runtime.sha256" >/dev/null
  prepared=$(cat "$root/prepared-proof")
  [[ "$prepared" == "$root/reports/chain-$protocol-high-prepare-"* ]]
  cmp "$proof/hardware.json" "$prepared/hardware.json"
  # Deliberately first: no provider import, TLS, backend network, persistent
  # native scope or old-window read/write can precede this frozen source gate.
  docker run --rm --runtime runc --pull never --network none --read-only --user "$(id -u):$(id -g)" --cap-drop ALL --security-opt no-new-privileges:true \
    --memory 512m --pids-limit 64 --volume "$source_dir:/src:ro" --volume "$prepared/bin:/bin-proof:ro" --workdir /src \
    --env "${chain_env}_GATE=1" "$go_image" \
    /bin-proof/app.test -test.v -test.timeout=30s -test.run="^$gate_test$" > "$proof/prerequisite-gate.txt" 2>&1
  window="/srv/storage/butler/native-acceptance-windows/$window_id"
  [[ ! -e "$window" && ! -L "$window" ]] || { printf 'Denied: sealed separate native window.\n' >&2; exit 2; }
  [[ "$(docker inspect --format '{{.State.Running}}' butler-lab-antigravity-proxy-1)" == true ]]
  docker run --rm --runtime runc --pull never --network butler-lab_backend --read-only --user 10001:10001 --cap-drop ALL --security-opt no-new-privileges:true --memory 64m --pids-limit 16 \
    --volume "$prepared/bin/tls-prerequisite:/tls:ro" "$go_image" /tls > "$proof/tls.json"
  import_parent=/srv/storage/butler/agents/antigravity-provider/profile/.gemini/antigravity-cli
  docker run --rm --network none --read-only --user 10001:10001 --cap-drop ALL --security-opt no-new-privileges:true --volume "$import_parent:/metadata:ro" alpine:3.21 sh -eu -c 'p=/metadata/butler-native-import.json; test -f "$p"; test ! -L "$p"; test "$(stat -c "%u:%a" "$p")" = 10001:600; size=$(stat -c %s "$p"); test "$size" -gt 0; test "$size" -le 65536'
  mkdir -p /srv/storage/butler/native-acceptance-windows
  mkdir "$window"
  printf '%s\n' "$release" > "$window/release-ref"
  printf '%s\n' "$source_dir" > "$window/source-ref"
  printf '%s\n' "$proof" > "$window/preflight-proof-ref"
  date -u -d "+$global_minutes minutes" +%Y-%m-%dT%H:%M:%SZ > "$window/deadline"
  export "${chain_env}_SEALED_ENTRY=1"
  exec bash "$source_dir/scripts/dev/scenario-release-chain-v5-process-lab.sh" "$root" "high_$protocol" "$release"
fi
name="butler-chain-$protocol-high-prepare-$(date -u +%Y%m%dT%H%M%SZ)-$$"; network="$name-net"; pg="$name-pg"
created_network=false; created_pg=false
cleanup() { $created_pg && docker rm -f "$pg" >/dev/null 2>&1 || true; $created_network && docker network rm "$network" >/dev/null 2>&1 || true; }
trap cleanup EXIT INT TERM
build=(docker run --rm --runtime runc --pull never --network none --user "$(id -u):$(id -g)" --memory 1536m --pids-limit 160 --cpus 2 --security-opt no-new-privileges:true
  --volume "$source_dir:/src:ro" --volume "$proof/bin:/bin-proof" --volume /srv/storage/butler/.cache:/cache --workdir /src
  --env GOPROXY=off --env GOSUMDB=off --env GOMAXPROCS=2 --env GOFLAGS=-p=2 --env GOCACHE=/cache/go-race --env GOMODCACHE=/cache/go-mod --env GOTMPDIR=/cache/go-tmp --env "CHAIN_AUDIT_PACKAGE=$audit_package" --env "CHAIN_PROTOCOL=$protocol")
"${build[@]}" "$go_image" bash -eu -c '
go test -c -race -o /bin-proof/app.test ./apps/orchestrator/internal/app
go build -o /bin-proof/test2json cmd/test2json
CGO_ENABLED=0 go build -trimpath -o /bin-proof/tool-broker ./apps/tool-broker
CGO_ENABLED=0 go build -trimpath -o /bin-proof/tool-converter ./apps/tool-converter
go test -c -o /bin-proof/extension.test ./apps/tool-extension/internal/runtime
go build -o /bin-proof/tls-prerequisite ./scripts/dev/delegation-final-tls
CGO_ENABLED=0 go build -trimpath -o /bin-proof/chain-final-audit "$CHAIN_AUDIT_PACKAGE"
if [[ "$CHAIN_PROTOCOL" != v7 ]]; then
 go test -race -json -count=1 "$CHAIN_AUDIT_PACKAGE" > /bin-proof/chain-audit-unit.jsonl
 go test -race -json -count=1 -run "^TestToolAuthorizationFailsClosed$" ./apps/orchestrator/internal/orchestrator > /bin-proof/core-batch-policy.jsonl
fi
go vet ./apps/orchestrator/internal/app ./apps/orchestrator/internal/orchestrator "$CHAIN_AUDIT_PACKAGE"
chmod 555 /bin-proof/*
' > "$proof/build-vet.log" 2>&1
docker network create --internal "$network" >/dev/null; created_network=true
docker run -d --runtime runc --name "$pg" --network "$network" --network-alias chain-prepare-postgres --memory 512m --pids-limit 64 --cpus 1 --security-opt no-new-privileges:true \
  --env POSTGRES_USER=fixture --env POSTGRES_PASSWORD=chain-prepare-fixture --env POSTGRES_DB=butler_chain_prepare_test \
  --volume "$proof/postgres:/var/lib/postgresql/data" \
  "$pg_image" -c shared_buffers=32MB -c max_connections=32 -c max_wal_size=64MB >/dev/null; created_pg=true
for attempt in $(seq 1 60); do docker exec "$pg" pg_isready -U fixture -d butler_chain_prepare_test >/dev/null 2>&1 && break; [[ "$attempt" != 60 ]] || exit 1; sleep 1; done
app_pattern='^Test(ReleaseChainV5(Window|AllFailedEntries|TerminalMetadata|MeasurementWriteFailure|WholeEntryDeadline|OperationalPrerequisite|Bridge|ProductEvidence|InitialWitness|ProspectiveHealthy|CancelledTimedOut|ContextDeadlineAndCancel|ContextFailureAfterCompleteFinal|ProviderObservationFacts|FailureOutcome|OriginalResult)|ConverterWorkerStructuralDiagnostics)'
if [[ "$protocol" == v6 ]]; then app_pattern='^Test(ReleaseChainV5(Window|AllFailedEntries|TerminalMetadata|MeasurementWriteFailure|WholeEntryDeadline|OperationalPrerequisite|Bridge|ProductEvidence|InitialWitness|ProspectiveHealthy|CancelledTimedOut|ContextDeadlineAndCancel|ContextFailureAfterCompleteFinal|ProviderObservationFacts|FailureOutcome|OriginalResult)|ReleaseChainV6(OperationalPrerequisiteClosed|ActualHistory|ActualBridge|HumanExpenseException|PrerequisiteTamper|ProviderStage|Window|Kernel|MeasuredRetry|HumanSafeTimeout|SafeRetry)|ConverterWorkerStructuralDiagnostics)'; fi
if [[ "$protocol" == v7 ]]; then app_pattern='^TestReleaseChainV7BootstrapTimeoutGuard$'; fi
docker run --rm --runtime runc --pull never --network "$network" --read-only --user "$(id -u):$(id -g)" --cap-drop ALL --security-opt no-new-privileges:true --memory 1536m --pids-limit 160 --cpus 2 \
  --tmpfs /tmp:rw,nosuid,nodev,size=64m --volume "$source_dir:/src:ro" --volume "$proof/bin:/bin-proof:ro" --workdir /src \
  --env 'BUTLER_TEST_MEMORY_POSTGRES_URL=postgres://fixture:chain-prepare-fixture@chain-prepare-postgres:5432/butler_chain_prepare_test?sslmode=disable' \
  --entrypoint /bin-proof/test2json "$go_image" -t -p github.com/butler/butler/apps/orchestrator/internal/app \
  /bin-proof/app.test -test.v=test2json "-test.timeout=$offline_timeout" "-test.run=$app_pattern" > "$proof/tests.jsonl"
! grep -Eq '"Action":"(skip|fail)"' "$proof/tests.jsonl"
if [[ "$protocol" != v7 ]]; then
! grep -Eq '"Action":"(skip|fail)"' "$proof/bin/core-batch-policy.jsonl"
! grep -Eq '"Action":"(skip|fail)"' "$proof/bin/chain-audit-unit.jsonl"
[[ "$expected_audit_passes" -gt 0 && "$(awk 'index($0,"\"Action\":\"pass\"") && index($0,"\"Test\":") { n++ } END { print n+0 }' "$proof/bin/chain-audit-unit.jsonl")" == "$expected_audit_passes" ]]
fi
if [[ "$protocol" == v6 ]]; then
  mapfile -t checker_required < "$source_dir/scripts/dev/release-chain-v6-final-audit/required-tests.txt"
  [[ "${#checker_required[@]}" == 231 ]]
  for test_name in "${checker_required[@]}"; do
    [[ "$test_name" =~ ^Test[A-Za-z0-9_/-]+$ ]]
    [[ "$(awk -v needle="\"Test\":\"$test_name\"" 'index($0,"\"Action\":\"pass\"") && index($0,needle) { n++ } END { print n+0 }' "$proof/bin/chain-audit-unit.jsonl")" == 1 ]]
  done
fi
if [[ "$protocol" != v7 ]]; then
 [[ "$(awk 'index($0,"\"Action\":\"pass\"") && index($0,"\"Test\":\"TestToolAuthorizationFailsClosed\"") { n++ } END { print n+0 }' "$proof/bin/core-batch-policy.jsonl")" == 1 ]]
fi
required=(TestReleaseChainV5OperationalPrerequisiteIsNotApproved TestReleaseChainV5WindowSharedCeilingRestartAndExpiryIntegration TestReleaseChainV5WindowChecksDeadlineAfterCounterWaitIntegration TestReleaseChainV5WindowSharesCounterAcrossPrivateTaskSchemasIntegration TestReleaseChainV5AllFailedEntriesSpentButSourceDenialZeroIntegration TestReleaseChainV5TerminalMetadataRetainedAndReleaseGatedIntegration TestConverterWorkerStructuralDiagnosticsExcludePrivatePayloadIntegration)
required+=(TestReleaseChainV5BridgeUnknownActualHistoryDeniesBeforeProvider TestReleaseChainV5ProductEvidenceRetainedBeforeCleanupIntegration)
required+=(TestReleaseChainV5MeasurementWriteFailureDeniesReleaseIntegration)
required+=(TestReleaseChainV5InitialWitnessDoesNotInventNativeOrAdapterProvenanceIntegration)
required+=(TestReleaseChainV5ProspectiveHealthyThenFaultsAndFiniteBudgets)
required+=(TestReleaseChainV5OriginalResultContentFreeProjection TestReleaseChainV5OriginalResultFileBounds TestReleaseChainV5OriginalResultActualStoredReceiptsIntegration)
for case_name in stored_known_receipts failed_check_preserved duplicate_check_uncertain owner_stop_effect_denied; do required+=("TestReleaseChainV5OriginalResultActualStoredReceiptsIntegration/$case_name"); done
required+=(TestReleaseChainV5BridgeActualPASSAndCarryHistory TestReleaseChainV5BridgeActualMetadataTamperDenied TestReleaseChainV5BridgeOriginalAuditBindingDenied TestReleaseChainV5BridgeNumericThresholdsDenied)
for case_name in baseline historical_total historical_unknown_total quality_start_hash history_file_hash carry_hash model_profile model_version generation_bound native_source native_hardware auditor_code_is_not_native_code auditor_binary_is_not_native_runtime private_report new_attempt_count old_known_dimension old_null_materialized old_artifact_replaced row_order row_removed new_total_unknown new_total_changed synthesis_cost_changed child_parent_removed child_parent_changed child_role_changed input_hash output_hash_removed native_outcome projection_threshold_boolean comparison_cost comparison_child_count correlation_quality_claim correlation_execution; do required+=("TestReleaseChainV5BridgeActualMetadataTamperDenied/$case_name"); done
for case_name in status originals diagnostic_rows correct_originals_not_invented auditor_inference premature_issue_ready diagnostics_hash different_nested_audit different_nested_correlation; do required+=("TestReleaseChainV5BridgeOriginalAuditBindingDenied/$case_name"); done
for case_name in overall_accuracy kind_accuracy missing_case missed_important false_important critical_schema critical_source critical_authority critical_tool critical_transport critical_commands p95_bound maximum_bound individual_attempt_bound known_total_cost unknown_child_cost synthesis_omitted; do required+=("TestReleaseChainV5BridgeNumericThresholdsDenied/$case_name"); done
required+=(TestReleaseChainV5CancelledTimedOutOriginalStateAndCostIntegration TestReleaseChainV5ContextDeadlineAndCancelAreDistinctIntegration TestReleaseChainV5ContextFailureAfterCompleteFinalAndNilStreamIntegration TestReleaseChainV5ProviderObservationFacts TestReleaseChainV5FailureOutcome)
for case_name in cancelled_known timed_out_known cancelled_after_final timed_out_after_final cancelled_true_zero timed_out_legacy_pair cancelled_missing_version cancelled_conflicting_usage timed_out_conflicting_version cancelled_provisional_version timed_out_provisional_usage; do required+=("TestReleaseChainV5CancelledTimedOutOriginalStateAndCostIntegration/$case_name"); done
for case_name in parent_deadline parent_cancel; do required+=("TestReleaseChainV5ContextDeadlineAndCancelAreDistinctIntegration/$case_name"); done
for case_name in stream_deadline_known_cost stream_cancel_known_cost nil_stream_unknown_cost; do required+=("TestReleaseChainV5ContextFailureAfterCompleteFinalAndNilStreamIntegration/$case_name"); done
for case_name in completed_final truncated_final conflicting_finish_reasons private_reason_not_retained all_original_terminal_kinds; do required+=("TestReleaseChainV5ProviderObservationFacts/$case_name"); done
for case_name in own_deadline own_cancel provider_error; do required+=("TestReleaseChainV5FailureOutcome/$case_name"); done
required+=(TestReleaseChainV5WholeEntryDeadlineIncludesProviderPreflightIntegration TestReleaseChainV5WholeEntryDeadlineIncludesProviderPreflightIntegration/global_deadline_earlier TestReleaseChainV5WholeEntryDeadlineIncludesProviderPreflightIntegration/entry_maximum120seconds)
for case_name in failed_terminal_known untrusted_diagnostic_type known_batch_cost_retained_for_ordinary_core empty_batch_denied batch_after_final_denied conflicting_complete_usage true_zero_triple missing_total_legacy_pair missing_metadata missing_version provisional_version missing_terminal_known duplicate_final_known truncated_final_known terminal_without_final_known generation_bound_known input_bound_known output_bound_known conflicting_finish_reasons_known completed_final foreign_metadata_never_supplies_cost_or_version own_complete_cost_survives_foreign_terminal; do required+=("TestReleaseChainV5TerminalMetadataRetainedAndReleaseGatedIntegration/$case_name"); done
if [[ "$protocol" == v6 ]]; then
  source "$source_dir/scripts/dev/scenario-release-chain-v6-required-tests.sh"
  required+=("${release_chain_v6_required[@]}")
  [[ "${#required[@]}" == 309 ]]
  [[ "$(awk 'index($0,"\"Action\":\"pass\"") && index($0,"\"Test\":") { n++ } END { print n+0 }' "$proof/tests.jsonl")" == 309 ]]
fi
if [[ "$protocol" == v7 ]]; then
  required=(TestReleaseChainV7BootstrapTimeoutGuard)
  [[ "$(awk 'index($0,"\"Action\":\"pass\"") && index($0,"\"Test\":") { n++ } END { print n+0 }' "$proof/tests.jsonl")" == 1 ]]
fi
for test_name in "${required[@]}"; do
  [[ "$(awk -v needle="\"Test\":\"$test_name\"" 'index($0,"\"Action\":\"pass\"") && index($0,needle) { n++ } END { print n+0 }' "$proof/tests.jsonl")" == 1 ]]
done
manifest | cmp -s "$proof/source.sha256" -
sha256sum "$proof/bin/"* > "$root/prepared-runtime.sha256"
cp "$proof/source.sha256" "$root/prepared-source.sha256"
printf '%s\n' "$source_dir" > "$root/prepared-source"
printf '%s\n' "$proof" > "$root/prepared-proof"
printf '%s\n' "$release" > "$root/prepared-release"
sha256sum "$proof/tests.jsonl" "$proof/source.sha256" > "$proof/evidence.sha256"
printf '{"status":"offline_preparation_passed","native_calls":0,"baseline":%s,"baseline_status":"actual_history_pending_fresh_chain_review","maximum_new_attempts_design":%s,"execution_enabled":false}\n' "$chain_baseline" "$chain_cap" > "$proof/summary.json"
printf 'PASS: offline prospective cap/current proof tests; native execution remains denied.\n'
