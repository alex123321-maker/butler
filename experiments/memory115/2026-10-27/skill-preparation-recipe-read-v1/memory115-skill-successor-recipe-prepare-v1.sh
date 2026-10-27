#!/bin/sh
set -eu
root=/mnt/host/wsl/butler-effort76
out="$root/src115/.cache"
test -f "$out/issue76-literal-r1-parent-joined-v1.json"
test ! -e "$out/memory115-skill-successor-recipe-v1/recipe.json"
test ! -e "$out/memory115-skill-successor-recipe-prepare-v1.log"
set +e
chroot "$root" /usr/bin/env PATH=/usr/local/go/bin:/usr/local/bin:/usr/bin:/bin GOMAXPROCS=2 BUTLER_ISSUE115_SKILL_SUCCESSOR=prepare-latest232-unapplied-v1 BUTLER_ISSUE115_SKILL_SUCCESSOR_PATH=/src115/.cache/memory115-skill-successor-recipe-v1/recipe.json /bin/sh -c 'cd /src115/apps/orchestrator/internal/app; exec /src115/.cache/memory115-skill-successor-v1.test -test.v -test.timeout=5m -test.run=^TestLocal115PrepareSkillPreparationDayRecipe$' >"$out/memory115-skill-successor-recipe-prepare-v1.log" 2>&1
status=$?
printf '{"exit_code":%s,"storage_auth_native_construction":0,"recipe_only":true}\n' "$status" >"$out/memory115-skill-successor-recipe-prepare-v1-exit.json"
exit "$status"
