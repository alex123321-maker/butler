# Prospective fixture gate interface and proof obligations v1

This is a public contract to review the coming test-only gate. It creates no
production authorization and does not approve inference. The actual implementation
and full public regressions must be frozen before new hidden controls.

## State and trusted construction

Use one monotonic study state and one monotonic state per original episode-arm:

* study: open -> globally_sealed (never clear/reopen);
* arm: open -> stopping_local -> quiescent_local -> evidence_sealed;
* normal arm completion: open -> stopping_normal -> quiescent_normal ->
  evidence_sealed;
* global stop can supersede any arm state at any time, including during drain.

Construct the gate with exact new window, arm, fixture/schema, mode enum,
amendment/manifest hash, original deadline/caps and a trusted cancellation
function. Missing/unknown mode is strict. No request/response/expectation may
select its own mode, scope, task identity or permission. Historical/default
collectors must neither opt into this enum nor acquire its local exception.

Keep scope identities opaque but exact: actual admitted input/run/task/session,
automatic cohort, recipient/runtime/Broker process, schema/anchor and accounting
window. A fresh fixture can share a host or durable accounting service without
sharing authority or work queues. The gate is an additional denial/barrier;
ordinary Core/Broker/currentness/lease/source/budget checks remain authoritative.

## Minimal operations (names illustrative)

* `Admit(ctx, actualOperation) -> permit/error`: verifies trusted scope, global
  state, arm state, original bounds and actual required evidence, then reserves
  the real immutable accounting entry and registers in-flight ownership.
* `BeforeRelease(ctx, permit) -> error`: final gate check at the existing native
  inference-authority/recipient release seam, alongside unchanged actual
  authority. It grants nothing and cannot replace production validation.
* `ObserveUnknown(actualCorrelatedEffect) -> stopDecision`: strict mode seals
  globally; explicit new mode records the allowed local barrier. A failed or
  ambiguous proof read seals globally, never returns "no UNKNOWN".
* `SealGlobal(actualCause)`: monotonic global denial with durable safe cause;
  synchronously prevents every later admission, including during local drain.
* `Finish(permit, actualOutcome)`: records actual terminal/partial/cancelled
  events/usage and releases in-flight ownership exactly once. Required evidence
  write failure globally seals; it does not decrement spent admission counts.
* `WaitQuiescent(ctx) -> receipt/error`: bounded join/verification only, no new
  native calls, effects, retries or authority changes. Failure is global.
* `SealEvidence(actualReceipt)`: binds immutable after-state, actual unfinished
  tail and all hashes before Next can be admitted. It cannot relabel UNKNOWN or
  alter the semantic verdict.

The implementation may combine these operations, but each proof obligation must
have a concrete owner. Do not hold a mutex across arbitrary network/SQL/drain
work merely to simplify a test. Avoid a gate-owned global lock whose blocked
holder prevents safety cancellation or recording a higher-priority global stop.

## Linearization and races

Define an auditable order for admission, arm closure and global closure. A poll
or unsynchronized bool check is insufficient. Under the same coordination rule,
an admission either wins before the stop barrier and is registered/accounted as
already admitted, or loses and never reaches the provider/runtime callback.
Native journaling/permit ownership must not be lost between successful reserve
and registration. On reservation failure, fail closed without inventing a paid
call; on uncertain commit, preserve UNKNOWN and globally stop.

Do not conflate admission with physical release/commit. A pre-barrier admitted
call may already have reached a recipient; its effect can settle during drain.
Record its actual outcome and cancellation. A call not yet released must see
the closed gate at the trusted release seam. No barrier can retroactively undo
an already released effect. Never claim no late effect solely from a context
cancel, zero model callbacks or an admission counter.

Use a fixed process/DB lock order if both are involved. Unknown observation,
provider release, budget accounting and global-source cancellation must not
deadlock against one another. A global failure observed while arm-local stop is
being recorded still wins, and its independent evidence is retained. No local
decision may clear, mask or overwrite a global reason.

Track every worker/Core run, model stream (including its journal finish),
recipient RPC/transaction and Broker process capable of acting for the fixture.
Both ordinary input workers and automatic continuations must be included;
snapshotting only the owner input's run IDs is insufficient. A known local-drain
cancel gets an exact immutable decision identity/cause, not a generic
context.Canceled exemption. Provider error, source failure or unknown cause
during that cancellation still globally seals.

## Quiescence receipt

Before Next, prove and retain all of the following with actual identities:

1. Arm admission closed, original stop decision recorded, no re-entry permitted;
   study global state checked again and any global condition denies Next.
2. Worker/Core execution and stream observer/journal finalization goroutines
   joined; in-flight counts zero by explicit registration, not sampling alone.
3. Recipient transactions/RPCs completed or safely bounded and runtime/Broker
   process stopped or otherwise unable to produce a later effect. Preserve
   actual dispatch/receipt/object state; a commit racing cancellation is not
   discarded. Retain subprocess stop/wait errors.
4. Actual session leases and input claims are released/expired by ordinary owning
   paths, with no active owner able to renew or execute. Pending queued/blocked/
   waiting inputs may remain durable, but are not claimed, resumed, rewritten,
   expired by research SQL or counted as answers. Tasks may remain active in
   storage; do not manufacture an owner Stop solely for test cleanup.
5. After-state, actual effects/receipts/source consumer proof tables, exact
   anchor bytes/SHA/watermark and accounting exports are written and hashed after
   no later transaction can mutate them. If export precedes process shutdown,
   prove the exact transaction/admission fence that makes this safe; otherwise
   stop execution first while keeping the DB/anchor available for export.
6. Every remaining original step gets an explicit unexecuted-after-local-stop
   record referencing the decision, never a fake snapshot/final/effect. Original
   schedule identity and next position are checked; no same-arm seed/replay.

No inactive or failed input is automatically a quiescence receipt. The public
test must demonstrate both no continuing work and the independent immutable
object/dispatch state, not only a finished owner message.

## Deterministic public tests before semantic freeze

Use barriers/channels to force each race rather than timing-only sleeps:

* admission-before-stop is counted once and drained; stop-before-admission denies
  callback/reserve; repeat with owner and delayed automatic run;
* runtime admitted before barrier can commit once, exact receipt and final bytes
  survive drain; runtime release after barrier is denied with actual evidence;
* local UNKNOWN during another same-arm model stream; intentional local cancel
  retains partial/NULL usage without granting completion, while an injected
  provider error or task/source watchdog cause globally seals;
* strict/default UNKNOWN denies Next; new explicit local UNKNOWN stops all
  owning cohort, preserves failed tail and allows exactly the next original
  independently isolated fixture only after all quiescence proofs;
* missing/foreign UNKNOWN proof, journal reserve/finish/stop write failure,
  process wait failure, active claim/lease, blocked recipient transaction and
  crossed scope all globally deny Next;
* valid wrapped single-tool handoff and correlated Submit succeed; empty,
  run_started-only, unsupported batch/mismatched tool ref and incomplete/error
  streams do not acquire a false handoff exemption;
* normal waiting/FIFO and worker early return preserve actual rows and do not
  hang or manufacture completion; automatic final/effects stay separate;
* original admission bounds/absolute deadlines, exact predecessor accounting,
  source/authority guards and instrumentation remain in both modes. No model,
  auth or credential factory is needed for these gates.

The historical authority latency cause is still unknown. This gate must preserve
global failure propagation and phase-specific tracing; it must not claim to fix
query/pool/JIT scheduling by changing the750ms bound or the existing reserve.
Passes here qualify a public infrastructure seam, not hidden semantic quality,
full pilot coverage or issue76 closure.
