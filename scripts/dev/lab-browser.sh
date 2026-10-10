#!/usr/bin/env bash
set -euo pipefail

# Run disposable real-browser tests only; no Compose services or durable data.
root=${1:?absolute agent lab root required}
[[ "$root" =~ ^/(srv|opt|home)/[a-zA-Z0-9_./-]+$ && "$root" != *..* ]] || exit 2
source_dir=$(readlink -f "$root/current")
[[ "$source_dir" == "$root"/snapshots/* ]] || exit 2
mkdir -p "$root/.cache" "$root/reports"
docker run --rm --user 1000:1000 \
  -v "$source_dir:/src:ro" -v "$root/.cache:/cache" -w /src \
  -e CGO_ENABLED=0 -e GOCACHE=/cache/go-build -e GOMODCACHE=/cache/go-mod \
  golang:1.24-alpine go test -c -o /cache/browser-runtime.test ./apps/tool-browser/internal/runtime
docker run --rm --user 1000:1000 \
  -v "$source_dir:/src:ro" -v "$root/.cache:/cache" -w /src \
  -e CGO_ENABLED=0 -e GOCACHE=/cache/go-build -e GOMODCACHE=/cache/go-mod \
  golang:1.24-alpine go build -o /cache/browser-runtime ./apps/tool-browser
docker run --rm --user 1000:1000 \
  -v "$source_dir:/src:ro" -v "$root/.cache:/cache" -w /src \
  -e CGO_ENABLED=0 -e GOCACHE=/cache/go-build -e GOMODCACHE=/cache/go-mod \
  golang:1.24-alpine go test -c -o /cache/browser-broker.test ./apps/tool-broker/internal/broker
docker run --rm --user 1000:1000 \
  -v "$source_dir:/src:ro" -v "$root/.cache:/cache" \
  -v "$source_dir/apps/tool-browser/scripts:/app/apps/tool-browser/scripts:ro" -w /src \
  -e BUTLER_BROWSER_E2E=1 -e BUTLER_BROWSER_E2E_SCRIPT=/app/apps/tool-browser/scripts/browser_runtime.mjs \
  --entrypoint /cache/browser-runtime.test butler-lab-tool-browser \
  -test.v -test.run '^TestBrowserSessionE2E$' \
  | tee "$root/reports/browser-e2e-$(date -u +%Y%m%dT%H%M%SZ).log"
docker run --rm --user 1000:1000 \
  -v "$source_dir:/src:ro" -v "$root/.cache:/cache" \
  -v "$source_dir/apps/tool-browser/scripts:/app/apps/tool-browser/scripts:ro" -w /src \
  -e BUTLER_BROWSER_E2E=1 -e BUTLER_BROWSER_E2E_SCRIPT=/app/apps/tool-browser/scripts/browser_runtime.mjs \
  -e BUTLER_BROWSER_E2E_BINARY=/cache/browser-runtime \
  --entrypoint /cache/browser-broker.test butler-lab-tool-browser \
  -test.v -test.run '^TestBrokerBrowserSequenceE2E$' \
  | tee "$root/reports/browser-broker-e2e-$(date -u +%Y%m%dT%H%M%SZ).log"
