#!/bin/sh
set -eu
umask 077
base=/srv/gitea/butler-acceptance/issue115-native-20261028-v1
cd "$base"
test ! -e postgres
mkdir postgres auth-source
d() { /usr/bin/docker --host unix:///var/run/docker.sock --config "$base/probe-v1/docker-config" "$@"; }
d network create --subnet 10.249.116.0/28 issue115-native-20261028-v1 > native-network.txt
d run -d --name issue115-native-pg-20261028-v1 --network issue115-native-20261028-v1 --cpus 1 --memory 512m --memory-swap 512m --pids-limit 128 --log-driver local --log-opt max-size=1m --log-opt max-file=2 --read-only --tmpfs /tmp:rw,nosuid,nodev,size=64m --tmpfs /var/run/postgresql:rw,nosuid,nodev,size=16m --mount type=bind,src="$base/postgres",dst=/var/lib/postgresql/data --publish 127.0.0.1:55455:55451 -e POSTGRES_HOST_AUTH_METHOD=trust -e POSTGRES_DB=behavior76_test sha256:ac08538c6f8b9904c33c8224c5e5706dbe760aca29db1d096972b4052c22a75d -p55451 -c shared_buffers=32MB -c max_wal_size=128MB -c min_wal_size=32MB > native-pg-container.txt
i=0
until d exec issue115-native-pg-20261028-v1 pg_isready -p55451 -U postgres -d behavior76_test >/dev/null 2>&1; do i=$((i+1)); test "$i" -lt 30; sleep 1; done
d exec issue115-native-pg-20261028-v1 psql -X -p55451 -Upostgres -dbehavior76_test -At -c 'SELECT version(); SHOW fsync;' > native-pg-version.txt
