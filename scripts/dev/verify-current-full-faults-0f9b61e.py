import pathlib, json, hashlib, collections, stat, os, datetime, argparse

parser=argparse.ArgumentParser(description='Read-only current0f mandatory-fault source/event/binary correlation. No model/authentication/PG queries.')
parser.add_argument('--output-directory',help='Optional named review directory; outputs use exclusive0600 creation. Default only prints a verification summary.')
args=parser.parse_args()

BASE = pathlib.Path('/srv/storage/butler/agents')
ROOT = BASE/'scenario-release-chain-v10-final-faults-20261013'
SRC = ROOT/'snapshots/20261013-235009-202'
PROOF = ROOT/'reports/fault-20261013T170617Z-2668214'
NATIVE_SRC = BASE/'scenario-release-chain-v10-high-20261013/snapshots/20261013-234239-392'
OLD = BASE/'scenario-v6-final-faults-20261013/reports/fault-20261013T103727Z-1467275'
COMMIT = '0f9b61e23fd56fefddd1c42cd3aa9efa14087d83'
TREE = '2096a0a5f7a80ad8a912dc0df1f7b2452053d68b27b55c6abdd783af6ce96879'
SCHEMA = 'f3673fd3db28bd98026f9bbbcc5d8e5f9ab06de32055602f3ad94b859521647d'
CRITERIA = '4329f8a374e610db78f7356e1b8e04304402d5bc0d5ff4086c1ff76505b16358'
QUALITY = '76437b99d5eb79baadb39c5eb6d7958a2d6108296c7b6dc6c6118f42b21cd058'

def sha(b): return hashlib.sha256(b).hexdigest()
def read(p):
    assert not p.is_symlink() and stat.S_ISREG(p.stat().st_mode)
    return p.read_bytes()
def files(root):
    out = {}
    for p in root.rglob('*'):
        assert not p.is_symlink()
        if p.is_file(): out[p.relative_to(root).as_posix()] = sha(read(p))
    return out
def tree(f): return sha(''.join(p+'\0'+f[p]+'\n' for p in sorted(f)).encode())
def hashfile(p):
    assert not p.is_symlink() and stat.S_ISREG(p.stat().st_mode)
    h=hashlib.sha256()
    with p.open('rb') as s:
        while b:=s.read(1024*1024): h.update(b)
    return {'sha256':h.hexdigest(),'bytes':p.stat().st_size,'mode':oct(stat.S_IMODE(p.stat().st_mode))}

source = files(SRC)
assert source == files(NATIVE_SRC) and len(source)==2280 and tree(source)==TREE
manifest = ''.join(source[p]+'  ./'+p+'\n' for p in sorted(source)).encode()
assert read(PROOF/'source-manifest.sha256')==manifest
assert sha(manifest)=='830204cc9f031430e20280bbf63136a340f318b3d80c536fa0c8f5deb5743543'
assert tree({p:h for p,h in source.items() if p.startswith('migrations/')})==SCHEMA
assert read(PROOF/'source-commit.txt')==(COMMIT+'\n').encode()
raw=read(PROOF/'report.json'); report=json.loads(raw)
assert sha(raw)=='c2f3787b0940952138861e5a1211a05a8788841128661469ae7068b0422af5f4' and len(raw)==41960
oldraw=read(OLD/'report.json'); old=json.loads(oldraw)
assert sha(oldraw)=='900da5cce76ea8663f28d4c04543d2da2f49cf7047f106230fa6a7bb70deb047'
assert report['code_commit']==COMMIT and report['source_tree_sha256']==TREE and report['schema_sha256']==SCHEMA
assert report['criteria_sha256']==CRITERIA and report['signed_quality_criteria_sha256']==QUALITY
assert report['mandatory_faults_passed'] is True and report['issue_33_ready'] is False
assert report['scenarios']==old['scenarios'] and len(report['scenarios'])==8
assert len(report['groups'])==35 and len({g['id'] for g in report['groups']})==35
def shape(g): return (g['id'],g['runtime'],tuple(t['name'] for t in g.get('tests',[])))
assert [shape(g) for g in report['groups']]==[shape(g) for g in old['groups']]
old_manifest={}
for line in read(OLD/'source-manifest.sha256').decode().splitlines():
    digest, path=line.split('  ',1)
    old_manifest[path.removeprefix('./')]=digest
