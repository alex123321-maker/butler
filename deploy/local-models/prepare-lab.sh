#!/usr/bin/env bash
set -euo pipefail
root=${1:-/srv/storage/butler/agents/local-models}
image=python:3.12-slim@sha256:a6e34c598f2467ed0e9a8d349809fcd8b5c603269512df273a0bb1784edc11b1
[[ "$root" == /srv/storage/butler/agents/* && "$root" != *..* ]] || exit 2
mkdir -p "$root/data" "$root/data/tmp" "$root/data/pip" "$root/data/home" "$root/reports" "$root/runtime"
# Preparation has network access, inference containers must run offline.
docker run --rm --name butler-local-models-prepare --user "$(id -u):$(id -g)" --volume "$root/data:/data" --volume "$root/current:/src:ro" \
  --env PIP_CACHE_DIR=/data/pip --env HF_HOME=/data/huggingface \
  --env TMPDIR=/data/tmp --env HOME=/data/home \
  --env HF_HUB_DISABLE_TELEMETRY=1 --entrypoint sh "$image" -c '
set -eu
python -m venv /data/venv
/data/venv/bin/pip install --dry-run --report /data/torch-download-plan.json torch==2.10.0 torchvision==0.25.0 --index-url https://download.pytorch.org/whl/cu128 >/dev/null
/data/venv/bin/python -c "import json; p=json.load(open(\"/data/torch-download-plan.json\")); print(\"Pinned Torch/CUDA wheel dependencies:\", len(p[\"install\"]))"
/data/venv/bin/pip install torch==2.10.0 torchvision==0.25.0 --index-url https://download.pytorch.org/whl/cu128 >/data/torch-install.log 2>&1
/data/venv/bin/pip install -r /src/deploy/local-models/requirements.lock.txt >/data/model-libraries-install.log 2>&1
/data/venv/bin/pip freeze > /data/requirements.lock.txt
/data/venv/bin/python /src/deploy/local-models/download.py /data/models
' > "$root/reports/model-prepare.log" 2>&1
du -sh "$root/data/venv" "$root/data/models" "$root/data/pip"
