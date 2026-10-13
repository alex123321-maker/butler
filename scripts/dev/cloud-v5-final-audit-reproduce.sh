#!/usr/bin/env bash
# Reproduce independent read-only V5 audit; paid native window remains sealed.
set -euo pipefail
umask 077
p=/srv/storage/butler/agents/scenario-shadow-v5-high-20261013/reports/shadow-execute-20261012T213414Z-3290402
s=/srv/storage/butler/agents/scenario-shadow-v5-high-20261013/snapshots/20261013-041550-328
b=/srv/storage/butler/agents/scenario-v5-readonly-audit-fix-20261013/reports/audit-prepare-20261013T060407Z-613262/bin/final-audit
out=${1:?new exclusive audit output path required}
[[ "$out" == "$p/"* && ! -e "$out" && ! -L "$out" ]]
[[ "$(stat -c '%a:%s' "$p/audit-final.json")" == 600:0 ]]
[[ "$(sha256sum "$b" | cut -d' ' -f1)" == 8f3c4b28ad656c71a584ba739362f3f612cddbd65dff4047180259891f01f937 ]]
mounts=(--mount type=bind,src=$b,dst=/auditor,readonly --mount type=bind,src=$s,dst=/src,readonly --mount type=bind,src=$p/runtime/observations.json,dst=/report.json,readonly --mount type=bind,src=$p/audit-inputs/pg.json,dst=/pg.json,readonly --mount type=bind,src=$p/audit-inputs/diagnostics.json,dst=/diagnostics.json,readonly)
for leaf in public-key.hex approval.json history.json compatibility.json; do mounts+=(--mount type=bind,src=$p/metadata/$leaf,dst=/metadata/$leaf,readonly); done
for ordinal in $(seq 284 535); do mounts+=(--mount type=bind,src=$p/runtime/final-$ordinal.raw,dst=/raw/final-$ordinal.raw,readonly); done
set -o noclobber
docker run --rm --network none --read-only --cap-drop ALL --security-opt no-new-privileges:true --user 10001:10001 --memory 512m --pids-limit 64 --cpus 2 "${mounts[@]}" --entrypoint /auditor sha256:5cf287a799e6b94384bad13d16b14904c531f51ba65792237e122ce42b392f61 --report-sha256 a5d4c7991143c2c75d2fc7663187d82acedb7092e6cec91eb67d246db016b830 --pg-sha256 cf1a2479041a972b4fe1f6b9097c058880d28da2328e806b0bac993389a8e1f8 --diagnostics-sha256 18c62eb81c6e3595189586a4ad4593e55a9dbe8469f7a34d607d3b44cda28f1a --commit 3c4089f35c0bdf33bf9b2c6328ba395c519cecf4 --source-sha256 61c8e729bb53f69439c3d69e9c0979e62e9a601ab3906a413f9434b0631db374 --hardware-sha256 d3632dff9b14bb6cb710b9f909dfaa8fd7ea6ff313babd28db392ecaa7731c3a --history-file-sha256 1299b074c6b317727495fdb2502456eb18829e4e05df9fae75a15a434215b56d --history-sha256 fb248690c6daa51c65a82a7b67f155511b369d3aadf6028338561d02fe4e951b --historical-known-total-tokens 289855 --historical-unknown-total-attempts 46 --compatibility-sha256 d3877d3bc9a2bdffca14c040e4aa8c397e3b5da806c0ff0e3a33a47221fd7cfe > "$out"
[[ "$(stat -c %a "$out")" == 600 ]]
sha256sum "$out"
[[ "$(sha256sum "$out" | cut -d' ' -f1)" == f8248d19ec2a0c222b39d46fbc46c0d0cf4ccc3c1361c89965b597eedbb21cab ]]
[[ "$(wc -c < "$out")" == 144579 ]]
