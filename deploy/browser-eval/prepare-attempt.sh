#!/usr/bin/env bash
# Operator must first confirm the previous Butler Input is terminal (or absent).
set -euo pipefail
attempt=${1:?new attempt ID}
previous=${2:?previous owned instance, or infra-smoke}
[[ "$attempt" =~ ^(read|nav|form|login|consent)-0[1-3]$ ]] || exit 2
[[ "$previous" == infra-smoke || "$previous" =~ ^(read|nav|form|login|consent)-0[1-3]$ ]] || exit 2
directory="/var/lib/butler-browser-eval/$attempt"
test ! -e "$directory"
install -d -m 700 -o butler-browser-eval -g butler-browser-eval "$directory"
/opt/butler-browser-eval/browser-eval -init -config "$directory/private.json"
chown butler-browser-eval:butler-browser-eval "$directory/private.json"
sha256sum /opt/butler-browser-eval/browser-eval > "$directory/recipient-binary.sha256"
systemctl stop "butler-browser-eval@$previous"
systemctl start "butler-browser-eval@$attempt"
systemctl is-active "butler-browser-eval@$attempt"
ready=false
for n in $(seq 1 50); do
 if curl --fail --silent http://127.0.0.1:18097/ | grep -q '<h1>Browser baseline</h1>'; then ready=true; break; fi
 sleep 0.1
done
$ready || { printf 'Recipient readiness failed; no native attempt should start.\n'; exit 1; }
printf 'New isolated recipient ready; previous journal retained.\n'
