"""Exclusive, model-free diagnostic composition from frozen source; no production edits."""
import hashlib, json, pathlib, shutil, subprocess, os, ipaddress
base=pathlib.Path('/srv/gitea/butler-acceptance')
r=base/'issue115-skill-latency-20261102-r2'
payload=base/'skill-latency-input-20261102'
if os.environ.get('LATENCY_RESUME_NETWORK') != '1':
 r.mkdir();(r/'reports').mkdir();(r/'tmp').mkdir();(r/'pgdata').mkdir()
 source=base/'issue115-utility-trace-workflow-20261030-r3/source'
 subprocess.run(['cp','-a','--reflink=auto',str(source),str(r/'source')],check=True)
 payload=base/'skill-latency-input-20261102'
 app=r/'source/apps/orchestrator/internal/app'
 shutil.copy2(payload/'latency_diagnostic_test.go.txt',app/'latency_diagnostic_test.go')
 p=app/'memory_experience_native_runtime_test.go';s=p.read_text();needle='fc.Skills = r.skillClient';assert s.count(needle)==1;p.write_text(s.replace(needle,'fc.Skills = latencyWrapCatalog(r.skillClient)'))
 p=app/'memory_utility_whole_test.go';s=p.read_text();needle='started := time.Now()\n\tout := r.execute';assert s.count(needle)==1
 p.write_text(s.replace(needle,'latencyActive = &latencyStats{}\n\tprovider = &latencyProvider{utilityWorkflowProvider: provider.(*utilityWorkflowProvider), fragments: latencyFragments}\n\tstarted := time.Now()\n\tout := r.execute'))
 image='sha256:5cf287a799e6b94384bad13d16b14904c531f51ba65792237e122ce42b392f61'
 pgimage='sha256:ac08538c6f8b9904c33c8224c5e5706dbe760aca29db1d096972b4052c22a75d'
 build=['docker','run','--rm','--network','none','--cpus','1','--memory','3g','--memory-swap','3g','--read-only','--cap-drop','ALL','--user','1000:1000','--tmpfs','/tmp:rw,size=256m','-v',str(r/'source')+':/source:ro','-v',str(r/'reports')+':/reports','-v',str(r/'tmp')+':/build-tmp','-v',str(base/'issue115-utility-trace-repair-20261030-r4/cache')+':/cache','-v','/srv/storage/butler/.cache/go-mod:/modules:ro','-e','GOCACHE=/cache','-e','GOTMPDIR=/build-tmp','-e','GOMODCACHE=/modules','-e','GOPROXY=off','-e','GOSUMDB=off','-e','GOTOOLCHAIN=local','-e','GOMAXPROCS=1','-e','GOFLAGS=-p=1 -buildvcs=false -mod=readonly','-w','/source']
 with (r/'reports/build.log').open('x') as f:
  subprocess.run(build+[image,'sh','-eu','-c','go test -race -c -o /reports/app.test ./apps/orchestrator/internal/app'],stdout=f,stderr=subprocess.STDOUT,check=True)
else:
 assert (r/'reports/app.test').exists() and not (r/'reports/tests.log').exists()
 image='sha256:5cf287a799e6b94384bad13d16b14904c531f51ba65792237e122ce42b392f61'
 pgimage='sha256:ac08538c6f8b9904c33c8224c5e5706dbe760aca29db1d096972b4052c22a75d'
net=r.name+'-net';pg=r.name+'-pg'
subnets=json.loads(subprocess.check_output(['docker','network','inspect',*subprocess.check_output(['docker','network','ls','-q'],text=True).split()]))
candidate=ipaddress.ip_network('10.249.121.0/28')
assert all(not candidate.overlaps(ipaddress.ip_network(c['Subnet'])) for n in subnets for c in (n['IPAM']['Config'] or []) if c.get('Subnet') and ipaddress.ip_network(c['Subnet']).version==4)
subprocess.run(['docker','network','create','--internal','--subnet',str(candidate),net],check=True,stdout=subprocess.DEVNULL)
status=1
try:
 subprocess.run(['docker','run','-d','--name',pg,'--network',net,'--cpus','1','--memory','768m','-e','POSTGRES_USER=utility_fixture','-e','POSTGRES_PASSWORD=utility_fixture_only','-e','POSTGRES_DB=utility_trace_test','-v',str(r/'pgdata')+':/var/lib/postgresql/data',pgimage],check=True,stdout=subprocess.DEVNULL)
 import time
 for _ in range(40):
  if subprocess.run(['docker','exec',pg,'pg_isready','-h','127.0.0.1','-U','utility_fixture'],stdout=subprocess.DEVNULL,stderr=subprocess.DEVNULL).returncode==0:break
  time.sleep(1)
 gate=base/'issue115-utility-trace-workflow-20261030-r3/reports'
 cmd=['docker','run','--rm','--network','container:'+pg,'--cpus','1','--memory','3g','--memory-swap','3g','--read-only','--cap-drop','ALL','--user','1000:1000','--tmpfs','/tmp:rw,size=512m','-v',str(r/'source')+':/source:ro','-v',str(r/'reports')+':/reports','-v',str(gate)+':/binproof:ro','-v',str(base/'issue115-utility-native-workflow-terra-20261030-r1')+':/historical:ro','-e','BUTLER_TEST_MEMORY_POSTGRES_URL=postgres://utility_fixture:utility_fixture_only@127.0.0.1:5432/utility_trace_test?sslmode=disable','-e','BUTLER_BROKER_E2E_BINARY=/binproof/broker','-e','BUTLER_EXTENSION_E2E_BINARY=/binproof/extension.test','-e','BUTLER_EXPERIENCE_HTTP_BINARY=/binproof/http.test','-e','BUTLER_EXTENSION_E2E=1','-e','BUTLER_LATENCY_OUTPUT=/reports/actual','-e','GOMAXPROCS=1','-w','/source/apps/orchestrator/internal/app',image,'/reports/app.test','-test.v','-test.timeout=15m','-test.run','^TestSkillLatency(Diagnostic|JournalReplay)$']
 with (r/'reports/tests.log').open('x') as f:
  result=subprocess.run(cmd,stdout=f,stderr=subprocess.STDOUT);status=result.returncode
finally:
 subprocess.run(['docker','stop','-t','15',pg],stdout=subprocess.DEVNULL,check=True)
 subprocess.run(['docker','network','disconnect',net,pg],check=True)
 subprocess.run(['docker','network','rm',net],stdout=subprocess.DEVNULL,check=True)
 (r/'reports/exit.json').write_text(json.dumps({'exit_code':status,'native_requests':0,'private_pg_stopped':True,'private_data_retained':True,'internal_network_removed':True,'source':'66be595bc122ea5a6e02def4689b92b70945fc3a','overlay_sha256':hashlib.sha256((payload/'latency_diagnostic_test.go.txt').read_bytes()).hexdigest()})+'\n')
raise SystemExit(status)
