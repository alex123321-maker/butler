#!/usr/bin/env bash
# Build a separately frozen read-only auditor; no provider, credential or window.
set -euo pipefail
root=${1:?isolated auditor root required}; release=${2:?exact clean commit required}
[[ "$root" == /srv/storage/butler/agents/scenario-v5-readonly-audit-fix-20261013 && "$release" =~ ^[a-f0-9]{40}$ ]] || exit 2
s=$(readlink -f "$root/current")
[[ "$s" == "$root/snapshots/"* ]] || exit 2
umask 077
p="$root/reports/audit-prepare-$(date -u +%Y%m%dT%H%M%SZ)-$$"
mkdir -p "$p/bin" /srv/storage/butler/.cache/go-tmp
manifest() { (cd "$s"; [[ -z "$(find . ! -type f ! -type d -print -quit)" ]]; find . -type f -print0 | sort -z | xargs -0 sha256sum); }
manifest > "$p/source.sha256"
printf '%s\n' "$release" > "$p/source-commit.txt"
image=sha256:5cf287a799e6b94384bad13d16b14904c531f51ba65792237e122ce42b392f61
[[ "$(docker image inspect golang:1.27.2-bookworm --format '{{.Id}}')" == "$image" ]]
docker run --rm --runtime runc --pull never --network none --user "$(id -u):$(id -g)" --cap-drop ALL --security-opt no-new-privileges:true --memory 1536m --pids-limit 160 --cpus 2 \
  --mount type=bind,src=$s,dst=/src,readonly --mount type=bind,src=$p/bin,dst=/proof --mount type=bind,src=/srv/storage/butler/.cache,dst=/cache --workdir /src \
  --env GOPROXY=off --env GOSUMDB=off --env GOMAXPROCS=2 --env GOFLAGS=-p=2 --env GOCACHE=/cache/go-race --env GOMODCACHE=/cache/go-mod --env GOTMPDIR=/cache/go-tmp \
  "$image" bash -eu -c '
go test -race -json -count=1 -run "^TestCloudV5(WindowAudit|PGCorrelation)" ./internal/acceptance > /proof/acceptance.jsonl
go test -race -json -count=1 ./scripts/dev/cloud-v5-final-audit > /proof/auditor.jsonl
go vet ./internal/acceptance ./scripts/dev/cloud-v5-final-audit
go build -trimpath -o /proof/final-audit ./scripts/dev/cloud-v5-final-audit
chmod 0555 /proof/final-audit
' > "$p/build-vet.log" 2>&1
! grep -Eq '"Action":"(fail|skip)"' "$p/bin/acceptance.jsonl" "$p/bin/auditor.jsonl"
required=(TestCloudV5WindowAuditRetainsDistinctHistoricalRequestsInOneRun TestCloudV5WindowAuditRetainsDistinctHistoricalRequestsInOneRun/changed_charge TestCloudV5WindowAuditRetainsDistinctHistoricalRequestsInOneRun/omitted_charge TestCloudV5WindowAuditRetainsDistinctHistoricalRequestsInOneRun/filled_unknown TestCloudV5WindowAuditRetainsDistinctHistoricalRequestsInOneRun/new_context_collides_with_old_run)
for test_name in "${required[@]}"; do
  [[ "$(awk -v needle="\"Test\":\"$test_name\"" 'index($0,"\"Action\":\"pass\"") && index($0,needle) { n++ } END { print n+0 }' "$p/bin/acceptance.jsonl")" == 1 ]]
done
manifest | cmp -s "$p/source.sha256" -
sha256sum "$p/bin/"* "$p/source.sha256" "$p/build-vet.log" > "$p/evidence.sha256"
printf '{"status":"readonly_auditor_preparation_passed","native_calls":0,"credentials_mounted":false,"native_window_reopened":false}\n' > "$p/summary.json"
printf '%s\n' "$p" > "$root/prepared-proof"
printf '%s\n' "$s" > "$root/prepared-source"
printf '%s\n' "$release" > "$root/prepared-release"
printf 'Read-only auditor proof: %s\n' "$p"
cat "$p/evidence.sha256"
