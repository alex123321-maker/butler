#!/usr/bin/env bash
set -euo pipefail
root=${1:-/srv/storage/butler/agents/local-models}
[[ "$root" == /srv/storage/butler/agents/* && "$root" != *..* ]] || exit 2
image=python:3.12-slim@sha256:a6e34c598f2467ed0e9a8d349809fcd8b5c603269512df273a0bb1784edc11b1
docker run --rm --name butler-local-models-extend --user "$(id -u):$(id -g)" \
 --volume "$root/data:/data" --volume "$root/current:/src:ro" --env TMPDIR=/data/tmp \
 --env HOME=/data/home --env PIP_CACHE_DIR=/data/pip --env HF_HOME=/data/huggingface \
 --env HF_HUB_DISABLE_TELEMETRY=1 --entrypoint sh "$image" -c '
set -eu
/data/venv/bin/pip install --dry-run --report /data/extension-plan.json -r /src/deploy/local-models/extension.lock.txt >/data/extension-plan.log 2>&1
/data/venv/bin/python -c "import json; p=json.load(open(\"/data/extension-plan.json\")); print(\"Additional dependency wheels:\",[(i[\"metadata\"][\"name\"],i[\"metadata\"][\"version\"]) for i in p[\"install\"]])"
/data/venv/bin/pip install --no-deps -r /src/deploy/local-models/extension.lock.txt >/data/extension-install.log 2>&1
/data/venv/bin/pip check
/data/venv/bin/pip freeze > /data/requirements.lock.txt
/data/venv/bin/python /src/deploy/local-models/download.py /data/models
' > "$root/reports/extension-prepare.log" 2>&1
du -sh "$root/data/venv" "$root/data/models"
