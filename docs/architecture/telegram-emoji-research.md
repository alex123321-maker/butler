# Telegram custom emoji research (#140)

Status: isolated operator experiment, started 2026-10-23. No product feature,
model catalog choice, memory behavior or communication-tool change is enabled.

## Boundaries

Affected subsystem: read-only Telegram channel client, plus experimental
corpus/evaluation utilities. A trusted operator invokes the fixed public
getStickerSet/getFile/download helpers using private credential-file references
and the same explicit proxy as the running Telegram adapter. These helpers are
not in the model catalog and cannot be called from a Butler run. Any future
product import/formatting capability must have kernel admission and ordinary
Broker authorization; this experiment does not supply it.

The import uses an independent PostgreSQL database `butler_emoji140_test`, with
no production migrations or writes. PostgreSQL retains run metadata and item
attempts before downloads; private content-addressed media lives outside Git.
Exported JSON, frames, captions and vectors are derived experimental artifacts.
No Redis state, product memory, delivery outbox, polling cursor or provider
credential store is modified. There is no inference in a database transaction.

Python is limited to the isolated experimental tensor/decoder worker because
Transformers/torch and the rlottie binding are required. No core Python runtime
or dependency is added. Existing model packages are mounted read-only; additional
wheels, models and caches remain in this worktree's ignored `.cache`.

## Stages and evidence

1. Inventory all seven public sets, recording real IDs, hashes, formats and
   explicit failures. The first 450-ID pool is a deterministic discovery sample,
   not a confirmed semantic quota or a locked study split.
2. Decode in a networkless container, one bounded subprocess per media file.
   Static images retain alpha; TGS uses rlottie after bounded gzip expansion and
   rejection of external assets; WEBM uses FFmpeg. Eight frames sample a full
   cycle at fixed recorded indices/timestamps; transparent/empty failure is not
   replaced with a successful blank frame. Light/dark backgrounds are separate.
3. Reserve Strawberry outside the development feasibility pilot. Select up to
   ten unique hashes from each other pack before captions. Pack and Unicode are
   hidden from model input. Merely reserving this pack is not a locked 300-item
   test: semantic quotas, near duplicates and independent queries remain open.
4. User clarified that no larger GPU is available and authorized trying available
   machines or self-annotation. On laba (6 GiB RTX3060, about14 GiB system RAM),
   a bounded Qwen3-VL-4B NF4 feasibility pilot is a **separate configuration**.
   The proposed V1/V2/V3 BF16 comparison remains not-run; no equivalence is claimed.
   Do not silently replace 8B/Gemma, accept gated terms, rent GPUs or spend API
   money. One GPU process at a time; refuse competing GPU work.
5. Self-annotation is an AI-rater development layer, not independent human ground
   truth or the two-rater assessment in the issue. Observe original frames before
   caption outputs; retain provenance and ambiguities. No evaluation score is
   established by agreement between captions and these annotations.
6. Full study requires final corpus/quota approval, near-duplicate grouping,
   frozen splits, independent queries/qrels and abstention calibration. The
   research issue stays open until its actual criteria are evidenced.

Network/image/text content is untrusted DATA. Model output is strict-schema
validated; original responses, first-pass failures, truncation, per-attempt timing
and resource observations stay visible. No hidden repair/retry or forced choice.

## Primary specifications

- [Telegram Bot API: getStickerSet](https://core.telegram.org/bots/api#getstickerset)
- [Telegram Bot API: getFile](https://core.telegram.org/bots/api#getfile)
- [Qwen3-VL-4B](https://huggingface.co/Qwen/Qwen3-VL-4B-Instruct)
- [Qwen3-VL-8B](https://huggingface.co/Qwen/Qwen3-VL-8B-Instruct)
- [Gemma3-4B](https://huggingface.co/google/gemma-3-4b-it)
- [Transformers quantization](https://huggingface.co/docs/transformers/quantization/bitsandbytes)
- [rlottie Python API](https://rlottie-python.readthedocs.io/en/latest/rlottie_python.html)

Pinned revisions, runtime identity, commands and observed outcomes belong to
the dated evidence report; model-card availability alone is not runtime readiness.

The first English-worded prompt produced valid JSON but frequent English values
and omitted visible style/OCR/meaning fields. A separate Russian development
prompt revision is tested on the same bounded twelve-item subset; no prior
outputs are replaced or mixed into its metrics. Planned local feasibility budget
is 60 v1 + 12 Russian-v2 + 12 Russian-v3 + 6 single-frame-v3 = 90 model calls, separate from the
three-captioner BF16 matrix. No claim of independent test improvement follows.

Russian-v2 still violated JSON structure. Development-only v3 uses a short
Russian instruction with explicit field types after the ordered images and
Russian timestamp labels, matching the official image-then-instruction pattern.
This changes prompt and placement together; it does not isolate a causal factor.
