#!/usr/bin/env bash
# Exact lab rollout. Private settings stay in protected files; no outcome is reset.
set -euo pipefail
set +x
umask 077
root=${1:?owned browser-owner test root}
[[ "$root" =~ ^/srv/storage/butler/agents/server-browser-owners-[0-9]{8}$ ]] || exit 2
bundle="$root/build-4a80a3a97c04"
source_dir=/tmp/butler-owner-build-4a80a3a97c04
main=/srv/storage/butler/main
release="$main/releases/browser-owner-outcomes-20261029-4a80a3a97c04"
test ! -e "$release"
for component in orchestrator tool-broker tool-browser;do
 case "$component" in
 orchestrator) old=b157b123215d9a7499551488054992c28a8fc41d9a3649bab79ba3e0fcee8614;;
 tool-broker) old=6407f3470786f11b0012c18054ceca5a071315d85bf02a277eccac0b19f1a592;;
 tool-browser) old=639ccf440bf95cd40e21dab7fe687f1b1d4c0f7ee94b9d724da253f67bef0142;;
 esac
 [[ "$(docker exec "butler-main-$component-1" sha256sum /proc/1/exe | cut -d ' ' -f 1)" == "$old" ]]
done
idle(){ [[ "$(docker exec butler-main-postgres-1 psql -XqAt -v ON_ERROR_STOP=1 -U butler -d butler -c "SELECT count(*) FROM accepted_inputs WHERE status IN ('queued','running','waiting_for_user');")" == 0 ]]; }
idle || { printf 'Live owner work exists; deployment deferred.\n';exit 1; }
sha256sum -c "$bundle/reports/binaries.sha256" >/dev/null
mkdir -m 700 "$release" "$release/private"
mkdir -m 755 "$release/core" "$release/broker" "$release/browser" "$release/scripts" "$release/migrations"
install -m 755 "$bundle/bin/orchestrator" "$release/core/orchestrator"
install -m 755 "$bundle/bin/broker" "$release/broker/broker"
install -m 755 "$bundle/bin/tool-browser" "$release/browser/tool-browser"
install -m 644 "$source_dir/browser/apps/tool-browser/scripts/browser_runtime.mjs" "$release/scripts/browser_runtime.mjs"
install -m 644 "$source_dir/core/migrations/105_browser_owner_outcomes.up.sql" "$release/migrations/105_browser_owner_outcomes.up.sql"
docker inspect butler-main-orchestrator-1 butler-main-tool-broker-1 butler-main-tool-browser-1 > "$release/private/before.json"
docker cp butler-main-tool-broker-1:/candidate/tools.json "$release/private/catalog-before.json"
jq --slurpfile project "$source_dir/browser/configs/tools.json" '
 (.tools|map(select(.tool_name=="browser.navigate"))|.[0]) as $nav |
 ($project[0].tools|map(select(.tool_name=="browser.freeze_snapshot"))) as $new |
 if ($new|length)!=1 then error("exact freeze contract missing") else . end |
 .tools=((.tools|map(select(.tool_name!="browser.freeze_snapshot")))+($new|map(.runtime_target=$nav.runtime_target|.allowed_domains=$nav.allowed_domains)))
' "$release/private/catalog-before.json" > "$release/broker/tools.json"
chmod 644 "$release/broker/tools.json"
jq -n --slurpfile before "$release/private/before.json" --arg r "$release" '
 def env: .Config.Env|map(. as $e|index("=") as $i|{key:$e[0:$i],value:$e[$i+1:]})|from_entries|with_entries(.value|=gsub("\\$";"$$"));
 {services:{orchestrator:{environment:($before[0][0]|env),volumes:[{type:"bind",source:($r+"/core"),target:"/candidate",read_only:true}]},
 "tool-broker":{environment:($before[0][1]|env),volumes:[{type:"bind",source:($r+"/broker"),target:"/candidate",read_only:true}]},
 "tool-browser":{environment:($before[0][2]|env),volumes:[{type:"bind",source:($r+"/browser"),target:"/candidate",read_only:true},{type:"bind",source:($r+"/scripts"),target:"/app/apps/tool-browser/scripts",read_only:true}]}}}
