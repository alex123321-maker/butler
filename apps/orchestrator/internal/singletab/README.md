# Owning browser selection

Legacy selection remains separate from guarded resource execution. Guarded
selection uses `internal/singletabcontrol` and mandatory current owner, source,
task/revision and Session lease checks.

`NewSelectionCheckpoints` owns the durable input/run/task transition. Composing
it through `NewDurable` suspends the current claimed input atomically with
publication of finite owner choices. `NewWaitingDurable` selects/revokes after
the old worker lease is released and queues the same accepted input. No model
or HTTP body supplies the worker binding or private native tab reference.

`ResumeSelection` requires a newly acquired Session fence and actual input claim.
It validates current source pins, task/constraints, original finite deadline and
the exact approved guarded session before activation. Restart after activation
can repeat these checks without extending any expiry or dispatching any effect.
The core reconstructs current definitions and starts a fresh model context with
only the opaque local selection receipt as private DATA. Unknown/incomplete tool
checkpoints cannot use this selection recovery path to replay an effect.

Default bootstrap keeps both operator flags off. A separate request composition
now proves the exact permanent Broker dispatch and original ingress claim, then
joins finite choices, immutable dispatch mapping and tool-running suspension in
one transaction. Core sends the durable request through its delivery surface;
Telegram directs to the protected Web choice page. Recovery checks a mapped
request's actual confirmed journal outcome before activation. An owner decision
cannot bypass an uncertain/lost Broker outcome. The private guarded status/click
runtime route remains a composition gate. A successful local
selection does not prove a browser action, recipient outcome or model quality.

Actual PG/pool2 tests run on the lab through `task-observations-lab.sh`, narrow
first. The broader native-host/Broker/Chromium runner verifies the unchanged
runtime boundary separately. Evidence is recorded in
[the acceptance report](../../../../docs/ai/single-tab-resource-acceptance.md).
