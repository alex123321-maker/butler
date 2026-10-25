# Emoji research on laba

This directory contains isolated corpus, feasibility and retrieval stages of #140.
The owner subsequently requested self-evaluation on the available hardware and
actual bounded bot enrichment. See the [scope closure and fresh local checks](../../docs/testing/emoji140-closure-2026-10-25.md)
and [opt-in abstention rollout](abstention-rollout.md). The original BF16/human
benchmark remains incomplete and is not a prerequisite for the negative ML
pipeline recommendation. These research utilities do not enable styling.

The first-stage report is historical. See [follow-up](followup-report.md) for
the repaired browser, exact52-item Kolobkitop import,502 AI annotations and
the bounded EmbeddingGemma2 retrieval experiment. AI annotations/qrels are not
independent human ground truth. Original450 and additional52 rank separately.

## Artifacts

- `pins.json`: exact source revisions and additional wheel versions.
- `evidence/inventory.json`: PostgreSQL export of all642 public metadata entries
  and the450 downloaded IDs, with content hashes and explicit unselected rows.
- `evidence/preprocessing.json`: compact derived decode/frame evidence; the full
  private artifact hash is retained.
- `evidence/pilot.json`:60 development candidates, not a final study split.
- `ai-annotations.json`, `ai-food-inventory.json`:143 AI visual annotations with
  real custom emoji IDs and media hashes. They are not human ground truth.
- `evidence/near-duplicates-reviewed.json`:three visually confirmed candidate
  pairs. Candidate recall is unknown; exhaustive deduplication is not claimed.
- `caption.schema.json`, `prompt.txt`: frozen feasibility output contract.

Media, rendered frames, model weights, caches, private DB URL and PostgreSQL data
stay below ignored `.cache`. Do not publish pack media or credential material.
There are no sends, polling calls, paid API calls or training. Authorized browser
inspection covered the requested pack and existing test-bot responses; private
chat contents and account screenshots are not retained in this repository.

## Runtime

Worktree: `/srv/storage/butler/agents/issue-140-emoji-research`.
Base source: `46bd354e`; separate branch `codex/issue-140-emoji-research`.
Go builder:1.27.2. The existing ML image is mounted with its existing venv read-only:
torch2.10.0+cu128, Transformers5.19.0, accelerate1.15.0, Pillow12.3.0.
Additional dependencies are in `.cache/python`; no shared venv is modified.

Python is restricted to this experimental tensor/decoder/evaluation runner.
Go owns import and durable evidence recording. No production constructor or
transport profile is installed. See the [boundary
spec](../../docs/architecture/telegram-emoji-research.md).

The lab's independent PostgreSQL container uses `--network none`, a private
Unix socket mounted at `/socket`, database `butler_emoji140_test`,384MiB memory
and one CPU. It publishes no port. Its data volume belongs to this worktree.
Only the importer and weight preparation join the existing backend network to
reach the existing CONNECT proxy. Offline decode/inference have no network and
no Telegram/provider credentials. Commands must use the image IDs in the report,
not an assumed unchanged mutable image tag.

## Commands

Build/test from the lab worktree using the cached Go builder, own `GOCACHE` and
read-only existing `GOMODCACHE`:

```sh
go test ./apps/orchestrator/internal/channel/telegramrich -run 'TestEmoji|TestCommunicationProxy' -count=1
go test ./apps/orchestrator/internal/channel/telegramrich ./apps/orchestrator/cmd/emoji-research -count=1
go vet ./apps/orchestrator/internal/channel/telegramrich ./apps/orchestrator/cmd/emoji-research
go build -o .cache/bin/emoji-research ./apps/orchestrator/cmd/emoji-research
go build -o .cache/bin/record-artifacts ./experiments/emoji140/cmd/record-artifacts
```

Inside an isolated container with the explicit current private references mounted:

```sh
/work/.cache/bin/emoji-research \
  -token-file /private/bot-token -proxy-file /private/telegram.proxy \
  -database-file /work/.cache/private/database.url \
  -out /work/.cache/research/inventory-01 -source-commit <full-base-SHA>
```

The output directory must be new. The private DB URL is
`postgres://postgres@/butler_emoji140_test?host=/socket`; it has no password because
the experiment DB has no network and its Unix socket directory is private.
Do not use this setting for a networked/product database.

ML commands use `/data/venv/bin/python`, `PYTHONPATH=/work/.cache/python`, read-only
`/data/venv`, and the worktree at `/work`. Only preparation mounts the explicit
current proxy reference. `prepare.py` installs pinned wheels in the worktree and
downloads the4B revision; it does not download8B or gated Gemma. The bounded
`fetch_weights.py` alternative can reuse a stopped standard download's partial
prefix, performs six ranged reads, and verifies the complete pinned LFS SHA256.
A failed range attempt is not a valid model and is not automatically retried.

