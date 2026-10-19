# Exact action authority

Fixed `tools.search`/`tools.load` contracts are credential-free local catalog reads. Only exact owning contract hashes/resources qualify as standard catalog operations for ordinary/delegated work. Current input claims, authority, mode policy and explicit denials remain required; search results and loading never grant execution permission.


Enabled fixed schedule controls are standard owner-goal capabilities. Effective
catalog tools include them without rewriting stored mandate lists; authorization
accepts only their exact owning hash, local resource, no credentials and read/
prepare effects. Explicit denials and all current mandate/approval/claim checks
remain authoritative. Other runtime and delegated permissions are unchanged.

The default-false owning `memory.propose_candidate` composition is a prepare
control gated by a current task and private readiness. Like a guarded tab request,
its grant pins the current input claim privately while its approved action hash
remains immutable. No proposal accepts a fact or grants task/mandate authority.

Shared typed contracts connect Orchestrator's trusted run/task/owner reader with
Tool Broker. Models propose tool arguments; they cannot author owner identity,
mandates, action bindings, approvals, grants or lease fences. Prepared intents
bind exact canonical arguments, contract/resource/credential metadata and task
versions. Grants are claimed once at the durable effect boundary.

The same-transaction source/lifecycle guard receives private read proof derived
from the actual matching PostgreSQL ownership row already locked during claim.
It overwrites caller token/Actor metadata and has no `ownership.Authority`; it
cannot acquire another write fence or escape to the RPC/runtime caller. This lets
an ordinary authenticated Broker RPC verify an owned converter continuation while
retaining every current input/source/task/lease/expiry check. See the
[fault-tail evidence](../../docs/ai/scenario-fault-matrix-acceptance.md).

Private HTTP context reads use a direct transport with no ambient proxy and
reject redirects. Provider proxy configuration cannot reroute service tokens
or trusted owner/task context through an external proxy.

Broad owner authority is authored only by the authenticated Orchestrator goal
module. `CheckTaskAuthorityTx` and `CheckTaskActionTx` run after the caller locks
current ownership/run/task, then lock mandate/goal/purpose-policy. Dispatch,
atomic results, model/child budget admission and task checkpoints retain those
locks until commit. `ResolveBudgetAuthority` uses the same transaction and
current assignment; no configured arbitrary mandate can override it. Exact task
mandates retain their private operator API.

Authority release deadlines clamp grants/model/runtime contexts to the earliest
goal, purpose-policy, mandate or active-hours boundary. Exact-action grants also
retain the verified approval expiry through dispatch waits. They do not reset the
immutable budget period. Same-transaction DB-time checks after budget waits
deny expired dispatch/model/child admission before permission is spent.

See [action authorization](../../docs/architecture/action-authorization-contract.md),
[owner goals](../../docs/architecture/owner-goals-mandates-spec.md),
[effect journal](../../docs/architecture/external-effect-journal.md) and
[budget enforcement](../../docs/architecture/execution-budget-contract.md).
Unit tests and isolated PostgreSQL suites cover hashes, exact scopes,
credentials, lease/task/owner changes, authority revocation, single dispatch,
uncertain effects and conservative shared reservations.
An actual PostgreSQL authority-row wait past a short exact approval deadline
denies claim without spending the grant or writing a dispatch record.

## Delegated permissions

Delegated tasks also require exact contract hashes, resources, effects and typed
credential references from their current bounded contract, before credential
inspection and again at dispatch. Parent versions/deadlines remain authoritative.
See `docs/architecture/delegation-contract.md` for the current implementation and
pending scheduling acceptance.
