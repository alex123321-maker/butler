# Independent closed temporal V8 review

Verdict: **NO_GO for whole-cohort semantic qualification**. All12 original
target answers pass the unchanged explicit-v1 rubric, including the previously
failed distinction between applicability expiry and verification expiry.
Full-answer accuracy is6/12: t02, t05, t06, t07, t09 and t12 add unsupported
temporal or verification-history statements. Correct targets and successful
citations do not waive those additional assertions. No source, corpus, rubric,
prompt, result or qualification flag was changed by this reviewer.

This review reads only the two hash-verified safe projections from
`/srv/storage/butler/agents/issue88-native-v8-20261018/reports/native-v8-preseal-v1`,
frozen source `9a0fd8c805e36ef36d64fee52d38da542a50f233`, actual source/citation
material and the unchanged local explicit-v1 corpus/controls. Facts contract4
and all12 scheduled originals are retained. The reviewer performs0 native/auth
calls and reads no reasoning, credential material or raw provider errors.
Native92 WIP remains in a separate worktree.

| Original | Target | Full accuracy | Independent assessment |
| --- | --- | --- | --- |
|t01 February|PASS|PASS|Kedr applies historically. March validity expiry is separate from the future verification deadline and requires_revalidation=false. Original quotation was actually opened.|
|t02 current head|PASS|FAIL|Pikhta and current owner decision are correct; omission does not imply rejection. Additional claim that independent verification was not performed is unsupported.|
|t03 earlier knowledge|PASS|PASS|Kedr precedes the real anchor; explicit not_known_at_observation=1 supports absence of the second recorded fact at that anchor. No later knowledge is backdated.|
|t04 before upload|PASS|PASS|Empty response/count2 is scoped to the selected records; no concrete value or claim that the server never existed.|
|t05 open and unknown|PASS|FAIL|Lon's open interval and unknown relative rule are distinguished, with two actual original opens. Additional source-record dates are labelled event dates, which remain unknown.|
|t06 overlap|PASS|FAIL|Both accepted colours remain unresolved; current head is not a historical winner. Old TTL error is fixed. Additional source-record/event-time conflation and negative verification-history assertion remain.|
|t07 exception|PASS|FAIL|Russian preference applies; omitted exception language/date/cause is not guessed. Additional independent-check history is unsupported.|
|t08 technical|PASS|PASS|Last accepted4.2 needs revalidation, is not currently verified, and is not declared false. No external request. Concrete null endpoints remain unprovided while validity is explicitly interval.|
|t09 event time|PASS|FAIL|Actual September13 EventTime is separate from February1 validity; Iva applies. Additional claim that external confirmation was not performed exceeds the recorded basis.|
|t10 correction|PASS|PASS|Actual facts/open return stale; no old value or hidden exclusion cause is disclosed. Absence of a delivered fact is not asserted as proof of historical nonacceptance.|
|t11 revocation|PASS|PASS|Denied/empty response, no revoked value or guessed private reason.|
|t12 completed episode|PASS|FAIL|Opened owner-accepted Olha report is not a new mandate or independent effect proof. The extra claim that effect verification was not performed goes beyond the report saying it was not attached.|

The t05 final twice labels `source_recorded_at` as `Время события/источника`.
Actual source-record values are `2026-10-18T07:59:46.171721Z` and
`2026-10-18T07:59:46.29729Z`. The t06 final twice says
`Время события в источнике (source_recorded_at)`, for
`2026-10-18T07:59:46.394977Z` and `2026-10-18T07:59:46.522294Z`.
All four actual original Catalog rows have `event_time:null`; these displayed
timestamps equal their `extracted_at`/source-record observations. No returned
event date supports relabelling those timestamps as event time. This is an extra
axis error even though t09's explicitly supplied EventTime is handled correctly.

The same unsupported negative must be judged consistently across all answers:

- t02: `независимая внешняя проверка не проводилась`.
- t06: `Независимая внешняя проверка не проводилась`.
- t07: `независимая внешняя проверка не проводилась`.
- t09: `внешнее подтверждение факта не проводилось`.
- t12: `независимая проверка эффекта не приложена и не проводилась`.

`verification_basis:owner_statement` establishes the basis in the returned record;
it does not prove the absence of any independent verification event. The t12
actually opened original specifically says that independent effect verification
is not attached. Missing recorded evidence cannot establish that a check was
never conducted. The corpus's pre-existing instruction not to invent missing
facts/reasons and its12/12 full-semantic gate apply unchanged. An initial
preliminary9/12 count covered only t05/t06/t12; the final6/12 consistently includes
the equivalent negative assertions in t02/t07/t09 as well.

The independent citation/receipt audit passes954 assertions. It adapts the V7
audit only for exact V8 input hashes, facts contract4 and actual measured
population/count identities. It checks every actual Start/question/allowed-source
binding, proposal/result correlation, final/public answer, original open bytes
and offsets, typed temporal provenance, current Catalog heads/access/hash, fact
pins and the full reservation union. There are18 actual calls:12 facts reads and
6 opens, including5 successful original opens and1 stale open. There are10
returned fact items,14 typed temporal annotations,42 actual fact pins and0
truncated pages. Snapshot values are not represented as opened original quotes;
successful quotations match actual primary bytes. Catalog content hashes are
checked as Catalog identities, separately from body-only SHA256 witnesses.

All30 native entries have complete reported totals:216,246 full tokens,
151,270 input and12,842 visible output; their sum164,112 is not the full total.
Measured serialized input is225,456 bytes, native method duration497,329ms and
whole original-case duration509,711ms. These are current-cohort measurements,
not billing, wire-byte, savings or p95 claims. Exactly one labelled145-turn
inclusive prior carry is retained; current owner-day charges include all175
turns. Inclusive native usage remains174 known/1 unknown, with full-token lower
bound1,321,800. The separate adaptive226 quotes/1 unknown, old failed windows and
auth0 setup failure remain referenced. Earlier complete raw history is not
re-audited here; its original independent proofs remain authoritative evidence.
The single operator control-plane loadCodeAssist handoff remains outside native
generation accounting and does not imply zero invoice cost.

| Artifact | SHA256 |
| --- | --- |
|native-review-safe-v8.json|031580a368b12ddd081a54a42ffa2f105613a68c6fee6404c05497584f393cfe|
|source-citation-material-safe.json|3e4cac2562ec1bf708dbf676d59b7cd06027091e8633fa46eb618d5fad5531bc|
|local audit.ps1|335eff27d360f15d98d0308e230ac73ef5f119b65e9843c3f345c939c5274d01|
|local audit.json|2503e144519f5af619a53b6b46ce878288f7b9605b66a059d091fbcc4c92e4b7|
|[independent review JSON](fixtures/issue88/native-v8-independent-mixed91-review.json)|5e3f97425660d6ede00a4698ba5bf29867c631340b52e822adea8d7850dacdda|

The reproducible audit and hash-verified safe inputs remain in the independent
review worktree's `.cache/issue88-v8-review/`. This is a bounded explicit lookup
review, not a natural date parser, adaptive retrieval screen, production enablement
or native92 coherent pilot. A new source/contract experiment cannot rewrite V8.
