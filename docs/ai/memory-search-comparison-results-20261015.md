# Memory search #79/#80: independent comparison, 2026-10-15

Verdict: **PASS_TARGETED_SEARCH_COMPARISON** and **PASS_SCOPED_TRACE_CORRELATION**. In the fixed eight matched retrieval questions, complete grounded answers improved from **3/8 to 8/8**. All five earlier omissions are now supplied by the actual context. This supports acceptance of the reviewed #79/#80 scope, with the availability and coverage limits below; it is not a population accuracy estimate or production activation approval.

Independent reviewer: model agent `/root/memory_independent_review`; implementation/corpus author: `/root`. I read every completed original and its actual model-visible context, checked witnesses and retained failures separately, and performed read-only correlation of the sealed evidence. I made no auth, inference or embedding requests, changed no implementation/corpus or sealed root, and did not repeat calibration or static source review. This is a fallible independent model review.

## Fixed comparison and judgments

The [preflight](memory-search-preflight-review-20261015.md) fixed semantic interpretation before answers. Corpus SHA is `9c78b9469bb6e61c143020324fe7af8b75e9344e6dd0a2154775f370ddeb964f`; [methodology](memory-eval-semantic-methodology.md) SHA is `ab41d8ec5040804d9060cdcf58762d3ef342329a9d2174533535141c42c75132`. The baseline is product b4a2fd3 plus the common test collector overlay; the candidate runs the same questions, histories, current tasks, model/answer instructions and five-minute answer timeout. Baseline has eight retrieval slots; candidate has no_ltm/oracle/retrieval × eight. No failed slot was retried or replaced.

| Variant / arm | Scheduled | Completed | Correct | Partial | Incorrect / unresolved | Failed |
| --- | ---: | ---: | ---: | ---: | ---: | ---: |
| Baseline retrieval | 8 | 8 | 3 | 5 | 0 / 0 | 0 |
| Candidate no_ltm | 8 | 6 | 1 | 5 | 0 / 0 | 2 |
| Candidate oracle | 8 | 8 | 8 | 0 | 0 / 0 | 0 |
| Candidate retrieval | 8 | 8 | 8 | 0 | 0 / 0 | 0 |

All 30 completed answers are grounded and correct relative to their actual available context. The five baseline retrieval partials and five candidate no-LTM partials are safe, incomplete answers to answerable questions, not observed fabricated facts. Candidate no-LTM g04 succeeds from the current task alone; that is not evidence of memory recovery. The two candidate failures remain in the full 24-slot availability denominator and have no invented semantic verdict. There are no skips or unobserved scheduled inputs.

The independent JSONs bind each judgment to exact case/arm/input/output SHA, corpus, method and original measurements; evidence contains original source/revision/quote plus provided-versus-corpus-only availability. Both Go replay reports reproduce the judgments offline; I checked their saved counts and raw hashes.

| Artifact under testdata/acceptance/results | SHA-256 |
| --- | --- |
| [Baseline independent review](../../testdata/acceptance/results/ru-memory-search-baseline-review-20261015.json) | `a66beb76eff8e90664358d364c722555e967000022e5556d92aad4e69d7db3b6` |
| [Candidate independent review](../../testdata/acceptance/results/ru-memory-search-candidate-review-20261015.json) | `91f464b3a4363ff7dda83ffe6c0750a5977cd9fc8b8a861fe8e496ca0edaa3b8` |
| [Baseline scored replay](../../testdata/acceptance/results/ru-memory-search-baseline-scored-20261015.json) | `45afc4e6dee3bb92ccad743bdfad6b307142dbfe5ec521a9fadd85c0b42c608e` |
| [Candidate scored replay](../../testdata/acceptance/results/ru-memory-search-candidate-scored-20261015.json) | `be33e79e863a2656c88768f404258232fe1bfa6135290f3c315f87940d926983` |
| [Safe evidence manifest/metrics](../../testdata/acceptance/results/ru-memory-search-evidence-20261015.json) | `f96806836b845f740ec32cd45363f7275456ee0763c56eff93eac9598f3baa87` |

