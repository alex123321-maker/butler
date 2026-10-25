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
 composition) selected='^TestBehavior76CorpusCompositionPreflight$';export BUTLER_BEHAVIOR76_CONTROL_PREFLIGHT=composition-only ;;
 boundary) selected='^TestBehavior76(ContinuationBoundary|LifecyclePreparationIntegration|PreparedTasksOnceIntegration)$' ;;
 offline) selected='^TestBehavior76(StimulusBoundary|InstructionIntegrity|QueuedTaskMandatesIntegration|CoreBrokerPrepareOfflineIntegration|CoreBrokerPrepareOnlyDeniesCommitIntegration|CoreBrokerCommitRecipientIntegration)$' ;;
 preflight) selected='^TestBehavior76CorpusSetupPreflight$';export BUTLER_BEHAVIOR76_CONTROL_PREFLIGHT=1 ;;
 native) selected='^TestBehavior76NativeLocal$';export BUTLER_BEHAVIOR76_NATIVE_ENABLE=synthetic-local-only ;;
 *) echo 'Unknown bounded local mode' >&2;exit 2 ;;
esac
tmp=$(mktemp -d /tmp/behavior76-test.XXXXXX)
trap 'rm -f "$tmp/collector.test"; rmdir "$tmp"' EXIT
cp .cache/behavior76/collector.test "$tmp/collector.test"
cmp .cache/behavior76/collector.test "$tmp/collector.test"
"$tmp/collector.test" -test.v -test.run "$selected" -test.timeout 250m
