# Confirmed owner goals and broad durable autonomy

Optional [agent schedules](agent-schedules.md) retain the current exact assignment
and finite quota period. Automatic future work requires current initiative and
scheduling permission; an exact owner-approved schedule authorizes its declared
future occurrences without enabling unrelated initiative. No model can select a
different mandate/purpose or turn saved scheduling permission into effect rights.

Authenticated manual task admission accepts an optional `project_ref` plus
SHA-256 `project_revision` pair. The typed owner metadata allows the installed
initiative observer to follow current project attempts; it supplies no resource,
tool or credential authority. Invalid or incomplete pairs are refused and replay
hashes include the pair. Goal/mandate checks retain their existing contract.

Issue #14's implementation lives in the Orchestrator `goal` module. PostgreSQL
owns confirmed objectives, preferences, constraints, proposed assumptions,
versioned owner mandates, task assignments and authority-change jobs. Tool Broker
continues to own exact action authorization and credential mediation. Ordinary
conversation and manually created tasks do not require a goal or mandate.

## Owner decisions and provenance

Only an authenticated `owneridentity.Actor` may create or mutate owner authority.
The existing owner-control token and configured owner ID supply Web proof; actor
headers, body IDs, model output, extension tokens and private service tokens do
not supply owner proof. Source author/channel/delivery identity are overwritten
from that trusted actor and retained with every immutable decision snapshot.
Each request has an expected revision and unique source delivery identity.
Replaying the same delivery returns its original receipt; changing the request
under that delivery fails. The server rejects unknown fields, trailing JSON and
requests larger than 256 KiB. Tokens never enter goals, model context or logs.

The channel's exact-reference `RevokeMandate` command accepts only a mandate ID
and its expected current revision; owner identity and delivery come from the
authenticated adapter. It locks the stored mandate and bound objective, copies
its finite policy unchanged, marks revocation and queues branch cancellation in
one transaction. Existing allowance/reservations and prior effects are retained.
Replay checks the immutable delivery fingerprint before current-revision CAS, so
a lost acknowledgement remains recoverable after the original revocation or an
adapter restart. Another body under the same delivery conflicts.

Objectives, preferences and constraints are confirmed owner records. An agent
may propose an assumption under a fenced run, but cannot promote it or obtain
authority from it. Explicit owner confirmation creates a separate objective
whose source retains the proposed record reference. Model-derived observations,
summaries and guesses are data; they cannot write owner mandates.

A goal records title, exact purpose key, full constraints, author/source, revision,
status and deadline. Confirmed preferences/constraints also version the owner's
purpose-policy row. This immediately invalidates older assignments for the same
purpose, including assignments to other objectives. Fresh context includes their
full current text, using typed `OwnerPolicy` provenance and exact goal, mandate
and policy revision references. Mandatory owner policy is not silently trimmed
to fit the context budget.

## Broad mandate and exact actions

The Android `mobile_inbox` source is a standing owner-authorized DATA consumer.
It can read permitted phone messages and prepare source-backed notes/noise rules
through exact named Broker tools without per-message review. Current goal,
mandate, privacy, budgets and pauses still apply. The fixed Task Service
`mobile_batch_v1` verifier checks actual processing receipts, not the truth of
model conclusions; it uses no synthetic owner identity. Notification content
and learned noise rules cannot grant new tools, widen authority or change goals.

A broad mandate pins one confirmed objective revision and purpose, owner and
source. It contains exact tool names, explicit read/prepare/commit/unknown effect
classes, HTTP(S) resource origins and path scopes, credential alias/field metadata,
privacy, all eight execution budget dimensions, original budget deadline, expiry,
optional IANA active hours, owner intervention conditions, reporting rule,
initiative permission and optional reviewed extension permissions. No wildcard
tool/account permission is inferred from natural-language similarity.

