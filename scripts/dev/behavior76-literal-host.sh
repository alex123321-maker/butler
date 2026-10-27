#!/bin/sh
# No build, dynamic test selector, fallback executable or output replacement.
set -eu
root=/mnt/host/wsl/butler-effort76
elf=${1:?prepared ELF required}
digest=${2:?prepared ELF hash required}
producer=${3:?clean producer required}
stimuli=${4:?fresh literal stimuli required}
output=${5:?new literal output required}
latest_epoch=${6:?finite latest admission epoch required}
not_before_utc=${7:?certificate not-before UTC required}
latest_start_utc=${8:?certificate latest-start UTC required}
case "$elf" in .cache/behavior76/literal-collector-*.test) ;; *) exit 2;; esac
case "$stimuli" in .cache/issue76-independent/hidden-literal-v11*/stimuli.jsonl) ;; *) exit 2;; esac
case "$output" in .cache/behavior76/native-literal-v11-*) ;; *) exit 2;; esac
for value in "$elf" "$stimuli" "$output";do
 case "$value" in *..*|*[!A-Za-z0-9_./-]*) exit 2;; esac
done
test "${#digest}" -eq 64
case "$digest" in *[!0-9a-f]*) exit 2;; esac
test ! -e "$root/src20/$output"
test -f "$root/src20/$elf"
test ! -L "$root/src20/$elf"
actual=$(sha256sum "$root/src20/$elf")
test "${actual%% *}" = "$digest"
case "$latest_epoch" in ''|*[!0-9]*) exit 2;; esac
# Strictly before the rounded-down latest second is conservative. The actual
# Go entry/window checks retain precise UTC and deny any later startup delay.
test "$(date -u +%s)" -lt "$latest_epoch"
exec /usr/sbin/chroot "$root" /usr/bin/env \
 PATH=/usr/local/go/bin:/usr/local/bin:/usr/bin:/bin GOMAXPROCS=2 \
 BUTLER_TEST_MEMORY_POSTGRES_URL='postgres://postgres@127.0.0.1:55451/behavior76_test?sslmode=disable' \
 BUTLER_BEHAVIOR76_BROKER_BINARY=/tmp/behavior76/tool-broker \
 BUTLER_BEHAVIOR76_PGX_OBSERVER_CONTRACT=callback-paired-public-only \
 BUTLER_BEHAVIOR76_NATIVE_ENABLE=literal-supplement-v11-synthetic-only \
 BUTLER_BEHAVIOR76_LITERAL_NOT_BEFORE_UTC="$not_before_utc" \
 BUTLER_BEHAVIOR76_LITERAL_LATEST_START_UTC="$latest_start_utc" \
 BUTLER_BEHAVIOR76_SOURCE="$producer" \
 BUTLER_BEHAVIOR76_STIMULI="/src20/$stimuli" BUTLER_BEHAVIOR76_OUTPUT="/src20/$output" \
 /bin/sh -c 'cd /src20; exec "$1" -test.v -test.timeout=5h -test.run="^TestBehavior76NativeLiteralSupplement$"' literal-entry "/src20/$elf"
