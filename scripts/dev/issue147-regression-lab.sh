#!/usr/bin/env bash
# Runs prebuilt tests only against the dedicated issue147 temporary database.
set -Eeuo pipefail
base=/tmp/butler-issue147-20261025
network=$(docker inspect butler-issue147-postgres | jq -r '.[0].NetworkSettings.Networks|keys[0]')
docker run --rm --name butler-issue147-regression --user "$(id -u):$(id -g)" --network "$network" --cap-drop ALL --security-opt no-new-privileges:true --tmpfs /runbin:rw,exec,size=160m,mode=1777 \
  -e BUTLER_TEST_MEMORY_POSTGRES_URL=postgres://issue147:synthetic-issue147@butler-issue147-postgres:5432/issue147_test?sslmode=disable \
  -e BUTLER_TEST_COMMUNICATION_POSTGRES_URL=postgres://issue147:synthetic-issue147@butler-issue147-postgres:5432/issue147_test?sslmode=disable \
  -v "$base/bin:/bin-output:ro" -v "$base/migrations:/work/github.com/butler/butler/migrations:ro" -w /work \
  golang:1.27.2 bash -eu -c 'cp /bin-output/orchestrator.test /bin-output/communication.test /runbin/; chmod u+x /runbin/*.test; /runbin/orchestrator.test -test.v -test.timeout=3m; /runbin/communication.test -test.v -test.timeout=3m -test.run "TestCommunicationWidgetResponseCapturedByCatalogAndRevokedIntegration|TestCommunicationOwnerResumeNewRunRetainsCanonicalAnswerIntegration"' \
  > "$base/reports/regression.log" 2>&1
grep -q '^--- PASS: TestCommunicationOwnerResumeNewRunRetainsCanonicalAnswerIntegration' "$base/reports/regression.log"
echo 'Isolated PostgreSQL regression passed.'
