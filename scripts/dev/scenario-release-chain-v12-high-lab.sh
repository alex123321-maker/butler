#!/usr/bin/env bash
# Fresh575 High chain: repeat owned GET reads within current Core budgets.
set -euo pipefail
root=${1:?isolated root required}; phase=${2:-prepare}; release=${3:?exact clean release required}
[[ "$root" == /srv/storage/butler/agents/scenario-release-chain-v12-high-20261013 && "$phase" =~ ^(prepare|execute)$ && "$release" =~ ^[a-f0-9]{40}$ ]] || exit 2
source_dir=$(readlink -f "$root/current")
[[ "$source_dir" == "$root/snapshots/"* ]] || exit 2
exec bash "$source_dir/scripts/dev/scenario-release-chain-v5-high-lab.sh" "$root" "$phase" "$release" v12
