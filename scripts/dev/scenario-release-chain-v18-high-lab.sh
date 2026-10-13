#!/usr/bin/env bash
# Remaining three cases immediately after the frozen build; Stop is already verified.
set -euo pipefail
root=${1:?isolated root required}; release=${2:?exact clean release required}
[[ "$root" == /srv/storage/butler/agents/scenario-release-chain-v18-high-20261013 && "$release" =~ ^[a-f0-9]{40}$ ]] || exit 2
source_dir=$(readlink -f "$root/current")
[[ "$source_dir" == "$root/snapshots/"* ]] || exit 2
bash "$source_dir/scripts/dev/scenario-release-chain-v5-high-lab.sh" "$root" prepare "$release" v18
exec bash "$source_dir/scripts/dev/scenario-release-chain-v5-high-lab.sh" "$root" execute "$release" v18
