#!/bin/sh
set -u
cd /src20
export PATH=/usr/local/go/bin:/usr/local/bin:/usr/bin:/bin GOMAXPROCS=2
export BUTLER_BEHAVIOR76_NATIVE_ENABLE=disabled BUTLER_BEHAVIOR76_PGX_OBSERVER_CONTRACT=callback-paired-public-only
export BUTLER_TEST_MEMORY_POSTGRES_URL='postgres://postgres@127.0.0.1:55451/behavior76_test?sslmode=disable'
export BUTLER_BEHAVIOR76_BROKER_BINARY=/tmp/behavior76/tool-broker
export BUTLER_BEHAVIOR76_STIMULI=/src20/.cache/issue76-independent/hidden-literal-v11-r1/stimuli.jsonl
export BUTLER_BEHAVIOR76_SOURCE='git:c8411429e36f2e7d085971617f0f84dc2f2e5b6e;binary-sha256:7336e3fcfc282f9663a7fb23641146400900a7056e8a57889b803d272c3c411e'
export BUTLER_BEHAVIOR76_CONTROL_PREFLIGHT=literal-setup-only
/src20/.cache/behavior76/literal-collector-c8411429-r1.test -test.v -test.timeout=5m -test.run='^TestBehavior76PreflightLiteralSetup$'
status=$?
printf '{"version":"literal-r1-actual-setup-v1","exit":%s,"source":"c8411429e36f2e7d085971617f0f84dc2f2e5b6e","elf_sha256":"7336e3fcfc282f9663a7fb23641146400900a7056e8a57889b803d272c3c411e","planned_episodes":16,"native_calls":0,"auth_access":0}\n' "$status" > .cache/behavior76/literal-r1-actual-setup-v1-exit.json
exit "$status"