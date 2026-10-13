#!/usr/bin/env bash
# After the parent explicitly confirms the actual native process is complete.
# Copies only thirteen content-free leaves; no authentication or native invocation.
set -euo pipefail
umask 077
root=/srv/storage/butler/agents/scenario-release-chain-v7b-high-20261013
src="$root/snapshots/20261013-201448-105"
proof=${1:?completed actual proof path required}
auditor_src=${2:?separate frozen public auditor snapshot required}
auditor_bin=${3:?separate readonly built audit binary required}
auditor_commit=${4:?separate exact auditor commit required}
auditor_tree=${5:?separate independently retained auditor TreeHash required}
auditor_binary_sha=${6:?separate independently retained audit executable SHA required}
[[ "$auditor_src" =~ ^/srv/storage/butler/agents/[a-z0-9-]+/snapshots/[0-9-]+$ && "$auditor_bin" =~ ^/srv/storage/butler/agents/[a-z0-9-]+/reports/[a-zA-Z0-9-]+/bin/chain-final-audit$ ]]
[[ "$auditor_commit" =~ ^[a-f0-9]{40}$ && "$auditor_tree" =~ ^[a-f0-9]{64}$ && "$auditor_binary_sha" =~ ^[a-f0-9]{64}$ ]]
[[ "$proof" =~ ^/srv/storage/butler/agents/scenario-release-chain-v7b-high-20261013/reports/release-chain-process-[0-9]{8}T[0-9]{6}Z-[0-9]+$ ]]
review=/srv/storage/butler/reviews/issue33-chain-v7-20261013
if [[ ! -e "$review" && ! -L "$review" ]]; then mkdir -m 700 "$review"; fi
[[ -d "$review" && ! -L "$review" && "$(stat -c %a "$review")" == 700 ]]
audit_dir="$review/v7b-audit-6d874f6-$(date -u +%Y%m%dT%H%M%SZ)-$$"
[[ ! -e "$audit_dir" && ! -L "$audit_dir" ]]
mkdir -m 700 "$audit_dir"
stage="$audit_dir/inputs"
mkdir -m 700 "$stage"
go_image=sha256:5cf287a799e6b94384bad13d16b14904c531f51ba65792237e122ce42b392f61
pg_image=sha256:ac08538c6f8b9904c33c8224c5e5706dbe760aca29db1d096972b4052c22a75d
runtime_image=sha256:ce64758a109eb420d874a118f87920e625e12d3634e03b4a5573fd9f6e5d3507
leaves=(scope pg-ledger pg-initial pg-product runtime-state hardware runtime-profile source-manifest source-commit quality prerequisite expense-exception timeout-waiver)
origins=("$proof/scope.json" "$proof/pg-ledger-witness.json" "$proof/pg-initial-witness.json" "$proof/pg-product-witness.json" "$proof/runtime-state.json" "$proof/hardware.json" "$proof/runtime-profile.json" "$proof/source-manifest.sha256" "$proof/source-commit.txt" "$src/deploy/acceptance/cloud-shadow-v5-final-audit-20261013.json" "$src/deploy/acceptance/release-chain-v7-high-prerequisite-20261013.json" "$src/deploy/acceptance/release-chain-v6-user-expense-exception-20261013.json" "$src/deploy/acceptance/release-chain-v7-user-policy-amendment-20261013.json")
names=(scope.json pg-ledger.json pg-initial.json pg-product.json runtime-state.json hardware.json runtime-profile.json source-manifest.sha256 source-commit.txt quality.json prerequisite.json expense-exception.json timeout-waiver.json)
copy_mounts=(--volume "$stage:/out")
audit_mounts=(--volume "$src:/src:ro" --volume "$proof/bin/app.test:/app.test:ro" --volume "$proof/bin/tool-broker:/tool-broker:ro" --volume "$proof/bin/tool-converter:/tool-converter:ro" --volume "$auditor_bin:/chain-final-audit:ro" --volume "$auditor_src:/auditor-src:ro")
for i in "${!origins[@]}"; do
 [[ -f "${origins[$i]}" && ! -L "${origins[$i]}" ]]
 copy_mounts+=(--volume "${origins[$i]}:/in-${names[$i]}:ro")
 audit_mounts+=(--volume "$stage/${names[$i]}:/${names[$i]}:ro")
done
# Disposable bounded copying helper sees only named safe leaves and its new output.
docker run --rm --runtime runc --pull never --network none --read-only --security-opt no-new-privileges:true --memory 32m --pids-limit 16 "${copy_mounts[@]}" "$runtime_image" sh -eu -c '
 umask 077
 for name in scope.json pg-ledger.json pg-initial.json pg-product.json runtime-state.json hardware.json runtime-profile.json source-manifest.sha256 source-commit.txt quality.json prerequisite.json expense-exception.json timeout-waiver.json; do
  test -f "/in-$name"; test ! -L "/in-$name"; test ! -e "/out/$name"
  cp "/in-$name" "/out/$name"; chmod 600 "/out/$name"; chown 10001:10001 "/out/$name"
 done
 chown 10001:10001 /out; chmod 700 /out
'
for b in app.test tool-broker tool-converter; do [[ "$(stat -c %a "$proof/bin/$b")" == 555 ]]; done
[[ "$(sha256sum "$proof/bin/app.test"|cut -d' ' -f1)" == 97f6ad4ce71fc067f48555cadcba1ef68a92d98878d0a2694ff5484e6d731c51 ]]
[[ "$(sha256sum "$proof/bin/tool-broker"|cut -d' ' -f1)" == caccde9e733875c1d7f32a734f184181c7b1184637a681f29d6bca9d283f91d5 ]]
[[ "$(sha256sum "$proof/bin/tool-converter"|cut -d' ' -f1)" == d62c672bac20820926e17774b01ac326d0ca9a2af52f1317e76280681e992fa0 ]]
[[ "$(stat -c %a "$auditor_bin")" == 555 && "$(sha256sum "$auditor_bin"|cut -d' ' -f1)" == "$auditor_binary_sha" ]]
hash() { sha256sum "$1"|cut -d' ' -f1; }
set +e
set -C
docker run --rm --runtime runc --pull never --network none --read-only --user 10001:10001 --cap-drop ALL --security-opt no-new-privileges:true --memory 512m --pids-limit 64 "${audit_mounts[@]}" "$go_image" /chain-final-audit -protocol=v7 -auditor-commit "$auditor_commit" -auditor-source-sha256 "$auditor_tree" -auditor-binary-sha256 "$auditor_binary_sha" \
 -commit 6d874f6625af7f21ac9758dbea9b287b4d5252ea \
 -source-sha256 0586a361fa246066244ffdcdce3269a2ce600ebf2bf35625383d551f0afd0e3c \
 -app-sha256 97f6ad4ce71fc067f48555cadcba1ef68a92d98878d0a2694ff5484e6d731c51 \
 -broker-sha256 caccde9e733875c1d7f32a734f184181c7b1184637a681f29d6bca9d283f91d5 \
 -converter-sha256 d62c672bac20820926e17774b01ac326d0ca9a2af52f1317e76280681e992fa0 \
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
