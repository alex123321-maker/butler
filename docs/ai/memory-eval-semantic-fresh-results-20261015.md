# Fresh semantic memory trial: independent final review, 2026-10-15

Verdict: **PASS_FRESH_DOCUMENT_MEMORY_HARNESS** and **PASS_INDEPENDENT_SEMANTIC_REVIEW_COMPLETENESS**, scoped to the fixed 12-case, 38-answer document-memory trial below. Collection, persistence across a PostgreSQL restart, original-answer retention, expense attribution and offline replay are complete. The observed semantic result is **29 correct, 9 partial, 0 incorrect, 0 unresolved**; all 38 answers are correct relative to their actual available context. This is a diagnostic result for this sample, not a general quality or promotion approval.

Reviewer: independent model agent `/root/memory_independent_review`, separately authorized by the owner; candidate and runner author: `/root`. I authored the independent judgment JSON and this report, read the original evidence without modifying the sealed trial, and made no authentication, embedding or inference requests. This review is fallible model judgment with explicit evidence, not mathematical proof or a substitute for independent human judgment.

## Frozen inputs and evidence

Executed root: `/srv/storage/butler/agents/memory-semantic-trial-20261015b`.

Executed release: `c8193fa7eee04ff4097a4c8943447f4dfe080976`; immutable source snapshot: `/srv/storage/butler/agents/memory-eval-semantic-20261015/snapshots/20261015-164602-865`. Subsequent result/document commits are not attributed to the executed release.

| Artifact | SHA-256 |
| --- | --- |
| Source tree | `43887f96dd72bb41683faaff22a65c70630bb5b109d7b1dcaf00e319933c9e6c` |
| Collector binary | `e956488c2ea57aa09e87c406bcb91dff6949d20c1db082d86ee71d1aadbbee25` |
| Offline CLI binary | `740ed985fecda78261392b5e46cd86ba12ed3ebb4c715a97a8e8dd3e014690b1` |
| Semantic pins | `ae49adc56028de91908e086f516c0bfb3a4b7a2524d4bca2f7974c973c93c936` |
| Corpus, ru-memory-semantic-v1.json | `9992c0ae196aa85ebdd1b0869aa435d733296f43164a30c8e989d012c5ea973b` |
| Semantic methodology | `ab41d8ec5040804d9060cdcf58762d3ef342329a9d2174533535141c42c75132` |
| Independent per-answer review JSON | `703007d84492950624bd1a269f49c8cf9670568a01534c2437260a4019df63e7` |
| Go offline replay/scored JSON | `95cab070b51539ab2832d7036359f034b06b148ca667b525f970f7f6f8121a13` |

The [source review](memory-eval-semantic-source-review-20261015.md), [prepared entry review](memory-eval-semantic-prepared-entry-review-20261015.md) and [preflight/calibration review](memory-eval-semantic-preflight-review-20261015.md) retain their earlier scoped checks. This final review does not rerun or broaden them. The prepared pin-only helper ran as the host UID to read its owned build directory; actual writer/reader execution retained UID 10001 and the reviewed restrictions. The preceding trial-a preparation failed before DB/auth/model entry and remains separate.

I independently read 282 explicitly named safe files through a network-disabled, read-only UID-10001 Docker helper, without reading credential imports, settings or keys. These included all 38 input/final/event/result/auth sets, all 36 auxiliary request/response originals, both phases' bundles/searches and the following ledger/manifest evidence. Local safe copies matched the same raw bytes.

| Root-relative private artifact | SHA-256 |
| --- | --- |
| writer-state.json | `14fc25d0f557f7a9369970acb0e3ed4850ae401366e3400cb732e0eb6a9f1e57` |
| writer-captures.json | `f4dd62702a7c56016b6da31d421d417a00e5e262c013ff3fd23c5030bac9f63c` |
| ledger.json, writer export | `94c503250c0fbfd49295bf6c00877ada298efd93337808bfaacf7990fa7aac1a` |
| read-after-restart/inputs-before-native.json | `1039c023f2137909caa32976a463900f0da133b3c70636a04d95699907df7162` |
| read-after-restart/captures-before-native.json | `f4dd62702a7c56016b6da31d421d417a00e5e262c013ff3fd23c5030bac9f63c` |
| read-after-restart/measurements.json | `ec4207b5540fd24c2465882a587bb6c988be56f95c12a998e46a984b05787b59` |
| read-after-restart/ledger.json | `e96d4aab8cad218ba99e0c195dbb2349a23187c7c47d1c238e3f0708854d3339` |

## Semantic judgments

