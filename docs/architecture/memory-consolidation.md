# Bounded owner-reviewed consolidation (#89)

This contract has a separately default-off consumer and a code-owned bounded
qualification gate. It is distinct from current-conversation
compaction. PostgreSQL catalog sources, claims and temporal decisions remain the
only durable evidence/accepted truth; consolidation is a versioned DATA view.
No new model, transport, scheduler, service or dependency is introduced.

The first supported policy is one-use (`max_jobs=1`) over an immutable prepared
source/accepted-decision snapshot. Existing source-change coalescing and quiet
eligibility schedule that exact preparation. A parent or owner-decision change
makes it stale: the owner prepares a fresh snapshot/policy, compares its diff and
chooses it with current-head CAS. This version does not continuously rebase an
old policy onto new accepted decisions or automatically refresh parented copies.

## Independent authority across sessions

Only the code-owned `memory-consolidation-v1` handler may accept nonempty
`ModelPolicyRequest.Contexts` (JSON `contexts`, omitted when nil). Each exact ref
names a session, original terminal run and optional root task. At most eight
unique sessions are allowed. With contexts, top-level `job.task_id` and
`job.original_run_id` must both be empty. Creation resolves separate immutable
`ModelPolicy.ContextPins`: owner/current task and constraints revisions, assignment
and privacy. The owner authorizes this bounded set explicitly; neither a model
nor a source may add a context. Delegated tasks, another task in the same session,
non-cloud task privacy, an active session/run, and an insufficient quiet window
fail closed. Completed task identity/revision are retained. No old lease is used.

This first manifest supports exact owner-user sources and session/root-task
sources matching those pins. Project-scoped sources and arbitrary cross-project
grants are explicitly rejected; a project label is not a session privacy proof.
No model-generated scope or old source grant expands the prepared context set.

The complete immutable context manifest is copied into each model job. Its
fingerprint covers both exact source manifest and context pins. Empty contexts
preserve the exact legacy source-only fingerprint and omitted policy fields, so
an existing finite policy never gains broader permission or fails identity merely
because this extension exists. Only the new handler can use the extension.

Creation, scheduling, claim/source access and finalization, model admission,
supervision, per-HTTP authority/dispatch and atomic publication recheck every
context against current PostgreSQL state. Sorted union sessions precede sorted
tasks/assignment authority, then control/policy, sorted source graph, job and
sorted budget scopes. No transaction crosses provider HTTP. Current amendment,
stop, cancellation, assignment/privacy change or expiry invalidates old work.

One attempt reserves its complete conservative quote atomically against the
existing job, policy, owner UTC-day and every distinct actual root task/mandate
scope. Duplicated shared scopes are charged once per operation. Earlier deadlines
and active-hours boundaries from any context clamp the actual call. The existing
no-refund, no-reset, known/unknown usage and possibly-sent semantics are unchanged.

## Evidence and versions

Trusted preparation selects at most32 units and20KiB complete evidence text from
the exact owner-approved sources/claim IDs. Complete source ancestry remains the
privacy graph even if only leaf bodies are shown to the model. Accepted facts
are read through #88's typed current/historical temporal contract, carrying exact
claim/head/temporal pins; a model cannot set their status or interval. The typed
catalog derivative-to-fact guard checks them at every source read/release and
publication, independently of source content hashes.

The model returns only a partition of all supplied unit IDs into single,
duplicate, related or conflict groups. Trusted validation rejects unknown or
duplicate fields, absent/repeated units, invented text/authority, and a claimed
duplicate with different bytes. Code renders every unique complete value exactly
and retains all original refs/time/scope/decision metadata. Byte-identical text
is rendered once with every evidence ref. Different values remain an unresolved
difference regardless of model agreement. Frequency never raises confidence,
accepts a hypothesis or resolves an owner decision. Current typed facts, task
goals, constraints, mandates, approvals and effect states remain authoritative.

Atomic publication creates one immutable catalog DATA derivative with the full
parent manifest, algorithm version and existing model attempt/provider/cost
receipt. Additional generation rows contain only pointers, pins and hashes, not
a second text/accepted-fact store. A deterministic diff records added, merged,
compressed, conflicting and stale unit/group IDs. It never copies old deleted or
corrected text into the new generation. Invalid/partial/stale output commits no
generation or active pointer and never displaces the prior current evidence.
The trusted staged-result handler creates its result initially quarantined;
ordinary catalog search/open cannot expose an unactivated representation. Other
handlers retain their existing active DATA behavior. Owner snapshot preparation
and finite policy admission share `CreateModelPolicyTx`, avoiding orphan input
sources or a partially registered permission after failure.

## Review, activation and retirement

Authenticated owner controls inspect exact model grouping separately from
original evidence and typed decisions. A version becomes active only through an
explicit owner CAS naming the expected current version. Activation/rollback both
require current parent/claim/temporal proof and newest deletion anchor; neither
restores an old permission, accepted head, preference or removed text. Choosing
an older version is the same current-proof CAS operation. Stale expected heads
deny a concurrent overwrite. Generated versions cannot accept themselves.

The active pointer selects a retrieval presentation; it is not accepted truth.
Live context/open/provider/final publication still require all exact original
pins. Source correction, access/revoke/deletion, accepted decision change or
temporal expiry immediately deny stale views, including after reconstruction.
Privacy deletion uses the existing full transitive derivative cascade and newest
external anti-rollback manifest. No cached bytes can resurrect a discarded view.

The existing worker bounds remain:32 policies per scan,128 unexpired policies,
64 pending/running jobs globally, at most32 jobs per finite owner policy. The
consumer additionally limits retained content-bearing preparations to eight per
owner view and32 per owner, including failed jobs and pending cleanup. Preparation
waits before any paid work at that cap; a unique input-to-result constraint admits
at most one immutable generation per preparation. Owner discard/prune removes only inactive derivatives
through catalog deletion, preserving original sources and content-free
cost/identity receipts. Active or changed-current choices require explicit CAS.

The [owner API workflow](../../internal/memory/consolidation/README.md) exposes
prepare/list/preview/activate/deactivate/retire. Activation CAS includes both the
expected ID and monotonic head revision, preventing ABA. Reviewing a quarantined
target is owner-only and proves its current full graph; an old selected root may
be locked as metadata without re-reading its obsolete parents. Its source cannot
be activated through that exception. Retire first records an irreversible fence
and quarantines the input/result, commits, then calls ordinary catalog deletion
without holding the phase-one transaction. Anchor/deletion failure returns
`cleanup_complete:false`; it never claims physical erasure or permits activation.
Pending work remains counted until normal deletion/reconciliation completes.

## Qualification

Process composition defaults off. `ReleaseQualified=true` covers the independently
reviewed native V3 three-pair profile plus the consumer-specific
publication/read/provider/retry/deletion/restore controls and all eight #83 attacks.
The finite native window measures full foreground plus background expense and
retains its byte/time regressions. All failed/unknown
attempts remain in cumulative evidence. Smaller foreground context does not
imply lower full cost. Claimed completed architecture, controlled boundary tests,
and native quality/cost qualification are recorded separately.
