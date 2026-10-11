# Opt-in native delegation experiment

Reviewable opt-in preparation for issue16; no native inference has been run by
this scenario yet. The composed production admission/readiness baseline is
`e7ca93e961f6250f66f7ca60142071044f03196f`, with the private-client ambient-proxy
denial fixes `d500847` and `25d9c75`. Execution waits for review of the
immutable runner delta and the operator's coordinated lab window. An unset
opt-in is a visible skip, never acceptance. Component PG/Broker evidence remains
separate from the pending model experiment.

The experiment uses a disposable PostgreSQL network, independent private
deletion anchor, real Broker control route and one shared actual Native3.8Flash
provider through the ordinary core and durable input workers. A second run changes
only the bounded child width from one to two. Selected source references, input
prompt, declared root allowance, owner/day scope, provider and requested model
stay equal. PG stores own durable state; optional transient Redis, profile,
retrieval, summary and auxiliary model adapters are omitted from this fixture.
Children cannot receive ambient parent history/profile, original
ancestor bodies or any tool permission. Candidate contract text is Catalog DATA.

Expected native requests per width are three parent turns (two admissions and a
receipt-only final) and one turn per child. A durable fixture counter reserves each
Start/Continue/Submit attempt before forwarding to the actual provider, including
requests interrupted by process loss. Its global cap is twelve across both
widths/restarts; it never deduplicates away an attempted request. Production
root/delegation/owner-day budgets remain independently enforced. No automatic
retry, model fallback, paid overage, other inference or extra quota probe runs.

The scheduling-gated crash pauses child execution before inference after actual
accepted publication, then kills/restarts the isolated worker process. It changes
neither provider behavior nor owning records. This is a reproducible crash at that
boundary, not an assertion about every possible production crash instant. Child
recovery waits for real claim/lease expiry; fixture SQL cannot revive ownership or
label an unfinished parent completed. Additional stop/source-delete/revision
checks occur after the actual native finals, consuming no extra native requests.
The first process finishes the actual parent run, claims the queued child inputs
and waits at the gate. SIGKILL loses that worker process. A new process builds
one concrete native Provider shared by its ordinary two input workers and waits
for the production 60-second claim expiry. No fixture UPDATE changes claim/run/
lease outcomes. Width one leaves the second child waiting until actual final
collection; width two requires both child claims gated before process loss.

Each child receives its own selected synthetic Russian observation and immutable
contract. The observations intentionally differ. The objective fixture checker
compares exact cited source/result identities, current versions/hashes and bounded
structured values with the immutable source fixture. It does not use model
agreement, narrative confidence or finish reason as verification. Independent
results and conflict remain recorded; parent/task status is not promoted to
completed by a model final or admission receipt. A failed model shape/citation is
a failed experiment, with no corrective inference retry.

Reports contain fixed status/reason enums, source/result/contract/effect references,
hashes, reservation and actual usage metadata, attempt counts and elapsed times.
No source/result body, provider thought, OAuth material, proxy URL or raw error
is printed. Private import/environment references are supplied as opaque
filesystem mounts and imported only by the runtime into encrypted isolated PG.
The runner never reads main credential settings or changes the main Compose,
database, shared source pointer or model selection. Required private files stay
outside source snapshots and all containers/caches remain on the lab storage disk.
The read-only import file is mounted individually. The existing typed auth manager
imports it into this test schema using a new private encryption key, preserves
its encrypted current row across worker reconstruction, and never writes main
PG or the read-only import file. Refresh traffic, if necessary, is authentication
traffic, not an extra inference/fallback. Its credential rotation is an operator
workflow concern to review before the single coordinated experiment.

`scripts/dev/delegation-native-lab.sh ROOT prepare` runs six zero-inference
PG/race groups and compiles both real process harnesses. A separate no-network
probe runs the private-file test as the actual runtime UID10001 under its
read-only root. These groups verify the
16-contender durable cap, reconstruction denial, strict objective checker, private
runtime-file guards, encrypted synthetic import, authentic owner scope and real
Broker → private admission → durable receipt under the exact finite policy.
This preflight has no model provider. Only after review, `execute` additionally
requires explicit `BUTLER_DELEGATION_NATIVE_ACCEPTANCE=1` and a 40-character
`BUTLER_DELEGATION_NATIVE_RELEASE` artifact reference. It runs once; a nonzero
outcome is recorded and never retried. Runtime UID10001 has a read-only root,
no capabilities, bounded memory/PIDs/CPU, a private writable fixture directory,
an isolated DB network and a join to the existing proxy network. Source/proxy/
main containers and settings remain unchanged.

The expected ten model attempts use native `gemini-3.8-flash-tiered`. Separate
PG root scopes allow six turns each; their assigned shared mandate and owner UTC
day allow twelve. All eight dimensions are finite. Native's declared maximum
input token allowance is conservatively reserved before every real request.
`subscription_no_overage` is an explicit operator price basis; zero monetary
reservation is not an observed invoice or permission to use paid fallback.
Reports separate conservative reservations from nullable actual usage metadata.

| Criterion | Experiment check | Status before execution |
| --- | --- | --- |
| Actual shared provider/core/input and private Broker admission | Independent real processes, current PG ownership and exact immutable source-bound args | Pending native run; exact Broker preflight passed |
| Independent selective contexts | Native request validator sees actual prepared DATA refs, denies sibling body or child tools | Pending actual requests |
| Single versus bounded parallel | Same refs/hashes/prompt and shared owner mandate/day; actual child run intervals must show peak one versus two | Pending actual requests |
| Parent process loss | Kill completed parent executor with real gated accepted child claims; await real TTL | Pending actual process-loss run |
| Independent/conflicting results | Strict fixed source/value/date/citation check, separate final/report refs; owner Accept denied without independent persisted verification | Pending actual results |
| Stop/amend/delete | Actual owner controls and Catalog deletion after finals, current historical-source/manager checks | Pending metadata barriers; no late native callback injection claim |
| Shared budgets/restarts | Durable pre-call cap twelve; real root/delegation/mandate/day ledgers and actual usage | Cap and finite Broker policy passed; model usage pending |
| One loaded local model rather than per-agent weights | Cloud internals cannot be observed by this runner | Separate shared local service evidence required; no cloud-weight claim |
| Arbitrary mid-native crash, blocked child historical report bridge | Outside this narrowly scheduled experiment | Pending separate evidence; never synthetic PASS |

Still pending: immutable delta review, coordinated actual execution, objective
quality/latency/usage comparison and final criterion audit. Native output shape,
citation or configured parallel overlap failure is a failed experiment without
repair inference. Nothing in this document establishes issue closure or an
executed native PASS.