The [independent review JSON](../../testdata/acceptance/results/ru-memory-semantic-v1-review-20261015.json) binds every judgment to case ID, arm, exact input/output SHA, corpus, measurements and methodology. Each item records available-context and full-corpus verdicts separately, evidence with source/revision/quote, uncertainty and applicable action judgments. I read every original answer and actual model-visible context; a citation ID alone was never treated as proof.

| Arm | Completed | Correct for full question | Partial | Incorrect / unresolved | Correct for available context |
| --- | ---: | ---: | ---: | ---: | ---: |
| no_ltm | 12 | 4 | 8 | 0 / 0 | 12 |
| oracle | 12 | 12 | 0 | 0 / 0 | 12 |
| retrieval | 12 | 11 | 1 | 0 / 0 | 12 |
| retrieval_repeat | 2 | 2 | 0 | 0 / 0 | 2 |
| Total | 38 | 29 | 9 | 0 / 0 | 38 |

All 38 scheduled inputs completed once; failed, skipped and unobserved counts are zero. The two repeats are additional generation observations for f01/f08 using the identical model-visible retrieval payloads; they are not two new searches or an enlarged independent retrieval denominator. Both pairs were semantically consistent.

Eight no-LTM answers safely stated insufficient evidence for facts that exist in the full corpus. They are partial at the task level, with no answer-generation error attributed. Baseline successes concern genuinely unresolved facts (f07/f08/f09) or the current task's explicit restriction (f11), so they are not evidence of memory retrieval.

The one retrieval partial is f12. Its actual context contains f12s1's address and f12s3's opening hours, and the answer correctly supplies both while explicitly declining to invent the missing pickup code. Code PF-6281 is present in the full corpus and oracle input. The final ledger independently binds f12s2 to active catalog source `source_8aed21af9436248747da04212152925d5e43b2a7ed7cd5a33e7447d78a091f54`, head revision 1, one retained source-revision body and one derived chunk containing that code. It is absent from the actual retrieval prompt. This is a context-selection miss, not an unsupported answer.

This single-read contour uses the product's default ChunkLimit 2; Search limits results to min(4, ChunkLimit). The three required f12 facts occupy separate source chunks, while the captured result supplies two. BundleBudget 12 counts item slots, not tokens or words. These facts establish the observed top-two/context limitation; they do not identify an embedding defect or prove a multi-step agent could never retrieve the third fact.

Other attribution checks matter: f10 retrieval's extra Topol-blue fact is actually visible in f10s1 and is supported. The oracle arm's statement that Topol is absent from the provided data is qualified to its smaller actual input. f07 has no confirmed selected time, f08 has an unresolved equal-date conflict, and f09 has no available phone number; uncertainty is appropriate. All three f11 answers respect task revision 7's checklist-only restriction and paused sending. These are model-proposed answers, not proof of external actions.

All 40 model citations across 26 answers were checked against original source text/revision and actual input visibility; each was authentic, visible and relevant to its supported claim. No unsupported assertion or canonical-format rejection was identified in this sample. The [Go replay](../../testdata/acceptance/results/ru-memory-semantic-v1-scored-20261015.json) preserves the same 38 bindings and counts; its raw file hash was independently checked.

## Runtime, persistence and originals

Actual write/read execution receipts match the reviewed image `sha256:5cf287a799e6b94384bad13d16b14904c531f51ba65792237e122ce42b392f61`, source/binary mounts and arguments. Both phases ran UID 10001:10001, 2 CPUs, 1536 MiB, 128 PIDs, read-only root, no added capabilities, all capabilities dropped and no-new-privileges. Source/build/metadata/auth-reference mounts were read-only. The writer had the exact own-PG/local-inference networks; the reader additionally had backend plus the required proxy and local bypass. Receipt SHA values are `19dae3803755db57126e0cdfec1110f2303bb620cf44edd05eee3bf90715780a` (write) and `5992cf310fe7c87834f6b1f1199ac6c140890c7c6cfa5b602248ddf0b73c566a` (read).

Independent final Docker State inspection found exit 0, OOMKilled=false, no error and restart count zero for each writer/reader container. Writer finished at 09:49:17.563385148Z; PostgreSQL's new start is 09:49:18.440877161Z; the distinct reader process started at 09:49:18.852041174Z and finished at 09:55:53.262937476Z. PostgreSQL then stopped cleanly at 09:55:53.672655002Z. This establishes writer exit → database restart → new read process rather than an in-process memory carryover.

All 12 post-restart captures match the writer capture file byte-for-byte. Current catalog source bodies/revisions, 28 active source-to-chunk/object-source relationships and 12 ready 768-dimensional index generations also agree across the writer/final exports. Reader query embeddings are new requests; the source/index records survive the restart. f09's deleted placeholder is excluded from model inputs; this tests that contour, not erasure of a previously embedded real secret.

