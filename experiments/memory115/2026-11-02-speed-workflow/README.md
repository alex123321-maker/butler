# Workflow latency and effort comparison

Native source: `dcee8f83391dbe3029bb0502d4f6b72283573bc1`. This is a new
prospective window; all previous results, failed calls and expense allocations
remain unchanged. See the [protocol](../../../docs/testing/issue115-speed-workflow-2026-11-02.md).

The exact closed workflow corpus is byte-pinned to SHA256
`8df9b914108c690c5a083648aa28cbb9bbad2b38abb205dbceb797c0e80f350a`.
Each profile runs 12 cases in balanced A/B/C order (36 tasks): A has no past
experience, B has notes, C has those notes and a reviewed migration skill.
Profiles are Gemini explicit MEDIUM/LOW and Luna max/high/low. All new profiles
use 65536 input/32768 output, 90s methods, 13 methods/task and unchanged
420s authority/480s task bounds. Runs are serial, isolated from working Butler.

The new composition performs exact collector/task-selected skill loads through
real Core action authorization and Broker before first inference, omits only
the duplicate tool summary when complete native schemas exist, and forwards
provisional plain text while retaining action/final events until complete
PostgreSQL/raw fsync receipts. Product startup has no automatic skill resolver;
this is a deterministic fixture selection, not a general semantic classifier.
The current documents already contain the migration recipe, so these tasks do
not establish that persistent memory is necessary for ordinary work.

The [control gate](gate/repair-summary.json) verifies 72 existing, 36 original
workflow and 36 optimized workflow tasks, eight exact applicable initial Broker
loads, no unrelated load, real approval/restart, race/vet and 6919 source blobs.
The independently generated controlled fixtures have different random IDs;
their scenario/permission shape and procedural DATA agree. Native freezing
separately requires exact old suite bytes and order for every profile.

Before inference, the artifact verifier was corrected to recognize only
Git-declared CRLF exports and compare the correct independent-control shape
instead of random identifier equality. No product/binary change or model call
was involved in these two rejected verification attempts. The successful
verifier's SHA256 and both controlled suite hashes are in the gate receipt;
its exact [source](gate/verifier.py) is retained separately from the native tree.

Native roots are
`/srv/gitea/butler-acceptance/issue115-utility-native-workflow-{profile}-speed-20261102-r1`.
Each freezes old receipt hashes, exact profile and a one-use journal. OpenAI
uses the existing authorized key over private SSH stdin, with no persistent
key copy or container environment secret. Gemini uses the existing protected
research authentication. No retry, fallback, old-run resume or production
deployment is part of this window.

Reporting retains strict answer/evidence quality and physical checked artifact
correctness separately, every actual call/expense, execution time, weighted mean
provider latency, residual system elapsed, tool subset, setup/observer and the
nullable first text at Core's delivery sink. Provider wait includes network and
adapter work; pure reasoning duration is unknown. Concurrent text validation
can overlap provider time, so subtraction is not a system CPU measurement.
The local sink does not measure Web/Telegram rendering or delivery ACK.

