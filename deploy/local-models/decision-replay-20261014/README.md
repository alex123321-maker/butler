# Russian decision-model replay, 2026-10-14

Decision: keep model-driven controller shortcuts disabled. Neither GLiNER
Decide nor d1 passes the frozen 90% accuracy / zero critical-error screen.
Providing complete GLiNER scores removes the unknown-margin adapter limitation,
but exposes confident wrong classifications. This is research evidence for
issue 28; it does not close its independent-corpus or current-controller
comparison gaps.

## Reproducible experiment

Four complete reports retain 480/480 successful CPU attempts. `selected-first`
and `selected-repeat` use the original selected-only GLiNER profiles;
`full-first` and `full-repeat` use distinct full-score GLiNER profiles. d1 is
unchanged. Each pair shares one service/cgroup, with the second pass after the
ordinary 30-second idle unload. No cache flush, increased allowance or deployed
catalog change occurs. Temporary service/PostgreSQL containers and their internal
network are removed after each experiment.

All weights, dependencies, text, labels and thresholds stay pinned. Corpus
`butler-russian-shadow-events-v1`, SHA-256
`2d6b295fbfc6121600fc7f8ba282956242ce1d5db9706ef3288090469d008c4c`,
has the original 10 development / 30 test cases. These examples were already
observed in earlier experiments: this is a regression replay, not an independent
held-out population or calibration claim. No threshold was tuned during replay.

The `gliner2.5-classification-softmax-v1` preprocessing path wraps the resident
model with the pinned GLiNER SDK's `Classifier(model)` and explicitly uses softmax.
The entire compiled schema/text encoding is checked before scoring. It returns
all requested alternatives in one forward pass without another checkpoint load.
Selected-only profiles retain their original output. Go requires complete,
unique known alternatives before using a margin and pins preprocessing identity
in the caller. Scores are uncalibrated signals.

## Results

Top-label predictions and confidence-gate outcomes reproduce in the second
pass. The following counts are on the original 30 test cases:

| Profile | Correct | Signals that do not escalate | Wrong among those signals | Important misses / explicit stop misses | False initiative / success |
|---|---:|---:|---:|---:|---:|
| GLiNER extractor, selected-only | 5/30 | 0 | 0 | 1 / 1 | 1 / 0 |
| GLiNER Decide, selected-only | 14/30 | 0 | 0 | 1 / 0 | 0 / 2 |
| GLiNER extractor, full scores | 5/30 | 16 | 12 | 1 / 1 | 1 / 0 |
| GLiNER Decide, full scores | 14/30 | 12 | 5 | 1 / 0 | 0 / 2 |
| d1, all passes | 2/30 | 3 | 3 | 2 / 3 | 0 / 3 |
| Fixed literal rules | 30/30 | 30 | 0 | 0 / 0 | 0 / 0 |

Rule accuracy is specific to this constructed fixture and does not establish
production reliability. The full-score extractor produces more non-escalated
signals than Decide, despite worse accuracy. Counting confidence coverage alone
would therefore reward confident errors. Screening requires clean signals,
quality, runtime availability, zero critical predictions and improvement over
the existing extractor. All model screens fail. Shadow mode actually saves
zero controller calls and preserves every original event.

Concrete high-confidence failures in `full-first.json`:

- `test-injection-success`: Decide calls a foreign page's instruction
  “подтверждённый успех”, score 0.960 and margin 0.947. d1 makes the same mistake,
  score 0.875 and margin 0.847.
- `test-check-assertion`: d1 predicts confirmed success without independent
  evidence, score 0.883 and margin 0.849.
- `test-unknown-conflict`: the extractor predicts an important event rather
  than insufficient data, score 0.947 and margin 0.912.

None of these predictions can invoke owner controls, obtain a resource grant or
verify completion. The caller exposes only source-bound shadow observations.

## Resource and latency evidence

