# Prepared Antigravity comparison — not authorized, not executed

Status: **PREPARED / NOT AUTHORIZED / NOT EXECUTED**. Preparation adds zero model
calls. This is a bounded proposal for a separate owner decision, not permission
from the diagnostic prompt and not a command to run the sealed delegation runner.

## Question and scope

Test whether one official CLI Flash request through its actual configured route
can complete while one native Butler AICode Flash request is rejected in the same
short observation interval, using the original same-account authorization and the
same user text. Preserve a safe typed explanation of any new429 instead of only
`rate_limited`. Observe, without promising attribution, whether backend Flash
quota changes across this pair.

This compares **client plus route plus serialized context**, not one isolated
variable or identical whole HTTP bytes. CLI includes its own system context and
thinking configuration. Results cannot retrospectively diagnose the fifth
window, establish a per-token subscription accounting rule, or satisfy issue16.
An exact wire/field isolation experiment would require a later separate proposal.

## Evidence that must remain pinned

The new private admission ledger must verify all five files below, carry forward
18 attempts and the five known/thirteen unknown usage pairs, and retain unknown
usage as NULL. Known24921 input/547 output stays a subtotal. The safe
[offline evidence artifact](../../deploy/antigravity/offline-wire-20261011.json)
also pins these hashes.

| Existing artifact under deploy/delegation | SHA256 |
| --- | --- |
| native-scenario-failed.json | 28c75c31827f8a25ca8ed101e6e04b8264a7e66ee9ce18808d504d5af748c8f0 |
| native-scenario-followup-failed.json | f5d25f36b574464508a4124ac7490d525e75d8c8cf60111a0bb637637e777619 |
| native-scenario-third-failed.json | b7a4b35b7b587fb8f0561cbdb18f5ad8088b7939e673ffa290e06ec10410b89b |
| native-scenario-fourth.json | 72d95bb5ac0bcf5bd4d8d4b28b1de6beb10d819a348c7e5efb187295ce28a1f9 |
| native-scenario-fifth.json | 5e180087d4e1d43fe52c6afac0c35763b69a8dfe906953a18eac49416bd7cded |

`/srv/storage/butler/native-acceptance-windows/baseline13` remains SEALED, with
no modification to any old marker/counter/report. A fresh independently named
window, if approved, starts at18 and ends at no more than20, including CLI
generation. It seals on the first execution invocation and records failures and
unknown cost. Remaining old numerical allowance and100% do not authorize it.

## Admission preparation before approval or forwarding

These controls still need a concrete private implementation and offline checks;
the current synthetic capture is intentionally incapable of live forwarding.
No live execution command is prepared here. The owner must receive the exact
source/snapshot and successful admission-control evidence before execution.

1. Pin the installed CLI1.3.3 executable hash
   `d97edcf90eda1799cd6cedc92616ef402f23c9752a723e8b8ae3e8f062ce458b`
   and the reviewed native Go source. Use a new isolated laba agent directory;
   leave the main deployment unchanged. No installation/update, Google login,
   refresh-origin replacement or fallback is part of this experiment.
