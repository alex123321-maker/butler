#!/bin/sh
set -eu
umask 077
base=/srv/gitea/butler-acceptance/issue115-20261028-v1
cd "$base"
test ! -e operator-v2.log
test ! -e postgres-operator
mkdir postgres-operator
docker run -d --name issue115-operator-pg-20261028-v1 --network issue115-20261028-v1 --cpus 1 --memory 256m --memory-swap 256m --pids-limit 128 --log-driver local --log-opt max-size=1m --log-opt max-file=2 --read-only --tmpfs /tmp:rw,nosuid,nodev,size=64m --tmpfs /var/run/postgresql:rw,nosuid,nodev,size=16m --mount type=bind,src="$base/postgres-operator",dst=/var/lib/postgresql/data -e POSTGRES_HOST_AUTH_METHOD=trust -e POSTGRES_DB=memory115_operator_test sha256:ac08538c6f8b9904c33c8224c5e5706dbe760aca29db1d096972b4052c22a75d -p 55451 -c shared_buffers=32MB -c max_wal_size=128MB -c min_wal_size=32MB > operator-pg-container.txt
i=0
until docker exec issue115-operator-pg-20261028-v1 pg_isready -p55451 -U postgres -d memory115_operator_test >/dev/null 2>&1; do i=$((i+1)); test "$i" -lt 30; sleep 1; done
set +e
docker run --rm --name issue115-operator-v2-20261028-v1 --network container:issue115-operator-pg-20261028-v1 --cpus 2 --memory 3g --memory-swap 3g --pids-limit 256 --read-only --cap-drop ALL --security-opt no-new-privileges --log-driver none --user 1000:1000 --tmpfs /tmp:rw,nosuid,nodev,size=256m --mount type=bind,src="$base/source",dst=/source,readonly --mount type=bind,src=/srv/storage/butler/.cache/go-mod,dst=/modules,readonly --mount type=bind,src="$base/cache",dst=/cache --mount type=bind,src="$base/build-tmp",dst=/build-tmp -e GOCACHE=/cache -e GOTMPDIR=/build-tmp -e GOMODCACHE=/modules -e GOPROXY=off -e GOSUMDB=off -e GOTOOLCHAIN=local -e GOMAXPROCS=2 -e GOFLAGS='-p=1 -buildvcs=false -mod=readonly' -e BUTLER_TEST_OPERATOR_POSTGRES_URL='postgres://postgres@127.0.0.1:55451/memory115_operator_test?sslmode=disable' -w /source sha256:5cf287a799e6b94384bad13d16b14904c531f51ba65792237e122ce42b392f61 go test -race -count=1 -v -timeout=3m scripts/dev/memory115-local-execute.go scripts/dev/memory115-local-execute_test.go scripts/dev/memory115-local-execute_integration_test.go > operator-v2.log 2>&1
status=$?
printf '{"exit_code":%s,"native_calls":0,"real_auth":0,"dedicated_loopback_namespace":true}\n' "$status" > operator-v2-exit.json
exit "$status"
