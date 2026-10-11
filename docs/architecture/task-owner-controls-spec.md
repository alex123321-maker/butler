# Owner amendments and stopping work

Owner control is a kernel operation, independent of a model response or the normal
input FIFO. PostgreSQL commits the new task revision, constraints, provenance,
transcript instruction, control receipt and successor input together. The owner
control deadline is five seconds; unavailable storage or contention fails instead
of claiming the control was applied. Repeating the same channel delivery returns
the saved result; changed content with that identity conflicts. Explicit API
controls require the expected current revision.

## Authority and input meaning

Web task mutations use the configured owner-control Bearer proof. Missing
configuration, an actor header, a body owner ID or a model instruction do not
authenticate an owner. The token is never a context item. Telegram uses the
allowed chat and numeric `From.ID`, with its delivery/session bound in typed
`owneridentity.Actor` context. Both channels can change the same owner's task
when their authenticated owner identities match exactly.

`POST /api/v3/tasks/{task_id}/control` accepts `kind=amend|stop|pause|resume`,
`expected_revision`, a bounded full `instruction`, and, for amendments, optional
`goal`, `constraints`, `completion_criteria`, `execution_policy=normal|prepare_only`.
A unique `X-Butler-Delivery-ID` is required. Amendments can explicitly replace
goal/constraints/criteria; the instruction also becomes protected task context,
with channel/delivery provenance. Model outputs cannot rewrite this authority.
`GET /api/v3/tasks/{task_id}/controls` returns saved control results. The status
endpoint uses the controller too; completion still requires a verified result
covering current criteria. Manual `/working` edits return 409; execution updates
working state under its run fence.

Ingress `payload.intent` distinguishes `amend`, `stop`, `pause`, `resume`,
`new_task` and `conversation`. New tasks are explicitly created with goal and
criteria through the authenticated collection API; `new_task` ingress references
the saved ID. Ordinary conversation creates no tasks. Explicit conversation or
new-task selection dismisses a previously ambiguous target receipt.

Only authenticated owner text receives the conservative command classifier:
`стоп`, `stop`, `/stop`, `отмени`, `cancel`; exact pause/resume commands; initial
`не отправляй`, `только подготовь`, `только черновик`, `prepare only`, `draft only`,
including `стоп, только подготовь`. Quoted phrases in explanatory text do not
become controls. Telegram also accepts `/stop TASK_ID`, `/pause TASK_ID`,
`/resume TASK_ID`, `/amend TASK_ID INSTRUCTION`, bypassing pending provider login
input. Other text stays conversation unless a typed intent is supplied.

An omitted reference resolves only one unfinished root task owned by the actor.
Zero or multiple roots produce a durable `waiting_for_user` receipt with
`task_target_required`, without model/tool execution or task mutation. Explicit
control with an ID resolves the waiting receipt in input and target sessions.

## Owner observations

`GET /api/v3/tasks/{task_id}/observations` requires the configured owner Bearer
proof before any store read and exact task ownership. A two-second read-only
repeatable-read PostgreSQL transaction projects the assignment/initiative,
first 100 direct owned children, latest 50 action intents and shared root budget.
Overflow is explicit. The response contains typed owning metadata rather than
source bodies, tool arguments, provider errors or credential material. It grants
no execution authority. The ordinary task detail keeps its deployment perimeter.

Child run completion is distinct from the task's verified completion result.
Action consequences come from the recorded contract (`read`, `prepare`, `commit`,
or unknown); journal confirmation remains separate. Run failure or activity is
not a risk assessment and the legacy risk field now reports `unknown`.
Shared root reservations are conservative allowances, not measured consumption.
Actual model attempts across the owned tree report known input/output subtotals
and separate missing-input/missing-output counts. Absent budget records remain
unavailable. Neither missing usage nor a missing scope is presented as zero cost.

The same owner snapshot includes the latest 50 background jobs across the owned
root task tree, with explicit truncation. It shows durable kind/status/attempts,
requested model, exact task revision and elapsed deadline. It omits raw source,
purpose text, errors, claim IDs and allowance payloads. Queue state is not runtime
readiness or current source/mandate permission. Completed background processing
does not verify the task; old task revisions and elapsed deadlines remain visible.

The Web view loads this snapshot explicitly using page-memory owner proof.
Changing the task or proof clears the snapshot and fences late responses;
successful controls invalidate it. Goal/mandate and initiative links expose the
existing authority and observation views. Read failure presents unavailable data.

## Versions and branches

Every control increments task/constraints revisions and saves prior working
state in `task_control_targets`. Obsolete plans, pending proposals and scratch
are cleared in the new version. Historical runs, transcripts, artifacts, results
and external observations remain. Pending approvals expire; approved records
remain audit data but cannot verify against the revised task. Old inputs and
nonterminal runs are cancelled, and the ownership epoch is revoked. Amendment or
resume queues a fresh input for the revised root; old provider/checkpoint state
never authorizes it.

Stop cancels every unfinished descendant. Amendment/pause/resume pauses existing
unfinished descendants for explicit review. Completed/cancelled descendants stay
historical. A child cannot resume under an inactive ancestor. New children inherit
parent constraints and policy. Restrictive `prepare_only` propagates through the
tree and permits only Broker `read` effects; approval or a mandate cannot bypass
it. Opaque browser navigation remains unknown and is denied. The UI checkbox only
adds the restriction; lifting it needs an explicit policy/constraints amendment.

