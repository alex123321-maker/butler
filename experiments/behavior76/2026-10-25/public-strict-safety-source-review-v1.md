# Public strict-safety source review before a future fixture-local freeze

Reviewed the uncommitted test-only diff against clean producer9c83730d and the
public zero-native safety/queued logs. No hidden case feedback, model answer,
golden or mapping was used. This is not a native admission approval.

The new cancellation hook correctly attempts to seal synchronously before
forwarding the exact owning watchdog reason. Pre-admission/post-Core UNKNOWN
queries fail globally if their own evidence read fails. The real-Broker
foreign-read integration retains the original UNKNOWN and proves no following
provider callback. The waiting/FIFO test uses the ordinary AcceptWaiting API and
checks that neither the original wait nor its queued successor was rewritten.
Worker completion now wakes the observation loop; workerErr synchronization uses
the closed done channel. These are useful public gates.

**Blocking transport classification bug:** observeStream currently seals any
`status=stream_closed`. Antigravity's actual single-tool handoff emits a
ToolCallRequested event with usage/version then returns and closes that
per-operation stream; it does not emit RunCompleted until the final text turn.
The wrapper changes status only on RunCompleted/TransportError, so a legitimate
GET proposal is currently sealed as an incomplete operation. Existing empty/
final stream unit tests do not cover that path, and the real-Broker test uses a
scripted provider outside this wrapper's normal invoke/observeStream path.

Required public regression: run_started -> exactly one valid tool handoff with
real correlation/usage -> close -> Broker result -> correlated SubmitToolResult
-> final/RunCompleted. It must not seal; empty/run_started-only, partial text
without a terminal/handoff, transport error, cancellation and unsupported batch
must retain strict failure behavior. A tool-looking field alone cannot waive
identity or stream-shape checks. This is transport-contract correctness, not a
relaxation based on hidden results.

Further prospective design requirements:

* Bind the new local mode only to new trusted admission metadata. Strict remains
  the default and historical entry points. Never clear a shared global flag.
* Native admission and local arm barrier must serialize; two ordinary input
  workers/automatic continuation can race. Repeated evidence polls alone do
  not prove that a new call cannot slip between UNKNOWN and arm closure.
* Do not use a Go subtest failure as an implicit local/global classifier. The
  old outer loop seals on !t.Run success. A future local stop needs its explicit
  durable disposition while preserving the failed/UNKNOWN quality outcome.
  A successful collection test does not mean its behavior passed acceptance.
* Current fixture Broker/server/client cleanup is t.Cleanup, while export/backup
  is a deferred function inside the subtest. t.Run returns only after cleanup,
  which can support next-fixture isolation, but a stronger quiescence receipt
  must name the point at which workers, streams, recipient transactions and
  Broker process are actually stopped. If exporting before process shutdown,
  prove no later transaction can change the exported state; otherwise stop the
  process while keeping DB/anchor available, then export. Retain cleanup errors.
* A globally caused cancellation always wins, even after a local flag is set.
  Only a specifically recorded local-barrier cancellation can be distinguished
  from genuine provider/authority/source failure; preserve its incomplete usage.
* Queued/waiting observation is not admission/authority or task completion.
  The new predecessor predicate matches the owning Claim terminal-status fence.
  Keep actual block identity and do not classify a queued no-run tail as a model
  answer. Automatic cohorts must remain separate from owner final results.

Public logs inspected:

* strict-global-safety-race-v2.log SHA256
  `008ce490a773070f6c176209c23e2c265408df413e1897afb3f0133f3f2bddb3`
* post-safety-queued-race-v1.log SHA256
  `5b665e8e4ff64e454d2d7e5a910e2fefc548930d346bbd67308b1abaec5bc448`

These gates pass their stated cases; they do not establish a complete native
pipeline, diagnose the historical750ms failures or approve the future local
mode. No fresh hidden authorship or native admission should occur before the
transport blocker and prospective isolation/global-precedence gates are resolved.
