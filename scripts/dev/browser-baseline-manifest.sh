#!/usr/bin/env bash
# Read-only, content-free deployment inventory. No docker Config.Env dump.
set -euo pipefail
umask 077
set -o noclobber
out=${1:?new report file required}
test ! -e "$out"
command -v jq >/dev/null
containers=(butler-main-orchestrator-1 butler-main-tool-broker-1 butler-main-tool-browser-1 butler-main-web-1)
docker inspect "${containers[@]}" | jq --arg at "$(date -u +%FT%TZ)" '
 {schema:1, captured_at:$at, scope:"deployment_inventory_only", native_attempts:0,
 containers:map({name:.Name, image_id:.Image, image_reference:.Config.Image,
 started_at:.State.StartedAt, running:.State.Running,
 health:(.State.Health.Status // "unavailable"),
 source_revision:(.Config.Labels["org.opencontainers.image.revision"] // null),
 flags:[(.Config.Env // [])[] |
 select(test("^BUTLER_(PUBLIC_WEB_READS_ENABLED|TOOL_DISCOVERY_ENABLED)=(true|false)$"))]})}
' > "$out"
test -s "$out"
printf 'Content-free deployment inventory saved. Missing source/flags remain unavailable.\n'
