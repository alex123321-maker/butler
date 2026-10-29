import json,subprocess,pathlib
sql="""SELECT jsonb_agg(jsonb_build_object('source_id',l.source_id,'body',r.body)) FROM memory_experience_lessons l JOIN memory_sources s USING(source_id) JOIN memory_source_revisions r ON r.source_id=s.source_id AND r.revision=s.head_revision;"""
p=subprocess.run(['docker','exec','-i','issue115-c43-pg-20261029-v1','psql','-X','-q','-U','postgres','-d','memory115_c43_test','-At','-v','ON_ERROR_STOP=1'],input=sql,text=True,capture_output=True,check=True)
items=[]
for row in json.loads(p.stdout):
 b=row['body'];l=json.loads(b);obs=l['observation'];n=l['model_notes']
 items.append({'source_id':row['source_id'],'body_chars':len(b),'notes_chars':len(json.dumps(n,ensure_ascii=False,separators=(',',':'))),'notes_position':b.find('"model_notes"'),'observation_position':b.find('"observation"'),'owner_verdict':obs['owner_verdict'],'verification':l['verification'],'notes':n})
sql="SELECT jsonb_agg(jsonb_build_object('digest',v.digest,'body',r.body)) FROM skill_versions v JOIN memory_sources s ON s.source_id=v.source_ref_json->>'source_id' JOIN memory_source_revisions r ON r.source_id=s.source_id AND r.revision=s.head_revision;"
p=subprocess.run(['docker','exec','-i','issue115-c43-pg-20261029-v1','psql','-X','-q','-U','postgres','-d','memory115_c43_test','-At','-v','ON_ERROR_STOP=1'],input=sql,text=True,capture_output=True,check=True)
skills=[]
for row in json.loads(p.stdout) or []:
 m=json.loads(row['body']);skills.append({'digest':row['digest'],'body_chars':len(row['body']),**{k:m[k] for k in ['description','when','not_applicable']}})
out=pathlib.Path('/srv/gitea/butler-acceptance/issue115-c43-stale-20261029-v1/reports/lesson-presentation-audit.json');out.write_text(json.dumps({'new_model_requests':0,'lessons':items,'skills':skills},ensure_ascii=False,indent=2)+'\n')
print('SKILLS',json.dumps(skills,ensure_ascii=False,indent=2))
print(json.dumps(items,ensure_ascii=False,indent=2))
