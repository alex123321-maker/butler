#!/usr/bin/env bash
set -euo pipefail
umask 077
base=${1:?owned browser-baseline root}
attempt=${2:?exact attempt}
[[ "$base" =~ ^/srv/storage/butler/agents/browser-baseline-[a-zA-Z0-9_-]+$ ]] || exit 2
[[ "$attempt" =~ ^(read|nav|form|login|consent)-0[1-3]$ ]] || exit 2
scenario=${attempt%-*}
metadata="$base/reports/$attempt-metadata-final.json"
ref=$(jq -er --arg event "browser-baseline-matrix-20261028-$attempt" 'select(.event==$event)|[.publications[]?|select(.purpose=="result")]|select(length==1)|.[0].ref' "$metadata")
token="$base/private/$attempt-verify-owner-token"
test ! -e "$token"
trap 'rm -f "$token"' EXIT
docker exec butler-main-orchestrator-1 cat /run/secrets/owner > "$token"
"$base/browser-eval-verifier" verify-fixture -endpoint http://192.168.0.5:8088 \
 -owner-token-file "$token" -publication-ref "$ref" -scenario "$scenario" \
 -config "$base/private/$attempt/private.json" \
 -journal "$base/private/$attempt/recipient.jsonl" \
 -output "$base/reports/$attempt-verification.json"
printf 'Private fixture comparison saved; only booleans/counts exported.\n'