HTTP resource scopes match the exact origin and a canonical path segment prefix.
For example, `/test-service` covers `/test-service/health` and rejects
`/test-service-other` and `/work-account`. Userinfo, traversal, encoded path
separators and backslash ambiguity are rejected. Exact action hashes still bind
the complete canonical arguments, including query and body; a changed query
cannot reuse an earlier approval. Credential permissions match exact alias and
field, and credential policy independently checks the account scope. Operators
must choose resource/credential scopes that identify the actual service/account;
a shared API whose target account is only in arbitrary request content needs a
dedicated typed contract before that content can become an account boundary.
Broad mandates currently support URL resources; arbitrary filesystem, shell and
non-URL resource namespaces require their own typed scope contracts.

Free-text goals and confirmed constraints remain protected owner instructions;
they are not translated into arbitrary executable predicates. Machine bounds
must be declared in typed mandate fields. Changing confirmed policy invalidates
old work rather than guessing a new scope from that text.

A trusted admission coordinator selects and persists the assignment. The model
cannot select an arbitrary mandate or purpose through tool arguments. Selection
uses exact owner/goal/purpose and current records: no candidate means denial;
multiple compatible candidates require an explicit ID. An explicit ID must
belong to that goal and purpose. Active explicit deny rules on the same goal
override a selected grant; two permits are not merged into a larger permission.
Mode 0/1, contract-required approvals and credential-required approvals remain
independent restrictions. Broad mandate permission does not bypass them.

The task binding records goal/mandate/purpose-policy revisions, initiative reason
and observation reference. Children inherit the exact binding. Every action
still receives a canonical prepared intent and one-use dispatch grant with live
task/constraints versions and worker fence. Shared mandate, root-task and owner
day budgets are checked atomically; a new run or independent task cannot reset
the mandate allowance. Mandate edits cannot change its original finite budget
limits or budget deadline. An operator must create a separately reviewed mandate
for a new allowance period.
An owner may shorten current mandate expiry while its original quota period and
limits remain unchanged; older assignments are still fenced by the new revision.
`local_only` denies current cloud/native inference; local inference also needs a
declared adapter privacy classification and bounds.

Authority release has a separate transient deadline: the earliest current
mandate, original budget period, objective, confirmed purpose policy or closing
active-hours boundary. It clamps dispatch grants, runtime contexts and model
turn contexts without changing persisted quota periods. Dispatch/model/child
admission rechecks the database clock after budget contention on the same held
authority rows. Expiry during that wait rolls back allowance and admission;
expired model contexts are rejected before calling transport. IANA active hours
use actual minute boundaries across DST folds/gaps and selected calendar days.

Reviewed extension permission pins exact source, ID, manifest hash and capability
subset. `CheckExtensionAttachmentTx` checks live task/goal/mandate/policy, hours,
expiry and explicit deny rules on the same transaction as a trusted installer's
attachment record. Changed bytes/capabilities or revoked rights are denied.
This is an attachment authorization gate. Discovery, byte verification, staging,
installation, rollback and enabling runtimes belong to the extension lifecycle
issues; they are not performed by the goal API. Later extension effects still
pass Tool Broker. Reporting and intervention fields are durable policy; this
module does not independently schedule daily reports or initiate discovery work.

## Revocation and transaction order

Checking order is ownership, session/run/task, mandate, goal, purpose-policy,
then shared budget/effect writes. Claim, result persistence, task state writes,
child admission and model reservation hold the current authority row locks until
their transaction commits. Owner authority mutations lock mandates in stable
order, then goal/purpose-policy, and never wait for task or ownership rows while
holding those authority locks. Admission callers must acquire session/candidate
locks before calling the authority functions; they must not reacquire an older
task after taking mandate/goal locks.

Pause, amendment, expiry or revoke changes authority synchronously and writes a
durable propagation job. Old dispatches, model admissions, task checkpoints and
atomic final results fail immediately after the authority commit, even if the
job fails or no propagation worker is running. An in-flight model is supervised
on the existing 250 ms lease cadence with bounded authority reads; transport is
cancelled when authority becomes stale. Late final output cannot create a result,
task result or delivery outbox entry. Descriptive run/input/transcript writes
retain their lease checks and may lag until that supervisor cancels the worker;
they do not grant execution or prove task completion.