For all 38 native samples I recomputed the frozen input SHA and model projection, matched raw final bytes to measurements, result output SHA and SQL output SHA, and correlated the original event chain/run ID. The 305 event IDs are unique; each sample has its start, one final and one completion, with no transport error or tool proposal. Final content, STOP finish reason, model version `gemini-3.8-flash-n` and usage agree. The 38 auth originals bind run/input and their SQL hashes, with 38 completed paid-discovery HTTP-200 receipts and one completed OAuth refresh plus completed durable persistence. No auth is inferred for unobserved history.

All 114 dispatch/final guards bind exact operation, current source references, task revision, constraints and live lease. Catalog aliases, selected source IDs, compact original bundle digests, actual prompt excerpts, raw searches, chunk IDs, source ownership and index generation identities correlate. All 36 auxiliary originals match SQL request/response hashes, embedding profile/revision, outcome, reservation and vector dimensions: 12 document-batch requests and 24 query requests produce 52 finite 768-dimensional vectors. Writer and reader auxiliary request IDs are disjoint.

## SQL export, counters and costs

The final PostgreSQL container was already stopped. I correlated its sealed **20-table SQL export**, not a new live SQL query, and did not restart or mutate it.

The closed window records 74 admissions (38 native + 36 auxiliary), reserved units 1,965,858, prior calls/units zero, and the unchanged four-hour deadline 2026-10-15T13:49:00.981463Z. PostgreSQL represents the writer's nanosecond deadline at microsecond precision, 258 ns earlier, not later. The 74 budget reservations, 38 model attempts, 38 fixture attempts, 38 auth rows, 36 auxiliary rows, 114 guards, 38 result rows and 452 expense rows correlate exactly. Reserved units are conservative admission allowances, not measured provider tokens.

The other export counts are 12 auxiliary jobs, 28 chunks, 12 index jobs, 28 object-source rows, 29 source revisions, 29 sources (28 active and one deleted placeholder), 12 semantic generations, and 62 runs/sessions/ownership records. Twelve task rows remain active without a finished timestamp. Stored result rows retain their original pending-semantic-review scope; independent judgments are separate artifacts rather than backfilled SQL outcomes.

| Stage | Expense rows | Logical calls |
| --- | ---: | ---: |
| write | 88 | 12 |
| embedding | 62 | 36 |
| background | 50 | 0 |
| read | 62 | 62 |
| generation | 38 | 38 |
| extraction / summary / consolidation / retry | 152 | 0 |
| Total | 452 | 148 |

All 60 writer expense rows remain in the combined final measurements; every exported expense matches the same measured record. Each answer has all required generation and disabled-stage records; retry is explicitly unused for all 38. There are no failed expense outcomes. Logical stage calls are a different unit from 74 model admissions or underlying HTTP requests.

Native provider usage totals are 13,313 input, 3,910 visible output and 46,503 provider-total tokens. The difference is not assigned an inferred hidden-token breakdown. Local tokenizer input/total usage remains unknown for the 36 embedding calls. Provider invoice amounts remain unknown; the owner-supplied incremental subscription price premise does not measure hardware or infrastructure costs. The local price basis, logical retained-byte figures and zero disabled-stage calls do not establish zero physical storage, CPU, energy, PostgreSQL or host-memory cost. Kernel/background overhead has explicit unknown fields.

Nearest-rank native latency p50/p95/max is 9,471 / 12,394 / 47,752 ms; the arithmetic median is approximately 9,484 ms. Embedding p50/p95/max is 84 / 95 / 12,976 ms. The single writer embedding outlier is observed without a cold-start diagnosis. Native sample latency includes the collector's surrounding work; it is not a separately measured pure-provider latency. Writer/read phase durations likewise are not sums of independent stage times.

## Acceptance boundary

This final evidence supports a complete, reproducible comparison of the declared document-memory contour and an independently reviewed semantic result. It demonstrates durable write/read behavior across restart, faithful original-answer/citation/version/usage retention, complete finite attempts, and costs/unknowns carried through the SQL export.

It does not establish general agent-task completion, tool effects, production activation, population-level model quality, all memory layers, full live-revision replacement, actual secret-derivative deletion or paid invoice reconciliation. Twelve synthetic public scenarios and two correlated repeats cannot establish those properties. No prompt, corpus or judgment rule was expanded in response to observed fresh answers.

The historical ru-memory-v2 canonical scores, six transport failures and eight skips remain unchanged. Their [retrospective semantic reassessment](memory-eval-v2-semantic-reassessment-20261015.md) separates evaluator false positives from grounded incomplete answers. This fresh methodology and trial do not silently turn the earlier numeric NO_GO into a promotion pass or reuse its fixed quality/cost thresholds. The current scoped harness acceptance and the limitations above should be stated together in PR #95.
