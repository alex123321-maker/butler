#!/usr/bin/env bash
# Run only after the parent explicitly confirms the actual V20 process is complete.
# Current composition: two current cases plus retained V17 Stop/V18 absence and V14 technical work; no old normalPASS.
# Copies fourteen current and twenty historical named safe leaves plus only named own-tool
# proposals from the safe scope metadata. Never mount the source private directory.
set -euo pipefail
umask 077
root=/srv/storage/butler/agents/scenario-release-chain-v20-high-20261013
src="$root/snapshots/20261014-050618-996"
proof=${1:?completed actual proof path required}
auditor_src=${2:?separate frozen public auditor snapshot required}
auditor_bin=${3:?separate readonly built audit binary required}
auditor_commit=${4:?separate exact auditor commit required}
auditor_tree=${5:?separate independently retained auditor TreeHash required}
auditor_binary_sha=${6:?separate independently retained audit executable SHA required}
measured_tree=${7:?actual independently reviewed708 source TreeHash required}
measured_app=${8:?actual independently reviewed708 app.test SHA required}
[[ "$measured_tree" == f997bf55998ac973cbd7a8ddea15b7bb58ad4656f7d355329ca098101e2459eb && "$measured_app" == c01b83e6985c4cdfc028479808b7a2f1ad039244bab3a697a5765674eac74357 ]]
[[ "$auditor_src" =~ ^/srv/storage/butler/agents/[a-z0-9-]+/snapshots/[0-9-]+$ && "$auditor_bin" =~ ^/srv/storage/butler/agents/[a-z0-9-]+/reports/[a-zA-Z0-9-]+/bin/chain-final-audit$ ]]
[[ "$auditor_commit" =~ ^[a-f0-9]{40}$ && "$auditor_tree" =~ ^[a-f0-9]{64}$ && "$auditor_binary_sha" =~ ^[a-f0-9]{64}$ ]]
[[ "$proof" =~ ^/srv/storage/butler/agents/scenario-release-chain-v20-high-20261013/reports/release-chain-process-[0-9]{8}T[0-9]{6}Z-[0-9]+$ ]]
review=/srv/storage/butler/reviews/issue33-chain-v20-20261013
if [[ ! -e "$review" && ! -L "$review" ]]; then mkdir -m 700 "$review"; fi
[[ -d "$review" && ! -L "$review" && "$(stat -c %a "$review")" == 700 ]]
audit_dir="$review/v20-audit-708def1-$(date -u +%Y%m%dT%H%M%SZ)-$$"
[[ ! -e "$audit_dir" && ! -L "$audit_dir" ]]
mkdir -m 700 "$audit_dir"
stage="$audit_dir/inputs"
mkdir -m 700 "$stage"
proposal_stage="$audit_dir/proposals"
empty_proposals="$audit_dir/empty-proposals"
mkdir -m 700 "$proposal_stage" "$empty_proposals"
go_image=sha256:5cf287a799e6b94384bad13d16b14904c531f51ba65792237e122ce42b392f61
pg_image=sha256:ac08538c6f8b9904c33c8224c5e5706dbe760aca29db1d096972b4052c22a75d
runtime_image=sha256:ce64758a109eb420d874a118f87920e625e12d3634e03b4a5573fd9f6e5d3507
leaves=(scope pg-ledger pg-initial pg-product runtime-state hardware runtime-profile source-manifest source-commit quality prerequisite expense-exception timeout-waiver historical557-expense-exception)
origins=("$proof/scope.json" "$proof/pg-ledger-witness.json" "$proof/pg-initial-witness.json" "$proof/pg-product-witness.json" "$proof/runtime-state.json" "$proof/hardware.json" "$proof/runtime-profile.json" "$proof/source-manifest.sha256" "$proof/source-commit.txt" "$src/deploy/acceptance/cloud-shadow-v5-final-audit-20261013.json" "$src/deploy/acceptance/release-chain-v20-high-prerequisite-20261013.json" "$src/deploy/acceptance/release-chain-v6-user-expense-exception-20261013.json" "$src/deploy/acceptance/release-chain-v20-user-policy-amendment-20261013.json" "$src/deploy/acceptance/release-chain-v11-user-expense-exception-20261013.json")
names=(scope.json pg-ledger.json pg-initial.json pg-product.json runtime-state.json hardware.json runtime-profile.json source-manifest.sha256 source-commit.txt quality.json prerequisite.json expense-exception.json timeout-waiver.json historical557-expense-exception.json)
copy_mounts=(--volume "$stage:/out" --volume "$proposal_stage:/proposalout" --volume "$empty_proposals:/emptyout")
audit_mounts=(--volume "$src:/src:ro" --volume "$proof/bin/app.test:/app.test:ro" --volume "$proof/bin/tool-broker:/tool-broker:ro" --volume "$proof/bin/tool-converter:/tool-converter:ro" --volume "$auditor_bin:/chain-final-audit:ro" --volume "$auditor_src:/auditor-src:ro" --volume "$empty_proposals:/proposals:ro")
for i in "${!origins[@]}"; do
 [[ -f "${origins[$i]}" && ! -L "${origins[$i]}" ]]
 copy_mounts+=(--volume "${origins[$i]}:/in-${names[$i]}:ro")
 audit_mounts+=(--volume "$stage/${names[$i]}:/${names[$i]}:ro")
