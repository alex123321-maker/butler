# Exact action authority

Shared typed contracts connect Orchestrator's trusted run/task/owner reader with
Tool Broker. Models propose tool arguments; they cannot author owner identity,
mandates, action bindings, approvals, grants or lease fences. Prepared intents
bind exact canonical arguments, contract/resource/credential metadata and task
versions. Grants are claimed once at the durable effect boundary.

Broad owner authority is authored only by the authenticated Orchestrator goal
module. `CheckTaskAuthorityTx` and `CheckTaskActionTx` run after the caller locks
current ownership/run/task, then lock mandate/goal/purpose-policy. Dispatch,
atomic results, model/child budget admission and task checkpoints retain those
locks until commit. `ResolveBudgetAuthority` uses the same transaction and
current assignment; no configured arbitrary mandate can override it. Exact task
mandates retain their private operator API.

Authority release deadlines clamp grants/model/runtime contexts to the earliest
goal, purpose-policy, mandate or active-hours boundary. They do not reset the
immutable budget period. Same-transaction DB-time checks after budget waits
deny expired dispatch/model/child admission before permission is spent.

See [action authorization](../../docs/architecture/action-authorization-contract.md),
[owner goals](../../docs/architecture/owner-goals-mandates-spec.md),
[effect journal](../../docs/architecture/external-effect-journal.md) and
[budget enforcement](../../docs/architecture/execution-budget-contract.md).
Unit tests and isolated PostgreSQL suites cover hashes, exact scopes,
credentials, lease/task/owner changes, authority revocation, single dispatch,
uncertain effects and conservative shared reservations.

## Delegated permissions

Delegated tasks also require exact contract hashes, resources, effects and typed
credential references from their current bounded contract, before credential
inspection and again at dispatch. Parent versions/deadlines remain authoritative.
See `docs/architecture/delegation-contract.md` for the current implementation and
pending scheduling acceptance.
