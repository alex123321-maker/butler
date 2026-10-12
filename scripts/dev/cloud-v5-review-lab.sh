#!/usr/bin/env bash
# Candidate API checks and one-use external public-corpus seal. Zero inference.
set -euo pipefail
root=${1:?fresh V5 root required}; phase=${2:-build}; review_sha=${3:-UNSET}
[[ "$root" == /srv/storage/butler/agents/scenario-shadow-v5-high-20261013 && "$phase" =~ ^(build|seal)$ ]] || exit 2
source_dir=$(readlink -f "$root/current")
[[ "$source_dir" == "$root/snapshots/"* ]] || exit 2
umask 077
proof="$root/reports/review-$phase-$(date -u +%Y%m%dT%H%M%SZ)-$$"
mkdir -p "$proof/binaries" "$root/.cache/go-tmp"
chmod 755 "$proof/binaries"
export DOCKER_CONFIG="$root/.cache/docker"
image=$(docker image inspect golang:1.27.2-bookworm --format '{{.Id}}')
(cd "$source_dir"; find . -type f -print0 | LC_ALL=C sort -z | xargs -0 sha256sum) > "$proof/source.sha256"
printf 'Private zero-inference V5 review proof: %s\n' "$proof"
docker run --rm --runtime runc --network none --user "$(id -u):$(id -g)" --memory 1536m --pids-limit 128 --cpus 2 --cap-drop ALL --security-opt no-new-privileges:true \
 --volume "$source_dir:/src:ro" --volume /srv/storage/butler/.cache:/cache --volume "$root/.cache/go-tmp:/tmp" --volume "$proof/binaries:/binaries" --workdir /src \
 --env GOCACHE=/cache/go-race --env GOMODCACHE=/cache/go-mod --env GOTMPDIR=/tmp --env TMPDIR=/tmp \
 "$image" bash -eu -c '
go test -json -race -p 1 -count=1 -run "^TestCloud(V5|FreshV5|SchemaDialectDiagnostic)" ./internal/acceptance
go test -json -race -p 1 -count=1 ./internal/transport/antigravity
go test -json -race -p 1 -count=1 -run "^TestCloudV5" ./cmd/cloud-observe
go build -o /binaries/review-seal ./scripts/dev/cloud-v5-review
go vet ./internal/acceptance ./internal/transport/antigravity ./cmd/cloud-observe ./scripts/dev/cloud-v5-review
chmod 555 /binaries/*
' > "$proof/tests.jsonl" 2> "$proof/diagnostics.log"
! grep -Eq '"Action":"(skip|fail)"' "$proof/tests.jsonl"
sha256sum "$proof/binaries/"* "$proof/tests.jsonl" "$proof/source.sha256" > "$proof/evidence.sha256"
[[ "$phase" == seal ]] || exit 0
[[ "$review_sha" =~ ^[a-f0-9]{64}$ ]] || exit 2
review=/srv/storage/butler/reviews/issue33-20261013-v5-corrected-sealed
[[ ! -e "$review" && ! -L "$review" ]] || exit 2
mkdir -m 755 "$review"
cp "$source_dir/docs/ai/ru-cloud-fresh-v5-independent-review-amended-20261013.md" "$review/independent-review.md"
docker run --rm --network none --read-only --user 0:0 --cap-drop ALL --cap-add CHOWN --cap-add FOWNER --security-opt no-new-privileges:true \
 --volume "$review:/review" alpine:3.22 sh -eu -c 'chown -R 10001:10001 /review; chmod 600 /review/independent-review.md; chmod 700 /review'
docker run --rm --runtime runc --network none --read-only --user 10001:10001 --cap-drop ALL --security-opt no-new-privileges:true --memory 512m --pids-limit 64 \
 --volume "$source_dir:/src:ro" --volume "$proof/binaries/review-seal:/seal:ro" --volume "$review:/review" \
 "$image" /seal \
 -corpus-sha256 03caf934ff0a420d8e7522225ddb2ccad732fbd43ae113389433e57113f0d171 \
 -review-sha256 "$review_sha" \
 -semantic-sha256 UNSET \
 -schema-sha256 b5362cee519809034f93c662d9d386534ca95ed6fd2c4bff2276b4a97dc351f5 \
 -policy-sha256 e77dea3dcb284c30e16f23f76e4a782cdb599316c7f0d1cc831e0b0716469b8b \
 -criteria-sha256 71565f56ab0be83b6d99f268ca82acc6852eab837421ba1897653de3b07b6a19 \
 -resources-sha256 e79bdeeb7bef6ad8ca1bc85889215f96901366b6698b49a1c9d9606e823ca7fa > "$proof/seal.json"
cat "$proof/seal.json"
