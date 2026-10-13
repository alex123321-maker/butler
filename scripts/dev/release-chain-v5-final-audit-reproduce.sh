#!/usr/bin/env bash
# Reproduce the exact failed-chain audit, never a provider/tool/native invocation.
set -euo pipefail
umask 077
set -C
out=${1:?new exclusive safe audit output path required}
[[ "$out" =~ ^/srv/storage/butler/(reviews|agents)/[a-zA-Z0-9_./-]+\.json$ && "$out" != *..* ]] || exit 2
[[ ! -e "$out" && ! -L "$out" && ! -e "$out.stderr" && ! -L "$out.stderr" ]] || exit 2
root=/srv/storage/butler/agents/scenario-release-chain-v5-high-20261013
src=$root/snapshots/20261013-143513-149
proof=$root/reports/release-chain-process-20261013T075645Z-968057
stage=/srv/storage/butler/reviews/issue33-chain-v5-20261013/audit-inputs-process-968057
commit=afa50b225b1341d2ff8a553188a69f6b6b2ac633
tree=862fafb7a38a372a288a363fd73260b1efa851837080bdafecf90545d75cf26b
go_image=sha256:5cf287a799e6b94384bad13d16b14904c531f51ba65792237e122ce42b392f61
pg_image=sha256:ac08538c6f8b9904c33c8224c5e5706dbe760aca29db1d096972b4052c22a75d
runtime_image=sha256:ce64758a109eb420d874a118f87920e625e12d3634e03b4a5573fd9f6e5d3507
app_sha=383729b5493e2038f95fa23924feac880552531537d06f072c3eba8f77768b00
broker_sha=36cfde3139d843673f9df60ba1848dcd58eee5a6fff5b16d66da463541cbfb83
converter_sha=8fb12abff9627f13f8d306cf85e138ed0c741559fb5eb31280f8b30f74a00e4e
declare -A pins=(
  [scope.json]=e1101e9f1b8b435e9f420aeea7d1507326cd0f0ee3bc80311c417792c899f709
  [pg-ledger.json]=778b8cd9e204ba074b98ac677d3867522728d25e3f7566473adff6f312160c05
  [pg-initial.json]=31a5e87d90569a8a950e76206b1825c818e9ebddec9be2c54e71111da05f78a0
  [pg-product.json]=82a3608fa9e86014140da6ad2e5bb769cfc1747cdab2c98027ccfdfb64557181
  [runtime-state.json]=4fb2e8ba23eaaf64be7b9967d265fddf2e0ccea07363bbfecfc38d5163716aca
  [hardware.json]=8c3c3f7440e8b2459acafffb8c438fa0177c5fe0871ec59a612d89cd8ae6463f
  [runtime-profile.json]=63a59f0f05d957b142dabef77c88dc056d596d98af774dc6d604ea7039d8db01
  [source-manifest.sha256]=ac9eb5ed617f28421582e605b2e260c5f6dd2eb78adc2c18d72539e877076277
  [quality.json]=f8248d19ec2a0c222b39d46fbc46c0d0cf4ccc3c1361c89965b597eedbb21cab
  [prerequisite.json]=9480184b2f5586d8f9826b8db328c52a845ee812bd48afbf1432ca7f30bfd5d3
)
pins[source-commit.txt]=$(printf '%s\n' "$commit" | sha256sum | cut -d' ' -f1)
declare -a mounts=(--volume "$src:/src:ro")
declare -a checked_leaves=()
for leaf in "${!pins[@]}"; do
  checked_leaves+=("$leaf" "${pins[$leaf]}")
  mounts+=(--volume "$stage/$leaf:/$leaf:ro")
done
# The safe-copy directory belongs to10001:0700. The host operator cannot inspect
# its leaves directly. This read-only metadata helper sees only those11 safe
# copies; the final checker still receives individually named mounts.
docker run --rm --pull never --network none --read-only --user 10001:10001 --cap-drop ALL --security-opt no-new-privileges:true \
  --memory 64m --pids-limit 16 --cpus 1 --volume "$stage:/safe-inputs:ro" --entrypoint sh "$runtime_image" -eu -c '
    while test "$#" -gt 0; do
      leaf=$1; expected=$2; shift 2
      test -f "/safe-inputs/$leaf"; test ! -L "/safe-inputs/$leaf"
      test "$(stat -c "%u:%a" "/safe-inputs/$leaf")" = 10001:600
      test "$(sha256sum "/safe-inputs/$leaf" | cut -d" " -f1)" = "$expected"
    done
  ' sh "${checked_leaves[@]}"
for pair in "app.test:$app_sha" "tool-broker:$broker_sha" "tool-converter:$converter_sha" 'chain-final-audit:0943723a8c6e67cdd1f499d6618461a724fbb01d12c236a2711537074bee4a4e'; do
  binary=${pair%%:*}; expected=${pair#*:}
  test -f "$proof/bin/$binary"; test ! -L "$proof/bin/$binary"
  test "$(stat -c %a "$proof/bin/$binary")" = 555
  test "$(sha256sum "$proof/bin/$binary" | cut -d' ' -f1)" = "$expected"
  mounts+=(--volume "$proof/bin/$binary:/$binary:ro")
done
set +e
docker run --rm --pull never --network none --read-only --user 10001:10001 --cap-drop ALL --security-opt no-new-privileges:true \
  --memory 512m --pids-limit 64 --cpus 1 "${mounts[@]}" --entrypoint /chain-final-audit "$go_image" \
  -commit "$commit" -source-sha256 "$tree" -app-sha256 "$app_sha" -broker-sha256 "$broker_sha" -converter-sha256 "$converter_sha" \
  -postgres-image "$pg_image" -runtime-image "$runtime_image" \
  -scope-sha256 "${pins[scope.json]}" -pg-ledger-sha256 "${pins[pg-ledger.json]}" -pg-initial-sha256 "${pins[pg-initial.json]}" -pg-product-sha256 "${pins[pg-product.json]}" \
  -runtime-state-sha256 "${pins[runtime-state.json]}" -hardware-sha256 "${pins[hardware.json]}" -runtime-profile-sha256 "${pins[runtime-profile.json]}" -source-manifest-sha256 "${pins[source-manifest.sha256]}" > "$out" 2> "$out.stderr"
audit_exit=$?
set -e
[[ "$audit_exit" == 1 && "$(stat -c %s "$out")" == 2634 ]]
[[ "$(sha256sum "$out" | cut -d' ' -f1)" == 592d27ee34178a9b336eb0f9cce4784fac1ba214b22f05e1ad0b247a48f97c04 ]]
printf 'PASS: exact read-only reproduction of the sealed NO_GO; native calls zero.\n'