Publication note by the primary author: the manifest/metrics row pins the LF
Git blob. The reviewed Windows CRLF serialization had SHA
`8b87e4b02018b8c879e7fd124e1bfc7949bfd4f434da9c812e20d2cb2dea341a`;
only line endings differ. Original-leaf, review, corpus and scored-report hashes
are unchanged and match their committed bytes.

## Per-case evidence and attribution

| Case | Baseline retrieval | Candidate retrieval | Observed evidence change / control |
| --- | --- | --- | --- |
| g01 Ольха-592 | Partial: correct address/hours, code explicitly unavailable | Correct: address, OL-3746, 10:15–18:45 | Baseline two selected chunks are g01s3/s1. Candidate four include g01s2's stored code; g01s4 belongs to another order and is not substituted. |
| g02 Ясень-73 | Partial: no room/floor supplied | Correct: room 418, fourth floor | Baseline raw search already returns the correct g02s1, but the bundle is filled by 12 unrelated profiles. Candidate filters 25 irrelevant profiles and keeps that document. This is an observed search-to-bundle loss, not an embedding failure. |
| g03 Иней-64 | Partial: transport preference unavailable | Correct: train, project-scoped preference | All 26 confirmed profile entries exist, including g03p26. Baseline input has only the first 12 unrelated keys; candidate ranks before the profile limit and supplies zz_iney_travel through memory:profile:51. The ordinary document discusses stationery, not transport. |
| g04 Клён-28 | Correct | Correct | Both preserve current task revision 7: short remarks only, no new full report or sending. Historical usual practice remains data. No-LTM also answers correctly from the task. |
| g05 Липа-306 | Partial: date/time/room unavailable | Correct: 16 October, 13:40, Озеро | Baseline raw search already returns g05s1, but 12 long unrelated profiles consume the item budget. Candidate excludes all 22 irrelevant profiles and supplies the meeting document. |
| g06 Ель-85 | Partial: correct section/time, warehouse unavailable | Correct: Южный, Д-12, after 16:30 | Baseline two chunks are g06s4/s3. Candidate four also include the old revoked warehouse and g06s2's correction; the answer selects the current Южный and preserves the time bound. |
| g07 Берег-120 | Correct | Correct | Both use the specific green-only approval, not Берег-12's blue approval or the conditional general profile. Entity separation and negation survive. |
| g08 Туман-57 | Correct uncertainty | Correct uncertainty | Neither day nor time is confirmed. Candidate's additional Wednesday-15:00 fact is actually visible in g08s2 and explicitly belongs to Туман-58. Candidate no-LTM failed, so its uncertainty behavior is not inferred. |

The oracle's 8/8 confirms that the declared sufficient originals support the questions under the frozen rubric. No exact reference wording was required. A concise project-scoped train answer or green-only answer is complete without parroting separate scope/negation sentences.

All seven baseline and 22 candidate model citations are authentic substrings of the stated revision and visible in the actual input. Six baseline citations and all 22 candidate citations support their associated factual clauses. Baseline g04's g04s1 quote only says that a report was prepared on 14 October; it does not prove the current checklist/stop instruction. That one background-only citation is recorded separately: the answer itself is supported by task revision 7 and visible g04p1. Authenticity alone is not treated as entailment, and this citation limitation is not mislabeled a fabricated answer or executed permission violation.

## Two availability failures

Both failed entries are candidate **no_ltm**, g01 and g08, with empty retained finals, blank model version and unknown provider usage. Their input SHA values are respectively `4e5503fa29e75b7c2bc0515f548ff16de039489837d266b747737618e99f0496` and `330d8f5aa779723109058eb522f5d4baea243fb0f3aef9c21747d6de4c1b4381`.

