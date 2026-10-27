# Frozen fresh observer: source-only wait-path review

Scope: public source only at frozen producer9c83730d. No live fresh evidence,
answers, controls, expectations or arm mapping was read for this review. No
process, source, native entry, approval, task state or deadline was changed.
This is forensic planning, not a diagnosis or quality judgment of the live slot.

The code has a definite legitimate waiting path that produces no inference:

1. Ordinary Input processing can persist `waiting_for_user` when Core returns
   AWAITING_APPROVAL; interaction/control-target paths can also park an Input.
2. Input Claim selects only queued/running records and respects waiting/blocked
   FIFO heads. ExpireQueued changes only queued Inputs, not waiting_for_user.
3. evaluate's owner-status switch recognizes completed, failed, blocked, expired
   and cancelled. It has no waiting_for_user branch, so its terminal flag remains
   false. It skips the terminal-cohort quiescence query and polls every50ms.
4. With native sealed=false, the only exit from this state is the original
   fixture context deadline or an actual ordinary external state transition.
   No native call is needed while waiting. Thus no model progress/idle clients
   can be consistent with this path, but cannot prove it is the current cause.

The fixture context is created once with20-minute timeout bounded by the parent
fresh four-hour context. Reconstruction does not create a new fixture context;
mandates are clipped to that existing deadline. A worker's individual execution
timeout is10 minutes, but an execution that has already returned waiting state
need not remain an active claimed process. The original fixture deadline, not
an invented new window/mandate, remains the controlling finite bound here.

The current concrete `Input.Service.Run` has no spontaneous early-return path:
it starts workers, waits for ctx.Done, waits its goroutines and returns nil.
Therefore an unselected done channel is a generic diagnosability weakness if
that implementation ever changes or panics, but source does not establish it
as an explanation of this live stall. Process panic would ordinarily fail the
test/process rather than silently produce a normal early return. Claim/pool
errors may be logged and retried by ordinary work loops; no lock wait does not
prove that every queue/state/recovery condition is healthy.

No change is appropriate inside the frozen run. Do not approve a synthetic
interaction manually, insert an owner answer, mark a row terminal, clear a
native seal, recreate a Core, retry the slot, extend the context/mandate or
restart the collector. Let the original deadline/safety gates determine the
actual boundary and retain all resulting partial state and nullable charges.

After the window is closed and immutable, distinguish these possibilities from
actual evidence rather than inferring them from this source path:

- accepted owner/cohort input status, checkpoint, error_code, actual run ID,
  claim identity/expiry, not_before and expires_at;
- run current_state/status/lease/fence, task status/revisions/mandate boundary;
- canonical approval/interaction/wait metadata and its real actor/source/target
  IDs, if any, preserving unanswered state and the absence of owner approval;
- native request/event order, completed versus partial/empty stream, actual
  tool proposals/dispatch/effect states, and independently observed resource
  state, without turning a stream_closed label into a successful final;
- observer return/failure phase, original context deadlines and cleanup outcome.

Some canonical approval/interaction tables are not in the public48-table raw
allowlist. The frozen per-fixture full PG dump plus exact deletion-anchor backup
is the preservation mechanism if extra post-closure metadata is needed. Extract
only required non-auth metadata into a separate derived forensic artifact; never
rewrite original rows, restore the running fixture, or read credential material.

If the original bound expires, the caller records the actual collector failure,
exports state and closes/seals according to current code. The deadline is not
evidence of a model refusal or of successful necessary clarification. An actual
waiting witness may later support a bounded observation, while missing final/
continuation evidence remains unavailable under the unchanged rubric. The
scripted preflight did not establish genuine waiting-path behavior, since its
provider returned constant finals rather than canonical user waits.

Any future observer improvement belongs after immutable forensic assessment and
a separate prospective decision: an explicit waiting observation class, safe
cohort boundary, done/error observation and offline waiting tests. This review
does not approve that future change or another native window.
