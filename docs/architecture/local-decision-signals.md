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
data. GLiNER returns only a selected score, so every observation escalates. d1
returns a full choice distribution, still uncalibrated. Duplicate/unknown labels,
invalid scores, missing correlation and stale data fail closed. No executor,
urgency, image or audio policy is inferred from these labels.

## Measured default

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
Full current-controller quality/cost comparison remains unmeasured. No unbudgeted
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
baseline, held-out calibration/error analysis, reliable repeat loading within
unchanged limits and a measured improvement without critical misses. Changes
to labels, descriptions, precision or revision require new dev calibration and
a new held-out corpus. Both profiles stay disabled in the ordinary catalog;
the lab evaluation catalog enables them temporarily. Restoring the ordinary
catalog disables them without changing owner authority or memory acceptance.