```sh
python /work/experiments/emoji140/prepare.py
# Offline, with2 CPUs/2GiB container memory; each decode child has CPU/AS/time limits:
python /work/experiments/emoji140/preprocess.py --inventory /work/.cache/research/inventory-01
python /work/experiments/emoji140/select_pilot.py
python /work/experiments/emoji140/duplicates.py
# Offline GPU container; own output name must be new:
python /work/experiments/emoji140/infer.py --limit 12 --name nf4-feasibility-01
python /work/experiments/emoji140/infer.py --limit 6 --name nf4-single-frame-01 --single-frame
```

The inference runner refuses an occupied GPU, caps its torch allocator at82% of
the6GiB device and stops if another compute process appears. This is cooperative
contention detection, not a shared-host scheduler or a hard limit on another
agent. Batch1,512 output tokens, per-attempt120s alarm, no repair/retry. Use an
8GiB container memory bound; stop the experiment container on a stuck CUDA call.
A process killed during a started attempt leaves that attempt incomplete.

The single-frame run is an explicitly labelled feasibility ablation with the
same ordered pilot subset. It is not the180-call three-model ablation in the issue.
`infer.py` logs actual grids/visual tokens, raw output, strict schema result,
truncation, first/total latency, load time, peak allocated/reserved GPU memory,
process RSS and missing planned attempts. Model/processor identities are pinned.
No self-confidence is treated as probability.

Record artifacts atomically in the experiment's PostgreSQL with
`record-artifacts -database-file <private-ref> -run <existing-run> <JSON files...>`.
It appends content-addressed versions; it does not replace prior evidence.

## AI annotation lookup

```sh
go run ./experiments/emoji140/search.go \
  -annotations experiments/emoji140/ai-annotations.json,experiments/emoji140/ai-food-inventory.json \
  -query "отказ"
```

This is exact Russian token overlap, without stemming, negation understanding,
dense embeddings or a calibrated no-match decision. It is an interactive
demonstration over the143 AI-labelled entries, not R1/model-caption evaluation.
Queries written after annotations cannot be relabelled as independent locked tests.

## Follow-up commands

`prepare_followup.py` numbers the remaining307 original items; the corresponding
AI visual labels are compiled by `freeze_followup.py`. `prepare_kolobki.py`
exports the separately imported52 items and five private contact sheets.
`freeze_retrieval.py` freezes all502 annotations and145 queries before ranking;
`grade_kolobki.py` records1300 explicit single-AI grades before viewing rankings.
The52-item cohort is never added silently to the original450 search space.

```sh
# Credentials/proxy are the same explicit private references as the first import.
emoji-research -token-file /private/bot-token -proxy-file /private/telegram.proxy \
  -database-file /work/.cache/private/database.url -packs Kolobkitop \
  -max-downloads 52 -out /work/.cache/research/kolobki-NEW -source-commit <full-SHA>
# After decoding and one-time annotation/input freeze:
sh experiments/emoji140/run_retrieval.sh retrieval-NEW
python3 experiments/emoji140/score_retrieval.py .cache/research/retrieval-NEW
python3 -m unittest discover -s experiments/emoji140 -p 'test_*.py' -v
go run ./experiments/emoji140/search.go \
  -annotations experiments/emoji140/ai-kolobki-annotations.json -query "наушники"
```

The preparers are fixed to the recorded `inventory-01`/`kolobki-01` provenance;
use a new experiment directory when recreating them. Freeze/result writers reject
existing files. Exact vectors remain private; config, query locks, top10 and
metrics are public evidence. `R1_AI/R2_AI/R3_AI` use hand-written AI annotations,
not machine captions. R4 averages frames and cannot establish temporal reasoning.

## Outstanding study gates

No WEBM is present in the downloaded pool. BF16 V1/V2/V3, independent human
ratings, the planned60/90/300 splits, exhaustive near duplicates,120 independent
human queries, complete original450 pooled qrels and independent widget
evaluation remain separate work. The follow-up measures dense/direct retrieval,
abstention and intervals only under the explicit AI-evaluation limitations.
A preview, a decoded file or a valid JSON
caption proves none of these. Gemma is gated; no access terms were accepted.


The later owner-authorized runtime alternative and stronger abstention evaluation
are in [abstention-rollout.md](abstention-rollout.md). Earlier reports remain dated
research snapshots; their "not deployed" statements describe those stages.
