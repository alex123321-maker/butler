# Exact action authority

Trusted Broker startup can opt into standing public URL/query release consent
using `SetPublicWebReads` (`BUTLER_PUBLIC_WEB_READS_ENABLED`, default false).
Prepare, authorize and verify apply it only to the fixed public page-reading
methods without credential refs or custom HTTP payloads. Independent current
authority, explicit denials, autonomy, contract approval and runtime network
checks stay authoritative; writes/extensions receive no exception.

Canonical local credential request/search controls can ask for owner input
without granting any credential capability. An owner-approved vault request can
authorize only its exact local browser preparation, with original Input claim,
current origin/account/task versions and existing deny/autonomy checks. External
submission remains under ordinary action and unknown-effect policy.

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

Revision1 `browser.submit_get_form` and `browser.follow_link` are pinned GET observations, with no
credential references or opaque/write policy. Its confirmed read does not
reconcile an earlier unknown mutation or revive a spent grant. Real PG/budget
tests cover independent read admission plus subsequent mutation denial.

`ReviewStoppedLocalTx` is an internal bounded recovery adapter for pinned
historical publication/read contracts, composed by the Input/Communication
kernel. It appends owning digest evidence under terminal executor locks; it
never grants/replays an action or treats an unknown read as successful. See
[stopped finalization](../../docs/architecture/stopped-input-finalization.md).

Delegated tasks also require exact contract hashes, resources, effects and typed
credential references from their current bounded contract, before credential
inspection and again at dispatch. Parent versions/deadlines remain authoritative.
See `docs/architecture/delegation-contract.md` for the current implementation and
pending scheduling acceptance.
