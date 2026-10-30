#!/usr/bin/env bash
set -euo pipefail
root=${1:?owned browser-owner test root}
[[ "$root" =~ ^/srv/storage/butler/agents/server-browser-owners-[0-9]{8}$ ]] || exit 2
src=$(readlink -f "$root/current")
[[ "$src" == "$root/snapshots/"* ]] || exit 2
proof="$root/reports/web-$(date -u +%Y%m%dT%H%M%SZ)"
mkdir -p "$proof" "$root/.cache/owner-web/node_modules"
browser_image=sha256:3a2e719f086e2d2a110a79f817f235518350aaf87ea47cb51a9a75158e32d9c6
docker run --rm --user "$(id -u):$(id -g)" --memory 3g --cpus 2 --pids-limit 256 --shm-size 256m \
 --security-opt no-new-privileges:true -v "$src:/src" -v "$root/.cache:/cache" \
 -v "$root/.cache/owner-web/node_modules:/src/web/node_modules" \
 -v /srv/storage/butler/.cache/ms-playwright:/browsers:ro -w /src/web \
 -e NPM_CONFIG_CACHE=/cache/npm -e PLAYWRIGHT_BROWSERS_PATH=/browsers --entrypoint bash "$browser_image" -eu -c '
 npm ci --prefer-offline
 npm run typecheck
 npm run lint
 npx --no-install playwright test --workers=1 tests/e2e/action-outcomes.spec.ts
 ' > "$proof/checks.log" 2>&1 || { tail -70 "$proof/checks.log"; exit 1; }
tail -12 "$proof/checks.log"
printf "Owner outcome web checks PASS; proof=%s\n" "$proof"
