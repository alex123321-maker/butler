# OpenAI Luna versus Gemini on the same V3 corpus

All **72/72** scheduled positions were observed for `gpt-6-luna`, High, through
real Core/Broker and the ordinary OpenAI provider factory. The exact suite bytes
match the earlier Gemini V3 comparison. The native source is
`157673530f7666ee51079663dee13ade60a0e310`.

The producer exited **1** because nine finals cite unmapped sources. This is a
complete descriptive comparison, not passing issue115/product acceptance. All
203 API turns have complete measured usage, corroborated by the raw PostgreSQL
journal and Core operation/run/reservation records. There were zero transport
errors, unknown usage, missing Core attempts or token mismatches.

## Frozen primary result

| Strict V3 task success | A: no experience | B: notes | C: same notes + skill |
|---|---:|---:|---:|
| Gemini Flash High | 7/24 | 20/24 | 20/24 |
| OpenAI Luna High | 7/24 | 8/24 | 5/24 |
| Luna minus Gemini | 0 | -12 | -15 |

Gemini totals **47/72** successes; Luna totals **20/72**. Within Luna, notes add
one success over A; adding the skill loses three relative to B, while increasing
tokens by **54.7%** and execution time by **98.8%**. This is one public development
corpus, not a general ranking of models or a proof that memory/skills are useless.

| Luna measurement | A | B | C |
|---|---:|---:|---:|
| Four factual families | 7/20 | 7/20 | 4/20 |
| Four action regression cases | 0/4 | 1/4 | 1/4 |
| Native API methods | 47 | 67 | 89 |
| Full tokens, including reasoning | 203631 | 349418 | 540715 |
| Input / full output tokens | 185143 / 18488 | 332488 / 16930 | 515564 / 25151 |
| Reasoning subset of output | 15165 | 12877 | 19802 |
| Cached input / cache writes | 87508 / 97494 | 208818 / 123469 | 366680 / 148617 |
| Execution time, sum, ms | 318283 | 330188 | 656510 |
| Preparation time, sum, ms | 55406 | 41384 | 80802 |
| Actual memory.search / memory.open | 0 / 0 | 19 / 1 | 19 / 0 |
| Actual skills.load | 0 | 0 | 21 |
| Limited memory reads | 0 | 0 | 0 |

Luna used **1093764 full tokens**, versus Gemini's **2466920**. Execution sums
were 21.75 versus 65.39 minutes. Including measured preparation gives 24.71
versus 68.00 minutes; this excludes unmeasured authoring and some observer/setup
overhead, and is not a normalized cost per equally completed task. Early
abstentions and missing evidence account for part of Luna's smaller workload.

## Accounting

At the published OpenAI Standard short-context tariff, Luna's known usage costs
**USD 0.083172960–0.12937046**:

| Standard tariff estimate | A | B | C |
|---|---:|---:|---:|
| USD, minimum | 0.02231993 | 0.026006905 | 0.034846125 |
| USD, maximum | 0.03450668 | 0.04144053 | 0.05342325 |

Every response reports service tier `default`. Cache-write totals are measured,
but their 30-minute/12-hour split is absent, so the two published lifetime rates
bound the estimate. Reasoning is already included in output; cache reads/writes
are input subsets. None are added again to total tokens. This is a tariff
estimate, **not an invoice** or account-specific adjustment. Gemini's subscription
allocation cannot be converted to an invented API price.

Historical receipts, interrupted spending and unknowns are pinned unchanged in
[launch.json](launch.json) and [comparison.json](comparison.json). Overlapping
historical allocations are not added together. Synthetic note/skill preparation
made zero native creation calls; manual authoring and autonomous learning costs
remain unmeasured.

## What failed

The real transport and Broker path worked: Luna loaded the reviewed skill 21
times and completed all 72 native finals. Its main observed retrieval problem is
query construction: **37 of 38** searches returned `empty`; only one source was
subsequently opened. Queries often combine many terms or Russian and English
paraphrases. The current `catalog_exact` path uses PostgreSQL
`plainto_tsquery('simple', query)`, requiring matching terms without semantic
expansion; its model-facing description does not explain that behavior explicitly.
This is an interaction between this model and the current retrieval interface.
No query policy or prompt was tuned after observing the results.

Factual answer diagnostics also remain worse with experience: exact outcome/value
matches are Luna **10/20, 11/20, 10/20**, versus Gemini **11/20, 20/20, 20/20**.
These are posthoc diagnostics, not replacement scores or proof of eligible reads.
Luna additionally misreports the factual tasks' `check` field in 9, 10 and 15 cases
in A/B/C respectively; 2, 4 and 6 of those cases have the correct value. The factual `effect`
field has no mismatches. Nine final source mappings fail the original observer.
See [provider-comparison.json](provider-comparison.json) and
[classification.json](classification.json).

On the four action cases, B and C correctly preserve the actual unknown effect,
where the earlier Gemini comparison had no passing action observations. All
three Luna arms also **actually prepare and verify an unapplied artifact**, with
the original files unchanged and the final artifact reference matching the real
receipt. They still fail the fixed answer-value contract: the expected value is
`ready`, while C returns the artifact identifier, A returns `true`, and B returns
`ready=true`. Their physical result and frozen strict score are both retained;
no success was retroactively awarded. See [action-diagnostics.json](action-diagnostics.json)
and [action-field-diagnostics.json](action-field-diagnostics.json).

## Implementation, validation and cleanup

The full provider uses Responses WebSocket/SSE, canonical Broker tool aliases,
string tool outputs, complete-turn usage, bounded current-source admission and
local cancellation. No replay follows an ambiguous dispatch. New configuration
and factory wiring cover model, reasoning and input/output ceilings. It shares
no OAuth state with Gemini or `openai-codex`; no new dependency was added.

[gate.json](gate.json) records all 72 controlled positions and the separate real
Broker skill-approval test passing on the native source, 22 actual PG usage checks,
325 transport tests, eight separately gated external skips, and source/binary
proof. All failed control roots remain retained, including the 750ms authority
observation failure in r5; it was not reclassified. The sequential r6 passed with
unchanged deadlines.

After the native window, a separate guard at `c5009153` rejects unsupported
per-request storage overrides before dispatch instead of silently dropping a
requested privacy choice. Its offline race/vet checks made zero new model calls;
all 92 tests passed without required skips. See [final-check.json](final-check.json).
It does not alter this window's source, results or expense.

The key traveled through private SSH stdin only. [cleanup.json](cleanup.json)
confirms the runner/network were removed, own PostgreSQL stopped cleanly, and
private database data was retained. No API key was persisted on laba. Production
deployment and issue115 closure are separate and were not performed.

See the [study report](../../../../docs/testing/issue115-openai-luna-v3-2026-10-30.md),
[OpenAI provider](../../../../internal/transport/openai/README.md),
[official pricing](https://developers.openai.com/api/docs/pricing), and
[cache accounting](https://developers.openai.com/api/docs/guides/prompt-caching).
