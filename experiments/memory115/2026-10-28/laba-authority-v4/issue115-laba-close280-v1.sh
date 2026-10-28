#!/bin/sh
set -eu
umask 077
base=/srv/gitea/butler-acceptance/issue115-native-20261028-v1
closed="$base/closed-laba280-v1"
test ! -e "$closed"
mkdir "$closed" "$closed/plan"
cp -a "$base/review-snapshot-abc-v3/actual" "$closed/actual"
cp -a "$base/history-abc-v3" "$closed/history"
chmod u+w "$closed/history"
mv "$closed/history/recipe.json" "$closed/history/history.json"
cp "$base/review-snapshot-abc-v3/preseal.json" "$closed/plan/preseal.json"
cp "$base/review-snapshot-abc-v3/actual/schedule.json" "$closed/plan/schedule.json"
cp "$base/review-snapshot-abc-v3/runner.log" "$closed/runner.log"
cp "$base/review-snapshot-abc-v3/server-handback.json" "$closed/closed-handback.json"
cp "$base/review-snapshot-abc-v3/process-exit.json" "$closed/process-exit.json"
cp "$base/private-native-raw-inventory-abc-v3.sha256" "$closed/sealed-native-raw-inventory.sha256"
cd "$closed"
find actual -type f | LC_ALL=C sort | while IFS= read -r name; do sha256sum "$name"; done > actual-files.sha256
jq -Rn '{format:"butler.experience.private-artifacts.v1",files:[inputs|capture("^(?<sha256>[a-f0-9]{64})  (?<path>actual/.*)$")]}' < actual-files.sha256 > private-artifacts.json
jq '{slots,denominator:57,original_final_window_present:true}' actual/window-final.json > all-original-outcomes.json
for name in plan/preseal.json plan/schedule.json history/history.json private-artifacts.json all-original-outcomes.json closed-handback.json process-exit.json runner.log sealed-native-raw-inventory.sha256; do sha256sum "$name"; done > reference-files.sha256
jq -Rn '[inputs|capture("^(?<sha256>[a-f0-9]{64})  (?<path>.*)$")]' < reference-files.sha256 > references.json
jq -n --slurpfile cost "$base/review-output-abc-v3/cost-review.json" --slurpfile plan plan/preseal.json --slurpfile refs references.json '{format:"butler.experience.closed-laba-owner-leaf.v2",owner:$plan[0].owner,day:$plan[0].history.day,deadline:$plan[0].deadline,preseal_sha256:$cost[0].plan_sha256,previous_history_sha256:$cost[0].history_sha256,denominator:57,maximum_new_entries:640,cost_population_complete:true,inclusive_carry:$cost[0].quotes.inclusive_quote,usage:{New:$cost[0].new_usage,Inclusive:$cost[0].inclusive_usage},references:$refs[0],release_qualified:false,independent_quality_review_required:true,invoice_cost:"unknown",control_plane_handback_is_inference:false}' > closed-leaf.json
chmod -R a-w "$closed"
sha256sum closed-leaf.json
jq '{finished_at,slot_counts:(.slots|group_by(.status)|map({status:.[0].status,count:length}))}' actual/window-final.json
