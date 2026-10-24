# Telegram custom emoji research (#140)

Status: experiment plus explicitly requested, bounded Telegram rollout (2026-10-23).
The owner subsequently requested self-annotation/evaluation on the available
hardware and actual enriched bot text. This supersedes the original research-only
restriction for this narrow renderer. It does not establish completion of the
unavailable BF16 captioner matrix. The experimental boundaries below describe
the corpus/ML stage; the runtime delta is specified separately below.

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
5. Self-annotation is an AI-rater exploratory layer, not independent human ground
   truth or the two-rater assessment in the issue. Observe original frames before
   caption outputs; retain provenance and ambiguities. No evaluation score is
   established by agreement between captions and these annotations. Retrieval
   diagnostics on AI labels must identify the shared rater/query-author bias.
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

## Authorized self-annotation and additional Kolobki cohort

The user clarified the intended installed pack with a pixel-art screenshot.
Its visible Telegram tab document ID was resolved by read-only
`getCustomEmojiStickers` to the public set `Kolobkitop`. The existing operator
importer ingests all52 items into a separate run. Original450 membership and all
previous evidence are preserved. Browser UI inspection reads only the intended
pack/test bot; account sessions, cookies and chat contents are not research data.

All502 entries have explicit single-AI visual annotations. The original450 split
is73/90/287 after conservative grouping of related icons; this is not the planned
60/90/300 human study. Query/input hashes freeze before ranking. The52-item pack
is a separate exploratory cohort; it is not used to tune original thresholds.

Offline exact search compares metadata lexical, AI-label lexical, AI-label dense,
fixed RRF and direct frame embeddings using the existing pinned EmbeddingGemma2
BF16768d model. Original450 and additional52 rank separately. Direct image
vectors average normalized frame embeddings and normalize again, losing temporal
order. Light/dark variants are separate. This does not install a renderer,
change the model catalog, create a Butler tool, or qualify automatic captioners.

Original450 relevance seeds are incomplete, so their metrics are seed-hit
diagnostics, not full recall/pooled nDCG. Separate Kolobki qrels explicitly grade
all52 candidates per25 queries, still by the same AI. Human independence is not
claimed. Calibration chooses abstention only on the30 original calibration
queries; transfer to Kolobki is reported separately with uncertainty. No threshold
is retuned after test results. Production and issue completion remain gated by
the missing original criteria recorded in the follow-up report.

## Reviewed runtime alternative, 2026-10-23

See `experiments/emoji140/abstention-rollout.md` for evidence and limitations.
Dense score alone is not approved to select emoji for live messages. The available
6 GiB GPU remains sufficient for the measured embedding experiment; runtime
decoration needs no GPU, model request, vector database query or new dependency.
An embedded, single-AI-reviewed Kolobkitop catalog enables 27 of 52 concepts.
The remaining entries abstain. Prefix/word rules are a closed vocabulary, not
a general semantic verifier. A trigger alone is insufficient: every content term
must be supported, ambiguous matches reject, and negatives/uncertainty reject.
Quotes, code, links, questions, existing symbols and oversized output abstain.
The whole message's failure/negation vocabulary suppresses sentence decoration.

The operator explicitly enables `-telegram-emoji-decoration` together with
`-communication-profile`. Default is off. The switch is process-local; shared
Broker profile JSON, credentials and existing model/Telegram proxy are unchanged.
No tool argument, main-agent prompt, catalog, recipient, capability or authority
is added. Existing Communication admission, Broker calls, current source guards
and the existing outbox still own publication and delivery.

`Planner.Plan` runs only bounded Go selection, then persists a versioned private
decoration manifest in each existing outbox Part: catalog/policy digest, original
text SHA-256, at most three byte-offset insertions with pinned emoji IDs and
Unicode fallbacks, and option IDs mapped to pinned button icons. Canonical
publication payload and `Part.Text` are retained exactly. No inference or network
access occurs inside the publication transaction. Oversize enrichment falls back
to the original representation before persistence. No choice falls back to text.

`Sender` renders only frozen choices into `sendMessage.entities` with UTF-16
offsets. Retries, restart and widget state edits do not select again. Invalid
stored manifests reject before sending; selection failure never changes text.
Widget option icons use stable option IDs and `icon_custom_emoji_id`; label,
order, callbacks, drafts, terminal state and question identity remain unchanged.
Photos and rich report blocks remain outside this first decoration version.
An explicit Bot API rejection and a transport-unknown outcome retain their
existing outbox meanings; this feature adds no alternate send/retry loop.

Disabling the flag stops decoration of new publications. A compatible binary
still renders already frozen Parts. Rolling back to an older binary that lacks
the field requires draining those Parts first; do not rewrite or resend UNKNOWN.
Catalog history needed by persisted choices must remain readable in later versions.