Safe original events in each case contain run_started → transport_error with typed code **provider_unavailable**. The correlated auth trace locates the failure at **paid_discovery: started → deadline**. g01 first completed OAuth refresh with HTTP 200 and completed refresh persistence; g08 records only the discovery attempt and deadline. Durations are 10,448 and 10,055 ms. These receipts identify a discovery-stage deadline, not a five-minute answer timeout, a semantic error or a demonstrated schema rejection. No retained HTTP status or deeper cause establishes why discovery missed its deadline.

Exact safe leaves are `private/read-after-restart/events-e53db248b23889fac74554ba3f3383f1-no_ltm.json` and `events-7d884b22f7eea029aab94c05238c297e-no_ltm.json`, with matching auth/input/final leaves. They have immutable attempt, model-admission, auth, expense and dispatch/final receipts. They intentionally have no successful QA result file/row: the candidate export has 24 attempts and 22 QA results. My first raw verifier incorrectly requested success-result files for these failures; it was corrected to check retained failure evidence without creating a result. The sealed originals and scores were untouched.

## Independent raw/SQL/persistence correlation

Both roots are retained separately:

- `/srv/storage/butler/agents/memory-semantic-trial-search-baseline-20261015`
- `/srv/storage/butler/agents/memory-semantic-trial-search-candidate-20261015`

| Pin | Baseline | Candidate |
| --- | --- | --- |
| Executed release | `94faa39d072f4e51a0494a3359adcd1e1a433f8e` | `0da5128f0c6743a764de30bc3688923961e4ee70` |
| Source tree SHA | `0665ac9d00bf247252fae377d67940f79d61799ff0232e59c4c1387249010c90` | `f88262364499fb6c8d2539b73e6924c4956e08adaf48f03152434d00e73ada1e` |
| Collector SHA | `d75f1d418bbd590c3d7ae615311e737d60c933a018c16f7665a2b68f22de61f7` | `fec1ed985ec71998ba1b9f6d57a97873db372db9f21db5ba4f8e86cfbab2e328` |
| Measurements SHA | `a7fd40251f3b9b459c1283fcfa262ebe46edc0069d2b42c4dc844d52595fafc8` | `d01e421fd4180917af721627e84748c35f99718bc78c62ed3263ee60349a40f9` |
| Final SQL-export SHA | `ffdae6b76ddd17be47a4e5b326d4706fea072b786e3807314793c91701fb2231` | `1c4c6f461c039a9cd5cfcab082c4fe02b4357627a1536c78a2f0e8e995806519` |

I checked the 16 named safe manifest leaves for each root against both local copies and original remote bytes, recomputed the source TreeHash and collector hash, and correlated 103 baseline plus 181 candidate private originals using read-only, network-disabled helpers. No credential, key, settings or import content was read. The baseline completed its full assertion set before the verifier's candidate-only missing-success-file assumption stopped the first batch; the corrected candidate check passed. Baseline inference or validation was not rerun.

All 32 exact input/model-projection hashes match the attempt ledger. All 30 completed raw finals match measurements, event finals, successful result hashes and SQL outputs; version/usage/event-run identities agree, event IDs are unique and no tool was proposed. The two empty failure finals match their retained attempt/output hashes and separate typed diagnostics. There are 24 baseline guards and 70 candidate guards: completed attempts have two dispatch proofs and one final proof, while each pre-generation discovery failure retains one dispatch and one final proof. Operation IDs, exact source references, task/constraint revisions and current lease deadlines correlate.

Each 23-table export independently contains 91 original sources/revisions, 16 document chunks, 75 confirmed claims/profile entries/current fact heads, 91 typed object-source links, and eight ready index generations. Every profile maps to its exact Catalog original, owner, source revision, confirmed claim and fact head; profile originals have no document chunk. Every document chunk maps to its original body/hash/scope and current generation. Source/claim/profile/index rows agree across writer/final exports, and the complete writer capture file containing all eight cases matches the new reader's capture file byte-for-byte. Actual selected profile/document text, compact bundle digest, raw search, logical alias and model-visible prompt correlate. This proves the observed accepted-profile/document contour; it does not prove automatic extraction.

