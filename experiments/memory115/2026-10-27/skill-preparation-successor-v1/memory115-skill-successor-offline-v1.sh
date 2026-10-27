#!/bin/sh
# Prepared only: invoke after the parent has joined the closed issue76 process.
set -eu
umask 077
test "${1-}" = --own-native-lane-free
root=/mnt/host/wsl/butler-effort76
out="$root/src115/.cache"
test -f "$out/issue76-literal-r1-parent-joined-v1.json"
test ! -e "$out/memory115-skill-successor-build-v1.log"
test ! -e "$out/memory115-skill-successor-v1.test"
chroot "$root" /usr/bin/env PATH=/usr/local/go/bin:/usr/local/bin:/usr/bin:/bin GOROOT=/usr/local/go GOMODCACHE=/module-cache GOCACHE=/go-cache GOFLAGS=-buildvcs=false GOTOOLCHAIN=local GOPROXY=off GOSUMDB=off GOMAXPROCS=2 /bin/sh -c 'cd /src115; exec go test -race -p 1 -c -o .cache/memory115-skill-successor-v1.test ./apps/orchestrator/internal/app' >"$out/memory115-skill-successor-build-v1.log" 2>&1
sha256sum "$out/memory115-skill-successor-v1.test" >"$out/memory115-skill-successor-elf-v1.txt"
set +e
chroot "$root" /usr/bin/env PATH=/usr/local/go/bin:/usr/local/bin:/usr/bin:/bin GOMAXPROCS=2 /bin/sh -c 'cd /src115/apps/orchestrator/internal/app; exec /src115/.cache/memory115-skill-successor-v1.test -test.v -test.count=1 -test.timeout=5m -test.run=^Test\(Local115SkillPreparationSuccessorPreservesLatestFailedPopulation\|Local115NativeNetworkProfileRejectsAmbientProxySubstitution\|Local115SuccessorPreservesOldDayCostsAndUnknown\|ExperienceNativePresealPinsSourceArtifactsHistoryAndDenominator\|ExperienceNativeWholeDeadlinePreservesForegroundAndUTC\|ExperienceNativeMechanicalInputsFailClosed\|Local115ClosedHandbackProfileBoundaries\|ClosedExperienceShortScopeRetainsOmissionsAndRejectsLateWork\|ClosedExperienceProductsPreserveUnknownAndRejectForgeries\|ClosedExperienceReferencesAndHandbackFailClosed\|ClosedExperienceFinitePresealRejectsExpandedAuthority\|ClosedExperienceDomainBindsInstantAndEveryIdentity\)$' >"$out/memory115-skill-successor-controls-v1.log" 2>&1
controls=$?
chroot "$root" /usr/bin/env PATH=/usr/local/go/bin:/usr/local/bin:/usr/bin:/bin GOMAXPROCS=2 BUTLER_ISSUE115_CLOSED_LOCAL_IMPORT=exact-failed-local-cohort-only BUTLER_ISSUE115_CLOSED_LOCAL_MANIFEST=/src115/.cache/memory115-closed-local-wrapper-v1/closed-leaf.json BUTLER_ISSUE115_CLOSED_LOCAL_SHA256=1a8754c08fc724a1a3c8c3c657bc390aa7060bb446748de959eb461248656911 /bin/sh -c 'cd /src115/apps/orchestrator/internal/app; exec /src115/.cache/memory115-skill-successor-v1.test -test.v -test.count=1 -test.timeout=5m -test.run=^TestLocal115ClosedNativeHistoryImport$' >"$out/memory115-skill-successor-latest232-v1.log" 2>&1
latest=$?
chroot "$root" /usr/bin/env PATH=/usr/local/go/bin:/usr/local/bin:/usr/bin:/bin GOMAXPROCS=2 BUTLER_ISSUE115_LOCAL_IMPORT=copied-exact-evidence-only BUTLER_COHERENT_NATIVE_HISTORY=/private115/closed90/closed-leaf-metadata.json BUTLER_COHERENT_NATIVE_HISTORY_SHA256=8d9329e13da9e51e620c5fa66d6625ae11c938006b18369d2e09f068f1517b3f BUTLER_ISSUE115_LOCAL_IMPORT_RECEIPT=/src115/.cache/memory115-skill-successor-old213-receipt-v1.json /bin/sh -c 'cd /src115/apps/orchestrator/internal/app; exec /src115/.cache/memory115-skill-successor-v1.test -test.v -test.count=1 -test.timeout=5m -test.run=^TestLocal115ClosedHistoryImport$' >"$out/memory115-skill-successor-old213-v1.log" 2>&1
legacy=$?
chroot "$root" /usr/bin/env PATH=/usr/local/go/bin:/usr/local/bin:/usr/bin:/bin GOROOT=/usr/local/go GOMODCACHE=/module-cache GOCACHE=/go-cache GOFLAGS=-buildvcs=false GOTOOLCHAIN=local GOPROXY=off GOSUMDB=off GOMAXPROCS=2 /bin/sh -c 'cd /src115; exec go vet -p 1 ./apps/orchestrator/internal/app' >"$out/memory115-skill-successor-vet-v1.log" 2>&1
vet=$?
printf '{"controls_exit":%s,"latest232_exit":%s,"old213_exit":%s,"vet_exit":%s,"PG_auth_native":0}\n' "$controls" "$latest" "$legacy" "$vet" >"$out/memory115-skill-successor-offline-v1-exit.json"
test "$controls" -eq 0 && test "$latest" -eq 0 && test "$legacy" -eq 0 && test "$vet" -eq 0
