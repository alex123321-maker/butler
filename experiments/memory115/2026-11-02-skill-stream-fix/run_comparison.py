"""Offline artifact orchestration only; real Core/Broker, no provider connection.

Reuse the archived diagnostic exactly, changing only skill_provider.go in AFTER.
Run each variant with a fresh internal network and PostgreSQL, serially.
"""
import hashlib, json, pathlib, subprocess

base=pathlib.Path('/srv/gitea/butler-acceptance')
payload=base/'skill-stream-fix-input-20261102'
old=payload/'run_lab.py'
for variant in ('before','after'):
    script=old.read_text()
    assert "r=base/'issue115-skill-latency-20261102-r2'" in script
    script=script.replace("r=base/'issue115-skill-latency-20261102-r2'", "r=base/'issue115-skill-stream-fix-20261102-"+variant+"-r4'")
    script=script.replace("payload=base/'skill-latency-input-20261102'", "payload=base/'skill-stream-fix-input-20261102'")
    if variant=='after':
        needle="app=r/'source/apps/orchestrator/internal/app'"
        assert script.count(needle)==1
        script=script.replace(needle,needle+"\n shutil.copy2(payload/'skill_provider.go',r/'source/apps/orchestrator/internal/orchestrator/skill_provider.go')\n shutil.copy2(payload/'skill_stream_test.go',r/'source/apps/orchestrator/internal/orchestrator/skill_stream_test.go')")
    needle='go test -race -c -o /reports/app.test ./apps/orchestrator/internal/app'
    assert script.count(needle)==1
    script=script.replace(needle,"go test -race -count=1 -run \"^(TestSkill|TestRecoveredSkills)\" ./apps/orchestrator/internal/orchestrator; go vet ./apps/orchestrator/internal/orchestrator; go test -race -c -o /reports/skills.test ./internal/skills; "+needle)
    # Replay timings first, then independent actual lifecycle/source/approval checks.
    needle="'-test.run','^TestSkillLatency(Diagnostic|JournalReplay)$'"
    assert script.count(needle)==1
    script=script.replace(needle,"'-test.run','^(TestSkillLatency(Diagnostic|JournalReplay)|TestMemoryUtilityActualCoreBrokerTraceIntegration)$'")
    script=script.replace("'-e','BUTLER_EXTENSION_E2E=1'", "'-e','BUTLER_EXTENSION_E2E=1','-e','BUTLER_UTILITY_TRACE_OFFLINE=1','-e','BUTLER_TEST_ACTIONAUTH_POSTGRES_URL=postgres://utility_fixture:utility_fixture_only@127.0.0.1:5432/utility_trace_test?sslmode=disable'")
    # The package suite opens no providers and includes PG lease/revision/deletion checks.
    needle='status=result.returncode'
    assert script.count(needle)==1
    script=script.replace(needle,needle+"\n if status==0:\n  with (r/'reports/skills-tests.log').open('x') as f:\n   extra=cmd[:cmd.index(image)+1]+['/reports/skills.test','-test.v','-test.timeout=5m']\n   status=subprocess.run(extra,stdout=f,stderr=subprocess.STDOUT).returncode")
    # This existing fixture resolves migrations relative to the repository root.
    needle="status=result.returncode"
    addition="\n if status==0:\n  with (r/'reports/reviewed-tests.log').open('x') as f:\n   extra=cmd[:cmd.index(image)+1];extra[extra.index('-w')+1]='/source'\n   status=subprocess.run(extra+['/reports/app.test','-test.v','-test.timeout=5m','-test.run','^TestReviewedSkillRealBrokerDurableApprovalE2E$'],stdout=f,stderr=subprocess.STDOUT).returncode"
    script=script.replace(needle,needle+addition)
    generated=payload/('run-'+variant+'-v5.py')
    with generated.open('x') as f:f.write(script)
    subprocess.run(['python3',str(generated)],check=True)
    r=base/('issue115-skill-stream-fix-20261102-'+variant+'-r4')
    text=(r/'reports/tests.log').read_text()
    build=(r/'reports/build.log').read_text()
    tests=(r/'reports/skills-tests.log').read_text()
    reviewed=(r/'reports/reviewed-tests.log').read_text()
    assert reviewed.rstrip().endswith('PASS')
    assert text.rstrip().endswith('PASS') and tests.rstrip().endswith('PASS')
    assert not any(mark in text+build+tests+reviewed for mark in ('DATA RACE','--- SKIP:','--- FAIL:'))
    rows=[]
    for p in sorted((r/'reports/actual').glob('metrics-*.json')):
        v=json.loads(p.read_text());slot=r/'reports/actual'/('slot-'+p.stem.split('-')[1])
        timing=json.loads((slot/'timing.json').read_text());snap=json.loads((slot/'snapshot.private.json').read_text());score=json.loads((slot/'score.json').read_text())
        assert score['task_success']
        v.update(execution_seconds=timing['execution_ns']/1e9,scripted_provider_methods=len(snap['Tables']['model_attempts']),task_success=score['task_success'])
        rows.append(v)
    summary={'variant':variant,'rows':rows,'native_requests':0,'journal':json.loads((r/'reports/actual/journal-metrics.json').read_text()),'sha256':{p:hashlib.sha256((r/p).read_bytes()).hexdigest() for p in ['source/apps/orchestrator/internal/orchestrator/skill_provider.go','reports/app.test','reports/build.log','reports/tests.log','reports/skills-tests.log','reports/reviewed-tests.log']},'mandatory_checks_passed':True,'no_skips':True}
    (r/'reports/summary.json').write_text(json.dumps(summary,indent=2))
    print(json.dumps(summary),flush=True)