Known OpenAI usage estimates use [Standard rates](https://developers.openai.com/api/docs/pricing),
checked 2026-11-02: Luna input/output/cached USD 0.10/0.50/0.01 per million,
30m/12h cache-write rates 0.125/0.25. Unknown retention keeps a price range.
Gemini subscription allocation, invoices and authoring labor remain unknown.
Overlapping old receipt allocations must never be added twice.

## Completed results

All five windows closed with 36/36 observed/timed tasks, exact native/raw/Core/
PostgreSQL receipts and legacy score reproduction: 180 tasks, 675 dispatched
model methods. All old receipt hashes were reverified. Owned helpers and
networks were removed; private PostgreSQL data and all failures remain retained,
databases stopped, zero native race warnings and zero persistent API-key copies.
No working Butler deployment or model-default switch was performed.

The [complete comparison](comparison/README.md) and [180 paired rows](comparison/paired-cases.csv)
retain quality, per-task/per-call timing, system/tool subsets and known-usage
prices. Primary A/B/C scores and execution totals are:

| Profile | Success A/B/C | All 36 tasks, minutes | Mean provider call, seconds | System elapsed, seconds |
|---|---:|---:|---:|---:|
| Gemini MEDIUM | 9/11/11 | 26.12 | 10.10 | 62.53 |
| Gemini LOW | 9/10/10 | 8.90 | 4.01 | 44.22 |
| Luna max | 9/10/9 | 23.73 | 10.06 | 55.09 |
| Luna high | 10/11/11 | 10.59 | 4.22 | 53.15 |
| Luna low | 8/12/9 | 7.09 | 2.82 | 58.39 |

Previous post-fix Gemini High was 27/36 in 44.33 minutes; previous Luna max
was 30/36 in 19.72 minutes. Comparisons change several variables, not one.
These are aggregate execution times, not time for a single question. Full native
wall windows were 27.91/10.26/25.20/11.94/8.88 minutes respectively; setup,
observation and task teardown explain the different boundaries.

All 40 applicable C tasks received exact reviewed procedural DATA after a real
Broker load and before first inference. No control received an initial load.
Luna nevertheless performed additional model-selected loads: max 3, high 5,
low 7. Preloading therefore removes the mandatory selection turn but does not
guarantee fewer total model turns. The current cohort shows no stable C-over-B
quality gain; both already have complete current migration documentation.

Physical checked artifacts were 24/24, 24/24, 23/24, 24/24 and 22/24. Strict
migration results were 24/24, 24/24, 23/24, 24/24 and 21/24. The difference in
Luna low includes a correct artifact with invalid final evidence. Existing
uncertainty guards stopped bad preparations rather than retrying or applying
anything. [Failure diagnostics](failure-diagnostics.json) separate bad citations,
wrong answers, incomplete/blocked runs, submitted before mismatches, native
transport errors, observed model versions and actual service tiers. All measured
OpenAI responses report the default serving tier. Empty service-tier maps in
the earlier per-profile diagnostic projection are superseded by this additive
projection of the correct TransportEvent field; original artifacts stay intact.

Luna max had two 90s timeouts and one connection failure; its native captures
retain three UNKNOWN usage observations. A separate [read-only reconciliation](reconciliation/retrieval-accounting.json)
used three GET requests against already-created response IDs, with zero new
generations. One returned a complete receipt: 7792 input + 10384 output = 18176
tokens, including 10356 reasoning-output tokens. The 8192 output bound would
have been insufficient for that completion, but a later receipt cannot repair
the failed original task or its delivery. The other two GETs returned 404,
which does not prove zero cost. Original UNKNOWN entries and scores are unchanged.
The [reader and receipt hashes](reconciliation/manifest.json) preserve attribution.

Original known native tokens: 4,632,294. Additive recovered tokens: 18,176.
Combined known tokens: 4,650,470, plus two unmeasured calls. Luna max's known
Standard tariff component is USD 0.085204–0.115938 plus those unknowns; high
USD 0.052315–0.080850; low USD 0.040999–0.068693. Gemini subscription allocation
and total invoices remain unknown. This is not a claim of zero expense for any
failed/unavailable response. Reconciliation never double-counts a known method.

First provisional text was measured at Core's sink for 35/36, 36/36, 33/36,
36/36 and 34/36 tasks. It arrives before full method retention; actions/finals
still wait for receipts and live guards. The fixed JSON-only, multi-tool fixture
usually emits text near the final step; these measurements do not establish
ordinary chat time-to-first-token, channel rendering or delivery ACK latency.

Changing optimizations, output ceiling and effort together prevents causal
attribution to any one of them. The new profiles share the same optimized
composition and bounds; this public development corpus is not a statistical
quality-equivalence or autonomous-learning acceptance test. In particular,
raising output while retaining the 90s method timeout does not guarantee max
finishes, and low cannot be globally promoted without a quality tradeoff.
