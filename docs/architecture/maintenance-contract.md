# Independent maintenance and rollback

Status: proposed implementation contract for issue21. This document does not
enable updates or establish release acceptance. Ordinary skills and tools retain
their existing admission contracts; diagnostics, a generated patch, a green
health endpoint and a model verdict grant no deployment authority.

Recovered from design commit e28a53ba for current-main local work on2026-10-25.
Implementation and independent acceptance remain incomplete; no production
updates are established by this local implementation/acceptance task.

Implemented so far: conservative inventory comparison, separate signed-review
integrity/qualification, immutable PostgreSQL evidence, profile/stage storage,
owner stop epochs, signed node-observation storage and owner-permission
registration/revocation with immutable request nonces, finite allowances and
no refund or resurrection; transactional claim reservation, one-use live dispatch
admission and immutable signed node-outcome storage.
Reverse-stage rollback permission is separate from the original forward report.
These are component boundaries only. Independent build/checker and
controller/node processes,
physical effect execution, trial/promotion/rollback and active-state preservation
acceptance remain pending. No deployment profile is enabled.

The local Store consumes allowance and advances the fence with its pending
claim. Dispatch admission rechecks the current authority and admits that claim
once. Stop cancels pending work without refund, retaining already dispatched
uncertainty. Deadline expiry is terminal UNKNOWN; late signed measurements are
retained without rewriting it. Outcome storage never clears newer owner stops,
restores PostgreSQL or replays an action. These owning APIs are components only;
independent checker/node processes and current state compatibility remain unimplemented.

The separate optional `maintenance-controller` now has a local-only owner control
entrypoint, default disabled. It requires an already installed canonical operator
profile, matching private signing seed, separate private owner credential and
database reference files. It binds only a configured loopback address, performs
no migration/profile installation, and exposes no node deployment or arbitrary
signing operation. Its bounded authenticated private HTTP endpoint follows the
existing owner control-plane pattern, independently of Core/channel ingress;
it is not the normal service/tool execution transport. A separate owner-process
test with Core absent proves authenticated stop of an enabled synthetic target
and preservation of that stop across process restart. Node/checker execution
and deployment remain pending.

Unknown reconciliation requires a new current signed node observation under the
node's exclusive effect fence, with the exact settled attempt and measured base
or candidate identity. Identity of a merely running process is insufficient.
Reconciliation retains terminal UNKNOWN, spent permission and newer owner stops;
the physical executor/inspection implementation still requires acceptance.

Source preparation reads complete immutable commit blobs from a fixed operator
repository, not export filters or dirty author checkout files. Snapshots live
outside the author tree, retain full hashes/Git modes, and reject links,
submodules, aliases and non-commit identities. Preparing source neither executes
it nor proves its built artifact: the independent sandbox build remains required.

The bounded compiler/packager now passes actual local component checks. Its
fixed private configuration pins a compiler image ID and module directory;
submitted Dockerfiles, tests, generation instructions, URLs and shell fragments
cannot select the compiler. It rechecks the complete snapshot, stages measured
bytes into an owned source volume, mounts source read only for Go compilation,
disables cgo/toolchain/module downloads and all networking, and uses bounded
memory/CPU/PIDs/tmpfs/time. The first profile supports dependency-free Go
capsules and fails on unavailable imports. It reads the actual bounded static
ELF without running it and can import only a fixed scratch package containing
that ELF. Actual image ID, executable, compiler and complete source identities
stay separate. Compilation/packaging grants no review, trial or installation
permission. Actual daemon compilation/import passes; independent release checker
and deployment acceptance remain pending.

Before compiler dispatch, the builder requires measured Linux
cgroup v2/cgroupfs limits and the exact host-visible worker PID. Actual
memory/CPU/PID, UID/no-capabilities/no-new-privileges and IPv4 route observations
must match the bounded profile. Effective no-swap needs a real zero at the worker
or an ancestor, not a Docker warning or "max" value. Hidden/unsupported physical
state denies compilation. The private ancestor workaround remains an actual
kernel measurement, never a general assumption about another daemon/platform.
These checks pass against the actual private worker. Its fixed trusted PID1
supervisor waits for a host marker before invoking Go and has independent finite
wait/compile/output-hold deadlines. The host opens the measured worker's `/out`
tmpfs with `os.Root`; a request cannot select a host path. Status publication uses
atomic rename. Earlier failed Docker exec/copy API probes remain failures; the
working path adds no privilege and does not loosen resource limits.

