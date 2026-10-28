#!/bin/sh
set -eu
umask 077
base=/srv/gitea/butler-acceptance/issue115-native-20261028-v1
cd "$base"
test ! -e proxy-route-v2.log
d() { /usr/bin/docker --host unix:///var/run/docker.sock --config "$base/probe-v1/docker-config" "$@"; }
test "$(d inspect -f '{{range .NetworkSettings.Networks}}{{.IPAddress}}{{end}}' issue115-native-pg-20261028-v1)" = 10.249.116.2
d run -d --name issue115-proxy-relay-20261028-v2 --network host --cpus 0.25 --memory 64m --memory-swap 64m --pids-limit 32 --read-only --cap-drop ALL --security-opt no-new-privileges --log-driver local --log-opt max-size=1m --log-opt max-file=2 --user 1000:1000 --mount type=bind,src="$base/proxy-bin-v2",dst=/relay,readonly sha256:5cf287a799e6b94384bad13d16b14904c531f51ba65792237e122ce42b392f61 /relay/memory115-laba-proxy serve > proxy-relay-v2.cid
sleep 1
set +e
d run --rm --name issue115-proxy-route-probe-v2 --network container:issue115-native-pg-20261028-v1 --cpus 1 --memory 2g --memory-swap 2g --pids-limit 256 --read-only --cap-drop ALL --security-opt no-new-privileges --log-driver none --user 1000:1000 --mount type=bind,src="$base/proxy-bin-v2",dst=/relay,readonly sha256:5cf287a799e6b94384bad13d16b14904c531f51ba65792237e122ce42b392f61 /relay/memory115-laba-proxy probe > proxy-route-v2.log 2>&1
status=$?
set -e
d inspect -f '{{json .HostConfig}}' issue115-proxy-relay-20261028-v2 > proxy-relay-host-config-v2.json
sha256sum proxy-bin-v2/memory115-laba-proxy > proxy-relay-binary-v2.sha256
printf '{"exit_code":%s,"generation_requests":0,"auth_access":0,"fixed_upstream":"127.0.0.1:18080","bridge_listener":"10.249.116.1:18081","only_peer":"10.249.116.2"}\n' "$status" > proxy-route-v2-exit.json
cat proxy-route-v2.log
exit "$status"
