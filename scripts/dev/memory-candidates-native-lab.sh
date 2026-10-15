#!/usr/bin/env bash
# One fresh fixed-corpus window; never reset its paid ledger or reuse a label.
set -euo pipefail
base=${1:?isolated root}
revision=${2:?reviewed commit}
label=${3:-native-v4}
prior=${4:?exact retained paid attempt count}
[[ "$prior" =~ ^[0-9]+$ && "$prior" -ge 24 && "$prior" -le 128 ]] || exit 2
remaining=32
[[ "$base" == /srv/storage/butler/agents/issue85-20261016 && "$revision" =~ ^[a-f0-9]{40}$ ]] || exit 2
[[ "$label" =~ ^native-v[1-9][0-9]*$ ]] || exit 2
[[ "$(cat "$base/current/source-revision.txt")" == "$revision" ]] || exit 2
proof=$base/reports/$label
test ! -e "$proof"
test -s "$base/private/native.env"
test -s "$base/private/auth-setting.csv"
umask 077
mkdir "$proof"
expires=$(date -u -d '+45 minutes' +%Y-%m-%dT%H:%M:%SZ)
corpus=$(sha256sum "$base/current/testdata/acceptance/ru-memory-candidates-v1.json" | cut -d' ' -f1)
holdout=$(sha256sum "$base/current/testdata/acceptance/ru-memory-candidates-holdout-v1.json" | cut -d' ' -f1)
jq -n --arg revision "$revision" --arg expires "$expires" --arg corpus "$corpus" --arg holdout "$holdout" --argjson prior "$prior" --argjson remaining "$remaining" '{revision:$revision,expires_at:$expires,cases_sha256:$corpus,holdout_sha256:$holdout,handler:"source-candidates-v2",max_calls:$remaining,start_case:0,prior_attempts:$prior,maximum_reserved_tokens:(1056768*$remaining),maximum_reserved_millis:(90000*$remaining),maximum_provider_input_bytes:(8388608*$remaining),cost_basis:"subscription_no_overage",fallback:false,automatic_acceptance:false,criteria:{exact_citation_rate:1,semantic_case_rate:0.95,holdout_semantic_case_rate:1,unsafe_promotions:0,authority_writes:0,unknown_usage:0}}' > "$proof/caps.json"
runner=(docker run --rm --network host --name butler-issue85-native --env-file "$base/private/native.env"
 -e GOPROXY=off -e GOCACHE=/cache/go-build -e GOTMPDIR=/cache/go-tmp -e GOMODCACHE=/gomod
 -e HTTPS_PROXY=http://127.0.0.1:18080 -e HTTP_PROXY= -e ALL_PROXY= -e NO_PROXY=localhost,127.0.0.1
 -v "$base/current:/src:ro" -v "$base/.cache:/cache" -v /srv/storage/butler/.cache/go-mod:/gomod:ro
 -v "$base/reports:/proof" -w /src)
"${runner[@]}" golang:1.27.2-bookworm go run ./apps/migrator -postgres-url 'postgres://butler@127.0.0.1:18585/issue85_native_test?sslmode=disable' > "$proof/migration.log" 2>&1
# This CSV contains only the encrypted setting and remains private server-side.
{
 printf 'CREATE TEMP TABLE native_auth_import (LIKE system_settings INCLUDING ALL);\nCOPY native_auth_import(key,value,component,is_secret,updated_at,updated_by) FROM STDIN WITH CSV;\n'
 cat "$base/private/auth-setting.csv"
 printf '\\.\nINSERT INTO system_settings SELECT * FROM native_auth_import ON CONFLICT(key) DO UPDATE SET value=EXCLUDED.value,updated_at=EXCLUDED.updated_at,updated_by=EXCLUDED.updated_by;\n'
} | docker exec -i butler-issue85-postgres psql -p 18585 -v ON_ERROR_STOP=1 -U butler -d issue85_native_test > "$base/private/import-$label.log"
"${runner[@]}" golang:1.27.2-bookworm go test -c -o "/proof/$label/candidate-api.test" ./apps/orchestrator/internal/api > "$proof/build.log" 2>&1
sha256sum "$proof/candidate-api.test" > "$proof/binary.sha256"
sha256sum "$base/current/internal/memory/candidates/handler.go" "$base/current/internal/memory/jobs/model_worker.go" "$base/current/apps/orchestrator/internal/api/memory_candidates_native_test.go" "$base/current/testdata/acceptance/ru-memory-candidates-v1.json" "$base/current/testdata/acceptance/ru-memory-candidates-holdout-v1.json" > "$proof/sources.sha256"
"${runner[@]}" -w /src/apps/orchestrator/internal/api -e "BUTLER_CANDIDATE_NATIVE_EVIDENCE=/proof/$label" -e 'BUTLER_CANDIDATE_NATIVE_POSTGRES_URL=postgres://butler@127.0.0.1:18585/issue85_native_test?sslmode=disable' golang:1.27.2-bookworm go tool test2json -p github.com/butler/butler/apps/orchestrator/internal/api "/proof/$label/candidate-api.test" -test.v -test.timeout=43m -test.run='^TestCandidateNativeRussianQuality$' > "$proof/runner.jsonl" 2>&1
