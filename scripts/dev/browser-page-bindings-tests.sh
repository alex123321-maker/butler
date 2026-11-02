#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")/../.."
: "${BUTLER_TEST_BROWSER_PAGES_POSTGRES_URL:?Disposable *_test PostgreSQL required}"
export GOTOOLCHAIN=local
mkdir -p .cache/browser-page-bindings-ci
report=.cache/browser-page-bindings-ci/pg-race.jsonl
if ! go test -race -count=1 -json ./internal/browsercontrol >"$report"; then
  grep -E '"Action":"fail"|"OutputType":"error"|"Output":".*[.]go:[0-9]+' "$report" || tail -40 "$report"
  exit 1
fi
if grep -q '"Action":"skip"' "$report"; then echo 'Required page binding case skipped'; exit 1; fi
for name in Lifecycle ContextFences ConcurrentCapsAndCAS OwnerTransitionRace RetentionAndDowngrade; do
  if ! grep -F '"Action":"pass"' "$report" | grep -Fq "\"Test\":\"TestPageBinding${name}Integration\""; then
    printf 'Required page binding case absent: %s\n' "$name"; exit 1
  fi
done
go vet ./internal/browsercontrol
echo 'Page bindings PostgreSQL/race/vet PASS; model_calls=0; runtime_popup_support=disabled'