`task.Store.BudgetRoot` supplies the stable root task ID and owner across runs and
amendments. A new run does not reset this identity. Logical action slots belong to
the exact task revision; old checkpoint slots cannot become new-version intent.
Two legitimate same-args actions need distinct trusted intent slots, independent
of provider call IDs.

## Irreversible boundary and late replies

Transaction lock order is **ownership row, session admission, approval/run, task**.
Control first takes the per-root task-tree admission barrier, then locks every
descendant session's ownership row in sorted order before session admission and
captures the complete tree. Worker transcript sequence triggers lock session admission
after ownership; this order avoids a cross-process deadlock. No external call
occurs in the control transaction. Broker's irreversible journal/grant claim must
share-lock the same ownership and exact task revision/constraints/policy in its
transaction (#11). Commit order determines whether dispatch was already claimed
when cancellation arrived.

The supervisor checks its exact PostgreSQL lease every 250 ms, with a 750 ms query
deadline. Revocation cancels the run context and requests transport cancellation
with a two-second deadline. Unsupported transport cancellation cannot restore
authority. Late responses cannot write transcript/state/artifacts or complete an
old task version. Runtime cancellation is best effort where supported; it cannot
roll back a committed remote effect.

Receipts conservatively report
`dispatch_may_have_committed_reconciliation_required` if an affected attempt
crossed its recorded dispatch/result boundary. Control never rewrites an executing
journal entry as prevented, promises compensation or retries an unknown effect.
The effects journal/reconciliation flow records actual outcome independently of
the cancelled worker.

## Evidence and limits

Lab PostgreSQL tests cover ownership revocation, idle provider late final rejection,
protected context after amendment, waiting approval expiry, input cancellation,
child stop/inherited restrictions, two-channel CAS, owner proof, duplicate delivery,
ambiguous/no-task receipt and a cross-process transcript/control race with two
connections per process. Broker tests reject new and previously approved mutations
under prepare-only. Browser fixtures exercise authentication, pinned revisions,
consecutive amendment/stop and uncertainty presentation without token storage.

Provider-stream tests use deterministic events. Combined #9/#11/#12 tests also
use real PostgreSQL approval/checkpoint stores, the private authenticated HTTP
authority adapter, actual `actionauth.PostgresStore.ClaimGrant`, an HTTP recipient
with key/fingerprint deduplication and receipt evidence, and the actual atomic
`completion.Coordinator.Save`. They prove these implementation boundaries rather
than claiming remote-world rollback or arbitrary third-party receipt guarantees.
Migration 036 rollback refuses when control history or restrictions exist.

The acceptance checks map to these boundaries:

| Requirement | Verified boundary | Combined evidence |
| --- | --- | --- |
| Stop or prepare-only prevents an unclaimed action | Current policy and old authorization rejected; amendment revokes the exact PostgreSQL fence | Owner-first actual claim race rejects dispatch; recipient receives zero effects |
| Old proposals and approvals cannot authorize the new version | Pending approval expires, approved audit stays, stale task revision/fence fails | Previously approved proposal cannot verify, reauthorize or claim after owner-first commit |
| Stop bypasses ordinary execution | Accepted owner control interrupts an idle provider and a waiting approval without calling the normal executor | Idle-provider cancellation and late final rejection still pass with the required atomic coordinator |
| All unfinished descendants stop | Complete tree capture and inherited restrictions; independent-session delegation acceptance is pending | Root stop revokes an actual approved grandchild grant; the recipient receives zero effects |
| Already committed or uncertain effects remain | Historical tool observations remain; receipt reports uncertainty | Claim-first recipient response is saved after stop/amend; lost acknowledgement remains unknown with append-only evidence and no replay |
| Context follows admission and current task revision | Fresh continuation sees the complete protected amendment; late old final cannot append | Stop-first actual coordinator saves no result/artifact/transcript/outbox; result-first retains all immutable components and revision-one journal evidence |
| Ordering, pause/resume and ambiguity are explicit | Two-channel revision CAS, delivery deduplication, owner proof, ambiguous receipt and explicit clarification | Storage/kernel tests and functional Web controls |
| Dispatch/cancel linearization is documented | Ownership-first lock order also passes the cross-process transcript race | Both actual claim/owner and final-result/owner commit orders use independent two-connection pools and observed PostgreSQL lock barriers |

`authority_boundaries_integration_test.go` exercises nine combined cases:
owner stop/amend before claim; claim before stop/amend with confirmed response or
lost acknowledgement; each order of stop versus atomic result commit; and root
stop revoking an actual grandchild grant. The
claim-first test deliberately contacts the recipient after owner control commits,
showing why a spent, earlier grant cannot imply guaranteed prevention. Broker
outcome recording remains independent of the revoked worker. The final-result
tests retain an actual confirmed effect in either order; only an earlier result
commit creates immutable final components and a stored notification. Their
notification is the Web `result_ready` record, not a claim of Telegram delivery.

Barriers pause real PostgreSQL transactions or their real artifact writer before
commit. `pg_stat_activity` confirms actual lock contention instead of assuming a
goroutine reached its boundary. All waits have deadlines; each controller,
authorization and execution pool has `MaxConns=2`. No live Compose deployment or
model credentials are required by these tests.

Validation on the isolated lab snapshot: all orchestrator packages plus action
authorization, owner identity and transcript passed with PostgreSQL/Redis; the
affected controller/ingress/execution/approval/API/Telegram/authorization/transcript
packages passed under the Go race detector. Web lint and all eight functional
Chromium fixtures passed. The integrated branch fixes baseline API body typing,
alias resolution and doctor/settings DTOs; its pinned Vue/TypeScript check passed
on the lab too.
