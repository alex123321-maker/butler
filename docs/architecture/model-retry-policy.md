# Bounded durable model retry

Core may retry one failed model turn per accepted input after a nominal model
turn timeout. The normal durable input worker queues the retry with a two-second
pause and releases its lease. The next claim obtains fresh Session ownership,
checks the current task and sources, reloads the authorized tool catalog and
starts a fresh provider session for the same nonterminal Butler run.

This policy applies to the production composition with durable checkpoints,
execution budgets and current-source verification. The transport does not retry
HTTP requests internally. Each actual Start, Continue or Submit request receives
its own committed model operation and conservative budget reservation.

## Eligibility and limits

Budget admission records whether the effective deadline comes from the nominal
model-turn limit or an earlier task, mandate, owner, delegated or caller deadline.
Only the nominal model-turn timeout can claim the retry. A typed provider timeout
is eligible only under that same trusted admission and a live parent context.
Provider text and a provider's `retryable` field grant no authority.
The request-local phase observer must have reached HTTP round trip or response
stream. Missing phases and timeouts during authentication or serialization deny
this retry. A phase remains content-free telemetry and proves no measured cost.

Cancellation, owner Stop, expired authority, exhausted budgets, changed sources,
authentication or protocol failures and uncertain external effects do not enter
this retry path. Core checks these conditions before scheduling and again under
the next claim. An unfinished tool call or an executing/unknown effect anywhere
in the task tree denies recovery.

The input's original acceptance deadline, ordinary recovery-claim allowance,
task/mandate/day scopes and any delegated allowance continue to apply. The
retry consumes a recovery claim and a new model reservation. No timeout,
restart, successful later answer or unknown cost refunds a spent reservation.
The default input limit remains three execution claims, including initial work
and crash recovery. The automatic timeout retry allowance is one per input.

## Durable recovery boundary

The owning input store atomically saves `checkpoint_json.model_retry` under the
exact live input claim and execution fence. The record binds count one, the
trusted timeout reason, the prior committed operation, a queued state and a
database-clock `claimed_at` and `not_before` exactly two seconds later. The prior operation must be the latest model
admission for that run. Normal checkpoint replacement preserves the retry record.

Recovery validates that record and the current authority. After the new budget
reservation, it stamps the record as started, binding the fresh operation before
transport execution. A crash after admission or that stamp cannot start another
fresh request from the old queued marker. Subsequent turns keep the exhausted
retry count. A second timeout follows the ordinary blocked/failed path.
An exhausted or ineligible nominal deadline cancels the provider context through
the normal budget-stop path. The retry must not bypass that cancellation.

The failed provider context is discarded. The fresh session receives Butler's
prepared input plus committed tool results as provenance-tagged user DATA.
Partial model output and provider-side session state establish no completion or
permission. Terminal runs are never reopened. Saved finalizing results continue
through the existing persistence-only recovery path.

## Tool and delivery safety

Recovery reads known tool receipts and never dispatches those stored calls again.
During timeout recovery, Core also checks new proposals against the committed
receipts by tool and canonical argument hash before action preparation or Broker
dispatch. An equivalent confirmed non-read proposal is denied even if the model
chooses a new call ID or changes JSON key order or numeric spelling. Repeated
reads require current trusted catalog and resource semantics that prove they are
reads. Missing metadata grants no read exemption.

Ordinary explicit repeated intentions retain their existing separate action-slot
identities. All admitted new actions still pass normal authorization and Tool
Broker execution. This model retry does not retry channel delivery, replay an
unknown external effect, or create another final notification.

## Evidence and acceptance

The original failed model attempt and its measured usage remain in the budget
ledger. Missing provider usage stays NULL. A successful retry cannot supply the
earlier request's measurements. Request-local provider-stage observations retain
only the allowlisted phase and do not establish cost or successful inference.

Issue #33's closed attempt537 remains NO_GO with unknown usage. Its original
human exception and the original prospective V6 protocol remain immutable.
Before any V6 native execution, a later direct human instruction permitted a
narrow expense exception for a proved nominal response-stream timeout followed
by the single safe retry. That failed request may retain three NULL usage
dimensions and a NULL observed version; partial or conflicting measurements do
not qualify. Every successful request still requires complete measured usage and
the selected observed version. The failed reservation stays spent, unknown
dimensions are counted separately, and the exact owning retry and eventual
scenario result or owner-Stop guarantees must pass independent checks. All
safety, quality and resource criteria remain. See the
[prospective protocol](../ai/scenario-release-chain-v6-high-plan-20261013.md).

The zero-inference regression runner is
`scripts/dev/model-retry-lab.sh ISOLATED_AGENT_ROOT EXACT_COMMIT`. It uses disposable PostgreSQL,
synthetic providers, race/vet and the same production worker/Core/budget path.
Native High release-chain acceptance is a separate once-only gate.
