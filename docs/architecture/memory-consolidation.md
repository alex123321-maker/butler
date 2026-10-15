# Bounded owner-reviewed consolidation (#89)

This contract is being implemented behind a separately default-off consumer and
a code-owned closed qualification gate. It is distinct from current-conversation
compaction. PostgreSQL catalog sources, claims and temporal decisions remain the
only durable evidence/accepted truth; consolidation is a versioned DATA view.
No new model, transport, scheduler, service or dependency is introduced.

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
consumer additionally limits retained content-bearing versions to eight per
owner view. Scheduling waits with a fixed reason before paid work at that cap;
publication rechecks it. Owner discard/prune removes only inactive derivatives
through catalog deletion, preserving original sources and content-free
cost/identity receipts. Active or changed-current choices require explicit CAS.

## Qualification

Process composition defaults off and `ReleaseQualified=false` until real
consumer-specific publication/read/provider/retry/deletion/restore boundaries
pass all eight #83 attacks and a prospective finite native paired task/answer
window measures full foreground plus background expense. All failed/unknown
attempts remain in cumulative evidence. Smaller foreground context does not
imply lower full cost. Claimed completed architecture, controlled boundary tests,
and native quality/cost qualification are recorded separately.
