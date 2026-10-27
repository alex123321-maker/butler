#!/bin/sh
set -eu
root=/mnt/host/wsl/butler-effort76
out="$root/src115/.cache"
test -f "$out/issue76-literal-r1-parent-joined-v1.json"
test ! -e "$out/memory115-skill-successor-read-v1.log"
set +e
chroot "$root" /usr/bin/env PATH=/usr/local/go/bin:/usr/local/bin:/usr/bin:/bin GOMAXPROCS=2 BUTLER_TEST_MEMORY_POSTGRES_URL='postgres://postgres@127.0.0.1:55451/memory115_test?sslmode=disable' BUTLER_BROKER_E2E_BINARY=/private/memory115-prepared-20261026-v1/bin/tool-broker BUTLER_GUARDED_REQUEST_BROKER_BINARY=/private/memory115-prepared-20261026-v1/bin/broker-control.test BUTLER_EXTENSION_E2E_BINARY=/private/memory115-prepared-20261026-v1/bin/extension.test BUTLER_EXPERIENCE_HTTP_BINARY=/private/memory115-prepared-20261026-v1/bin/http.test BUTLER_EXPERIENCE_PROCESS_PROOF=/private/memory115-skill-successor-read-v1 BUTLER_EXPERIENCE_NATIVE_HISTORY=/src115/.cache/memory115-skill-successor-recipe-v1/recipe.json BUTLER_EXPERIENCE_NATIVE_HISTORY_SHA256=370e4c2ae13fb25ca4a4dbb9891464959df57a4b9bb7ea0ad0a4d25888ddabe4 BUTLER_ISSUE115_SKILL_SUCCESSOR_READ=fresh-scripted-latest232-v1 /bin/sh -c 'cd /src115/apps/orchestrator/internal/app; exec /src115/.cache/memory115-skill-successor-v1.test -test.v -test.count=1 -test.timeout=25m -test.run=^TestExperienceNativeSkillPreparationSuccessorReadIntegration$' >"$out/memory115-skill-successor-read-v1.log" 2>&1
status=$?
printf '{"exit_code":%s,"native_calls":0,"real_credentials":0,"profile_derived_from_latest232_recipe":true,"component_only":true}\n' "$status" >"$out/memory115-skill-successor-read-v1-exit.json"
exit "$status"
