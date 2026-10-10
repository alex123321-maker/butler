#!/usr/bin/env bash
set -euo pipefail
root=${1:?absolute lab root required}
[[ "$root" =~ ^/(srv|opt|home)/[a-zA-Z0-9_./-]+$ && "$root" != *..* ]] || exit 2
storage="$root/.cache/buildkit"
mkdir -p "$storage"
storage=$(readlink -f "$storage")
[[ "$storage" == "$root/.cache/buildkit" ]] || {
  echo 'Build cache must stay inside the selected lab storage root.' >&2; exit 2;
}
suffix=$(printf '%s' "$root" | sha256sum | cut -c1-12)
builder="butler-lab-storage-$suffix"
node="${builder}0"
volume="buildx_buildkit_${node}_state"
if docker volume inspect "$volume" >/dev/null 2>&1; then
  actual=$(docker volume inspect --format '{{index .Options "device"}}' "$volume")
  [[ "$actual" == "$storage" ]] || {
    echo 'Existing build cache volume points outside the selected lab root.' >&2; exit 2;
  }
else
  docker volume create --driver local --opt type=none --opt o=bind --opt "device=$storage" \
    --label com.butler.purpose=build-cache "$volume" >/dev/null
fi
if ! docker buildx inspect "$builder" >/dev/null 2>&1; then
  docker buildx create --name "$builder" --node "$node" --driver docker-container >/dev/null
fi
driver=$(docker buildx inspect "$builder" | awk '$1 == "Driver:" {print $2}')
[[ "$driver" == docker-container ]] || {
  echo 'Lab storage builder must use the Docker container driver.' >&2; exit 2;
}
# Output contains a builder name only; Compose receives it through an environment
# variable rather than changing the global Docker builder for other projects.
printf '%s\n' "$builder"
