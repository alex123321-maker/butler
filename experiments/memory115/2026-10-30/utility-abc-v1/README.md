# Complete native development ABC on laba, 2026-10-30

All **24 paired cases x 3 arms = 72 scheduled observations** reached execution
and have scores. The native producer exited **1**, because 32 observations failed
the observer gates; reaching the full denominator is not a passing acceptance.
Issue #115 remains **NO_GO/open**. No production deployment was changed.

Source: `cb9849ecb3e13bd723cab6d5a00190d0e6d149cb`.
Model: `gemini-3.8-flash-high`, 8192 output tokens including thinking, 65536 input
reservation, 13 native methods per slot, 90 seconds per method, seven-minute
authority/eight-minute case bounds. Fresh schemas, common current inputs and
documents, balanced ABC/BCA/CAB order; no retry, continuation of an old window,
or fallback. See the [prospective protocol](../../../../docs/testing/issue115-utility-native-2026-10-30.md).

## Frozen primary results

| Measurement | A: no past experience | B: notes | C: same notes + reviewed skill |
|---|---:|---:|---:|
| Scheduled / observed | 24 / 24 | 24 / 24 | 24 / 24 |
| Strict task success | 5 / 24 (20.8%) | 11 / 24 (45.8%) | 12 / 24 (50.0%) |
| Native methods dispatched | 58 | 100 | 124 |
| Full measured tokens, including thinking | 517445 | 822757 | 1145964 |
| Input / visible output / thinking tokens | 347183 / 4208 / 166054 | 679716 / 7685 / 135356 | 1006496 / 7862 / 131606 |
| Unknown native usage / transport errors | 0 / 0 | 0 / 0 | 0 / 0 |
| Execution time, sum across slots, ms | 1360299 | 1274253 | 1366375 |
| Cold setup time, sum, ms | 40463 | 38580 | 71659 |
| Prepared note source writes | 0 | 36 | 36 |
| Scripted setup model-attempt / tool receipts | 2 / 2 | 2 / 2 | 98 / 74 |
| Genuine native creation methods | 0 | 0 | 0 |
| Actual memory.search / memory.open calls | 0 / 0 | 38 / 11 | 44 / 11 |
| Actual skills.search / skills.load calls | 0 / 0 | 0 / 0 | 3 / 18 |

**B-A = +6/24 successes (+25 percentage points), at 1.590x native tokens.**
**C-B = +1/24 (+4.2 percentage points), at 1.393x native tokens.**
These are descriptive differences on one small public development corpus, with
substantial protocol/measurement confounding, not qualified causal estimates or
evidence of general memory/skill effectiveness.

There is a concrete bounded positive signal: on the six historical useful/
distractor cases across recall, preference exception and task resume, B and C
both succeeded 6/6; A succeeded 0/6. The procedural slice went A=3/5, B=3/5,
C=1/5. The skill is actually consumed, but its net benefit/cost is not established.
All four native regression cases failed in every arm; they cannot be called
native accepted merely because their controlled counterparts pass.

Posthoc exact-value diagnostics, excluding the four regression cases, are A=8/20,
B=15/20, C=16/20. They do not replace task success: they ignore completion,
effect and evidence gates. No old score or outcome was changed.

## Read-only diagnosis of all 44 unsuccessful observations

[classification.json](classification.json) is generated from frozen artifacts by
`scripts/dev/memory-utility-abc-audit.py`, without new model calls. It verifies
every recorded slot artifact hash against the original report before classifying.

- **15 blocked effect_unknown**: 14 wrong verification requests to
  `?verify_unknown=current` where that task had no original unknown action, plus
  one check request to the wrong endpoint. The common instruction mentions this
  special verification path in every case. These are observed wrong tool choices
  under a potentially misleading common prompt, not proof that memory retrieval
  returned a wrong fact. The Broker's unknown results remain unknown.
- **6 blocked memory_evidence_search_exhausted**: C50, C52, C66, B67, C68, C70.
  The policy stops the entire run after fruitless reading, including cases whose
  goal is a current verification or artifact. C70 had actually prepared and
  checked its artifact before the later memory search exhausted the policy.
  This is a concrete interaction worth reproducing offline; relaxing authority
  or pretending the blocked runs completed would not be a valid fix.