2. Private backend only: compare the retained CLI authorization and encrypted-PG
   import through fixed audit booleans. Read minimal current model/license/quota
   metadata without generation. Resolve the actual CLI slug using the official
   [headless guidance](https://www.antigravity.google/docs/cli/headless/) and its
   current catalog. Export only `same_authorization`, `google_ai_pro`, safe
   wire model/experience, endpoint host/path, remainingFraction and timestamps.
   A current token mismatch may be rotation; investigate privately rather than
   switching accounts. No private profile, identity or raw credential output.
3. Require exact Flash `gemini-3.8-flash-tiered` in the *serialized generation* of
   each client before any upstream write. The offline AICode-mode CLI selected
   `chat-gemini-3.1-pro-low` with fixture licenses: copying that flag blindly is
   prohibited. Capture actual routing first; reject a changed/unrecognized host,
   schema, model or entitlement. No trial generation to discover the slug.
4. Establish privately that account AI Credit Overages is **Never**, CLI credit
   use is disabled, and neither request admits extra credits. Google documents
   the overage control in [Plans](https://www.antigravity.google/docs/plans).
   If the route's credit policy cannot be established, do not forward. No API
   key, paid supplemental credits, purchase or account change is allowed.
5. Put both clients behind a separate private dispatch gate. It must recognize
   the exact serialized envelope, bound payload and deadline, persist the
   attempt before network write and admit **one** generation per client. Deny
   retries/redirects/extra model calls before forwarding; cancellation must not
   reopen admission. Secrets remain inside this backend; report projection is
   an allowlist, not redaction of arbitrary provider text. Prove these controls
   offline with synthetic auth, success/429/malformed/timeout/retry fixtures.

## Exact profile and hard limits

| Item | Proposed limit |
| --- | --- |
| Prompt | `Ответь ровно BUTLER_AGY_COMPARE_20261011. Не вызывай инструменты.` |
| Context | fresh; no user files/history; no native tools, MCP, skills, plugins or delegation; CLI's built-in system context is acknowledged |
| Model/account | exact Flash wire model above, original audited account |
| Order | one normal configured CLI request; only after its successful terminal response, one native Go request |
| Attempts | at most2 new generations, at most1 per client, cumulative at most20 |
| Whole serialized input | at most65,536 bytes per request, including hidden system/context and wrappers; reject oversize before HTTP write |
| Output | explicit generationConfig.maxOutputTokens=128 per request, at most256 across the pair |
| Response bound | at most64KiB response bytes per request; cancel/close on overflow |
| Time |45s wall-clock per request,120s total generation window; remaining time never authorizes retry |
| Supplemental monetary cost |0; no credit overage, purchased credits or API billing; baseline subscription quota may be consumed |

Both serialized generationConfig values must be inspected and set to128 by the
private gate before forwarding, with the deliberate experimental override recorded.
The current Go production constant remains8192; offline CLI emitted no explicit
output cap. No undocumented CLI flag is assumed to enforce this limit. Unknown
serialization or inability to enforce this parameter makes the experiment
inadmissible. This lowered cap is not a test of the previous8192 delegation
profile. If thinking exhausts128 and yields MAX_TOKENS or an empty/truncated
answer, stop and report that limitation; do not increase the cap or retry.

Input byte bounds are not measured token counts. Conservatively reserve the
model's full1,048,576 input-token allowance per attempt for admission unless a
verified private tokenizer provides a tighter bound; preserve actual returned
usage or NULL. The cost0 cap concerns supplemental money, not a claim that
subscription consumption or all18 historical costs are zero.

## Stop conditions and retained observations

Before each generation, revalidate pinned artifacts, admission, account/model/
route/credit policy and deadline. Abort without forwarding on any mismatch or
unproven control. Charge each admitted attempt even if its result is unknown.

Stop the entire window after the first401/403/429/5xx, timeout, malformed or
truncated response, model mismatch, nonterminal stream, unexpected tool proposal,
extra attempt, cost-policy uncertainty or bound violation. A failed first CLI
request prevents the Go request. A failed Go request ends the window immediately.
No retry, model/account/endpoint fallback or enlargement of the budget.

Retain fixed safe fields per attempt: client and pinned source hash, timestamp,
actual host/path and model/experience, safe header metadata, request byte count,
enforced output cap, HTTP status, terminal finish enum, marker-match boolean,
nullable usage and nullable supplemental cost observation. Private error parsing
may export only allowlisted status/reason values such as RESOURCE_EXHAUSTED and
HIGH_DEMAND; unrecognized/missing/malformed/oversized detail becomes UNKNOWN.
Do not export raw response text, OAuth headers, thought signatures, IDs or account
metadata. Unknown reason or usage never becomes a guessed capacity/quota/zero.

At most one safe metadata snapshot before and one after the pair should record
backend Flash remainingFraction and tier fields. CLI may perform its own ordinary
metadata requests, but the private gate limits generations independently and
blocks credit-changing operations. Metadata and its safe timestamps are not
additional generation authorization. No periodic polling follows.

## Interpretation rules

- Both clients succeed: neither sampled request reproduced the failure. This
  does not erase18 historical attempts or prove a resolved production defect.
- First CLI429 HIGH_DEMAND: capacity is confirmed for that official-client
  observation; no Go request follows and the last historic reason remains unknown.
- CLI succeeds then Go429: a difference in these two client/route/context samples
  is established. Timing, hidden context, thinking configuration and route remain
  confounders; no single field is convicted without another approved experiment.
- Any generic429: upstream rejection is confirmed, quota versus capacity remains
  UNKNOWN unless recognized evidence accompanies it.
- A quota delta is observed: consumption occurred during the interval; this alone
  does not assign the delta to either individual request. No delta, including100%,
  does not establish free routing or absence of subscription consumption.

Save preparation and observed results separately. A fresh owner instruction must
explicitly approve this pinned bounded experiment; approval for diagnosis alone
is insufficient. No issue closure or deployment follows from this comparison.
