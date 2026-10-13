#!/usr/bin/env bash
# Fresh full High chain from factual557 with value-free source-check observations.
set -euo pipefail
root=${1:?isolated root required}; phase=${2:-prepare}; release=${3:?exact clean release required}
[[ "$root" == /srv/storage/butler/agents/scenario-release-chain-v11-high-20261013 && "$phase" =~ ^(prepare|execute)$ && "$release" =~ ^[a-f0-9]{40}$ ]] || exit 2
source_dir=$(readlink -f "$root/current")
[[ "$source_dir" == "$root/snapshots/"* ]] || exit 2
exec bash "$source_dir/scripts/dev/scenario-release-chain-v5-high-lab.sh" "$root" "$phase" "$release" v11