- **9 partial MAX_TOKENS finals**: all actual task/run/revision/content hashes
  matched. The failed final gate was `answer_state=partial`, not an identity or
  hash violation. All nine last provider finals explicitly report MAX_TOKENS.
  Thus 8192 including thinking is still insufficient in these cases. Preserve
  the partial content/usage; none is accepted as a valid JSON final.
- **2 unqualified http.request paths**: the existing runtime fixture exposes this
  tool, but this observer has no supported projection for it. These are a real
  measurement coverage gap. Both remain unsuccessful in the frozen denominator.
- **9 unsupported_answer**: all are current_sufficient, have the exact expected
  value, and contain opened citations. The frozen scorer requires the exact
  support set, which is empty for these cases, so additional valid citations
  cause rejection. This is a scoring-design limitation, not nine wrong values.
  It must be corrected prospectively and tested without relabeling this cohort.
- **3 answer_mismatch**, all A recall cases, remain actual answer mismatches.

The 32 observer rejections are 15+6 blocked runs, nine partial finals, and two
unsupported tool paths. The remaining 12 unsuccessful observations are the nine
exact-support rejections and three value/outcome mismatches. No unsuccessful
observation or model request has been dropped from the denominator or costs.

## Cost and preserved history

This new completed window contains **282 dispatched native methods and 2486166
full measured tokens**, including thinking and all unsuccessful slots. Every raw
attempt equals its durable PostgreSQL journal entry; all 282 operation/run/
reservation bindings correlate to Core attempts, with zero missing attempts or
input/output-token mismatches. The raw usage is retained separately from Core's
visible input/output accounting.

The legacy per-slot `execution.json.native_requests` field is hardcoded to zero
by the shared controlled fixture and is inaccurate for native runs. It is not
used for accounting: the independent native journal/raw receipts above are the
authoritative request count. Preserve this metadata defect in the frozen source;
correct it for a future producer rather than editing the old execution files.

The reboot-interrupted r2 remains 101 methods, **824166 known full tokens plus
one UNKNOWN usage measurement**, 27 scored positions and no final producer marker.
It was recovered read-only and never resumed. These two disjoint windows alone
are 383 methods and 3310332 known tokens **plus that unknown**; this is not the
total for all previous research. Native r1 dispatched zero methods after its DB
preflight failure. See [prior attempts](prior-attempts.md).

The five older historical receipts and interrupted predecessor are hash-verified
in [comparison.json](comparison.json). Earlier inclusive receipts share
allocations and are not blindly added. Their unknowns, failure records and old
expenses remain untouched. Invoice money is **unknown**, not zero. Setup is
scripted synthetic note/source generation plus actual verified Broker skill
preparation; native creation is zero by design. Manual authorship, autonomous
learning and invoice costs are unmeasured. No claim of full autonomous-learning
cost, amortization, or production learning acceptance follows from this table.

## Validation and retained evidence

[controlled-gate.json](controlled-gate.json) records all72 controlled positions,
zero skips/failures, zero native methods and 2466 verified producer code/config/
migration/build-script blobs. Scripted expected successes A=18, B=24, C=24 are
mechanism witnesses, not model-quality measurements. The preceding gate included
the utility regressions, reviewed-skill Broker E2E, acceptance race tests,
orchestrator/adapter vet and the race-instrumented app binary; build hashes are in
[controlled-build.log](controlled-build.log).

Raw suite/oracle, model request/events, sealed task/input/run/Broker snapshots,
full preparation snapshots, actual skill lifecycle and journal remain private at
`/srv/gitea/butler-acceptance/issue115-utility-native-20261030-r3`. The new public
comparison and offline classification contain hashes/counts/metadata only.

[cleanup.json](cleanup.json) independently verifies: helper and its cgroup removed,
only the helper sleep PID remained before removal, own network removed, own target
PG stopped, borrowed experimental source PG stopped after zero remaining clients,
neither PG OOM-killed. Resource caps were verified from the actual kernel before
auth. No production Compose service was deployed or updated.

The next justified step is offline reproduction of the search-exhaustion versus
current-goal interaction, prospective correction of exact-support scoring and
http.request coverage, and removal of misleading special-case instructions from
unrelated tasks. Increasing model limits alone does not address these findings.
Any later native study must have a new freeze and keep this cohort unchanged.
