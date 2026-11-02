#!/usr/bin/env bash
# Private lab composition only. Approval/uncertainty/provider settings stay put.
set -euo pipefail
umask 077
root=${1:?owned browser evaluation root}
[[ "$root" =~ ^/srv/storage/butler/agents/server-browser-get-form-[0-9]{8}$ ]] || exit 2
main=/srv/storage/butler/main
bundle="$root/core-evidence-47f9429c-cb7c937e"
release="$main/releases/browser-core-evidence-20261028-6cb3cff0"
core=butler-main-orchestrator-1
old_sha=10aecd518d3ca819c737ccfb154138cd0817a8174274bfde31ca08b144f8316e
new_sha=0d0c38632dd351455c6989deeb465e1becdb095c31e040f7b468a9cc705a13f0
old_bin="$main/releases/current-request-20261025-47f9429c/bin"
test ! -e "$release"
[[ "$(docker exec "$core" sha256sum /proc/1/exe | cut -d ' ' -f 1)" == "$old_sha" ]]
[[ "$(sha256sum "$bundle/bin/orchestrator" | cut -d ' ' -f 1)" == "$new_sha" ]]
idle() {
 [[ "$(docker exec butler-main-postgres-1 psql -XqAt -v ON_ERROR_STOP=1 -U butler -d butler -c "SELECT count(*) FROM accepted_inputs WHERE status IN ('queued','running','waiting_for_user');")" == 0 ]]
}
idle || { printf 'Live owner work exists; Core unchanged.\n'; exit 1; }
mkdir -m 700 "$release" "$release/private"
mkdir -m 755 "$release/bin"
install -m 755 "$bundle/bin/orchestrator" "$release/bin/orchestrator"
docker inspect "$core" > "$release/private/core-before.json"
jq -e --arg old "$old_bin" '.[0].Path=="/candidate/orchestrator" and any(.[0].Mounts[];.Destination=="/candidate" and .Source==$old)' "$release/private/core-before.json" >/dev/null
# JSON keeps multiline values private. Escape Compose interpolation, then check
# the actual resulting environment hash before accepting the replacement.
for mode in candidate rollback; do
 bin="$release/bin"; [[ "$mode" == rollback ]] && bin="$old_bin"
 jq -n --slurpfile before "$release/private/core-before.json" --arg bin "$bin" '
  ($before[0][0].Config.Env | map(. as $e | index("=") as $i | {key:$e[0:$i],value:$e[$i+1:]}) | from_entries | with_entries(.value |= gsub("\\$";"$$"))) as $env |
  {services:{orchestrator:{environment:$env,volumes:[{type:"bind",source:$bin,target:"/candidate",read_only:true}]}}}
 ' > "$release/private/$mode.compose.json"
done
IFS=, read -ra files <<< "$(docker inspect --format '{{index .Config.Labels "com.docker.compose.project.config_files"}}' butler-main-tool-broker-1)"
args=()
for file in "${files[@]}"; do
 [[ "$file" == "$main/"* && -f "$file" ]] || exit 1
 args+=(-f "$file")
done
[[ "${files[0]}" == "$main/compose.json" ]]
printf '%s\n' "${files[@]}" > "$release/private/base-compose-files.txt"
env_before=$(jq -cS '.[0].Config.Env|sort' "$release/private/core-before.json" | sha256sum | cut -d ' ' -f 1)
peers_before=$(docker inspect butler-main-tool-broker-1 butler-main-tool-browser-1 butler-main-postgres-1 | jq -c 'map({id:.Id,started_at:.State.StartedAt})' | sha256sum | cut -d ' ' -f 1)
health() { docker inspect "$core" | jq -e 'all(.[];.State.Running and .State.Health.Status=="healthy")' >/dev/null; }
rollback() {
 printf 'Core rollout failed; restoring pinned previous binary and environment.\n'
 docker compose -p butler-main "${args[@]}" -f "$release/private/rollback.compose.json" up -d --no-build --no-deps orchestrator > "$release/private/rollback.log" 2>&1
 for i in $(seq 1 45); do health && break; sleep 1; done
 health
 [[ "$(docker exec "$core" sha256sum /proc/1/exe | cut -d ' ' -f 1)" == "$old_sha" ]]
}
idle || { printf 'Live owner work arrived; Core unchanged.\n'; exit 1; }
if ! docker compose -p butler-main "${args[@]}" -f "$release/private/candidate.compose.json" up -d --no-build --no-deps orchestrator > "$release/private/rollout.log" 2>&1; then rollback; exit 1; fi
healthy=false
for i in $(seq 1 45); do if health; then healthy=true; break; fi; sleep 1; done
if ! $healthy; then rollback; exit 1; fi
env_after=$(docker inspect "$core" | jq -cS '.[0].Config.Env|sort' | sha256sum | cut -d ' ' -f 1)
peers_after=$(docker inspect butler-main-tool-broker-1 butler-main-tool-browser-1 butler-main-postgres-1 | jq -c 'map({id:.Id,started_at:.State.StartedAt})' | sha256sum | cut -d ' ' -f 1)
actual=$(docker exec "$core" sha256sum /proc/1/exe | cut -d ' ' -f 1)
if [[ "$env_before" != "$env_after" || "$actual" != "$new_sha" || "$peers_before" != "$peers_after" ]]; then rollback; exit 1; fi
jq -n --arg release "$release" --arg sha "$actual" '{scope:"core-only-rollout",release:$release,executable_sha256:$sha,healthy:true,environment_unchanged:true,broker_browser_postgres_unchanged:true,provider_configuration_changed:false,unknown_effects_reconciled:0,native_qualification:"PENDING"}' > "$root/reports/core-evidence-rollout.json"
cat "$root/reports/core-evidence-rollout.json"
