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

Calibration caught broad-prefix collisions: death under the
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

Local test counts and initial native Telegram evidence are recorded below.
Existing communication-resume changes from the other agent are retained in the
deployment integration checkout.

Final local validation: 102 Telegram PG/race cases, 288 Communication cases and
13 control cases passed without failures/skips (native transport corpus excluded
from this run). The additional option-description veto test, owner/profile/proxy
App tests, vet and integration-build decoration/ownership tests also passed.
Private PostgreSQL fixtures INSERT the frozen manifest at creation; the immutable
payload trigger correctly rejected an earlier faulty UPDATE-based fixture.

On laba Ryzen 5 5600H, two-CPU container, 1000 iterations of three-sentence Freeze:
525629 ns/op, 116009 B/op, 1541 allocations/op. This is measured preparation
cost, not network/model latency; the embedded catalog is 29614 bytes.

Initial deployment of a dynamically linked binary failed at process startup in
the target image (missing loader). The previous candidate was restored before
rebuilding with CGO_ENABLED=0 and checking -help in the actual runtime image.
The static integration source 4ec1ce01 retains #142 commit 4cd45a1c; its running
binary SHA256 is aec2f379613322b0f4d93a1ca0c5d685a4201756aa80c54e690d1189eae00e83.
Only the Core candidate mount and CLI flag changed; profile, proxy, Broker,
budgets and history were preserved.


### Initial native acceptance on 4ec1ce01

The real profile bot is @butler_rich_lab_20261021_bot. Ordinary Telegram input,
model output, scoped owner approval, Broker publication and durable outbox
produced ACK 78 for the exact canonical three-line text "Привет! / Слушаю
музыку. / Спасибо большое." The input completed. Telegram Web rendered three
actual custom-emoji elements with IDs 5456201430635386946,
5456606106748983383 and 5456365098954136383; these are the reviewed waving,
headphones and thumbs-up Kolobki. The canonical publication and Web result
remain plain, unchanged text. No direct Bot API demo send substituted for this
path.

The negative text "Не готово. / В красных наушниках. / Синий пузырь жвачки."
was delivered as ACK 80 without a decoration manifest or custom-emoji DOM
elements. Its delivery succeeded, but the run subsequently stopped at
model_budget_reserved with memory_source_check_timeout. That is not successful
run completion. The corresponding #142 pool fix is being validated separately;
the initial stopped trace is retained in native-initial.json.

Additional review found that option labels containing an existing emoji could
receive another icon. Commit cd1d0c3c moves symbol rejection into the common
selector, with regression cases for conflicting crying/fire emoji in labels.
The decoration race tests passed and all 18/84/25 frozen evaluator decisions
remained unchanged. No label or held-out query was modified.

### Combined candidate and first widget attempt

At 2026-10-23T08:15:05Z the Core alone moved to integration
9b29a94c7abe06a0256d4353a6aa2d9812907c86, combining #144 cd1d0c3c and
#143 70bccfec. The running /proc/1/exe hash matched
bea57284c2e1648d5b114dc82097a21921977160093d60692200abff4f670cb9.
The private backup was validated; unchanged profile/proxy/Broker/budgets/history
and original main source pointers are recorded in deployment.json.

The final integration delta passed 82 App, 104 Telegram, 3 ownership, 2 source
observer and 81 Experience tests, with vet passing. Experience's external
pg_dump/restore procedure case was gated/skipped; it is not claimed as passed.
The previous integration 4fef99d had the separate full matrix of 587 targeted,
23 control and 1024 regression passes (8 gated broad-suite skips). Neither result
is relabeled as a full-suite run of 9b29a94c. Both GitHub workflows for #144 code
head cd1d0c3c passed.

Own negative input c380 was closed through the normal owner control, preserving
its original error and old observations/reservations/NULL usage/publications.
The queued widget input 7de3 then started, but stopped before its first model
attempt with "memory retrieval failed: context canceled". There were zero main-model
attempts, approvals and publications. A separate local retrieval reservation
was created for this run (4096 token allowance, 60000 ms allowance, zero monetary
cost); these are reserved bounds, not observed usage, and remain in the ledger. Its expiry was still the next day; no
restart was made after cutover. This failed native attempt is retained in
widget-initial-failure.json and is not widget acceptance.

### Final shared runtime and native widget

The local retrieval failure exposed a separate periodic local-authority observer
using the saturated business pool. #143 reproduced it on PostgreSQL, retained
admission/result/ledger transactions and moved only the periodic observation to
the existing reserved connection, with rollback and a safe typed cancellation
cause. Integration faf188b3 passed 591 targeted, 23 control, 12 local-authority
and 1032 broad regression cases (0 failures; 8 explicit broad environment gates),
plus vet and static builds. The root adb833ea CI was also fully successful.

A further scoped owner-close attempt returned 403 because the early memory
failure had no communication_admissions row. No direct SQL close was used.
The d3d32c96 correction allows only the ordinary Web/Telegram early stopped
receipt with matching durable owner/session and current verified Web binding,
while retaining task/lease/claim/effect guards. Its full close PG/race suite
passed 31 cases, 0 failures/skips, and vet. Initial 403 and the successful audited
close are retained in early-close-failure.json and widget-failure-close.json.

At 2026-10-23T09:08:30Z Core moved to integration
da0ba4c08b2242d37ce4d09222324d8576bc9a0a (#144 cd1d0c3c + #143 d3d32c96).
The actual running executable SHA256 is
46a5e68f61d1ee7fc08a6a1e557625e020ce45e5afc18172e4a4987d032e1a91.
Build flags/path, validated private backup, unchanged operational settings and
the exact preceding validation sources are retained in deployment-final.json.
The close-only delta is not mislabeled as a full-suite run of da0ba4c0.

Fresh widget input d7c1 completed through the normal model/Broker/approval path.
ACK 83 carries the unchanged option list with two sentence insertions; ACK 84
has music/code option icons and no receipt icon. Telegram Web DOM and screenshot
confirmed actual IDs 5456606106748983383 and 5458569525278547985 on the first
two buttons, zero on the third. With this question open, Core was restarted at
quiescence using the same binary. Stored representations and ACKs stayed exact.
The actual UI selection returned option_ids=["music"], the question resolved
and its closed projection appeared. The model's canonical "Готово." was delivered
as ACK 85 with reviewed thumbs-up ID 5456365098954136383. The Input reached
completed/finalizing with no error. See native-widget.json and widget-restart.json.

The fresh negative Input 07879c1b delivered the exact three-line text as ACK 87
without a decoration manifest; Telegram Web contained zero custom-emoji elements.
It completed without an error. For both final widget and negative cases,
accepted_inputs.status, runs.status and runs.current_state are completed, and
there is exactly one durable run_final_results record. The widget finished at
09:13:09Z and negative at 09:16:04Z. The retained accepted-input checkpoint name
"finalizing" is not the run state. See native-negative-final.json and each
artifact's durable_terminal record.

The requested bounded implementation is enabled on the existing laba host and
profile bot, with native text, option icons, abstention and restart evidence.
This is not evidence that every custom-emoji pack or arbitrary paraphrase is
supported. Source images/contact sheets and account screenshots remain local;
only synthetic test text, public metadata, hashes and scoped evidence are in Git.
