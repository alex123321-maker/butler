# Owned converter parent continuations

Migration055 adds two bounded phases of the existing approved developer workflow.
It creates no owner message, goal, child permissions or semantic acceptance.
Actual053 admission atomically writes two content-free WAITING intents:

- `candidate_return`: ordinary collection of the tested054 candidate, independent
  parent checker/trial and separately authorized activation.
- `activated_catalog`: a fresh ordinary attempt on the original parent task,
  after confirmed activation in the first phase, using its current tool catalog.

The immutable identity pins owner, parent/constraints revisions,053 hash, original
run/input, deadline and the complete original Catalog graph. The ready projection
contains hashes, SourceRefs and actual action/effect IDs. It contains no document,
procedure, child narrative or report body. One ordered input and the once-only
WAITING→QUEUED CAS commit together. Restart cannot reset that identity; rollback
refuses retained055 history.

## Admission and current authority

`app.ConverterContinuations` owns bounded maintenance: four intents per sweep,
fair content-free cursor and five-second enqueue bound. Lock order is current
task tree→ownership→session admission→runs/task→goal/mandate→sources→effects/budget.
A live parent is skipped under ownership before exclusive session/run locks,
preventing maintenance from blocking its active verifier. With no ownership row,
session admission still serializes a new lease before the final idle check.

The original input/run and immutable final must be completed with exact pinned
task/constraints revisions. The actual delegation dispatch must be confirmed.
Both061 and053 identity, or unknown identity, deny without reader fallback.
Actual054 requires the independently tested package, three confirmed child actions
and retained final/report. Child completion alone is insufficient. The original
working document remains a privacy dependency while only selected Need/contract
bodies reach the child.

Current parent purpose/tool/resource authority must permit collection, checks,
trial, activation and original conversion. Prepare-only, revoked or expired rights
cannot become activation permission. Full original/prior-parent pins, graph,
restore anchor and current parent scope grants are rechecked before enqueue.
The second phase also requires the first055 input's actual immutable final,
its confirmed exact activation action/effect and current installed digest/generation.
Neither a model final nor an unrelated activation substitutes for the receipt.

Existing budget uncertainty blocks continuation. Prospective next-turn admission
reads immutable owner/day/root/mandate/delegation allowances without creating,
refilling or reserving scopes. Every actual provider/tool still reserves normally.
Maintenance never calls a provider or runtime.

## Private input, recovery and publication

The accepted `converter-return` input contains only `{task_id,continuation_id}`.
`input.Store.VerifyConverterContinuationTx` requires the exact private queue claim,
current worker run/key/session/owner, lease/fence,055 identity and fresh DB-clock
claim/input/intent expiry after waits. Owner Actors and forged payloads deny.
The existing context-only delegated input verifier remains strict and unchanged.

`Config.ConverterContinuations` resolves proof after ordinary lease acquisition
and pins the entire original/prior-parent graph into the new run. Core replaces
the synthetic ingress OwnerInput with a compiled kernel label and separate private
SubagentOutput DATA readiness. Original owner context and truthful tool observations
remain intact. Durable preparation retains the opaque event and DATA reload refs;
recovery repeats the private claim and current proof. No provider reads PG.

`generatedtools.CheckContinuationRunTx` composes after Catalog/current authority
at provider release/recovery, every Broker dispatch and atomic final. It rechecks
fresh fence/claim/deadline after source waits. Converter publication repeats it
locally. Ordinary inputs gain no055 capability. Deletion, amend/stop, revoke, stale
fence, unknown effects, expiry or missing sources block late release/publication.
Collection is still a separate Broker action granting only candidate/Need/contract
sources; the parent gains no child report/final narrative. Tasks remain unverified.

The compound owning converter verifier advertises a static two-second check bound.
Core accepts only200ms..2s; invalid bounds deny. Ordinary Catalog checks keep200ms,
parent cancellation/deadlines clamp the bound, and checks before every provider
call, final/tool proposal and periodic250ms observation remain uncached. Longer
bounded verification latency grants no authorization grace: timeout denies release.

## Production composition

Constructors enable no deployment. Under the existing explicit converter-delegation
opt-in and immutable053/054/055 readiness, compose:

1. `NewConverterContinuations(pool, tasks, inputs, catalog, budgets, allowance,
   profile)` using the same owned pool/profile and a known finite model quote.
   `OutputTokens` must equal `budget.Policy.OutputTokens`.