Read-only Docker inspection is now a local node component. Its private startup
configuration fixes a local Unix socket and exact literal container name; no
model request selects a target, daemon or URL. Artifact identity is Docker's
actual sha256 image ID, not a tag, label or health result. Runtime identity
canonically hashes the entire measured Config except its separately bound Image,
the entire HostConfig, mounts and network name-to-ID membership. Operator-owned
targets must keep deployment configuration such as hostname stable; a changed
configuration is a different runtime, never silently accepted as compatible.
Raw configuration/environment and daemon error bodies remain private to the node.
Missing state is explicit; ambiguous/oversized JSON, partial state, transient
Docker lifecycle, redirects, unavailable socket or cancellation cannot produce a
qualified observation. This component neither signs arbitrary data nor asserts
effect settlement. Its public tests use synthetic responses over an actual local
Unix HTTP listener. A separate opt-in Linux/race test now inspects the exact
stopped test container in a private local Docker28.3.3 daemon. The daemon uses
an isolated mount namespace and separate8GiB store; a trusted public fixture
proves effective UID/memory/CPU/PID/capability/egress and host-file/socket bounds.
Swap is prohibited at its private ancestor cgroup because this host's Docker
driver does not expose the per-container swap limit. The initial failed
pivot-root/swap probes remain failures. This is inspection/isolation component
evidence only; independent checker/node/deployment acceptance remains pending.

The separate `maintenance-node` observer is default disabled and passes an actual
separate-process identity test. Private startup pins the profile, node key and
literal Unix socket/container. Its one-shot canonical challenge contains only
profile digest, epoch and nonce. The process signs its own physical inspection;
the caller cannot supply identity or settlement fields. It always leaves effect
settlement false and cannot reconcile UNKNOWN. Store currentness remains required
after the response. This source is not a node executor or functional acceptance.

The initial classifier treats existing apps/cmd/internal and deployment/checker
paths as protected. Ordinary automatic updates require explicitly pinned isolated
module capsules; no existing monolithic service is reclassified as ordinary by a
path boundary. Kernel permission remains exact and separate from ordinary grants.

## Owners and scope

The author prepares a bounded separate source/artifact version. The independent
checker owns fixed regressions, hidden cases, thresholds and compatibility checks.
The maintenance controller owns exact owner maintenance permission and durable
release state. A narrow node executor owns deployment of a fixed operator target.
The controller and executor run independently of the model and core process.
They expose no general shell, Docker API, repository path or credential lookup to
the author. PostgreSQL remains the durable source of maintenance decisions.

Changes are classified from the independently measured old/new source inventory,
not an author-supplied label. Classes are skills, ordinary tools, model
configuration, application modules and protected kernel. Unknown paths or mixed
changes inherit the more restrictive class. Policy, approval, owner controls,
credential mediation, journals, fencing, revocation, sandbox/egress, checker and
updater are protected. Updating the maintenance controller itself is an external
operator operation, outside its ordinary deployment targets.

An operator profile pins the supported module, its current immutable artifact,
allowed path/class boundaries, checker and regression identities, protocol/state
compatibility, fixed deployment topology and finite trial limits. Candidate data
cannot select another profile, checker, executable, network, mount or secret.
Protection is checked against the actual diff and built artifact, including
deployment/configuration changes. An ordinary repair permission cannot approve
changes to its own enforcement.

## Versioned records

The local implementation uses migration105, after current-main104; recheck that
number before merge. The former draft's059 reservation is obsolete.
Immutable records bind owner,
target/profile revision, exact base and candidate artifact digests, source/diff
digests, changed inventory, schema/protocol identities and measured classification.
An independently issued report binds that entire candidate, the fixed checker
binary/corpus and criteria, separately recording fixed regressions, hidden cases,
state compatibility and functional smoke verdicts. Author tests are additive.
The author cannot rewrite reports or replace failing cases with easier cases.

A separate owner maintenance mandate selects the fixed target/profile, permitted
class and source/version boundaries, criteria, finite budget and expiry. Within a
preapproved ordinary module, the controller may derive a one-use exact deployment
grant after independent checks, without another owner message. That grant binds
the measured known base and candidate, exact changes/report, trial deadline and
retained rollback artifact. A changed base must be checked again; a tag is not a
version boundary. Protected-kernel changes require explicit owner permission for
the measured protected diff, candidate and report, rather than a grant inferred
from an ordinary mandate. A general repair request or tool-installation mandate
is insufficient. Artifact tags and labels do not replace measured identities.

