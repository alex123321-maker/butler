# Native browser citation repair without refetch, 2026-10-29

The new `evidence-reuse-01` task completed public form validation, one accepted
POST, browser reference reading and Web/Telegram delivery without HTTP fallback.
After a rejected mixed-evidence draft, the model retained its supported browser
citation and removed the unsupported one, without another page read. The joined
[qualification](browser-baseline/evidence-reuse/evidence-reuse-01-qualification.json)
is **assisted_complete_without_http_refetch**, not autonomous POST reconciliation.

## Problem and change

The previous form trial had one citation supported only by a frozen/local
observation and one supported by fresh browser.read. The guard correctly rejected
the first. Its feedback nevertheless prescribed `web.fetch`, and general research
instructions also directed the model to fetch primary pages and quote content_text.
The prior model fetched the reference again through HTTP before publishing.

Feedback now tells the model to reuse sufficient fresh current-run reads, retain
supported evidence and correct/remove unsupported citations with their unsupported
claims/links. A new read is needed only for missing, stale or insufficient facts.
The general instruction accepts actual qualifying browser page text and removes
the unconditional HTTP instruction. It explicitly separates local input, owning
action-outcome reports and external page observations.

Only instruction/feedback text changes. Supports/Pages eligibility, exact quotes,
URL/role/freshness checks, four-correction limit, current authority, source guards,
tool schemas/hashes, approval policy and POST effect verification are unchanged.
Dropping a citation cannot make its unsupported claim confirmed. The kernel does
not edit the model's answer, dispatch a replacement read or promote a snapshot
into a remote receipt.

## Validation before native execution

The new Orchestrator test composes the real researchFeedback/Check path with two
recorded observations: a frozen local draft and a fresh external browser read.
The mixed draft is rejected; removing its unsupported entry allows the retained
live/guarded-GET citation without another tool call. Foreign-run, stale, generic
frozen, local, failed and invented-quote variants still reject the repaired draft.

Windows targeted research/Orchestrator tests passed. The initial new fixture
omitted ArgsJSON for its browser read, so both positive cases correctly failed;
adding the required empty JSON object fixed the fixture before Linux/native work.
Full source and actual narrow Core each passed **45 selected tests/subtests under
race, zero skips/failures**, plus vet and production build. An intermediate build
is retained; the final build also removes the two conflicting HTTP/content_text
instructions. [Final build](browser-baseline/evidence-reuse/build-20261029T111341Z-2745920/).

## Native scenario and independent checks

Canonical owner ingress used the configured Antigravity/gemini-3.8-flash-high
provider and the same task wording as the earlier form case. The fresh task did
not receive tool calls, selectors, an expected reference, a forced draft or an
HTTP ban. All tools and kernel permissions remained available under normal policy.
The fixture received a fresh identity/config; no historical Input was replayed.

The model observed invalid quantity 8/range_overflow while frozen, corrected it
to 3/valid, and submitted once. The first title preparation was safely rejected
on a stale observation after delayed fixture mutation; its snapshot refresh and
failure remain in history. The receiver's fsynced journal contains **one accepted
quantity 3 POST**, at 11:19:04.427845951 UTC, zero invalid POSTs and **one read**.

The opaque submit transcript remains UNKNOWN. An explicit protected operator
report, based on the independent journal and the synchronous fixture handler,
confirmed its effect. Assistance was **five exact public-action approvals plus
one factual owner report**. Every action effect has exactly one dispatch. No
credential/PIN/budget reset, machine receipt or fault injection occurred.

After browser.read and browser.close, the model proposed two evidence entries:
one qualified browser citation and one unsupported frozen/local citation. The
new feedback was delivered as a known local non-dispatch. The model made one
additional catalog search and then published with **one supported entry, zero
unsupported entries**. It made **zero web/HTTP calls** and **zero browser reads
after rejection**. The remaining catalog search is not an optimized discovery
path and still counts in time/usage.

Private fixed comparisons verify the delivered reference, title/quantity and
validation/correction wording, with no credential canary. Input and run completed;
Web reached result_ready and Telegram persisted sent ACK. Raw answer, citation
text, control reference and private fixture config are not exported.

