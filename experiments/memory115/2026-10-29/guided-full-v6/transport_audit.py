"""Only fixed diagnostic vocabulary; never error messages/provider details."""
import json
from pathlib import Path
r=Path('/srv/gitea/butler-acceptance/issue115-native-20261028-v1')
p=r/'review-snapshot-guided-full-v6/actual'
window=json.loads((p/'window-final.json').read_bytes())
slots={s['id']:s for s in window['slots']}
types={'provider_unavailable','transport_connection_error','provider_timeout','provider_protocol_error','invalid_tool_request','internal_transport_error','rate_limited','model_capacity_unavailable','memory_source_stale','memory_source_check_timeout'}
reasons={'stream_json_invalid','stream_envelope_invalid','stream_usage_ambiguous','stream_usage_invalid','stream_usage_conflict','stream_model_version_invalid','stream_candidate_unsupported','stream_candidate_unexpected','stream_finish_rejected','stream_automatic_function_execution','stream_part_invalid','stream_part_unsupported','stream_thought_invalid','stream_function_ambiguous','stream_content_missing','stream_terminal_missing','stream_frame_limit','stream_function_truncated','stream_empty_answer','http_non_sse_response','http_do_failed_without_response'}
reasons.update({'response_schema_rejected','upstream_invalid_argument','upstream_bad_request_non_json','upstream_failed_precondition','upstream_model_rejected','upstream_thinking_rejected','upstream_output_limit_rejected','upstream_system_instruction_rejected','unsupported_schema_min_items','unsupported_schema_max_items','batch_tool_calls_unsupported'})
rows=[]
for f in sorted(p.glob('entry-*-end.json')):
 e=json.loads(f.read_bytes())
 if e.get('usage') is not None:continue
 diagnostics=[]
 for v in e.get('events') or []:
  err={'Type':v.get('error_code'),'SafeReason':v.get('error_reason'),'HTTPStatus':v.get('http_status',0)} if v.get('error_code') else {}
  if not err:continue
  status=err.get('HTTPStatus',0)
  diagnostics.append({'type':err.get('Type') if err.get('Type') in types else 'other','reason':err.get('SafeReason') if err.get('SafeReason') in reasons else 'other','http_status':status if isinstance(status,int) and 300<=status<=599 else 0})
 s=slots[e['slot']]
 rows.append({'native_sequence':e['sequence'],'slot_sequence':s['sequence'],'case':s['case_id'],'arm':s['arm'],'repetition':s['repetition'],'kind':e['kind'],'usage':None,'diagnostics':diagnostics})
result={'format':'butler115.native-unknown-diagnostics.v1','entries':rows,'usage_reconstructed':False,'reservation_refunded':False,'new_model_requests':0}
with (r/'review-output-guided-full-v6/native-unknown-diagnostics.json').open('x') as f:json.dump(result,f,sort_keys=True,indent=2)
print(json.dumps(result))
