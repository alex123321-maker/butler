# Post-fix native workflow repeat

Native source: `d1f21ac9ad9d8ba9477616ee9f2e5c19dfff9c72`.
Product skill-stream fix: `e1fa749488835112fa0df4477ce534733db16a6e`.
Profiles: Gemini `gemini-3.8-flash-high` (existing High), OpenAI
`gpt-6.1-terra` **medium**, OpenAI `gpt-6-luna` **max**.

The exact earlier `skill-workflow-v1` suite is reused, byte SHA256
`8df9b914108c690c5a083648aa28cbb9bbad2b38abb205dbceb797c0e80f350a`:
12 cases × A/B/C = 36 slots per profile, 108 native slots in total. Order,
current documentation, notes/skill preparation, instructions and limits are
unchanged: 65536 input/8192 output, 90s per method, 13 methods per case,
420s authority/480s case, 4h per profile. No retries, model fallback or prompt
tuning inside a window. Failures and unmeasured usage stay in the denominator.

Each window is in a fresh
`/srv/gitea/butler-acceptance/issue115-utility-native-workflow-LABEL-fixed-20261102-r1`
root. They run serially, preserving the 1 CPU/3GiB race-instrumented app and
separate private PG. A journal namespace derived from the frozen window plan
allows the same suite to repeat without touching the old Gemini journal in the
private auth DB. OpenAI secrets use the previously authorized private stdin
path, never persisted; Gemini reuses the existing protected research auth.

The [control gate](gate/repair-summary.json) passed all 72 old and 36 workflow
scripted slots, actual Broker reviewed-skill approval/load, race checks and vet,
without required skips. All 6880 source Git blobs were verified before native
launch. The first setup root failed on cache permissions before inference;
its replacement control source was superseded by the one-use journal change.
Those roots are retained; both had zero native requests.

Evaluation keeps original V3 scores, then separately replays the same
`utility-trace-paged-reads-v1` policy used for the old results. The observer
requires exact legacy reproduction and unchanged original manifest hashes.
No model calls are needed for reporting or corrected projection.

The repeat compares each model/arm with its own old closed workflow window.
Terra changed **none → medium**, so its differences do not isolate the stream
fix. Gemini and Luna retain their requested profiles, but model randomness,
provider state and cache reuse remain uncontrolled. This is one public
development corpus, not independent holdout or production learning acceptance.

Times retain execution, weighted mean provider wait, system/tool subsets,
preparation/observer phases and full native wall windows. Provider wait includes
network and adapter work; hidden reasoning duration is unavailable. System time
includes race instrumentation and research journaling. The capture wrapper still
buffers provider events, identically to the earlier comparison.

OpenAI estimates use the [Standard price table](https://developers.openai.com/api/docs/pricing)
checked 2026-11-02: Terra input/output/cached input USD 1/5/0.05 per million;
Luna 0.10/0.50/0.01, with 30m/12h write pricing bounds when retention is unknown.
These are known-usage estimates, not invoices. Gemini subscription allocation
and scripted/manual authorship cost remain unknown. Closed historical receipts
are pinned and rehashed, never added again across overlapping allocations.

Results and per-call/per-task exports are written only after the windows close.
The final comparison lives in `repeat/`, alongside the complete new A/B/C
tables in `comparison/` and all profile receipts. No merge or production deploy
is implied by this research run.

## Completed comparison

All three windows closed with 36/36 observed/timed slots and matching Core/raw/
PostgreSQL method receipts. There were 451 dispatched methods (189/126/136) and
3,776,761 known full tokens; unknown usage is zero. Every historical receipt was
rehashed. Owned runners/networks were removed, private PG data retained and
databases stopped. OpenAI secret-copy counts remain zero; owned Gemini auth-source
use was stopped after the window. The one-use window journal preserved old rows.

The [old/new comparison](repeat/README.md) and [108 paired task rows](repeat/paired-cases.csv)
use the same paged-read policy for both cohorts. All 108 original new scores/errors
reproduced exactly before that additive projection; it changed no new score.
Corrected old/new total quality is **92/108 → 92/108**:
Gemini 27→27, Terra 36→35, Luna 29→30. Arm scores are:

- Gemini: 8/9/10 → 9/10/8.
- Terra: 12/12/12 → 12/11/12 (none → medium).
- Luna: 9/10/10 → 8/11/11.

The stream fix removed the intended C-arm overhead in actual native traces:
Terra C has **50→50 model methods** and system time **151.28→23.74s**;
Luna C has 55→54 methods and **140.89→27.13s**. Their A/B system times remain
roughly 13–17s per arm. Gemini C changes much less (39.36→36.40s), consistent
with the earlier [fragment-count investigation](../2026-11-02-skill-latency/README.md).
This supports the measured runtime improvement, not a claim that fewer model
calls or improved reasoning caused the overhead reduction.

API consumption/cost changed little: new known-usage tariff bounds are Terra
USD 0.386493–0.663447 and Luna 0.085506–0.115672, versus old 0.378265–0.655474
and 0.087924–0.118624. Gemini used 2,088,198 tokens versus 2,074,086, with
subscription allocation still unknown. A speed improvement in unbilled internal
RPCs is not automatically an API-cost improvement. Scripted authorship and
invoices remain outside those estimates.

## Failed-case interpretation

Terra slot20/B and Luna slot19/A stopped at an unknown preparation effect and
retained no claimed success. Offline inspection found their submitted `before`
strings did not equal the actually inspected README, although the submitted
project revision matched. These are retained argument/precondition failures,
not grounds for replaying a possibly dispatched mutation. Physical checked
artifacts: Gemini 8/8/8; Terra 8/7/8; Luna 7/8/8. A correct after-text alone does
not qualify a current, prepared and checked artifact.

Luna retained three `provider_protocol_error` responses. Each reports output
8192 and reasoning-output 8192: the entire declared output ceiling was consumed
by reasoning. Their costs remain included. The old max window also had three
such responses; no output bound was raised mid-run and no paid retry followed.
[Provider diagnostics](provider-diagnostics.json) retain those counts, actual
observed model-version strings and service tiers. Observed version labels match
the earlier labels; that is not proof of immutable provider weights/cache state.

There is still no stable quality benefit of C over B across profiles. The current
documentation already supplies all migration rules, and a public 12-case cohort
does not establish a general memory/learning result. The confirmed change is
the reduction in stream-check overhead, with unchanged action/final/source
security fences and all old results/expenses preserved.
