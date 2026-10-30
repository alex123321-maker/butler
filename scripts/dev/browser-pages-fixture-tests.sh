#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")/../.."
export GOTOOLCHAIN=local
export GOCACHE="$PWD/.cache/go-build-linux"
export GOMODCACHE="$PWD/.cache/go-mod"
export NPM_CONFIG_CACHE="$PWD/.cache/npm"
export PLAYWRIGHT_BROWSERS_PATH="${PLAYWRIGHT_BROWSERS_PATH:-$PWD/.cache/ms-playwright}"
export BUTLER_SSO_E2E_MODULE_ROOT="$PWD/apps/tool-browser/scripts"
mkdir -p .cache/browser-pages-ci

required() {
  local report=".cache/browser-pages-ci/$1.jsonl"
  shift
  if ! go test -race -count=1 -json ./cmd/browser-sso-eval "$@" >"$report"; then
    grep -E '"Action":"fail"|"OutputType":"error"|"Output":".*[.]go:[0-9]+' "$report" || tail -40 "$report"
    exit 1
  fi
  if grep -q '"Action":"skip"' "$report"; then
    echo 'Required SSO fixture case skipped'; exit 1
  fi
  if ! grep -q '"Action":"pass","Package".*"Test"' "$report"; then
    echo 'No required SSO fixture test executed'; exit 1
  fi
}

required unit -run '^TestSSO(Private|Origin|Fixture)'
go vet ./cmd/browser-sso-eval
export BUTLER_SSO_BROWSER_E2E=1
required chromium -run '^TestSSOPopupChromium$'
for name in normal late twins closed_before_login forged_message; do
  if ! grep -F '"Action":"pass"' .cache/browser-pages-ci/chromium.jsonl | grep -Fq "\"Test\":\"TestSSOPopupChromium/$name\""; then
    printf 'Required Chromium case absent: %s\n' "$name"; exit 1
  fi
done
echo 'SSO fixture race/vet and all five Chromium cases passed; model_calls=0'