Both variants made 24 actual embedding requests: eight document batches and 16 queries, producing 32 finite 768-dimensional vectors. Request/response hashes, pinned embedding profile/revision, unique request IDs and reservations match. No reranker request occurred.

Actual Docker receipts and final inspection match the pinned image, immutable source/binary mounts, UID 10001, 2 CPUs, 1536 MiB, 128 PIDs, read-only root, all capabilities dropped and no-new-privileges. The writer has own-PG/local-inference networks; the reader additionally has backend plus the required proxy/local bypass. Writer/reader exit 0 without OOM. In each root, writer finish precedes PostgreSQL's new start, which precedes the distinct reader process; PostgreSQL stops after reader completion. The databases were already stopped, so this review correlates the sealed SQL exports and did not restart or mutate them.

Closed fixture windows retain baseline **32 admissions / 579,223 reserved units**, expiry 15:11:04.998206Z, and candidate **48 / 1,252,900**, expiry 15:13:50.900909Z, all on 2026-10-15. Each keeps its original four-hour deadline and zero carried prior calls/units. Admissions/reservations include retained failures; allowance units are not actual provider tokens. Each export still has eight active task rows, so successful fixture collection is not backfilled as completed real owner tasks.

## Resources, regression attribution and acceptance limit

All 133 baseline and 293 candidate expenses correlate exactly with the SQL records, including every writer expense and both failed generations. No retry/extraction/summary/consolidation call occurred. Across the whole scheduled matrices, logical stage calls are 214 baseline versus 246 candidate: both have 158 write calls and 24 embeddings; reads are 24 versus 40, generations 8 versus 24. This unequal whole-matrix total is not the matched retrieval comparison. Logical calls are accounting units rather than a count of raw SQL statements or HTTP packets.

For the matched eight retrieval answers, memory-section UTF-8 bytes are 17,259 → 3,791 and provider input/output/total usage is 8,202/801/16,434 → 3,898/862/11,731. These bytes are not provider tokens or complete request sizes. Reported total usage is preserved without inferring a hidden-token breakdown. Local tokenizer totals and failed-native usage remain unknown. Native expense invoice fields remain null; the owner's zero incremental subscription/local price premise is distinct from measured invoices or free host hardware. Physical storage/CPU/energy and some background overhead remain unknown. Full resource tables and retained logical-byte counters are in the [validation report](memory-search-validation-20261015.md) and safe evidence JSON.

Matched retrieval latency p50 improves from 12.560 to 10.127 s, while p95 increases from 18.743 to 26.896 s. This sample does not establish a reliable speed improvement. Answer completeness and actually supplied facts are the primary observed improvement.

The initial operator description of three skipped “quality” suites was wrong. They were Hybrid PostgreSQL foundation checks missing their separate environment variable. The author corrected this and reports final-source 59 passes/3 skips in `memory-search-verify-20261015T110753Z-2337628`, then those three distinct real-PG/race passes with zero skips in `...T110943Z-2344456`: 62 distinct targeted passes, no remaining targeted skip. The preflight records the correction. I did not rerun these checks or count their earlier skips as native-quality evidence.

This is sufficient scoped evidence for the targeted #79/#80 regression correction and a completed, auditable comparison. The local-only/external-release invariant is supported by the separate source/real-PG policy regression evidence; these public cloud-answer scenarios do not themselves prove a private Gemini path. The small author-known synthetic set and shared profile scaffolds cannot establish general production accuracy, isolate every changed component's causal contribution, validate multi-step search, automatic write/extraction quality or all memory layers. No new tuning or retries were used to replace an observed result. Deployment and external task effects remain separate from this acceptance.
