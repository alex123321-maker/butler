#!/usr/bin/env python3
"""Read-only arithmetic from original validated receipts, not a quality oracle."""
import json
from datetime import datetime
from pathlib import Path

base = Path('/srv/gitea/butler-acceptance/issue115-native-20261028-v1')
directory = base / 'review-output-abc-v6'
read = lambda p: json.loads(p.read_bytes())
accounting = read(directory / 'cost-review.json')
facts = read(directory / 'abc-facts.json')
window = read(base / 'review-snapshot-abc-v6/actual/window-final.json')
assert facts['all24_owning_outcomes_present'] and len(facts['cases']) == 24
assert accounting['original_cost_and_journal_validators_passed']
groups = facts['shared_and_arm_native_costs']
fields = ('entries', 'unknown_usage', 'missing_total', 'known_full_tokens', 'input_tokens', 'output_tokens', 'model_millis')

def add(*values):
    return {field: sum(value.get(field, 0) for value in values) for field in fields}

total = add(*groups.values())
assert total['entries'] == accounting['new_usage']['native_entries']
assert total['known_full_tokens'] == accounting['new_usage']['known_full_token_lower_bound']
complete_usage = not (total['unknown_usage'] or total['missing_total'] or accounting['new_usage']['product_without_native_entry'])
elapsed = {}
for slot in window['slots'][:33]:
    assert slot['started_at'] and slot['finished_at']
    group = slot['kind'] if slot['kind'] != 'control' else slot['arm']
    start, end = [datetime.fromisoformat(slot[k].replace('Z', '+00:00')) for k in ('started_at', 'finished_at')]
    elapsed[group] = elapsed.get(group, 0) + round((end-start).total_seconds()*1000)

S, N, K = [groups[k] for k in ('original', 'note', 'skill_preparation')]
arms = {}
for prefix in ('A', 'B', 'C'):
    name = next(name for name in groups if name.startswith(prefix+'_'))
    allocated = add(S, groups[name], *([N] if prefix in ('B', 'C') else []), *([K] if prefix == 'C' else []))
    arms[prefix] = {'control_tasks': 8, 'query_use_native': groups[name], 'full_allocated_native': allocated,
                    'query_use_elapsed_millis': elapsed[name],
                    'full_allocated_elapsed_millis': elapsed['original'] + elapsed[name] +
                        (elapsed['note'] if prefix in ('B', 'C') else 0) + (elapsed['skill_preparation'] if prefix == 'C' else 0)}

delta = {label: (arms[left]['full_allocated_native']['known_full_tokens'] - arms[right]['full_allocated_native']['known_full_tokens']) if complete_usage else None
         for label, left, right in [('B_minus_A', 'B', 'A'), ('C_minus_B', 'C', 'B')]}
result = {'version': 'butler115.validated-cost-arithmetic.v1', 'plan_sha256': facts['plan_sha256'],
          'shared_preparation': {'S_originals': S, 'N_notes': N, 'K_skill': K}, 'arms': arms,
          'full_allocated_token_differences': delta, 'new_unique_native_cost': total,
          'new_full_token_totals_measured': complete_usage, 'invoice_cost': 'unknown',
          'inclusive_all_failed_and_current_usage': accounting['inclusive_usage'],
          'all_unique_reserved_quotes': accounting['quotes'], 'window_elapsed_millis': window['elapsed_millis'],
          'omitted_second_repetition_controls': 24, 'selected_control_outcomes': 24,
          'policy': 'S+QA; S+N+QB; S+N+K+QC. Unique spend counts S,N,K once. Allocated arm costs overlap.',
          'monetary_ratios': None, 'independent_quality_qualified': False, 'new_native_admission': False}
with (directory / 'cost-comparison.json').open('x') as f:
    json.dump(result, f, ensure_ascii=False, sort_keys=True, indent=2)
    f.write('\n')
print(json.dumps({'new_full_tokens': total['known_full_tokens'], 'measured': complete_usage, 'full_allocated_tokens': {k: v['full_allocated_native']['known_full_tokens'] for k,v in arms.items()}, 'deltas': delta}))
