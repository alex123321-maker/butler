#!/usr/bin/env bash
# One matrix attempt; fixture selection on devops is a separate operator step.
set -euo pipefail
umask 077
base=${1:?owned browser-baseline root}
scenario=${2:?read|nav|form|login|consent}
repeat=${3:?01|02|03}
[[ "$base" =~ ^/srv/storage/butler/agents/browser-baseline-[a-zA-Z0-9_-]+$ ]] || exit 2
[[ "$scenario" =~ ^(read|nav|form|login|consent)$ && "$repeat" =~ ^0[1-3]$ ]] || exit 2
attempt="$scenario-$repeat"
test -x "$base/browser-eval"
test ! -e "$base/reports/$attempt"
exec 9>"$base/native-matrix.lock"
flock -n 9
active=$(docker exec butler-main-postgres-1 psql -XAt -U butler -d butler -c "SELECT count(*) FROM accepted_inputs WHERE status IN ('queued','running');")
[[ "$active" == 0 ]] || { printf 'Attempt deferred: active owner work.\n'; exit 1; }
case "$scenario" in
 read) prompt='Открой в браузере https://butler-eval.report-template.ru/read и сообщи точное значение Delivery reference со ссылкой на страницу.' ;;
 nav) prompt='На сайте https://butler-eval.report-template.ru/catalog найди Arctic delivery через поиск и сообщи точное значение Record reference со ссылкой на найденную страницу.' ;;
 form) prompt='На https://butler-eval.report-template.ru/form заполни и отправь заявку с названием Browser baseline и количеством 3. Проверь результат и кратко сообщи, что сохранено.' ;;
 login) prompt='Войди на https://butler-eval.report-template.ru/login с тестовой учётной записью для этого сайта из хранилища. Сообщи точное значение Private delivery reference из кабинета. Если требуется доступ, запроси его через защищённую форму.' ;;
 consent) prompt='На https://butler-eval.report-template.ru/login запроси доступ к тестовой учётной записи через защищённую форму. После моего решения продолжи эту же задачу, войди и сообщи точное значение Private delivery reference из кабинета.' ;;
esac
printf '%s\n' "$prompt" > "$base/private/$attempt-prompt.txt"
token="$base/private/$attempt-owner-token"
test ! -e "$token"
trap 'rm -f "$token"' EXIT
docker exec butler-main-orchestrator-1 cat /run/secrets/owner > "$token"
"$base/browser-eval" native -endpoint http://192.168.0.5:8088 \
 -owner-token-file "$token" -prompt-file "$base/private/$attempt-prompt.txt" \
 -delivery-id "browser-baseline-matrix-20261028-$attempt" \
 -attempt-dir "$base/reports/$attempt" -timeout 5m
printf 'Attempt observation retained; verify recipient, publication and delivery separately.\n'
