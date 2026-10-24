#!/bin/sh
# Run only from the isolated issue140 worktree on laba. No credentials/network.
set -eu
test "$(pwd -P)" = /srv/storage/butler/agents/issue-140-emoji-research
test $# -eq 1
case "$1" in ''|*[!a-zA-Z0-9-]*) exit 2;; esac
docker run --rm --name butler-issue140-retrieval --user 0:0 --network none \
  --gpus all --cpus 2 --memory 8g --memory-swap 8g --pids-limit 256 \
  --cap-drop ALL --cap-add DAC_OVERRIDE --security-opt no-new-privileges \
  --read-only --tmpfs /tmp:rw,nosuid,size=256m \
  --entrypoint /data/venv/bin/python \
  -e PYTHONPATH=/work/.cache/python -e HF_HOME=/work/.cache/hf \
  -v "$PWD:/work" \
  -v /srv/storage/butler/agents/local-models/data/venv:/data/venv:ro \
  -v /srv/storage/butler/agents/local-models/data/models/914f7f89142e33e77833254d9c9b90c3cef7303b:/model:ro \
  sha256:1f8334477430a5d885a9defcbe79ac21367591dbe85f8f9ede4d4b50e45ff502 \
  /work/experiments/emoji140/retrieve.py --name "$1"
