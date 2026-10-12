#!/usr/bin/env bash
# One-use independently reviewed public corpus seal; no provider/auth/native I/O.
set -euo pipefail
root=${1:?fresh shadow root required}; phase=${2:-build}
[[ "$root" == /srv/storage/butler/agents/scenario-shadow-v4-high-20261013 && "$phase" =~ ^(build|seal)$ ]] || exit 2
source_dir=$(readlink -f "$root/current")
[[ "$source_dir" == "$root/snapshots/"* ]] || exit 2
umask 077
proof="$root/reports/review-$phase-$(date -u +%Y%m%dT%H%M%SZ)-$$"
mkdir -p "$proof/binaries" "$root/.cache/go-tmp"
chmod 755 "$proof/binaries"
export DOCKER_CONFIG="$root/.cache/docker"
image=$(docker image inspect golang:1.27.2-bookworm --format '{{.Id}}')
(cd "$source_dir"; find . -type f -print0 | LC_ALL=C sort -z | xargs -0 sha256sum) > "$proof/source.sha256"
printf 'Private zero-inference review proof: %s\n' "$proof"
docker run --rm --runtime runc --network none --user "$(id -u):$(id -g)" --memory 1536m --pids-limit 128 --cpus 2 --cap-drop ALL --security-opt no-new-privileges:true \
 --volume "$source_dir:/src:ro" --volume /srv/storage/butler/.cache:/cache --volume "$root/.cache/go-tmp:/tmp" --volume "$proof/binaries:/binaries" --workdir /src \
 --env GOCACHE=/cache/go-race --env GOMODCACHE=/cache/go-mod --env GOTMPDIR=/tmp --env TMPDIR=/tmp \
 "$image" bash -eu -c '
go test -json -race -p 1 -count=1 ./scripts/dev/cloud-v4-final-audit
go build -o /binaries/review-seal ./scripts/dev/cloud-v4-review
go build -o /binaries/final-audit ./scripts/dev/cloud-v4-final-audit
go vet ./scripts/dev/cloud-v4-review ./scripts/dev/cloud-v4-final-audit
chmod 555 /binaries/*
' > "$proof/tests.jsonl" 2> "$proof/diagnostics.log"
! grep -Eq '"Action":"(skip|fail)"' "$proof/tests.jsonl"
sha256sum "$proof/binaries/"* "$proof/tests.jsonl" "$proof/source.sha256" > "$proof/evidence.sha256"
[[ "$phase" == seal ]] || exit 0
review=/srv/storage/butler/reviews/issue33-20261013-v4-sealed
[[ ! -e "$review" && ! -L "$review" ]] || { printf 'Denied: existing review seal.\n' >&2; exit 2; }
# Exact approved public declaration. No signing key exists before this boundary.
mkdir -m 755 "$review" # Public declaration only; made0700 before key generation.
cp "$source_dir/docs/ai/ru-cloud-fresh-v4-independent-review-20261013.md" "$review/independent-review.md"
docker run --rm --network none --read-only --user 0:0 --cap-drop ALL --cap-add CHOWN --cap-add FOWNER --security-opt no-new-privileges:true \
 --volume "$review:/review" alpine:3.22 sh -eu -c 'chown -R 10001:10001 /review; chmod 600 /review/independent-review.md; chmod 700 /review'
docker run --rm --runtime runc --network none --read-only --user 10001:10001 --cap-drop ALL --security-opt no-new-privileges:true --memory 512m --pids-limit 64 \
 --volume "$source_dir:/src:ro" --volume "$proof/binaries/review-seal:/seal:ro" --volume "$review:/review" \
 "$image" /seal \
 -corpus-sha256 a4bf81093ec00be4c424fc511c80f446e96908580b3e4cb5abb0398f94274949 \
 -review-sha256 c9680d537535e51b01324369c28413ae746c112771f73325713ee83f54a3abfd \
 -semantic-sha256 c250f9e23cd1e7dc50793dd3819b028e61dbdda999661e592b02d8fd75a1f2ea \
 -schema-sha256 93737ee1d08004166fca2d053f65c3f83d4dbfb7606db2d58b900647f1fbd4fe \
 -policy-sha256 c791f53975adcc10632d6c0261d92c4b42b99b0764af4829ba806f9e6bce0dd0 \
 -criteria-sha256 a2a3c80789d1cb1902bf02f5c7c74690a1c406b5b9ccef6ba77335e134dbcc02 \
 -resources-sha256 0571ed15d40bfc96a440e3463a42649b0e87c04eb452c96ff90cf5e3a78df126 > "$proof/seal.json"
cat "$proof/seal.json" # Public hashes only. Private key never leaves /review.
