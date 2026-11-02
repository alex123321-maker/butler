#!/usr/bin/env bash
# Compare a private fixture reference with already recorded observations only.
set -euo pipefail
umask 077
set -o noclobber
root=${1:?owned evaluation root}; trial=${2:?existing trial}
[[ "$root" =~ ^/srv/storage/butler/agents/(browser-authority-isolation|server-browser-get-form)-[0-9]{8}$ && "$trial" =~ ^((authority-isolation|core-evidence)-0[1-3]|get-form-control-0[1-8])$ ]] || exit 2
fixture="$trial-fixture"
if [[ "$trial" == get-form-control-* ]]; then fixture="control${trial##*-}-fixture"; fi
if ! jq -e '.record|strings|test("^[a-f0-9]{32}$")' "$root/private/$fixture/private.json" >/dev/null 2>&1; then
 printf 'Native fixture reference is invalid; no audit verdict.\n'; exit 1
fi
raw="$root/private/$trial-observations-private.json"
out="$root/reports/$trial-observation-audit.json"
test ! -e "$raw"; test ! -e "$out"
trap 'rm -f "$raw"' EXIT
docker exec -i butler-main-postgres-1 psql -XqAt -v ON_ERROR_STOP=1 -U butler -d butler -v ev="browser-baseline-$trial" > "$raw" <<'SQL'
BEGIN TRANSACTION ISOLATION LEVEL REPEATABLE READ READ ONLY;
SELECT COALESCE(json_agg(json_build_object('tool',t.tool_name,'status',t.status,'result',t.result)),'[]'::json)
FROM tool_calls t JOIN accepted_inputs i USING(run_id)
WHERE i.event_id=:'ev' AND (t.tool_name LIKE 'browser.%' OR t.tool_name='web.fetch');
COMMIT;
SQL
jq -n --slurpfile config "$root/private/$fixture/private.json" --slurpfile observations "$raw" --arg trial "$trial" '
 ($config[0].record) as $expected | ($observations[0]) as $rows |
 {scope:"existing_observation_private_comparison",trial:$trial,
 browser_record_observed:any($rows[];.status=="completed" and (.tool|startswith("browser.")) and ((.result.text // "")|contains($expected))),
 http_record_observed:any($rows[];.status=="completed" and .tool=="web.fetch" and ((.result.content_text // "")|contains($expected))),
 guarded_search_completed:any($rows[];.status=="completed" and .tool=="browser.submit_get_form"),
 raw_text_exported:false,new_requests:0,new_model_calls:0}
' > "$out"
cat "$out"
