#!/bin/sh
set -eu
cd /src20
mode=${1:?mode required}
if [ "$mode" = build ]; then
 mkdir -p .cache/behavior76
 test "$(go list -m -f '{{.Version}}' github.com/jackc/pgx/v5)" = v5.8.0
 go run scripts/dev/behavior76-pgx-build.go /module-cache /src20/.cache/behavior76/pgx-observer-build-v1
 export GOFLAGS='-buildvcs=false -modfile=/src20/.cache/behavior76/pgx-observer-build-v1/research.mod -mod=readonly'
 exec go test -race -c -o .cache/behavior76/collector.test ./apps/orchestrator/internal/app
fi
export BUTLER_BEHAVIOR76_NATIVE_ENABLE=disabled
export BUTLER_BEHAVIOR76_PGX_OBSERVER_CONTRACT=callback-paired-public-only
export BUTLER_BEHAVIOR76_CONTROL_PREFLIGHT=0
case "$mode" in
 safety) selected='^TestBehavior76(GlobalStop.*|UnknownEffectStopsBeforeNextAdmissionIntegration|LocalUnknown.*)$' ;;
 resources) selected='^TestBehavior76LocalUnknownResourceProfileIntegration$' ;;
 public) selected='^TestBehavior76(DayWindowAdmission|AuthorityStream(FinalSealFailure|BeyondLegacyCapacity|WriteFailureAndLateEventStop|FixtureIntegration)|AuthorityTraceCancelledRollbackIntegration|StimulusBoundary|InstructionIntegrity|Queued.*Integration|CoreBroker.*Integration|EvidenceExportIntegration|ContinuationBoundary|LifecyclePreparationIntegration|PreparedTasksOnceIntegration|FreshHistoryBoundaryIntegration|GlobalStop.*|UnknownEffectStopsBeforeNextAdmissionIntegration|LocalUnknown.*)$' ;;
 evidence) selected='^TestBehavior76EvidenceExportIntegration$' ;;
 queued) selected='^TestBehavior76Queued(TaskMandates|FailedOwner)Integration$' ;;
 composition) selected='^TestBehavior76CorpusCompositionPreflight$';export BUTLER_BEHAVIOR76_CONTROL_PREFLIGHT=composition-only ;;
 freshpreflight) selected='^TestBehavior76FreshCorpusSetupPreflight$';export BUTLER_BEHAVIOR76_CONTROL_PREFLIGHT=fresh-setup-only;export BUTLER_BEHAVIOR76_STIMULI=/src20/.cache/issue76-independent/hidden-fresh-v3/stimuli.jsonl ;;
 freshcomposition) selected='^TestBehavior76FreshCorpusCompositionPreflight$';export BUTLER_BEHAVIOR76_CONTROL_PREFLIGHT=fresh-composition-only;export BUTLER_BEHAVIOR76_STIMULI=/src20/.cache/issue76-independent/hidden-fresh-v3/stimuli.jsonl ;;
 localpreflight) selected='^TestBehavior76LocalCorpusSetupPreflight$';export BUTLER_BEHAVIOR76_CONTROL_PREFLIGHT=local-setup-only;export BUTLER_BEHAVIOR76_STIMULI=/src20/.cache/issue76-independent/hidden-local-v4-r2/stimuli.jsonl ;;
 localcomposition) selected='^TestBehavior76LocalCorpusCompositionPreflight$';export BUTLER_BEHAVIOR76_CONTROL_PREFLIGHT=local-composition-only;export BUTLER_BEHAVIOR76_STIMULI=/src20/.cache/issue76-independent/hidden-local-v4-r2/stimuli.jsonl ;;
 streampreflight) selected='^TestBehavior76StreamCorpusSetupPreflight$';export BUTLER_BEHAVIOR76_CONTROL_PREFLIGHT=stream-setup-only;export BUTLER_BEHAVIOR76_STIMULI=/src20/.cache/issue76-independent/hidden-stream-v5/stimuli.jsonl ;;
 streamcomposition) selected='^TestBehavior76StreamCorpusCompositionPreflight$';export BUTLER_BEHAVIOR76_CONTROL_PREFLIGHT=stream-composition-only;export BUTLER_BEHAVIOR76_STIMULI=/src20/.cache/issue76-independent/hidden-stream-v5/stimuli.jsonl ;;
 daypreflight) selected='^TestBehavior76DayCorpusSetupPreflight$';export BUTLER_BEHAVIOR76_CONTROL_PREFLIGHT=day-setup-only;export BUTLER_BEHAVIOR76_STIMULI=/src20/.cache/issue76-independent/hidden-day-v6/stimuli.jsonl ;;
 daycomposition) selected='^TestBehavior76DayCorpusCompositionPreflight$';export BUTLER_BEHAVIOR76_CONTROL_PREFLIGHT=day-composition-only;export BUTLER_BEHAVIOR76_STIMULI=/src20/.cache/issue76-independent/hidden-day-v6/stimuli.jsonl ;;
 streampublic) selected='^TestBehavior76AuthorityStream(FixtureIntegration|BeyondLegacyCapacity|WriteFailureAndLateEventStop)$';export BUTLER_BEHAVIOR76_STREAM_FIXTURE_OUTPUT=/src20/.cache/behavior76/stream-fixture-public-v1 ;;
 boundary) selected='^TestBehavior76(ContinuationBoundary|LifecyclePreparationIntegration|PreparedTasksOnceIntegration|FreshHistoryBoundaryIntegration)$' ;;
 offline) selected='^TestBehavior76(StimulusBoundary|InstructionIntegrity|QueuedTaskMandatesIntegration|CoreBrokerPrepareOfflineIntegration|CoreBrokerPrepareOnlyDeniesCommitIntegration|CoreBrokerCommitRecipientIntegration)$' ;;
 preflight) selected='^TestBehavior76CorpusSetupPreflight$';export BUTLER_BEHAVIOR76_CONTROL_PREFLIGHT=1 ;;
 native) selected='^TestBehavior76NativeFreshLocal$';export BUTLER_BEHAVIOR76_NATIVE_ENABLE=fresh-synthetic-local-only;export BUTLER_BEHAVIOR76_STIMULI=/src20/.cache/issue76-independent/hidden-fresh-v3/stimuli.jsonl ;;
 localnative) selected='^TestBehavior76NativeFixtureLocal$';export BUTLER_BEHAVIOR76_NATIVE_ENABLE=fixture-local-synthetic-only;export BUTLER_BEHAVIOR76_STIMULI=/src20/.cache/issue76-independent/hidden-local-v4-r2/stimuli.jsonl ;;
 streamnative) selected='^TestBehavior76NativeFixtureStream$';export BUTLER_BEHAVIOR76_NATIVE_ENABLE=fixture-stream-synthetic-only;export BUTLER_BEHAVIOR76_STIMULI=/src20/.cache/issue76-independent/hidden-stream-v5/stimuli.jsonl ;;
 daynative) selected='^TestBehavior76NativeDayWindow$';export BUTLER_BEHAVIOR76_NATIVE_ENABLE=fixture-day-fit-synthetic-only;export BUTLER_BEHAVIOR76_STIMULI=/src20/.cache/issue76-independent/hidden-day-v6/stimuli.jsonl ;;
 *) echo 'Unknown bounded local mode' >&2;exit 2 ;;
esac
tmp=$(mktemp -d /tmp/behavior76-test.XXXXXX)
trap 'rm -f "$tmp/collector.test"; rmdir "$tmp"' EXIT
cp .cache/behavior76/collector.test "$tmp/collector.test"
cmp .cache/behavior76/collector.test "$tmp/collector.test"
"$tmp/collector.test" -test.v -test.run "$selected" -test.timeout 250m
