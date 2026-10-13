#!/usr/bin/env bash
# After the parent explicitly confirms the actual native process is complete.
# Copies fourteen individually named safe leaves plus only individually named own-tool
# proposals from the safe scope metadata. Never mount the source private directory.
set -euo pipefail
umask 077
root=/srv/storage/butler/agents/scenario-release-chain-v13-high-20261013
src="$root/snapshots/20261014-015432-120"
proof=${1:?completed actual proof path required}
auditor_src=${2:?separate frozen public auditor snapshot required}
auditor_bin=${3:?separate readonly built audit binary required}
auditor_commit=${4:?separate exact auditor commit required}
auditor_tree=${5:?separate independently retained auditor TreeHash required}
auditor_binary_sha=${6:?separate independently retained audit executable SHA required}
measured_tree=${7:?actual independently admitted09cf source TreeHash required}
measured_app=${8:?actual independently admitted09cf app.test SHA required}
[[ "$measured_tree" =~ ^[a-f0-9]{64}$ && "$measured_app" =~ ^[a-f0-9]{64}$ ]]
[[ "$auditor_src" =~ ^/srv/storage/butler/agents/[a-z0-9-]+/snapshots/[0-9-]+$ && "$auditor_bin" =~ ^/srv/storage/butler/agents/[a-z0-9-]+/reports/[a-zA-Z0-9-]+/bin/chain-final-audit$ ]]
[[ "$auditor_commit" =~ ^[a-f0-9]{40}$ && "$auditor_tree" =~ ^[a-f0-9]{64}$ && "$auditor_binary_sha" =~ ^[a-f0-9]{64}$ ]]
[[ "$proof" =~ ^/srv/storage/butler/agents/scenario-release-chain-v13-high-20261013/reports/release-chain-process-[0-9]{8}T[0-9]{6}Z-[0-9]+$ ]]
review=/srv/storage/butler/reviews/issue33-chain-v13-20261013
if [[ ! -e "$review" && ! -L "$review" ]]; then mkdir -m 700 "$review"; fi
[[ -d "$review" && ! -L "$review" && "$(stat -c %a "$review")" == 700 ]]
audit_dir="$review/v13-audit-09cf34c-$(date -u +%Y%m%dT%H%M%SZ)-$$"
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
origins=("$proof/scope.json" "$proof/pg-ledger-witness.json" "$proof/pg-initial-witness.json" "$proof/pg-product-witness.json" "$proof/runtime-state.json" "$proof/hardware.json" "$proof/runtime-profile.json" "$proof/source-manifest.sha256" "$proof/source-commit.txt" "$src/deploy/acceptance/cloud-shadow-v5-final-audit-20261013.json" "$src/deploy/acceptance/release-chain-v13-high-prerequisite-20261013.json" "$src/deploy/acceptance/release-chain-v6-user-expense-exception-20261013.json" "$src/deploy/acceptance/release-chain-v13-user-policy-amendment-20261013.json" "$src/deploy/acceptance/release-chain-v11-user-expense-exception-20261013.json")
names=(scope.json pg-ledger.json pg-initial.json pg-product.json runtime-state.json hardware.json runtime-profile.json source-manifest.sha256 source-commit.txt quality.json prerequisite.json expense-exception.json timeout-waiver.json historical557-expense-exception.json)
copy_mounts=(--volume "$stage:/out" --volume "$proposal_stage:/proposalout" --volume "$empty_proposals:/emptyout")
audit_mounts=(--volume "$src:/src:ro" --volume "$proof/bin/app.test:/app.test:ro" --volume "$proof/bin/tool-broker:/tool-broker:ro" --volume "$proof/bin/tool-converter:/tool-converter:ro" --volume "$auditor_bin:/chain-final-audit:ro" --volume "$auditor_src:/auditor-src:ro" --volume "$empty_proposals:/proposals:ro")
for i in "${!origins[@]}"; do
 [[ -f "${origins[$i]}" && ! -L "${origins[$i]}" ]]
 copy_mounts+=(--volume "${origins[$i]}:/in-${names[$i]}:ro")
 audit_mounts+=(--volume "$stage/${names[$i]}:/${names[$i]}:ro")
done
# Read only public-safe scope metadata to construct the finite named mounts.
python3 - "$proof/scope.json" > "$audit_dir/proposal-inventory.tsv" <<'PY'
import sys,json,re
v=json.load(open(sys.argv[1]))
assert v['window_id']=='issue33-high-release-chain-v13-20261013-baseline575'
items=v['proposal_observations'];assert isinstance(items,list) and len(items)<=120
previous=575
for m in items:
 ordinal=m['ordinal'];assert isinstance(ordinal,int) and previous<ordinal<=695
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
docker run --rm --runtime runc --pull never --network none --read-only --user 10001:10001 --cap-drop ALL --security-opt no-new-privileges:true --memory 512m --pids-limit 64 "${audit_mounts[@]}" "$go_image" /chain-final-audit -protocol=v13 -auditor-commit "$auditor_commit" -auditor-source-sha256 "$auditor_tree" -auditor-binary-sha256 "$auditor_binary_sha" \
 -commit 09cf34c05ab1fc1840dc73f1c9e1ecb2766b0faf \
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
