"""Historical attribution only; no result, success or authority synthesized."""
def current_run(d,o):
 result=o.get('result') or {}
 if result.get('RunID'):return result['RunID']
 t=o['task']
 tasks=[x for x in d['tasks'] if x['task_id']==t['task_id'] and x['owner_id']==t['owner_id'] and x['session_key']==t['session_key'] and x['revision']>=t['revision'] and x['constraints_revision']==t['constraints_revision']]
 if len(tasks)!=1:raise ValueError('current owning task identity is not unique')
 runs={x['run_id']:x for x in d['runs'] if x['task_id']==t['task_id'] and x['session_key']==t['session_key'] and x['task_revision']==t['revision']}
 ids={x['run_id'] for x in d['accepted_inputs'] if x.get('run_id') in runs and x['task_id']==t['task_id'] and x['owner_id']==t['owner_id'] and x['session_key']==t['session_key']}
 if len(ids)!=1:raise ValueError('current input/run is not uniquely corroborated')
 return ids.pop()