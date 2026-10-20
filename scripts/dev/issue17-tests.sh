#!/usr/bin/env bash
# Local Docker Desktop acceptance: real Linux/PG/Broker/files/Chromium, no inference.
set -euo pipefail
cd /src
phase=${1:?prepare, core or browser required}
export GOTOOLCHAIN=local
export GOCACHE=/src/.cache/go-build-linux
export GOMODCACHE=/src/.cache/go-mod-linux
export GOMAXPROCS=4
if [[ "$phase" != browser ]]; then
  mkdir -p "$GOCACHE" "$GOMODCACHE"
fi
if [[ "$phase" == prepare ]]; then
  go mod download
  # A preceding root-owned Docker build can leave private Go cache entries.
  # These are this checkout's caches, never a profile or shared host directory.
  chown -R 10001:10001 "$GOCACHE" "$GOMODCACHE"
  exit 0
fi
export GOPROXY=off GOSUMDB=off
: "${BUTLER_TEST_POSTGRES_URL:?disposable PostgreSQL required}"
export BUTLER_TEST_MEMORY_POSTGRES_URL="$BUTLER_TEST_POSTGRES_URL"
export BUTLER_TEST_ACTIONAUTH_POSTGRES_URL="$BUTLER_TEST_POSTGRES_URL"

run_tests() {
  local label=$1; shift
  local timeout=5m
  # Recovery cases wait for each real finite lease to expire, without clock edits.
  [[ "$label" != workspaces ]] || timeout=10m
  echo "Running $label"
  if ! go test -json -race -p 1 -count=1 -timeout="$timeout" "$@" > "/reports/$label.jsonl" 2> "/reports/$label.stderr"; then
    grep -E '"Action":"(build-output|build-fail|fail)"|"OutputType":"error"' "/reports/$label.jsonl" || true
    tail -50 "/reports/$label.jsonl"
    cat "/reports/$label.stderr"
    return 1
  fi
  if grep -Eq '"Action":"fail"|"Action":"skip".*"Test":' "/reports/$label.jsonl"; then
    grep -E '"Action":"fail"|"Action":"skip".*"Test":' "/reports/$label.jsonl"
    return 1
  fi
  grep -q '"Action":"pass".*"Test":' "/reports/$label.jsonl"
}

if [[ "$phase" == core ]]; then
  run_tests resources ./internal/resources ./internal/resourceexec ./internal/budget
  run_tests memory-head -run '^Test(SourceDuplicateAndConflictingPreferencesIntegration|ConcurrentExplicitSupersessionIntegration|ExtractionConcurrentWorkerCandidatesNeedOwnerHeadCASIntegration)$' ./internal/memory/catalog
  run_tests child-budget -run '^Test(ChildAdmissionCannotResetRootBudgetIntegration|ControlStopPropagatesChildrenAndRetainsExternalObservations|ControlTwoChannelsCASAndOwnerProof|HTTPControlRequiresProofAndCurrentRevisionIntegration)$' ./apps/orchestrator/internal/task
  run_tests tab-control ./internal/singletabcontrol ./apps/orchestrator/internal/singletab ./apps/browser-bridge/...

  echo 'Building actual Broker and workspace subprocesses'
  go build -race -p 1 -trimpath -o /proof/tool-broker ./apps/tool-broker
  go test -c -race -p 1 -o /proof/broker-race.test ./apps/tool-broker/internal/broker
  go test -c -race -p 1 -o /proof/workspace.test ./apps/tool-workspace/internal/app
  export BUTLER_GUARDED_REQUEST_BROKER_BINARY=/proof/broker-race.test
  export BUTLER_MEMORY_CANDIDATE_APP_BINARY=/proof/tool-broker
  export BUTLER_WORKSPACE_E2E_BINARY=/proof/workspace.test

  # The actual Chromium test is executed separately, never counted as a skip/pass.
  broker_tests=$(go test -list '^Test(SingleTabResource|Workspace|GuardedTab|Resource)' ./apps/tool-broker/internal/broker | grep '^Test' | grep -Ev '^Test(SingleTabResourceActualChromiumNativeHostIntegration|ResourceBrokerActualChromiumPageCASIntegration)$' | paste -sd '|' -)
  [[ -n "$broker_tests" ]]
  run_tests broker -run "^($broker_tests)$" ./apps/tool-broker/internal/broker
  run_tests workspace-bootstrap -run '^Test(WorkspaceActualApplicationBootstrapIntegration|GuardedTabRuntimeBootstrap|GuardedTabOriginPolicyIsExplicit)$' ./apps/tool-broker/internal/app
  run_tests core-controls -run '^Test(MemoryCandidate|GuardedTabRequestDispatchedIntegration|WorkspaceOwningContextPinsCurrentInputIntegration)' ./apps/orchestrator/internal/app
  run_tests owner-controls -run '^Test(OwnerWorkspace|GuardedTab)' ./apps/orchestrator/internal/api
  run_tests core-suspension -run '^TestGuardedTabRequestCoreSuspendsBeforeGenericResult$' ./apps/orchestrator/internal/orchestrator
  # Check application composition before the longer natural-expiry recovery suite.
  run_tests workspaces ./internal/workspace ./internal/workspaceexec

  go vet -p 1 ./internal/resources ./internal/resourceexec ./internal/budget ./internal/workspace ./internal/workspaceexec ./internal/singletabcontrol ./apps/browser-bridge/... ./apps/tool-broker/internal/broker ./apps/tool-broker/internal/app ./apps/orchestrator/internal/app ./apps/orchestrator/internal/api > /reports/vet.log 2>&1
  # Static binaries run in the existing production Playwright base (older libc).
  CGO_ENABLED=0 go build -p 1 -trimpath -o /proof/guarded-native-host ./apps/browser-bridge/cmd/guarded
  CGO_ENABLED=0 go test -c -p 1 -o /proof/broker.test ./apps/tool-broker/internal/broker
  chmod 555 /proof/guarded-native-host /proof/broker.test
  sha256sum /proof/* > /reports/binaries.sha256
  echo 'Core acceptance passed; native inference calls: 0'
elif [[ "$phase" == browser ]]; then
  export BUTLER_SINGLE_TAB_E2E=1
  export BUTLER_SINGLE_TAB_HOST_BINARY=/proof/guarded-native-host
  export BUTLER_SINGLE_TAB_FIXTURE_SCRIPT=/src/apps/browser-bridge/scripts/guarded_chromium_fixture.mjs
  export BUTLER_SINGLE_TAB_EXTENSION=/src/web/extensions/chromium-butler
  export DISPLAY=:91
  Xvfb :91 -screen 0 1280x900x24 -nolisten tcp > /tmp/xvfb.log 2>&1 &
  trap 'kill "$!" 2>/dev/null || true' EXIT
  cd /src/apps/tool-broker/internal/broker
  /proof/broker.test -test.v -test.timeout=5m -test.run='^TestSingleTabResourceActualChromiumNativeHostIntegration$' > /reports/chromium.log 2>&1
  ! grep -Eq -- '--- (FAIL|SKIP):' /reports/chromium.log
  for scenario in valid manual_input manual_new_observation dom_mutation navigation foreign_origin foreign_host lost_ack_restart two_executors installed_native; do
    grep -Fq -- "--- PASS: TestSingleTabResourceActualChromiumNativeHostIntegration/$scenario (" /reports/chromium.log
  done
  echo 'Actual Chromium, extension, installed native host and Broker passed'
else
  echo 'Unknown phase' >&2
  exit 2
fi