done
# Only individually named safe old leaves. Preserve the original V17 0B
# scope as 0B; its dedicated checker never interprets it as JSON or zero calls.
historical_stage="$audit_dir/historical-safe"
v18_stage="$audit_dir/retained-v18"
v19_stage="$audit_dir/retained-v19"
mkdir -m 700 "$historical_stage" "$v18_stage" "$v19_stage"
old16=/srv/storage/butler/agents/scenario-release-chain-v16-high-20261013/reports/release-chain-process-20261013T200606Z-3246680
old17=/srv/storage/butler/agents/scenario-release-chain-v17-high-20261013/reports/release-chain-process-20261013T203729Z-3344610
old18=/srv/storage/butler/agents/scenario-release-chain-v18-high-20261013/reports/release-chain-process-20261013T211753Z-3469508
old19=/srv/storage/butler/agents/scenario-release-chain-v19-high-20261013/reports/release-chain-process-20261013T215220Z-3589182
old_names=(); old_origins=(); old_targets=()
for version in 16 17; do
 if [[ "$version" == 16 ]]; then old_proof="$old16"; else old_proof="$old17"; fi
 for name in high pg-ledger pg-product pg-initial runtime-state; do
  case "$name" in high) original=scope.json;; pg-ledger) original=pg-ledger-witness.json;; pg-product) original=pg-product-witness.json;; pg-initial) original=pg-initial-witness.json;; runtime-state) original=runtime-state.json;; esac
  old_names+=("release-chain-v$version-$name-20261013.json")
  old_origins+=("$old_proof/$original")
  old_targets+=("$historical_stage/release-chain-v$version-$name-20261013.json")
 done
done
for version in 18 19; do
 if [[ "$version" == 18 ]]; then old_proof="$old18"; target_stage="$v18_stage"; else old_proof="$old19"; target_stage="$v19_stage"; fi
 for name in scope ledger product initial runtime; do
  case "$name" in scope) original=scope.json;; ledger) original=pg-ledger-witness.json;; product) original=pg-product-witness.json;; initial) original=pg-initial-witness.json;; runtime) original=runtime-state.json;; esac
  old_names+=("retained-v$version-$name.json")
  old_origins+=("$old_proof/$original")
  old_targets+=("$target_stage/$name.json")
 done