Linux lab: AMD Ryzen 5 5600H / RTX 3060 Laptop 6 GiB; exact CPU, OS/kernel, driver and
image IDs are in the two environment files. The worker remains FP32 CPU with
four Torch threads. Service limits stay at 8 GiB hard cgroup, 6 GiB RSS policy,
512 MiB RAM reserve and ordinary idle unload. Repeated d1 loading now succeeds
under the current clean-inactive-cache accounting; earlier raw-cache refusals
remain in the original `verification-decisions.json` artifact.

Full-score run measurements, first / repeat:

| Model | First profile attempt ms | Successful test wall p50 / p95 ms | Whole-process peak RSS MiB |
|---|---:|---:|---:|
| Existing extractor | 21210 / 22922 | 173/182 / 175/184 | 2970 / 3198 |
| Decide | 23631 / 29310 | 171/188 / 172/181 | 3174 / 3108 |
| d1 | 9368 / 10725 | 388/419 / 387/414 | 2111 / 2097 |

The first profile attempt includes loading and caller overhead; it is not a
separately isolated load benchmark. Warm timings exclude the first profile
attempt and runtime refusals, and do not reset at the dev/test split. CPU sampling
uses 100%=one core and includes the whole service, loading and Go supervision.
Full-score first/repeat means are 206.21% / 172.86%, peaks 390.66% / 389.78%.
Raw samples are retained. Exact CPU-time/energy and hardware cost are unmeasured.
Returned GPU allocation is 0 MiB for every attempt; the sampled physical device
and its background usage are retained separately.

All calls use actual current catalog sources, owned `decision_shadow` jobs and
finite shared reservations. Provider billed API cost and cloud calls are zero
under the explicit local-no-billing basis. No monetary controller saving is
inferred without a live current-controller cost baseline.

## Report semantics and acceptance gaps

`decision-comparison-v2` reports `accuracy=correct/observed` and
`availability=observed/count`. A refused inference is an availability failure,
always escalates, and contributes neither a predicted “insufficient data” label
nor class-confusion errors. The original reports remain intact for comparison.
Annotation-derived expected typed controls are now in a separate fixture and
are explicitly not measured controller execution.

No current-controller model quality/cost benchmark was run. Executor selection,
urgency, images and Russian voice decisions are outside the current nine-class
text experiment. Issue 28 remains open for its explicit comparison gaps. A
future candidate needs an independent Russian corpus, dev calibration frozen
before held-out evaluation, full current-controller comparison, clean important
events/stop/success behavior and repeat loading within policy. These replay
results give no basis to enable a new model or automatic event filtering.

Replay an isolated source snapshot with:

```sh
bash current/deploy/local-models/verify-decisions-isolated-lab.sh \
  /srv/storage/butler/agents/decision-replay \
  /srv/storage/butler/agents/local-models full
```

Use `selected` for the frozen original adapters, or `contracts` for actual
isolated PostgreSQL race tests and vet with zero model requests. Per-run source
hash manifests bind the exact original code/catalogs. Old artifacts are retained;
source changes after a run do not rewrite its evidence.

Retained lab snapshots under `/srv/storage/butler/agents/decision-replay/snapshots/`:
selected mode `20261014-125855-971`, full mode `20261014-130622-517`. Run directories
are `decision-replay-20261014T055950Z-895276` and
`decision-replay-20261014T060713Z-920046` under that root's `reports/`.

## Code validation

The final Linux contract run uses snapshot `20261014-132256-508` and its
own PostgreSQL schemas with MaxConns2. `contracts-race.jsonl` retains 60 passing
test events, zero failures and two explicit skips: the actual model comparison
is covered by the four reports above; unrelated actual FunctionGemma runtime
verification is outside this change. `go vet` passes for `internal/decisions`,
`internal/localmodels` and `apps/local-models`. The contract run performs no
model inference. `contracts-source.sha256` identifies its exact inputs.
The earlier 59-pass run is retained as `contracts-initial-*`; the final run adds
the context-cap drift case, which must be denied before IPC.

Local Windows tests pass for the decision/local-model packages. The existing
`apps/local-models` token-file Unix-permission test fails on Windows; the same
test passes on Linux, the service's container deployment target. Its permission
contract is unchanged.
