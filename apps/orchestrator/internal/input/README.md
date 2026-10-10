# Durable input

PostgreSQL-backed acceptance receipts and session-ordered worker claims. The
adapter `Execute` commits admission and returns a receipt; `Run` owns execution
independently of the request context. Orchestrator owns execution and recovery;
Session Service owns expiring fenced ownership. Approval Service uses the typed
transaction hooks to suspend and ready an input without writing the queue ad hoc.

`AcceptTx` joins an owning service's admission transaction, so an initiative's
task and receipt become visible together. The caller locks the session before
checking goal/mandate authority. Rolling back also rolls back the receipt;
`Accept` retains the standalone transaction boundary for ordinary ingress.

Defaults: two workers, 250 ms polling, 60-second renewed claims, ten-minute
execution deadline, 24-hour acceptance deadline and three recovery attempts.
Busy-session retries and approved continuations do not consume recovery attempts.
Waiting/blocked session heads deliberately block later execution. PostgreSQL is
authoritative; no Redis queue or live approval channel is needed for recovery.

See [durable execution contract](../../../../docs/architecture/durable-execution-spec.md).
Tests require an isolated `BUTLER_TEST_POSTGRES_URL`; run them on the lab. The
lifecycle suite uses real stores and fault-injected provider/tool adapters, and
does not claim real external reconciliation or exactly-once delivery.


Authenticated owner controls bypass the normal FIFO and atomically cancel old task inputs. Ambiguous targets are saved as `waiting_for_user`; explicit conversation/new-task selection or an explicit target control resolves them. See [Owner Controls](../../../../docs/architecture/task-owner-controls-spec.md).
