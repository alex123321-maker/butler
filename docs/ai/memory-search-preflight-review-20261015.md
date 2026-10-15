# Memory search #79/#80: independent preflight, 2026-10-15

Verdict: **PASS_CORPUS_PREFLIGHT** for the fixed eight synthetic cases and the declared baseline/candidate comparison. The narrow static production review found no concrete privacy or correctness blocker in the inspected changes. This is pre-inference review, not a model-quality result, runtime receipt or production activation approval.

Reviewer: independent model agent `/root/memory_independent_review`; corpus/candidate author: `/root`. I changed neither corpus nor implementation and made no authentication, embedding or native-model calls. Relevant memory, owned-source, run-lifecycle and engineering rules were inspected.

Corpus: [ru-memory-search-v1.json](../../testdata/acceptance/ru-memory-search-v1.json), raw SHA-256 **`9c78b9469bb6e61c143020324fe7af8b75e9344e6dd0a2154775f370ddeb964f`**. The earlier pre-observation proposal was `bce838860d96a177dbc0d8e7686013ce802b8c7d87e0e6b66a271c8a01bb347f`. I compared them: only the 22 g05 background-profile bodies changed, reducing the same repeated multilingual filler from twelve copies to six to fit the shared input bound. Questions, required facts, forbidden claims, source IDs, dates, oracle selections and profile-fixture mappings are unchanged. No answer observations informed that change.

Use the existing [semantic methodology](memory-eval-semantic-methodology.md). Required facts describe meaning, not literal answer strings. Ordinary paraphrases, address/date/number formatting and concise answers are acceptable when they preserve the requested fact, scope, negation and uncertainty.

| Case | Independently checked sufficient answer/evidence | Interpretation fixed before answers |
| --- | --- | --- |
| g01 | g01s1/s2/s3: Строителей 24, OL-3746, 10:15–18:45 | All three requested fields; no transfer from Ольха-593. |
| g02 | g02s1: room 418, fourth floor | Administrative-building detail is optional; 25 board-game profiles are irrelevant. |
| g03 | g03p26: train preference for Иней-64 | “Поезд” answers the already scoped question; repeating the project name is not mandatory. Do not generalize the preference to every project. |
| g04 | Current task revision 7: short remarks/checklist only; sending paused; no new full report | Judge the proposed next action and respect for the current restriction. A concise checklist-only answer need not recite every prohibition verbatim. Historical usual practice cannot authorize sending. |
| g05 | g05s1: 16 October, 13:40, Озеро | All requested date/time/room fields; unrelated multilingual profiles add no required fact. |
| g06 | g06s2/s3/s4: Южный, Д-12, after 16:30 | Preserve the time bound and current warehouse; g06s1 explicitly expires before the as-of time. |
| g07 | g07s2: green approved for Берег-120; blue not approved | “Зелёный” or “только зелёный” can answer completely without a separate literal blue-negation sentence. Neither Берег-12 nor the conditional general preference overrides the exception. |
| g08 | g08s1: neither day nor time confirmed for Туман-57 | Justified uncertainty is complete. Do not import Wednesday 15:00 from Туман-58 or turn a discussed option into confirmation. |

Every oracle selection is sufficient together with the common current task; all supporting originals are active/public revision 1 and precede the declared as-of time. Genuine missing information is confined to g08. Its safe no-memory refusal can be correct; in other answerable cases a grounded refusal remains incomplete. g04 may also be answered from the current task alone, so its baseline success is not evidence of memory recovery. Quotes/citations must be checked against the actual supplied body and revision, not accepted merely because a source ID exists.

The declared profile fixtures comprise 75 owner-accepted entries: g02=25, g03=26, g04=1, g05=22, g07=1. I checked each mapping against its original history entry, including the repeated background patterns. The collector overlay uses actual Catalog Propose/Decide, omits these originals from document parsing/indexing, and reads the accepted Profile store in both processes. This tests accepted-profile retrieval alongside documents; it does not test automatic profile extraction or owner acceptance by a model. Post-restart persistence and actual selected/visible facts still require final evidence correlation.

## Declared comparison and source scope

Baseline product is `b4a2fd3f26ee7f479141a8d91ad78d86a965b798` plus the same test-only collector overlay: eight retrieval answers. Candidate schedule is no_ltm/oracle/retrieval × eight: 24 answers. There are 32 planned native answers total, the same questions/originals/current tasks/model/instructions and five-minute per-answer timeout, no retries or old-root reopening. Each variant's source tree, binary, corpus and actual visible context must remain separately attributable; differing retrieval payloads are the measured treatment. Retain all failures and include them in their original denominators.

This is a targeted public regression sample with author-known cases and repeated profile scaffolds, not an untouched blind control set or a population estimate. A joint #79/#80 comparison cannot isolate the causal contribution of each changed component. The ordinary native comparison does not itself establish the private/local-only query-release invariant; the targeted real-PG policy tests address that separate property. The author reported 58 targeted/race passes and three explicit opt-in quality-suite skips; I inspected the relevant test assertions but did not independently rerun or convert those skips into passes.

The production review covered query-policy propagation and task-authority recheck, pinned local-backend proof, existing live admission/source guards, profile effective-state filtering, relevance/selection, default chunk limit four, and itemwise final-context trimming with continuation reserve. In the inspected local-only path, legacy external embedding is unavailable; unproven local RPC is skipped with fixed diagnostics while eligible local SQL retrieval remains possible. Existing live admission still binds current ownership/mandate/budgets, and changed source/task authority is rejected. Mandatory history/current state is not silently shortened to satisfy the allowance.

Selection remains an explicit lexical heuristic with a finite four-chunk result cap, item cap and conservative UTF-8 JSON allowance. It can miss synonyms or facts beyond that bounded context; those are limitations to measure rather than promised semantic completeness. Allowance units are not provider-token or invoice measurements. Profile relevance does not create instructions or execution authority.

Inspected production byte pins (working diff over the baseline):

| File | SHA-256 |
| --- | --- |
| apps/orchestrator/internal/app/memory_adapters.go | `0b984cfcd576c5bf6109452a0905caf90230e02bbbd86d9087a97adbce3cdd4b` |
| apps/orchestrator/internal/orchestrator/service.go | `c7ea36a18b5f4fce3431820794dfc0d6cec8c008701c8a5f765a56be8ec50122` |
| apps/orchestrator/internal/orchestrator/service_prepare.go | `f067ad136421e902d9795ca172f66c40b1c36bdeb59048c27d1e47c3beedcb92` |
| internal/memory/semantic/client.go | `5db6cb19b8466b00d390c058978a18e28e658e89ccc286464d04a5fa10072363` |
| internal/memory/semantic/hybrid.go | `0a843f083897c1ef1d68ca47f41ed58462e85d1f478425dc88d1c57cbe325a6a` |
| internal/memory/service/service.go | `5b5415304ee34cdbc03ce181cb67bd3da2c65fea62215213cc57e09524a0dcba` |
| internal/memory/service/selection.go | `7688fbaa66785273778521f9150cea437976ca45205f9faa79b5f8d1fbca988d` |

Before merging, corresponding memory/run-lifecycle documentation and the service README need to describe separate local/external query policy, the new relevance/round-robin selection, byte allowance and itemwise trimming/reserve; the prior profile-first item ordering is obsolete for this candidate. This documentation alignment is separate from the corpus judgment. Final acceptance needs retained originals and independent per-answer review, with missing retrieval distinguished from unsupported generation, stopped actions from model wording, and known costs from physical/invoice unknowns.
