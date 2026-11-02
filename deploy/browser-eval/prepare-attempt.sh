#!/usr/bin/env bash
# Operator must first confirm the previous Butler Input is terminal (or absent).
set -euo pipefail
attempt=${1:?new attempt ID}
previous=${2:?previous owned instance, or infra-smoke}
[[ "$attempt" =~ ^[a-z][a-z0-9-]{1,70}$ ]] || exit 2
[[ "$previous" =~ ^[a-z][a-z0-9-]{1,70}$ ]] || exit 2
directory="/var/lib/butler-browser-eval/$attempt"
test ! -e "$directory"
install -d -m 700 -o butler-browser-eval -g butler-browser-eval "$directory"
/opt/butler-browser-eval/browser-eval -init -config "$directory/private.json"
chown butler-browser-eval:butler-browser-eval "$directory/private.json"
sha256sum /opt/butler-browser-eval/browser-eval > "$directory/recipient-binary.sha256"
systemctl stop "butler-browser-eval@$previous"
unit="butler-browser-eval@$attempt"
systemctl start "$unit"
ready=false
ready_now() {
 systemctl is-active --quiet "$unit" &&
 curl --connect-timeout 1 --max-time 2 --fail --silent http://127.0.0.1:18097/login | grep -q '<form method=post>'
}
for n in $(seq 1 50); do
 if ready_now; then ready=true; break; fi
 sleep 0.1
done
$ready || { printf 'Recipient readiness failed; no native attempt should start.\n'; exit 1; }
pid=$(systemctl show "$unit" -p MainPID --value)
[[ "$pid" =~ ^[1-9][0-9]*$ ]]
sleep 0.25
ready_now
[[ $(systemctl show "$unit" -p MainPID --value) == "$pid" ]]
test ! -s "$directory/recipient.jsonl"
printf 'New isolated recipient ready; previous journal retained.\n'
