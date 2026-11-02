#!/usr/bin/env bash
set -euo pipefail
root=${1:?owned browser-owner test root}
[[ "$root" == /srv/storage/butler/agents/server-browser-owners-20261029 ]] || exit 2
src=/tmp/butler-browser-expired-access-20261029-v8
test -s "$src/internal/budget/reviewed_preparation.go"
run="$(date -u +%Y%m%dT%H%M%SZ)-$$"
proof="$root/reports/expired-access-$run";mkdir "$proof"
pg="butler-expired-access-$run"
created=false
cleanup(){ local code=$?;if $created;then docker rm -f "$pg" >/dev/null;fi;printf '{"exit_code":%d,"native_model_calls":0}\n' "$code" > "$proof/exit.json"; }
trap cleanup EXIT
docker run -d --name "$pg" --network none --memory 1g --cpus 1 --pids-limit 128 --tmpfs /var/lib/postgresql/data:rw,size=768m -e POSTGRES_HOST_AUTH_METHOD=trust -e POSTGRES_DB=reviewed_budget_test pgvector/pgvector:pg17 -c max_connections=40 -c max_wal_size=64MB -c min_wal_size=32MB -c shared_buffers=32MB >/dev/null
created=true
for i in $(seq 1 30);do docker exec "$pg" pg_isready -U postgres -d reviewed_budget_test >/dev/null 2>&1 && break;sleep 1;done
docker exec "$pg" pg_isready -U postgres -d reviewed_budget_test >/dev/null
args=(--rm --user 0:0 --memory 3g --cpus 2 --pids-limit 256 --security-opt no-new-privileges:true
 --network "container:$pg" -v "$src:/src:ro" -v "$root/.cache:/cache" -v /srv/storage/butler/.cache/go-mod:/modules:ro -w /src
 -e GOCACHE=/cache/go-build -e GOMODCACHE=/modules -e GOTMPDIR=/cache/go-tmp -e GOPROXY=off -e GOSUMDB=off
 -e BUTLER_TEST_ACTIONAUTH_POSTGRES_URL='postgres://postgres@127.0.0.1:5432/reviewed_budget_test?sslmode=disable'
 -e BUTLER_TEST_VAULT_POSTGRES_URL='postgres://postgres@127.0.0.1:5432/reviewed_budget_test?sslmode=disable'
 -e BUTLER_TEST_POSTGRES_URL='postgres://postgres@127.0.0.1:5432/reviewed_budget_test?sslmode=disable')
if ! docker run "${args[@]}" golang:1.27.2-bookworm go test -json -race -p 1 -count=1 -timeout 4m ./internal/actionauth ./internal/budget ./internal/credentials \
 -run '^TestStoppedLocalPublicationOutcomeIntegration$|^TestOwnerOutcomePinnedCurrentMessageContract$|^TestVaultLocalPostgresIntegration$|^TestCredentialControlBrokerLocalPostgres$|^TestOwnerOutcomeStoppedExpiredAccess|^TestOwnerOutcomePinnedAccess|^TestCredentialOwnerGrant|^TestCredentialRequestLocator|^TestOwnerOutcomeReviewed|^TestNormalizedLoop|^TestStoppedAttemptIsolation|^TestCommunicationConversationLoop|^TestCredentialInteractionRetained|^TestRuntimeAdmission|^TestBudgetRootPreflight' > "$proof/pg-race.jsonl" 2>&1;then grep -E '"Action":"fail"|"Output":".*[.]go:[0-9]+' "$proof/pg-race.jsonl" || tail -50 "$proof/pg-race.jsonl";exit 1;fi
if grep -q '"Action":"skip"' "$proof/pg-race.jsonl";then exit 1;fi
for required in TestOwnerOutcomePinnedCurrentMessageContract TestStoppedLocalPublicationOutcomeIntegration/ecc90b76/absent TestStoppedLocalPublicationOutcomeIntegration/ecc90b76/wrong_revision TestVaultLocalPostgresIntegration TestOwnerOutcomeStoppedExpiredAccessRequestIntegration TestCredentialOwnerGrantIsLocalAndPinsInputClaim TestOwnerOutcomeReviewedPreparationBudgetRequiresCompleteNoWriteProofIntegration TestStoppedAttemptIsolationCannotAuthorizeAnotherWriteIntegration TestCommunicationConversationLoopNewInputCannotEvadeUnknownEffectIntegration TestCredentialInteractionRetainedUncertaintyMatrixIntegration;do grep -q "\"Action\":\"pass\".*\"Test\":\"$required\"" "$proof/pg-race.jsonl";done
docker run "${args[@]}" golang:1.27.2-bookworm go vet ./internal/actionauth ./internal/budget ./internal/credentialcontrol
printf 'Reviewed local preparation PG/race and vet PASS; proof=%s\n' "$proof"
