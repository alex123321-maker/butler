# Read-only operator accounting, never a model/provider retry or Core runtime.
import json,pathlib,sys,urllib.request,urllib.error,hashlib,re,datetime
class NoRedirect(urllib.request.HTTPRedirectHandler):
 def redirect_request(self,*args,**kwargs):return None
key=sys.stdin.buffer.read(4097).strip()
if not 20<len(key)<=4096:raise SystemExit('private operator input unavailable')
r=pathlib.Path('/srv/gitea/butler-acceptance/issue115-utility-native-workflow-luna-max-speed-20261102-r1')
out=r/'retrieval-accounting.json'
if out.exists():raise SystemExit('one-use accounting read already sealed')
items=[]
for p in sorted((r/'out/actual').glob('slot-*/native-*.private.json')):
 n=json.loads(p.read_text())
 if not n['dispatched'] or n['usage'] is not None:continue
 refs={e['ProviderSessionRef']['ResponseRef'] for e in n['events'] if e.get('ProviderSessionRef') and e['ProviderSessionRef'].get('ResponseRef')}
 assert len(refs)==1
 ref=refs.pop();assert re.fullmatch(r'resp_[A-Za-z0-9_-]{1,240}',ref)
 items.append((n,p,ref))
assert len(items)==3
opener=urllib.request.build_opener(urllib.request.ProxyHandler({'https':'http://127.0.0.1:18080'}),NoRedirect())
receipt={'native_generation_requests':0,'api_read_requests':0,'original_unknown_and_scores_unchanged':True,'retrieved_at':datetime.datetime.now(datetime.timezone.utc).isoformat(),'rows':[]}
for n,p,ref in items:
 row={'slot':n['slot'],'turn':n['turn'],'operation_id':n['operation_id'],'raw_sha256':hashlib.sha256(p.read_bytes()).hexdigest(),'response_id_sha256':hashlib.sha256(ref.encode()).hexdigest(),'usage':None}
 req=urllib.request.Request('https://api.openai.com/v1/responses/'+ref,headers={'Authorization':'Bearer '+key.decode(),'Accept':'application/json'},method='GET')
 receipt['api_read_requests']+=1
 try:
  with opener.open(req,timeout=20) as response:
   row['http_status']=response.status
   data=response.read(8*1024*1024+1)
  assert len(data)<=8*1024*1024
  value=json.loads(data);assert value.get('id')==ref and value.get('model')=='gpt-6-luna'
  row['response_body_sha256']=hashlib.sha256(data).hexdigest()
  row['status']=value.get('status');row['model']=value.get('model');row['service_tier']=value.get('service_tier')
  usage=value.get('usage')
  if row['status'] in ('completed','incomplete','failed','cancelled') and isinstance(usage,dict):
   assert all(type(usage.get(k)) is int and usage[k]>=0 for k in ('input_tokens','output_tokens','total_tokens'))
   assert usage['total_tokens']==usage['input_tokens']+usage['output_tokens']
   row['usage']={k:usage[k] for k in ('input_tokens','output_tokens','total_tokens')}
   for k in ('input_tokens_details','output_tokens_details'):
    if isinstance(usage.get(k),dict):row['usage'][k]={a:b for a,b in usage[k].items() if type(b) is int and b>=0}
 except urllib.error.HTTPError as e:row['http_status']=e.code;row['read_failure']='http_rejection'
 except Exception as e:row['read_failure']=type(e).__name__
 receipt['rows'].append(row)
 if row.get('http_status') in (401,403,429):break
key=b''
with out.open('x') as f:json.dump(receipt,f,indent=2)
print(json.dumps({'api_read_requests':receipt['api_read_requests'],'recovered_usage':sum(x['usage'] is not None for x in receipt['rows']),'http_statuses':[x.get('http_status') for x in receipt['rows']],'native_generation_requests':0}))
