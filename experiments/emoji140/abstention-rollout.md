# #140: conservative Kolobki decoration on available hardware

2026-10-23. This follow-up implements the owner's later request to self-label,
self-evaluate and show actual system-enriched bot text on the current machine.
It supersedes the original research-only boundary for this bounded implementation.
It does **not** convert missing BF16 measurements or single-AI labels into human
gold. Earlier dated reports and raw results remain intact.

## Decision

Do not use dense similarity alone for live emoji selection. Use the embedded
reviewed Kolobkitop concept gate for Telegram message sentences and widget option
icons. It runs in Go on CPU with no runtime inference, embedding call, dependency,
paid API or larger GPU. 27 of 52 visually inspected Kolobki concepts are enabled;
25 unqualified/context-sensitive concepts abstain. Every content word must be
supported, multiple matches abstain, negative/uncertain/failure text rejects.
Unknown subject/color/action details reject rather than force a related emoji.
Quotes/code/links/questions and existing emoji remain undecorated. This is a
closed vocabulary with intentionally limited paraphrase coverage.

## Evaluation

All labels and judgments are by the same AI that reviewed the animations and
implemented the gate. The new examples were constructed after the rule draft;
they are a held-out engineering regression set, **not** an independent blinded
estimate of open-world quality. Data was frozen before execution; only the 18
calibration examples informed lexical corrections. The 84 test examples were
run after freezing the policy and were not used to expand its vocabulary.

| Slice | Positives accepted / total | Correct among accepted | No-match false accepts |
|---|---:|---:|---:|
| New calibration | 6/6 | 6/6 | 0/12 |
| New test | 31/36 | 31/31 | 0/48 |
| Original frozen Kolobki queries | 15/20 | 15/15 (grade >=2) | 0/5 |

New test positive coverage is 86.1%; its 95% Wilson interval is approximately
71.3–93.9%. Zero false accepts on 48 negatives still has an upper Wilson bound
of 7.4%; zero on the original five has an upper bound of 43.4%. These intervals
describe the finite examples, not independent real-world generalization.

The original R2 AI-caption dense score threshold accepted 1/5 Kolobki negatives
and 18/20 positives (17/18 correct). The new gate trades coverage for fewer
observed inappropriate choices. Original k04 accepts kolobki-024, which has
relevance 3 alongside kolobki-006; compare against all frozen qrels, not the
first dictionary key. No claim of statistical superiority on five negatives.

Calibration caught broad-prefix collisions: death/smell-like words under the
Russian laughter prefix, climbing down under tears, clan under bowing, and
"clumsy" under axe. These rules were narrowed before the test run. The five
test misses are abstentions: a mouth-action inflection and four unsupported
paraphrases (greeting, enjoying music, successful completion, developing code).
They remain misses; the test was not repaired into a perfect score.

Frozen inputs, decisions, denominators and policy hashes are in
`evidence/abstention/`. Reproduce from the repository root:

```sh
go run ./apps/orchestrator/cmd/emoji-evaluate < experiments/emoji140/evidence/abstention/test.jsonl
python3 experiments/emoji140/build_decoration_catalog.py
```

The Go evaluator reads explicit AI query labels and emits decisions without
network/credentials. The Python script only regenerates the reviewed JSON data
from the frozen public metadata and literal rules; Python is not a product
dependency. Rebuild the Go binary after regenerating the embedded catalog.
`original-kolobki-results.jsonl` additionally records the selected relevance
grade from the existing multi-positive qrels; its `correct` uses grade >=2.

## Runtime invariants

- Opt-in `-telegram-emoji-decoration` requires the owner communication profile;
  default off. Shared profile JSON and Broker binary need no change.
- Original canonical publication and `Part.Text` are unchanged. The optional
  existing-outbox manifest pins policy digest, source hash, insertions and
  option IDs. Selection is bounded Go, with no network/inference in the transaction.
- At most three distinct emoji per text part; UTF-16 entity offsets account for
  supplementary characters and multi-codepoint fallback emoji. Oversize choices
  fall back to plain text before persistence.
- Sender/retry/widget projection renders stored choices. Button order, labels,
  callback data, draft selections, question lifecycle and current authorization
  are preserved. Option descriptions participate in rejection; options with an
  unreviewed image are left without an added icon.
- No catalog or instruction is added to the main model prompt/tool schema.
  No queue, model service, SQL migration or embedding runtime is introduced.
- Photo captions and rich report blocks are outside v1. A sendMessage portion
  accompanying a photo can be decorated like other plain messages.
- Turning off the flag affects new publications. Drain existing decorated Parts
  before rollback to an incompatible old binary. UNKNOWN is never silently resent.

## Research completion disposition under the revised request

The reproducible corpus and measurements are retained in `report.md`,
`followup-report.md` and `retrieval-results.md`: 450 original elements plus all
52 Kolobki, real IDs/hashes, 502 single-AI annotations, complete Kolobki qrels,
the measured EmbeddingGemma2 text and direct image paths, and the NF4 pilot.
The measured retrieval job took 977.944 s, with 1368 MiB peak reserved GPU memory
and 2097 MiB peak RSS. Runtime decoration does not use that GPU process.

V1/V2/V3 BF16 and their single/multiframe comparisons remain not-run on this
hardware; the NF4 pilot was not good enough to recommend automatic captioning.
No blind human comparison, complete original pooled qrels, universal OCR/emotion
qualification or independent rater agreement is claimed. Import labor was not
timed as human work. These are explicit limits, not reasons to rent hardware or
hold the user's requested bounded implementation waiting for another evaluator.
The recommendation is a no-go for autonomous dense/captioner-based decoration,
and an opt-in for the reviewed, conservative Kolobki subset tested here.

## Validation and live evidence

Final test counts, deployed source/binary identity and native Telegram evidence
are recorded below after the run. Existing communication-resume changes from the
other agent are retained in the deployment integration checkout.
