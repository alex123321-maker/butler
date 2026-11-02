# Owner mandate preparation without weakening POST, 2026-10-29

Existing owner-goal/mandate and operator tool contracts can remove individual
local-preparation approvals while preserving one exact submit decision. This is
isolated API/Broker/PG/Chromium acceptance, **not native model or delivery acceptance**.
Production configuration, mandates and binaries were not changed in this work.

| Configuration | Three local fills | Submit | Actual recipient POSTs |
| --- | --- | --- | --- |
| Mandate read/prepare only | 0 approvals | Out-of-scope denial, even before approval proposal | 0 |
| Mandate includes unknown; default click contract | 0 approvals | Automatically authorized: unsafe comparison | 1 |
| Same scoped mandate; click requires_approval=true | 0 approvals | One exact protected owner approval | 1 |

The unsafe comparison deliberately demonstrates why adding `unknown` to a mandate
alone is insufficient. Task prose requesting approval is not kernel policy.
The strict variant uses an existing authoritative registry flag; it adds no
permission field, credential exception or production authorization change.

## Composition and evidence

The new opt-in test joins authenticated goal/mandate/Task/control/approval APIs,
full PostgreSQL migrations, real approval and authority readers, a separate Broker
process, resource guards and the actual Chromium runtime. The Task controller
queues a canonical Input. A test-only mechanical worker claims it and creates its
run/lease/checkpoint; the existing fixture source-row adapter supplies currentness.
There is no model turn or full native Core worker in this composition.

Each scope selects the exact synthetic origin and path, tool names and effects,
positive finite budgets, expiry and allow_initiative=false. Actual local field
preparation freezes page activity: title, invalid quantity 8, corrected quantity 3
emit zero autosave POSTs. Grants reference the selected mandate with no approval ID;
the approval table remains empty before submit.

The strict click requires a real pending approval, authenticated owner approval
and current exact binding. Its one dispatch sends quantity 3. PostgreSQL retains
UNKNOWN; recipient observation does not silently promote that journal state.
Replay is denied, including after a real Broker process restart.
Changing args/origin, revoking the mandate, stopping the Task, mandate expiry or
loading a genuinely changed full contract hash all yield zero original dispatches
and zero POSTs. A smaller mandate budget stops execution with
`budget_egress_bytes_mandate`; reservations are conservative bounds, not measured
network traffic.

[Final logs, phase durations and hashes](browser-baseline/owner-mandate/integration-20261029T130050Z-3191177/)
record **11/11 cases PASS, zero skips**, race binaries and vet. Existing authority,
Broker restart, runtime-loss and form-validation regressions also passed with zero
skips: [regression](browser-baseline/owner-mandate/regression-20261029T130335Z-3203809/).
The credential CI script now requires the new composition; CI on the final commit
must be checked separately. No credential consent or OTP test was run here.

## Owner work and timing

Preparation has **zero action approvals**, submit has **one**. Setup still uses
four protected owner mutations: create goal, create mandate, create Task, amend
Task to queue work. Worker claim and pending-approval proposal are not owner
decisions. Thus the strict example needs five owner operations before any factual
outcome report. UNKNOWN reconciliation remains separate and was not exercised.
No reduction in total first-task owner work or native latency is claimed. Reuse
across multiple tasks requires separate qualification.

Final component parent time is **42.87s**, including the expiry case's **30.09s**.
The strict subcase is **1.38s** and restart subcase **1.42s**. These are mechanical
test durations with zero model calls and no human response delay. They cannot be
compared directly with historical native Input time.

[Per-operation CSV](browser-baseline/owner-mandate/operations.csv) is derived from
the existing final log; it includes prepare/authorize/execute and owner/worker
operations, with no rerun to reconstruct metrics. Build/vet/test phase times are
in the original timings.jsonl. The inclusive Codex work window starts at
**2026-10-29 12:19:59 UTC**; its final end is recorded in the issue/PR verification
comment after CI. That elapsed window includes reasoning, debugging, builds,
writing and waits, and is distinct from Butler model time (zero here).

Failed isolated iterations remain on laba under
`/srv/storage/butler/agents/browser-mandate-20261029/reports/`:

| Report timestamp | Result and test-driver correction |
| --- | --- |
| 12:40:32 | Wrong expected public delivery literal; actual contract is attempt |
| 12:43:47 | Conservative reservation exceeded the initial 10 MB mandate cap |
| 12:45:32 | Expected RPC policy denials/statuses mishandled by test driver |
| 12:48:57 | Registry.Get returns a clone; attempted mutation changed no registry |
| 12:51:57 | Changed serialized contract correctly failed old helper hash expectation |
| 12:56:13 | 11/11 PASS; final rerun adds distinct owner/worker operation labels |
| 13:00:50 | Final current-source 11/11 PASS |

The ordinary cap is now 100 MB; the intentional budget-denial case retains 10 MB.
No old native Input, budget or historical failure was reset. Public owner Task
delivery remains attempt; checking that enum does not establish Web/Telegram delivery.

## Safe next step

Use the strict authoritative click contract consistently in Core, Broker and the
resource profile before issuing an unknown-capable mandate. Its full hash changes,
so older prepared intents/approvals/grants must not be reused. Scope mandates to
the required origin/tools/effects/time/budgets; keep credential consent independent.
Before reverting to a looser registry, revoke/cancel such mandates/tasks and verify
denial: otherwise newly prepared actions could be automatically submitted.

Then qualify a fresh native owner Task with ordinary model ingress, real owner
submit approval, protected factual outcome resolution and actual delivery. Count
setup, action decisions, reporting and inclusive model/operator time separately.
No deployment or native test is claimed here. Issue #163 remains open and overall
browser baseline remains NO_GO.
