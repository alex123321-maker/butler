# Prospective skill workflow comparison

This separate `skill-workflow-v1` cohort preserves every earlier V3 result and
expense. Twelve cases run in balanced A/B/C order: no experience; reviewed past
notes; identical notes plus an applicable reviewed skill. Eight instances migrate
an embedded deployment configuration in README.md using actual Core, Broker,
PostgreSQL and the bounded project runtime. Four controls cover inspect-only,
unknown effect, typed rejection and a current answer that needs no skill.

Current documentation, tools, permissions and result format are equal across
arms. The common prompt does not provide the solving sequence. The skill has
inspect, prepare and verify steps, explicit inputs, applicability, current-value
rules, exact-content preservation and stop conditions. Actual separate training
artifacts establish its existing Broker lifecycle evidence; source instances and
final artifact identities are not embedded in the procedure. This is a public
development cohort with scripted notes/skill preparation, not an independent
holdout or autonomous learning claim. All cases are frozen before native results.

Requested native profiles are Gemini Flash High, GPT-6.1 Terra `none` and GPT-6
Luna `max`. Each uses 65536 input/8192 output ceilings, 90-second API methods,
13 methods per case, 420-second authority and a 480-second case deadline. Failures
remain in the denominator and expense journal; no retry or model fallback.

Per-case timing records monotonic execution duration from owner ingress until
the run returns, provider-call boundary duration less measured Butler admission
callbacks, and actual tool RPC duration. System elapsed time is execution minus
provider wait; tool RPC is its subset, never added again. Provider wait includes
network, adapter work and remote processing. Pure provider reasoning time and
remote queue time are unavailable. Fixture/skill preparation and observer/export
work are measured separately. Full phase totals include them and explicitly
exclude later reporting and environment teardown. Failed calls retain timing;
unknown measurements are not represented as zero. Aggregate mean API latency is
total provider wait divided by the actual number of calls, not a mean of means.

These are elapsed measurements of the isolated research composition. The native
app binary has Go's race instrumentation enabled, and execution includes durable
research journal/raw-file recording. That recording overhead is not separately
instrumented. Consequently `butler_total_seconds` includes Butler plus in-run
measurement overhead; it is not a production CPU/latency benchmark. Preparation,
observer/export and later teardown have the separate boundaries described above.

Strict answer/evidence scores remain V3. Physical prepared-artifact correctness
is reported separately from final field formatting, with current exact contents,
receipt/check correlation and unchanged originals. No old score is regraded.

Strict V3 projection also rejects a memory read with a `denied` result, even
when the artifact was already prepared correctly and the denied read supplied
no data. Therefore strict failures cannot all be described as incorrect patches.
The supplemental physical metric checks an unchanged recipient, exactly one
completed preparation, and a completed check for the same artifact, revision
and exact prepared content with `checks_passed=true` and `applied=false`.
It does not substitute for final-answer or authorization/evidence conformance.
Reports retain final states, finish reasons, projection failures, completed skill
loads, and successful/empty/limited/denied memory outcomes separately.

The frozen native source is `66be595bc122ea5a6e02def4689b92b70945fc3a`.
The successful control root is
`/srv/gitea/butler-acceptance/issue115-utility-trace-workflow-20261030-r3`:
72 existing and 36 new controlled slots, real Broker skill approval/load,
zero required test failures/skips and zero native model requests. Its source
manifest verifies 6814 Git blobs; only Git-declared Windows script line endings
are normalized. Earlier control roots remain preserved: r1 exposed a timing
wrapper that hid authorized Broker dispatch; r2 exposed PostgreSQL bootstrap
socket readiness before TCP readiness. Both were fixed before native inference.

The profiles are deliberately the user's requested combinations, not equal
reasoning settings. A single execution per case does not establish statistical
significance or distinguish model capability from inference effort or provider
latency. Native results must not be directly ranked against the older 24-case
corpus, which contains different tasks. Within this cohort, B minus A is the
notes effect and C minus B is the incremental reviewed-skill effect.

