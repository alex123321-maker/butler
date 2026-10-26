#!/bin/sh
set -eu
root=/mnt/host/wsl/butler-effort76
out="$root/src115/.cache"
test ! -e "$out/memory115-skill-read-diagnostic-v3.log"
chroot "$root" /usr/bin/env PATH=/usr/local/go/bin:/usr/local/bin:/usr/bin:/bin GOROOT=/usr/local/go GOMODCACHE=/module-cache GOCACHE=/go-cache GOFLAGS=-buildvcs=false GOTOOLCHAIN=local GOPROXY=off GOSUMDB=off GOMAXPROCS=2 /bin/sh -c 'cd /src115; exec go test -race -c -o .cache/memory115-skill-read-diagnostic-v3.test ./apps/orchestrator/internal/app' >"$out/memory115-skill-read-diagnostic-build-v3.log" 2>&1
sha256sum "$out/memory115-skill-read-diagnostic-v3.test" >"$out/memory115-skill-read-diagnostic-elf-v3.txt"
set +e
chroot "$root" /usr/bin/env PATH=/usr/local/go/bin:/usr/local/bin:/usr/bin:/bin GOROOT=/usr/local/go GOMODCACHE=/module-cache GOCACHE=/go-cache GOMAXPROCS=2 BUTLER_TEST_MEMORY_POSTGRES_URL='postgres://postgres@127.0.0.1:55451/memory115_test?sslmode=disable' BUTLER_BROKER_E2E_BINARY=/private/memory115-prepared-20261026-v1/bin/tool-broker BUTLER_EXTENSION_E2E_BINARY=/private/memory115-prepared-20261026-v1/bin/extension.test BUTLER_EXPERIENCE_HTTP_BINARY=/private/memory115-prepared-20261026-v1/bin/http.test BUTLER_EXPERIENCE_PROCESS_PROOF=/private/memory115-skill-read-diagnostic-v3 BUTLER_EXPERIENCE_SKILL_READ_DIAGNOSTIC=fresh-scripted-lessons-only /bin/sh -c 'cd /src115/apps/orchestrator/internal/app; exec /src115/.cache/memory115-skill-read-diagnostic-v3.test -test.v -test.count=1 -test.timeout=22m -test.run=^TestExperienceNativeSkillReadPermissionDiagnosticIntegration$' >"$out/memory115-skill-read-diagnostic-v3.log" 2>&1
status=$?
printf '{"exit_code":%s,"native_calls":0,"auth_access":0,"acceptance":false}\n' "$status" >"$out/memory115-skill-read-diagnostic-v3-exit.json"
exit "$status"
