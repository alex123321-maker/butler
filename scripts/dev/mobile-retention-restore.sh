#!/usr/bin/env bash
set -euo pipefail
root=${BUTLER_MOBILE_TEST_ROOT:?isolated test root required}
pg=butler-mobile-memory-tests-pg
network=butler-mobile-memory-tests
[[ "$root" == /srv/storage/butler/agents/mobile-retention-* ]]
test ! -e "$root/.cache/retention-restore.manifest"
phase() {
 local phase=$1 db=$2
 docker run --rm --pull never --network "$network" --memory 512m --cpus 1 \
  -v "$root/.cache:/artifacts" -v "$root/current:/src:ro" \
  -e BUTLER_TEST_MOBILE_RETENTION_PHASE="$phase" \
  -e BUTLER_TEST_MOBILE_RETENTION_POSTGRES_URL="postgres://butler:synthetic-test@$pg:5432/$db?sslmode=disable" \
  -e BUTLER_TEST_MOBILE_RETENTION_MANIFEST=/artifacts/retention-restore.manifest \
  golang:1.27.2-bookworm /artifacts/mobile.test -test.run '^TestRetainedMobileActualBackupPhase$' -test.v
}
for db in mobile_retention_backup_test mobile_retention_expiry_test mobile_retention_forget_test; do
 docker exec "$pg" createdb -U butler "$db"
done
phase seed mobile_retention_backup_test
docker exec "$pg" pg_dump -U butler -Fc mobile_retention_backup_test > "$root/.cache/retention-before-expiry.pgdump"
chmod 600 "$root/.cache/retention-before-expiry.pgdump"
phase expire mobile_retention_backup_test
docker exec -i "$pg" pg_restore -U butler -d mobile_retention_expiry_test --exit-on-error < "$root/.cache/retention-before-expiry.pgdump"
phase restore-expiry mobile_retention_expiry_test
phase forget mobile_retention_backup_test
docker exec -i "$pg" pg_restore -U butler -d mobile_retention_forget_test --exit-on-error < "$root/.cache/retention-before-expiry.pgdump"
phase restore-forget mobile_retention_forget_test
