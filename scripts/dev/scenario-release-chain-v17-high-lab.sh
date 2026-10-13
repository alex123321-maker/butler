#!/usr/bin/env bash
# One fresh frozen run of the four pending cases; independent checks run in parallel.
set -euo pipefail
root=${1:?isolated root required}; release=${2:?exact clean release required}
[[ "$root" == /srv/storage/butler/agents/scenario-release-chain-v17-high-20261013 && "$release" =~ ^[a-f0-9]{40}$ ]] || exit 2
source_dir=$(readlink -f "$root/current")
[[ "$source_dir" == "$root/snapshots/"* ]] || exit 2
bash "$source_dir/scripts/dev/scenario-release-chain-v5-high-lab.sh" "$root" prepare "$release" v17
exec bash "$source_dir/scripts/dev/scenario-release-chain-v5-high-lab.sh" "$root" execute "$release" v17