criteria_source_paths=('internal/acceptance/matrix.go','internal/acceptance/full_faults.go','internal/acceptance/local_load_oom.go','internal/acceptance/local_load_witness.go','apps/orchestrator/internal/input/budget_integration_test.go')
for p in criteria_source_paths: assert source[p]==old_manifest[p]

hardware_raw=read(PROOF/'hardware.json'); hardware=json.loads(hardware_raw)
assert hardware==report['hardware'] and sha(hardware_raw)=='c34b4a4d3ece3da2079ce128aa8484b9ecdabb6b3160f722b57c10296d108977'
assert sha(read(PROOF/'hardware-cpu.json'))==hardware['cpu_manifest_sha256']=='7ef32970c3534eec4c10fd9dc8d3206f0db5ccee56c0bcb150f5de161418199e'
assert hardware['go_image']=='sha256:5cf287a799e6b94384bad13d16b14904c531f51ba65792237e122ce42b392f61'
assert hardware['postgres_image']=='sha256:ac08538c6f8b9904c33c8224c5e5706dbe760aca29db1d096972b4052c22a75d'
assert hardware['redis_image']=='sha256:858f009f9709ce576febc734aa78b8f6d624b82571f9ddb6bda4377c833b3499'
assert hardware['browser_image']=='sha256:2819ba6ab15df584366426f60c3db37141a6fa6b3aa532f4f074f306f5f25998'
pinraw=read(PROOF/'events/fault-run-pin.json'); pin=json.loads(pinraw)
assert sha(pinraw)=='6370a0d2a86b01cc0c0228b4817fb0c3612b37d822f4a3411e7d0e0df89c8c60'
assert pin==dict(version='butler-mandatory-faults-v3',code_commit=COMMIT,source_tree_sha256=TREE,schema_sha256=SCHEMA,criteria_sha256=CRITERIA,hardware_sha256=sha(hardware_raw))
allpasses=[]; selected=[]; groupmeta=[]; deadline=None
for g in report['groups']:
    assert g['status']=='PASS'
    if g['id'] in ('release-pin','model-load-oom'): continue
    leaf=PROOF/('events/'+g['id']+'.jsonl')
    eventsraw=read(leaf); assert sha(eventsraw)==g['events_sha256']
    events=[json.loads(line) for line in eventsraw.splitlines() if line]
    assert not any(e['Action'] in ('fail','skip') for e in events)
    passed=[e for e in events if e['Action']=='pass' and e.get('Test')]
    assert len(passed)==len({e['Test'] for e in passed})
    byname={e['Test']:e for e in passed}
    for t in g['tests']:
        assert t['status']=='PASS' and t['name'] in byname and byname[t['name']].get('Elapsed',0)==t['seconds']
        selected.append((g['id'],t['name']))
        if t['name']=='TestRuntimeBudgetDeadlineCancelsIdleInferenceIntegration': deadline={'group':g['id'],'name':t['name'],'elapsed_seconds':t['seconds'],'raw_event_action':byname[t['name']]['Action']}
    allpasses.extend((g['id'],e['Test']) for e in passed)
    groupmeta.append({'id':g['id'],'sha256':sha(eventsraw),'bytes':len(eventsraw),'selected_names':[t['name'] for t in g['tests']],'all_named_passes':len(passed),'failed':0,'skipped':0})
