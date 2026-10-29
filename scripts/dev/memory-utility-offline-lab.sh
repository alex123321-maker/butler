#!/bin/sh
# Existing isolated source snapshot only. No models, credentials, DB or network.
set -eu
umask 077
root=${1:?absolute isolated utility root required}
case "$root" in /srv/gitea/butler-acceptance/issue115-memory-utility-*) ;; *) echo 'isolated utility root required' >&2; exit 1;; esac
test -f "$root/source/go.mod"
test ! -e "$root/reports/started"
date -u +%FT%TZ > "$root/reports/started"
image=sha256:5cf287a799e6b94384bad13d16b14904c531f51ba65792237e122ce42b392f61
set +e
docker run --rm --network none --cpus 1 --memory 2g --memory-swap 2g --pids-limit 256 \
  --read-only --cap-drop ALL --security-opt no-new-privileges --log-driver none \
  --user 1000:1000 --tmpfs /tmp:rw,nosuid,nodev,size=64m \
  -v "$root/source:/source:ro" -v "$root/cache:/cache" -v "$root/tmp:/build-tmp" \
  -v "$root/reports:/reports" -v /srv/storage/butler/.cache/go-mod:/modules:ro \
  -e GOCACHE=/cache -e GOTMPDIR=/build-tmp -e GOMODCACHE=/modules -e GOPROXY=off \
  -e GOSUMDB=off -e GOTOOLCHAIN=local -e GOMAXPROCS=1 \
  -e GOFLAGS='-p=1 -buildvcs=false -mod=readonly' -w /source "$image" /bin/sh -eu -c '
go version
go test -race -count=1 -json -run "^(TestMemory|TestInitSeparatesOracle|TestScoreCLI)" ./internal/acceptance ./cmd/memory-utility-eval > /reports/tests.jsonl
go vet ./internal/acceptance ./cmd/memory-utility-eval
go build -o /reports/memory-utility-eval ./cmd/memory-utility-eval
/reports/memory-utility-eval -init-dir /reports/prospective-suite
sha256sum /reports/tests.jsonl /reports/memory-utility-eval /reports/prospective-suite/manifest.json
' > "$root/reports/build.log" 2>&1
status=$?
set -e
printf '{"exit_code":%s,"new_model_requests":0,"network":"none"}\n' "$status" > "$root/reports/exit.json"
date -u +%FT%TZ > "$root/reports/finished"
cat "$root/reports/build.log"
exit "$status"
