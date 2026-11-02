#!/usr/bin/env bash
# Operator only. Resolve DB connection in a private file, never command args.
set -euo pipefail
umask 077
root=${1:?owned review root}
mode=${2:?dry|apply}
ids=${3:?exact comma-separated Input refs}
[[ "$root" =~ ^(/srv/storage/butler/agents/browser-continuation-|/tmp/butler-browser-continuation-)[a-zA-Z0-9_-]+$ ]] || exit 2
[[ "$mode" == dry || "$mode" == apply ]] || exit 2
[[ "$ids" =~ ^input-[a-f0-9]{32}(,input-[a-f0-9]{32}){0,7}$ ]] || exit 2
test -x "$root/.cache/stopped-review"
run="$(date -u +%Y%m%dT%H%M%SZ)-$$"
private="$root/.cache/review-$run.env"
trap 'rm -f "$private"' EXIT
docker inspect --format '{{json .Config.Env}}' butler-main-orchestrator-1 | jq -er '.[]|select(startswith("BUTLER_POSTGRES_URL="))' >"$private"
test "$(wc -l <"$private")" = 1
snapshot() {
 docker exec -i butler-main-postgres-1 psql -XqAt -v ON_ERROR_STOP=1 -U butler -d butler -v ids="$ids" <<'SQL'
BEGIN TRANSACTION ISOLATION LEVEL REPEATABLE READ READ ONLY;
WITH selected AS(SELECT * FROM accepted_inputs WHERE input_id=ANY(string_to_array(:'ids',','))), rr AS(SELECT run_id FROM selected)
SELECT jsonb_build_object(
 'stops',(SELECT jsonb_agg(jsonb_build_object('input',f.input_id,'classification',f.classification,'reason',f.reason) ORDER BY f.input_id) FROM input_stopped_attempts f JOIN selected USING(input_id)),
 'effects',(SELECT jsonb_agg(jsonb_build_object('id',action_id,'state',state) ORDER BY action_id) FROM action_effects WHERE initial_binding->>'run_id' IN(SELECT run_id FROM rr)),
 'history_hash',md5(jsonb_build_object('inputs',(SELECT jsonb_agg(to_jsonb(i) ORDER BY input_id) FROM selected i),'runs',(SELECT jsonb_agg(to_jsonb(r) ORDER BY run_id) FROM runs r WHERE run_id IN(SELECT run_id FROM rr)),
 'usage',(SELECT jsonb_agg(to_jsonb(x) ORDER BY operation_id) FROM execution_model_attempts x WHERE run_id IN(SELECT run_id FROM rr)),
 'grants',(SELECT jsonb_agg(to_jsonb(g) ORDER BY authorization_id) FROM action_authorizations g WHERE action_id IN(SELECT action_id FROM action_intents WHERE run_id IN(SELECT run_id FROM rr))),
 'dispatch',(SELECT jsonb_agg(to_jsonb(d) ORDER BY authorization_id) FROM action_effect_dispatches d WHERE execution_action_id IN(SELECT action_id FROM action_intents WHERE run_id IN(SELECT run_id FROM rr))),
 'publications',(SELECT jsonb_agg(to_jsonb(p) ORDER BY publication_id) FROM communication_publications p WHERE run_id IN(SELECT run_id FROM rr)))::text));
COMMIT;
SQL
}
before="$root/reports/review-$run-before.json"
after="$root/reports/review-$run-after.json"
snapshot >"$before"
args=(-inputs "$ids")
if [[ "$mode" == apply ]]; then args+=(-apply); fi
sha256sum "$root/.cache/stopped-review" >"$root/reports/review-$run-binary.sha256"
set +e
docker run --rm --read-only --network container:butler-main-orchestrator-1 --user "$(id -u):$(id -g)" \
 --memory 128m --cpus 1 --pids-limit 64 --cap-drop ALL --security-opt no-new-privileges:true \
 --env-file "$private" -v "$root/.cache/stopped-review:/review:ro" --entrypoint /review golang:1.27.2-bookworm "${args[@]}" | tee "$root/reports/review-$run-result.jsonl"
status=${PIPESTATUS[0]}
set -e
snapshot >"$after"
[[ "$(jq -r .history_hash "$before")" == "$(jq -r .history_hash "$after")" ]] || { printf 'Retained history comparison failed.\n'; exit 1; }
if [[ "$mode" == dry ]]; then cmp "$before" "$after"; fi
printf 'review=%s mode=%s original_history_retained=true\n' "$run" "$mode"
exit "$status"
