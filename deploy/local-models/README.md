# Reproducible lab model runtime

All operations run on `ssh laba`, in a private source snapshot and
`/srv/storage/butler/agents/local-models`. Do not change the shared lab source
pointer or start the shared Compose stack from this directory.

1. Create the private root and sync with
   `scripts/dev/lab.ps1 -Action Sync -RemoteRoot /srv/storage/butler/agents/local-models`.
2. Run `bash current/deploy/local-models/prepare-lab.sh` on the lab. Network is
   allowed only during preparation. Public checkpoint download explicitly uses
   `token=False`, a full immutable HF revision, safetensors and data/config files;
   no remote Python code is downloaded or trusted.
   For a previously prepared embedding-only environment, use `extend-lab.sh`
   instead: the extra GLiNER libraries reuse installed Torch/CUDA wheels. The
   dry-run dependency plan and installation logs stay on the storage bind.
3. Run `bash current/deploy/local-models/verify-lab.sh`. It builds Go binaries on
   the lab and launches only `butler-local-models-proof`, using `--network none`,
   UID10001, read-only source/dependency/model mounts, no capabilities and private
   tmpfs. A random bearer token is generated in `private/token`, never printed.
   The fixture contains only public synthetic project statements and code.
4. Review `reports/verification-gpu.json`, `verification-constrained.json`, GPU
   CSVs and service logs. The helper unloads its own worker/container and records
   the device before/after. `constrained` or `gpu` as the second argument selects
   one mode; `capabilities` runs the complete reviewed model routing proof.
   It never unloads other applications' GPU processes.

The scripts use existing `butler-local-models-ollama` only for `ollama ps` evidence
from the isolated compatibility probe; it is not the inference backend. For a
fresh lab without that probe container, the helper records its absence. The
shared Butler Ollama is untouched. The optional Compose overlay is integrated
separately by the deployment owner after preparation; its service stays opt-in.

## Recorded hardware and results (2026-10-10 UTC)

Lab: Ubuntu kernel 7.0.0-34, RTX3060 Laptop 6144 MiB, driver595.91.07/CUDA13.2.
Torch2.10.0+cu128 is compatible with this driver's CUDA12 support. Main dependencies
and complete version lock are included here. The 271,002,624-parameter text model
loads BF16 on GPU and FP32 on CPU, omitting vision/audio encoders. Full downloaded
checkpoint occupies ~1.5 GiB; prepared framework/CUDA environment ~7.4 GiB.
All bytes are on `/srv/storage`, outside image layers. Installer temp/cache paths
also point to the storage bind; `pip --dry-run` can download large complete wheels.

Recorded GPU artifact: whole-device peak 1323 MiB for an 8000-byte four-document
batch (280 ms); small queries use about751 MiB. Cold index request 8.7 seconds,
warm small batches/queries 78–99 ms including final whole-device telemetry.
Cold CPU requests include ~8 seconds framework startup; the long CPU batch takes
8.9 seconds. Simultaneous indexing/search waits in the shared queue (80 ms GPU,
98 ms CPU). Over-context input receives InvalidArgument instead of truncation.
Twelve synthetic Russian/code queries retrieved the expected first
document (top1=1.0). These easy cases do not establish production quality or a win
against Butler's existing retrieval baseline. Constrained 1024 MiB manager budget
forces actual CPU fallback without allocating artificial OOM pressure. GPU usage
returns to the same 17 MiB baseline after workers stop. Periodic telemetry includes
other applications; finite/norm/dimension validation and explicit failure states
prevent a false successful empty embedding.

## Shared capability verification (2026-10-10 UTC)

The complete selected profile now includes the actual official
[Qwen3-Reranker-0.6B](https://huggingface.co/Qwen/Qwen3-Reranker-0.6B), revision
`e61197ed45024b0ed8a2d74b80b4d909f1255473`, and
[GLiNER2.5-multi-v1](https://huggingface.co/fastino/gliner2.5-multi-v1), revision
`cf5593a5d45e3bbf204b9df621b13b1c0cf25ee3`, alongside the existing Gemma2 checkpoint.
All weights occupy 3.7 GiB; the reused framework environment remains 7.4 GiB.
Additional libraries/dependencies are pinned in `extension.lock.txt` and the
complete installed `requirements.lock.txt`. No CUDA library is copied into an image.

`verification-capabilities.json` records real CPU Russian entity spans and label
scores, BF16 GPU ranking, overlapping CPU classification/GPU ranking (137/202 ms),
and loading Gemma2 text+vision 440M for a finite normalized 768d image vector.
Cold calls took 33.2 seconds GLiNER, 8.1 seconds Qwen and 10.2 seconds vision.
Peak whole-device use was 1935 MiB; full process RSS including both worker runtimes
peaked at 4394 MiB. The 6144 MiB RSS/device budgets each retain a 512 MiB reserve.
The service uses an 8 GiB hard container limit to include reclaimable file cache.
GPU returned to its unchanged 17 MiB baseline after container shutdown.

The image proof uses a synthetic public RGB image, not a claim of OCR/vision
quality; the Russian scores are runtime signals, not calibrated decisions or a
held-out evaluation. Audio/video remain unavailable. Production embedding,
reranking and extraction quality decisions belong to #25–#28.

## Tensor runtime exception

[Google's official card](https://huggingface.co/google/embeddinggemma-2) supports
Transformers/SentenceTransformers, text-only encoder selection and BF16/FP32;
FP16 is explicitly unsuitable. The card uses
[Apache-2.0](https://ai.google.dev/gemma/apache_2). We pin revision
`914f7f89142e33e77833254d9c9b90c3cef7303b` and reject version mismatches.

The official [Ollama BF16 text artifact](https://ollama.com/library/embeddinggemma-2:270m-bf16-text)
was also tested: Ollama0.40.2 tags identify revision
`afeb999480b2a5e48b590fc0583f8cbc7d206b74156feaead3a7840b81719295`,
format safetensors, runner `mlx`. Both GPU and `num_gpu:0` requests on Linux return
HTTP500 `MLX not available`. Its presence in the Ollama catalog does not mean it
runs on NVIDIA Linux. We retain that incompatibility evidence instead of silently
substituting the first EmbeddingGemma generation. Go delegates only tensor
computation to a managed subprocess; cancellation kills and waits for that process.

The official processor requires Pillow/torchvision even when vision/audio model
encoders are omitted. They are pinned; neither multimodal capability nor audio
quality is claimed. The runtime never resolves cloud models or follows network
fallbacks. Automatic generation-model loading is outside this service.