' > "$release/private/candidate.compose.json"
IFS=, read -ra files <<< "$(docker inspect --format '{{index .Config.Labels "com.docker.compose.project.config_files"}}' butler-main-orchestrator-1)"
args=()
for file in "${files[@]}";do [[ "$file" == "$main/"* && -f "$file" ]] || exit 1;args+=(-f "$file");done
printf '%s\n' "${files[@]}" > "$release/private/base-compose-files.txt"
env_before=$(jq -cS 'map(.Config.Env|sort)' "$release/private/before.json" | sha256sum | cut -d ' ' -f 1)
peers_before=$(docker inspect butler-main-postgres-1 butler-main-redis-1 butler-main-web-1 | jq -c 'map({id:.Id,started:.State.StartedAt})' | sha256sum | cut -d ' ' -f 1)
docker inspect --format '{{json .Config.Env}}' butler-main-orchestrator-1 | jq -er '.[]|select(startswith("BUTLER_POSTGRES_URL="))' > "$release/private/migrator.env"
[[ "$(wc -l < "$release/private/migrator.env")" == 1 ]]
idle || exit 1
docker run --rm --read-only --network container:butler-main-orchestrator-1 --memory 128m --cpus 1 --pids-limit 64 --cap-drop ALL --security-opt no-new-privileges:true \
 --env-file "$release/private/migrator.env" -v "$bundle/bin/migrator:/operator:ro" -v "$release/migrations:/migrations:ro" --entrypoint /operator golang:1.27.2-bookworm \
 -direction up -migrations-dir /migrations > "$release/private/migration.log" 2>&1
rm -f "$release/private/migrator.env"
healthy(){ docker inspect butler-main-orchestrator-1 butler-main-tool-broker-1 butler-main-tool-browser-1 | jq -e 'all(.[];.State.Running and .State.Health.Status=="healthy")' >/dev/null; }
rollback(){
 printf 'Restoring previous service bytes/settings; additive migration/evidence retained.\n'
 docker compose -p butler-main "${args[@]}" up -d --no-build --no-deps orchestrator tool-broker tool-browser > "$release/private/rollback.log" 2>&1
}
idle || exit 1
if ! docker compose -p butler-main "${args[@]}" -f "$release/private/candidate.compose.json" up -d --no-build --no-deps orchestrator tool-broker tool-browser > "$release/private/rollout.log" 2>&1;then rollback;exit 1;fi
ok=false
for i in $(seq 1 45);do if healthy;then ok=true;break;fi;sleep 1;done
if ! $ok;then rollback;exit 1;fi
env_after=$(docker inspect butler-main-orchestrator-1 butler-main-tool-broker-1 butler-main-tool-browser-1 | jq -cS 'map(.Config.Env|sort)' | sha256sum | cut -d ' ' -f 1)
peers_after=$(docker inspect butler-main-postgres-1 butler-main-redis-1 butler-main-web-1 | jq -c 'map({id:.Id,started:.State.StartedAt})' | sha256sum | cut -d ' ' -f 1)
if [[ "$env_before" != "$env_after" || "$peers_before" != "$peers_after" ]];then rollback;exit 1;fi
for pair in orchestrator:orchestrator tool-broker:broker tool-browser:tool-browser;do
 component=${pair%:*};binary=${pair#*:}
 expected=$(sha256sum "$bundle/bin/$binary" | cut -d ' ' -f 1)
 if [[ "$(docker exec "butler-main-$component-1" sha256sum /proc/1/exe | cut -d ' ' -f 1)" != "$expected" ]];then rollback;exit 1;fi
done
jq -n --arg release "$release" '{release:$release,healthy:true,environment_unchanged:true,postgres_redis_web_unchanged:true,migration:105,unknowns_resolved_by_deployment:0,native_acceptance:"PENDING"}' > "$root/reports/rollout.json"
cat "$root/reports/rollout.json"
