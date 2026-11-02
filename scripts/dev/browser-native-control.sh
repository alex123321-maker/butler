#!/usr/bin/env bash
# One operator-authorized, public read control through the ordinary owner ingress.
set -euo pipefail
umask 077
base=${1:?owned browser-baseline root required}
[[ "$base" =~ ^/srv/storage/butler/agents/browser-baseline-[a-zA-Z0-9_-]+$ ]] || exit 2
test -x "$base/browser-eval"
mkdir -p "$base/private" "$base/reports"
exec 9>"$base/native-control.lock"
flock -n 9
test ! -e "$base/reports/control-01"
# Avoid queueing behind unrelated active requests. No task is stopped or reset.
active=$(docker exec butler-main-postgres-1 psql -XAt -U butler -d butler -c "SELECT count(*) FROM accepted_inputs WHERE status IN ('queued','running');")
[[ "$active" == 0 ]] || { printf 'Native control deferred: active owner work.\n'; exit 1; }
token="$base/private/control-owner-token"
test ! -e "$token"
trap 'rm -f "$token"' EXIT
docker exec butler-main-orchestrator-1 cat /run/secrets/owner > "$token"
printf '%s\n' 'Открой https://example.com/ в браузере. Прочитай заголовок и объяснение назначения домена на странице. Сообщи краткий результат со ссылкой. Никаких изменений на сайте не требуется.' > "$base/private/control-prompt.txt"
"$base/browser-eval" native -endpoint http://192.168.0.5:8088 \
 -owner-token-file "$token" -prompt-file "$base/private/control-prompt.txt" \
 -delivery-id browser-baseline-public-control-20261028-01 \
 -attempt-dir "$base/reports/control-01" -timeout 3m
printf 'Control observation saved; task/effect/delivery still require independent evidence.\n'
