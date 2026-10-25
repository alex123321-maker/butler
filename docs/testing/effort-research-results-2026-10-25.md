# Issue97: local native Low/High pilot, 2026-10-25

**Decision: retain the working High default.** Low is a candidate for further
bounded testing on simple, source-complete tasks. Extra passes over unchanged
evidence brought no semantic improvement in this small corpus and cost more.
New verified observations enabled current answers in a separate controlled
fixture experiment. Neither result establishes production readiness, statistical
equivalence, a subscription saving, or performance on real mail.

This closes the first research experiment requested by #97, not the wider #76
behavioral acceptance. All inference, PostgreSQL, tests and analysis ran on this
Windows PC (Linux checks in its local WSL). No server deployment was changed.
The owner authorized the subscription and a new local browser login. Auth stays
encrypted in an isolated local store; committed exports contain synthetic data.

## Reproducible boundary

The [prospective v2 protocol](effort-research-protocol-2026-10-25.md),
[supplement protocol](effort-evidence-protocol-2026-10-25.md) and
[command README](../../cmd/effort-eval/README.md) define the exact driver.
The [evidence directory](../../experiments/effort97/2026-10-25/README.md)
contains manifests, actual input/output/state/attempt/outcome exports, closed
windows, per-case CSVs, separate profile-blinded reviews, expenses and hashes.
Each native window was admitted and exported before its first inference.
No window was reopened and no failed quality slot was retried or replaced.

| Window | Frozen producer | Binary SHA256 |
| --- | --- | --- |
| 96-slot v2 | `bc6056d2692dc1d907c9b38eba8abf43efbdebd8` | `ee51f685f07dc15903a33b93c880609214a59d1c00efd7495ec41b111bc20b9d` |
| 18-slot supplement | `925d6f34c879448f242e05fc15357f279b01db4a` | `f01a5c3cb0dedaf676f969e5efa63cd9eb73ea97c1b7aedc020ba262732de80a` |

Both binaries report Go1.27.2/windows/amd64. Later summary-only changes expose
the current amended goal/version to reviewers; they do not change producers.
The manifest retains instructions, native response schema, fixtures, source
availability, fixed grading facets, ordering and finite limits. One repetition
is descriptive: no noninferiority margin or statistical equivalence is claimed.

The authenticated subscription inventory advertised `gemini-3.8-flash-low` and
`gemini-3.8-flash-high`. Both used the same Cloud Code route and explicit trusted
`thinkingLevel=LOW/HIGH`, the same bounded response schema and 8192-token output
cap. Successful replies reported `gemini-3.8-flash-n`. This confirms tested
request settings and observed version, not undisclosed weights or allocations.
Production's existing automatic High configuration remains unchanged.

This is a **separate synthetic driver**, not the production Orchestrator/Broker
baseline. Its baseline reproduces the ordinary proposal → observation → next
model pass pattern; its fixture reads are simulated, with no external effects.
The real transport already retains function calls/signatures/responses; ordinary
history accumulation is not a new feature. The opt-in prototype adds typed
goal-versioned working state, primary observations, untrusted candidates,
hypotheses, open checks, contradictions, read receipts and remaining budget.
PostgreSQL persists research state. No new production memory class is installed.
No hidden chain of thought is requested, inspected or stored.

## Main matrix: 12 cases × 2 profiles × 4 modes

The corpus includes simple inventory, arithmetic with sufficient context,
multi-step lookup, old mail about tomorrow, ambiguous Derek, cancellation,
timezone conversion, absence, incomplete search, search error, owner amendment
and equal-version conflicting facts. All modes have identical initial source
opportunities and limits. Mode/profile order rotates. Baseline stops at its
first terminal candidate. Other modes add one generic repeat, a checklist on
the same evidence, or a checklist permitted to read additional fixture evidence.

