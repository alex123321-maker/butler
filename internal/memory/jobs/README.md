# Owned memory auxiliary jobs

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

Production legacy auxiliary workers remain disabled. Semantic/parser adapters
and completion scheduling are follow-up wiring; this package alone does not
claim completed issue #22/#23/#25. Actual PG/race tests prove post-run independent
ownership, stale takeover/task/source rejection, per-attempt concurrent allowance
and local/cloud/secret fail-closed boundaries with pool size two.
