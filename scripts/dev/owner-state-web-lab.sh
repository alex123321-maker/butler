#!/usr/bin/env bash
# Real Chromium presentation/control checks; intercepted fixtures, no inference.
set -euo pipefail
root=${1:?isolated agent root required}
[[ "$root" =~ ^/srv/storage/butler/agents/[a-zA-Z0-9_-]+$ ]] || exit 2
source_dir=$(readlink -f "$root/current")
[[ "$source_dir" == "$root/snapshots/"* && -f "$source_dir/web/package-lock.json" ]] || exit 2
mkdir -p "$root/reports" /srv/storage/butler/.cache/npm /srv/storage/butler/.cache/ms-playwright
report="$root/reports/owner-state-web-$(date -u +%Y%m%dT%H%M%SZ).log"
printf 'Owner Web presentation proof: %s\n' "$report"
docker run --rm --user "$(id -u):$(id -g)" --network butler-lab_backend \
  --memory 2g --cpus 2 --pids-limit 256 --shm-size 256m --security-opt no-new-privileges:true \
  --volume "$source_dir:/src" --volume /srv/storage/butler/.cache:/cache --workdir /src/web \
  --env NPM_CONFIG_CACHE=/cache/npm --env PLAYWRIGHT_BROWSERS_PATH=/cache/ms-playwright \
  --entrypoint bash butler-lab-tool-browser -eu -c '
npm ci
npm run typecheck
npm run lint
npx --no-install playwright test --workers=1 --grep-invert "screenshots: key pages visual baseline" tests/e2e/task-execution-result.spec.ts tests/e2e/task-owner-controls.spec.ts tests/e2e/task-centric-pages.spec.ts tests/e2e/owner-approval-facts.spec.ts tests/e2e/durable-input-approval.spec.ts tests/e2e/owner-goals.spec.ts tests/e2e/initiatives.spec.ts tests/e2e/reviewed-extensions.spec.ts tests/e2e/versioned-memory.spec.ts tests/e2e/owner-memory-proof.spec.ts tests/e2e/owner-workspace-recovery.spec.ts' \
  > "$report" 2>&1
tail -n 18 "$report"
sha256sum "$report"
