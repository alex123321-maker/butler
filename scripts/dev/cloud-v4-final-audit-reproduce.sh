#!/usr/bin/env bash
# Read-only independent audit of the closed V4 window. Zero native calls.
set -euo pipefail
umask 077
p=/srv/storage/butler/agents/scenario-shadow-v4-high-20261013/reports/shadow-execute-20261012T192502Z-2893141
s=/srv/storage/butler/agents/scenario-shadow-v4-high-20261013/snapshots/20261013-020922-884
out=${1:?new retained audit output path required}
[[ "$out" == "$p/"* && ! -e "$out" && ! -L "$out" ]] || exit 2
mounts=(--mount type=bind,src=$p/binaries/final-audit,dst=/auditor,readonly --mount type=bind,src=$s,dst=/src,readonly --mount type=bind,src=$p/runtime/observations.json,dst=/report.json,readonly --mount type=bind,src=$p/audit-inputs/pg.json,dst=/pg.json,readonly --mount type=bind,src=$p/audit-inputs/diagnostics.json,dst=/diagnostics.json,readonly --mount type=bind,src=$p/metadata/public-key.hex,dst=/metadata/public-key.hex,readonly --mount type=bind,src=$p/metadata/approval.json,dst=/metadata/approval.json,readonly)
for ordinal in 269 270 271 272 273 274 275 276 277 278 279; do mounts+=(--mount type=bind,src=$p/runtime/final-$ordinal.raw,dst=/raw/final-$ordinal.raw,readonly); done
set -o noclobber
docker run --rm --network none --read-only --cap-drop ALL --security-opt no-new-privileges:true --user 10001:10001 --memory 512m --pids-limit 64 --cpus 2 "${mounts[@]}" --entrypoint /auditor sha256:5cf287a799e6b94384bad13d16b14904c531f51ba65792237e122ce42b392f61 --report-sha256 8c675d8136419a371142fa9e90c56423e7f1a216188e7fcc6d4ad4f25884ab8d --pg-sha256 dc7689428e55575ce7023de8282da3f73192e1064845d385bd7776217d9b2f31 --diagnostics-sha256 02c7129de44b2980c1758c3061025792da5e37c6f0c26d58d781c3d9e4641ac7 --commit 925e1729923fd9865622c475893d7bffb21699e8 --source-sha256 32b592de311888907b4e90af24988e9166fb0be22ac5e92af97aa773a2489b21 --hardware-sha256 d3632dff9b14bb6cb710b9f909dfaa8fd7ea6ff313babd28db392ecaa7731c3a > "$out"
[[ "$(sha256sum "$out" | cut -d' ' -f1)" == 431e4371e7975604971019930c1f912eb9c610fa7ad316738cb50601ab8507cb ]]
[[ "$(wc -c < "$out")" == 15764 ]]
sha256sum "$out"
