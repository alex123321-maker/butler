#!/bin/sh
set -eu
cd /src20
mode=${1:?mode required}
if [ "$mode" = build ]; then
 mkdir -p .cache/behavior76
 exec go test -race -c -o .cache/behavior76/collector.test ./apps/orchestrator/internal/app
fi
export BUTLER_BEHAVIOR76_NATIVE_ENABLE=disabled
export BUTLER_BEHAVIOR76_CONTROL_PREFLIGHT=0
case "$mode" in
 safety) selected='^TestBehavior76(GlobalStop.*|UnknownEffectStopsBeforeNextAdmissionIntegration|LocalUnknown.*)$' ;;
 resources) selected='^TestBehavior76LocalUnknownResourceProfileIntegration$' ;;
 public) selected='^TestBehavior76(StimulusBoundary|InstructionIntegrity|Queued.*Integration|CoreBroker.*Integration|EvidenceExportIntegration|ContinuationBoundary|LifecyclePreparationIntegration|PreparedTasksOnceIntegration|FreshHistoryBoundaryIntegration|GlobalStop.*|UnknownEffectStopsBeforeNextAdmissionIntegration|LocalUnknown.*)$' ;;
 evidence) selected='^TestBehavior76EvidenceExportIntegration$' ;;
 queued) selected='^TestBehavior76Queued(TaskMandates|FailedOwner)Integration$' ;;
 composition) selected='^TestBehavior76CorpusCompositionPreflight$';export BUTLER_BEHAVIOR76_CONTROL_PREFLIGHT=composition-only ;;
 freshpreflight) selected='^TestBehavior76FreshCorpusSetupPreflight$';export BUTLER_BEHAVIOR76_CONTROL_PREFLIGHT=fresh-setup-only;export BUTLER_BEHAVIOR76_STIMULI=/src20/.cache/issue76-independent/hidden-fresh-v3/stimuli.jsonl ;;
 freshcomposition) selected='^TestBehavior76FreshCorpusCompositionPreflight$';export BUTLER_BEHAVIOR76_CONTROL_PREFLIGHT=fresh-composition-only;export BUTLER_BEHAVIOR76_STIMULI=/src20/.cache/issue76-independent/hidden-fresh-v3/stimuli.jsonl ;;
 localpreflight) selected='^TestBehavior76LocalCorpusSetupPreflight$';export BUTLER_BEHAVIOR76_CONTROL_PREFLIGHT=local-setup-only;export BUTLER_BEHAVIOR76_STIMULI=/src20/.cache/issue76-independent/hidden-local-v4-r2/stimuli.jsonl ;;
 localcomposition) selected='^TestBehavior76LocalCorpusCompositionPreflight$';export BUTLER_BEHAVIOR76_CONTROL_PREFLIGHT=local-composition-only;export BUTLER_BEHAVIOR76_STIMULI=/src20/.cache/issue76-independent/hidden-local-v4-r2/stimuli.jsonl ;;
 boundary) selected='^TestBehavior76(ContinuationBoundary|LifecyclePreparationIntegration|PreparedTasksOnceIntegration|FreshHistoryBoundaryIntegration)$' ;;
 offline) selected='^TestBehavior76(StimulusBoundary|InstructionIntegrity|QueuedTaskMandatesIntegration|CoreBrokerPrepareOfflineIntegration|CoreBrokerPrepareOnlyDeniesCommitIntegration|CoreBrokerCommitRecipientIntegration)$' ;;
 preflight) selected='^TestBehavior76CorpusSetupPreflight$';export BUTLER_BEHAVIOR76_CONTROL_PREFLIGHT=1 ;;
 native) selected='^TestBehavior76NativeFreshLocal$';export BUTLER_BEHAVIOR76_NATIVE_ENABLE=fresh-synthetic-local-only;export BUTLER_BEHAVIOR76_STIMULI=/src20/.cache/issue76-independent/hidden-fresh-v3/stimuli.jsonl ;;
 localnative) selected='^TestBehavior76NativeFixtureLocal$';export BUTLER_BEHAVIOR76_NATIVE_ENABLE=fixture-local-synthetic-only;export BUTLER_BEHAVIOR76_STIMULI=/src20/.cache/issue76-independent/hidden-local-v4-r2/stimuli.jsonl ;;
 *) echo 'Unknown bounded local mode' >&2;exit 2 ;;
esac
tmp=$(mktemp -d /tmp/behavior76-test.XXXXXX)
trap 'rm -f "$tmp/collector.test"; rmdir "$tmp"' EXIT
cp .cache/behavior76/collector.test "$tmp/collector.test"
cmp .cache/behavior76/collector.test "$tmp/collector.test"
"$tmp/collector.test" -test.v -test.run "$selected" -test.timeout 250m