Evidence: [citation repair audit](browser-baseline/evidence-reuse/evidence-reuse-01-citation-audit.json),
[receiver](browser-baseline/evidence-reuse/evidence-reuse-01-recipient-audit.json),
[local validation](browser-baseline/evidence-reuse/evidence-reuse-01-local-validation-audit.json),
[owning journal audit](browser-baseline/evidence-reuse/evidence-reuse-01-final-audit.json),
[private reference comparison](browser-baseline/evidence-reuse/evidence-reuse-01-read-verification.json),
[answer check](browser-baseline/evidence-reuse/evidence-reuse-01-validation-answer-check.json),
[lifecycle and deliveries](browser-baseline/evidence-reuse/evidence-reuse-01-metadata-final.json).
The form verifier's read-specific expected_answer_present=false is not a failed
form criterion: its recipient/answer criteria and separate reference comparison
are true. Complete-task status requires the joined evidence, not one verifier.

## Inclusive time and usage

Acceptance 11:16:01.199217 UTC to completion 11:21:24.789996 UTC took **323.591s**.
Web was ready at 312.847s and Telegram sent ACK at **314.259s**; delivery overlaps
execution and is not a human-read receipt. There were **17 model attempts**,
**208,645 known tokens**, zero unknown usage pairs; monetary cost is unavailable.

[Raw timings](browser-baseline/evidence-reuse/timings/evidence-reuse-01.json) and
[per-action CSV](browser-baseline/evidence-reuse/inclusive-actions/README.md) retain
all 17 tool spans, including stale preparation, UNKNOWN submit and rejected draft.
Each action interval includes deliberation/context, scheduling, approvals/operator
waits and execution. The partition has zero unrounded residual. Rounded components
are 310.855s of intervening decision/context/wait, 2.117s of tool spans and 10.618s
of finalization; rounding can differ by 1ms from the total.

Diagnostic run states show model_running 216.841s and awaiting_approval 83.376s.
These are overlapping alternative views of the same execution, not additions to
Input time or pure isolated model deliberation.

The previous form Input took 368.632s, 19 attempts/230,597 tokens and one HTTP read.
The new record is 45.041s shorter and uses two fewer attempts / 21,952 fewer tokens.
This is descriptive, not causal speed evidence: approval-state time fell from
166.893s to 83.376s while model_running rose from 182.341s to 216.841s; context,
operator pacing and provider latency differ. No p95/SLO or general speedup follows
from these two assisted trials.

The separate Codex operator window from first timestamp 11:08:33 UTC through joined
verification 11:24:27 UTC was **954s (15m54s)**, including reasoning, implementation,
failed fixture construction, builds, rollout, native supervision and verification.
Documentation and CI after that boundary are separate. Earlier time is unknown.

## Narrow lab deployment and remaining scope

Only Core was rolled out 11:14:57.425053354–11:15:03.958641520 UTC, release
`/srv/storage/butler/main/releases/browser-evidence-repair-20261029-v17`, SHA256
`769255b6be083810af407b9306ed4dd891037a6411e0dd0e753dea1d0279583e`.
Broker v13, browser v15, PostgreSQL, Redis and Web identities/start times and Core
environment are unchanged. Existing 5s dispatch/3s persistence bounds remain;
zero migrations, unknowns resolved by deployment, budget or credential resets.
This is a narrow backport, not the complete PR or owner-review UI deployment.
[Rollout](browser-baseline/evidence-reuse/rollout-v17.json).

The host had 19MiB free. Three exact retired browser-task binaries were archived
with all running/stopped container mounts checked, each archive decompressed/hash
verified on durable storage before its original was removed. Active releases,
DB/history and unrelated files were retained; 76MiB was available before rollout.
[Retention records](browser-baseline/evidence-reuse/retired-retention-v17.jsonl).

General acceptance remains **NO_GO**, #163 stays open. This proves one assisted
form/citation-repair task without refetch. It does not establish autonomous POST
receipts, fewer owner approvals, immunity to arbitrary page changes, mid-POST
runtime recovery, popup SSO, CAPTCHA or WebSocket support. Earlier baseline and
native-runtime-loss records retain their original outcomes. Final CI is reported
on PR #177 / #163 for its exact SHA; prior green checks do not qualify new source.