State separates staged, checked, trial_pending, trial_running, ready,
promotion_pending, promoting, enabled, rollback_pending, rolling_back,
rolled_back, blocked, unknown and stopped. A successful process start is not
checked, ready or enabled. Each effect attempt has a stable ID and monotonically
increasing fence; a transport timeout after dispatch becomes unknown and is never
automatically replayed. Node reconciliation must independently observe the exact
current artifact and deployment attempt before further action.

## Independent checks and state preservation

The checker executes the immutable candidate in a bounded isolated environment
with a synthetic database and runtime credentials. Owner credentials, live
transcripts, memory, arbitrary host files and the Docker socket are absent.
The checker binary, hidden cases and thresholds are operator-owned, mounted read
only and outside the author's workspace. The candidate does not publish its own
passing result. A failure to run or inspect a required check is a failure.

Checks exercise actual functions through their service boundaries: ordered input
acceptance, lease fencing, current authorization, approval suspension/recovery,
tool/result privacy, durable task/result and channel delivery as applicable.
Alive, ready and functionally working are distinct observations. Provider-dependent
checks report unavailable when unavailable; they cannot become zero-cost success.
Quality/error/cost comparisons use the same held-out cases and disclose unknown
usage rather than replacing it with zero.

Before a trial/promotion, the controller asks the owning modules for current
contract/state compatibility of active tasks, accepted inputs, approvals, grants
and effect/checkpoint continuations. It pins the current schema and protocol
identities and rejects incompatible active continuations before deployment.
Conservative blocking is explicit; incompatible state is never silently resumed
under a new contract. Compatible updates retain stable IDs, revisions, ordering,
source privacy, owner controls and current approval decisions.

The retained backup includes the known executable/configuration and a bounded
state-recovery record. It is not authority to overwrite live PostgreSQL. In
particular, rollback must retain newer owner stops, deletion/tombstone anchors,
accepted work, effect evidence and approval decisions. It must not undo external
effects or replay unknown ones. Secrets are retained only by their existing
credential subsystem, never copied into maintenance artifacts or model context.

Automatic application-module updates require unchanged state schema. A migration,
including an allegedly reversible one, requires a separately reviewed exact plan
and explicit owner permission. Irreversible conversion requires its own recovery
procedure and cannot be called automatically reversible. Executable rollback must
be compatible with the current schema; inability to establish that blocks rollback
and identifies the remaining operator action without restoring an old database.

## Trial, promotion and stop

The trial uses a fixed isolated target, synthetic inputs and finite time/call/
resource limits. It grants no live work or new permission. The independent checker
records actual functional outcomes, retained artifact identity and the trial's
measured costs/errors. Promotion needs successful current checks, current owner
permission, an unchanged known base, preserved compatibility, a valid rollback
artifact and current finite budget.

The controller commits a one-use deployment claim and exact effect identity before
contacting the node executor. No database transaction holds locks over network or
Docker operations. The executor checks the current claim/fence immediately before
the fixed target change, then reports measured artifact identity and outcome.
If stop races an already admitted effect, the actual outcome remains completed or
unknown; stop is never rewritten as proof that the effect did not occur.

Owner stop uses an independent authenticated controller path and durable epoch,
available while the main agent is down. It prevents new claims and promotions;
loss of controller/database authority denies new changes. Recovery does not
clear the stop or manufacture permission. A separately authorized exact rollback
may remain available to return to the retained compatible executable; it cannot
deploy an arbitrary artifact or lower protection. The author/model cannot unstop.

Initial Docker Compose deployment must keep this controller optional and default
disabled. Main services and author runtimes receive no additional Docker or root
capability. The fixed node-executor exception is justified only by deployment and
does not extend restart-helper's existing restart-only responsibility.

## Required isolated acceptance

Acceptance requires actual separate checker/controller/executor processes and
retained PostgreSQL history, not interfaces backed only by fixtures. Prove a good
candidate, broken release, live-but-functionally-broken release, incompatible
schema/continuation, author-modified tests, attempted protected-policy change,
concurrent version change, owner stop before/during a deployment, lost response
without replay and return to a compatible working version. Keep active tasks and
approvals across a compatible process update and explicitly deny incompatible
continuation. Report the original task's independently checked benefit and measured
error/quality/cost change. A component proof alone does not close issue21.

The current owner instruction confines execution to this PC. Use isolated local
processes, storage and synthetic effects. No SSH, remote test runner or update of
the working Butler deployment is part of this task. Keep exact source/artifact
versions and failed/unknown observations; local acceptance never implies rollout.
