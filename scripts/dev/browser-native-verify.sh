#!/usr/bin/env bash
# Verify the single public control result without printing its text.
set -euo pipefail
umask 077
base=${1:?owned browser-baseline root}
[[ "$base" =~ ^/srv/storage/butler/agents/browser-baseline-[a-zA-Z0-9_-]+$ ]] || exit 2
metadata=${2:?exact-attempt metadata file}
test ! -e "$base/reports/control-01-verification.json"
ref=$(jq -er 'select(.event=="browser-baseline-public-control-20261028-01")|.publications|select(length==1)|.[0]|select(.purpose=="result")|.ref' "$metadata")
token="$base/private/verify-owner-token"
test ! -e "$token"
trap 'rm -f "$token"' EXIT
docker exec butler-main-orchestrator-1 cat /run/secrets/owner > "$token"
"$base/browser-eval" verify-public-control -endpoint http://192.168.0.5:8088 \
 -owner-token-file "$token" -publication-ref "$ref" \
 -output "$base/reports/control-01-verification.json"
printf 'Protected publication checked; only rubric booleans exported.\n'
