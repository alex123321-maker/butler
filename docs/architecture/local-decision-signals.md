# Local decision signals

Decision models are optional observations. They never suppress accepted input,
undo an owner stop, grant a resource, create a task, bypass approval or mark a
result verified. Durable owner control, action authorization and objective
evidence retain those roles. `internal/decisions` exposes no such operations;
there is no automatic production consumer or controller shortcut.

## Current source and budget boundary

Input is an exact current catalog source under a live bounded owner-created
auxiliary job with the distinct `decision_shadow` purpose. Source graph,
content hash, access revision, sensitivity and current claim are checked before
IPC. Secret data never reaches the model. Typed locality comes from the composed
private internal service, independently of the explicit no-billing price basis.
Text cannot choose another endpoint or grant permission. Every actual attempt
charges a fresh shared durable reservation; retries cannot reuse old allowance.
The bounded context survives waits, cancellation invalidates a late result and
current source/job access is checked after the call.

Source hints such as owner, administrator, verified or public are data, never
trusted identity or proof. Scores/labels are not accepted memories, goals,
permissions or owner mandates. Output has `shadow_only=true` and
`event_preserved=true`; no irreversible filtering is implemented.

## Signals and escalation

Nine Russian labels describe stop, changed conditions, clarification, new task,
important event, foreign instruction, insufficient data, result checking and
confirmed success. The last label is a prediction, not Butler verification.
Scores are not calibrated probabilities of correctness. Fixed gates escalate
below 0.65 score, below 0.15 margin, unknown margin, empty output or insufficient
data. The original GLiNER profiles return only a selected score and always
escalate. Separately pinned full-score profiles and d1 return every alternative;
the complete label set is required before a score margin is usable. Two through
eight categories cannot pretend to be a complete nine-class distribution.
Duplicate/unknown labels, invalid scores, missing correlation, preprocessing,
FP32/context-cap drift and stale data fail closed. No executor,
urgency, image or audio policy is inferred from these labels.

## Current measured default

Selected mode remains **disabled**. The 2026-10-14
[replay report](../../deploy/local-models/decision-replay-20261014/README.md)
records 480 successful CPU attempts: two identical passes through the original
profiles and two through separate full-score GLiNER profiles with unchanged
d1. Each experiment uses one persistent service and its original 8 GiB cgroup,
6 GiB RSS policy and 512 MiB reserve. d1 repeat loading succeeds on the current
cache-accounting implementation; older refusals remain historical evidence.

The full-score path uses the existing pinned SDK's `Classifier(model).score`
and explicit softmax activation, preprocessing
`gliner2.5-classification-softmax-v1`. It wraps the admitted model without another
checkpoint load. Original selected-only outputs remain reproducible. Neither
labels, source text nor numerical thresholds changed. Top-label accuracy and
critical errors reproduce in all four passes. Of 30 regression-test events,
full-score GLiNER extractor/Decide/d1 produce 16/12/3 non-escalated signals,
of which 12/5/3 are wrong. No candidate passes quality or critical-error screening.
Full margins remove an adapter limitation but do not establish reliability.

Report version `decision-comparison-v2` separates runtime availability from
prediction accuracy and class confusion. A refused RPC always escalates and is
excluded from class errors and successful warm latency. First-attempt timing is the first
profile attempt, rather than the first test-split row. Screening also requires
more correct non-escalated signals than the existing extractor and no wrong
non-escalated signal; raw accuracy alone cannot establish fewer controller calls.
Actual calls saved remain zero in shadow mode.

The corpus has already been observed, so these passes are a regression replay,
not fresh held-out generalization or calibration. Literal rules still score
30/30 on this constructed fixture. The annotation-derived typed control
expectations are separate from classifier metrics; a live current-controller
quality/cost comparison remains unmeasured. This and an independent corpus remain
explicit acceptance gaps for issue 28. Production scheduling/shortcuts stay off.

## Historical measurements

Selected mode: **disabled**. The fixed public corpus has 10 dev / 30 held-out
Russian events: negated stops, temporal amendments, ambiguity/conflict, foreign
injections, partial/stale evidence and unsupported claims of success. Labels,
score/margin thresholds and accuracy/critical-error gates were fixed before
measurement. Three immutable CPU FP32 profiles receive identical inputs through
the same persistent service and actual owned admission.

The original run made 120 successful model inferences. Held-out accuracy was 16.7%
for the existing GLiNER extractor, 46.7% for Decide and 6.7% for d1. Decide/d1 falsely
predicted confirmed success 2/3 times; all models missed important events. Decide
improves raw accuracy on this small fixture but fails the 90%/zero-critical-miss
gate and cannot reduce controller calls with its unknown margin. It stays off.

Exact replay repaired CPU measurement without changing inputs/thresholds. The
GLiNER results reproduced, but one first GLiNER RPC was Unavailable and all 40
d1 attempts were ResourceExhausted. After idle, about 3.65 GiB of an 8 GiB service
cgroup allowance remained charged mostly as cached weight files. Conservative
RAM admission counts this against available headroom; d1's 4608 MiB peak plus 512 MiB
reserve no longer fits. This is repeat-load refusal, not 40 wrong predictions.
No larger limit, global cache flush or extra tensor process concealed it.

Literal rules score 30/30 on this deliberately synthetic corpus, not a production
population. The report's `current_controller` entry is a typed owner-stop/amend
and objective-proof contract fixture, not a live cloud model benchmark or the
production controller's classification outputs. Other events escalate there.
Full current-controller quality/cost comparison was unmeasured. No unbudgeted
cloud calls or claimed monetary savings are introduced.

## Runtime, license and reassessment

Profiles/revisions and RAM allowances are pinned in the operator catalog.
GLiNER Decide has Apache-2.0 metadata; d1 uses LFM Open License 1.0 with a commercial
revenue threshold, retained with the checkpoint. Its local loader is the exact
reviewed offline custom-code exception in
[the audit](../../deploy/local-models/decision-code-review.md): seven code/config
hashes checked before loading, and complete encoding compared to refuse silent
truncation. Images/audio are not exposed. Upstream English metrics do not
establish Russian voice/image quality.

Both complete reports retain per-class errors, timings, RSS/GPU and sampled CPU.
Calls were CPU-only, measured GPU allocation 0 MiB, cloud/billed API calls 0.
Electricity/hardware price and exact process CPU time were not measured. CPU
sampling uses 100%=one core and includes loading and Go supervision.

Reassessment requires an independent Russian corpus, full current-controller
baseline, held-out calibration/error analysis, continued repeat loading within
unchanged limits and a measured improvement without critical misses. Changes
to labels, descriptions, precision or revision require new dev calibration and
a new held-out corpus. Both profiles stay disabled in the ordinary catalog;
the lab evaluation catalog enables them temporarily. Restoring the ordinary
catalog disables them without changing owner authority or memory acceptance.
