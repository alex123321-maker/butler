#!/bin/sh
set -eu
umask 077
r=/srv/gitea/butler-acceptance/issue115-native-20261028-v1
p=$r/skill-broker-20261029-v1
pg=issue115-native-pg-20261028-v1
test ! -e "$p/safety-exit.json"
test "$(docker inspect -f '{{.Id}}' "$pg")" = "$(cat "$r/native-pg-container.txt")"
test "$(docker inspect -f '{{.State.Running}}' "$pg")" = false
cp "$r/prepared-abc-v6/tool-broker" "$p/bin/tool-broker"
cp "$r/prepared-abc-v6/extension.test" "$p/bin/extension.test"
cp "$r/review-output-abc-v6/cost-review.json" "$p/previous/abc.json"
cp "$r/skill-force-20261028-v1/out/native-result.json" "$p/previous/force.json"
cp "$r/skill-prompt-20261029-v1/out/result.json" "$p/previous/prompt.json"
cp "$r/skill-context-20261029-v1/out/result.json" "$p/previous/context.json"
chmod 0555 "$p/bin/tool-broker" "$p/bin/extension.test"
docker start "$pg" > /dev/null
trap 'docker stop -t 15 "$pg" > /dev/null' EXIT
for n in 1 2 3 4 5; do
  if docker exec "$pg" pg_isready -U postgres -p 55451 > /dev/null; then break; fi
  sleep 1
done
docker exec "$pg" createdb -U postgres -p 55451 skill_broker_safety_v1_test
docker exec "$pg" createdb -U postgres -p 55451 skill_broker_prompt_v1_test
set +e
docker run --rm --name skill-broker-safety-20261029-v1 --network host --cpus 1 --memory 2g --memory-swap 2g --pids-limit 256 --read-only --cap-drop ALL --security-opt no-new-privileges --log-driver none --user 1000:1000 --tmpfs /tmp:rw,nosuid,nodev,size=128m --mount type=bind,src="$r/source-abc-v6",dst=/source,readonly --mount type=bind,src="$p/bin",dst=/binproof,readonly -e BUTLER_EXTENSION_E2E=1 -e BUTLER_EXTENSION_E2E_BINARY=/binproof/extension.test -e BUTLER_BROKER_E2E_BINARY=/binproof/tool-broker -e BUTLER_TEST_ACTIONAUTH_POSTGRES_URL='postgres://postgres@127.0.0.1:55455/skill_broker_safety_v1_test?sslmode=disable' -w /source sha256:5cf287a799e6b94384bad13d16b14904c531f51ba65792237e122ce42b392f61 /binproof/app.test -test.run='^TestReviewedSkillRealBrokerDurableApprovalE2E$' -test.v -test.timeout=180s > "$p/safety.log" 2>&1
status=$?
set -e
printf '{"exit_code":%s,"native_calls":0,"auth_mounts":0,"real_broker_runtime_recipient":true}\n' "$status" > "$p/safety-exit.json"
cat "$p/safety.log"
exit "$status"
