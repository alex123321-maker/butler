#!/usr/bin/env bash
set -euo pipefail

# Download verified public binaries without running a remote installer or
# modifying shell profiles. Login is a separate interactive operation.
task_root=/srv/storage/butler/agents/antigravity-provider
source_root=$(cd "$(dirname "$0")/../.." && pwd)
expected_version=1.3.3
manifest_base=https://antigravity-cli-auto-updater-974169037036.us-central1.run.app/manifests
case $(uname -m) in
  x86_64) architecture=amd64 ;;
  aarch64) architecture=arm64 ;;
  *) echo 'Unsupported Antigravity lab architecture.' >&2; exit 1 ;;
esac
command -v jq >/dev/null
mkdir -p "$task_root/.cache" "$task_root/runtime" "$task_root/workspace/.agents/agents" "$task_root/profile"
for variant in native musl; do
  platform=linux_$architecture
  [[ $variant == native ]] || platform=${platform}_musl
  manifest="$task_root/.cache/manifest-$variant.json"
  curl -fsSL "$manifest_base/$platform.json" -o "$manifest"
  test "$(jq -r .version "$manifest")" = "$expected_version" || {
    echo 'Antigravity CLI version changed; review the adapter before upgrading.' >&2; exit 1;
  }
  download_url=$(jq -r .url "$manifest")
  case "$download_url" in
    https://storage.googleapis.com/antigravity-public/antigravity-cli/*) ;;
    *) echo 'Unexpected Antigravity download host.' >&2; exit 1 ;;
  esac
  payload="$task_root/.cache/agy-$variant.tar.gz"
  curl -fsSL "$download_url" -o "$payload"
  test "$(sha512sum "$payload" | cut -d ' ' -f 1)" = "$(jq -r .sha512 "$manifest")"
  stage=$(mktemp -d "$task_root/.cache/extract-XXXXXX")
  tar -xzf "$payload" -C "$stage" antigravity
  destination="$task_root/runtime/agy"
  [[ $variant == native ]] || destination="$task_root/runtime/agy-musl"
  cp "$stage/antigravity" "$destination"
  chmod 755 "$destination"
  rm -f "$stage/antigravity"
  rmdir "$stage"
done
cp "$source_root/internal/transport/antigravity/agent.md" "$task_root/workspace/.agents/agents/butler-model.md"
cp "$source_root/internal/transport/antigravity/settings.json" "$task_root/runtime/settings.json"
cp "$source_root/deploy/antigravity/lab-login.sh" "$task_root/login.sh"
chmod 755 "$task_root/login.sh"
docker run --rm --user 0 \
  --mount "type=bind,source=$task_root/profile,target=/profile" \
  --mount "type=bind,source=$task_root/runtime/settings.json,target=/restricted-settings.json,readonly" \
  --entrypoint sh butler-lab-tool-browser -c '
    set -eu
    mkdir -p /profile/.gemini/antigravity-cli
    if [ ! -f /profile/.gemini/antigravity-cli/settings.json ]; then
      cp /restricted-settings.json /profile/.gemini/antigravity-cli/settings.json
    fi
    chown -R 10001:10001 /profile
    chmod 700 /profile
    chmod 600 /profile/.gemini/antigravity-cli/settings.json
  '
"$task_root/runtime/agy" --version
echo "Login: ssh -t laba 'bash $task_root/login.sh'"
