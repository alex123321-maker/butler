# Optional mastered-command proposals

Issue #30 remains active. The current work is a default-disabled pretrained
baseline, not a production command fast path. Runtime, owned source/job admission,
fixed-schema normalization and a frozen Russian quality harness are independently
reviewable. No core provider switch, automatic stronger model, new action grant
or task lifecycle transition is supplied by the model.

## Model and distribution

[Google FunctionGemma](https://ai.google.dev/gemma/docs/functiongemma) is a
specialized 270M function-calling model that usually needs task-specific tuning.
The [official Ollama integration](https://ai.google.dev/gemma/docs/integrations/ollama)
and [public Q8_0 registry release](https://ollama.com/library/functiongemma:270m-it-q8_0)
provide lawful public distribution under the
[Gemma Terms dated March 24, 2025](https://ai.google.dev/gemma/terms).
This experiment does not access gated Hugging Face weights or credentials.

The 300,796,576-byte model layer is SHA256
`415f8f959d807bd4d4da891f01225d7b330416947fb011a8473080ae4fd07885`.
The current downloaded registry manifest is
`f6078f93a342963cabae694fd53e09d885801a6e82157c73f0edf67a7e879020`;
it wraps the earlier public registry manifest
`7c19b650567acfb1bee50d12bb286370e8a54d8877f6a0ecfd01f8c8fdc223bb`.
All config/license/parameter/model layers are hashed through bounded contained
regular-file reads before first load. Read-only binds are required after that
verification; this is not protection against a malicious host operator.

Ollama0.40.2 comes from the existing verified image digest
`b86366bb528bbf7f1424435d165028497a5b69bf6ddb4fa5a87102e2b79f44fb`.
Only its 61 MiB CPU binary/library bundle is mounted from storage; checked-in
hashes pin every regular runtime file and exact library symlink target. No CUDA
libraries or new Python dependency are used. This adds an optional upstream
inference executable, not an extra service/container/network authority.

## Runtime contract

`CAPABILITY_FUNCTION_PROPOSE` takes bounded text plus one exact reviewed catalog
version. It returns at most one complete typed candidate or insufficient data.
Unknown function, duplicate argument key, extra parameter, unsafe URL shape,
incomplete decoder output and parallel batches are rejected as a complete set.
Raw dialogue is discarded. Requests cannot select definitions, schema, model,
runtime paths, server address, sampling parameters or credentials.

The official [Google control-token grammar](https://ai.google.dev/gemma/docs/functiongemma/formatting-and-best-practices)
is rendered explicitly and passed to documented
[Ollama raw generate](https://docs.ollama.com/api/generate). The initial high-level
chat probes emitted invalid batches (three empty function names; seven repeated
known functions with the trigger instruction); the boundary rejected them. They
are protocol diagnostics on one public synthetic prompt, not scored corpus
results. Raw generation with the fixed developer/declaration format produced a
valid EOS/no-call response on that prompt. It does not demonstrate useful Russian
classification. No guard was relaxed to accept a partial batch.

Linux process groups, scrubbed environment, fixed loopback listener,
`OLLAMA_NO_CLOUD=1`, `num_gpu=0`, four CPU threads, 2,048 context tokens, 192 output
tokens, 60-second request deadline and internal-only container network bound the
owned child. Cancellation waits for process-group termination before RAM release.
The normal 6 GiB RSS admission, 512 MiB reserve, shared CPU/GPU RAM reservations,
8 GiB hard cgroup and 30-second idle unload remain unchanged.

## Acceptance matrix

| Requirement | Current evidence / remaining work |
| --- | --- |
| Pinned catalog/model and independent test split | Fixed three read-only functions, four dev/twenty held-out Russian cases; no training or held-out tuning |
| Correct function and all arguments | Frozen exact-match harness; no JSON-validity substitute |
| Unknown command or schema | Unknown names/batches fail closed; current complete Broker schema/revision required |
| Newly created tools | Excluded from fixed catalog; extension contracts denied by normalizer |
| Common action authority | Normal transport proposal shape and Broker schema test; no activation hook; full production revision/permission proof still ordinary controller-owned |
| Russian negation/durations/numbers/names/clarification | Frozen cases measured, gate fails; no accepted positive command, four decoder/output refusals |
| Latency/RAM/false calls/cost vs rules/current agent | Real CPU measurement and limited literal baseline; full live native3.8 comparison remains pending, no monetary saving inferred |
| Disable or no benefit | Normal profile disabled; no always-resident model or core fast path |

## Actual frozen lab result

[The complete artifact](../../deploy/local-models/verification-functiongemma.json)
retains all 24 cases, including four dev cases and twenty independent test cases.
Every actual private-service inference has a separate durable shared reservation.
The measured end-to-end configuration scores **10/20** exact test results versus
**18/20** for the frozen literal baseline, with four `DataLoss` output refusals.
No accepted candidate is emitted on no-call gold cases; this does not assert that
malformed raw model text contains no erroneous intent. In this run the model
produces no accepted positive command at all. The frozen 95% exact/zero-refusal
gate fails, and no useful command optimization or automatic dispatch is enabled.

Successful-response RAM telemetry peaks at 429 MiB; test wall latency has p50
723 ms and p95 3,241 ms. These cover source/job/budget/database/IPC work and are
not pure decoder timings. Failed responses do not expose per-call RAM telemetry.
GPU remains at the observed 17 MiB idle baseline before/after this CPU window.
Electricity, hardware costs and complete live native-agent latency are not
measured. The result describes this pretrained model/runtime/catalog/decoder
configuration, not isolated weights or a guarantee about future fine-tuning.
Normal nine-profile catalog/voice-off image was restored after measurement.

The initial measurement charges the serialized typed service request. The final
caller additionally reserves a fixed 4 KiB bound for its reviewed declarations
and generation envelope, within each existing finite 8 KiB job byte allowance;
this conservative accounting change does not alter the model/corpus or gate.

The pretrained model is not silently fine-tuned because a corpus gate fails. Any
later tuning needs a separate reviewed dataset/version and untouched independent
test population. Ambiguous/multistep text, foreign documents and partial voice
input cannot acquire an owner-command role from model confidence or a label.

## Verification and reproduction

All compilation/testing runs on `ssh laba` with Go1.27.2. Narrow real PostgreSQL
MaxConns2/race tests cover finite actual-call reservations, forged/Secret/changed
sources, current job cancellation and the common Broker schema contract. The
Linux runner test waits for a real child before process-group cancellation; the
actual offline Manager smoke checks cgroup/RAM admission and idle unload. Full
`go test ./...` passes with optional external integration suites unset; this is
not a substitute for the separate actual PostgreSQL/ML runs.

Prepare model/runtime on `/srv/storage` only, without credential mounts: pull the
exact public Q8_0 tag in a bounded non-GPU preparation container from the pinned
existing Ollama image, then stop it. Verify the complete manifest/blob hashes
against the pins above. Export its `/usr/bin/ollama` to `cpu/bin/ollama` and only
regular files/symlinks at the top of `/usr/lib/ollama` to `cpu/lib/ollama`, retaining
bundled license files. Do not copy CUDA/MLX/Vulkan directories into this CPU
bundle or image layers. `function_runtime_manifest.json` checks those exact bytes.
Both model and CPU-runtime binds stay read-only during inference.

`deploy/local-models/functiongemma-stage.sh` is an explicit lab-only operator
preview: it accepts one immutable owned source snapshot, builds/recreates only
`local-models`, retains existing normal profiles and enables the CPU experiment
only in the evaluation catalog. It does not change main `current`, native/core,
PostgreSQL, original Ollama or private token paths. Restore the prior normal
catalog/image afterward. The quality runner requires an isolated test database,
the single private service-token file and an explicit public report path:
`go test ./internal/functionproposal -run TestActualRussianFunctionProposalComparison`.
Every case has an owner-created current finite job, one fresh actual reservation
and explicit completed/failed job receipt; all errors remain in the artifact.