| Profile / mode | Correct fixed facets /12 | Calls | Reads | Known total tokens | p50 ms | p95 ms |
| --- | ---: | ---: | ---: | ---: | ---: | ---: |
| High baseline | 12 | 17 | 6 | 29,095 | 8,621 | 35,007 |
| High repeat | 12 | 29 | 6 | 54,037 | 19,833 | 79,670 |
| High check same | 12 | 29 | 6 | 52,384 | 17,283 | 36,637 |
| High checked | 12 | 29 | 6 | 57,762 | 17,236 | 45,781 |
| Low baseline | 11 | 17 | 6 | ≥15,048 | 6,796 | 50,554 |
| Low repeat | 12 | 29 | 6 | 27,552 | 8,372 | 52,654 |
| Low check same | 12 | 29 | 6 | 27,824 | 8,734 | 55,363 |
| Low checked | 12 | 29 | 6 | 27,676 | 8,123 | 58,372 |

All96 planned slots produced outcomes; 208 actual native calls were made.
95 terminal candidates passed the fixed decision/value/source facets. Low
baseline `search_error`, attempt163, failed in `response_stream` with
`provider_protocol_error`; no terminal answer, usage or version was retained.
HTTP status and a finer cause are unknown. It is not evidence of a reasoning
mistake, quota exhaustion or server capacity. Its failure remains in completion
and latency denominators; its unknown expense is not counted as zero.

The remaining 95 include 56 supported answers, 16 necessary clarifications and
23 correct unavailable responses. These are distinct outcomes, not 95 successful
mail retrievals. Every case/arm is in `quality-v2-20261025a/per-case.csv`.
All successful paired arms have the same fixed correctness. A repeat's 12/12
versus baseline Low's 11/12 cannot establish a semantic benefit: the difference
is the independent stream failure. No review arm read new evidence after its
first final; each had the same six total fixture reads. V2 therefore measures
extra-pass expense but cannot measure a fresh-evidence advantage.

The extra modes added12 calls each, roughly 1.80–1.99× High baseline tokens.
Low totals are smaller, but its baseline total is incomplete and High/Low
generation differs. Median Low latency was smaller; tail latency was not
uniformly better. Nearest-rank p50/p95 include all slots; at n=12 p95 is the
maximum, not a stable population tail estimate. Snapshot byte counts measure
retained repeated driver context, not the complete provider wire encoding.

## Controlled new evidence: separate 18-call window

Three disjoint cases change cancellation status, meeting date and seat count.
Each profile produces one supported T0 draft. That exact draft is shared by
two reviews with identical current goal/version, prompt, checklist and limits.
Both learn that T0 no longer proves currentness at T1; only one receives the
new authoritative T1 observation. This supplement was committed prospectively
after inspecting v2 read counts, before reading v2 quality answer text.

| Profile / phase | Correct /3 | Current T1 answer /3 | Known total | p50 ms | p95 ms |
| --- | ---: | ---: | ---: | ---: | ---: |
| High shared T0 draft | 3 | n/a | 4,885 | 9,506 | 10,270 |
| High review same | 3 | 0 | 9,810 | 17,189 | 18,992 |
| High review fresh | 3 | 3 | 7,597 | 11,484 | 15,111 |
| Low shared T0 draft | 3 | n/a | 2,416 | 4,119 | 4,133 |
| Low review same | 3 | 0 | 3,021 | 4,134 | 5,121 |
| Low review fresh | 3 | 3 | 3,328 | 3,630 | 4,471 |

All18 completed correctly. Both profiles acknowledge missing current evidence
in all six no-evidence reviews; both give the new supported value in all six
fresh-evidence reviews. Extra thinking alone did not establish the withheld
fact. This isolates source availability under this fixture, not search ability,
real tool reliability, or general reasoning superiority. The six shared drafts
cost7,301 tokens, counted once; reviews cost23,756, total31,057. No draft is
double charged to both forks. With n=3 p95 is again only the observed maximum.

## Semantic review and complete known expense

