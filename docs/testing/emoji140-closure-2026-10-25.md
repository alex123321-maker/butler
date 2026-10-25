# Issue 140: closure of the owner-selected available-hardware scope

## Scope and decision

The original issue proposed three BF16 captioners and independent human labels.
The owner subsequently said there was no larger GPU, allowed self-annotation,
and explicitly requested: “Доработай отказ от неподходящих эмоджи, особенно на
колобках, и добивай задачу на текущем железе. Разметку и оценку делай сам.”
The same request asked to see actual system-enriched bot text. These are direct
owner messages in chat `01a16724-3185-7351-b244-e901da05a09e`, not a new
interpretation of an unchecked research checklist.

Close the completed scope selected by the owner. The result is **no-go for
autonomous captioner/dense selection**, with a narrow opt-in Go concept gate
for 27 reviewed Kolobki concepts. This does not complete or validate the original
BF16/human benchmark. Hardware rental, further training and a larger benchmark
are unnecessary for this negative research decision and were not authorized.

PR [144](https://github.com/alex123321-maker/butler/pull/144) was merged as
`69053a65`; its final source/report commit is `51abbfc1`. Existing evidence is
in [the pilot](../../experiments/emoji140/report.md),
[retrieval follow-up](../../experiments/emoji140/followup-report.md), and
[abstention/live acceptance](../../experiments/emoji140/abstention-rollout.md).
The current review used main source `8315227a`; it made no new server operation,
model call, Telegram send, installation or production change.

## Evidence and explicit limits

| Original criterion | Evidence or retained limitation |
| --- | --- |
| Real files/IDs/hashes/splits/skips | Inventory 642; original downloaded corpus 450 plus separate 52-item Kolobkitop cohort. Frozen retrieval lock, exact file identities, explicit skips and duplicate groups are committed. Near-duplicate recall remains unknown. |
| V1/V2/V3 and frame comparison | Full BF16 matrix not run. Separate 90-call Qwen3-VL-4B NF4 feasibility study retains each prompt revision and the six single-frame attempts. Its best broad schema-validity result is 57/60, below the 98% gate. It cannot rank the proposed BF16 models. |
| Independent blind descriptions/qrels | Replaced by explicitly authorized self-evaluation. 502 AI annotations; original 120 queries plus 25 Kolobki queries. Kolobki has exhaustive 25×52 AI qrels. Original corpus qrels are seeds, not complete recall. Zero independent human raters; no blind-human quality or inter-rater agreement claim. |
| Captioner vs embedding | R0 metadata lexical, R1 AI-caption lexical, R2 AI-caption text embeddings, R3 fixed RRF and R4 direct multiframe EmbeddingGemma2 are separately reported. AI captions confound general captioner claims. R4 is measured, not hypothetical; R4 loses temporal order and misses a bubble-gum animation. |
| Resources/cost | Historical 90 attempts include schema failures/truncations; no OOM/inference exception. Retrieval: 977.944 s, peak reserved GPU 1368 MiB, RSS 2097 MiB. New 52-item pack: import 20.52 s, decode child wall-time sum 21.23 s, 57.56/59.96 s image embedding by background. 100-item forecast is explicitly linear, not a new measurement. Human labor and energy were not measured; no currency saving claimed. |
| Reproducibility/private artifacts | Pinned revisions, frozen lock, metrics, per-attempt outputs, commands and safe IDs/hashes are tracked. Original media/weights/credentials are not republished. Historical private PG persistence/restore and native-channel evidence remain dated historical evidence. |
| Recommendation by use | Objects: reviewed labels can aid manual candidate choice. Emotions: close alternatives remain ambiguous. Animation: mean embeddings do not qualify temporal understanding. OCR: CER/hallucination qualification absent. No automatic captioning/dense deployment recommendation. |
| Product boundary | The owner separately authorized actual bot enrichment. The finite Go gate is opt-in, CPU-only, no per-response inference. Existing outbox choices remain frozen and canonical/model text and option IDs are preserved. This bounded rollout is not the unimplemented automatic ML pipeline from the original proposal. |

On the self-labelled 84-example test, the conservative gate accepted 31/36
positive examples, all 31 correctly, with 0/48 negative false accepts. Five
positive abstentions remain misses. The existing report gives Wilson intervals:
positive coverage 71.3–93.9%, zero-negative-error upper bound 7.4%. These describe
this engineering set, not independent open-world reliability.

Historical native acceptance includes actual enriched text and widget delivery,
canonical-content preservation and restart checks. It also retains the failed
dynamic-binary launch and rollback before the static build. None of these was
rerun against the working bot during this local-only closure review.

## Fresh validation on this PC

The closure review reran, without inference:

- Nine top-level Go tests for public emoji client boundaries, Russian lookup,
  canonical/UTF-16 preservation, abstention, planner/sender and stable widget IDs.
  Zero skips/failures. PG-dependent and native-channel tests were not selected.
- Five frozen-evidence integrity tests: unchanged input hashes, partition groups,
  real IDs/hashes, query denominators and exhaustive Kolobki qrels.
- Five retrieval metric tests: grade-1 rejection, absent nDCG, abstention threshold
  ties and uncertainty with zero observed errors.
- Rebuilt the Go evaluator and reproduced all 84 stored test decisions exactly:
  31/36 positives accepted correctly, 0/48 false accepts; five abstentions retained.
- Scoped Go vet and diff whitespace check.

Python must run with `-X utf8` on Windows: the first default-codepage run failed
to decode UTF-8 fixtures under cp1251. It made no data changes. The corrected
invocation passed. `test_infer.py` imports Linux-only `resource` and is not claimed
as a fresh Windows run; its earlier Linux schema validation is historical.

```powershell
go test -count=1 -json ./apps/orchestrator/internal/channel/telegramrich ./experiments/emoji140 -run '^Test(Emoji|RussianLookup|CommunicationDecoration(Canonical|Abstention|Planner|Widget|Option|Existing))'
python -X utf8 -m unittest discover -s experiments/emoji140 -p test_evidence.py -v
python -X utf8 -m unittest discover -s experiments/emoji140 -p test_retrieval.py -v
go build -o .cache/emoji-evaluate.exe ./apps/orchestrator/cmd/emoji-evaluate
Get-Content experiments/emoji140/evidence/abstention/test.jsonl | .cache/emoji-evaluate.exe
go vet ./apps/orchestrator/internal/channel/telegramrich ./experiments/emoji140 ./apps/orchestrator/cmd/emoji-evaluate
```

The original dated study files remain unchanged. Any future claim about BF16
captioner superiority, OCR quality or independent user retrieval needs its own
prospective experiment; closure grants no such claim or automatic activation.
