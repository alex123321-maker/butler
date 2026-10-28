#!/bin/sh
set -eu
umask 077
base=/srv/gitea/butler-acceptance/issue115-20261028-v1
cd "$base"
test -d postgres; test -z "$(find postgres -mindepth 1 -print -quit)"
:
docker network create --internal --subnet 10.249.115.0/28 issue115-20261028-v1 > network.txt
docker run -d --name issue115-pg-20261028-v1 --network issue115-20261028-v1 --network-alias issue115-pg --cpus 1 --memory 512m --memory-swap 512m --pids-limit 128 --log-driver local --log-opt max-size=1m --log-opt max-file=1 --read-only --tmpfs /tmp:rw,nosuid,nodev,size=64m --tmpfs /var/run/postgresql:rw,nosuid,nodev,size=16m --mount type=bind,src="$base/postgres",dst=/var/lib/postgresql/data -e POSTGRES_HOST_AUTH_METHOD=trust -e POSTGRES_DB=memory115_test sha256:ac08538c6f8b9904c33c8224c5e5706dbe760aca29db1d096972b4052c22a75d -c shared_buffers=32MB -c max_wal_size=128MB -c min_wal_size=32MB > postgres-container.txt
i=0
until docker exec issue115-pg-20261028-v1 pg_isready -U postgres -d memory115_test >/dev/null 2>&1; do
 i=$((i+1)); test "$i" -lt 30; sleep 1
done
docker exec issue115-pg-20261028-v1 psql -X -U postgres -d memory115_test -At -c "SELECT version(); SHOW fsync;" > postgres-version.txt