API tariff estimates use the [OpenAI Standard price table](https://developers.openai.com/api/docs/pricing#text-tokens),
checked on 2026-10-30. Per million tokens: Terra input/output/cached input are
USD 1.00/5.00/0.05; Luna 0.10/0.50/0.01. Unknown cache-write retention keeps
a bounded estimate instead of an invented exact charge. Account invoices and
Gemini subscription allocation remain unknown. All failed calls stay in usage
and elapsed totals. The [Terra profile](https://developers.openai.com/api/docs/models/gpt-6.1-terra)
supports `none`, and the [Luna profile](https://developers.openai.com/api/docs/models/gpt-6-luna)
supports `max`; the selected values are also pinned in each native plan.

Postprocessing exports `workflow-report.json` and its `.calls.csv` companion
from real traces without further model requests. `skill-workflow-comparison.py`
requires matching source, suite bytes and case order across all three profiles,
then emits all 108 task rows with timing, usage and separate quality measures.
Native Go test elapsed time supplies an additional full-window measurement;
the difference from measured phases includes teardown and uninstrumented gaps,
not an inferred breakdown of either.

During the Gemini run, a legitimate 637-character note was read in pages
0..500 and 500..637. Legacy projection rejected the first truncated page before
considering complete coverage. The additive `utility-trace-paged-reads-v1`
projection fixes this observer defect; it does not alter native instructions,
execution, expenses or stored V3 results. It verifies every page's current pin,
owning Broker digest, exact Unicode text/range, truncation and complete coverage.
Missing/changed/foreign/redacted/conflicting pages fail closed. All profiles are
replayed offline under the same rule only after their original scores and errors
reproduce exactly. The comparison exports both versions. This correction was
identified from Gemini before the Terra and Luna results were available.

## Completed results

All three frozen windows observed all 36 slots, correlated every native method
with Core and the complete PostgreSQL journal, and retained zero unknown-usage
calls. Gemini dispatched 190 methods, Terra 128 and Luna 137. Native task failures
remain failures: Gemini scored 26/36 under legacy projection, Terra 36/36, Luna
29/36. Luna retained three `provider_protocol_error` responses with measured usage.
The additive paged projection changes only Gemini B, yielding 27/36 total;
Terra and Luna remain unchanged. Legacy scores/errors reproduced exactly for
all 108 snapshots before the new policy was applied.

The [complete comparison and per-task CSV](../../experiments/memory115/2026-10-30/skill-workflow-v1/comparison/README.md)
record all task durations, average provider call, provider wait, system/tool
subsets, preparation/observer phases, full native windows and tariff bounds.
Each profile also has a `.calls.csv` export with every individual method,
including failures. The [offline verification](../../experiments/memory115/2026-10-30/skill-workflow-v1/paged/verification.json)
pins the evaluator binary, overlay and successful race-test/vet build log.

Correct prepared/check artifacts were Gemini 8/8 in each arm, Terra 8/8 in each
arm, and Luna A 7/8, B 8/8, C 8/8. All profiles loaded the actual reviewed skill
in all eight applicable C cases and in no unrelated control. C minus B under the
paged policy is Gemini +1 strict task, Terra 0 and Luna 0; artifact correctness
does not improve between B and C for any profile. This cohort therefore does
not demonstrate an artifact-quality benefit from adding the skill to notes.
Current task documentation already contains the complete migration rules.
Terra has a ceiling result even in A; stronger claims require different real
tasks or an independent holdout, not further tuning against these results.

System elapsed in C is notably larger for the OpenAI profiles: Terra 151.28s
and Luna 140.89s, compared with roughly 14–15s in their A/B arms. Measured tool
RPC accounts for only a small subset. The remaining cost is not separately
attributed to a product component versus research instrumentation; no claim of
an identified root cause or production latency fix is made.

All owned runners/networks were removed, private PostgreSQL data retained and
owned databases stopped without OOM. Closed historical receipts were rehashed.
The Gemini auth source was stopped after its owned use; OpenAI keys were supplied
through private stdin with zero persistent key copies. Native source/limits and
all previous results/expenses remain preserved. No production deployment follows.
