# Extraction and decision acceptance matrix

Measured experiments are separate from production readiness. A failed model
gate is not passed by literal source-preserving rules or by a runner that
successfully records a report. Extraction scheduling/controller shortcuts stay off.

## Structured extraction (issue 27)

| Criterion | Status / evidence |
|---|---|
| Source text/ID, positions, model/schema version, score for each record | PASS: exact Unicode spans, provenance/source pins, typed records and actual PG reload |
| No invented required fact, explicit absence/ambiguity | PASS: fabricated spans deny publication; absent fields and unresolved polarity/exception scope stay explicit |
| Russian negation/exception/conditions/earlier-now | TESTED, QUALITY FAILED: fixed 24 cases; model misses 5 prohibition cases. Literal quote evidence is not learned negation |
| Relative time anchored to event/timezone | PASS within documented grammar; missing context unresolved |
| Candidate mechanism, no direct rights/goals/profile rewrite | PASS: ProposeTx/candidate status/separate owner acceptance, no authority write path |
| Foreign document does not become owner command | PASS: data/derived origin and candidate status cannot mint owner proof |
| Secrets and sensitive storage | PASS: Secret denied before IPC, private origin retained, current graph/privacy/job guards |
| Precision/recall, negation errors, CPU latency/RAM, comparisons | MEASURED, quality NO-GO: held-out P0.484375/R0.563636; rules P0.975/R0.709091; existing local generic adapter P0.319149/R0.272727. Live disabled legacy-cloud baseline unmeasured |
| Pinned schema/model/dependencies, no duplicate extraction | PASS: exact pipeline/source identity, body dedup, complete empty/retry cache; repeat 0 extra IPC |
| Exact acceptance restriction | PASS source-preserving candidate: model resource/action/deadline spans plus explicit literal_rule prohibition/exception preserve full quote; mandate acceptance separate |

Recommendation: **issue 27 remains active for production acceptance**. Frozen
P≥0.9/R≥0.8/no-prohibition-miss gate fails. The opt-in experiment and quoted
acceptance scenario work, but do not establish reliable learned restriction
understanding. Scheduling is not automatically enabled. Full legacy-cloud
comparison is unmeasured because that unbudgeted path stays disabled.

## Decision comparison (issue 28)

| Criterion | Status / evidence |
|---|---|
| Russian events/clarifications/negation/conflict/ambiguity/injections | PASS: fixed 40 cases/9 labels/source hints |
| Dev/test separation and thresholds not tuned on final test | PASS:10/30;0.65 score/0.15 margin / 90% accuracy/zero critical error; exact replay unchanged |
| Per-class errors, important misses, false initiative/success | PASS measurement: all120 results in both runs, confusion/TP-FP-FN and refusals retained |
| Insufficient data/escalation, scores not truth probabilities | PASS: empty/low/ambiguous/unknown-margin escalates; GLiNER one-class output always escalates |
| Shadow first, record decisions, no permanent event suppression | PASS: complete public artifacts, shadow-only/event-preserved output, no suppression interface |
| Cannot undo owner stop, grant resource or bypass approval | PASS boundary: no task/broker/grant/verdict API; real PG finite-job, forged/Secret, changed-source/cancellation tests |
| CPU/GPU/latency/cost; extra model only on win | MEASURED, selection NO-GO: original 120 inferences succeed but quality fails; exact replay d1 refuses40 loads. CPU/RSS/GPU/timings retained, explicit local basis/billed API cost0; electricity/hardware unmeasured. Defaults off |
| Pinned/audited custom loader | PASS: exact d1 revision, five reviewed modules/seven hashes, contained reads/offline RO load/no-truncation; its license differs from Apache |
| Chosen mode/errors/thresholds/off switch/reassessment | PASS: disabled mode, literal gates, known errors and catalog rollback; independent held-out reassessment required |
| Rules/existing extractor/current-controller baselines | PARTIAL: first two actually measured; current entry is a typed owner-control/proof fixture, not full live current-controller classification/cost benchmark |

Recommendation: **measured local research no-go**; keep issue 28 active if full
current-controller comparison is mandatory. No evidence justifies deploying a
new classifier. Predicting a success label is never a verifier. Russian voice,
image quality and executor/urgency selection are outside this text-only corpus.

Original successful model measurements are in
`verification-decisions-before-cpu-repair.json`; the full unchanged-input replay
is `verification-decisions.json`, including CPU sampling and every refusal. The
original CPU sampler permission failure is fixed in the replay script, not hidden
by discarding either run. Extraction artifacts likewise retain fixed infrastructure
errors separately from the unchanged failed model gate.
