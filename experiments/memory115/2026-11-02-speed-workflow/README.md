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

Results are in progress. Changing optimizations, output ceiling and effort
together prevents causal attribution to any one of them. The new profiles share
the same optimized composition and bounds; this public development corpus is
not a statistical quality-equivalence or autonomous-learning acceptance test.
