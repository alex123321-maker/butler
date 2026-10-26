#!/bin/sh
set -eu
root=/mnt/host/wsl/butler-effort76
out="$root/src115/.cache"
test ! -e "$out/memory115-closed-local-consumer-build-v2.log"
cd "$root/src115"
sha256sum apps/orchestrator/internal/app/memory115_closed_local_native_history_test.go apps/orchestrator/internal/app/memory_closed_experience_native_history_test.go apps/orchestrator/internal/app/memory_experience_native_entry_test.go >"$out/memory115-closed-local-source-before-v2.txt"
chroot "$root" /usr/bin/env PATH=/usr/local/go/bin:/usr/local/bin:/usr/bin:/bin GOROOT=/usr/local/go GOMODCACHE=/module-cache GOCACHE=/go-cache GOFLAGS=-buildvcs=false GOTOOLCHAIN=local GOPROXY=off GOSUMDB=off GOMAXPROCS=2 /bin/sh -c 'cd /src115; exec go test -race -c -o .cache/memory115-closed-local-consumer-v2.test ./apps/orchestrator/internal/app' >"$out/memory115-closed-local-consumer-build-v2.log" 2>&1
sha256sum apps/orchestrator/internal/app/memory115_closed_local_native_history_test.go apps/orchestrator/internal/app/memory_closed_experience_native_history_test.go apps/orchestrator/internal/app/memory_experience_native_entry_test.go >"$out/memory115-closed-local-source-after-v2.txt"
cmp "$out/memory115-closed-local-source-before-v2.txt" "$out/memory115-closed-local-source-after-v2.txt"
sha256sum "$out/memory115-closed-local-consumer-v2.test" >"$out/memory115-closed-local-consumer-elf-v2.txt"
set +e
chroot "$root" /usr/bin/env PATH=/usr/local/go/bin:/usr/local/bin:/usr/bin:/bin GOMAXPROCS=2 /bin/sh -c 'cd /src115/apps/orchestrator/internal/app; exec /src115/.cache/memory115-closed-local-consumer-v2.test -test.v -test.count=1 -test.timeout=3m -test.run=^Test\(Local115ClosedHandbackProfileBoundaries\|ClosedExperienceShortScopeRetainsOmissionsAndRejectsLateWork\|ClosedExperienceProductsPreserveUnknownAndRejectForgeries\|ClosedExperienceReferencesAndHandbackFailClosed\|ClosedExperienceFinitePresealRejectsExpandedAuthority\|ClosedExperienceDomainBindsInstantAndEveryIdentity\)$' >"$out/memory115-closed-local-controls-v2.log" 2>&1
controls=$?
chroot "$root" /usr/bin/env PATH=/usr/local/go/bin:/usr/local/bin:/usr/bin:/bin GOMAXPROCS=2 BUTLER_ISSUE115_CLOSED_LOCAL_IMPORT=exact-failed-local-cohort-only BUTLER_ISSUE115_CLOSED_LOCAL_MANIFEST=/src115/.cache/memory115-closed-local-wrapper-v1/closed-leaf.json BUTLER_ISSUE115_CLOSED_LOCAL_SHA256=1a8754c08fc724a1a3c8c3c657bc390aa7060bb446748de959eb461248656911 /bin/sh -c 'cd /src115/apps/orchestrator/internal/app; exec /src115/.cache/memory115-closed-local-consumer-v2.test -test.v -test.count=1 -test.timeout=5m -test.run=^TestLocal115ClosedNativeHistoryImport$' >"$out/memory115-closed-local-import-v2.log" 2>&1
local=$?
chroot "$root" /usr/bin/env PATH=/usr/local/go/bin:/usr/local/bin:/usr/bin:/bin GOMAXPROCS=2 BUTLER_ISSUE115_LOCAL_IMPORT=copied-exact-evidence-only BUTLER_COHERENT_NATIVE_HISTORY=/private115/closed90/closed-leaf-metadata.json BUTLER_COHERENT_NATIVE_HISTORY_SHA256=8d9329e13da9e51e620c5fa66d6625ae11c938006b18369d2e09f068f1517b3f BUTLER_ISSUE115_LOCAL_IMPORT_RECEIPT=/src115/.cache/memory115-closed-local-legacy-receipt-v2.json /bin/sh -c 'cd /src115/apps/orchestrator/internal/app; exec /src115/.cache/memory115-closed-local-consumer-v2.test -test.v -test.count=1 -test.timeout=5m -test.run=^TestLocal115ClosedHistoryImport$' >"$out/memory115-closed-local-legacy-import-v2.log" 2>&1
legacy=$?
chroot "$root" /usr/bin/env PATH=/usr/local/go/bin:/usr/local/bin:/usr/bin:/bin GOROOT=/usr/local/go GOMODCACHE=/module-cache GOCACHE=/go-cache GOFLAGS=-buildvcs=false GOTOOLCHAIN=local GOPROXY=off GOSUMDB=off GOMAXPROCS=2 /bin/sh -c 'cd /src115; exec go vet ./apps/orchestrator/internal/app' >"$out/memory115-closed-local-vet-v2.log" 2>&1
vet=$?
printf '{"controls_exit":%s,"local_import_exit":%s,"legacy_import_exit":%s,"vet_exit":%s,"PG_auth_native":0}\n' "$controls" "$local" "$legacy" "$vet" >"$out/memory115-closed-local-consumer-v2-exit.json"
test "$controls" -eq 0 && test "$local" -eq 0 && test "$legacy" -eq 0 && test "$vet" -eq 0