assert len(groupmeta)==33 and len(allpasses)==321 and len(selected)==176 and deadline
assert set(p.name for p in (PROOF/'events').iterdir() if p.is_file())=={g['id']+'.jsonl' for g in report['groups'] if g['id'] not in ('release-pin','model-load-oom')}|{'fault-run-pin.json'}
oomraw=read(PROOF/'local-load-oom.json'); oom=json.loads(oomraw)
assert sha(oomraw)=='23fc8f4cab1d105333d3057f136008d7ce0964feba7fbea0bec222c00c90b234'
assert oomraw==read(OLD/'local-load-oom.json') and report['local_model_load_oom']==old['local_model_load_oom']
w=report['local_model_load_witness']; assert w==old['local_model_load_witness']
assert sha(read(PROOF/'public-weight-loader.log'))==w['loader_log_sha256']=='2303783bdb57a79266815f68908a10b053107acaebd3485badce8bbb7cf0d5ab'
assert w['original_strict_gate']=='FAIL' and w['actual_inference_calls']==0 and w['actual_native_calls']==0 and w['butler_kernel_received_oom'] is False and w['complete_weights_loaded'] is False
assert oom['oom_killed'] is True and oom['model_metadata_read'] is False and oom['memory_bytes']==268435456
bins={name:hashfile(PROOF/'bin'/name) for name in ('acceptance','app.test','broker.test','browser.test','extension.test','guarded-native-host','test2json','tool-broker')}
assert set(p.name for p in (PROOF/'bin').iterdir())==set(bins)
result={'version':'butler-independent-current-full-fault-review-v1','status':'CURRENT_0F_MANDATORY_FAULTS_VERIFIED','verified_utc':datetime.datetime.now(datetime.timezone.utc).isoformat(),'source_commit':COMMIT,'source_tree_sha256':TREE,'source_files':len(source),'source_manifest_sha256':sha(manifest),'schema_sha256':SCHEMA,'criteria_sha256':CRITERIA,'retained_quality_criteria_sha256':QUALITY,'actual_fault_source_equals_measured_v10_source':True,'report_sha256':sha(raw),'report_bytes':len(raw),'pretest_release_pin_sha256':sha(pinraw),'hardware_sha256':sha(hardware_raw),'hardware':hardware,'raw_group_artifacts':groupmeta,'all_named_passes':len(allpasses),'selected_required_names':len(selected),'named_failures':0,'named_skips':0,'mandatory_scenarios':report['scenarios'],'restored_runtime_deadline_cancellation':deadline,'binaries':bins,'fault_script_sha256':source['scripts/dev/fault-lab.sh'],'retained_physical_oom':{'original_report_sha256':sha(oomraw),'same_retained_report_as_73':True,'original_strict_gate':'FAIL','physical_scope':'cached_Qwen3_CPU_LOAD_before_inference','high_remote_RAM_or_weights_measured':False,'butler_kernel_received_physical_oom':False,'complete_weights_loaded':False},'mandatory_faults_passed':True,'issue33_ready':False,'native_chain_verified_cases':0,'limitations':['Read-only source/event/binary hash correlation; no rebuilt binary attestation or independent administrator isolation.','Fault harness records image IDs then invokes local fixed tags; this review does not claim an independent build-image attestation.','Synthetic effects and actual isolated Chrome/PG/backup controls are not five completed native High release scenarios.','The retained physical CPU LOAD OOM is an earlier sealed experiment, not a new High RAM observation.','V11 future fixture-only observation delta requires its own narrow preparation and source comparison.']}
if args.output_directory:
    destination=pathlib.Path(args.output_directory)
    assert destination.is_absolute() and destination.parent==pathlib.Path('/srv/storage/butler/reviews') and destination.name.startswith('issue33-')
    assert not destination.is_symlink()
    destination.mkdir(mode=0o700,exist_ok=True)
    assert destination.is_dir() and stat.S_IMODE(destination.stat().st_mode)==0o700
    for name,content in [('current-0f9b61e-full-fault-report-20261013.json',raw),('current-0f9b61e-full-fault-independent-review-20261013.json',(json.dumps(result,ensure_ascii=False,indent=2)+'\n').encode())]:
        p=destination/name
        fd=os.open(p,os.O_CREAT|os.O_EXCL|os.O_WRONLY,0o600)
        with os.fdopen(fd,'wb') as f: f.write(content);f.flush();os.fsync(f.fileno())
        print(json.dumps({'path':str(p),'bytes':len(content),'sha256':sha(content),'mode':oct(stat.S_IMODE(p.stat().st_mode))}))
print(json.dumps({'status':result['status'],'source_commit':COMMIT,'source_tree_sha256':TREE,'source_manifest_sha256':sha(manifest),'report_sha256':sha(raw),'report_bytes':len(raw),'all_named_passes':321,'selected_required_names':176,'raw_go_leaves':33,'mandatory_scenarios':8,'binary_count':len(bins),'failures':0,'skips':0,'issue33_ready':False,'output_created':bool(args.output_directory)}))
