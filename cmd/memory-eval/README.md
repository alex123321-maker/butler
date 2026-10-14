# Memory evaluation

`memory-eval` extends `internal/acceptance`, the existing issue33 evaluation
contour. It is a local offline command, not a new service, provider wrapper or
tool executor. The initial issue78 delivery provides versioned data, pre-run
freezing, input export, strict measurement import and programmatic scoring.

## Reproduce the mandatory offline checks on laba

From an isolated checkout on Windows:

```powershell
pwsh -NoProfile -File scripts/dev/lab.ps1 -Action Sync `
  -RemoteRoot /srv/storage/butler/agents/memory-eval-78-20261014
ssh laba 'bash /srv/storage/butler/agents/memory-eval-78-20261014/current/scripts/dev/memory-eval-lab.sh /srv/storage/butler/agents/memory-eval-78-20261014 <exact-40-character-candidate-commit>'
```

The script first runs the narrow `TestMemory` checks, then vet and the existing
acceptance package regressions under race detection. It uses Go1.27.2, two CPUs,
1536MiB,128PIDs, network none, a read-only source snapshot and repo-local caches.
It mounts no credentials, PostgreSQL, Redis or model worker. It creates a new
private report directory and never changes the main deployment's `current`.

`fixture` exercises the current `memory/service.BuildBundle` with explicitly
fixed synthetic store responses. It does **not** implement a competing search
algorithm or call embeddings. Its answers are constructed from the rubric to
exercise the scorer. Historical-only citations, missing-memory abstentions,
secret exclusions and oversized full-context inputs remain visible negatives.
These results cannot measure an agent's answer quality or task success.

Saved artifacts include hardware, exact source/data/config/prompt/criteria pins,
each frozen input, permitted originals, controlled observations, every expense
stage, per-case results and artifact hashes. Existing directories/files cannot
be overwritten. All artifacts remain private; public summaries omit bodies.

## Bounded model experiment preparation and import

There is deliberately no `native` mode. Actual requests must be collected through
the existing Butler provider/ownership/privacy/budget admission contour with a
fresh independent review, finite window and original charged ledger. Existing
sealed windows and their spent attempts are never reused by this command.

Before collection, provide a `MemoryPlan` JSON with exact source commit/tree,
hardware, requested model and observed/version identity, runtime, pricing basis,
memory configuration, suite and criteria SHA256, fresh window ID and selected
split. Use `evidence_kind=model_observation`; cloud weights are not fabricated.
The shared plan fixes identical model/limits for all three arms. Policy constants
are frozen at90% answer/current-evidence and90% current-instruction accuracy,
zero false-positive unsupported answers and critical violations,60s nearest-rank
p95,240 total recorded calls,1,000,000 total tokens,3,000,000 monetary microunits,
32KiB input and8KiB output. They are initial experiment bounds, not a readiness
SLO established by this fixture. Changes require a new criteria digest/review.

Supply captures from the **actual current retrieval path**, containing the opaque
case ID, memory configuration digest, immutable original evidence digest, ordered
original IDs and exact bundle prompt. Empty legitimate search results are empty
captures. Missing captures produce explicit skips, never full-context fallback.
The collector must include every source backing profile/episodic/summary/chunk
context; unavailable legacy provenance must be reported instead of invented.

```sh
memory-eval -mode freeze -root /src -code-commit <commit> \
  -hardware /private/hardware.json -plan /private/plan.json \
  -captures /private/current-retrieval.json -output-dir /private/new-freeze

memory-eval -mode score -root /src -code-commit <same-commit> \
  -hardware /private/hardware.json -frozen /private/new-freeze/frozen.json \
  -frozen-sha256 <independently-pinned-frozen-sha256> \
  -measurements /private/original-measurements.json \
  -golden-public-key /review/public-key.hex \
  -golden-approval /review/approval.json -output-dir /private/new-score
