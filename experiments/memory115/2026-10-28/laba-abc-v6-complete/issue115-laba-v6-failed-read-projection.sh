#!/bin/sh
set -eu
base=/srv/gitea/butler-acceptance/issue115-native-20261028-v1
root="$base/review-snapshot-abc-v6/actual"
for seq in 016 018 032; do
 run=$(jq -r '.result.RunID // .result.run_id' "$root/case-$seq-outcome.json")
 jq --arg run "$run" --arg seq "$seq" '{sequence:$seq,unknowns:[.action_effects[]|select(.state=="effect_unknown" and .initial_binding.run_id==$run)|{action_id,tool:.initial_binding.tool_name,effect:.initial_binding.effect,policy,state,dispatch_outcome}],failed_search:[.tool_calls[]|select(.run_id==$run and .tool_name=="memory.search" and .status!="completed")|{args,status,error}],input:[.accepted_inputs[]|select(.run_id==$run)|{status,error_code,error_message,attempts,max_attempts}]}' "$root/case-$seq-database.json"
done | jq -s '{version:"butler115.native-failed-read-observations.v1",cases:.,root_cause:"unresolved after captured validation error",outcomes_rewritten:false,new_inference:false}' > "$base/review-output-abc-v6/failed-memory-search-observations.json"