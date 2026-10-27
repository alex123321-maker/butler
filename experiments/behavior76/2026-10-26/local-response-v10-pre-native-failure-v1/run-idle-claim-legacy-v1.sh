#!/bin/sh
set -eu
root=/mnt/host/wsl/butler-effort76
out="$root/src20/.cache/issue76-independent/idle-claim-legacy-v1"
mkdir "$out"
test ! -e "$root/tmp/issue76-independent/idle-claim-legacy-v1.test"
mkdir -p "$root/tmp/issue76-independent"
cd "$root/src20"
sha256sum apps/orchestrator/internal/app/behavior76_idle_claim_diagnostic_test.go apps/orchestrator/internal/app/behavior76_collect_test.go apps/orchestrator/internal/app/behavior76_scope_test.go apps/orchestrator/internal/input/claim_admission.go apps/orchestrator/internal/input/service.go .cache/behavior76/collector.test >"$out/source-before-sha256.txt"
chroot "$root" /usr/bin/env PATH=/usr/local/go/bin:/usr/local/bin:/usr/bin:/bin GOROOT=/usr/local/go GOMODCACHE=/module-cache GOCACHE=/go-cache GOFLAGS='-buildvcs=false -modfile=/src20/.cache/behavior76/pgx-observer-build-v1/research.mod -mod=readonly' GOTOOLCHAIN=local GOMAXPROCS=4 /bin/sh -c 'cd /src20; exec go test -race -c -o /tmp/issue76-independent/idle-claim-legacy-v1.test ./apps/orchestrator/internal/app' >"$out/build.log" 2>&1
sha256sum "$root/tmp/issue76-independent/idle-claim-legacy-v1.test" >"$out/binary-sha256.txt"
set +e
chroot "$root" /usr/bin/env PATH=/usr/local/go/bin:/usr/local/bin:/usr/bin:/bin GOROOT=/usr/local/go GOMODCACHE=/module-cache GOCACHE=/go-cache GOFLAGS='-buildvcs=false -modfile=/src20/.cache/behavior76/pgx-observer-build-v1/research.mod -mod=readonly' GOTOOLCHAIN=local GOMAXPROCS=4 BUTLER_TEST_MEMORY_POSTGRES_URL='postgres://postgres@127.0.0.1:55451/behavior76_test?sslmode=disable' BUTLER_BEHAVIOR76_BROKER_BINARY=/tmp/behavior76/tool-broker BUTLER_BEHAVIOR76_NATIVE_ENABLE=disabled BUTLER_BEHAVIOR76_PGX_OBSERVER_CONTRACT=callback-paired-public-only BUTLER_BEHAVIOR76_IDLE_CLAIM_DIAGNOSTIC=reproduce-cancelled-idle-claim-v1 /bin/sh -c 'cd /src20; exec /tmp/issue76-independent/idle-claim-legacy-v1.test -test.v -test.run "^TestBehavior76IdleClaimCancellationLegacyDiagnosticIntegration$" -test.count=1 -test.timeout=40s' >"$out/test.log" 2>&1
status=$?
sha256sum apps/orchestrator/internal/app/behavior76_idle_claim_diagnostic_test.go apps/orchestrator/internal/app/behavior76_collect_test.go apps/orchestrator/internal/app/behavior76_scope_test.go apps/orchestrator/internal/input/claim_admission.go apps/orchestrator/internal/input/service.go .cache/behavior76/collector.test >"$out/source-after-sha256.txt"
cmp "$out/source-before-sha256.txt" "$out/source-after-sha256.txt" || exit 90
printf '{"version":"issue76-idle-claim-legacy-diagnostic-v1","test_exit_code":%s,"zero_native_auth":true,"old_harness":true,"hidden_corpus_read":false,"acceptance_PASS_claimed":false}\n' "$status" >"$out/exit.json"
cat "$out/test.log"
exit "$status"