2. `converterAdmission.EnableContinuations(service)` before accepting children.
3. `flow.Config.ConverterContinuations = service`; join `service.Sweep` with
   existing common maintenance, retaining both errors.
4. Compose `CheckContinuationRunTx` after Catalog in every Broker/final data guard,
   preserving role/skill/converter/version guards. Provider/recovery uses the
   owning `converterSources` adapter and its fixed bound.

Allowance must match actual enforceable provider request limits. A smaller policy
than a provider's declared output cannot be represented by a bookkeeping number
alone: enforce the finite request cap or deny admission. Current main declared8192
matches policy. Core bootstrap now composes these adapters under the existing
developer opt-in and immutable055 readiness. A provider whose enforced output
limit is below policy gets the same conservative policy quote as ordinary model
admission; an output limit above policy denies startup instead of being relabeled.
Broker's global data guard and atomic completion retain055 checks independently
of new-admission enablement. Final deployed acceptance remains separate; no
model-visible continuation endpoint or permission field is introduced.

## Evidence and limits

Actual isolated PostgreSQL MaxConns2/race checks cover eight concurrent once-only
enqueues, restart, unchanged budget ledger, atomic queue rollback, original/report
deletion, stop/amend, exhausted budget, live parent, unknown dispatch, expiry after
waits, malformed owned schema, missing/forged claim, stale fence, owner Actor denial,
recovery and late-final denial. A held-run-lock regression checks that maintenance
skips a live parent before session/run locks. Slow current real-PG verification
beyond200ms passes; deletion during slow final verification, cancellation and
invalid/expired bounds release no final.

Snapshot `20261011-110401-019` passed production Broker, actual network-none
converter and ordinary durable parent/child workers with app race detector: one
owner input, two055 continuations, same original task, exact checked original JSON,
ten confirmed actions and four authoritative unverified finals. Automatic26.18s,
explicit continuation9.18s, direct Broker1.28s. The scripted provider has no
PG/network API. Its finite5s turn quote changes no production quotas. Native calls0.
Snapshot `20261011-110304-210` passed affected actual-PG/race budget/generatedtools
and full converter app cases (app136.116s).

Final source snapshot `20261011-111023-895` repeated the actual process: automatic
28.42s, explicit8.94s and direct Broker1.76s. It also checked exactly14 durable
finite model admissions across the four runs, one owner input and two internal
inputs. Actual-PG/race once-only/restart, unchanged ledger and held-run-lock checks
passed10.782s; affected `go vet` passed.

This proves automatic owning process integration. Native model planning,
independently measured improvement and final deployed converter-developer acceptance
remain pending. Technical candidates, local ACKs or scripted execution cannot
establish verified owner benefit.

Activated055 preparations reload bounded confirmed tool observations from the
exact completed preceding ROOT attempt. Each qualified run/call must have a
matching owner/task/constraints and goal/mandate binding, confirmed effect,
completed authorization/dispatch and retained canonical runtime-result evidence.
Missing, unfinished or uncertain journal/transcript rows deny. Child reports and
unrelated session/task histories are never selected. Original sources and the
private claim/current owner proof are checked again after the reads.

Observations remain private sanitized DATA, not native function responses,
semantic owner verification or execution grants. Results retain actual artifact
and check receipts. Original authorization hashes stay exact; sanitized arguments
and equivalence hashes are included only when stored JSONB arguments match that
exact hash. Otherwise arguments are omitted, without reconstructing numeric
lexemes. Context is bounded to32 observations/256KiB. Recovered preparations
discard and reload the projection under current guards.

Only a private automatic055 continuation blocks equivalent confirmed non-read
proposals before slot admission or Broker preparation. Object-key order, numeric
spelling and a fresh call ID cannot repeat a verified prior mutation. Unavailable
prior equivalence makes a non-read proposal for the same tool fail closed. Repeat reads require both the
prior read effect and a current active trusted read-only catalog contract.
Ordinary owner inputs retain distinct intentional action slots. There is no
fabricated ToolResult, prior-effect replay, refund or permission bypass. Current
task/source/lease, Stop, budget, final and Broker guards still apply. Private proof
reloads before proposals use a two-second maximum clamped by the live caller.
