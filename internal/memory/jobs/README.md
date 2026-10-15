# Owned memory auxiliary jobs

Migration071 adds `ModelWorker` to this same queue, using the configured native
generative transport, independent finite owner policies, per-attempt shared
reservations/usage and atomic catalog DATA publication. It supports completed
root tasks under current revisions, source/session/schedule events, coalescing,
queue bounds, interactive priority, pause/revoke/cancel and crash takeover.
The registry defaults empty; the explicitly enabled `source-quote-v1` diagnostic
does not enable summary/extraction/consolidation consumers. Local-only waits
without cloud fallback. See [the full contract](../../../docs/architecture/background-memory-jobs.md).
The `Admit`/`LocalTarget` description below remains the existing local path;
`ModelWorker.Admit` is the separately checked generative path.

New consumers also require the [memory lifecycle security gate](../../../docs/ai/memory-lifecycle-acceptance.md).
The shared corpus currently runs through diagnostic quote publication, repeated
job/ACK handling and forgetting, with existing in-flight withdrawal/crash tests.
These kernel checks grant no summary/extraction/consolidation quality approval.
The model supervisor runs every 250ms with a 200ms proof deadline. It checks
the deletion manifest, current typed policy/assignment, complete claim snapshot,
source heads/access/hashes/scopes/parent pins, summary original-source predicate
(migration 072), and interactive priority in one owning transaction. It reads no
source bodies and takes no publication job lock. Admission, each network release
and dispatch, and publication retain the complete `AccessTx` proof. A failed or
timed-out supervisor still cancels the provider; unknown paid usage stays reserved.
After the provider stream closes, the worker stops and joins this observer before
local observation/publication, whose full proof owns the policy/session locks.
Within a job, full pre-HTTP authority/dispatch proofs serialize with periodic
observation. A tick yields only while that same mandatory full proof is running;
the provider cannot send until it succeeds, and the call deadline remains active.
This avoids self-contention while auth resolution repeats authority before HTTP.
Observation ends with bounded `Rollback(ctx)`, not a durable commit. Its helper
chain (`CheckRestoreTx`, `checkModelPolicyTx`/`checkPolicyTaskTx`,
`CheckTaskAuthorityTx`/`CheckAssignmentTx`, `butler_task_check_delegation`, claim
snapshot and source/summary metadata predicates) only reads/checks/locks; it has
no durable business/audit update to publish. Rollback errors still deny/cancel.
Full network and publication transactions retain their existing commits.
Rejected consumer output records the fixed `consumer_output_invalid` reason;
validator/model text is never copied into a public diagnostic or authority error.

`ClaimPurpose(kind, purpose)` additionally separates workers in the same kind.
It uses the existing bounded claim/fence/deadline path; selecting a queue does
not confer source access or replace per-attempt admission.

`memory_aux_jobs` is a durable bounded queue with stable identity, immutable
owner/purpose, original task/run/goal/mandate references, task/constraint versions,
exact source hashes/access revisions, finite local allowance/deadline and current
claim ID/epoch. A completed run's session lease never grants job permission.
Expired claims are reclaimed with a new epoch; late publication/finish is denied.

An authenticated owner creates the finite job policy, including unassigned jobs.
Task jobs additionally require the current original assignment. Every actual
`Admit` generates a new operation reservation, charges job/task/mandate/day scope
and rechecks the DB clock and authority after shared budget waits. Nothing uses
`local_no_billing` as locality evidence. Only an operator-pinned internal local
transport identity/endpoint is accepted; cloud/secret-derived jobs remain denied.

`Context` supplies the private catalog checker. The lock order is task/authority,
sorted complete source manifest, current job claim, budget. A 250ms supervisor
cancels in-flight local requests after revocation; every publication also checks
fresh PG proof, so late callbacks cannot regain authority. Claim deadlines bound
crash recovery. Attempt and allowance limits remain immutable across retries.
Private caller snapshots are compared against the complete canonical manifest,
allowance, original references, deadline and exact live claim deadline before
access or budget initialization. Context copies its source slice and claim time.
Ancestor revisions are included in the durable manifest, so a parent correction
also invalidates an unchanged report or chunk.

Production legacy cloud auxiliary workers remain disabled. The indexing Runtime
uses `EnqueueWithPublication` to save job and generation projection atomically;
`ClaimKind` isolates workers. Every model read/publication uses the current job
context, independently of old leases. Actual PG/race tests prove post-run independent
ownership, stale takeover/task/source rejection, per-attempt concurrent allowance
and local/cloud/secret fail-closed boundaries with pool size two.