done
[[ "${#old_names[@]}" == 20 ]]
old_copy_mounts=(--volume "$historical_stage:/historical-out" --volume "$v18_stage:/v18-out" --volume "$v19_stage:/v19-out")
old_copy_args=()
for i in "${!old_names[@]}"; do
 [[ -f "${old_origins[$i]}" && ! -L "${old_origins[$i]}" ]]
 if [[ "${old_names[$i]}" == release-chain-v17-high-20261013.json ]]; then [[ ! -s "${old_origins[$i]}" ]]; fi
 old_copy_mounts+=(--mount "type=bind,src=${old_origins[$i]},dst=/old-${old_names[$i]},readonly")
 case "${old_names[$i]}" in retained-v18-*) mount_path="/retained-v18/$(basename "${old_targets[$i]}")"; output_path="/v18-out/$(basename "${old_targets[$i]}")";; retained-v19-*) mount_path="/retained-v19/$(basename "${old_targets[$i]}")"; output_path="/v19-out/$(basename "${old_targets[$i]}")";; *) mount_path="/historical-safe/${old_names[$i]}"; output_path="/historical-out/${old_names[$i]}";; esac
 old_copy_args+=("/old-${old_names[$i]}" "$output_path" "$(sha256sum "${old_origins[$i]}"|cut -d' ' -f1)")
 audit_mounts+=(--mount "type=bind,src=${old_targets[$i]},dst=$mount_path,readonly")
done
docker run --rm --runtime runc --pull never --network none --read-only --security-opt no-new-privileges:true --memory 32m --pids-limit 16 "${old_copy_mounts[@]}" "$runtime_image" sh -eu -c '
 umask 077
 while test "$#" -gt 0; do
  original="$1"; output="$2"; expected="$3"; shift 3
  test -f "$original"; test ! -L "$original"; test ! -e "$output"
  test "$(sha256sum "$original"|cut -d" " -f1)" = "$expected"
  cp "$original" "$output"; chmod 600 "$output"; chown 10001:10001 "$output"
  test "$(sha256sum "$output"|cut -d" " -f1)" = "$expected"
 done
 chown 10001:10001 /historical-out /v18-out /v19-out
 chmod 700 /historical-out /v18-out /v19-out
' copy-only "${old_copy_args[@]}"

# Read only public-safe scope metadata to construct the finite named mounts.
python3 - "$proof/scope.json" > "$audit_dir/proposal-inventory.tsv" <<'PY'
import sys,json,re
v=json.load(open(sys.argv[1]))
assert v['window_id']=='issue33-high-release-chain-v20-20261013-baseline706'
items=v['proposal_observations'];assert isinstance(items,list) and len(items)<=120
previous=706
for m in items:
 ordinal=m['ordinal'];assert isinstance(ordinal,int) and previous<ordinal<=826
 previous=ordinal
 sha=m['private_proposal_sha256'];size=m['private_proposal_bytes']
 assert re.fullmatch('[a-f0-9]{64}',sha) and isinstance(size,int) and 0<size<=262144
 assert m['version']=='butler-native-tool-proposal-diagnostics-v1'
 print(f'tool-proposal-{ordinal:06d}.json\t{sha}\t{size}')
PY
proposal_names=()
while IFS=$'\t' read -r leaf pin bytes; do
 [[ "$leaf" =~ ^tool-proposal-[0-9]{6}\.json$ && "$pin" =~ ^[a-f0-9]{64}$ && "$bytes" =~ ^[0-9]+$ ]]
 origin="$proof/private/native/$leaf"
 # The containing native directory is deliberately unreadable to the SSH UID.
 # Docker resolves this exact non-creating bind as root; validation is below.
 proposal_names+=("$leaf" "$pin" "$bytes")
 copy_mounts+=(--mount "type=bind,src=$origin,dst=/in-$leaf,readonly")
 audit_mounts+=(--mount "type=bind,src=$proposal_stage/$leaf,dst=/proposals/$leaf,readonly")
