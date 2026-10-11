# Durable execution acceptance evidence

Issues #5 and #6 change ingress, Session ownership, Orchestrator recovery and
Approval continuation. The implemented contract is
[durable-execution-spec.md](durable-execution-spec.md). The following evidence
was collected on the lab in isolated test databases, with PostgreSQL and Redis;
application execution was not tested on the Windows development host.

## Issue #5

| Criterion | Implementation and evidence |
| --- | --- |
| Repeated delivery creates no second work item | PostgreSQL acceptance identity; `TestDurableInputAcceptanceOrderingRecoveryIntegration` checks duplicate delivery, changed-payload rejection and one execution. Explicit task attachment remains optional. |
| Client disconnection cannot cancel accepted work | HTTP 202 receipt commit precedes the application worker. `TestHTTPAcceptedInputSurvivesClientDisconnectIntegration` closes the client context and subsequently executes the saved receipt. |
| Busy sessions wait with defined order/fairness | Serialized admission, oldest unfinished head per session, oldest ready head across sessions, `SKIP LOCKED`; the durable input ordering test checks concurrent sessions, second-message order and blocked-head protection. Busy ownership does not consume retries. |
| Expiring ownership with monotonic authority | PostgreSQL clock/lease/epoch, guarded writes and independently checked Broker authority. Session integration tests cover expiry, takeover, stale renewal/release and two-session execution with a two-connection pool. `TestStaleExecutorCannotPublishDurableAuthorityIntegration` checks stale run/task/working/artifact/activity/delivery writes. |
| Checkpoint recovery without arbitrary replay | Real PostgreSQL checkpoints survive replacement of worker/Service instances. Preparation, approval wait and known-result recovery preserve IDs. `TestDurableUnknownEffectAndKnownResultRecoveryIntegration` blocks dispatched missing/unknown results without executing a second effect. |
| Observable deadlines, blockages and bounded retries | Receipt state/checkpoint/attempts/deadline/error code, queued expiry and ten-minute execution deadline. Input ordering test verifies repeated expired claims stop at the retry bound; `TestModelStreamReadHonorsExecutionDeadline` checks an idle provider stream. |
| Context prepared after ownership at a known history version | `prepareRun` moves behind lease acquisition; preparation recovery avoids duplicate current-run user input. `TestCrashAfterUserTranscriptRecoversPreparationOnceIntegration` proves no provider call before preparation completes and one user row after recovery. Main's #10 verified summary/sequence/tail/context admission must be retained when integrating this branch. |
| Updated ingress/client/docs | Additive gRPC receipt fields and session fencing epoch; REST 202/Location plus GET receipt; Telegram submits to the same worker. Smoke event client polls the receipt; Web `/inputs/:id` fixtures observe queued-to-completed and separate task verification. |

## Issue #6

| Criterion | Implementation and evidence |
| --- | --- |
| Reply after restart continues the same work | Approval/action/versioned data continuation/input checkpoint commit together; worker releases its lease. `TestDurableApprovalRestartsSameRunAndActionIntegration` creates replacement Services and checks same run/tool IDs, one effect and a fresh provider context. |
| Exact args/resource/constraints/expiry | Canonical full binding and displayed-args hash; current/pinned task and constraints revisions; PostgreSQL expiry. Changed saved arguments test rejects dispatch; Broker's independent authorization tests cover contract/resource/credential metadata and policy checks. |
| Fast replies survive | `TestApprovalDecisionBeforeNotificationReturnsIntegration` decides inside notification delivery before the worker returns; receipt becomes queued and the same action executes once. |
| Repeat/two-channel answers cannot double execute | Durable decision identities, pending CAS and channel proof. Restart test repeats a decision; decision-race test sends concurrent yes/no; wrong-channel checks fail before dispatch. |
| Cancel/change/expiry invalidate permission | Decision-race/invalidation suite exercises task cancellation, changed constraints and expiry. `TestApprovalExpiryWithoutOwnerReplyIntegration` runs the controller sweep; `TestApprovalExpiresAfterDecisionBeforeDispatchIntegration` expires an already-approved action before a new lease can dispatch it. |
| Owner/channel proof | Typed trusted actor; owner-control Bearer token/configured owner for Web, numeric Telegram owner plus saved chat/channel. HTTP tests reject a forged actor header and absent/wrong token; Telegram test rejects the wrong saved chat. Environment/file authority cannot be assigned through database settings. |
| Refusal/expiry explain business blockage | Task/receipt become blocked with approval reason, without changing the waiting run into a system failure. The denial/expiry integration cases check no effect and preserved run history. Cancellation/new revisions are not overwritten. |
| Data continuation and legacy migration | Version-1 JSON contains prepared Butler input/request/binding, no stack or provider-granted authority. Migration audit applies 032/033, rolls them back and reapplies them; historical run/message rows survive, legacy waits expire and no exact action record is fabricated. |

## Validation scope

The lab passed the full Go suite (`./apps/... ./internal/... ./scripts/...`)
with PostgreSQL and Redis. The changed queue, session, approval, orchestration,
API and config packages also passed `go test -race`. Web lint/typecheck and seven
Chromium functional tests passed with the dependency versions already merged
in main. The new multiple-known-result test verifies that fresh recovery includes
earlier effects and that subsequent live results use `SubmitToolResult`.

Native integration regressions also cover same-process approval suspension and
resume, two known observations in a fresh unsigned context, rejection of the old
provider reference, and exact new call/signature correlation on live continuation.
Preparation recovery preserves verified summary coverage and all uncovered
messages while recording the current owner input once. Known failed tool results
retain their recorded error evidence during replay.

The original lifecycle suites inject failures at persisted boundaries, cancel
worker contexts and create new Services against the same PostgreSQL state. The
later lab-only `TestApprovalWorkerSIGKILLRestartIntegration` adds actual Linux
SIGKILL after durable approval suspension, duplicate owner decision and recovery
with the same run/action and a two-connection pool. It does not claim real provider
login. The separate
Broker/browser integration suite supplies real private gRPC/HTTP, PostgreSQL
approval and Chromium credential-effect coverage; the lifecycle fault suites use
transport/tool fixtures to isolate restart behavior.

Run the migration preservation fixture against a fresh dedicated database:

```sh
sh scripts/smoke/durable-migration-audit.sh /path/to/repository postgres-container butler_durable_migrations_unique
```

It refuses an existing database, performs no database deletion and leaves the
database for inspection. Before production validation, retain main's #10
summary/tail logic and direct Transcript Store ownership guards, and provision
the separate owner-control token/owner ID through the lab terminal. Missing proof
must keep owner decisions denied. Network effects, reconciliation, atomic result
publication and delivery outbox guarantees belong to their separate issues;
missing evidence remains blocked rather than being relabelled success.
