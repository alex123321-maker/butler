# Terra 6.1 versus Luna and Gemini on the identical V3 corpus

All **72/72** positions completed for `gpt-6.1-terra`, High, Standard tier,
through the existing OpenAI Responses provider, actual Core/PostgreSQL and Tool
Broker on laba. The suite bytes, instructions, scorer and nonmonetary limits
match the closed Luna/Gemini V3 experiments. The native source is
`e4835ce1cfced57454f064b67992ef25f5effcc9`.

## Frozen primary result

| Strict V3 task success | A: no experience | B: notes | C: same notes + skill | Total |
|---|---:|---:|---:|---:|
| Gemini Flash High | 7/24 | 20/24 | 20/24 | 47/72 |
| OpenAI Luna High | 7/24 | 8/24 | 5/24 | 20/72 |
| OpenAI Terra 6.1 High | 16/24 | 22/24 | 21/24 | 59/72 |

Terra's notes improve the primary result by **6/24**, or 25 percentage points,
over its own A baseline. C loses one action case relative to B. On the twenty
factual cases, Terra scores **14/20, 20/20, 20/20**; on four action regressions,
**2/4, 2/4, 1/4**. The primary gains over Luna are 9, 14 and 16 tasks in A/B/C;
there are no Luna-only successes in the paired positions. This is a descriptive
result on one public development corpus, not a general model ranking or an
independent holdout result. High is a provider setting, not equal compute.

| Terra measurement | A | B | C |
|---|---:|---:|---:|
| Native API methods | 47 | 78 | 96 |
| Full tokens | 190664 | 400990 | 576227 |
| Input / full output tokens | 184718 / 5946 | 392389 / 8601 | 565929 / 10298 |
| Reasoning subset of output | 3126 | 3228 | 3667 |
| Cached input / cache writes | 87406 / 97171 | 265179 / 126976 | 414794 / 150847 |
| Execution time, sum, ms | 199060 | 296410 | 634031 |
| Preparation time, sum, ms | 46358 | 42006 | 85240 |
| Actual memory.search / memory.open | 0 / 0 | 25 / 6 | 22 / 6 |
| Actual skills.load | 0 | 0 | 21 |
| Limited memory reads | 0 | 0 | 0 |

All **221** dispatched methods correlate with the raw PostgreSQL journal and
Core operation/run/reservation records. There are zero transport failures,
unknown usage, missing Core attempts, token mismatches or rejected source
projections. All 72 finals finish `completed` and satisfy the format contract.
The producer exits zero; this does not turn its thirteen failed tasks into
successes or establish product/issue115 acceptance.

## Memory, skills and remaining failures

Terra makes twelve successful searches and opens twelve sources through
Broker, compared with Luna's one successful search and one open. Terra still
has **35 empty searches out of 47**; the current exact lexical retrieval
interface remains a source of wasted calls. The model and the retrieval contract
both matter. The result contradicts a blanket claim that memory cannot help this
agent, while leaving room to improve query construction.

The reviewed skill is really loaded **21 times**; absence of skill loading does
not explain C here. C uses **43.7% more tokens** and **113.9% more execution time**
than B without improving primary quality. Both get all factual cases right.
These observations do not measure autonomous skill acquisition or generalize to
all skills. Notes and the reviewed skill are scripted preparation, with zero
native creation calls; authoring labor and production learning cost are unknown.

Remaining B/C failures are in action fields. C's inspect-only answer encodes
`status=failed; ready=false` instead of the fixed value `failed`. Both B/C
verified-negative answers have the correct value/effect but `check=failed`
instead of `not_checked`. All three arms actually prepare and verify an
unapplied artifact, retain the original recipient files and cite the real
artifact. They still return `ready=true` instead of the expected value `ready`.
See [diagnostics.json](diagnostics.json) and
[action-diagnostics.json](action-diagnostics.json). These posthoc observations
preserve the frozen strict scores, including semantically close value failures.
Factual effect/check diagnostics have zero mismatches in every Terra arm.

## Cost and time

| Whole comparison | Gemini | Luna | Terra 6.1 |
|---|---:|---:|---:|
| Native API methods | 281 | 203 | 221 |
| Full tokens, including reasoning | 2466920 | 1093764 | 1167881 |
| Execution time, summed minutes | 65.39 | 21.75 | 18.83 |
| Execution plus preparation, minutes | 68.00 | 24.71 | 21.72 |
| Known-usage Standard tariff estimate, USD | Subscription allocation unknown | 0.08317–0.12937 | 0.63200–1.10074 |

Terra's tariff estimate is **USD 0.63199945–1.10074195**:

| Standard tariff estimate, USD | A | B | C |
|---|---:|---:|---:|
| Minimum | 0.15570505 | 0.21521795 | 0.26107645 |
| Maximum | 0.27716880 | 0.37393795 | 0.44963520 |

These estimates use published Standard short-context rates, including separately
priced cache writes. Every response reports tier `default`. The 30-minute versus
12-hour cache-write split is unreported, so both lifetimes bound the estimate.
Reasoning is included in output and cache counts are input subsets, never added
again to full tokens. This is **not an invoice** or account-specific adjustment.
Measured times exclude manual authoring and some observer/setup overhead. Lower
workload from failures or abstention is not equal completion at lower cost.
See [versus-luna.json](versus-luna.json),
[versus-gemini.json](versus-gemini.json),
[official pricing](https://developers.openai.com/api/docs/pricing), and
[cache accounting](https://developers.openai.com/api/docs/guides/prompt-caching).

## Reproduction, preservation and cleanup

[launch.json](launch.json) freezes High, 65536 input/8192 output tokens, thirteen
methods per variant, 90-second requests, 420-second authority, 480-second cases,
the source/binary/plan hashes and actual kernel limits before key input.
Terra's conservative admission quotes are input/output 3/5 USD micros per token,
with five USD per task/mandate/conversation scope. This monetary reservation
change accommodates the requested model's prices without increasing tokens,
methods, actions or time. It is separate from measured cost.

[gate.json](gate.json) records all 72 controlled positions, actual Broker skill
approval/load, the explicit-model/price tests and source verification passing
without required skips. Unchanged production code retains separately identified
provider race/vet, PostgreSQL usage and transport validation provenance. The
reporter's legacy Luna/Gemini output was reproduced exactly, except for two
new descriptive model labels, without changing any old artifact or score.
See [report-validation.json](report-validation.json).

All eleven pinned historical receipts, including failed/interrupted spending and
the completed Gemini/Luna results, were reverified unchanged. Historical totals
with overlapping allocations are not added. [comparison.json](comparison.json)
and [classification.json](classification.json) contain the new measurements.

The authorized key traveled through private SSH stdin only. [cleanup.json](cleanup.json)
confirms no persistent API key copy, no OOM, removal of the isolated runner and
network, clean PostgreSQL stop, and retained private database files. No production
deployment, model default switch, merge or issue closure is part of this run.

See [study protocol](../../../../docs/testing/issue115-openai-terra61-v3-2026-10-30.md)
and the [OpenAI provider](../../../../internal/transport/openai/README.md).