Codex reviewed all114 profile-blinded entries against primary observations and
the current amended goal/version, before opening the profile mapping. Gemini
did not judge itself. The rater authored the fixtures and is not an independent
human; there are zero human raters and no inter-rater agreement claim. Fixed
facet grading was frozen code, not model confidence or free-text phrase matching.
The review found113 source-consistent terminal candidates, one missing candidate,
no unsupported factual additions and no unnecessary clarification. Two timeout
answers could say "at present" more explicitly; their current-attempt limitation
is recorded, without converting a transient error into proven absence.

| Expense category | Actual calls | Known total tokens | Unavailable total attempts |
| --- | ---: | ---: | ---: |
| Automatic-budget preflight a | 2 | 11,749 | 0 |
| Locally rejected schema preflight b | 0 | unknown in retained journal | 2 |
| Explicit-level/schema preflight c | 2 | 1,440 | 0 |
| Main v2 | 208 | 291,378 | 1 |
| Controlled supplement | 18 | 31,057 | 0 |
| Total | 230 | **≥335,624** | **3 journal entries** |

Preflight a Low's fenced JSON failure cost8,402 tokens; it is retained, not
repaired into a pass. Preflight b's two durable admissions were rejected by the
local native-schema constructor before HTTP (missing bounded array maxItems).
Its legacy UNKNOWN rows remain NULL; constructor regression evidence explains
zero dispatch without rewriting them as measured zero use. Preflight c separately
qualified both explicit levels/schema before quality inference.

Full known input152,962 + output55,997 + thinking126,665 = total335,624.
Total already includes thinking. Of232 admitted records,107 lack a thinking
count: 95 main, nine supplement, one preflight c and two local preflight b.
Missing counts remain NULL, not zero. The main's lower bound per correct fixed
response is291,378/95 =3,067.14 tokens, mixing supported answers, clarification
and unavailability; each arm's denominator and unknown counts are in its summary.
Input includes all repeated context. Currency and subscription quota expenditure
are unknown; token counts do not establish cash or quota savings.

## Validation and issue criteria

Fresh local validation:79 top-level Windows tests, no skips/failures, plus vet;
78 Linux race tests before the supplement delta, then57 targeted Linux race
tests for that delta; one separate actual native-adapter synthetic-HTTP/PG memory
budget integration test under race. Counts overlap and are not summed. Final
summary projection received Windows regression coverage. Two unrelated historical
fixtures were explicitly unselected in the four-package gate, not counted as
passed; the native adapter PG test ran separately. Whole-repository Linux
`go test -p 1 -run '^$' ./...` compiled successfully; it executed no tests.
GitHub Actions were skipped to keep execution on this PC. Raw logs are included.

| #97 criterion | Evidence |
| --- | --- |
| Real comparable profiles | Authenticated inventory, explicit level construction/wire tests, disjoint preflight c, observed native versions |
| Current loop versus addition | Boundary above, README, unchanged production function history |
| Opt-in bounded prototype | `BUTLER_EFFORT_LAB_ENABLE`, command, PG reservations/windows, finite call/time/context/output limits |
| Stop/progress/contradictions/fact separation | State tests, cancellation/deadline/stale-goal/duplicate-read/unsupported-quote/strict-JSON tests |
| Predetermined matrix, per-task failures | Producer bc6056d2, manifest/order, 96 per-case outcomes, attempt163 preserved |
| Extra pass versus fresh evidence | V2 actual read counts and separately frozen 18-call shared-draft supplement |
| Published decision | Keep High; further Low source-complete pilot only; reject routine extra unchanged-data passes on this corpus |
| Synthetic/native/real-mail separation | Native provider, simulated observations, no real mail or production Core/Broker acceptance claim |

The research harness reuses the evaluation principles from #78/#87 (off-model
facts, explicit sufficiency, complete known cost), without claiming their full
production acceptance. Further work, if commissioned, needs harder held-out
tasks, repetitions, an independent reviewer and a prospectively bounded actual
Core/Broker fixture before any rollout or adaptive profile switching.
