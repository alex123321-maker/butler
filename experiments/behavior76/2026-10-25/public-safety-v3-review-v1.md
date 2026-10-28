# Safety v3 public source/log review

Reviewed the current public test-only source and
`.cache/behavior76/strict-global-safety-race-v3.log`, SHA256
`fec3d07d561053d7171ad20be92d9a0e917021e654cf18b2500f8fa9deb57ca7`.
No holdout/mapping/quality disclosure, new hidden authorship or native execution.

The previously reported legitimate single-tool handoff blocker is resolved for
the covered path. The wrapper now records `tool_handoff` when a normalized tool
proposal arrives and separately records `completed` for an assistant final.
The new public integration passes through actual Input/Core/current fixture
authority and production Broker GET, with the wrapper's invoke/observeStream
path active. The scripted provider emits run_started plus one measured proposal
without run_completed, verifies exact run/ref in SubmitToolResult, and emits a
measured final-only response. Assertions prove one start, one correlated submit,
two accounting admissions, one handoff and one final, without global seal.
The fake admission journal is in a separate public-safety database, not the
original closed native history. This is zero-native infrastructure evidence.

The same log passes the existing owning-cancellation ordering, incomplete-stream
seal, actual foreign-read UNKNOWN strict stop and canonical waiting/FIFO checks.
It supports those stated cases and does not establish the coming local gate,
quiescence/isolation races, complete provider stream-shape qualification or the
historical authority latency cause.

Remaining public pre-freeze coverage should include the negative classifier
boundaries in the new gate contract. In particular, the current handoff branch
also accepts a nonempty ToolCallBatch payload as a handoff; the owning Core
advertises SupportsBatchToolCalls=false and rejects batches. That rejection
must not be mistaken for a successful provider handoff/workflow, or for a local
UNKNOWN exception. Add the bounded public unsupported-batch/mismatched-ref cases
at the actual Core/gate seam. This is a negative safety gate, not a request to
enable batch execution or tune model instructions.

Review snapshot hashes:

* behavior76_native_test.go:
  `bdd66a78e479b8262cac16acb329e08ee96f030ef063c650194122f99c3dedd9`
* behavior76_safety_test.go:
  `a424bc6edd74d535452f3a363cc644d9f649baf7840a24ee30529e418de67b9c`

The supplied prospective matrix and gate interface define required behavior for
linearized admission, global priority, exact local-cancel causes and independent
in-flight/process/transaction quiescence. They are not an implementation review
of a gate that has not yet been supplied, nor a semantic source freeze/admission.