```

Exported inputs contain no expected answers, categories, behavior labels or review
rubrics. Current goal/instruction/revision are identical across arms. `no_ltm`
contains no historical memory; `full_context` contains all permitted undeleted
history, including superseded originals needed for temporal questions; `retrieval`
retains the exact captured current bundle and source metadata. It does not append
full original bodies to the retrieval prompt. The scorer checks quotes against
both the provided prompt and pinned original in the verifier's suite. No deleted
body is exported. Secret-bearing cases are blocked before input. If full history
exceeds the fixed byte budget, it is skipped in full with a reason. Byte bounds
are a conservative export limit, not a claim about a provider's token capacity;
the live collector must also enforce its actual tokenizer/context limit.

Measurements contain the frozen SHA, one observation per case/arm, exact input
SHA, the actual model/version/runtime for every completed observation,
completed/failed/skipped status, a safe reason, original JSON output,
nullable latency and independent effect outcomes with an original evidence SHA.
Answer/citation and current-instruction checks do not establish kernel effects.
Task success and permission/privacy/stop/deletion violations come from separate
runner records, never from model assertions. Use JSON `null` for unknown values.

For each admitted case/arm explicitly account for `read`, `write`, `embedding`,
`extraction`, `summary`, `consolidation`, `generation`, `retry`, `background`.
Keep every failed/repeated sample and source evidence identity. Measurements
include calls, input/output/total tokens (including hidden billed tokens), latency,
monetary microunits and storage bytes. `not_used` means explicit measured zeros;
unknown or missing measurements/stages deny complete cost. Shared preparation
expenses must use a declared attribution rule so they are counted once per arm,
never silently omitted or counted as free. Storage is cumulative observed bytes
in the supplied stage records, not an inferred database footprint.

Frozen `verifier_captures` retain original retrieval captures even when an input
exceeds the budget; they belong to the verifier, not model input. The collector
must use only the `inputs` array. Already spent capture/read/background costs of
skipped cases remain in accounting. Every case/arm, including a blocked or skipped
one, needs explicit stage coverage; a skipped generation charge is a violation.

The score command additionally retains the exact original imported measurement
file, including whitespace and malformed raw model answers. A structurally valid
envelope rejected by scoring produces a negative report and retained original;
malformed envelope JSON is rejected before scoring and remains at its input path.

The same `GoldenApproval` Ed25519 format as issue33 signs suite digest, criteria,
version, reviewer and different candidate author. The private signing key and
independent checker stay outside candidate code. This command does not sign.
The new dataset is presently **unreviewed**. Candidate-host imports and hashes
alone do not prove native inference, runner/ledger correlation or independence;
promotion and execution remain false even after successful offline scoring.

## Dataset and #76

`testdata/acceptance/ru-memory-v1.json` contains30 explicitly synthetic cases,
15 development and15 holdout cases, each with timestamps, session identity,
source status/revision, task instruction, literal answer and witnesses. No real
user memory or public benchmark data were imported. Source/topic and declared
template groups do not cross parts; normalized exact duplicate originals fail.
This structural screen is not semantic independence. Independent whole-content
review must check paraphrases/template similarity and answer ambiguity before
model trials. Holdout must never drive tuning; a candidate selected using these
holdout results needs a fresh control dataset and separate review. Future real
cases require human-checked provenance and separate dataset/version annotation.

Case `behavior_refs` point into the common issue76 behavior categories. Reports
link these references to input/output evidence hashes and per-case discrepancies;
they do not create another catalog, invent a causal diagnosis, or complete #76.
Answer/current-fact correctness, exact citations, false positive answers, false
abstentions, confidence Brier components, retrieval recall/MRR, current task
instructions and independent task outcomes are separate metrics. A model judge
is not used. Literal answer equality is a transparent initial rubric, not a
general semantic answer judge; human-reviewed alternative answer sets can be
added only as a new frozen version.

## Remaining issue78 acceptance

Actual admission/ledger-backed collection for all three arms, full current-memory
write/index/background expense instrumentation, independently reviewed control
data and one bounded model comparison remain required. Offline fixture success
does not close issue78, change memory policy or imply production readiness.
