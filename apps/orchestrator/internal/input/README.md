# Durable input

Dormant communication hooks use `CheckCommunicationClaimTx` for the exact private
claim at admission, publication/correction and explicit completion. A live lease
does not let an old caller borrow a replacement worker's current claim. Distinct
interaction markers, ready/Finish monotonicity and ordinary legacy paths remain
separate; the profile is not installed by Bootstrap.
`ErrCommunicationStatePending` requeues only bounded verification. If the
checkpoint already records a question wait, Finish retains waiting_for_user;
if it records an early answer, it retains readiness. An uncertain durable read
cannot turn a no-expiry question into a queued Input that expires by ordinary TTL.

PostgreSQL-backed acceptance receipts and session-ordered worker claims. The
adapter `Execute` commits admission and returns a receipt; `Run` owns execution
independently of the request context. Orchestrator owns execution and recovery;
Session Service owns expiring fenced ownership. Approval Service uses the typed
transaction hooks to suspend and ready an input without writing the queue ad hoc.
Guarded owner tab selection uses the same transaction hooks with a distinct
version1 selection marker. Its exact ready hook permits revocation while already
queued, retains the original run/input and deadline, and preserves an immediate
decision until the previous worker finishes. `CheckSelectionClaimTx` requires
the current live input claim and exact stored selection marker before recovery.

`AcceptTx` joins an owning service's admission transaction, so an initiative's
task and receipt become visible together. The caller locks the session before
checking goal/mandate authority. Rolling back also rolls back the receipt;
`Accept` retains the standalone transaction boundary for ordinary ingress.

`AcceptBoundedTx` is the manager-owned delegated handoff: it saves only the child
task ID and exact versioned contract source, with an immutable deadline and one
to three attempts. A replay cannot extend either bound. Session and task creation,
shared child allowance, source grants and this receipt commit together; the normal
input worker still owns claims and run execution. No parent prose/provider session
is copied. Production delegated context/Broker composition remains separate.

Workers retain the original refs-only kernel event and use its accepted `input_id`
as the run deduplication key on every retry. `Store.VerifyDelegatedInput` implements
the core's private typed verifier for that mapping. It checks the current claim,
owner/session, run key, ownership fence, active pinned task and exact original
event/contract source on one connection, then rechecks lease, claim and admission
expiry with the PostgreSQL clock after any lock wait. A rewritten key without this
verifier fails closed. Catalog still independently controls selected source DATA;
the mapping grants no source or tool permission.

Defaults: two workers, 250 ms polling, 60-second renewed claims, ten-minute
execution deadline, 24-hour acceptance deadline and three recovery attempts.
`NewServiceWithExecutionTimeout` permits a positive startup execution bound up
to forty minutes for a declared runtime profile. Earlier caller, durable input,
task, mandate, claim and source deadlines still deny work; retries extend none.
Busy-session retries and approved continuations do not consume recovery attempts.
Waiting/blocked session heads deliberately block later execution. PostgreSQL is
authoritative; no Redis queue or live approval channel is needed for recovery.

One trusted nominal model-turn timeout may queue a durable model retry with a
two-second pause. It consumes a recovery claim and a fresh model reservation,
keeps the original deadline and retains count one across checkpoint replacement
and restart. A started retry cannot be admitted again after a crash. Current
ownership, task, sources, catalog and known effect checks precede recovery;
equivalent confirmed non-read proposals are denied before Broker preparation.
See [model retry policy](../../../../docs/architecture/model-retry-policy.md).

Independent claim expiries can be staggered after worker loss: the first ready
session may finish before another claim expires. Two available worker slots do
not synchronize those deadlines. The app's zero-inference
`TestDelegationParallelRecoveryWaitsNaturalClaimExpiryWithoutInferenceIntegration`
uses actual Broker admission, equal short claim TTLs started at different PG
times, natural expiry, current source/lease checks and two simultaneous core
streams. It keeps all accepted refs and conservative model reservations intact.

See [durable execution contract](../../../../docs/architecture/durable-execution-spec.md).
Tests require an isolated `BUTLER_TEST_POSTGRES_URL`; run them on the lab. The
lifecycle suite uses real stores and fault-injected provider/tool adapters, and
does not claim real external reconciliation or exactly-once delivery.


Authenticated owner controls bypass the normal FIFO and atomically cancel old task inputs. Ambiguous targets are saved as `waiting_for_user`; explicit conversation/new-task selection or an explicit target control resolves them. See [Owner Controls](../../../../docs/architecture/task-owner-controls-spec.md).

The dormant communication reply resolver joins `AcceptTx` only after current
verified binding/ACK, canonical session/task, source and authority checks. It
neither merges sessions nor guesses an ordinary reply's target. A repeated stable
delivery returns its Input; changed payload returns typed `ErrDeliveryConflict`.
The standalone communication owner handler maps that conflict to HTTP409.

Migration093 adds a private immutable communication ingress proof. Verified
Telegram messages and real Task-controller continuations mark their accepted
Input inside the owning transaction. Public payload fields cannot select this
epoch; epoch-0 workers and the database claim fence refuse such Inputs. The proof
pins actor/delivery, bot/binding/version and the actual accepted payload hash.

`ObserveStopped` returns content-free typed evidence to the owner-authorized
experience adapter for one already blocked/failed original. It requires an empty
claim, no current Session Service lease, exact owner/root-task/run identity and a
stopped run or actual UNKNOWN tool boundary. It neither exposes checkpoint text
nor requeues/unblocks/completes the input. Migration080 maintains the exact
material input head and tombstone used by later source/currentness fences.

`ObserveReconciled` is a separate owner-only content-free observer. It selects
existing original RunID, immutable pre-Resume observation and actual owner
control IDs; the database checks the preserved original checkpoint, standard
controller transition/revision/queued continuation and independently authorized
read/final/ABSENT reconciliation. No execution or input-control permission is
returned and the stale observation body is never read. Migration081's separate
input-link/event heads capture the exact control relationship without changing
the ordinary080 projection or treating arbitrary diagnostic text as its body.
