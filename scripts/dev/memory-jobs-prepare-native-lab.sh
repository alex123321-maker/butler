#!/usr/bin/env bash
# Read-only import of the currently configured provider's encrypted credential
# row. Never print secret material, inspect private files or reuse old windows.
set -euo pipefail
base=${1:?isolated root}
[[ "$base" == /srv/storage/butler/agents/issue82-20261015 ]] || exit 2
umask 077
mkdir -p "$base/private"
docker inspect butler-lab-orchestrator-1 | jq -r '.[0].Config.Env[] | select(startswith("BUTLER_SETTINGS_ENCRYPTION_KEY="))' > "$base/private/native.env"
test "$(wc -l < "$base/private/native.env")" -eq 1
test ! -e "$base/private/owner.token"
openssl rand -hex 32 > "$base/private/owner.token"
openssl rand -hex 32 > "$base/private/action.token"
chmod 700 "$base/private"
chmod 600 "$base/private/"*
printf 'Native private configuration prepared; no inference authorized yet.\n'
