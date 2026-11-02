#!/usr/bin/env bash
# Reconstruct the actual Core source, then apply only this browser outcome slice.
set -euo pipefail
root=${1:?owned browser-owner test root}
[[ "$root" =~ ^/srv/storage/butler/agents/server-browser-owners-[0-9]{8}$ ]] || exit 2
core_base=/srv/storage/butler/agents/browser-authority-isolation-20261028/build-cc4d7701-59569a98/src
browser_base=/srv/storage/butler/agents/server-browser-get-form-20261028/snapshots/20261028-204708-452
core_patch=$(sha256sum "$root/browser-owner-core.patch" | cut -d ' ' -f 1)
browser_patch=$(sha256sum "$root/browser-owner-browser.patch" | cut -d ' ' -f 1)
bundle="$root/build-${core_patch:0:12}"
scratch="/tmp/butler-owner-build-${core_patch:0:12}"
test ! -e "$bundle";test ! -e "$scratch"
mkdir -p "$bundle/bin" "$bundle/reports" "$scratch/core" "$scratch/browser"
phase(){
 local name=$1 before after code;shift
 read -r before _ < /proc/uptime
 if "$@" > "$bundle/reports/$name.log" 2>&1;then code=0;else code=$?;fi
 read -r after _ < /proc/uptime
 awk -v phase="$name" -v a="$before" -v b="$after" -v code="$code" 'BEGIN{printf "{\"phase\":\"%s\",\"duration_ms\":%.0f,\"exit_code\":%d}\n",phase,(b-a)*1000,code}' >> "$bundle/reports/timings.jsonl"
 if ((code));then tail -60 "$bundle/reports/$name.log";return "$code";fi
 printf '%s PASS\n' "$name"
}
reconstruct(){
 cp -a "$core_base/." "$scratch/core/" || return
 cp -a "$browser_base/." "$scratch/browser/" || return
 git -C "$scratch/core" apply --check "$root/browser-owner-core.patch" || return
 git -C "$scratch/core" apply "$root/browser-owner-core.patch" || return
 git -C "$scratch/browser" apply --check "$root/browser-owner-browser.patch" || return
 git -C "$scratch/browser" apply "$root/browser-owner-browser.patch" || return
 test -s "$scratch/core/internal/actionauth/owner_outcome.go" || return
 grep -q 'EffectReviews:.*outcomeKernel' "$scratch/core/apps/orchestrator/internal/app/bootstrap.go" || return
}
phase source_reconstruction reconstruct
go_image=$(docker image inspect --format '{{.Id}}' golang:1.27.2-bookworm)
args=(--rm --user "$(id -u):$(id -g)" --memory 3g --cpus 2 --pids-limit 256 --security-opt no-new-privileges:true --network none
 -v "$root/.cache:/cache" -v /srv/storage/butler/.cache/go-mod:/modules:ro -v "$bundle/bin:/out" -w /src
 -e GOCACHE=/cache/go-build -e GOMODCACHE=/modules -e GOTMPDIR=/cache/go-tmp -e GOPROXY=off -e GOSUMDB=off -e CGO_ENABLED=0)
phase core_compile docker run "${args[@]}" -v "$scratch/core:/src:ro" "$go_image" go build -o /out/orchestrator ./apps/orchestrator
phase broker_compile docker run "${args[@]}" -v "$scratch/browser:/src:ro" "$go_image" go build -o /out/broker ./apps/tool-broker
phase browser_compile docker run "${args[@]}" -v "$scratch/browser:/src:ro" "$go_image" go build -o /out/tool-browser ./apps/tool-browser
sha256sum "$bundle/bin/"* > "$bundle/reports/binaries.sha256"
jq -n --arg core_base "$core_base" --arg browser_base "$browser_base" --arg core_patch "$core_patch" --arg browser_patch "$browser_patch" --arg scratch "$scratch" --arg go "$go_image" \
 '{core_base:$core_base,browser_base:$browser_base,core_patch_sha256:$core_patch,browser_patch_sha256:$browser_patch,source:$scratch,go_image:$go,new_migrations:[105],native_calls:0,live_changes:0}' > "$bundle/reports/source-manifest.json"
printf 'Minimal source build PASS; bundle=%s\n' "$bundle"
