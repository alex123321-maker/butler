#!/usr/bin/env bash
# One fresh fixed-corpus window; never reset its paid ledger or reuse a label.
set -euo pipefail
base=${1:?isolated root}
revision=${2:?reviewed commit}
[[ "$base" == /srv/storage/butler/agents/issue85-20261016 && "$revision" =~ ^[a-f0-9]{40}$ ]] || exit 2
[[ "$(cat "$base/current/source-revision.txt")" == "$revision" ]] || exit 2
proof=$base/reports/native-v1
test ! -e "$proof"
test -s "$base/private/native.env"
test -s "$base/private/auth-setting.csv"
umask 077
mkdir "$proof"
expires=$(date -u -d '+45 minutes' +%Y-%m-%dT%H:%M:%SZ)
corpus=$(sha256sum "$base/current/testdata/acceptance/ru-memory-candidates-v1.json" | cut -d' ' -f1)
jq -n --arg revision "$revision" --arg expires "$expires" --arg corpus "$corpus" '{revision:$revision,expires_at:$expires,cases_sha256:$corpus,max_calls:24,maximum_reserved_tokens:25362432,maximum_reserved_millis:2160000,maximum_provider_input_bytes:201326592,cost_basis:"subscription_no_overage",fallback:false,automatic_acceptance:false,criteria:{exact_citation_rate:1,semantic_case_rate:0.95,unsafe_promotions:0,authority_writes:0,unknown_usage:0}}' > "$proof/caps.json"
docker exec butler-issue85-postgres createdb -U butler issue85_native_test
runner=(docker run --rm --network host --name butler-issue85-native --env-file "$base/private/native.env"
 -e GOPROXY=off -e GOCACHE=/cache/go-build -e GOTMPDIR=/cache/go-tmp -e GOMODCACHE=/gomod
 -e HTTPS_PROXY=http://127.0.0.1:18080 -e HTTP_PROXY= -e ALL_PROXY= -e NO_PROXY=localhost,127.0.0.1
 -v "$base/current:/src:ro" -v "$base/.cache:/cache" -v /srv/storage/butler/.cache/go-mod:/gomod:ro
 -v "$base/reports:/proof" -w /src)
"${runner[@]}" golang:1.27.2-bookworm go run ./apps/migrator -postgres-url 'postgres://butler@127.0.0.1:18585/issue85_native_test?sslmode=disable' > "$proof/migration.log" 2>&1
# This CSV contains only the encrypted setting and remains private server-side.
docker exec -i butler-issue85-postgres psql -v ON_ERROR_STOP=1 -U butler -d issue85_native_test -c 'COPY system_settings(key,value,component,is_secret,updated_at,updated_by) FROM STDIN WITH CSV' < "$base/private/auth-setting.csv" > "$base/private/import.log"
"${runner[@]}" -e BUTLER_CANDIDATE_NATIVE_EVIDENCE=/proof/native-v1 -e 'BUTLER_CANDIDATE_NATIVE_POSTGRES_URL=postgres://butler@127.0.0.1:18585/issue85_native_test?sslmode=disable' golang:1.27.2-bookworm go test -json -count=1 -timeout 43m -run '^TestCandidateNativeRussianQuality$' ./apps/orchestrator/internal/api > "$proof/runner.jsonl" 2>&1
