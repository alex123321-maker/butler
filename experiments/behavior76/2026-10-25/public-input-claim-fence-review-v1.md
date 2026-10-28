# Startup-only Input claim fence: bounded API review

Affected subsystem: Input worker admission/lifecycle. The optional hook is a
denial fence, not a task/controller substitute. A production Service API change
requires its run-lifecycle specification, orchestrator README and focused worker
tests, even when only the research fixture opts in. No inference is authorized.

## Acceptable scope

An immutable startup option may install a trusted callback around the owning
Store.ExpireQueued plus Store.Claim operation. Default NewService with no option
must retain existing behavior. The callback may return a non-nil release
function or a typed denial/error; it cannot supply/change a Record, select a
task/owner, manufacture authority, bypass FIFO or alter durable queue state.
The research scope uses the same admission coordination as model/runtime
admission and local closure. No model/input/corpus field can install the hook.

Acquire the fence BEFORE both queue expiry and durable claim acquisition; release
it exactly once on every return path and BEFORE executing process. Holding the
admission mutex throughout Core execution would deadlock local stop/diagnostics.
A claimed process that won before closure remains real work: let it finish
through ordinary owning result/claim/lease paths under its original context,
authority and deadline, even if a later admission is denied.

ErrClaimAdmissionClosed is a normal claim-loop stop only when the trusted gate
actually closed admission. It must execute neither ExpireQueued nor Claim and
must not cancel another worker's already executing process. An unexpected fence
error or successful callback with nil release is a configuration/proof failure,
not ordinary admission closure. Expose it to the owner/global stop; do not hide
it as a normal close or repeatedly log it while later claims can still run.
Likewise the research profile must surface required queue/claim SQL failures,
while default behavior remains unchanged. No claim refund or record rewrite.

Prefer a constructor option or enforce that configuration occurs before Run.
A mutable setter documented "startup only" is not enforcement: changing it
during Run can race with workers or remove the fence. Reject late mutation and
test it, or do not offer the mutable runtime path.

## Draining ownership: essential distinction

The observed live-claim race follows actual Service.process semantics: parent
cancellation deliberately leaves the claim for takeover. Thus closing provider/
runtime admission and then cancelling worker.Run before owning finish is unsafe
for this research quiescence claim. Do not erase, expire or filter that record.

Waiting for SQL count(live claims/leases)=0 is necessary but insufficient. An
expired claim may still have a live process goroutine; before starting process,
a worker may already hold an admitted record; source/lease cleanup may still be
running after the last provider stream. Need an actual claim-worker/process
drain/join signal under closed admission, before parent cancellation.

One bounded implementation is for the opt-in Run profile to notice when all
claim-worker loops have naturally returned after ErrClaimAdmissionClosed. Idle
workers return immediately; already executing workers first complete process
and owning finish, then encounter the closed fence. Only after all those worker
goroutines are joined may Run stop/join maintenance/sweeper goroutines and
return. Default Run semantics stay unchanged. An explicit workers-drained signal
instead of a changed opt-in return is also acceptable if it proves the same
ordering. A research evaluation counter alone is not this signal.

Quiescence then checks actual no-live lease/claim rows, in-flight stream/recipient
counts, final journal reconciliation and Broker/runtime shutdown. Pending queued,
blocked and waiting rows remain unchanged and do not count as responses. SQL
absence cannot substitute for joining live owners; goroutine completion cannot
substitute for checking the durable rows.

## Deterministic public regressions

* Default constructor has unchanged admission/FIFO/execution behavior.
* Close wins before claim: no ExpireQueued/Claim, no new attempts or executor
  callback, queued record unchanged.
* Claim wins first: an executor pauses on a public channel; close the fence and
  let the other idle worker exit. Assert the admitted executor's original ctx
  is NOT cancelled, Run does not report drained early, then release it and prove
  ordinary finish clears the actual claim and lease before workers join.
* Include already queued automatic work and a second session, proving no new
  claim after closure while previously owned work is accounted separately.
* Nil release, arbitrary hook error, queue/claim SQL error and late setter are
  observable fail-closed errors, not normal local stops or implicit retries.
* Release happens exactly once on success, not-found, SQL error and cancellation;
  no mutex is held while executing process/heartbeat/finish.
* An intentionally expired-but-still-live process cannot satisfy the drain
  receipt merely because SQL live-count is zero. Global/parent deadline wins
  without a synthetic cleanup success.

This API and the new fixture gate require a final precise source review after
the currently moving implementation settles. No new hidden controls, semantic
freeze or native admission should precede those proofs.
