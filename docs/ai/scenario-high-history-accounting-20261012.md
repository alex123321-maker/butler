# Closed High history: 248 spent attempts

`readScenarioHighHistory` is a separate, pure historical reader. It is not wired
into either old admission path and grants no future window, retry, refund or
execution. The old 45-call and 228-call readers remain unchanged.

The append-only reader checks exact regular-file bytes of the independently
audited V2 content-free report and independent PG correlation. It binds original
private report `383917dd…`, SQL witness `e732ad5b…`, audit `f00347b7…`, correlation
`9c374c2c…`, prepared code `40984705…`, source `7d08028f…`, hardware `d3632dff…`
and the fixed V2 schema/policy/resources/corpus. Both original collector failure
and NO_GO remain required. It does not read private outputs or rescore goldens.

All 20 charged ordinals 229–248 survive: 19 completed requests with known
input/output/total measurements and one timeout with NULL usage/version. Known
new subtotals are 7141 input,244 output,31083 total; complete cost is unavailable.
The cumulative count is248,222 known input/output pairs and26 NULL pairs, with
known input/output subtotals144546/5537. Total-token history has46 unknown
attempts: the first45 plus the final timeout. The known total subtotal249727 is
not complete cost. The final timeout proves the attempt deadline; quota,
capacity and upstream finish reason remain unknown.

Artifact17 is the exact V2 safe audit. Its baseline fold is SHA256 of the prior
228-call hash followed by original V2 report, safe audit, SQL witness and
correlation hashes, each separated and terminated by LF. No old artifact or
usage entry is replaced. Unknown proprietary weights, server effort, RAM and
invoice are unchanged. An empty comparison contains no measurements and cannot
establish comparison acceptance. Failed V1/V2 quality stays NO_GO forever.

Pure regression coverage verifies all accounting, NULL preservation, both prior
readers, missing/changed artifacts, wrong status/version/cost/ordinal/policy/source
and false PG/quality claims. Tests run on isolated laba Docker with race/vet,
network disabled and zero native calls.