The bounded restartable maintenance worker pauses amended branches or stops
revoked branches through the existing owner Controller. It reports `pending`
until branch controls actually finish; generic failed-attempt status is saved
without raw provider/database errors. Restart retries pending jobs. Sweep checks
the latest authority, so an older job cannot cancel a newly valid assignment.
Resuming a mandate or goal creates a new revision; old branches remain stale
until a trusted coordinator admits work with the new assignment.

If dispatch committed first, the grant stays spent and the journal retains the
external confirmed/unknown evidence, even when the owner revokes before the
recipient responds. If a final result committed first, its immutable evidence
and delivery intent remain. Revocation never implies an external effect was
undone, a delivery was recalled, or a model final verified the owner's goal.

## Owner API and private admission contracts

All public routes require owner-control Bearer proof. Mutation routes also need
`X-Butler-Delivery-ID` and return a saved revision plus propagation state.

| Route | Contract |
|---|---|
| `GET /api/v3/owner-authority` | Goals, bounded mandates, current/stale obligations and pending propagation |
| `POST /api/v3/owner-goals` | `{goal, expected_revision, parent_ref?}` |
| `POST /api/v3/owner-mandates` | `{mandate, expected_revision}` |
| `POST /api/v3/owner-goals/tasks` | Owner task admission with goal/mandate/purpose, rationale and observation |

The `/goals` Web page reads and edits these records, explicitly confirms proposed
assumptions, pauses/revokes/resumes mandates and shows each obligation's authority
and reason. Advanced mandate JSON editing exposes exact hours, deny scopes and
reviewed extension manifests. The token stays in page memory, is cleared on
unmount and is never persisted to browser storage. Provision/retrieve the token
through the terminal, not model chat. Stale decisions are refused and require
review of the current revision.

`Manager.CheckAdmissionTx(ctx, tx, AdmissionRequest, at)` returns a current typed
`actionauth.Assignment`. `actionauth.CheckAssignmentTx` returns its current typed
mandate/goal and retains locks for prospective budget admission.
`task.Store.CreateTx(ctx, tx, CreateRequest)` persists the private `Authority`
binding; that field is excluded from JSON ingress. Coordinators may add task and
durable input in the same owned transaction. The standalone owner task API
creates a task, not an execution input. Initiative scheduling, deduplication and
task/input promotion belong to the initiative coordinator.

`CheckAdmissionTx` always requires `AllowInitiative=true`. Authenticated manual
task creation uses a private admission path that keeps every current scope,
owner/version/deadline/conflict check but does not require permission to initiate
spontaneous work. Only the proved owner surface can choose this path; there is
no ingress field that lets a scheduler, extension or model bypass that flag.

Trusted admission may set private `InitialExecutionPolicy=prepare_only` on a new
root. It is excluded from JSON and accepts only existing policy enum values.
Children inherit their parent policy and reject conflicting overrides; a broad
mandate never widens an explicitly restricted task.

Migration `038_owner_goals_mandates` retains existing exact task mandates and
adds no authority to ordinary/legacy tasks. The additive
`038_owner_purpose_policies` migration versions confirmed purpose policy;
filename-based migration identity orders it after the foundation. Down refuses
authority/history that would be lost. No seeded owner goals, mandates or personal
facts are installed; fixture examples use synthetic test services only.

See [acceptance evidence](owner-goals-mandates-acceptance.md),
[exact action authorization](action-authorization-contract.md) and
[shared execution budgets](execution-budget-contract.md).

`Mandate.converters` separately admits a finite pure delimited-to-JSON class:
exact converter ID, schema hashes, runtime profile hash, byte/row/field/time limits
and trials. Current tool/resource/effect/purpose/hour/budget/deny checks remain
mandatory; generated code never selects its own authority. It does not widen the
reviewed ExtensionPermission or allow another account. See
[generated converters](generated-converters.md); actual child admission must
additionally retain the separately reviewed delegated permission boundary.
