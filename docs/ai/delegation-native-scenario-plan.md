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
The parent's final must contain exactly the two actual admitted child IDs and
no additional verdict fields. Report publication also checks that durable native
attempt rows and independent production model reservations are complete; a
failed evidence read makes the experiment fail. Timing reports separate the
remaining real claim-expiry wait from total recovery plus child execution.

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

The first reviewed execute invocation on snapshot `20261011-093941-771`
stopped at the host import metadata gate: the shell user cannot traverse the
private UID10001 import directory. No execute database/container, import or
native request was created. The content-free record is
`deploy/delegation/native-preflight-blocked.json`. No second invocation was
performed under that one-window authorization; all actual model criteria remain
pending. The host metadata gate needs an explicitly reviewed exact-file runtime
check before a separately coordinated experiment can proceed.
The corrected gate uses a no-network, read-only, capability-free UID10001
metadata helper. Only the fixed literal `antigravity-cli` parent is mounted for
that check; it rejects a symlink or nonregular approved basename, wrong owner or
mode, empty file, or size above 64KiB. It never reads or prints file contents.
The inference runtime still mounts only the single approved file read-only.
No host ownership, profile permission or credential content changes are made.

## Coordinated actual run

The single actual experiment used immutable runner
`85bef824e838caef64407891d5b228f982c51032`, snapshot
`20261011-094307-366`. It **failed** after three native requests: one parent
StartRun and two real tool-result continuations. Two actual admission receipts
were committed; the parent input completed and the gated worker process was
SIGKILLed. The restarted worker waited for the original claim expiry, then a child
input stopped before its first native request. No second execute, fallback,
corrective generation or quota probe followed.

The exact content-free runtime report is
`deploy/delegation/native-scenario-failed.json`, SHA256
`28c75c31827f8a25ca8ed101e6e04b8264a7e66ee9ce18808d504d5af748c8f0`.
Private lab evidence is
`/srv/storage/butler/agents/durable-tasks/reports/delegation-native-execute-20261011T024417Z-3147832`;
test event SHA256 is
`f8eb2ad59f073000bec6546ad4212d6535634b219d579ec9663e04e4778d575d`.
The six zero-inference preparation groups and runtime UID probe passed again
before the actual invocation. The earlier host metadata denial remains separately
recorded; this actual failure is not replaced by preparation PASS.

| Observation | Actual evidence | Limit |
| --- | --- | --- |
| Native attempts | Three committed pre-call charges; actual parent with two Broker/private admissions | No child native request or width-two run |
| Durable scope | Same two selected source refs/hashes and two child reservations; parent input completed before process loss | Child selective prepared context and independent final results not reached |
| Process loss | Actual SIGKILL; 48,637ms original claim-expiry time remained at restart | Recovery ended with stopped child input; successful child survival is unproven |
| Runtime timing | Width-one failed after 66,904ms; whole test 69.45s | No successful single-versus-parallel latency comparison |
| Shared budgets | Owner-day reserved three model turns, 3,170,304 tokens, two external calls and two children | Conservative allowance, not observed spend |
| Actual model usage | Final parent reservation observed input 5,526/output 101 tokens; preceding two rows have null usage | Full token total and invoice unavailable; null is not zero |
| Objective acceptance | None of the child source/result checks ran | Parent receipt-final checker, conflicting results and Accept denial not reached |
| Stop/amend/delete | Post-final metadata controls did not run | Their false report defaults mean unperformed checks, not negative-control PASS |

The report's empty child result arrays and false unfinished-phase flags preserve
the failed boundary; they must not be interpreted as completed observations.
The content-free report did not establish the exact child stop reason.
Disposable schema cleanup ran after failure; the original private PG files were
preserved, but their fixture schema had already been removed. Issue16 acceptance,
single-versus-parallel comparison and closure remain unestablished.

## Zero-inference recovery diagnosis

The isolated reproduction used actual PostgreSQL with two pooled connections,
real separate Broker/private admission, current Catalog and the ordinary input
workers. Its provider only counts entry and synchronously denies; no native
provider, credential import, proxy, model stream or model response is involved.
Before the fix, both fresh and naturally expired queued claims stopped before
provider entry even though current Catalog loading and MarkExecuting passed.

The worker correctly retains its accepted input ID as the run deduplication key.
The delegated envelope validator incorrectly required that rewritten key to equal
the original kernel event ID. Production fix `2233c85` adds a typed owning-store
verification of the actual current claim/run/fence, session owner, active pinned
task and exact original refs-only event/contract source. It checks current PG
clock after waits and preserves the real accepted input ID in run/checkpoint
identity. Strict original direct-core events retain their existing key contract;
arbitrary keys and missing verification remain denied.

Snapshot `20261011-100822-713` passed seven top-level tests and 21 subtests under
the race detector, with no failure or skip. Fresh and naturally expired worker
claims each reached the entry-denial spy once. Six altered or unverified handoffs
reached it zero times. Direct real-PG checks denied wrong owner, claim, fence,
session, run, source, run dedup key and task revision. Real task-row lock waits
crossed claim and input deadlines naturally; both were denied after release.
The existing original-key preparation/recovery checks and the new accepted-key
checkpoint retention check also passed.

The content-free record is `deploy/delegation/native-recovery-diagnosis.json`.
Private event evidence is
`/srv/storage/butler/agents/durable-tasks/reports/delegation-native-diagnosis-20261011T030825Z-3260863`,
SHA256 `3164f43494db65981c18e1c96ee3783b0aa39f9c0b26b8224b18be81515e43a7`.
`scripts/dev/delegation-native-diagnose-lab.sh` reproduces these checks without
native auth or inference. This diagnosis spent zero new native requests; the
failed actual experiment still records three. A provider-entry regression PASS
does not replace the failed native run or establish delegation acceptance.
