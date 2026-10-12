#!/usr/bin/env bash
# Reproduce the frozen independent compatibility audit. Zero native calls.
set -euo pipefail
umask 077
p=/srv/storage/butler/agents/scenario-schema-dialect-high-20261013/reports/schema-dialect-execute-20261012T204819Z-3148336
s=/srv/storage/butler/agents/scenario-schema-dialect-high-20261013/snapshots/20261013-034124-672
b=/srv/storage/butler/agents/scenario-schema-dialect-high-20261013/reports/schema-dialect-prepare-20261012T204128Z-3126538/binaries/orchestrator.test
out=${1:?new retained audit output path required}
[[ "$out" == "$p/"* && ! -e "$out" && ! -L "$out" && ! -e "$out.log" && ! -L "$out.log" ]] || exit 2
[[ "$(sha256sum "$b" | cut -d' ' -f1)" == 93b8876a9cba071e5aff4d1771e93932d78c7164b2dfdaf63de812879b9b406f ]]
mounts=(--mount type=bind,src=$b,dst=/orchestrator.test,readonly --mount type=bind,src=$s,dst=/src,readonly --mount type=bind,src=$p/runtime/diagnostic.json,dst=/report.json,readonly --mount type=bind,src=$p/audit-inputs/pg.json,dst=/pg.json,readonly)
for ordinal in 280 281 282 283; do mounts+=(--mount type=bind,src=$p/runtime/final-$ordinal.raw,dst=/raw/final-$ordinal.raw,readonly); done
set -o noclobber
docker run --rm --network none --read-only --cap-drop ALL --security-opt no-new-privileges:true --user 10001:10001 --memory 512m --pids-limit 64 --cpus 2 "${mounts[@]}" --workdir /src --env BUTLER_SCHEMA_DIALECT_AUDIT=1 --env BUTLER_SCHEMA_DIALECT_REPORT_SHA=05419e82a3c9b32a1250c4c67f5c1b127aa6919a6f6c043f058b89ca80f5330b --env BUTLER_SCHEMA_DIALECT_PG_SHA=5ad40ecd6a33e7643a7c38e02422cdcc47e445973bdfc2386d50d80574343b48 --env BUTLER_SCHEMA_DIALECT_SOURCE_SHA=890aa41435d36e418788a2d01285e56e81b2c025b9617bb80ef00f5a7adeeef4 sha256:5cf287a799e6b94384bad13d16b14904c531f51ba65792237e122ce42b392f61 /orchestrator.test -test.v -test.timeout=30s -test.run='^TestScenarioSchemaDialectFinalAudit$' > "$out.log"
[[ "$(grep -c '^schema_dialect_final_audit=' "$out.log")" == 1 ]]
[[ "$(grep -c '^--- PASS: TestScenarioSchemaDialectFinalAudit ' "$out.log")" == 1 ]]
sed -n 's/^schema_dialect_final_audit=//p' "$out.log" > "$out"
[[ "$(sha256sum "$out" | cut -d' ' -f1)" == d3877d3bc9a2bdffca14c040e4aa8c397e3b5da806c0ff0e3a33a47221fd7cfe ]]
[[ "$(wc -c < "$out")" == 5832 ]]
sha256sum "$out"