done < "$audit_dir/proposal-inventory.tsv"
# Disposable bounded copying helper sees only named safe leaves and its new output.
docker run --rm --runtime runc --pull never --network none --read-only --security-opt no-new-privileges:true --memory 32m --pids-limit 16 "${copy_mounts[@]}" "$runtime_image" sh -eu -c '
 umask 077
 for name in scope.json pg-ledger.json pg-initial.json pg-product.json runtime-state.json hardware.json runtime-profile.json source-manifest.sha256 source-commit.txt quality.json prerequisite.json expense-exception.json timeout-waiver.json historical557-expense-exception.json; do
  test -f "/in-$name"; test ! -L "/in-$name"; test ! -e "/out/$name"
  cp "/in-$name" "/out/$name"; chmod 600 "/out/$name"; chown 10001:10001 "/out/$name"
 done
 while test "$#" -gt 0; do
  name="$1"; pin="$2"; bytes="$3"; shift 3
  test -f "/in-$name"; test ! -L "/in-$name"; test ! -e "/proposalout/$name"
  test "$(stat -c %a "/in-$name")" = 600; test "$(stat -c %u "/in-$name")" = 10001
  test "$(stat -c %s "/in-$name")" = "$bytes"; test "$(sha256sum "/in-$name"|cut -d" " -f1)" = "$pin"
  cp "/in-$name" "/proposalout/$name"; chmod 600 "/proposalout/$name"; chown 10001:10001 "/proposalout/$name"
  test ! -e "/emptyout/$name"; : > "/emptyout/$name"; chmod 600 "/emptyout/$name"; chown 10001:10001 "/emptyout/$name"
 done
 chown 10001:10001 /out; chmod 700 /out
 chown 10001:10001 /proposalout /emptyout; chmod 700 /proposalout /emptyout
' copy-only "${proposal_names[@]}"
for b in app.test tool-broker tool-converter; do [[ "$(stat -c %a "$proof/bin/$b")" == 555 ]]; done
[[ "$(sha256sum "$proof/bin/app.test"|cut -d' ' -f1)" == "$measured_app" ]]
[[ "$(sha256sum "$proof/bin/tool-broker"|cut -d' ' -f1)" == 2ed16aca088b7cb90b85b643a7e9533d336ca2d43ea0af5746e44847496d9e8e ]]
[[ "$(sha256sum "$proof/bin/tool-converter"|cut -d' ' -f1)" == 5f2dee5acef95c2f279b849d7af76003688a3b46a274e0d44456a37c40536f75 ]]
[[ "$(stat -c %a "$auditor_bin")" == 555 && "$(sha256sum "$auditor_bin"|cut -d' ' -f1)" == "$auditor_binary_sha" ]]
hash() { sha256sum "$1"|cut -d' ' -f1; }
set +e
set -C
docker run --rm --runtime runc --pull never --network none --read-only --user 10001:10001 --cap-drop ALL --security-opt no-new-privileges:true --memory 512m --pids-limit 64 "${audit_mounts[@]}" "$go_image" /chain-final-audit -protocol=v20 -auditor-commit "$auditor_commit" -auditor-source-sha256 "$auditor_tree" -auditor-binary-sha256 "$auditor_binary_sha" \
 -commit 708def1e582eb8759c753c9feb6a811cd5f5702c \
 -source-sha256 "$measured_tree" \
 -app-sha256 "$measured_app" \
 -broker-sha256 2ed16aca088b7cb90b85b643a7e9533d336ca2d43ea0af5746e44847496d9e8e \
 -converter-sha256 5f2dee5acef95c2f279b849d7af76003688a3b46a274e0d44456a37c40536f75 \
 -postgres-image "$pg_image" -runtime-image "$runtime_image" \
 -scope-sha256 "$(hash "${origins[0]}")" -pg-ledger-sha256 "$(hash "${origins[1]}")" \
 -pg-initial-sha256 "$(hash "${origins[2]}")" -pg-product-sha256 "$(hash "${origins[3]}")" \
 -runtime-state-sha256 "$(hash "${origins[4]}")" -hardware-sha256 "$(hash "${origins[5]}")" \
 -runtime-profile-sha256 "$(hash "${origins[6]}")" -source-manifest-sha256 "$(hash "${origins[7]}")" \
 > "$audit_dir/audit-reviewed.json" 2> "$audit_dir/audit-reviewed.stderr"
audit_exit=$?
set +C
set -e
printf '%s\n' "$audit_exit" > "$audit_dir/audit-exit.txt"
chmod 600 "$audit_dir/audit-reviewed.json" "$audit_dir/audit-reviewed.stderr" "$audit_dir/audit-exit.txt"
sha256sum "$audit_dir/audit-reviewed.json" "$audit_dir/audit-reviewed.stderr" "$audit_dir/audit-exit.txt" > "$audit_dir/output.sha256"
printf 'Saved independent read-only output, exit=%s, review=%s\n' "$audit_exit" "$audit_dir"
sha256sum "$audit_dir/audit-reviewed.json"
exit "$audit_exit"
